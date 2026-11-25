// 盘点清点：选一张盘点中的单，扫范围内的库位，逐件扫货，一个库位提交一笔。屏上不显示账面数。
//
// 到 Go 端核过（CountHandler.Record）：只收盘点中（COUNTING）且库位在这张单范围内的。同一事实号同样内容重放答已有结果。
// 同一库位再提交一笔，完成盘点时以后发生的那一笔为准。空库位的明细是空数组。认不出的货只带观察和数量，不带商品编码。
// 认得出的货要货主、单位和库存状态；状态只认良品、待检、次品，不预选。

import type { GuidedFormValues } from '../guided-forms';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import type { WorkFactVerdict } from './work-fact-submit';

export interface CountSheet {
  countId: string;
  warehouse: string;
  status: string;
  owner: string | null;
  locations: string[];
  /** 已经有清点记录的库位。再提交一笔会在完成时盖过它。 */
  counted: string[];
}

export interface CountLine {
  observed?: string;
  ownerAccount?: string;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  quantity: number;
}

export type CountPhase = 'waiting' | 'opening' | 'location' | 'goods' | 'submitting' | 'unsent';

export interface CountState {
  phase: CountPhase;
  sheet: CountSheet | null;
  location: string | null;
  stockState: string | null;
  lines: CountLine[];
  doneLocations: number;
  feedback: { tone: 'waiting' | 'ok' | 'wrong' | 'unsent'; headline: string; detail: string };
  scanned: string;
  cue: ScanCue | null;
}

export type CountEffect = { kind: 'open'; countId: string } | { kind: 'identify'; owner: string; code: string } | { kind: 'submit'; values: GuidedFormValues } | { kind: 'resend' };

export interface CountStep {
  state: CountState;
  effects: CountEffect[];
}

function blank(): CountState {
  return {
    phase: 'waiting',
    sheet: null,
    location: null,
    stockState: null,
    lines: [],
    doneLocations: 0,
    feedback: { tone: 'waiting', headline: '选一张盘点中的盘点单', detail: '等你点一张，或扫盘点单号。' },
    scanned: '',
    cue: null,
  };
}

export function startCount(countId: string | null): CountStep {
  if (!countId) return { state: blank(), effects: [] };
  const state = blank();
  state.phase = 'opening';
  state.feedback = { tone: 'waiting', headline: '在读这张盘点单', detail: countId };
  return { state, effects: [{ kind: 'open', countId }] };
}

function hold(state: CountState): CountStep {
  return { state, effects: [] };
}

export function countOpened(state: CountState, sheet: CountSheet | null, warehouse: string, failed?: string): CountStep {
  if (state.phase !== 'opening' && state.phase !== 'waiting') return hold(state);
  if (failed) return hold({ ...state, phase: 'waiting', feedback: { tone: 'unsent', headline: '这一次没读成', detail: failed }, cue: 'wrong' });
  if (!sheet) return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '没有这张盘点单', detail: '' }, cue: 'wrong' });
  if (sheet.warehouse !== warehouse) {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '这张盘点单不在这个仓', detail: `它属于仓 ${sheet.warehouse}。` }, cue: 'wrong' });
  }
  if (sheet.status !== 'COUNTING') {
    return hold({ ...state, phase: 'waiting', feedback: { tone: 'wrong', headline: '这张盘点单现在不能清点', detail: '只有盘点中的单能录入。' }, cue: 'wrong' });
  }
  return hold({
    ...state,
    phase: 'location',
    sheet,
    feedback: { tone: 'waiting', headline: '扫要清点的库位', detail: `范围内 ${sheet.locations.length} 个库位，已有记录 ${sheet.counted.length} 个。` },
    cue: null,
  });
}

export function scanCode(state: CountState, raw: string): CountStep {
  if (state.phase === 'opening' || state.phase === 'submitting') return hold(state);
  const code = normalizeScan(raw);
  if (code === '') return hold(state);
  if (state.phase === 'waiting') {
    return { state: { ...state, phase: 'opening', scanned: code, feedback: { tone: 'waiting', headline: '在读这张盘点单', detail: code }, cue: null }, effects: [{ kind: 'open', countId: code }] };
  }
  if (state.phase === 'location' && state.sheet) {
    if (!state.sheet.locations.includes(code)) {
      return hold({ ...state, scanned: code, feedback: { tone: 'wrong', headline: '这个库位不在这张盘点单里', detail: `${code} 没有记下。` }, cue: 'wrong' });
    }
    const again = state.sheet.counted.includes(code);
    return hold({
      ...state,
      phase: 'goods',
      location: code,
      lines: [],
      scanned: '',
      feedback: { tone: 'waiting', headline: '接着扫这个库位里的货', detail: again ? '这个库位已经点过，再提交一笔会以这一笔为准。' : '点到多少算多少，屏上不显示账面。' },
      cue: again ? 'wrong' : 'ok',
    });
  }
  if (state.phase !== 'goods' || !state.sheet) return hold(state);
  if (!state.stockState) {
    return hold({ ...state, scanned: code, feedback: { tone: 'wrong', headline: '先选这一格的库存状态', detail: '良品、待检、次品里选一个，不替你选。' }, cue: 'wrong' });
  }
  const owner = state.sheet.owner;
  if (!owner) {
    return hold({ ...state, scanned: code, feedback: { tone: 'wrong', headline: '这张单没有货主', detail: '认不出的货请点「认不出的货」。' }, cue: 'wrong' });
  }
  return {
    state: { ...state, scanned: code, feedback: { tone: 'waiting', headline: '在认这个码', detail: code }, cue: null },
    effects: [{ kind: 'identify', owner, code }],
  };
}

export function goodsFound(state: CountState, code: string, found: { goodsCode: string; unit: string } | null): CountStep {
  if (state.phase !== 'goods' || !state.stockState) return hold(state);
  if (!found) {
    return hold({ ...state, feedback: { tone: 'wrong', headline: '认不出这个码', detail: `${code} 没有记数。可以点「认不出的货」。` }, scanned: code, cue: 'wrong' });
  }
  return addLine(state, { ownerAccount: state.sheet?.owner ?? undefined, goodsCode: found.goodsCode, unit: found.unit, stockState: state.stockState, quantity: 1 }, code);
}

function addLine(state: CountState, line: CountLine, scanned: string): CountStep {
  const index = state.lines.findIndex((item) => item.goodsCode === line.goodsCode && item.observed === line.observed && item.stockState === line.stockState && item.unit === line.unit);
  const lines = index < 0 ? [...state.lines, line] : state.lines.map((item, at) => (at === index ? { ...item, quantity: item.quantity + line.quantity } : item));
  const total = lines.reduce((sum, item) => sum + item.quantity, 0);
  return hold({ ...state, lines, scanned, feedback: { tone: 'ok', headline: `${line.goodsCode ?? line.observed} 记下了`, detail: `这个库位已点 ${lines.length} 种，共 ${total} 件。` }, cue: 'ok' });
}

export function chooseStockState(state: CountState, stockState: string): CountStep {
  if (state.phase !== 'goods') return hold(state);
  if (!['GOOD', 'PENDING_INSPECTION', 'DEFECTIVE'].includes(stockState)) return hold(state);
  return hold({ ...state, stockState, feedback: { ...state.feedback, detail: '这一格按你选的状态点。' } });
}

export function markUnknown(state: CountState): CountStep {
  if (state.phase !== 'goods' || state.scanned === '') return hold(state);
  return addLine(state, { observed: state.scanned, quantity: 1 }, state.scanned);
}

export function setQuantity(state: CountState, index: number, quantity: number): CountStep {
  if (state.phase !== 'goods' || !Number.isInteger(quantity) || quantity < 0) return hold(state);
  if (!state.lines[index]) return hold(state);
  return hold({ ...state, lines: state.lines.map((line, at) => (at === index ? { ...line, quantity } : line)) });
}

export function submitLocation(state: CountState, empty = false): CountStep {
  if (state.phase !== 'goods' || !state.sheet || !state.location) return hold(state);
  if (!empty && state.lines.length === 0) {
    return hold({ ...state, feedback: { tone: 'wrong', headline: '这个库位还没有货', detail: '先扫货，或点「这个库位是空的」。' }, cue: 'wrong' });
  }
  const lines = empty ? [] : state.lines;
  return {
    state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: empty ? '正在把这个库位记成空的' : '正在提交这个库位', detail: '' }, cue: null },
    effects: [{ kind: 'submit', values: countValues(state.sheet.countId, state.location, lines) }],
  };
}

export function countValues(countId: string, location: string, lines: CountLine[]): GuidedFormValues {
  return {
    countId,
    locationCode: location,
    lines: lines.map((line) => {
      const row: Record<string, string> = { quantity: String(line.quantity) };
      if (line.observed) row.observed = line.observed;
      else {
        row.ownerAccount = line.ownerAccount ?? '';
        row.goodsCode = line.goodsCode ?? '';
        row.unit = line.unit ?? '';
        row.stockState = line.stockState ?? '';
      }
      return row;
    }),
  };
}

export function countVerdict(state: CountState, verdict: WorkFactVerdict): CountStep {
  if (state.phase !== 'submitting' && state.phase !== 'unsent') return hold(state);
  if (verdict.verdict === 'retry') {
    return hold({ ...state, phase: 'unsent', feedback: { tone: 'unsent', headline: '这一笔没送到', detail: '再送一次用的是同一个作业事实号。' }, cue: 'wrong' });
  }
  if (verdict.verdict !== 'done' || !state.sheet || !state.location) {
    return hold({ ...state, phase: 'goods', feedback: { tone: 'wrong', headline: verdict.headline, detail: '这个库位没有记下。' }, cue: 'wrong' });
  }
  const counted = state.sheet.counted.includes(state.location) ? state.sheet.counted : [...state.sheet.counted, state.location];
  return hold({
    ...state,
    phase: 'location',
    sheet: { ...state.sheet, counted },
    location: null,
    lines: [],
    doneLocations: state.doneLocations + 1,
    feedback: { tone: 'ok', headline: '这个库位已记下', detail: `还差 ${state.sheet.locations.length - counted.length} 个库位。` },
    cue: 'ok',
  });
}

export function resend(state: CountState): CountStep {
  if (state.phase !== 'unsent') return hold(state);
  return { state: { ...state, phase: 'submitting', feedback: { tone: 'waiting', headline: '正在再送一次', detail: '' }, cue: null }, effects: [{ kind: 'resend' }] };
}
