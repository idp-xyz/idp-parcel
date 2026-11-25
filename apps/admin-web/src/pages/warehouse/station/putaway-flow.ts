// 上架与移库：先选货主，扫源库位，扫商品或箱号，再扫目标库位，一笔提交一行。
// 不预选货主，不预填数量以外的批号（批号取扫中的那一格库存）。移进次品库区时屏上说明状态会变成次品，报文仍带源状态，后端自己改。

import type { GuidedFormValues } from '../guided-forms';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import type { PlaceVerdict } from './putaway-place';
import type { WorkFactVerdict } from './work-fact-submit';

export interface StockCell {
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
  batchNumber?: string;
  expiryDate?: string;
  inboundBatchId?: string;
}

export interface StockCarton {
  cartonNumber: string;
  summary: string;
}

export type PutawayPhase = 'owner' | 'source' | 'verifying' | 'goods' | 'target' | 'submitting' | 'unsent';

export interface PutawayDraft {
  carton: boolean;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  quantity: number;
  batchNumber?: string;
  expiryDate?: string;
  inboundBatchId?: string;
  cartonNumber?: string;
}

export interface PutawayState {
  phase: PutawayPhase;
  owner: string | null;
  source: string | null;
  target: string | null;
  /** 目标库区是次品时为真，提交前说一声。 */
  targetDefective: boolean;
  draft: PutawayDraft | null;
  cells: StockCell[];
  cartons: StockCarton[];
  moved: number;
  feedback: { tone: 'waiting' | 'ok' | 'wrong' | 'unsent'; headline: string; detail: string };
  scanned: string;
  cue: ScanCue | null;
}

export type PutawayEffect =
  | { kind: 'verify'; code: string; role: 'source' | 'target-goods' | 'target-carton' }
  | { kind: 'load'; owner: string; source: string }
  | { kind: 'submit'; values: GuidedFormValues }
  | { kind: 'resend' };

export interface PutawayStep {
  state: PutawayState;
  effects: PutawayEffect[];
}

function fresh(owner: string | null): PutawayState {
  return {
    phase: owner ? 'source' : 'owner',
    owner,
    source: null,
    target: null,
    targetDefective: false,
    draft: null,
    cells: [],
    cartons: [],
    moved: 0,
    feedback: owner
      ? { tone: 'waiting', headline: '扫源库位', detail: '等你扫码。' }
      : { tone: 'waiting', headline: '先选货主', detail: '库存按货主取，一趟只做一位。' },
    scanned: '',
    cue: null,
  };
}

export function startPutaway(): PutawayStep {
  return { state: fresh(null), effects: [] };
}

function hold(state: PutawayState): PutawayStep {
  return { state, effects: [] };
}

export function chooseOwner(state: PutawayState, owner: string): PutawayStep {
  if (state.phase === 'verifying' || state.phase === 'submitting') return hold(state);
  const next = fresh(owner);
  next.moved = state.moved;
  return hold(next);
}

export function scanCode(state: PutawayState, raw: string): PutawayStep {
  if (state.phase === 'verifying' || state.phase === 'submitting' || state.phase === 'owner') return hold(state);
  const code = normalizeScan(raw);
  if (code === '') return hold(state);
  if (state.phase === 'source') {
    return {
      state: { ...state, phase: 'verifying', scanned: code, feedback: { tone: 'waiting', headline: '在核这个库位', detail: code }, cue: null },
      effects: [{ kind: 'verify', code, role: 'source' }],
    };
  }
  if (state.phase === 'goods') return scanGoods(state, code);
  if (state.phase === 'target' && state.draft) {
    return {
      state: { ...state, phase: 'verifying', scanned: code, feedback: { tone: 'waiting', headline: '在核目标库位', detail: code }, cue: null },
      effects: [{ kind: 'verify', code, role: state.draft.carton ? 'target-carton' : 'target-goods' }],
    };
  }
  return hold(state);
}

function scanGoods(state: PutawayState, code: string): PutawayStep {
  const carton = state.cartons.find((item) => item.cartonNumber === code);
  if (carton) {
    return hold({
      ...state,
      phase: 'target',
      draft: { carton: true, cartonNumber: code, quantity: 1 },
      feedback: { tone: 'ok', headline: '扫目标库位', detail: `移整箱 ${code}。` },
      scanned: code,
      cue: 'ok',
    });
  }
  const cell = state.cells.find((item) => item.goodsCode === code);
  if (!cell) {
    if (state.draft) {
      return {
        state: { ...state, phase: 'verifying', scanned: code, feedback: { tone: 'waiting', headline: '在核目标库位', detail: code }, cue: null },
        effects: [{ kind: 'verify', code, role: state.draft.carton ? 'target-carton' : 'target-goods' }],
      };
    }
    return hold({
      ...state,
      feedback: { tone: 'wrong', headline: '这个源库位里没有这个货', detail: `${code} 没有记。换一个，或先确认货主。` },
      scanned: code,
      cue: 'wrong',
    });
  }
  const same = state.draft && !state.draft.carton && state.draft.goodsCode === cell.goodsCode && state.draft.stockState === cell.stockState && state.draft.inboundBatchId === cell.inboundBatchId;
  const quantity = same ? (state.draft?.quantity ?? 0) + 1 : 1;
  return hold({
    ...state,
    draft: {
      carton: false,
      goodsCode: cell.goodsCode,
      unit: cell.unit,
      stockState: cell.stockState,
      quantity,
      batchNumber: cell.batchNumber,
      expiryDate: cell.expiryDate,
      inboundBatchId: cell.inboundBatchId,
    },
    feedback: { tone: 'ok', headline: quantity >= cell.quantity ? '可以扫目标库位' : '接着扫，或扫目标库位', detail: `${cell.goodsCode} 已扫 ${quantity} / 在库 ${cell.quantity}。` },
    scanned: code,
    cue: 'ok',
  });
}

export function placeVerdict(state: PutawayState, code: string, verdict: PlaceVerdict): PutawayStep {
  if (state.phase !== 'verifying') return hold(state);
  if (verdict.kind !== 'ok') {
    // 目标没核过就回到扫货：核过的源库位还在，人再扫一个目标。源本身没核过才回到扫源库位。
    const back = state.source === null ? 'source' : 'goods';
    const headline = verdict.kind === 'unverified' ? '这一次没核成' : '这个库位不能用';
    const tone = verdict.kind === 'unverified' ? 'unsent' : 'wrong';
    return hold({ ...state, phase: back, feedback: { tone, headline, detail: verdict.reason }, cue: 'wrong' });
  }
  if (state.source === null) {
    return {
      state: { ...state, phase: 'goods', source: code, feedback: { tone: 'waiting', headline: '扫商品或箱号', detail: `源库位 ${code}。` }, cue: 'ok', scanned: '' },
      effects: state.owner ? [{ kind: 'load', owner: state.owner, source: code }] : [],
    };
  }
  const next: PutawayState = {
    ...state,
    phase: 'submitting',
    target: code,
    targetDefective: verdict.usage === 'DEFECTIVE',
    feedback: {
      tone: 'waiting',
      headline: verdict.usage === 'DEFECTIVE' ? '目标在次品库区，提交后库存状态变成次品' : '正在提交这一笔',
      detail: `从 ${state.source} 到 ${code}。`,
    },
    cue: null,
  };
  return { state: next, effects: [{ kind: 'submit', values: moveValues(next) }] };
}

export function stockLoaded(state: PutawayState, cells: StockCell[], cartons: StockCarton[], failed?: string): PutawayStep {
  if (failed) {
    return hold({ ...state, feedback: { tone: 'unsent', headline: '这个库位的库存没读成', detail: failed }, cue: 'wrong' });
  }
  return hold({ ...state, cells, cartons, feedback: { ...state.feedback, detail: cells.length + cartons.length === 0 ? '这个库位里这位货主没有货。' : state.feedback.detail } });
}

export function setQuantity(state: PutawayState, quantity: number): PutawayStep {
  if (state.phase !== 'goods' && state.phase !== 'target') return hold(state);
  if (!state.draft || state.draft.carton || !Number.isInteger(quantity) || quantity < 1) return hold(state);
  return hold({ ...state, draft: { ...state.draft, quantity } });
}

export function submitMove(state: PutawayState): PutawayStep {
  if (!state.owner || !state.source || !state.target || !state.draft) return hold(state);
  if (state.phase !== 'target') return hold(state);
  return {
    state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: '正在提交这一笔', detail: '' }, cue: null },
    effects: [{ kind: 'submit', values: moveValues(state) }],
  };
}

export function moveValues(state: PutawayState): GuidedFormValues {
  const draft = state.draft;
  const row: Record<string, string> = {
    ownerAccount: state.owner ?? '',
    sourceLocation: state.source ?? '',
    targetLocation: state.target ?? '',
  };
  if (draft && !draft.carton) {
    row.goodsCode = draft.goodsCode ?? '';
    row.unit = draft.unit ?? '';
    row.stockState = draft.stockState ?? '';
    row.quantity = String(draft.quantity);
    if (draft.batchNumber) row.batchNumber = draft.batchNumber;
    if (draft.expiryDate) row.expiryDate = draft.expiryDate;
    if (draft.inboundBatchId) row.inboundBatchId = draft.inboundBatchId;
  }
  if (draft?.carton && draft.cartonNumber) row.cartonNumber = draft.cartonNumber;
  return { lines: [row] };
}

export function moveVerdict(state: PutawayState, verdict: WorkFactVerdict): PutawayStep {
  if (state.phase !== 'submitting' && state.phase !== 'unsent') return hold(state);
  if (verdict.verdict === 'retry') {
    return hold({ ...state, phase: 'unsent', feedback: { tone: 'unsent', headline: '这一笔没送到', detail: '再送一次用的是同一个作业事实号。' }, cue: 'wrong' });
  }
  if (verdict.verdict !== 'done') {
    return hold({ ...state, phase: 'target', feedback: { tone: 'wrong', headline: verdict.headline, detail: '这一笔没有记下。' }, cue: 'wrong' });
  }
  const next = fresh(state.owner);
  next.moved = state.moved + 1;
  next.feedback = {
    tone: 'ok',
    headline: verdict.outcome === 'SHORTAGE_REGISTERED' ? '够的部分已移走，不够的记成了作业差异' : '这一笔已经移过去了',
    detail: '可以扫下一个源库位。',
  };
  next.cue = 'ok';
  return hold(next);
}

export function resend(state: PutawayState): PutawayStep {
  if (state.phase !== 'unsent') return hold(state);
  return { state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: '正在再送一次', detail: '' }, cue: null }, effects: [{ kind: 'resend' }] };
}
