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
// 屏上文字与扫码顺序照屏幕示意里拣货那一项：开工扫一次待出库库位 → 扫库位码 → 逐件扫商品加一（也可直接改数量）→
// 扫满自动提交这一格、跳到下一格。

import type { GuidedFormValues } from '../guided-forms';
import type { GoodsIdentification } from './pick-goods';
import type { PickCell } from './pick-queue';
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

export interface PickSubmission {
  cell: PickCell;
  quantity: number;
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
  /** 没成的那一笔里，这条任务在后端是不是已经不能拣了：是，「下一格」把它从队列里去掉；不是，挪到队尾。 */
  gone: boolean;
}

export type PickEffect =
  | { kind: 'identify'; cell: PickCell; code: string }
  | { kind: 'verify-staging'; code: string }
  | { kind: 'submit'; cell: PickCell; quantity: number }
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
    gone: false,
  };
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
        counter: `这一格要拣 ${cell.goodsCode}，共 ${cell.quantity} 件`,
      };
    case 'goods':
    case 'resolving':
      return {
        label,
        headline: wrong ? (wrongHeadlines.goods as string) : `扫商品 ${cell.goodsCode}，还差 ${remaining(state)} 件`,
        counter: `已扫 ${state.counted} / ${cell.quantity}`,
      };
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
  const short = sub.quantity < sub.cell.quantity ? `（应拣 ${sub.cell.quantity} 件，按短拣记）` : '';
  return `${sub.cell.goodsCode} ${sub.quantity} 件${short}，放到 ${state.staging ?? '待出库库位'}`;
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
  return step(
    {
      ...state,
      phase: 'submitting',
      counted: count,
      submission: { cell, quantity: cell.quantity },
      feedback: { ...feedback, text: `${text}扫满了，正在提交这一格。` },
    },
    'ok',
    [{ kind: 'submit', cell, quantity: cell.quantity }],
  );
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
      return step(
        { ...state, phase: 'goods', counted: 0, feedback: { kind: 'ok', text: `对。到了 ${cell.locationCode}，接着扫商品。`, scanned: code } },
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
  const scanning = state.phase === 'staging' || state.phase === 'location' || state.phase === 'goods';
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
  if (value < 1) return { ok: false, reason: '至少 1 件。一件都拣不到的格，用「跳过这一格」。' };
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

/** 少拣要满足：在等商品的阶段，已扫的数在 1 与应拣之间（不含应拣：够数就已经自动提交了）。不满足答 null。 */
export function shortPickPlan(state: PickState): ShortPickPlan | null {
  if (state.phase !== 'goods' || state.staging === null) return null;
  const cell = state.queue[0];
  if (state.counted < 1 || state.counted >= cell.quantity) return null;
  return { cell, quantity: state.counted, short: cell.quantity - state.counted, staging: state.staging };
}

/** 少拣确认框里的话：这几件移进待出库库位、这一格记成短拣并记一笔差额、差的由系统换别的库位补。 */
export function shortPickLines(plan: ShortPickPlan): string[] {
  return [
    `这一格要拣 ${plan.cell.quantity} 件 ${plan.cell.goodsCode}，现在只记 ${plan.quantity} 件。`,
    `这 ${plan.quantity} 件移进待出库库位 ${plan.staging}。`,
    `这一格记成「短拣」，提交之后不能再拣，差额记一笔短拣差异。差的 ${plan.short} 件系统会在这个波次里换别的库位补：换得出就添进余下的格，换不出就等这张出库单别的格拣完后回到「已预占」。`,
  ];
}

/** 确认之后提交：按已扫的数。 */
export function submitShortPick(state: PickState): PickStep {
  const plan = shortPickPlan(state);
  if (plan === null) return { state, effects: [] };
  return step(
    {
      ...state,
      phase: 'submitting',
      submission: { cell: plan.cell, quantity: plan.quantity },
      feedback: { kind: 'waiting', text: `按 ${plan.quantity} 件提交这一格（短拣，差 ${plan.short} 件）…` },
    },
    undefined,
    [{ kind: 'submit', cell: plan.cell, quantity: plan.quantity }],
  );
}

// —— 跳过、换放货位 ——

/** 跳过这一格：挪到队尾，不提交。扫了数的不让跳——那几件已经拿在手里，要么提交要么少拣。队里只有这一格时也没处可挪。 */
export function skipCell(state: PickState): PickStep {
  const onCell = state.phase === 'location' || (state.phase === 'goods' && state.counted === 0);
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
  return { taskId: submission.cell.taskId, quantity: String(submission.quantity), targetLocation: staging };
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
};

/** 后端答这条任务已经不能拣：「下一格」要把它从队列里去掉，而不是挪到队尾。 */
export function taskGoneAfter(outcome: string | undefined): boolean {
  return outcome === 'CONTENT_CONFLICT' || outcome === 'WAVE_NOT_FOUND';
}

function successText(sub: PickSubmission, staging: string | null, verdict: WorkFactVerdict): string {
  if (verdict.outcome !== 'PICKED') return verdict.headline;
  const place = staging ?? '待出库库位';
  return sub.quantity < sub.cell.quantity
    ? `这一格按 ${sub.quantity} 件记成短拣（应拣 ${sub.cell.quantity} 件，差 ${sub.cell.quantity - sub.quantity} 件）。${sub.quantity} 件已放到 ${place}。差的若换得出别的库位，会添进余下的格。`
    : `这一格拣好了：${sub.quantity} 件 ${sub.cell.goodsCode} 放到了 ${place}。`;
}

/** 后端答了：成了就这一格出队、进下一格；没送到等「再送一次」；没成说明原因等「下一格」。 */
export function applyVerdict(state: PickState, verdict: WorkFactVerdict): PickStep {
  if (state.phase !== 'submitting' || state.submission === null) return { state, effects: [] };
  const sub = state.submission;
  switch (verdict.verdict) {
    case 'done': {
      const queue = state.queue.filter((cell) => cell.taskId !== sub.cell.taskId);
      return step(
        {
          ...state,
          phase: queue.length === 0 ? 'finished' : 'location',
          queue,
          done: state.done + 1,
          counted: 0,
          submission: null,
          gone: false,
          feedback: { kind: 'ok', text: successText(sub, state.staging, verdict) },
        },
        'ok',
      );
    }
    case 'retry':
      return step({ ...state, phase: 'unsent', feedback: { kind: 'unsent', text: verdict.headline } }, 'wrong');
    case 'refused':
      return step(
        { ...state, phase: 'refused', gone: taskGoneAfter(verdict.outcome), feedback: { kind: 'wrong', text: verdict.headline } },
        'wrong',
      );
  }
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
}

/** 这一刻哪些按钮能点。不能点的在界面上禁用，不藏起来：人要看得出它存在，只是现在不是时候。 */
export function pickActions(state: PickState): PickActions {
  const canSkip = (state.phase === 'location' || (state.phase === 'goods' && state.counted === 0)) && state.queue.length >= 2;
  return {
    changeCount: state.phase === 'goods',
    shortPick: shortPickPlan(state) !== null,
    skip: canSkip,
    changeStaging: state.phase === 'location' || state.phase === 'goods' || state.phase === 'refused' || state.phase === 'finished',
    // 扫了一半的格不让刷新：刷新会把队列换掉，手里那几件就没了去处
    refresh:
      (state.phase === 'staging' ||
        state.phase === 'location' ||
        state.phase === 'goods' ||
        state.phase === 'refused' ||
        state.phase === 'finished') &&
      state.counted === 0,
    resend: state.phase === 'unsent',
    next: state.phase === 'refused',
  };
}

/** 扫描框收不收码：只有等放货位、等库位、等商品这三步收。 */
export function acceptingScans(state: PickState): boolean {
  return state.phase === 'staging' || state.phase === 'location' || state.phase === 'goods';
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
  return notes;
}
