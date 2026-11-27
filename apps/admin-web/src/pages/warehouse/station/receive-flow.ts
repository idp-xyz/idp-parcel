// 收货工位的会话状态机（纯 TypeScript，node:test 直接测）：现在等扫什么、扫到一个码之后变成什么、该提交什么。
// 判断一律在这里。预报能不能收、报文怎么装在 receive-sheet.ts；暂存库位核没核成是 StagingVerdict。
//
// 一趟的次序：扫入库单号（预报标识或单据号）→ 扫一次收货暂存库位（本机记着的先重核）→ 商品入库要先自己选这一批的库存状态，
// 不预选 → 逐件扫货或逐箱扫箱号 → 人点「提交这一次收货」。一张预报可以分几次收，提交完回到等扫单号，库位与库存状态留着。
// 认不出的码不记数，等人点「照收」或「登记成待认领」。

import type { StagingVerdict } from './pick-staging-check';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import {
  addGoods,
  adviceMatches,
  cartonFactValues,
  goodsFactValues,
  judgeAdvice,
  unclaimedFactValues,
  type AdviceFit,
  type AdviceSheet,
  type CartonDraft,
  type GoodsDraft,
} from './receive-sheet';
import type { WorkFactVerdict } from './work-fact-submit';

export type ReceivePhase =
  | 'waiting'
  | 'finding'
  | 'resolving'
  | 'locating'
  | 'verifying'
  | 'state'
  | 'goods'
  | 'asking'
  | 'submitting'
  | 'unsent'
  | 'refused';

export type ReceiveSpecId = 'receipt-goods' | 'receipt-carton' | 'unclaimed-arrival';

export type ReceiveEffect =
  | { kind: 'find'; code: string }
  | { kind: 'verify-location'; code: string }
  | { kind: 'identify'; owner: string; code: string }
  | { kind: 'submit'; spec: ReceiveSpecId; values: ReturnType<typeof goodsFactValues> }
  | { kind: 'resend' }
  | { kind: 'remember-location'; code: string };

export interface Ask {
  /** 扫到的码，照收时它就是 observed。 */
  observed: string;
  goodsCode: string | null;
  unit: string;
  /** 单位还要人填（认不出的货没有包装层级可取）。 */
  needUnit: boolean;
  needBatch: boolean;
  needExpiry: boolean;
  /** 预报行上已经写了的，追问时先摆上，人可以改。 */
  batchNumber: string;
  expiryDate: string;
}

export interface ReceiveFeedback {
  tone: 'waiting' | 'ok' | 'wrong' | 'unsent';
  headline: string;
  detail: string;
}

export interface ReceiveState {
  phase: ReceivePhase;
  advice: AdviceSheet | null;
  fit: AdviceFit | null;
  location: string | null;
  /** 这一批的库存状态。人没选过是 null，不预选。整箱入库用不上。 */
  stockState: string | null;
  goods: GoodsDraft[];
  cartons: CartonDraft[];
  /** 下一只箱记成外箱破损。 */
  damageNext: boolean;
  ask: Ask | null;
  /** 认不出时等人决定照收还是登记待认领。 */
  stray: string | null;
  feedback: ReceiveFeedback;
  scanned: string;
  cue: ScanCue | null;
}

export interface ReceiveStep {
  state: ReceiveState;
  effects: ReceiveEffect[];
}

const waitingFeedback = (detail = '等你扫码。'): ReceiveFeedback => ({
  tone: 'waiting',
  headline: '扫入库单号',
  detail,
});

const receivableStates = ['GOOD', 'PENDING_INSPECTION', 'DEFECTIVE'] as const;

/** 这一仓上次选过的库存状态。不在这三种里、或还没选过，就是没选。 */
export function rememberedStockState(value: string | null): string | null {
  return value !== null && (receivableStates as readonly string[]).includes(value) ? value : null;
}

export function startReceive(location: string | null, adviceId: string | null, stockState: string | null = null): ReceiveStep {
  const base: ReceiveState = {
    phase: adviceId ? 'finding' : 'waiting',
    advice: null,
    fit: null,
    location,
    stockState: rememberedStockState(stockState),
    goods: [],
    cartons: [],
    damageNext: false,
    ask: null,
    stray: null,
    feedback: adviceId ? { tone: 'waiting', headline: '在找这张预报', detail: '等读口答。' } : waitingFeedback(),
    scanned: '',
    cue: null,
  };
  return { state: base, effects: adviceId ? [{ kind: 'find', code: adviceId }] : [] };
}

function hold(state: ReceiveState): ReceiveStep {
  return { state, effects: [] };
}

function say(state: ReceiveState, feedback: ReceiveFeedback, cue: ScanCue | null, scanned = state.scanned): ReceiveStep {
  return hold({ ...state, feedback, cue, scanned });
}

const busy: ReadonlySet<ReceivePhase> = new Set(['finding', 'resolving', 'verifying', 'submitting']);

export function scanCode(state: ReceiveState, raw: string): ReceiveStep {
  if (busy.has(state.phase) || state.phase === 'asking') return hold(state);
  const code = normalizeScan(raw);
  if (code === '') return hold(state);
  if (state.phase === 'waiting' || state.phase === 'unsent' || state.phase === 'refused') {
    return {
      state: {
        ...state,
        phase: 'finding',
        advice: null,
        fit: null,
        goods: [],
        cartons: [],
        ask: null,
        stray: null,
        scanned: code,
        cue: null,
        feedback: { tone: 'waiting', headline: '在找这张预报', detail: code },
      },
      effects: [{ kind: 'find', code }],
    };
  }
  if (state.phase === 'locating') {
    return {
      state: { ...state, phase: 'verifying', scanned: code, cue: null, feedback: { tone: 'waiting', headline: '在核这个库位', detail: code } },
      effects: [{ kind: 'verify-location', code }],
    };
  }
  if (state.phase === 'state') {
    return say(state, { tone: 'wrong', headline: '先选这一批的库存状态', detail: '良品、待检、次品里选一个，不替你选。' }, 'wrong', code);
  }
  if (state.phase !== 'goods' || state.advice === null) return hold(state);
  if (adviceMatches(state.advice, code)) {
    return say(state, { tone: 'wrong', headline: '这是这张预报的号', detail: '现在要扫货，不是再扫一次单号。' }, 'wrong', code);
  }
  if (state.advice.inboundMode === 'BY_CARTON') return scanCarton(state, code);
  return {
    state: { ...state, phase: 'resolving', scanned: code, cue: null, stray: null, feedback: { tone: 'waiting', headline: '在认这个码', detail: code } },
    effects: [{ kind: 'identify', owner: state.advice.ownerAccount, code }],
  };
}

function scanCarton(state: ReceiveState, code: string): ReceiveStep {
  const advice = state.advice;
  if (advice === null) return hold(state);
  const known = advice.cartons.find((carton) => carton.cartonNumber === code);
  if (known === undefined) {
    return say(
      { ...state, stray: code },
      { tone: 'wrong', headline: '这张预报里没有这只箱', detail: `${code} 没有记数。可以登记成待认领，不能照箱收。` },
      'wrong',
      code,
    );
  }
  if (state.cartons.some((line) => line.cartonNumber === code)) {
    return say(state, { tone: 'wrong', headline: '这只箱这一次已经扫过', detail: `${code} 不重复记。` }, 'wrong', code);
  }
  const damaged = state.damageNext;
  return say(
    { ...state, cartons: [...state.cartons, { cartonNumber: code, damaged }], damageNext: false, stray: null },
    { tone: 'ok', headline: damaged ? `${code} 记下了，外箱破损` : `${code} 记下了`, detail: known.summary || '这只箱在预报上。' },
    'ok',
    code,
  );
}

export type FoundAdvice =
  | { kind: 'sheet'; sheet: AdviceSheet }
  | { kind: 'bad-mode'; mode: string }
  | { kind: 'absent' }
  | { kind: 'unread'; reason: string };

function afterAdvice(state: ReceiveState, sheet: AdviceSheet, fit: AdviceFit): ReceiveStep {
  const divert = fit.kind === 'divert';
  const next: ReceiveState = {
    ...state,
    advice: sheet,
    fit,
    goods: [],
    cartons: [],
    stray: null,
    scanned: '',
    cue: null,
  };
  const note = divert ? '这张预报已取消或已完结，提交后不入账，转成待认领。' : '等你扫码。';
  if (state.location === null) {
    return hold({ ...next, phase: 'locating', feedback: { tone: 'waiting', headline: '扫收货暂存库位', detail: note } });
  }
  if (sheet.inboundMode === 'BY_GOODS' && state.stockState === null) {
    return hold({ ...next, phase: 'state', feedback: { tone: 'waiting', headline: '选这一批的库存状态', detail: note } });
  }
  return hold({ ...next, phase: 'goods', feedback: { tone: divert ? 'unsent' : 'waiting', headline: '接着扫货', detail: note } });
}

export function adviceFound(state: ReceiveState, found: FoundAdvice, warehouse: string): ReceiveStep {
  if (state.phase !== 'finding' || state.advice !== null) return hold(state);
  if (found.kind === 'unread') {
    return hold({
      ...state,
      phase: 'waiting',
      feedback: { tone: 'unsent', headline: '这一次没核成', detail: found.reason },
      cue: 'wrong',
    });
  }
  if (found.kind === 'absent') {
    return hold({
      ...state,
      phase: 'waiting',
      feedback: { tone: 'wrong', headline: '没有这张预报', detail: `${state.scanned} 按单据号和预报标识都没找到。` },
      cue: 'wrong',
    });
  }
  if (found.kind === 'bad-mode') {
    return hold({
      ...state,
      phase: 'waiting',
      feedback: { tone: 'wrong', headline: '这张预报的入库形态这里不收', detail: '只收按商品或按整箱的预报。' },
      cue: 'wrong',
    });
  }
  const fit = judgeAdvice(found.sheet, warehouse);
  if (fit.kind === 'elsewhere') {
    return hold({
      ...state,
      phase: 'waiting',
      feedback: { tone: 'wrong', headline: '这张预报不在这个仓', detail: `它属于仓 ${fit.warehouse}，这台设备绑的是仓 ${warehouse}。` },
      cue: 'wrong',
    });
  }
  return afterAdvice(state, found.sheet, fit);
}

export function locationVerdict(state: ReceiveState, code: string, verdict: StagingVerdict): ReceiveStep {
  if (state.phase !== 'verifying') return hold(state);
  if (verdict.kind === 'unverified') {
    return hold({
      ...state,
      phase: 'locating',
      feedback: { tone: 'unsent', headline: '这一次没核成', detail: `${verdict.reason}。再扫一次。` },
      cue: 'wrong',
    });
  }
  if (verdict.kind === 'rejected') {
    return hold({
      ...state,
      phase: 'locating',
      feedback: { tone: 'wrong', headline: '这个库位不能收货', detail: verdict.reason },
      cue: 'wrong',
    });
  }
  const placed: ReceiveState = { ...state, location: code, cue: 'ok' };
  const step = afterAdvice(placed, state.advice as AdviceSheet, state.fit as AdviceFit);
  return { state: { ...step.state, feedback: { ...step.state.feedback, detail: `收货暂存位 ${code}。${step.state.feedback.detail}` } }, effects: [{ kind: 'remember-location', code }] };
}

export function chooseStockState(state: ReceiveState, stockState: string): ReceiveStep {
  if (state.phase !== 'state' && state.phase !== 'goods') return hold(state);
  if (rememberedStockState(stockState) === null) return hold(state);
  return hold({
    ...state,
    phase: 'goods',
    stockState,
    cue: null,
    feedback: { tone: 'waiting', headline: '接着扫货', detail: '这一批按你选的状态收。' },
  });
}

export interface IdentifiedGoods {
  kind: 'found' | 'unknown' | 'failed';
  goodsCode?: string;
  unit?: string;
  trackBatch?: boolean;
  trackExpiry?: boolean;
  reason?: string;
}

export function goodsIdentified(state: ReceiveState, code: string, identified: IdentifiedGoods): ReceiveStep {
  if (state.phase !== 'resolving' || state.advice === null || state.advice.inboundMode !== 'BY_GOODS') return hold(state);
  if (identified.kind === 'failed') {
    return hold({
      ...state,
      phase: 'goods',
      feedback: { tone: 'unsent', headline: '这一次没认成', detail: identified.reason ?? '读商品资料没成' },
      cue: 'wrong',
    });
  }
  const onAdvice = state.advice.goods.find((line) => line.goodsCode === identified.goodsCode);
  if (identified.kind === 'unknown' || onAdvice === undefined) {
    const known = identified.kind === 'found';
    return hold({
      ...state,
      phase: 'goods',
      stray: code,
      feedback: {
        tone: 'wrong',
        headline: known ? '这张预报里没有这个货' : '认不出这个码',
        detail: known
          ? `${identified.goodsCode} 不在预报上。照收会入账并记成错货差异；也可以登记成待认领。`
          : `${code} 没有记数。照收会记成收货差异；也可以登记成待认领。`,
      },
      cue: 'wrong',
    });
  }
  const batch = onAdvice.batchNumber ?? '';
  const expiry = onAdvice.expiryDate ?? '';
  const needBatch = identified.trackBatch === true && batch === '';
  const needExpiry = identified.trackExpiry === true && expiry === '';
  if (needBatch || needExpiry) {
    const ask: Ask = {
      observed: code,
      goodsCode: onAdvice.goodsCode,
      unit: onAdvice.unit,
      needUnit: false,
      needBatch,
      needExpiry,
      batchNumber: batch,
      expiryDate: expiry,
    };
    return hold({ ...state, phase: 'asking', ask, feedback: { tone: 'waiting', headline: '这件货要补批号或效期', detail: onAdvice.goodsCode }, cue: null });
  }
  return counted(state, { observed: code, goodsCode: onAdvice.goodsCode, unit: onAdvice.unit, quantity: 1, batchNumber: batch || undefined, expiryDate: expiry || undefined });
}

function counted(state: ReceiveState, line: GoodsDraft): ReceiveStep {
  const goods = addGoods(state.goods, line);
  return hold({
    ...state,
    phase: 'goods',
    goods,
    ask: null,
    stray: null,
    feedback: { tone: 'ok', headline: `${line.observed} 记下了`, detail: `这一次 ${line.quantity} 件。` },
    cue: 'ok',
  });
}

export function confirmAsk(state: ReceiveState, batchNumber: string, expiryDate: string, unit: string): ReceiveStep {
  if (state.phase !== 'asking' || state.ask === null) return hold(state);
  const ask = state.ask;
  const nextUnit = ask.needUnit ? unit.trim() : ask.unit;
  const nextBatch = (ask.needBatch ? batchNumber : ask.batchNumber).trim();
  const nextExpiry = (ask.needExpiry ? expiryDate : ask.expiryDate).trim();
  if (ask.needUnit && nextUnit === '') {
    return say(state, { tone: 'wrong', headline: '要填计量单位', detail: '认不出的货没有包装层级可取，单位得你写。' }, 'wrong');
  }
  if (ask.needBatch && nextBatch === '') {
    return say(state, { tone: 'wrong', headline: '这件货要批号', detail: '商品跟踪批号，不填这一笔提交会被退回。' }, 'wrong');
  }
  if (ask.needExpiry && nextExpiry === '') {
    return say(state, { tone: 'wrong', headline: '这件货要失效日', detail: '商品跟踪效期，不填这一笔提交会被退回。按日历日填。' }, 'wrong');
  }
  return counted(state, {
    observed: ask.observed,
    goodsCode: ask.goodsCode,
    unit: nextUnit,
    quantity: 1,
    batchNumber: nextBatch || undefined,
    expiryDate: nextExpiry || undefined,
  });
}

export function cancelAsk(state: ReceiveState): ReceiveStep {
  if (state.phase !== 'asking') return hold(state);
  return hold({ ...state, phase: 'goods', ask: null, feedback: { tone: 'waiting', headline: '接着扫货', detail: '刚才那一件没记。' }, cue: null });
}

/** 预报里没有的、或认不出的，人明确点了照收。认不出的还要单位，进追问。 */
export function acceptStray(state: ReceiveState, identified: IdentifiedGoods): ReceiveStep {
  if (state.phase !== 'goods' || state.stray === null || state.advice === null) return hold(state);
  const code = state.stray;
  if (identified.kind === 'found' && identified.goodsCode && identified.unit) {
    const needBatch = identified.trackBatch === true;
    const needExpiry = identified.trackExpiry === true;
    if (needBatch || needExpiry) {
      return hold({
        ...state,
        phase: 'asking',
        ask: {
          observed: code,
          goodsCode: identified.goodsCode,
          unit: identified.unit,
          needUnit: false,
          needBatch,
          needExpiry,
          batchNumber: '',
          expiryDate: '',
        },
      });
    }
    return counted(state, { observed: code, goodsCode: identified.goodsCode, unit: identified.unit, quantity: 1 });
  }
  return hold({
    ...state,
    phase: 'asking',
    ask: { observed: code, goodsCode: null, unit: '', needUnit: true, needBatch: false, needExpiry: false, batchNumber: '', expiryDate: '' },
    feedback: { tone: 'waiting', headline: '照收要一个计量单位', detail: code },
  });
}

export function registerStray(state: ReceiveState): ReceiveStep {
  if (state.phase !== 'goods' || state.stray === null || state.advice === null || state.location === null) return hold(state);
  return {
    state: { ...state, phase: 'submitting', cue: null, feedback: { tone: 'waiting', headline: '正在登记待认领', detail: state.stray } },
    effects: [{ kind: 'submit', spec: 'unclaimed-arrival', values: unclaimedFactValues(state.advice, state.stray, state.location) }],
  };
}

export function toggleDamage(state: ReceiveState): ReceiveStep {
  if (state.phase !== 'goods' || state.advice?.inboundMode !== 'BY_CARTON') return hold(state);
  const damageNext = !state.damageNext;
  return hold({
    ...state,
    damageNext,
    feedback: { tone: 'waiting', headline: damageNext ? '下一只箱记成外箱破损' : '接着扫箱号', detail: damageNext ? '扫到的下一只箱带上破损。' : '不再把下一只箱记成破损。' },
  });
}

export function submitReceipt(state: ReceiveState): ReceiveStep {
  if (state.phase !== 'goods' || state.advice === null || state.location === null) return hold(state);
  if (state.advice.inboundMode === 'BY_GOODS') {
    if (state.stockState === null || state.goods.length === 0) {
      return say(state, { tone: 'wrong', headline: '这一次还没有货', detail: '先扫到至少一件再提交。' }, 'wrong');
    }
    return {
      state: { ...state, phase: 'submitting', cue: null, feedback: { tone: 'waiting', headline: '正在提交这一次收货', detail: '' } },
      effects: [{ kind: 'submit', spec: 'receipt-goods', values: goodsFactValues(state.advice, state.location, state.stockState, state.goods) }],
    };
  }
  if (state.cartons.length === 0) {
    return say(state, { tone: 'wrong', headline: '这一次还没有箱', detail: '先扫到至少一只箱再提交。' }, 'wrong');
  }
  return {
    state: { ...state, phase: 'submitting', cue: null, feedback: { tone: 'waiting', headline: '正在提交这一次收货', detail: '' } },
    effects: [{ kind: 'submit', spec: 'receipt-carton', values: cartonFactValues(state.advice, state.location, state.cartons) }],
  };
}

export function receiptVerdict(state: ReceiveState, verdict: WorkFactVerdict, spec: ReceiveSpecId): ReceiveStep {
  if (state.phase !== 'submitting' && state.phase !== 'unsent') return hold(state);
  if (spec === 'unclaimed-arrival') {
    if (verdict.verdict === 'done') {
      return hold({ ...state, phase: 'goods', stray: null, feedback: { tone: 'ok', headline: '已登记成待认领', detail: verdict.headline }, cue: 'ok' });
    }
    if (verdict.verdict === 'retry') {
      return hold({ ...state, phase: 'unsent', feedback: { tone: 'unsent', headline: '待认领没送到', detail: '再送一次用的是同一个作业事实号。' }, cue: 'wrong' });
    }
    return hold({ ...state, phase: 'goods', feedback: { tone: 'wrong', headline: verdict.headline, detail: '这一笔没有记下。' }, cue: 'wrong' });
  }
  if (verdict.verdict === 'done') {
    const diverted = verdict.outcome === 'DIVERTED_TO_UNCLAIMED';
    return hold({
      ...state,
      phase: 'waiting',
      advice: null,
      fit: null,
      goods: [],
      cartons: [],
      stray: null,
      feedback: {
        tone: diverted ? 'unsent' : 'ok',
        headline: diverted ? '没有入账，转成了待认领' : '这一次收货已入账',
        detail: verdict.headline,
      },
      cue: diverted ? 'wrong' : 'ok',
    });
  }
  if (verdict.verdict === 'retry') {
    return hold({ ...state, phase: 'unsent', feedback: { tone: 'unsent', headline: '这一次没送到', detail: '再送一次用的是同一个作业事实号，不会记两笔。' }, cue: 'wrong' });
  }
  return hold({ ...state, phase: 'refused', feedback: { tone: 'wrong', headline: verdict.headline, detail: '这一笔没有记下。' }, cue: 'wrong' });
}

export function resend(state: ReceiveState): ReceiveStep {
  if (state.phase !== 'unsent') return hold(state);
  return {
    state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: '正在再送一次', detail: '用的是同一个作业事实号。' }, cue: null },
    effects: [{ kind: 'resend' }],
  };
}

export function leaveAdvice(state: ReceiveState): ReceiveStep {
  if (state.phase === 'submitting' || state.phase === 'finding' || state.phase === 'resolving' || state.phase === 'verifying') return hold(state);
  return hold({
    ...startReceive(state.location, null).state,
    stockState: state.stockState,
    feedback: waitingFeedback(state.goods.length > 0 || state.cartons.length > 0 ? '刚才没提交的没有记下。' : '等你扫码。'),
  });
}

export function receivesScan(phase: ReceivePhase): boolean {
  return !busy.has(phase) && phase !== 'asking';
}
