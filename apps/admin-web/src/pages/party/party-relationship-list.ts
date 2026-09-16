// 业务参与方页关系册的筛选与排序纯逻辑（票 admin-web-group-legal-entities/09）。全部是纯函数，node:test 钉着；
// 页面只负责摆。
//
// 两条都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，归票 04）。
// 端点今天答的是每段关系的最新修订、上限 isolatedReadLimit 一页，这里的排序是对这一页排，不是对册排。

import type { PartyRelationshipRecord } from './api';
import {
  partyRoleLabels,
  relationshipStatusLabels,
  type PartyRoleCode,
  type RelationshipStatusCode,
} from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter, type SelectOption } from './list-order';

/**
 * 角色与状态两个筛选各取「全部」或词表里的一个码。码的封闭集由 presentation.ts 的两张词表拥有（CONTEXT 原词在
 * 前端的唯一一处），类型从它们的键派生，这里不抄一份联合。
 */
export type PartyRelationshipRoleFilter = CodeFilter<PartyRoleCode>;
export type PartyRelationshipStatusFilter = CodeFilter<RelationshipStatusCode>;

export interface PartyRelationshipFilter {
  search: string;
  role: PartyRelationshipRoleFilter;
  status: PartyRelationshipStatusFilter;
}

/**
 * 排序键。默认生效起新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」——关系册上
 * 「刚登进去」最贴近的时点是生效起）；标识按字典序。
 */
export type PartyRelationshipSortKey = 'effective-start-desc' | 'id-asc';

// 两张选项表的码与词都从词表派生，页面直接渲染。
export const partyRelationshipRoleFilterOptions = codeFilterOptions(partyRoleLabels, '全部角色');
export const partyRelationshipStatusFilterOptions = codeFilterOptions(relationshipStatusLabels, '全部状态');

export const partyRelationshipSortOptions: readonly SelectOption<PartyRelationshipSortKey>[] = [
  { value: 'effective-start-desc', label: '生效起 新→旧' },
  { value: 'id-asc', label: '关系标识 A→Z' },
];

/**
 * 搜索是包含匹配、不分大小写，命中关系标识 / 持有方标识与名称 / 相对方标识与名称 / 角色 / 状态原名任一格；
 * 角色与状态两个下拉各精确到码，三者叠加。
 */
export function filterPartyRelationships(
  rows: readonly PartyRelationshipRecord[],
  filter: PartyRelationshipFilter,
): PartyRelationshipRecord[] {
  return rows.filter((row) => {
    if (filter.role !== 'ALL' && row.role !== filter.role) return false;
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    return matchesSearch(filter.search, [
      row.relationshipId,
      row.holderId,
      row.holderName,
      row.counterpartyId,
      row.counterpartyName,
      row.role,
      row.status,
    ]);
  });
}

/**
 * 过滤条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报，单位沿用读签原词「段」。筛「候选关系」筛没了时
 * 这里照显「共 N 段，当前显示 0 段」，表格区另显 partyRelationshipNoMatchNote，不把页面切成空态——「0 段」会与
 * 状态区「这不是目录为空」直接矛盾（票 09 完成判据点名的那一格）。
 */
export function partyRelationshipCountSummary(total: number, visible: number): string {
  return `共 ${total} 段参与方关系，当前显示 ${visible} 段`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const partyRelationshipNoMatchNote = '当前筛选条件下没有匹配的参与方关系';

/** 排序交回新数组，不改输入。 */
export function sortPartyRelationships(
  rows: readonly PartyRelationshipRecord[],
  key: PartyRelationshipSortKey,
): PartyRelationshipRecord[] {
  const sorted = [...rows];
  switch (key) {
    case 'effective-start-desc':
      sorted.sort((left, right) => byInstant(right.effectiveStartsAt, left.effectiveStartsAt));
      break;
    case 'id-asc':
      sorted.sort((left, right) => byString(left.relationshipId, right.relationshipId));
      break;
  }
  return sorted;
}
