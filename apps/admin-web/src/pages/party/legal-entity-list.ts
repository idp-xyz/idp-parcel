// 集团与法人页的筛选、排序与计数纯逻辑（票 admin-web-group-legal-entities/01 立，票 15 改成工作台口径）。全部是纯函数，
// node:test 钉着；页面只负责摆。
//
// 几件事都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个法人的最新修订、上限 isolatedReadLimit 一页，这里的排序与计数是对这一页，
// 不是对册；分页下推之前这条限制如实存在，页面不假装成全量。

import type { WorkbenchSort } from '../../templates/workbench';
import type { GroupLegalEntityRecord } from './api';
import { identityStatusLabels, type IdentityStatusCode } from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type LegalEntityStatusFilter = CodeFilter<IdentityStatusCode>;

export interface LegalEntityFilter {
  search: string;
  status: LegalEntityStatusFilter;
  /** 只看待补的行（legalEntityNeedsAttention）。 */
  attentionOnly: boolean;
}

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const legalEntityStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 表头可排的四列。 */
export type LegalEntitySortKey = 'legal-entity' | 'party-name' | 'effective-from' | 'registered-at';

/** 默认登记时间新→旧（票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultLegalEntitySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/**
 * 待补：最新修订登记于身份层落地之前（注册国家与终身注册号两格没有），或参与方册查无这个身份（名称转写不到）。
 * 两件都要有人去处理，是这张册上唯一「需要人去做点什么」的形态，但处置不同：前者登记下一修订补齐；后者是写入门
 * 失败留下的悬空（api.ts GroupLegalEntityRecord 头注），法人钉着的参与方身份改不了，要去查参与方册。停用是终局，不算待补。
 */
export function legalEntityNeedsAttention(row: GroupLegalEntityRecord): boolean {
  return !row.identityLayerRegistered || !row.partyNameKnown;
}

/** 搜索是包含匹配、不分大小写，命中法人标识 / 参与方身份 / 名称 / 状态原名任一格。 */
export function filterLegalEntities(
  rows: readonly GroupLegalEntityRecord[],
  filter: LegalEntityFilter,
): GroupLegalEntityRecord[] {
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    if (filter.attentionOnly && !legalEntityNeedsAttention(row)) return false;
    return matchesSearch(filter.search, [row.legalEntityId, row.partyId, row.partyName, row.status]);
  });
}

export interface LegalEntityCounts {
  total: number;
  byStatus: Record<IdentityStatusCode, number>;
  attention: number;
}

/** 命令头指标与状态胶囊的计数，数的是已取回的这一页。 */
export function countLegalEntities(rows: readonly GroupLegalEntityRecord[]): LegalEntityCounts {
  const byStatus: Record<IdentityStatusCode, number> = { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 };
  let attention = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as IdentityStatusCode] += 1;
    if (legalEntityNeedsAttention(row)) attention += 1;
  }
  return { total: rows.length, byStatus, attention };
}

/**
 * 在用法人里已生效的占比（整数百分比）。分母只数在用的两格（已登记、已生效）：已停用的永远不会再生效，算进分母
 * 这条就永远到不了 100%。没有在用法人时答 null，页面显「—」而不是给空集编一个 0%。
 */
export function effectiveShareOfInUse(counts: LegalEntityCounts): number | null {
  const inUse = counts.byStatus.REGISTERED + counts.byStatus.EFFECTIVE;
  if (inUse === 0) return null;
  return Math.round((counts.byStatus.EFFECTIVE / inUse) * 100);
}

/**
 * 工具条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报。四态里的空态说的是「登记册为空」，
 * 筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，后者改条件）——所以筛空时这里照显
 * 「共 N 个，当前显示 0 个」，表格区另显 legalEntityNoMatchNote，不把页面切成空态。
 */
export function legalEntityCountSummary(total: number, visible: number): string {
  return `共 ${total} 个责任法人，当前显示 ${visible} 个`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const legalEntityNoMatchNote = '当前筛选条件下没有匹配的法人';

/** 按表头排序交回新数组，不改输入；同值保持原序。认不得的键原样交回。 */
export function sortLegalEntities(
  rows: readonly GroupLegalEntityRecord[],
  sort: WorkbenchSort,
): GroupLegalEntityRecord[] {
  const compare = comparatorOf(sort.key as LegalEntitySortKey);
  if (compare === null) return [...rows];
  return [...rows].sort((left, right) => sort.dir * compare(left, right));
}

function comparatorOf(
  key: LegalEntitySortKey,
): ((left: GroupLegalEntityRecord, right: GroupLegalEntityRecord) => number) | null {
  switch (key) {
    case 'legal-entity':
      return (left, right) => byString(left.legalEntityId, right.legalEntityId);
    case 'party-name':
      return (left, right) => byString(left.partyName ?? '', right.partyName ?? '');
    case 'effective-from':
      return (left, right) => byInstant(left.effectiveFrom, right.effectiveFrom);
    case 'registered-at':
      return (left, right) => byInstant(left.registeredAt, right.registeredAt);
    default:
      return null;
  }
}
