// 价格表列表的页内状态筛选：纯函数，node:test 直接钉。检索词下推到读口的 q（ADR-0144）；价卡状态是页面拿此刻对适用期与收尾
// 各格现算的（priceCardStatus），不是登记册上的列，读口不能按它筛（ports.PriceCardPages 头注），所以状态胶囊只筛已取回的这一页，
// 页上照实说「只筛本页」。不带计数：一页里数出来的不是各状态的总数。

import { priceCardStatus, type PriceCardStatusInput } from './price-card-content';
import { priceCardStatusLabels, type PriceCardStatusCode } from '../../vocabulary/price-card';

export type PriceCardStatusFilter = 'ALL' | PriceCardStatusCode;

export const priceCardNoMatchNote = '没有符合条件的价卡。';

/** 这一页有行、按状态筛空了：说的是这一页，别的页可能有。 */
export const priceCardStatusNoMatchNote = '这一页没有这个状态的价卡。状态只筛本页，翻到别的页再看。';

/** 状态胶囊的先后：先看还在用的，再看未生效，最后是已收尾的。 */
export const priceCardStatusOrder: readonly PriceCardStatusCode[] = ['IN_FORCE', 'NOT_YET', 'EXPIRED', 'CUT_OFF', 'RETIRED'];

export function priceCardRowKey(row: { planId: string; planVersion: string }): string {
  return `${row.planId}@${row.planVersion}`;
}

export function priceCardsOfStatus<Row extends PriceCardStatusInput>(
  rows: readonly Row[],
  status: PriceCardStatusFilter,
  now: Date,
): Row[] {
  return status === 'ALL' ? [...rows] : rows.filter((row) => priceCardStatus(row, now) === status);
}

/** 胶囊旁那一句：只筛本页；选了状态且拿到答复时再报这一页筛出几张。 */
export function priceCardStatusScopeNote(answered: boolean, pageCount: number, shown: number, status: PriceCardStatusFilter): string {
  if (!answered || status === 'ALL') return '状态只筛本页';
  return `状态只筛本页：本页 ${pageCount} 张里有 ${shown} 张`;
}

export function priceCardStatusFilterOptions(): { value: PriceCardStatusFilter; label: string }[] {
  return [
    { value: 'ALL', label: '全部' },
    ...priceCardStatusOrder.map((code) => ({ value: code as PriceCardStatusFilter, label: priceCardStatusLabels[code].label })),
  ];
}
