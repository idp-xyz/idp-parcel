// 集团与法人页的筛选与排序纯逻辑（票 admin-web-group-legal-entities/01）。全部是纯函数，node:test
// 钉着；页面只负责摆。
//
// 两条都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个法人的最新修订、上限 isolatedReadLimit 一页，这里的排序是对这一页排，
// 不是对册排；分页下推之前这条限制如实存在，页面不假装成全量。

import type { GroupLegalEntityRecord } from './api';

/** 身份状态封闭三格（domain IdentityStatus 原名）加「全部」。 */
export type LegalEntityStatusFilter = 'ALL' | 'REGISTERED' | 'EFFECTIVE' | 'DEACTIVATED';

export interface LegalEntityFilter {
  search: string;
  status: LegalEntityStatusFilter;
}

/**
 * 排序键。默认登记时间新→旧（裁决 2：运营配置员最常看「刚登进去的那条」）；标识按字典序；
 * 生效自早→晚给「哪些还没到生效时点」这种问题用。
 */
export type LegalEntitySortKey = 'registered-desc' | 'id-asc' | 'effective-asc';

export interface SelectOption<Value extends string> {
  value: Value;
  label: string;
}

// 选项表由页面直接渲染，词取 identityStatusLabels 那三格的 CONTEXT 原词，不另抄一份中文。
export const legalEntityStatusFilterOptions: readonly SelectOption<LegalEntityStatusFilter>[] = [
  { value: 'ALL', label: '全部状态' },
  { value: 'REGISTERED', label: '已登记' },
  { value: 'EFFECTIVE', label: '已生效' },
  { value: 'DEACTIVATED', label: '已停用' },
];

export const legalEntitySortOptions: readonly SelectOption<LegalEntitySortKey>[] = [
  { value: 'registered-desc', label: '登记时间 新→旧' },
  { value: 'id-asc', label: '法人标识 A→Z' },
  { value: 'effective-asc', label: '生效自 早→晚' },
];

/** 搜索是包含匹配、不分大小写，命中法人标识 / 参与方身份 / 名称 / 状态原名任一格。 */
export function filterLegalEntities(
  rows: readonly GroupLegalEntityRecord[],
  filter: LegalEntityFilter,
): GroupLegalEntityRecord[] {
  const needle = filter.search.trim().toLowerCase();
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    if (needle === '') return true;
    return [row.legalEntityId, row.partyId, row.partyName ?? '', row.status].some((value) =>
      value.toLowerCase().includes(needle),
    );
  });
}

/**
 * 过滤条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报。四态里的空态说的是「登记册为空」，
 * 筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，后者改条件）——所以筛空时这里照显
 * 「共 N 个，当前显示 0 个」，表格区另显 legalEntityNoMatchNote，不把页面切成空态。
 */
export function legalEntityCountSummary(total: number, visible: number): string {
  return `共 ${total} 个责任法人，当前显示 ${visible} 个`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const legalEntityNoMatchNote = '当前筛选条件下没有匹配的法人';

// 时刻按字符串比：端点给的是 RFC 3339 UTC，同一格式下字典序即时间序，不必解析；解析失败的
// 值会静默排到一头去，而字符串比至少是稳定且可预期的。
const byString = (left: string, right: string) => (left < right ? -1 : left > right ? 1 : 0);

/** 排序交回新数组，不改输入。 */
export function sortLegalEntities(
  rows: readonly GroupLegalEntityRecord[],
  key: LegalEntitySortKey,
): GroupLegalEntityRecord[] {
  const sorted = [...rows];
  switch (key) {
    case 'registered-desc':
      sorted.sort((left, right) => byString(right.registeredAt, left.registeredAt));
      break;
    case 'id-asc':
      sorted.sort((left, right) => byString(left.legalEntityId, right.legalEntityId));
      break;
    case 'effective-asc':
      sorted.sort((left, right) => byString(left.effectiveFrom, right.effectiveFrom));
      break;
  }
  return sorted;
}
