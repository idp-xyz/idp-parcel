// 业务参与方页关系册的筛选、排序与计数纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成
// 工作台口径）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 几件事都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，归票 04）。
// 端点今天答的是每段关系的最新修订、上限 isolatedReadLimit 一页，这里的排序与计数是对这一页，不是对册。

import type { WorkbenchSort } from '../../templates/workbench';
import type { PartyRelationshipRecord } from './api';
import {
  partyRoleLabels,
  relationshipStatusLabels,
  type PartyRoleCode,
  type RelationshipStatusCode,
} from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter } from './list-order';

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
  /** 只看待查的行（partyRelationshipNeedsAttention）。 */
  attentionOnly: boolean;
}

/** 表头可排的四列。登记时间不上列（默认排序按生效起，不需要它解释「为什么这一条在最上面」），也就不按它排。 */
export type PartyRelationshipSortKey = 'relationship' | 'holder' | 'counterparty' | 'effective-start';

/**
 * 默认生效起新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」——关系册上「刚登进去」最贴近
 * 的时点是生效起）。
 */
export const defaultPartyRelationshipSort: WorkbenchSort = { key: 'effective-start', dir: -1 };

// 两张选项表的码与词都从词表派生，页面直接渲染。角色仍是下拉（六格并排成胶囊太长），状态是胶囊。
export const partyRelationshipRoleFilterOptions = codeFilterOptions(partyRoleLabels, '全部角色');
export const partyRelationshipStatusFilterOptions = codeFilterOptions(relationshipStatusLabels, '全部');

/**
 * 待查：持有方或相对方的名称从参与方册转写不到（partyNameUnknownNote）。那是写入门失败留下的悬空引用，关系钉着的
 * 参与方身份改不了，要去查参与方册——与法人册「参与方悬空」同一判据（legal-entity-list.ts 的 legalEntityNeedsAttention）。
 * 候选关系等批准、到期撤销替代是终局，都是状态自己的事，不算待查。
 */
export function partyRelationshipNeedsAttention(row: PartyRelationshipRecord): boolean {
  return !row.holderNameKnown || !row.counterpartyNameKnown;
}

/**
 * 搜索是包含匹配、不分大小写，命中关系标识 / 持有方标识与名称 / 相对方标识与名称 / 角色 / 状态原名任一格；
 * 角色与状态各精确到码，与只看待查叠加。
 */
export function filterPartyRelationships(
  rows: readonly PartyRelationshipRecord[],
  filter: PartyRelationshipFilter,
): PartyRelationshipRecord[] {
  return rows.filter((row) => {
    if (filter.role !== 'ALL' && row.role !== filter.role) return false;
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    if (filter.attentionOnly && !partyRelationshipNeedsAttention(row)) return false;
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

export interface PartyRelationshipCounts {
  total: number;
  byStatus: Record<RelationshipStatusCode, number>;
  attention: number;
}

/** 命令头指标与状态胶囊的计数，数的是已取回的这一页；词表之外的状态码不进任何格。 */
export function countPartyRelationships(rows: readonly PartyRelationshipRecord[]): PartyRelationshipCounts {
  const byStatus: Record<RelationshipStatusCode, number> = { CANDIDATE: 0, EFFECTIVE: 0, EXPIRED: 0, REVOKED: 0, SUPERSEDED: 0 };
  let attention = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as RelationshipStatusCode] += 1;
    if (partyRelationshipNeedsAttention(row)) attention += 1;
  }
  return { total: rows.length, byStatus, attention };
}

/**
 * 工具条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报，单位沿用读签原词「段」。筛「候选关系」筛没了时
 * 这里照显「共 N 段，当前显示 0 段」，表格区另显 partyRelationshipNoMatchNote，不把页面切成空态——「0 段」会与
 * 状态区「这不是目录为空」直接矛盾（票 09 完成判据点名的那一格）。
 */
export function partyRelationshipCountSummary(total: number, visible: number): string {
  return `共 ${total} 段参与方关系，当前显示 ${visible} 段`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const partyRelationshipNoMatchNote = '当前筛选条件下没有匹配的参与方关系';

/** 按表头排序交回新数组，不改输入；同值保持原序。认不得的键原样交回。 */
export function sortPartyRelationships(
  rows: readonly PartyRelationshipRecord[],
  sort: WorkbenchSort,
): PartyRelationshipRecord[] {
  const compare = comparatorOf(sort.key as PartyRelationshipSortKey);
  if (compare === null) return [...rows];
  return [...rows].sort((left, right) => sort.dir * compare(left, right));
}

function comparatorOf(
  key: PartyRelationshipSortKey,
): ((left: PartyRelationshipRecord, right: PartyRelationshipRecord) => number) | null {
  switch (key) {
    case 'relationship':
      return (left, right) => byString(left.relationshipId, right.relationshipId);
    // 双方按标识排而不按名称：名称可能转写不到，标识永远在；两列的首行显示的也是标识。
    case 'holder':
      return (left, right) => byString(left.holderId, right.holderId);
    case 'counterparty':
      return (left, right) => byString(left.counterpartyId, right.counterpartyId);
    case 'effective-start':
      return (left, right) => byInstant(left.effectiveStartsAt, right.effectiveStartsAt);
    default:
      return null;
  }
}
