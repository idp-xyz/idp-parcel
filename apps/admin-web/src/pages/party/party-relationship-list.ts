// 业务参与方页关系册的列表纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成工作台口径）。
// 全部是纯函数，node:test 钉着；页面只负责摆。
//
// 检索、角色与状态筛选、表头排序下推到目录读口（ADR-0144；读口的可排维与筛选维由后端 PartyRelationshipPages 声明）：
// partyRelationshipListConditions 编条件，页面按游标翻页。读口按关系标识与双方的标识、名称检索；角色词与状态词不在其内，
// 按角色、状态看用各自的筛选。「待查」是由名称转写推出来的，读口筛不了，在到手的那一页上筛：整册一页装得下时
// 它是精确的，否则只筛本页（complete-register.ts）。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
import type { PartyRelationshipRecord } from './api';
import {
  partyRoleLabels,
  relationshipStatusLabels,
  type PartyRoleCode,
  type RelationshipStatusCode,
} from './presentation';
import { catalogueSortOf, codeFilterOptions, type CatalogueSortColumns, type CodeFilter } from './list-order';

/**
 * 角色与状态两个筛选各取「全部」或词表里的一个码。码的封闭集由 presentation.ts 的两张词表拥有（CONTEXT 原词在
 * 前端的唯一一处），类型从它们的键派生，这里不抄一份联合。
 */
export type PartyRelationshipRoleFilter = CodeFilter<PartyRoleCode>;
export type PartyRelationshipStatusFilter = CodeFilter<RelationshipStatusCode>;

/**
 * 表头可排的列 → 读口声明的可排维。持有方与相对方的列不可排：读口不按双方排。登记时间不上列（默认排序按生效起，不需要
 * 它解释「为什么这一条在最上面」），也就不按它排。
 */
export const partyRelationshipSortColumns: CatalogueSortColumns = { 'effective-start': 'effectiveStartsAt' };

/**
 * 默认生效起新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」——关系册上「刚登进去」最贴近
 * 的时点是生效起）。读口的缺省序是登记时间新→旧，与它不同，所以总是写进查询串。
 */
export const defaultPartyRelationshipSort: WorkbenchSort = { key: 'effective-start', dir: -1 };

// 两张选项表的码与词都从词表派生，页面直接渲染。角色仍是下拉（六格并排成胶囊太长），状态是胶囊。
export const partyRelationshipRoleFilterOptions = codeFilterOptions(partyRoleLabels, '全部角色');
export const partyRelationshipStatusFilterOptions = codeFilterOptions(relationshipStatusLabels, '全部');

/** 关系册一问的条件：检索词、角色与状态（「全部」即那一维缺席）与表头排序。 */
export function partyRelationshipListConditions(
  keyword: string,
  role: PartyRelationshipRoleFilter,
  status: PartyRelationshipStatusFilter,
  sort: WorkbenchSort,
): CatalogueConditions {
  return {
    q: keyword,
    filters: { role: role === 'ALL' ? [] : [role], status: status === 'ALL' ? [] : [status] },
    sort: catalogueSortOf(sort, partyRelationshipSortColumns, '-registeredAt'),
  };
}

/**
 * 待查：持有方或相对方的名称从参与方册转写不到（partyNameUnknownNote）。那是写入门失败留下的悬空引用，关系钉着的
 * 参与方身份改不了，要去查参与方册——与法人册「参与方悬空」同一判据（legal-entity-list.ts 的 legalEntityNeedsAttention）。
 * 候选关系等批准、到期撤销替代是终局，都是状态自己的事，不算待查。
 */
export function partyRelationshipNeedsAttention(row: PartyRelationshipRecord): boolean {
  return !row.holderNameKnown || !row.counterpartyNameKnown;
}

export interface PartyRelationshipCounts {
  total: number;
  byStatus: Record<RelationshipStatusCode, number>;
  attention: number;
}

/** 命令头指标与状态胶囊的计数。只拿整册来数（complete-register.ts 的 completeRegisterRows 判过的那一份）。 */
export function countPartyRelationships(rows: readonly PartyRelationshipRecord[]): PartyRelationshipCounts {
  const byStatus: Record<RelationshipStatusCode, number> = { CANDIDATE: 0, EFFECTIVE: 0, EXPIRED: 0, REVOKED: 0, SUPERSEDED: 0 };
  let attention = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as RelationshipStatusCode] += 1;
    if (partyRelationshipNeedsAttention(row)) attention += 1;
  }
  return { total: rows.length, byStatus, attention };
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const partyRelationshipNoMatchNote = '当前筛选条件下没有匹配的合作关系';
