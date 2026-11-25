// 盘点：生命周期在进度条上的落点、列表上的下一步、库位清点进度，以及账面与清点的比对（纯 TypeScript，node:test 直接测）。
//
// 一份盘点创建后停在待开始；开始后盘点范围进入作业锁定，到作业台按库位录入清点数。每个库位都有清点录入才能结束，结束时按每个库位
// 最后一次清点数与账面比对，不一致的各成一条差异，等批准入账或驳回。已盘点的不能再作废，作废只发生在待开始或盘点中。

import type { DocumentTrack, TrackStep } from './document-track';

// 待开始在这一页叫「待开始」，不用词表里通用的「待处理」：同一个状态码在调整与作业差异那边是「待批」，各说各的。
export const countStatusLabels: Readonly<Record<string, string>> = {
  PENDING: '待开始',
  COUNTING: '盘点中',
  COUNTED: '已盘点',
  VOID: '已作废',
};

export function countStatusLabel(status: string): string {
  return countStatusLabels[status] ?? status;
}

export const countSteps: readonly TrackStep[] = [
  { id: 'PENDING', label: '待开始' },
  { id: 'COUNTING', label: '盘点中' },
  { id: 'COUNTED', label: '已盘点' },
];

/**
 * 待开始卡在等人动手，用待办色标出；盘点中是正常往前走，不标。已盘点而差异还没批完的，卡在批准这一步，同样用待办色并写明还剩几条。
 * 作废是人决定停下，不是失败，用中性的停止标记，步子都不算走过。读口没答出的陌生状态词原样示在第一步上，不替它猜走到了哪。
 */
export function countTrack(status: string, pendingDifferences: number): DocumentTrack {
  switch (status) {
    case 'PENDING':
      return { steps: countSteps, currentId: 'PENDING', currentLabel: '待开始' };
    case 'COUNTING':
      return { steps: countSteps, currentId: 'COUNTING' };
    case 'COUNTED':
      return pendingDifferences > 0
        ? { steps: countSteps, currentId: 'COUNTED', currentLabel: `已盘点，${pendingDifferences} 条差异待批` }
        : { steps: countSteps, currentId: 'COUNTED' };
    case 'VOID':
      return { steps: countSteps, currentId: 'PENDING', halt: { label: '已作废', tone: 'neutral' } };
    default:
      return { steps: countSteps, currentId: 'PENDING', currentLabel: status };
  }
}

export interface CountProgress {
  total: number;
  counted: number;
  /** 还没有清点录入的库位，按盘点范围里的次序。 */
  missing: string[];
}

/** 盘点范围里有几个库位已经有清点录入、哪几个还没有。结束盘点要求每个库位都有，缺一个就被拒。 */
export function countProgress(locations: readonly string[], entryLocations: readonly string[]): CountProgress {
  const range = [...new Set(locations)];
  const seen = new Set(entryLocations);
  const missing = range.filter((location) => !seen.has(location));
  return { total: range.length, counted: range.length - missing.length, missing };
}

export type CountNextStep =
  /** 在盘点单上就地开始。 */
  | { kind: 'start'; label: string }
  /** 要去作业台录入清点数。 */
  | { kind: 'go'; label: string; moduleId: string }
  /** 在盘点单上就地结束并出差异。 */
  | { kind: 'complete'; label: string }
  /** 差异等人批：批与驳回都落在某一条差异上，要先打开盘点单看。 */
  | { kind: 'review'; label: string };

interface CountFacts {
  status: string;
  locations: readonly string[];
  entryCount: number;
  pendingDifferences: number;
}

/**
 * 列表每行最右那一步。盘点中分两种：一笔清点录入只管一个库位，笔数还不到库位数，必有库位没录，此刻结束会被拒，下一步是去作业台；
 * 笔数够了也不保证每个库位都录过（同一库位可以重录），列表上看不出，仍给结束，盘点单上才有逐库位的进度。
 * 已作废与差异都批完的没有下一步。
 */
export function countNextStep(count: CountFacts): CountNextStep | null {
  switch (count.status) {
    case 'PENDING':
      return { kind: 'start', label: '开始盘点' };
    case 'COUNTING':
      return count.entryCount < count.locations.length
        ? { kind: 'go', label: '去作业台录数', moduleId: 'warehouse-workstation' }
        : { kind: 'complete', label: '结束并出差异' };
    case 'COUNTED':
      return count.pendingDifferences > 0 ? { kind: 'review', label: `审批 ${count.pendingDifferences} 条差异` } : null;
    default:
      return null;
  }
}

/**
 * 这份盘点停在当前这一步多久了，从哪一刻算：待开始从创建算，盘点中从开始算，已盘点而差异没批完的从结束算。别的不在等谁，答 undefined。
 * 列表读口可排的是创建时刻，所以排序与这里的起点在已开始的盘点上会差开，差得不大。
 */
export function countWaitingSince(count: {
  status: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  pendingDifferences: number;
}): string | undefined {
  switch (count.status) {
    case 'PENDING':
      return count.createdAt;
    case 'COUNTING':
      return count.startedAt || count.createdAt;
    case 'COUNTED':
      return count.pendingDifferences > 0 ? count.completedAt || undefined : undefined;
    default:
      return undefined;
  }
}

/** 一长串编码在单格里写前几个，其余写个数：「L-01、L-02、L-03 等 12 个」。 */
export function codesPreview(values: readonly string[], limit: number): string {
  if (values.length <= limit) return values.join('、');
  return `${values.slice(0, limit).join('、')} 等 ${values.length} 个`;
}

/** 批号、效期、贴标别名这三样只有少数行才有：有哪样写哪样，都没有答空串。 */
export function lotLabel(row: { batchNumber?: string; expiryDate?: string; labelAlias?: string }): string {
  return [
    row.batchNumber ? `批号 ${row.batchNumber}` : '',
    row.expiryDate ? `效期 ${row.expiryDate}` : '',
    row.labelAlias ? `贴标 ${row.labelAlias}` : '',
  ]
    .filter((part) => part !== '')
    .join(' · ');
}

// —— 账面与清点的比对 ——

export interface DifferenceLike {
  bookQuantity: number;
  countedQuantity: number;
}

export interface DecidableDifference extends DifferenceLike {
  locationCode: string;
  goodsCode: string;
  unit: string;
  status: string;
}

export type DifferenceDirection = 'gain' | 'loss';

export const differenceDirectionLabels: Readonly<Record<DifferenceDirection, string>> = { gain: '盘盈', loss: '盘亏' };

/** 清点数减账面数：正的是账上少记了，负的是账上多记了。 */
export function differenceGap(row: DifferenceLike): number {
  return row.countedQuantity - row.bookQuantity;
}

/** 清点多于账面是盘盈，少于账面是盘亏；两边一致不是差异，答 null。 */
export function differenceDirection(row: DifferenceLike): DifferenceDirection | null {
  const gap = differenceGap(row);
  return gap > 0 ? 'gain' : gap < 0 ? 'loss' : null;
}

/** 带正负号写差多少：+3、-2。 */
export function signedGap(gap: number): string {
  return gap > 0 ? `+${gap}` : String(gap);
}

/**
 * 只留账面与清点不一致的行，一致的不混进差异里。待批的排最前，那是要人动手的；其余按库位、商品。
 * 后端结束盘点时本来就只为不一致的格子生成差异，这里再筛一道，页面不靠后端不出错。
 */
export function mismatchedDifferences<T extends DecidableDifference>(rows: readonly T[]): T[] {
  return rows
    .filter((row) => differenceDirection(row) !== null)
    .sort(
      (a, b) =>
        Number(b.status === 'PENDING') - Number(a.status === 'PENDING') ||
        a.locationCode.localeCompare(b.locationCode) ||
        a.goodsCode.localeCompare(b.goodsCode),
    );
}

export interface DifferenceTotals {
  /** 不一致的行数，一致的行不计。 */
  rows: number;
  gainRows: number;
  lossRows: number;
  pendingRows: number;
  /** 按单位合计，loss 记成正数。件与箱加在一起没有意义，所以不跨单位合计。 */
  byUnit: ReadonlyArray<{ unit: string; gain: number; loss: number }>;
}

export function differenceTotals(rows: readonly DecidableDifference[]): DifferenceTotals {
  const byUnit = new Map<string, { unit: string; gain: number; loss: number }>();
  let gainRows = 0;
  let lossRows = 0;
  let pendingRows = 0;
  for (const row of rows) {
    const gap = differenceGap(row);
    if (gap === 0) continue;
    const total = byUnit.get(row.unit) ?? { unit: row.unit, gain: 0, loss: 0 };
    if (gap > 0) {
      gainRows += 1;
      total.gain += gap;
    } else {
      lossRows += 1;
      total.loss += -gap;
    }
    byUnit.set(row.unit, total);
    if (row.status === 'PENDING') pendingRows += 1;
  }
  return { rows: gainRows + lossRows, gainRows, lossRows, pendingRows, byUnit: [...byUnit.values()] };
}

/** 「+7 件、+2 箱」；这一栏在任何单位上都没有数时答空串。 */
export function unitQuantities(byUnit: DifferenceTotals['byUnit'], field: 'gain' | 'loss'): string {
  return byUnit
    .filter((total) => total[field] > 0)
    .map((total) => `${field === 'gain' ? '+' : '-'}${total[field]} ${total.unit}`)
    .join('、');
}

/** 「盘盈 2 行（+7 件） · 盘亏 1 行（-3 件）」；没有差异答空串。 */
export function differenceSummary(totals: DifferenceTotals): string {
  const parts: string[] = [];
  if (totals.gainRows > 0) parts.push(`盘盈 ${totals.gainRows} 行（${unitQuantities(totals.byUnit, 'gain')}）`);
  if (totals.lossRows > 0) parts.push(`盘亏 ${totals.lossRows} 行（${unitQuantities(totals.byUnit, 'loss')}）`);
  return parts.join(' · ');
}

// —— 清点录入 ——

export interface CountEntryLike {
  factId: string;
  locationCode: string;
  occurredAt: string;
  deviceSequence: number;
  operatorId: string;
  lines: ReadonlyArray<{
    ownerAccount?: string;
    goodsCode?: string;
    unit?: string;
    stockState?: string;
    labelAlias?: string;
    batchNumber?: string;
    expiryDate?: string;
    quantity: number;
    observed?: string;
  }>;
}

function instantMs(value: string): number {
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? Number.NEGATIVE_INFINITY : ms;
}

function supersedes(candidate: CountEntryLike, held: CountEntryLike): boolean {
  const a = instantMs(candidate.occurredAt);
  const b = instantMs(held.occurredAt);
  return a > b || (a === b && candidate.deviceSequence > held.deviceSequence);
}

/**
 * 每个库位以最后一次清点为准，返回算数的那几笔录入的事实标识。取法与后端结束盘点时同一把尺：发生时刻晚的胜出，
 * 同一时刻按设备序号大的。较早的那几笔仍留着，只是不再进比对。
 */
export function effectiveEntryIds(entries: readonly CountEntryLike[]): Set<string> {
  const latest = new Map<string, CountEntryLike>();
  for (const entry of entries) {
    const held = latest.get(entry.locationCode);
    if (held === undefined || supersedes(entry, held)) latest.set(entry.locationCode, entry);
  }
  return new Set([...latest.values()].map((entry) => entry.factId));
}

export interface CountEntryRow {
  key: string;
  factId: string;
  locationCode: string;
  occurredAt: string;
  operatorId: string;
  ownerAccount?: string;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  labelAlias?: string;
  batchNumber?: string;
  expiryDate?: string;
  quantity: number;
  /** 现场看到、认不出是哪种货的那句描述；它不进账实比对。 */
  observed?: string;
  /** 这一笔录入里一行都没有：清点的结果是这个库位上没有货。 */
  empty: boolean;
  /** 同一库位后来又录过，这一笔不再算数。 */
  superseded: boolean;
}

/** 把清点录入摊成一行一格货，同一库位的录入挨在一起、先录的在前，这样重录与被它取代的那一笔紧挨着。 */
export function countEntryRows(entries: readonly CountEntryLike[]): CountEntryRow[] {
  const effective = effectiveEntryIds(entries);
  const ordered = [...entries].sort((a, b) => {
    const byLocation = a.locationCode.localeCompare(b.locationCode);
    if (byLocation !== 0) return byLocation;
    const timeA = instantMs(a.occurredAt);
    const timeB = instantMs(b.occurredAt);
    if (timeA !== timeB) return timeA < timeB ? -1 : 1;
    return a.deviceSequence - b.deviceSequence;
  });
  return ordered.flatMap((entry): CountEntryRow[] => {
    const head = {
      factId: entry.factId,
      locationCode: entry.locationCode,
      occurredAt: entry.occurredAt,
      operatorId: entry.operatorId,
      superseded: !effective.has(entry.factId),
    };
    if (entry.lines.length === 0) return [{ ...head, key: `${entry.factId}-0`, quantity: 0, empty: true }];
    return entry.lines.map((line, index) => ({
      ...head,
      key: `${entry.factId}-${index}`,
      ownerAccount: line.ownerAccount || undefined,
      goodsCode: line.goodsCode || undefined,
      unit: line.unit || undefined,
      stockState: line.stockState || undefined,
      labelAlias: line.labelAlias || undefined,
      batchNumber: line.batchNumber || undefined,
      expiryDate: line.expiryDate || undefined,
      quantity: line.quantity,
      observed: line.observed || undefined,
      empty: false,
    }));
  });
}
