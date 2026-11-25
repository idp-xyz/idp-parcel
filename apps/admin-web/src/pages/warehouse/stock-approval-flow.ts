// 库存调整与库内作业差异：两种都是提交（差异是库内作业形成）之后停在待批，批准才入账、驳回就此结束，没有第三条路
// （纯 TypeScript，node:test 直接测）。

import type { DocumentTrack, TrackStep } from './document-track';

// 待批在这一页叫「待批」，不用词表里通用的「待处理」：同一个状态码在盘点那边是「待开始」，各说各的。
// 批准之后入账，所以终点写「已批准入账」，调整的状态码是 POSTED、差异的是 APPROVED，说的是同一件事。
export const adjustmentStatusLabels: Readonly<Record<string, string>> = {
  PENDING: '待批',
  POSTED: '已批准入账',
  REJECTED: '已驳回',
};

export const discrepancyStatusLabels: Readonly<Record<string, string>> = {
  PENDING: '待批',
  APPROVED: '已批准入账',
  REJECTED: '已驳回',
};

export function adjustmentStatusLabel(status: string): string {
  return adjustmentStatusLabels[status] ?? status;
}

export function discrepancyStatusLabel(status: string): string {
  return discrepancyStatusLabels[status] ?? status;
}

export const adjustmentSteps: readonly TrackStep[] = [
  { id: 'PENDING', label: '待批' },
  { id: 'POSTED', label: '已批准入账' },
];

export const discrepancySteps: readonly TrackStep[] = [
  { id: 'PENDING', label: '待批' },
  { id: 'APPROVED', label: '已批准入账' },
];

/**
 * 待批用待办色标出：它在等人批准或驳回，是这张单上唯一要人动手的一步。驳回是流程停下，用红色的停止标记，步子都不算走过。
 * 读口没答出的陌生状态词原样示在第一步上，不替它猜走到了哪。
 */
function approvalTrack(steps: readonly TrackStep[], settledId: string, status: string): DocumentTrack {
  if (status === 'PENDING') return { steps, currentId: 'PENDING', currentLabel: '待批' };
  if (status === settledId) return { steps, currentId: settledId };
  if (status === 'REJECTED') return { steps, currentId: 'PENDING', halt: { label: '已驳回', tone: 'danger' } };
  return { steps, currentId: 'PENDING', currentLabel: status };
}

export function adjustmentTrack(status: string): DocumentTrack {
  return approvalTrack(adjustmentSteps, 'POSTED', status);
}

export function discrepancyTrack(status: string): DocumentTrack {
  return approvalTrack(discrepancySteps, 'APPROVED', status);
}

// —— 库存调整的行 ——

/** 调增、调减、销毁：读者按这个先后看，不认识的种类排在后面。 */
export const adjustmentKindOrder: readonly string[] = ['INCREASE', 'DECREASE', 'DESTRUCTION'];

/** 调增是加库存，调减与销毁是减库存；不认识的种类不替它定方向。 */
export function adjustmentKindSign(kind: string): '+' | '-' | '' {
  if (kind === 'INCREASE') return '+';
  if (kind === 'DECREASE' || kind === 'DESTRUCTION') return '-';
  return '';
}

export function signedAdjustmentQuantity(line: { kind: string; quantity: number }): string {
  return `${adjustmentKindSign(line.kind)}${line.quantity}`;
}

export interface AdjustmentLineLike {
  kind: string;
  locationCode: string;
  goodsCode: string;
  unit: string;
  quantity: number;
}

export interface AdjustmentKindTotal {
  kind: string;
  rows: number;
  /** 按单位合计：件与箱加在一起没有意义，所以不跨单位合计。 */
  byUnit: ReadonlyArray<{ unit: string; quantity: number }>;
}

export interface AdjustmentTotals {
  lines: number;
  goods: number;
  locations: number;
  /** 只含这张调整单里有的种类。 */
  kinds: readonly AdjustmentKindTotal[];
}

export function adjustmentTotals(lines: readonly AdjustmentLineLike[]): AdjustmentTotals {
  const goods = new Set<string>();
  const locations = new Set<string>();
  const kinds = new Map<string, { kind: string; rows: number; byUnit: Map<string, number> }>();
  for (const line of lines) {
    goods.add(line.goodsCode);
    locations.add(line.locationCode);
    const total = kinds.get(line.kind) ?? { kind: line.kind, rows: 0, byUnit: new Map<string, number>() };
    total.rows += 1;
    total.byUnit.set(line.unit, (total.byUnit.get(line.unit) ?? 0) + line.quantity);
    kinds.set(line.kind, total);
  }
  const rank = (kind: string) => {
    const index = adjustmentKindOrder.indexOf(kind);
    return index === -1 ? adjustmentKindOrder.length : index;
  };
  return {
    lines: lines.length,
    goods: goods.size,
    locations: locations.size,
    // sort 稳定：同样不认识的种类保持它们第一次出现的先后。
    kinds: [...kinds.values()]
      .sort((a, b) => rank(a.kind) - rank(b.kind))
      .map((total) => ({
        kind: total.kind,
        rows: total.rows,
        byUnit: [...total.byUnit].map(([unit, quantity]) => ({ unit, quantity })),
      })),
  };
}

/** 「-5 件、-2 箱」：带方向写这一类调整各单位的合计。 */
export function kindQuantities(total: AdjustmentKindTotal): string {
  const sign = adjustmentKindSign(total.kind);
  return total.byUnit.map((entry) => `${sign}${entry.quantity} ${entry.unit}`).join('、');
}

// —— 库内作业差异 ——

/** 这条差异为什么没入账，列表上一格写完：账面短少、占用、整箱缺失；都没有答空数组。 */
export function discrepancySignals(row: { shortOnBook: number; heldQuantity: number; cartonMissing: boolean }): string[] {
  return [
    row.shortOnBook > 0 ? `账面短少 ${row.shortOnBook}` : '',
    row.heldQuantity > 0 ? `占用 ${row.heldQuantity}` : '',
    row.cartonMissing ? '整箱缺失' : '',
  ].filter((signal) => signal !== '');
}

/**
 * 批准这条差异会在账上做什么，按它来自哪种库内作业说。作业的种类不在差异本身上，要读它来自的那次作业才知道；
 * 读不到（jobKind 为空）就写各种的总说。整箱缺失不论作业种类，批准都是同一个动作，所以先判。
 */
export function discrepancyEffect(jobKind: string | undefined, row: { cartonMissing: boolean }): string {
  if (row.cartonMissing) return '批准后，整箱在账上时从它现在所在的库位移到报告的库位；箱不在账上或被占着时批准不成立。';
  switch (jobKind) {
    case 'INTERNAL_MOVE':
      return '批准后，没入账的数量从来源库位补进目标库位。';
    case 'SHORTAGE':
      return '批准后，在所在库位把短少的数量从账上调减。';
    case 'FORM_CONVERSION':
    case 'CONDITION_CONVERSION':
      return '批准后，按原作业的转出单位或状态取货，补进转入的单位或状态。';
    default:
      return '批准后按这次库内作业的种类补账：上架或移库从来源补进目标库位，短少在所在库位调减，形态与状态转换按原作业补入账，整箱缺失把箱移到报告的库位。';
  }
}

/**
 * 差异是库内作业哪一行形成的。作业的每一行结果记着它形成的差异号，按号认最准；认不到号时退到差异记下的行位置，位置也越界就答 -1，
 * 不猜。
 */
export function originLineIndex(lines: ReadonlyArray<{ discrepancyId?: string }>, discrepancyId: string, linePosition: number): number {
  const byId = lines.findIndex((line) => line.discrepancyId === discrepancyId);
  if (byId !== -1) return byId;
  return Number.isInteger(linePosition) && linePosition >= 0 && linePosition < lines.length ? linePosition : -1;
}
