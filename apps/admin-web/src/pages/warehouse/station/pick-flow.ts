// 拣货工位的会话状态机（纯 TypeScript，node:test 直接测）：现在等扫什么、扫到一个码之后变成什么、该提交什么。
// 判断一律在这里；组件只管摆与接线，不写「这个码对不对」。
//
// 到 Go 端核过的规则（拣货用例是 application.WaveHandler.Pick；载荷由 registrationjson.PickCommand 译；口在
// warehousehttp.NewRecordPickEndpoint）。屏上的话照这些说，不照猜的说：
//   - 实拣数量是不小于 1 的整数。0 与负数在读载荷时就是 400，连用例都到不了：一件都没拣到的格不能提交，只能跳过。
//   - 实拣数量可以小于应拣。大于这条任务还没拣的数量，答内容不合（CONTENT_INVALID）。
//   - 少拣照常成立：这条任务记成短拣（SHORT），差的件数记一笔待处理的拣货差异，已拣的那部分预占放开、没拣的预占留下。
//     差的那几件后端当场在同一个波次里换别的库位再分配，避开少拣的这一格；分得出就添成新的待拣格，要重读队列才看得到。
//     出库单不退出波次：它还有格等着拣时仍是已放波次；格都有了结果之后，拣齐的是已拣货，没拣齐的才回到已预占，
//     等有货了再放一次波次。短拣之后这条任务不再是待拣，不能补拣：换一个事实号再提交答内容冲突（CONTENT_CONFLICT）。
//   - 目标库位须是这张出库单所在仓的库位、没停用，所在库区没停用且用途是待出库（OUTBOUND_STAGING），否则内容不合。
//     放货位在扫到的时候就照这一条到登记册核（核法是 pick-staging-check.ts 的 createStagingVerifier），提交时后端还会再核一遍。
//   - 任务已经拣过（已完成、短拣或迟到）：同一个事实号同样数量重放答已有结果（EXISTING_RESULT），别的答内容冲突；
//     任务找不到答 WAVE_NOT_FOUND；这个租户一个仓都没登记答未配置。
//   - 出库单截单会把还没拣的任务取消（CANCELLED）。对被取消的任务提交，这一笔照记（RECORDED）：任务转成迟到（LATE），
//     拣出来的货生成回库任务。只有被取消的任务照记，迟到的任务不再照记。
//
//   - 拣货可以带周转箱号：箱空着就绑到这张出库单上，一张单可以用几个箱，一个箱同一时刻只装一张单。箱没登记
//     （TOTE_NOT_FOUND）、已停用（TOTE_NOT_ACCEPTING）、绑着别的出库单（TOTE_IN_USE）时这一笔整份不成立，任务仍待拣。
//     这个仓登记了箱，工位才提示扫箱：一张单的第一格扫一次，同一张单后面的格沿用；不用箱可以点「不用箱」。
//
// 屏上文字与扫码顺序照屏幕示意里拣货那一项：开工扫一次待出库库位 → 扫库位码 → 逐件扫商品加一（也可直接改数量）→
// 扫满自动提交这一格、跳到下一格。

import type { GuidedFormValues } from '../guided-forms';
import type { GoodsIdentification } from './pick-goods';
import { allotMembers, pickedTogether, type PickCell } from './pick-queue';
import type { StagingVerdict } from './pick-staging-check';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import type { WorkFactVerdict } from './work-fact-submit';

export type PickPhase =
  /** 等放货位：这一趟拣的货放到哪个待出库库位。 */
  | 'staging'
  /** 正在核一个放货位码（读口在途）。 */
  | 'verifying'
  /** 等这一格的库位码。 */
  | 'location'
  /** 等这张出库单的周转箱：只在这个仓登记了箱、这张单这一趟还没定过箱时出现。 */
  | 'tote'
  /** 等商品码，逐件加一。 */
  | 'goods'
  /** 正在认一个商品码（读口在途）。 */
  | 'resolving'
  /** 这一格正在提交，不收扫码。 */
  | 'submitting'
  /** 提交没送到：等「再送一次」。 */
  | 'unsent'
  /** 提交业务上没成：说明原因，等「下一格」。 */
  | 'refused'
  /** 没有余下的格。 */
  | 'finished';

export interface PickFeedback {
  /** 等扫 / 扫对 / 扫错三种样子；unsent 是结果还不确定（提交没送到、放货位没核成），用另一种颜色。 */
  kind: 'waiting' | 'ok' | 'wrong' | 'unsent';
  text: string;
  /** 刚扫到的码。 */
  scanned?: string;
}

/** 合成一格拿的货分给其中一条任务的那一份：`cell` 是这条任务（应拣数量是它自己的），`quantity` 是记给它几件。 */
export interface PickAllotment {
  cell: PickCell;
  quantity: number;
}

export interface PickSubmission {
  cell: PickCell;
  quantity: number;
  /** 这一格的货放进的周转箱；没用箱就没有这一格。 */
  tote?: string;
}

export interface PickState {
  phase: PickPhase;
  /** 还没拣完的格，次序就是作业的次序；第一格是当前格。 */
  queue: readonly PickCell[];
  /** 这一趟已经拣完（提交成功）的格数。 */
  done: number;
  /** 这一趟一共多少格：开工时队列的长度，格没了（别处拣掉了）就减一。 */
  total: number;
  /** 当前格已扫的件数。 */
  counted: number;
  /** 放货位；还没有为 null。 */
  staging: string | null;
  /** 换放货位之前停在哪一步，扫到合格的放货位后回到那里；不是在换放货位为 null。 */
  resume: PickPhase | null;
  /** 正在核的放货位码，以及它是本机记着的上次那个还是刚扫到的；不在核为 null。 */
  verifying: { code: string; remembered: boolean } | null;
  feedback: PickFeedback;
  /** 在途或没成的那一笔提交。 */
  submission: PickSubmission | null;
  /**
   * 合成一格拿的几条任务正在一条一条交：`members` 是这一次要交的那几条和各自记几件，`sent` 是已经记成的条数。
   * 一条任务一格时为 null。
   */
  batch: { members: readonly PickAllotment[]; sent: number } | null;
  /** 没成的那一笔里，这条任务在后端是不是已经不能拣了：是，「下一格」把它从队列里去掉；不是，挪到队尾。 */
  gone: boolean;
  /** 这个仓登记了周转箱：是，一张出库单的第一格要扫箱（或点「不用箱」）。 */
  toteOn: boolean;
  /** 租户给这个仓登记了「拣货必须用周转箱」：是，不能点「不用箱」，不带箱的拣货后端不收。 */
  toteRequired: boolean;
  /** 这一趟各张出库单定下的箱：箱号是用这个箱，null 是这张单不用箱，没有这一项是还没问过。 */
  totes: Readonly<Record<string, string | null>>;
}

export type PickEffect =
  | { kind: 'identify'; cell: PickCell; code: string }
  | { kind: 'verify-staging'; code: string }
  | { kind: 'submit'; cell: PickCell; quantity: number; tote?: string }
  | { kind: 'resend' }
  | { kind: 'remember-staging'; code: string };

export interface PickStep {
  state: PickState;
  effects: readonly PickEffect[];
  /** 这一步该响哪种声音；没有就不响。 */
  cue?: ScanCue;
}

// —— 开工 ——

export interface PickStart {
  cells: readonly PickCell[];
  /** 已经核过、此刻能用的放货位；没有为 null。 */
  staging: string | null;
  /** 这个仓登记了周转箱没有；不给就是没有。 */
  totes?: boolean;
}

const waitingFeedback: PickFeedback = { kind: 'waiting', text: '等你扫码。' };

export function startPick(input: PickStart): PickState {
  const phase: PickPhase = input.staging === null ? 'staging' : input.cells.length === 0 ? 'finished' : 'location';
  return {
    phase,
    queue: input.cells,
    done: 0,
    total: input.cells.length,
    counted: 0,
    staging: input.staging,
    resume: null,
    verifying: null,
    feedback: waitingFeedback,
    submission: null,
    batch: null,
    gone: false,
    toteOn: input.totes === true,
    toteRequired: false,
    totes: {},
  };
}

/**
 * 这个仓有没有登记周转箱、租户要不要求用箱，是开工之后才读回来的：读回来了改这两格。要求用箱时一定提示扫箱。
 * 已经定过箱的出库单不受影响；这一趟点过「不用箱」的出库单，要求用箱时那个决定作废、回头重新问。
 * 正停在等箱那一步而读回来是没有箱、也不要求时，直接去扫商品。
 */
export function setToteMode(state: PickState, on: boolean, required = false): PickStep {
  const toteOn = on || required;
  if (state.toteOn === toteOn && state.toteRequired === required) return { state, effects: [] };
  if (!toteOn && state.phase === 'tote') {
    return { state: { ...state, toteOn: false, toteRequired: false, phase: 'goods', feedback: waitingFeedback }, effects: [] };
  }
  const totes = required ? withoutDeclined(state.totes) : state.totes;
  return { state: { ...state, toteOn, toteRequired: required, totes }, effects: [] };
}

// 去掉「这张单不用箱」的那些决定：要求用箱之后它们不作数了。
function withoutDeclined(totes: Readonly<Record<string, string | null>>): Readonly<Record<string, string | null>> {
  return Object.fromEntries(Object.entries(totes).filter(([, tote]) => tote !== null));
}

/** 这一格的货放进哪个箱：这张出库单这一趟定下的那个；没定或不用箱为 undefined。 */
export function toteOfCell(state: PickState, cell: PickCell | undefined): string | undefined {
  if (cell === undefined) return undefined;
  return state.totes[cell.requestId] ?? undefined;
}

/**
 * 开一趟：本机记着上次的放货位就先核它，用的是扫到放货位时同一套核（效果 verify-staging、答复交给 stagingVerified），
 * 核完才收扫码；没有记着就等人扫一个。记着的那个不直接信：库位会被停用、库区会改用途，这台设备也可能换了仓。
 */
export function beginPick(cells: readonly PickCell[], remembered: string | null): PickStep {
  const state = startPick({ cells, staging: null });
  if (remembered === null) return { state, effects: [] };
  return {
    state: {
      ...state,
      phase: 'verifying',
      verifying: { code: remembered, remembered: true },
      feedback: { kind: 'waiting', text: `正在核对上次用的放货位 ${remembered}…` },
    },
    effects: [{ kind: 'verify-staging', code: remembered }],
  };
}

/** 刷新队列：用重新读出来的格换掉队列，从头数进度；当前格若在途或扫了一半，刷新不让点，所以这里不用保它。 */
export function reloadQueue(state: PickState, cells: readonly PickCell[]): PickStep {
  const stuck =
    state.phase === 'submitting' || state.phase === 'resolving' || state.phase === 'verifying' || state.phase === 'unsent' || state.counted > 0;
  if (stuck) return { state, effects: [] };
  const phase: PickPhase = state.staging === null ? 'staging' : cells.length === 0 ? 'finished' : 'location';
  return {
    state: {
      ...state,
      phase,
      queue: cells,
      done: 0,
      total: cells.length,
      counted: 0,
      resume: null,
      submission: null,
      batch: null,
      gone: false,
      feedback: { kind: 'waiting', text: cells.length === 0 ? '刷新了，没有还没拣的格。' : '队列刷新了。' },
    },
    effects: [],
  };
}

// —— 屏上的字 ——

export interface PickScreen {
  /** 标题上方的小字：「下一步」，扫错时是「扫错了」。 */
  label: string;
  /** 最大的字：下一步该扫什么。 */
  headline: string;
  /** 一行小字：这一格要拣什么、扫到几件。 */
  counter: string;
}

const wrongHeadlines: Partial<Record<PickPhase, string>> = {
  staging: '这个库位不能当放货位',
  location: '这不是这一格的库位',
  tote: '这不是周转箱的码',
  goods: '这不是这一格要的货',
};

const remaining = (state: PickState): number => (state.queue[0]?.quantity ?? 0) - state.counted;

export function pickScreen(state: PickState): PickScreen {
  const cell = state.queue[0];
  const wrong = state.feedback.kind === 'wrong' && wrongHeadlines[state.phase] !== undefined;
  const label = wrong ? '扫错了' : '下一步';
  switch (state.phase) {
    case 'staging':
      return {
        label,
        headline: wrong
          ? (wrongHeadlines.staging as string)
          : state.resume === null
            ? '开工先扫待出库库位'
            : '扫新的待出库库位',
        counter: '这一趟拣的货都放到那里。扫一次，这台设备记住它。',
      };
    case 'verifying':
      return {
        label: '请稍等',
        headline: '正在核对放货位…',
        counter: `核对库位 ${state.verifying?.code ?? ''}：在不在这个仓、有没有停用、所在库区是不是待出库用途。`,
      };
    case 'location':
      return {
        label,
        headline: wrong ? (wrongHeadlines.location as string) : `去库位 ${cell.locationCode}，扫库位码`,
        counter:
          cell.members === undefined
            ? `这一格要拣 ${cell.goodsCode}，共 ${cell.quantity} 件`
            : cell.batch === true
              ? `这一格要拣 ${cell.goodsCode}，共 ${cell.quantity} 件：${ordersOf(cell)} 张出库单的货合在一起拿，拿回来再分拣`
              : `这一格要拣 ${cell.goodsCode}，共 ${cell.quantity} 件：${cell.quantity} 张单品单件的出库单，一张一件，合在一起拿`,
      };
    case 'tote':
      return {
        label,
        headline: wrong ? (wrongHeadlines.tote as string) : '扫这张单的周转箱',
        counter: state.toteRequired
          ? `这一格的货属于出库单 ${cell.requestId}，它的货都放进同一个箱。这个仓拣货必须用周转箱。`
          : `这一格的货属于出库单 ${cell.requestId}，它的货都放进同一个箱。不用箱就点「不用箱」。`,
      };
    case 'goods':
    case 'resolving': {
      const tote = toteOfCell(state, cell);
      return {
        label,
        headline: wrong ? (wrongHeadlines.goods as string) : `扫商品 ${cell.goodsCode}，还差 ${remaining(state)} 件`,
        counter: tote === undefined ? `已扫 ${state.counted} / ${cell.quantity}` : `已扫 ${state.counted} / ${cell.quantity}，放进箱 ${tote}`,
      };
    }
    case 'submitting':
      return { label: '请稍等', headline: '正在提交这一格…', counter: submissionCounter(state) };
    case 'unsent':
      return { label: '没送到', headline: '这一笔没有送到', counter: submissionCounter(state) };
    case 'refused':
      return { label: '没记成', headline: '这一格没有记成', counter: submissionCounter(state) };
    case 'finished':
      return {
        label: '做完了',
        headline: state.done > 0 ? '拣完了' : '没有还没拣的格',
        counter: state.done > 0 ? `这一趟拣了 ${state.done} 格` : '本仓现在没有还没拣的拣货任务。',
      };
  }
}

function submissionCounter(state: PickState): string {
  const sub = state.submission;
  if (sub === null) return '';
  if (state.batch !== null) {
    return `${sub.cell.goodsCode} ${sub.quantity} 件，分开记给 ${state.batch.members.length} 张出库单，已记 ${state.batch.sent} / ${state.batch.members.length}，放到 ${state.staging ?? '待出库库位'}`;
  }
  const short = sub.quantity < sub.cell.quantity ? `（应拣 ${sub.cell.quantity} 件，按短拣记）` : '';
  const tote = sub.tote === undefined ? '' : `，装在箱 ${sub.tote}`;
  return `${sub.cell.goodsCode} ${sub.quantity} 件${short}，放到 ${state.staging ?? '待出库库位'}${tote}`;
}

/** 屏上「进度」那一格：「第 3 格，共 12 格」。 */
export function pickProgress(state: PickState): string {
  if (state.total === 0) return '没有待拣的格';
  if (state.queue.length === 0) return `已拣完，共 ${state.total} 格`;
  return `第 ${Math.min(state.done + 1, state.total)} 格，共 ${state.total} 格`;
}

// —— 扫码 ——

function step(state: PickState, cue?: ScanCue, effects: readonly PickEffect[] = []): PickStep {
  return cue === undefined ? { state, effects } : { state, effects, cue };
}

function wrong(state: PickState, text: string, scanned: string): PickStep {
  return step({ ...state, feedback: { kind: 'wrong', text, scanned } }, 'wrong');
}

/** 当前格又扫对了一件（或数量改成了 n）：够数就自动提交这一格。 */
function counted(state: PickState, count: number, text: string, scanned?: string): PickStep {
  const cell = state.queue[0];
  const feedback: PickFeedback = { kind: 'ok', text, ...(scanned === undefined ? {} : { scanned }) };
  if (count < cell.quantity) return step({ ...state, phase: 'goods', counted: count, feedback }, 'ok');
  if (cell.members !== undefined) {
    return submitMembers({ ...state, counted: count }, cell, cell.quantity, { ...feedback, text: `${text}扫满了，正在把这 ${cell.quantity} 件记给各张出库单。` }, 'ok');
  }
  const tote = toteOfCell(state, cell);
  return step(
    {
      ...state,
      phase: 'submitting',
      counted: count,
      submission: withTote({ cell, quantity: cell.quantity }, tote),
      feedback: { ...feedback, text: `${text}扫满了，正在提交这一格。` },
    },
    'ok',
    [withTote({ kind: 'submit' as const, cell, quantity: cell.quantity }, tote)],
  );
}

/**
 * 合成一格拿的几条任务开始提交：拿到的件数按 allotMembers 分给各条任务，一条一条交（账上一条任务一笔）。
 * 先交第一条，后端答成了由 applyVerdict 接着交下一条。合着拿的货不按出库单分周转箱，报文不带箱号。
 */
function submitMembers(state: PickState, cell: PickCell, quantity: number, feedback: PickFeedback, cue?: ScanCue): PickStep {
  const members = allotMembers(cell, quantity);
  return step(
    { ...state, phase: 'submitting', submission: { cell, quantity }, batch: { members, sent: 0 }, feedback },
    cue,
    [{ kind: 'submit', cell: members[0].cell, quantity: members[0].quantity }],
  );
}

/** 合成的一格里有几张出库单：一张单在这一格可以有不止一条任务，按单号数。 */
function ordersOf(cell: PickCell): number {
  return new Set((cell.members ?? []).map((member) => member.requestId)).size;
}

/** 分下去的几份里分到了一些、但没记满的那一条；照 allotMembers 的分法至多一条，没有为 undefined。 */
const partialAllotment = (members: readonly PickAllotment[]): PickAllotment | undefined =>
  members.find((member) => member.quantity > 0 && member.quantity < member.cell.quantity);

/** 一件都没分到的那几条：它们按 0 件交，也是短拣。 */
const emptyAllotments = (members: readonly PickAllotment[]): PickAllotment[] => members.filter((member) => member.quantity === 0);

/** 有没有哪一条没记满（含一件都没分到的）：有就是有短拣，后端会换别的库位补。 */
const anyShortAllotment = (members: readonly PickAllotment[]): boolean => members.some((member) => member.quantity < member.cell.quantity);

// 没用箱时不带 tote 这一格：报文里不带箱号才是「没用箱」，空串后端不收。
function withTote<T extends object>(value: T, tote: string | undefined): T & { tote?: string } {
  return tote === undefined ? value : { ...value, tote };
}

/**
 * 扫到一个码。码是扫描框规范化过的；这里再规范一次，空的不算。各阶段的判定：
 * 放货位 → 要到登记册核（effect: verify-staging），核完由 stagingVerified 接着判；库位 → 要等于这一格的库位码；
 * 商品 → 等于商品编码当场算对，是库位码算错，别的要去认（effect: identify），认出来之后由 goodsIdentified 接着判。
 * 核放货位、提交、认码、没送到、没记成与做完的阶段不收扫码。
 */
export function scanCode(state: PickState, raw: string): PickStep {
  const code = normalizeScan(raw);
  if (code === '') return { state, effects: [] };
  const cell = state.queue[0];
  switch (state.phase) {
    case 'staging':
      return step(
        {
          ...state,
          phase: 'verifying',
          verifying: { code, remembered: false },
          feedback: { kind: 'waiting', text: `正在核对放货位 ${code}…`, scanned: code },
        },
        undefined,
        [{ kind: 'verify-staging', code }],
      );
    case 'location': {
      if (code !== cell.locationCode) {
        return wrong(
          state,
          `扫到的是 ${code}，这一格在库位 ${cell.locationCode}。没有记任何东西，到 ${cell.locationCode} 再扫它的库位码。`,
          code,
        );
      }
      // 这个仓登记了箱、这张单这一趟还没定过箱：先定箱。定过的（含定了不用箱的）不再问。
      // 货合在一起拿的格不问箱：单品单件波次到复核打包扫货配单，合并拣货的波次到分拣才按出库单分箱。
      if (state.toteOn && !pickedTogether(cell) && state.totes[cell.requestId] === undefined) {
        return step(
          {
            ...state,
            phase: 'tote',
            counted: 0,
            feedback: { kind: 'ok', text: `对。到了 ${cell.locationCode}。这张单还没有箱，扫一个周转箱。`, scanned: code },
          },
          'ok',
        );
      }
      const tote = toteOfCell(state, cell);
      const text =
        tote === undefined ? `对。到了 ${cell.locationCode}，接着扫商品。` : `对。到了 ${cell.locationCode}，货放进箱 ${tote}，接着扫商品。`;
      return step({ ...state, phase: 'goods', counted: 0, feedback: { kind: 'ok', text, scanned: code } }, 'ok');
    }
    case 'tote': {
      if (code === cell.locationCode || code === state.staging) {
        return wrong(state, `扫到的是库位码 ${code}，现在要扫周转箱。没有记下。`, code);
      }
      if (code === cell.goodsCode) {
        return wrong(state, `扫到的是商品码 ${code}，现在要扫周转箱。没有记数，先扫箱。`, code);
      }
      // 这一趟里已经给了别的出库单的箱不收：后端也会拒，这里先拦住，货不至于放错箱才发现
      const taken = Object.entries(state.totes).find(([request, tote]) => tote === code && request !== cell.requestId);
      if (taken !== undefined) {
        return wrong(state, `箱 ${code} 这一趟已经给了出库单 ${taken[0]}，一个箱只装一张单。换一个箱再扫。`, code);
      }
      return step(
        {
          ...state,
          phase: 'goods',
          counted: 0,
          totes: { ...state.totes, [cell.requestId]: code },
          feedback: { kind: 'ok', text: `箱 ${code} 定给这张单了，接着扫商品。这个箱能不能用，提交这一格时后端会核。`, scanned: code },
        },
        'ok',
      );
    }
    case 'goods': {
      if (code === cell.goodsCode) return counted(state, state.counted + 1, `对。这是 ${cell.goodsCode}。`, code);
      if (code === cell.locationCode || code === state.staging) {
        return wrong(
          state,
          `扫到的是库位码 ${code}，现在要扫商品。这一格要 ${cell.goodsCode}，还差 ${remaining(state)} 件。没有记数。`,
          code,
        );
      }
      return step(
        { ...state, phase: 'resolving', feedback: { kind: 'waiting', text: `正在认 ${code}…`, scanned: code } },
        undefined,
        [{ kind: 'identify', cell, code }],
      );
    }
    default:
      return { state, effects: [] };
  }
}

/** 商品码认出来了：对就加一，错就红、不记数。认的是哪个码要对得上，对不上说明这一步已经过期，不动。 */
export function goodsIdentified(state: PickState, code: string, identification: GoodsIdentification): PickStep {
  if (state.phase !== 'resolving' || state.feedback.scanned !== code) return { state, effects: [] };
  const cell = state.queue[0];
  const back: PickState = { ...state, phase: 'goods' };
  const left = remaining(state);
  switch (identification.kind) {
    case 'same':
      return counted(back, state.counted + 1, identification.via === 'alias' ? `对。这个码是 ${cell.goodsCode}。` : `对。这是 ${cell.goodsCode}。`, code);
    case 'other':
      return wrong(
        back,
        `扫到的是 ${code}，认成商品 ${identification.goodsCode}。这一格要 ${cell.goodsCode}，还差 ${left} 件。没有记数，放回去再扫。`,
        code,
      );
    case 'unknown':
      return wrong(
        back,
        `扫到的是 ${code}，认不出这是什么商品（只认商品编码，和这位货主登记过的条码、平台商品码）。这一格要 ${cell.goodsCode}，还差 ${left} 件。没有记数，放回去再扫。`,
        code,
      );
    case 'failed':
      return wrong(back, `没能认出 ${code}：${identification.reason}。这一格要 ${cell.goodsCode}，还差 ${left} 件。没有记数，再扫一次。`, code);
  }
}

// 读口没问成的原因有的是整句（接入、问题码的现成说法带句号），拼进句子里前去掉句号，免得出现「。。」
const withoutStop = (reason: string): string => reason.replace(/[。.]+$/, '');

/**
 * 放货位核出结果了。核的是哪个码要对得上，对不上说明这一步已经过期，不动。
 * 核得过才定下（刚扫的记在本机，记着的上次那个本来就在本机，不再写）；核不过说清是哪一条，没核成说读口没问成——
 * 两种都什么也不记，放货位仍是原来的（没有就仍是没有）。本机记着的那个没核成时也不删：它多半还能用，只是这一次没问成。
 */
export function stagingVerified(state: PickState, code: string, verdict: StagingVerdict): PickStep {
  const checking = state.verifying;
  if (state.phase !== 'verifying' || checking === null || checking.code !== code) return { state, effects: [] };
  const back: PickState = { ...state, phase: 'staging', verifying: null };
  // 刚扫的码屏上有「刚才扫到」可写；本机记着的那个不是刚扫的，不带
  switch (verdict.kind) {
    case 'ok': {
      const phase: PickPhase = state.resume ?? (state.queue.length === 0 ? 'finished' : 'location');
      const placed: PickState = { ...back, phase, staging: code, resume: null };
      if (checking.remembered) return step({ ...placed, feedback: waitingFeedback });
      return step(
        { ...placed, feedback: { kind: 'ok', text: `放货位定为 ${code}。这一趟拣的货都放那里。`, scanned: code } },
        'ok',
        [{ kind: 'remember-staging', code }],
      );
    }
    case 'rejected':
      return checking.remembered
        ? step({ ...back, feedback: { kind: 'wrong', text: `上次用的放货位 ${code} 现在不能用：${verdict.reason}请重新扫一个待出库库位。` } })
        : wrong(back, `${verdict.reason}没有记下，换一个再扫。`, code);
    case 'unverified':
      return checking.remembered
        ? step({
            ...back,
            feedback: {
              kind: 'unsent',
              text: `上次用的放货位 ${code} 这一次没核成：${withoutStop(verdict.reason)}。先不当它能用，也没有忘掉它。请再扫一次它（扫同一个就行），或另扫一个待出库库位。`,
            },
          })
        : step(
            {
              ...back,
              feedback: { kind: 'unsent', text: `没能核对放货位 ${code}：${withoutStop(verdict.reason)}。没有记下，再扫一次（扫同一个也行）。`, scanned: code },
            },
            'wrong',
          );
  }
}

/** Esc：清掉扫错与没核成，回到等扫。这两样只是屏上的一句话，什么都没记，所以清它们不改别的。 */
export function dismissWrong(state: PickState): PickStep {
  const scanning = state.phase === 'staging' || state.phase === 'location' || state.phase === 'tote' || state.phase === 'goods';
  if (!scanning || (state.feedback.kind !== 'wrong' && state.feedback.kind !== 'unsent')) return { state, effects: [] };
  return step({ ...state, feedback: waitingFeedback });
}

// —— 数量 ——

export type QuantityJudgement = { ok: true; value: number } | { ok: false; reason: string };

/** 「改数量」填的数：1 到应拣之间的整数。 */
export function judgeQuantity(text: string, wanted: number): QuantityJudgement {
  const trimmed = text.trim();
  if (trimmed === '') return { ok: false, reason: '请填数量。' };
  if (!/^\d+$/.test(trimmed)) return { ok: false, reason: '数量要填整数。' };
  const value = Number.parseInt(trimmed, 10);
  if (value < 1) return { ok: false, reason: '至少 1 件。这一格一件都没有的话，点「少拣」按 0 件提交。' };
  if (value > wanted) return { ok: false, reason: `最多 ${wanted} 件，这一格要拣的就是这么多。` };
  return { ok: true, value };
}

/** 直接把已扫数改成 n。只在等商品的阶段可以改；改到应拣数就像扫满一样自动提交。 */
export function changeCount(state: PickState, text: string): { ok: true; step: PickStep } | { ok: false; reason: string } {
  if (state.phase !== 'goods') return { ok: false, reason: '先扫这一格的库位码，再改数量。' };
  const cell = state.queue[0];
  const judged = judgeQuantity(text, cell.quantity);
  if (!judged.ok) {
    // 已经扫了数的格不能退回 0：那几件在手里，跳过不让点；要么扫满，要么少拣按已扫的数提交
    const zero = /^\d+$/.test(text.trim()) && Number.parseInt(text.trim(), 10) < 1;
    return zero && state.counted > 0
      ? { ok: false, reason: `已经扫了 ${state.counted} 件，不能改回 0：手里的货要么扫满，要么用「少拣」按已扫的数提交。` }
      : judged;
  }
  return { ok: true, step: counted(state, judged.value, `数量改成 ${judged.value}。`) };
}

// —— 少拣 ——

export interface ShortPickPlan {
  cell: PickCell;
  /** 按这个数提交。 */
  quantity: number;
  /** 差的件数。 */
  short: number;
  staging: string;
}

/**
 * 少拣要满足：已经到了这一格（扫过库位，在等周转箱或等商品），已扫的数比应拣少（够数就已经自动提交了）。不满足答 null。
 * 已扫 0 件也算：到了库位一件都没有，按 0 件交，后端照少拣办。等周转箱那一步也能点——没有货就不用先定箱。
 */
export function shortPickPlan(state: PickState): ShortPickPlan | null {
  if ((state.phase !== 'goods' && state.phase !== 'tote') || state.staging === null) return null;
  const cell = state.queue[0];
  const counted = state.phase === 'goods' ? state.counted : 0;
  if (counted >= cell.quantity) return null;
  return { cell, quantity: counted, short: cell.quantity - counted, staging: state.staging };
}

const reallocationNote = '盘点盘过之前，这个库位的这种货不再派人来拣。';

/**
 * 少拣确认框里的话：这几件移进待出库库位、这一格记成短拣并记一笔差额、差的由系统换别的库位补。
 * 合成一格拿的：拿到的件数按任务的先后分下去，分到了但没分满的那一条、一件都没分到的那几条都记短拣。
 * 单品单件的每条任务只有一件，所以没有「没分满」的那一条。
 */
export function shortPickLines(plan: ShortPickPlan): string[] {
  const moved =
    plan.quantity === 0 ? '到了库位一件都没有，没有货移进待出库库位。' : `这 ${plan.quantity} 件移进待出库库位 ${plan.staging}`;
  if (plan.cell.members !== undefined && plan.cell.batch === true) {
    const allotted = allotMembers(plan.cell, plan.quantity);
    const partial = partialAllotment(allotted);
    const empty = emptyAllotments(allotted).length;
    const full = allotted.length - empty - (partial === undefined ? 0 : 1);
    const lines = [
      `这一格是 ${ordersOf(plan.cell)} 张出库单合着拿的 ${plan.cell.goodsCode}，共要 ${plan.cell.quantity} 件，现在只拿到 ${plan.quantity} 件。`,
      plan.quantity === 0 ? moved : `${moved}，按任务的先后分给各张出库单：${full} 条任务记满。`,
    ];
    if (partial !== undefined) {
      lines.push(
        `出库单 ${partial.cell.requestId} 在这一格要 ${partial.cell.quantity} 件，只分到 ${partial.quantity} 件，记成「短拣」并记一笔短拣差异；差的 ${partial.cell.quantity - partial.quantity} 件系统会在这个波次里换别的库位补，换不出就等这张单别的格拣完后回到「已预占」。`,
      );
    }
    if (empty > 0) {
      lines.push(
        `${plan.quantity === 0 ? '这' : '还有'} ${empty} 条任务一件都没分到，各按 0 件记成「短拣」并记一笔短拣差异；差的由系统在这个波次里换别的库位补，换不出的那几张单回到「已预占」等货。${reallocationNote}`,
      );
    }
    return lines;
  }
  if (plan.cell.members !== undefined) {
    return [
      `这一格是 ${plan.cell.quantity} 张单品单件的出库单合着拿的 ${plan.cell.goodsCode}，现在只拿到 ${plan.quantity} 件。`,
      plan.quantity === 0 ? moved : `${moved}，各记给一张出库单。`,
      `${plan.quantity === 0 ? '这' : '剩下的'} ${plan.short} 张出库单没有拣到，各按 0 件记成「短拣」并记一笔短拣差异；系统会在这个波次里换别的库位补，换不出的回到「已预占」等货。${reallocationNote}`,
    ];
  }
  return [
    `这一格要拣 ${plan.cell.quantity} 件 ${plan.cell.goodsCode}，现在只记 ${plan.quantity} 件。`,
    plan.quantity === 0 ? moved : `${moved}。`,
    `这一格记成「短拣」，提交之后不能再拣，差额记一笔短拣差异；${reallocationNote}差的 ${plan.short} 件系统会在这个波次里换别的库位补：换得出就添进余下的格，换不出就等这张出库单别的格拣完后回到「已预占」。`,
  ];
}

/** 确认之后提交：按已扫的数。一件都没拣到时不带周转箱——没有货放进去，后端也不绑。 */
export function submitShortPick(state: PickState): PickStep {
  const plan = shortPickPlan(state);
  if (plan === null) return { state, effects: [] };
  if (plan.cell.members !== undefined) {
    return submitMembers({ ...state, counted: plan.quantity }, plan.cell, plan.quantity, {
      kind: 'waiting',
      text: `按 ${plan.quantity} 件提交这一格，分开记给各张出库单（还差 ${plan.short} 件没拿到）…`,
    });
  }
  const tote = plan.quantity === 0 ? undefined : toteOfCell(state, plan.cell);
  return step(
    {
      ...state,
      phase: 'submitting',
      counted: plan.quantity,
      submission: withTote({ cell: plan.cell, quantity: plan.quantity }, tote),
      feedback: { kind: 'waiting', text: `按 ${plan.quantity} 件提交这一格（短拣，差 ${plan.short} 件）…` },
    },
    undefined,
    [withTote({ kind: 'submit' as const, cell: plan.cell, quantity: plan.quantity }, tote)],
  );
}

// —— 跳过、换放货位 ——

/** 跳过这一格：挪到队尾，不提交。扫了数的不让跳——那几件已经拿在手里，要么提交要么少拣。队里只有这一格时也没处可挪。 */
export function skipCell(state: PickState): PickStep {
  const onCell = state.phase === 'location' || state.phase === 'tote' || (state.phase === 'goods' && state.counted === 0);
  if (!onCell || state.queue.length < 2) return { state, effects: [] };
  const [head, ...rest] = state.queue;
  return step({
    ...state,
    phase: 'location',
    queue: [...rest, head],
    counted: 0,
    feedback: { kind: 'waiting', text: `跳过了 ${head.locationCode}，它排到了队尾。` },
  });
}

/** 「不用箱」：这张出库单这一趟不用周转箱，后面它的格也不再问。只在等箱那一步能点；这个仓要求用箱时不能点。 */
export function skipTote(state: PickState): PickStep {
  if (state.phase !== 'tote' || state.toteRequired) return { state, effects: [] };
  const cell = state.queue[0];
  return step({
    ...state,
    phase: 'goods',
    counted: 0,
    totes: { ...state.totes, [cell.requestId]: null },
    feedback: { kind: 'waiting', text: '这张单不用箱。接着扫商品。' },
  });
}

/** 换放货位：回到等放货位，扫到合格的再回到原来这一步。提交与认码在途时不让换。 */
export function changeStaging(state: PickState): PickStep {
  const ok = state.phase === 'location' || state.phase === 'goods' || state.phase === 'refused' || state.phase === 'finished';
  if (!ok) return { state, effects: [] };
  return step({ ...state, phase: 'staging', resume: state.phase, feedback: { kind: 'waiting', text: '扫新的待出库库位。' } });
}

// —— 提交 ——

/**
 * 这一笔拣货要填的格：任务号、实拣数量、放货位。事实号与仓由 work-fact-submit 按现有作业台 `pick` 那条定义补，
 * 所以装出来的报文与旧作业台逐键相同（pick-flow.test.ts 把两边钉在一起）。数量按文本交，与旧作业台表单里敲进去的一样，
 * 由定义里的整数格去校验与转成数。
 */
export function pickFactValues(submission: PickSubmission, staging: string): GuidedFormValues {
  const values: GuidedFormValues = { taskId: submission.cell.taskId, quantity: String(submission.quantity), targetLocation: staging };
  // 没用箱就不带这一格：定义里它是可选的，不带时报文里没有这个键
  return submission.tote === undefined ? values : { ...values, toteCode: submission.tote };
}

// —— 提交的结果 ——

/**
 * 工位对个别答复的说法：比通用词表贴切的几句。哪些答复算成、算没成不在这里，在 judgeWorkFactAnswer。
 * 少拣是成功：同一个事实号同样数量重放答已有结果，也是成功。
 */
export const pickOutcomeLabels: Readonly<Record<string, string>> = {
  PICKED: '这一格已记下。',
  EXISTING_RESULT: '这一格之前已经记过了（同一个作业事实号），这次没有重复记。',
  RECORDED: '这一格已经作废：它的出库单已经截单，或从这个波次撤出了。这一笔照记了：拣出来的货已生成回库任务，不要当成发货的货。',
  CONTENT_CONFLICT: '这一格已经不能再拣：它已经被拣过了（含短拣）。这一笔没有记。',
  WAVE_NOT_FOUND: '找不到这一格的拣货任务，它可能已经被撤掉。这一笔没有记。',
  CONTENT_INVALID:
    '后端不认这一笔的内容，没有记。最常见的是放货位不合（不在这个仓、已停用，或所在库区不是待出库用途）；也可能是数量超过了这一格还没拣的件数。换一个待出库库位，或刷新后再来。',
  NOT_CONFIGURED: '未配置：本租户还没有登记仓，拣货记不了。',
  TOTE_IN_USE: '这个箱上绑着别的出库单，这一笔没有记。货先别放进去：点「下一格」，这一格排到队尾，回头再来时换一个箱扫。',
  TOTE_NOT_FOUND: '这个仓没有登记过这个箱号，这一笔没有记。点「下一格」，这一格排到队尾，回头再来时换一个登记过的箱扫。',
  TOTE_NOT_ACCEPTING: '这个箱已经停用，这一笔没有记。点「下一格」，这一格排到队尾，回头再来时换一个箱扫。',
  TOTE_REQUIRED: '这个仓拣货必须用周转箱，这一笔没带箱号，没有记。点「下一格」，这一格排到队尾，回头再来时先扫箱。',
  WAVE_ASSIGNED_TO_OTHER: '这一格所在的波次派给了别人，这一笔没有记。货放回原处；点「下一格」，这一格从你的队列里去掉。',
};

/**
 * 后端不认这张出库单这一趟定下的箱（或定下的「不用箱」）：那个决定要忘掉，回头再来这一格时重新问。
 * 必须用箱这一种（TOTE_REQUIRED）是这一趟开工之后租户才登记的规则，工位当时还不知道。
 */
export function toteRefusedAfter(outcome: string | undefined): boolean {
  return outcome === 'TOTE_IN_USE' || outcome === 'TOTE_NOT_FOUND' || outcome === 'TOTE_NOT_ACCEPTING' || outcome === 'TOTE_REQUIRED';
}

/**
 * 后端答这条任务已经不能拣，或不归这个人拣（波次派给了别人）：「下一格」要把它从队列里去掉，而不是挪到队尾。
 */
export function taskGoneAfter(outcome: string | undefined): boolean {
  return outcome === 'CONTENT_CONFLICT' || outcome === 'WAVE_NOT_FOUND' || outcome === 'WAVE_ASSIGNED_TO_OTHER';
}

function successText(sub: PickSubmission, staging: string | null, verdict: WorkFactVerdict): string {
  if (verdict.outcome !== 'PICKED') return verdict.headline;
  const place = staging ?? '待出库库位';
  if (sub.quantity >= sub.cell.quantity) return `这一格拣好了：${sub.quantity} 件 ${sub.cell.goodsCode} 放到了 ${place}。`;
  const moved = sub.quantity === 0 ? '' : `${sub.quantity} 件已放到 ${place}。`;
  return `这一格按 ${sub.quantity} 件记成短拣（应拣 ${sub.cell.quantity} 件，差 ${sub.cell.quantity - sub.quantity} 件）。${moved}差的若换得出别的库位，会添进余下的格。`;
}

/** 后端答了：成了就这一格出队、进下一格；没送到等「再送一次」；没成说明原因等「下一格」。 */
export function applyVerdict(state: PickState, verdict: WorkFactVerdict): PickStep {
  if (state.phase !== 'submitting' || state.submission === null) return { state, effects: [] };
  const sub = state.submission;
  const batch = state.batch;
  switch (verdict.verdict) {
    case 'done': {
      // 合成一格拿的：这一条记成了，还有没交的就接着交下一条，这一格还在提交中
      if (batch !== null && batch.sent + 1 < batch.members.length) {
        const sent = batch.sent + 1;
        return step(
          {
            ...state,
            batch: { ...batch, sent },
            feedback: { kind: 'waiting', text: `已记 ${sent} / ${batch.members.length} 张出库单，接着记下一张…` },
          },
          undefined,
          [{ kind: 'submit', cell: batch.members[sent].cell, quantity: batch.members[sent].quantity }],
        );
      }
      // 合着拿的每条任务都交过了（没分到的按 0 件交），这一格整个出队，没有留在队里的任务
      const queue = state.queue.filter((cell) => cell.taskId !== sub.cell.taskId);
      return step(
        {
          ...state,
          phase: queue.length === 0 ? 'finished' : 'location',
          queue,
          done: state.done + 1,
          total: state.done + 1 + queue.length,
          counted: 0,
          submission: null,
          batch: null,
          gone: false,
          feedback: { kind: 'ok', text: batch === null ? successText(sub, state.staging, verdict) : batchText(sub, batch.members, state.staging) },
        },
        'ok',
      );
    }
    case 'retry':
      return step({ ...state, phase: 'unsent', feedback: { kind: 'unsent', text: verdict.headline } }, 'wrong');
    case 'refused': {
      let totes = state.totes;
      if (toteRefusedAfter(verdict.outcome)) {
        const { [sub.cell.requestId]: _forgotten, ...rest } = state.totes;
        totes = rest;
      }
      // 后端说这个仓必须用箱：从这一笔起工位也按必须用箱来，不再让人点「不用箱」
      const required = state.toteRequired || verdict.outcome === 'TOTE_REQUIRED';
      return step(
        {
          ...state,
          phase: 'refused',
          gone: taskGoneAfter(verdict.outcome),
          toteOn: state.toteOn || required,
          toteRequired: required,
          totes: required ? withoutDeclined(totes) : totes,
          feedback: {
            kind: 'wrong',
            // 合成一格拿的交到一半没成：前面记成的几张是真的记下了，要说清，手里还有几件没记
            text:
              batch === null
                ? verdict.headline
                : `${verdict.headline} 这一格拿了 ${sub.quantity} 件，已经记下 ${batch.sent} 条任务共 ${allottedTo(batch.members.slice(0, batch.sent))} 件，还有 ${allottedTo(batch.members.slice(batch.sent))} 件没有记。`,
          },
        },
        'wrong',
      );
    }
  }
}

/**
 * 合成的一格去掉已经交过的那几条任务：还剩的仍合成一格，都交完了为 null。只在交到一半后端没成时用——
 * 都交成了的话没有剩下的。交过而没记满的那一条也去掉：它在后端已是短拣，不能再拣。
 */
function withoutMembers(cell: PickCell, taken: readonly PickAllotment[]): PickCell | null {
  const gone = new Set(taken.map((member) => member.cell.taskId));
  const members = (cell.members ?? []).filter((member) => !gone.has(member.taskId));
  if (members.length === 0) return null;
  const head = members[0];
  const quantity = members.reduce((sum, member) => sum + member.quantity, 0);
  return { ...cell, taskId: head.taskId, waveId: head.waveId, requestId: head.requestId, quantity, members };
}

const allottedTo = (members: readonly PickAllotment[]): number => members.reduce((sum, member) => sum + member.quantity, 0);

function batchText(sub: PickSubmission, members: readonly PickAllotment[], staging: string | null): string {
  const place = staging ?? '待出库库位';
  if (sub.quantity >= sub.cell.quantity) {
    return `这一格拣好了：${sub.quantity} 件 ${sub.cell.goodsCode} 分开记给了 ${members.length} 张出库单，放到了 ${place}。`;
  }
  const partial = partialAllotment(members);
  const empty = emptyAllotments(members).length;
  const got = members.length - empty;
  const short =
    partial === undefined
      ? ''
      : `出库单 ${partial.cell.requestId} 只记到 ${partial.quantity} 件（要 ${partial.cell.quantity} 件），按短拣记了。`;
  const none = empty === 0 ? '' : `${got === 0 ? '' : '还有 '}${empty} 条任务一件都没拣到，按 0 件记成了短拣。`;
  const head =
    sub.quantity === 0
      ? `这一格一件 ${sub.cell.goodsCode} 都没拿到。`
      : `这一格拿到 ${sub.quantity} 件 ${sub.cell.goodsCode}，记给了 ${got} 条任务，放到了 ${place}。`;
  return `${head}${short}${none}差的若换得出别的库位，会添进余下的格。`;
}

/**
 * 这一笔记成之后要不要重读队列。短拣成立后，后端会在这个波次里换别的库位补差的那几件；添出来的格在后端，重读才看得到。
 * 合成一格拿的要等最后一条记成：交到一半时队列不让换。一件都没分到、按 0 件交的任务也是短拣，后端同样为它们添格。
 */
export function reloadAfter(before: PickState, verdict: WorkFactVerdict): boolean {
  if (verdict.verdict !== 'done' || verdict.outcome !== 'PICKED' || before.submission === null) return false;
  const batch = before.batch;
  if (batch === null) return before.submission.quantity < before.submission.cell.quantity;
  return batch.sent + 1 >= batch.members.length && anyShortAllotment(batch.members);
}

/** 「再送一次」：用同一个事实号（由接线的一方保留那一笔 attempt）。 */
export function resend(state: PickState): PickStep {
  if (state.phase !== 'unsent') return { state, effects: [] };
  return step({ ...state, phase: 'submitting', feedback: { kind: 'waiting', text: '正在再送一次…' } }, undefined, [{ kind: 'resend' }]);
}

/** 「下一格」：没成的这一格，后端已不能拣的去掉，还能拣的挪到队尾回头再来。 */
export function nextCell(state: PickState): PickStep {
  if (state.phase !== 'refused' || state.submission === null) return { state, effects: [] };
  const [head, ...rest] = state.queue;
  if (state.batch !== null) {
    // 合成一格拿的交到一半没成：记成的那几条去掉；没成的这一条，后端已不能拣的也去掉，还能拣的留着。
    // 剩下的任务仍合成一格排到队尾。
    const batch = state.batch;
    const settled = batch.members.slice(0, state.gone ? batch.sent + 1 : batch.sent);
    const leftover = withoutMembers(head, settled);
    const queue = leftover === null ? rest : [...rest, leftover];
    // 记成过至少一张，这一格算拣过一格；余下的任务是队里新的一格
    const done = batch.sent > 0 ? state.done + 1 : state.done;
    return step({
      ...state,
      phase: queue.length === 0 ? 'finished' : 'location',
      queue,
      done,
      total: done + queue.length,
      counted: 0,
      submission: null,
      batch: null,
      gone: false,
      feedback: {
        kind: 'waiting',
        text:
          leftover === null
            ? `${head.locationCode} 这一格没有余下的任务了。`
            : `${head.locationCode} 这一格还有 ${leftover.members?.length ?? 0} 条任务共 ${leftover.quantity} 件没记，排到了队尾，回头再来。`,
      },
    });
  }
  const queue = state.gone ? rest : [...rest, head];
  return step({
    ...state,
    phase: queue.length === 0 ? 'finished' : 'location',
    queue,
    total: state.gone ? Math.max(state.total - 1, state.done) : state.total,
    counted: 0,
    submission: null,
    gone: false,
    feedback: {
      kind: 'waiting',
      text: state.gone ? `${head.locationCode} 这一格已经不在待拣里，去掉了。` : `${head.locationCode} 这一格没记成，排到了队尾，回头再来。`,
    },
  });
}

// —— 按钮 ——

export interface PickActions {
  changeCount: boolean;
  shortPick: boolean;
  skip: boolean;
  changeStaging: boolean;
  refresh: boolean;
  resend: boolean;
  next: boolean;
  /** 「不用箱」：只在等箱那一步。 */
  skipTote: boolean;
}

/** 这一刻哪些按钮能点。不能点的在界面上禁用，不藏起来：人要看得出它存在，只是现在不是时候。 */
export function pickActions(state: PickState): PickActions {
  const canSkip =
    (state.phase === 'location' || state.phase === 'tote' || (state.phase === 'goods' && state.counted === 0)) && state.queue.length >= 2;
  return {
    changeCount: state.phase === 'goods',
    shortPick: shortPickPlan(state) !== null,
    skip: canSkip,
    changeStaging: state.phase === 'location' || state.phase === 'goods' || state.phase === 'refused' || state.phase === 'finished',
    // 扫了一半的格不让刷新：刷新会把队列换掉，手里那几件就没了去处
    refresh:
      (state.phase === 'staging' ||
        state.phase === 'location' ||
        state.phase === 'tote' ||
        state.phase === 'goods' ||
        state.phase === 'refused' ||
        state.phase === 'finished') &&
      state.counted === 0,
    resend: state.phase === 'unsent',
    next: state.phase === 'refused',
    skipTote: state.phase === 'tote' && !state.toteRequired,
  };
}

/** 扫描框收不收码：只有等放货位、等库位、等周转箱、等商品这几步收。 */
export function acceptingScans(state: PickState): boolean {
  return state.phase === 'staging' || state.phase === 'location' || state.phase === 'tote' || state.phase === 'goods';
}

/** 不收码时扫描框里写的字，让人知道为什么现在不理扫码。 */
export function scanPlaceholder(state: PickState): string {
  switch (state.phase) {
    case 'verifying':
    case 'resolving':
    case 'submitting':
      return '正在处理，先不收扫码';
    case 'unsent':
      return '先点「再送一次」';
    case 'refused':
      return '先点「下一格」';
    case 'finished':
      return '没有要扫的了';
    default:
      return '扫码，或敲进去按回车';
  }
}

// —— 队列读得全不全 ——

export interface QueueScope {
  wavesRead: number;
  truncated: boolean;
  waveMissing: boolean;
  /** 还有格没拣、但派给了别人所以没摆的波次数。 */
  assignedElsewhere?: number;
}

/**
 * 队列上方要如实说的话：只看了多少、指名的波次没找到。读全了没有要说的。
 * 指名的波次是按号读的那一张，没有就是没有；只看了多少只会出在看本仓全部波次、还没拣完的波次多过页数上限的时候。
 */
export function queueNotes(queue: QueueScope, wave: string | null): string[] {
  const notes: string[] = [];
  if (queue.waveMissing && wave !== null) {
    notes.push(`没有找到波次 ${wave}（它可能不属于本仓）。`);
  } else if (queue.truncated) {
    notes.push(`只看了最近放出的 ${queue.wavesRead} 个还没拣完的波次；更早放出的波次里还有没拣的格，这里看不到。`);
  }
  if ((queue.assignedElsewhere ?? 0) > 0) {
    notes.push(`另有 ${queue.assignedElsewhere} 个波次派给了别人，它们的格没有摆在这里。要看其中一个，在「换波次」里选它。`);
  }
  return notes;
}
