// 价格表列表的查询条件与页内状态筛选：纯函数，node:test 直接钉。检索词下推到读口的 q（ADR-0144）。状态胶囊的五态是页面拿此刻对
// 适用期与收尾各格现算的（priceCardStatus），读口只放了其中领域状态那一刀——退役与否（ports.PriceCardPages 的 retired）：「已退役」
// 与 retired=true 同说一件事，整胶囊下推；其余四态都在未退役的版本里，先下推 retired=false，再在到手的这一页上按此刻现算的状态筛，
// 页上照实说这四态「只筛本页」。不带计数：一页里数出来的不是各状态的总数。

import type { CatalogueConditions } from '../catalogue-query';
import { priceCardStatus, type PriceCardStatusInput } from './price-card-content';
import { priceCardStatusLabels, type PriceCardStatusCode } from '../../vocabulary/price-card';

export type PriceCardStatusFilter = 'ALL' | PriceCardStatusCode;

export const priceCardNoMatchNote = '没有符合条件的价卡。';

/** 这一页有未退役的行、按页内那四态筛空了：说的是这一页，别的页可能有。 */
export const priceCardStatusNoMatchNote = '这一页没有这个状态的价卡。这个状态只筛本页，翻到别的页再看。';

/**
 * 胶囊下推到读口的那一半：「全部」不带 retired；「已退役」即 retired=true；其余四态是未退役版本此刻的读数，带 retired=false。
 * 退役时刻就是退役登记的时刻（领域 RetireForCoveringSuccessor 头注：不倒填也不预填），所以 priceCardStatus 判出的「已退役」与册上
 * 有没有退役那一行是同一个集合，页上不必再筛。
 */
export function priceCardListConditions(keyword: string, status: PriceCardStatusFilter): CatalogueConditions {
  if (status === 'ALL') return { q: keyword };
  return { q: keyword, filters: { retired: [status === 'RETIRED' ? 'true' : 'false'] } };
}

/** 这一格胶囊还要不要在页上再筛：「全部」与「已退役」读口已筛完，其余四态要。 */
export function priceCardStatusFiltersPage(status: PriceCardStatusFilter): boolean {
  return status !== 'ALL' && status !== 'RETIRED';
}

/** 状态胶囊的先后：先看还在用的，再看未生效，最后是已收尾的。 */
export const priceCardStatusOrder: readonly PriceCardStatusCode[] = ['IN_FORCE', 'NOT_YET', 'EXPIRED', 'CUT_OFF', 'RETIRED'];

export function priceCardRowKey(row: { planId: string; planVersion: string }): string {
  return `${row.planId}@${row.planVersion}`;
}

/** 到手的这一页上再筛：只在页内那四态时筛，「全部」与「已退役」原样交出。 */
export function priceCardsOfStatus<Row extends PriceCardStatusInput>(
  rows: readonly Row[],
  status: PriceCardStatusFilter,
  now: Date,
): Row[] {
  return priceCardStatusFiltersPage(status) ? rows.filter((row) => priceCardStatus(row, now) === status) : [...rows];
}

/**
 * 胶囊旁那一句。选了页内那四态之一且拿到答复时，报这一页未退役的几张里筛出几张；选了「已退役」不说——那一格按的是整本册；
 * 其余时候提醒哪几格只筛本页。
 */
export function priceCardStatusScopeNote(answered: boolean, pageCount: number, shown: number, status: PriceCardStatusFilter): string {
  if (status === 'RETIRED') return '';
  if (!answered || status === 'ALL') return '除已退役外，状态只筛本页';
  return `${priceCardStatusLabels[status].label}只筛本页：本页未退役的 ${pageCount} 张里有 ${shown} 张`;
}

export function priceCardStatusFilterOptions(): { value: PriceCardStatusFilter; label: string }[] {
  return [
    { value: 'ALL', label: '全部' },
    ...priceCardStatusOrder.map((code) => ({ value: code as PriceCardStatusFilter, label: priceCardStatusLabels[code].label })),
  ];
}
