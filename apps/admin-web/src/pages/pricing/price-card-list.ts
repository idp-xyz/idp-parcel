// 价格表列表的筛选与计数：纯函数，node:test 直接钉。筛选与计数只在已取回的这一页上做（读口一页答完，不下推查询参数）。

import { priceCardStatus, type PriceCardStatusInput } from './price-card-content';
import { priceCardStatusLabels, type PriceCardStatusCode } from '../../vocabulary/price-card';

export type PriceCardListRow = PriceCardStatusInput & {
  planId: string;
  planVersion: string;
  scope: string;
};

export type PriceCardStatusFilter = 'ALL' | PriceCardStatusCode;

export const priceCardNoMatchNote = '没有符合条件的价卡。';

/** 状态胶囊的先后：先看还在用的，再看未生效，最后是已收尾的。 */
export const priceCardStatusOrder: readonly PriceCardStatusCode[] = ['IN_FORCE', 'NOT_YET', 'EXPIRED', 'CUT_OFF', 'RETIRED'];

export function priceCardRowKey(row: { planId: string; planVersion: string }): string {
  return `${row.planId}@${row.planVersion}`;
}

export function filterPriceCards<Row extends PriceCardListRow>(
  rows: readonly Row[],
  filter: { search: string; status: PriceCardStatusFilter },
  now: Date,
): Row[] {
  const needle = filter.search.trim().toLowerCase();
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && priceCardStatus(row, now) !== filter.status) return false;
    if (needle === '') return true;
    return [row.planId, row.planVersion, row.scope].some((field) => field.toLowerCase().includes(needle));
  });
}

export interface PriceCardCounts {
  total: number;
  byStatus: Record<PriceCardStatusCode, number>;
}

export function countPriceCards(rows: readonly PriceCardListRow[], now: Date): PriceCardCounts {
  const byStatus: Record<PriceCardStatusCode, number> = { IN_FORCE: 0, NOT_YET: 0, EXPIRED: 0, CUT_OFF: 0, RETIRED: 0 };
  for (const row of rows) byStatus[priceCardStatus(row, now)] += 1;
  return { total: rows.length, byStatus };
}

export function priceCardCountSummary(total: number, shown: number): string {
  return shown === total ? `共 ${total} 张价卡` : `显示 ${shown} / ${total} 张价卡`;
}

export function priceCardStatusFilterOptions(): { value: PriceCardStatusFilter; label: string }[] {
  return [
    { value: 'ALL', label: '全部' },
    ...priceCardStatusOrder.map((code) => ({ value: code as PriceCardStatusFilter, label: priceCardStatusLabels[code].label })),
  ];
}
