// 业务参与方页身份本体册的筛选与排序纯逻辑（票 admin-web-group-legal-entities/09，与票 01 的
// legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 两条都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个参与方的最新修订、上限 isolatedReadLimit 一页，这里的排序是对这一页排，
// 不是对册排；分页下推之前这条限制如实存在，页面不假装成全量。

import type { BusinessPartyRecord } from './api';
import { identityStatusLabels, type IdentityStatusCode } from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter, type SelectOption } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

export interface BusinessPartyFilter {
  search: string;
  status: BusinessPartyStatusFilter;
}

/**
 * 排序键。默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）；
 * 标识按字典序；生效时点早→晚给「哪些还没到生效时点」这种问题用。
 */
export type BusinessPartySortKey = 'registered-desc' | 'id-asc' | 'effective-asc';

// 选项表由页面直接渲染，码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const businessPartyStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部状态');

export const businessPartySortOptions: readonly SelectOption<BusinessPartySortKey>[] = [
  { value: 'registered-desc', label: '登记时间 新→旧' },
  { value: 'id-asc', label: '参与方标识 A→Z' },
  { value: 'effective-asc', label: '生效时点 早→晚' },
];

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

/**
 * 过滤条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报。四态里的空态说的是「登记册为空」，
 * 筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，后者改条件）——所以筛空时这里照显
 * 「共 N 个，当前显示 0 个」，表格区另显 businessPartyNoMatchNote，不把页面切成空态。
 */
export function businessPartyCountSummary(total: number, visible: number): string {
  return `共 ${total} 个参与方身份，当前显示 ${visible} 个`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const businessPartyNoMatchNote = '当前筛选条件下没有匹配的参与方身份';

/** 排序交回新数组，不改输入。 */
export function sortBusinessParties(
  rows: readonly BusinessPartyRecord[],
  key: BusinessPartySortKey,
): BusinessPartyRecord[] {
  const sorted = [...rows];
  switch (key) {
    case 'registered-desc':
      sorted.sort((left, right) => byInstant(right.registeredAt, left.registeredAt));
      break;
    case 'id-asc':
      sorted.sort((left, right) => byString(left.partyId, right.partyId));
      break;
    case 'effective-asc':
      sorted.sort((left, right) => byInstant(left.effectiveFrom, right.effectiveFrom));
      break;
  }
  return sorted;
}
