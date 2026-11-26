// 发货交接的会话状态机。扫出库单号，屏上列出包裹；再扫这张单的号，或点「交出这个包裹」，提交还没交出的下一只。
// 离线已交出是单独的按钮：它会照记并消耗待出库的货，不能让一次普通扫码触发。
// 判断在这里；找单与读包裹在接线里。

import type { OrderLookup } from './pack-order';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import {
  handoffSpec,
  handoffValues,
  judgeDispatch,
  markParcelDone,
  nextParcel,
  type DispatchSheet,
} from './dispatch-sheet';
import type { WorkFactVerdict } from './work-fact-submit';

export type DispatchPhase = 'waiting' | 'finding' | 'opening' | 'handing' | 'submitting' | 'unsent' | 'refused';

export type DispatchEffect =
  | { kind: 'find'; code: string }
  | { kind: 'open'; requestId: string }
  | { kind: 'submit'; spec: 'outbound-confirmation' | 'pickup-handover'; values: ReturnType<typeof handoffValues> }
  | { kind: 'resend' };

export interface DispatchFeedback {
  tone: 'waiting' | 'ok' | 'wrong' | 'unsent';
  headline: string;
  detail: string;
}

export interface DispatchState {
  phase: DispatchPhase;
  sheet: DispatchSheet | null;
  /** 这一只正在交，离线与否在提交那一刻定下。 */
  pending: { sequence: number; offline: boolean } | null;
  /** 这一趟已经交出的包裹数，换单不清到零以外的展示用。 */
  handed: number;
  party: string;
  credential: string;
  feedback: DispatchFeedback;
  scanned: string;
  cue: ScanCue | null;
}

export interface DispatchStep {
  state: DispatchState;
  effects: DispatchEffect[];
}

const wait: DispatchFeedback = { tone: 'waiting', headline: '扫出库单号', detail: '等你扫码。' };

export function startDispatch(requestId: string | null): DispatchStep {
  const state: DispatchState = {
    phase: requestId ? 'opening' : 'waiting',
    sheet: null,
    pending: null,
    handed: 0,
    party: '',
    credential: '',
    feedback: requestId ? { tone: 'waiting', headline: '在读这张单的包裹', detail: '' } : wait,
    scanned: '',
    cue: null,
  };
  return { state, effects: requestId ? [{ kind: 'open', requestId }] : [] };
}

function hold(state: DispatchState): DispatchStep {
  return { state, effects: [] };
}

const busy: ReadonlySet<DispatchPhase> = new Set(['finding', 'opening', 'submitting']);

export function scanCode(state: DispatchState, raw: string): DispatchStep {
  if (busy.has(state.phase)) return hold(state);
  const code = normalizeScan(raw);
  if (code === '') return hold(state);
  if (state.sheet && (code === state.sheet.requestId || code === state.sheet.number) && state.phase === 'handing') {
    return submitNext(state, false);
  }
  return {
    state: {
      ...state,
      phase: 'finding',
      sheet: null,
      pending: null,
      scanned: code,
      cue: null,
      feedback: { tone: 'waiting', headline: '在找这张出库单', detail: code },
    },
    effects: [{ kind: 'find', code }],
  };
}

export function orderFound(state: DispatchState, lookup: OrderLookup): DispatchStep {
  if (state.phase !== 'finding') return hold(state);
  if (lookup.kind === 'unverified') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'unsent', headline: '这一次没核成', detail: lookup.reason }, cue: 'wrong' });
  }
  // 发货交接这里只按出库单号找，不按周转箱找（打完包货已经不在箱里）；没绑单的箱这一种答复它收不到，收到了也按没有这张单说
  if (lookup.kind === 'absent' || lookup.kind === 'emptyTote') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '没有这张出库单', detail: `${state.scanned} 在这个仓和别的仓都没找到。` }, cue: 'wrong' });
  }
  if (lookup.kind === 'elsewhere') {
    return hold({
      ...state,
      phase: 'waiting',
      feedback: { tone: 'wrong', headline: '这张单不在这个仓', detail: `${lookup.number} 属于仓 ${lookup.warehouse}。` },
      cue: 'wrong',
    });
  }
  return { state: { ...state, phase: 'opening', feedback: { tone: 'waiting', headline: '在读这张单的包裹', detail: '' }, cue: null }, effects: [{ kind: 'open', requestId: lookup.row.requestId }] };
}

export function orderOpened(state: DispatchState, sheet: DispatchSheet | null, warehouse: string): DispatchStep {
  if (state.phase !== 'opening') return hold(state);
  if (sheet === null) {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '没有这张出库单', detail: '读包裹时这张单不在了。' }, cue: 'wrong' });
  }
  const fit = judgeDispatch(sheet, warehouse);
  if (fit.kind === 'elsewhere') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '这张单不在这个仓', detail: `它属于仓 ${fit.warehouse}。` }, cue: 'wrong' });
  }
  if (fit.kind === 'not-packed') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '这张单还不能交出', detail: fit.text }, cue: 'wrong' });
  }
  if (fit.kind === 'no-parcels') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '这张单还没有包裹', detail: '先去打包。' }, cue: 'wrong' });
  }
  const next = nextParcel(sheet);
  if (next === null) {
    return hold({ ...state, phase: 'waiting', sheet, feedback: { tone: 'ok', headline: '这张单的包裹都已交出', detail: sheet.number }, cue: 'ok' });
  }
  return hold({
    ...state,
    phase: 'handing',
    sheet,
    feedback: {
      tone: 'waiting',
      headline: sheet.pickup ? '填提货方和凭证，再交出' : `交出包裹 ${next.sequence}`,
      detail: `${sheet.dispatchLabel}。再扫一次这张单的号，或点「交出这个包裹」。`,
    },
    cue: null,
  });
}

function submitNext(state: DispatchState, offline: boolean): DispatchStep {
  if (state.phase !== 'handing' || state.sheet === null) return hold(state);
  const parcel = nextParcel(state.sheet);
  if (parcel === null) {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'ok', headline: '这张单的包裹都已交出', detail: state.sheet.number }, cue: 'ok' });
  }
  return {
    state: {
      ...state,
      phase: 'submitting',
      pending: { sequence: parcel.sequence, offline },
      feedback: { tone: 'waiting', headline: offline ? `正在照记包裹 ${parcel.sequence}（离线已交出）` : `正在交出包裹 ${parcel.sequence}`, detail: '' },
      cue: null,
    },
    effects: [{ kind: 'submit', spec: handoffSpec(state.sheet), values: handoffValues(state.sheet, parcel.sequence, offline, state.party, state.credential) }],
  };
}

export function confirmParcel(state: DispatchState): DispatchStep {
  return submitNext(state, false);
}

export function confirmOffline(state: DispatchState): DispatchStep {
  return submitNext(state, true);
}

export function setPickup(state: DispatchState, party: string, credential: string): DispatchStep {
  if (state.phase !== 'handing') return hold(state);
  return hold({ ...state, party, credential });
}

export function handoffVerdict(state: DispatchState, verdict: WorkFactVerdict): DispatchStep {
  if (state.phase !== 'submitting' && state.phase !== 'unsent') return hold(state);
  if (verdict.verdict === 'retry') {
    return hold({ ...state, phase: 'unsent', feedback: { tone: 'unsent', headline: '这一笔没送到', detail: '再送一次用的是同一个作业事实号。' }, cue: 'wrong' });
  }
  if (verdict.verdict !== 'done' || state.sheet === null || state.pending === null) {
    return hold({ ...state, phase: 'handing', pending: null, feedback: { tone: 'wrong', headline: verdict.headline, detail: '这个包裹没有记下。' }, cue: 'wrong' });
  }
  const sheet = markParcelDone(state.sheet, state.pending.sequence);
  const handed = state.handed + 1;
  const next = nextParcel(sheet);
  if (next === null) {
    return hold({
      ...state,
      phase: 'waiting',
      sheet: null,
      pending: null,
      handed,
      feedback: { tone: 'ok', headline: '这张单的包裹都已交出', detail: verdict.headline },
      cue: 'ok',
    });
  }
  return hold({
    ...state,
    phase: 'handing',
    sheet,
    pending: null,
    handed,
    feedback: { tone: 'ok', headline: `包裹 ${state.pending.sequence} 已交出`, detail: `下一只是包裹 ${next.sequence}。` },
    cue: 'ok',
  });
}

export function resend(state: DispatchState): DispatchStep {
  if (state.phase !== 'unsent') return hold(state);
  return {
    state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: '正在再送一次', detail: '用的是同一个作业事实号。' }, cue: null },
    effects: [{ kind: 'resend' }],
  };
}

export function leaveOrder(state: DispatchState): DispatchStep {
  if (busy.has(state.phase)) return hold(state);
  return hold({ ...startDispatch(null).state, handed: state.handed, feedback: { ...wait, detail: '刚才没送到的那一笔丢掉了。' } });
}
