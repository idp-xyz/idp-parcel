// 业务参与方页身份本体册的筛选、排序与计数纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成
// 工作台口径，与 legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 几件事都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个参与方的最新修订、上限 isolatedReadLimit 一页，这里的排序与计数是对这一页，
// 不是对册；分页下推之前这条限制如实存在，页面不假装成全量。

import type { WorkbenchSort } from '../../templates/workbench';
import type { BusinessPartyRecord } from './api';
import { identityStatusLabels, type IdentityStatusCode } from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

export interface BusinessPartyFilter {
  search: string;
  status: BusinessPartyStatusFilter;
}

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const businessPartyStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 表头可排的四列。 */
export type BusinessPartySortKey = 'party' | 'party-name' | 'effective-from' | 'registered-at';

/** 默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultBusinessPartySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/** 搜索是包含匹配、不分大小写，命中参与方标识 / 名称 / 状态原名任一格。 */
export function filterBusinessParties(
  rows: readonly BusinessPartyRecord[],
  filter: BusinessPartyFilter,
): BusinessPartyRecord[] {
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    return matchesSearch(filter.search, [row.partyId, row.partyName, row.status]);
  });
}

export interface BusinessPartyCounts {
  total: number;
  byStatus: Record<IdentityStatusCode, number>;
}

/**
 * 命令头指标与状态胶囊的计数，数的是已取回的这一页。本册没有「待补」一格：参与方册是名称的来源，没有转写不到的格；
 * 身份层两格是法人册的事。所以这里只按状态分格。
 */
export function countBusinessParties(rows: readonly BusinessPartyRecord[]): BusinessPartyCounts {
  const byStatus: Record<IdentityStatusCode, number> = { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 };
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as IdentityStatusCode] += 1;
  }
  return { total: rows.length, byStatus };
}

/**
 * 工具条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报。页面态里的空态说的是「登记册为空」，
 * 筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，后者改条件）——所以筛空时这里照显
 * 「共 N 个，当前显示 0 个」，表格区另显 businessPartyNoMatchNote，不把页面切成空态。
 */
export function businessPartyCountSummary(total: number, visible: number): string {
  return `共 ${total} 个参与方身份，当前显示 ${visible} 个`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const businessPartyNoMatchNote = '当前筛选条件下没有匹配的参与方身份';

/** 按表头排序交回新数组，不改输入；同值保持原序。认不得的键原样交回。 */
export function sortBusinessParties(rows: readonly BusinessPartyRecord[], sort: WorkbenchSort): BusinessPartyRecord[] {
  const compare = comparatorOf(sort.key as BusinessPartySortKey);
  if (compare === null) return [...rows];
  return [...rows].sort((left, right) => sort.dir * compare(left, right));
}

function comparatorOf(key: BusinessPartySortKey): ((left: BusinessPartyRecord, right: BusinessPartyRecord) => number) | null {
  switch (key) {
    case 'party':
      return (left, right) => byString(left.partyId, right.partyId);
    case 'party-name':
      return (left, right) => byString(left.partyName, right.partyName);
    case 'effective-from':
      return (left, right) => byInstant(left.effectiveFrom, right.effectiveFrom);
    case 'registered-at':
      return (left, right) => byInstant(left.registeredAt, right.registeredAt);
    default:
      return null;
  }
}
