// 分拣工位的会话状态机（纯 TypeScript，node:test 直接测）：合并拣货的波次拿回来的货，一件一件按出库单分进周转箱。
// 判断一律在这里；组件只管摆与接线。
//
// 到 Go 端核过的规则（用例是 application.WaveHandler.Sort；载荷由 registrationjson.SortCommand 译；口在
// warehousehttp.NewRecordSortEndpoint，挂拣货那一种作业事实）。屏上的话照这些说：
//   - 一次投放说清波次、出库单、商品、件数与周转箱，箱号必带。记成答 SORTED；同一个事实号同样内容重放答已有结果。
//   - 件数不能超过这张单的这种货在这个波次里已经拣出来而还没投放的，超了答内容不合（CONTENT_INVALID）；
//     不用分拣的波次（逐单拣、单品单件）也答内容不合。
//   - 箱照拣货绑箱的规矩：空着就绑到这张单上，一张单可以用几个箱，一个箱同一时刻只装一张单。没登记 TOTE_NOT_FOUND、
//     已停用 TOTE_NOT_ACCEPTING、绑着别的单 TOTE_IN_USE，这一笔整份不成立。
//   - 已复核、已取消的单不再收投放（STATUS_MISMATCH）。投放不动库存。
//   - 一张单在合并拣货的波次里拣出来的货没分完，复核不收（SORT_PENDING）；分完才能去复核打包。
//
// 扫码顺序：扫一件货 → 屏上说它归哪张单、放进哪个箱 → 扫那个箱确认（这张单还没有箱就扫一个空箱）→ 记下，回到扫货。
// 扫箱确认是为了防放错：货认对了、手放错了箱，后面复核才发现就晚了。

import type { GuidedFormValues } from '../guided-forms';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import type { WorkFactVerdict } from './work-fact-submit';

export interface SortLine {
  goodsCode: string;
  unit: string;
  /** 拣出来的件数。 */
  picked: number;
  /** 已经投放进箱的件数。 */
  sorted: number;
  /** 拣货的人还没拿回来的件数。 */
  open: number;
}

export interface SortOrder {
  requestId: string;
  /** 单据号；没有就用请求号示人。 */
  documentNumber: string;
  ownerAccount: string;
  /** 这张单此刻绑着的周转箱。 */
  totes: readonly string[];
  lines: readonly SortLine[];
}

export type SortPhase =
  /** 等一件货。 */
  | 'unit'
  /** 正在认一个码是哪种货（读口在途）。 */
  | 'resolving'
  /** 货认出来了、归了单，等扫箱确认。 */
  | 'tote'
  /** 这一次投放正在提交。 */
  | 'submitting'
  /** 没送到：等「再送一次」。 */
  | 'unsent';

export interface SortFeedback {
  kind: 'waiting' | 'ok' | 'wrong' | 'unsent';
  text: string;
  scanned?: string;
}

export interface SortTarget {
  requestId: string;
  goodsCode: string;
  /** 这一次投放几件：平常一件，点了「剩下的都放进去」是这张单这种货还没分的全部。 */
  quantity: number;
  /** 这张单已经有箱，还要再加一个：下一个扫到的空箱收下。 */
  adding: boolean;
}

export interface SortSubmission {
  requestId: string;
  goodsCode: string;
  quantity: number;
  tote: string;
}

export interface SortState {
  waveId: string;
  phase: SortPhase;
  orders: readonly SortOrder[];
  target: SortTarget | null;
  feedback: SortFeedback;
  submission: SortSubmission | null;
  /** 这一趟投放了几件。 */
  put: number;
}

export type SortEffect =
  /** 扫到的码不是哪张单的商品编码：按这几位货主各认一次，看它是不是登记过的条码、平台商品码。 */
  | { kind: 'identify'; code: string; owners: readonly string[] }
  | { kind: 'submit'; submission: SortSubmission }
  | { kind: 'resend' }
  /** 后端说这一笔不合：屏上的分拣情况多半旧了，重读一次。 */
  | { kind: 'reload' };

export interface SortStep {
  state: SortState;
  effects: readonly SortEffect[];
  cue?: ScanCue;
}

const waiting: SortFeedback = { kind: 'waiting', text: '等你扫货。' };

export function startSort(waveId: string, orders: readonly SortOrder[]): SortState {
  return { waveId, phase: 'unit', orders, target: null, feedback: waiting, submission: null, put: 0 };
}

/** 重读回来的分拣情况换上去。认货、扫箱、提交在途时不换：那一步是照旧的那一份判的，换了就对不上。 */
export function reloadPlan(state: SortState, orders: readonly SortOrder[]): SortStep {
  if (state.phase !== 'unit') return { state, effects: [] };
  return { state: { ...state, orders }, effects: [] };
}

const step = (state: SortState, cue?: ScanCue, effects: readonly SortEffect[] = []): SortStep =>
  cue === undefined ? { state, effects } : { state, effects, cue };

const wrong = (state: SortState, text: string, scanned: string): SortStep => step({ ...state, feedback: { kind: 'wrong', text, scanned } }, 'wrong');

const leftOf = (line: SortLine): number => Math.max(line.picked - line.sorted, 0);

/** 屏上示人的单号。 */
export const orderName = (order: SortOrder): string => (order.documentNumber === '' ? order.requestId : order.documentNumber);

/** 这张单还有几件拣出来了没分。 */
export const orderLeft = (order: SortOrder): number => order.lines.reduce((sum, line) => sum + leftOf(line), 0);

/** 这张单还有几件没拣回来。 */
export const orderOpen = (order: SortOrder): number => order.lines.reduce((sum, line) => sum + line.open, 0);

/** 这张单分完了：拣出来的都进了箱，也没有还没拣回来的。一件都没拣出来的不算分完。 */
export function orderSorted(order: SortOrder): boolean {
  const picked = order.lines.reduce((sum, line) => sum + line.picked, 0);
  return picked > 0 && orderLeft(order) === 0 && orderOpen(order) === 0;
}

export interface SortSummary {
  orders: number;
  /** 分完的单数。 */
  done: number;
  /** 拣出来而还没分的件数。 */
  left: number;
  /** 还没拣回来的件数。 */
  open: number;
}

export function sortSummary(state: SortState): SortSummary {
  return {
    orders: state.orders.length,
    done: state.orders.filter(orderSorted).length,
    left: state.orders.reduce((sum, order) => sum + orderLeft(order), 0),
    open: state.orders.reduce((sum, order) => sum + orderOpen(order), 0),
  };
}

/** 顶条上的进度。 */
export function sortProgress(state: SortState): string {
  const summary = sortSummary(state);
  if (summary.orders === 0) return '这个波次没有要分的单';
  return `分完 ${summary.done} / ${summary.orders} 张，还有 ${summary.left} 件没分`;
}

const orderOf = (state: SortState, requestId: string): SortOrder | undefined => state.orders.find((order) => order.requestId === requestId);

const toteHolder = (state: SortState, tote: string): SortOrder | undefined => state.orders.find((order) => order.totes.includes(tote));

/**
 * 一件货归哪张单。`goodsOf` 答这张单的货主名下这个码指的是哪种货（认不出为 null）。
 * 要这种货、还有没分的单里，先给已经有箱的（开着的箱先装满，台面上少摆几个箱），再照受理的先后。
 */
function assign(state: SortState, code: string, goodsOf: (order: SortOrder) => string | null): SortStep {
  let known = false;
  let pick: { order: SortOrder; line: SortLine } | null = null;
  for (const order of state.orders) {
    const goods = goodsOf(order);
    if (goods === null) continue;
    const line = order.lines.find((candidate) => candidate.goodsCode === goods);
    if (line === undefined) continue;
    known = true;
    if (leftOf(line) === 0) continue;
    if (pick === null || (pick.order.totes.length === 0 && order.totes.length > 0)) pick = { order, line };
  }
  const back: SortState = { ...state, phase: 'unit', target: null };
  if (pick === null) {
    return wrong(
      back,
      known
        ? `扫到的是 ${code}。这个波次里要这种货的单都分够了，这一件是多出来的，或者它的单还没拣回来。没有记，放在一边，刷新后再看。`
        : `扫到的是 ${code}。这个波次里没有单要这种货（只认商品编码，和货主登记过的条码、平台商品码）。没有记，放在一边。`,
      code,
    );
  }
  const { order, line } = pick;
  const tote = order.totes[0];
  return step(
    {
      ...back,
      phase: 'tote',
      target: { requestId: order.requestId, goodsCode: line.goodsCode, quantity: 1, adding: false },
      feedback: {
        kind: 'ok',
        text:
          tote === undefined
            ? `这是 ${line.goodsCode}，归出库单 ${orderName(order)}。它还没有箱：拿一个空箱，扫箱号。`
            : `这是 ${line.goodsCode}，归出库单 ${orderName(order)}。放进箱 ${order.totes.join('、')}，扫箱确认。`,
        scanned: code,
      },
    },
    'ok',
  );
}

/** 扫到一个码。等货时认货，等箱时认箱；别的阶段不收。 */
export function scanCode(state: SortState, raw: string): SortStep {
  const code = normalizeScan(raw);
  if (code === '') return { state, effects: [] };
  switch (state.phase) {
    case 'unit': {
      const holder = toteHolder(state, code);
      if (holder !== undefined) {
        return wrong(state, `扫到的是箱 ${code}（出库单 ${orderName(holder)} 的）。现在要扫货，先扫手里这一件。`, code);
      }
      // 等于哪张单的商品编码就当场认下；不等的可能是条码或平台商品码，按还有货没分的各位货主各认一次
      if (state.orders.some((order) => order.lines.some((line) => line.goodsCode === code))) {
        return assign(state, code, (order) => (order.lines.some((line) => line.goodsCode === code) ? code : null));
      }
      const owners = [...new Set(state.orders.filter((order) => order.lines.length > 0).map((order) => order.ownerAccount))];
      if (owners.length === 0) {
        return wrong(state, `扫到的是 ${code}。这个波次现在没有要分的货。没有记。`, code);
      }
      return step(
        { ...state, phase: 'resolving', feedback: { kind: 'waiting', text: `正在认 ${code}…`, scanned: code } },
        undefined,
        [{ kind: 'identify', code, owners }],
      );
    }
    case 'tote': {
      const target = state.target;
      const order = target === null ? undefined : orderOf(state, target.requestId);
      if (target === null || order === undefined) return { state, effects: [] };
      if (state.orders.some((candidate) => candidate.lines.some((line) => line.goodsCode === code))) {
        return wrong(state, `扫到的是商品码 ${code}，现在要扫箱。手里这一件归出库单 ${orderName(order)}，先扫它的箱。`, code);
      }
      const holder = toteHolder(state, code);
      if (holder !== undefined && holder.requestId !== order.requestId) {
        return wrong(
          state,
          `箱 ${code} 是出库单 ${orderName(holder)} 的。手里这一件归出库单 ${orderName(order)}${order.totes.length > 0 ? `，它的箱是 ${order.totes.join('、')}` : ''}。没有记，放对了再扫。`,
          code,
        );
      }
      if (holder === undefined && order.totes.length > 0 && !target.adding) {
        return wrong(
          state,
          `出库单 ${orderName(order)} 的箱是 ${order.totes.join('、')}，扫到的是 ${code}。没有记。箱装满了要再用一个，先点「加一个箱」再扫。`,
          code,
        );
      }
      const submission: SortSubmission = { requestId: order.requestId, goodsCode: target.goodsCode, quantity: target.quantity, tote: code };
      return step(
        {
          ...state,
          phase: 'submitting',
          submission,
          feedback: { kind: 'waiting', text: `正在记：${target.quantity} 件 ${target.goodsCode} 放进箱 ${code}…`, scanned: code },
        },
        undefined,
        [{ kind: 'submit', submission }],
      );
    }
    default:
      return { state, effects: [] };
  }
}

/** 条码认回来了：各位货主名下它指的是哪种货（认不出为 null）。没问成时 `resolved` 为 null，带上原因。 */
export function goodsIdentified(state: SortState, code: string, resolved: Readonly<Record<string, string | null>> | null, reason = ''): SortStep {
  if (state.phase !== 'resolving' || state.feedback.scanned !== code) return { state, effects: [] };
  if (resolved === null) {
    return wrong({ ...state, phase: 'unit' }, `没能认出 ${code}：${reason}。没有记，再扫一次。`, code);
  }
  return assign(state, code, (order) => resolved[order.ownerAccount] ?? null);
}

/** 「这一件先放一边」：不分这一件了，回到等货。 */
export function setAside(state: SortState): SortStep {
  if (state.phase !== 'tote') return { state, effects: [] };
  return step({ ...state, phase: 'unit', target: null, feedback: { kind: 'waiting', text: '这一件没有记。扫下一件。' } });
}

/** 「加一个箱」：这张单的箱装满了，下一个扫到的空箱加给它。只在这张单已经有箱时有用。 */
export function addTote(state: SortState): SortStep {
  const order = state.target === null ? undefined : orderOf(state, state.target.requestId);
  if (state.phase !== 'tote' || state.target === null || order === undefined || order.totes.length === 0) return { state, effects: [] };
  return step({
    ...state,
    target: { ...state.target, adding: true },
    feedback: { kind: 'waiting', text: `拿一个空箱扫它，加给出库单 ${orderName(order)}。` },
  });
}

/** 这张单的这种货还有几件拣出来了没分；等箱那一步之外为 0。 */
export function targetLeft(state: SortState): number {
  const order = state.target === null ? undefined : orderOf(state, state.target.requestId);
  const line = order?.lines.find((candidate) => candidate.goodsCode === state.target?.goodsCode);
  return line === undefined ? 0 : leftOf(line);
}

/** 「剩下的都放进去」：这张单这种货还没分的几件一次记下，不用一件一件扫。再点一次改回一件。 */
export function putAll(state: SortState): SortStep {
  const left = targetLeft(state);
  if (state.phase !== 'tote' || state.target === null || left < 2) return { state, effects: [] };
  const quantity = state.target.quantity === left ? 1 : left;
  return step({
    ...state,
    target: { ...state.target, quantity },
    feedback: {
      kind: 'waiting',
      text: quantity === 1 ? '改回一次放一件。扫箱确认。' : `这一次把 ${quantity} 件 ${state.target.goodsCode} 都放进去。数清楚了再扫箱确认。`,
    },
  });
}

/** 这一次投放要填的格；事实号与仓由 work-fact-submit 按定义补。 */
export function sortFactValues(submission: SortSubmission, waveId: string): GuidedFormValues {
  return {
    waveId,
    requestId: submission.requestId,
    goodsCode: submission.goodsCode,
    quantity: String(submission.quantity),
    toteCode: submission.tote,
  };
}

/** 工位对个别答复的说法。哪些答复算成、算没成不在这里，在 judgeWorkFactAnswer。 */
export const sortOutcomeLabels: Readonly<Record<string, string>> = {
  SORTED: '记下了。',
  EXISTING_RESULT: '这一次投放之前已经记过了（同一个作业事实号），这次没有重复记。',
  CONTENT_CONFLICT: '这个作业事实号已经记过一笔内容不同的投放，这一笔没有记。',
  CONTENT_INVALID:
    '后端不认这一笔，没有记：这张单的这种货已经分够了，或者还没拣回来，也可能这个波次不用分拣。这一件先放一边，屏上的分拣情况已经重读。',
  STATUS_MISMATCH: '这张单现在不能再分货：它已经复核过，或已经取消。这一笔没有记，这一件先放一边。',
  WAVE_NOT_FOUND: '找不到这个波次或这张出库单。这一笔没有记。',
  NOT_CONFIGURED: '未配置：本租户还没有登记仓，这一笔没有记。',
  TOTE_IN_USE: '这个箱上绑着别的出库单，这一笔没有记。货先别放进去，换一个空箱再扫。',
  TOTE_NOT_FOUND: '这个仓没有登记过这个箱号，这一笔没有记。换一个登记过的箱再扫。',
  TOTE_NOT_ACCEPTING: '这个箱已经停用，这一笔没有记。换一个箱再扫。',
};

const toteRefused = (outcome: string | undefined): boolean =>
  outcome === 'TOTE_IN_USE' || outcome === 'TOTE_NOT_FOUND' || outcome === 'TOTE_NOT_ACCEPTING';

// 这一次投放记成之后的分拣情况：这张单这种货的已分数加上去，箱添到这张单名下。
function withPut(orders: readonly SortOrder[], put: SortSubmission): SortOrder[] {
  return orders.map((order) =>
    order.requestId !== put.requestId
      ? order
      : {
          ...order,
          totes: order.totes.includes(put.tote) ? order.totes : [...order.totes, put.tote],
          lines: order.lines.map((line) =>
            line.goodsCode === put.goodsCode ? { ...line, sorted: Math.min(line.sorted + put.quantity, line.picked) } : line,
          ),
        },
  );
}

/** 后端答了：记成了回到等货；箱不认回到等箱、换一个箱；别的没成回到等货并重读；没送到等「再送一次」。 */
export function applyVerdict(state: SortState, verdict: WorkFactVerdict): SortStep {
  if (state.phase !== 'submitting' || state.submission === null) return { state, effects: [] };
  const put = state.submission;
  switch (verdict.verdict) {
    case 'done': {
      const orders = withPut(state.orders, put);
      const next: SortState = { ...state, phase: 'unit', orders, target: null, submission: null, put: state.put + put.quantity };
      const order = orderOf(next, put.requestId);
      const name = order === undefined ? put.requestId : orderName(order);
      const summary = sortSummary(next);
      const rest =
        order !== undefined && orderSorted(order)
          ? `出库单 ${name} 分完了，可以去复核打包。`
          : order === undefined
            ? ''
            : `出库单 ${name} 还有 ${orderLeft(order)} 件没分${orderOpen(order) > 0 ? `、${orderOpen(order)} 件没拣回来` : ''}。`;
      const wave = summary.left === 0 ? (summary.open === 0 ? '这个波次分完了。' : `拿回来的都分完了，还有 ${summary.open} 件没拣回来。`) : '';
      return step({ ...next, feedback: { kind: 'ok', text: `${put.quantity} 件 ${put.goodsCode} 放进了箱 ${put.tote}。${rest}${wave}` } }, 'ok');
    }
    case 'retry':
      return step({ ...state, phase: 'unsent', feedback: { kind: 'unsent', text: verdict.headline } }, 'wrong');
    case 'refused':
      if (toteRefused(verdict.outcome) && state.target !== null) {
        // 箱不认：手里这一件还归这张单，换一个箱再扫。这张单已有箱时，换的那一个算加给它的
        return step(
          { ...state, phase: 'tote', submission: null, target: { ...state.target, adding: true }, feedback: { kind: 'wrong', text: verdict.headline, scanned: put.tote } },
          'wrong',
        );
      }
      return step(
        { ...state, phase: 'unit', target: null, submission: null, feedback: { kind: 'wrong', text: verdict.headline } },
        'wrong',
        verdict.outcome === undefined ? [] : [{ kind: 'reload' }],
      );
  }
}

/** 「再送一次」：用同一个事实号。 */
export function resend(state: SortState): SortStep {
  if (state.phase !== 'unsent') return { state, effects: [] };
  return step({ ...state, phase: 'submitting', feedback: { kind: 'waiting', text: '正在再送一次…' } }, undefined, [{ kind: 'resend' }]);
}

/** Esc：清掉扫错，回到等扫。只是屏上的一句话，什么都没记。 */
export function dismissWrong(state: SortState): SortStep {
  if ((state.phase !== 'unit' && state.phase !== 'tote') || state.feedback.kind !== 'wrong') return { state, effects: [] };
  return step({ ...state, feedback: state.phase === 'unit' ? waiting : { kind: 'waiting', text: '等你扫箱。' } });
}

export interface SortScreen {
  label: string;
  headline: string;
  counter: string;
}

export function sortScreen(state: SortState): SortScreen {
  const summary = sortSummary(state);
  const label = state.feedback.kind === 'wrong' ? '扫错了' : '下一步';
  switch (state.phase) {
    case 'unit':
      if (summary.orders === 0) {
        return { label: '没有要分的', headline: '这个波次没有要分的单', counter: '波次里的单都已经打包、取消或撤出了。' };
      }
      if (summary.left === 0) {
        return summary.open === 0
          ? { label: '做完了', headline: '这个波次分完了', counter: `${summary.done} 张单的货都进了箱，可以去复核打包。` }
          : { label: '等拣货', headline: '拿回来的都分完了', counter: `还有 ${summary.open} 件没拣回来。拣货的人拿回来之后点「刷新」。` };
      }
      return {
        label,
        headline: state.feedback.kind === 'wrong' ? '这一件没有记' : '扫一件货',
        counter: `还有 ${summary.left} 件没分${summary.open > 0 ? `，另有 ${summary.open} 件没拣回来` : ''}。扫到的货归哪张单、放哪个箱，屏上会说。`,
      };
    case 'resolving':
      return { label: '请稍等', headline: '正在认这一件…', counter: '' };
    case 'tote': {
      const order = state.target === null ? undefined : orderOf(state, state.target.requestId);
      const target = state.target;
      if (order === undefined || target === null) return { label, headline: '扫箱', counter: '' };
      const totes = order.totes.join('、');
      return {
        label,
        headline:
          state.feedback.kind === 'wrong'
            ? '这不是这张单的箱'
            : order.totes.length === 0 || target.adding
              ? `拿一个空箱，扫箱号`
              : `放进箱 ${totes}，扫箱确认`,
        counter: `${target.quantity} 件 ${target.goodsCode} 归出库单 ${orderName(order)}；这张单这种货还有 ${targetLeft(state)} 件没分。`,
      };
    }
    case 'submitting':
      return { label: '请稍等', headline: '正在记这一次投放…', counter: '' };
    case 'unsent':
      return { label: '没送到', headline: '这一次投放没有送到', counter: '用同一个作业事实号再送，不会记两笔。货先别动。' };
  }
}

export interface SortActions {
  setAside: boolean;
  addTote: boolean;
  putAll: boolean;
  resend: boolean;
  refresh: boolean;
}

export function sortActions(state: SortState): SortActions {
  const order = state.target === null ? undefined : orderOf(state, state.target.requestId);
  return {
    setAside: state.phase === 'tote',
    addTote: state.phase === 'tote' && order !== undefined && order.totes.length > 0 && state.target?.adding !== true,
    putAll: state.phase === 'tote' && targetLeft(state) >= 2,
    resend: state.phase === 'unsent',
    refresh: state.phase === 'unit',
  };
}

export function acceptingScans(state: SortState): boolean {
  return state.phase === 'unit' || state.phase === 'tote';
}

export function scanPlaceholder(state: SortState): string {
  switch (state.phase) {
    case 'unit':
      return '扫货，或敲进去按回车';
    case 'tote':
      return '扫箱号，或敲进去按回车';
    case 'unsent':
      return '先点「再送一次」';
    default:
      return '正在处理，先不收扫码';
  }
}
