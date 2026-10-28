// 业务参与方页身份本体册的筛选、排序与计数纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成
// 工作台口径，与 legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 几件事都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个参与方的最新修订、上限 isolatedReadLimit 一页，这里的排序与计数是对这一页，
// 不是对册；分页下推之前这条限制如实存在，页面不假装成全量。
//
// 「类型」一列不是册上的字段：参与方是客户还是承运商，由它在关系册上持有的角色说（CONTEXT：角色由关系表达，身份本体不带
// 角色）。这里按关系册给每个参与方算出它持有的角色集合，列表与筛选按它称呼；一个角色都没有的叫「往来单位」。

import type { WorkbenchSort } from '../../templates/workbench';
import type { BusinessPartyRecord, PartyRelationshipRecord } from './api';
import {
  identityStatusLabels,
  partyRoleLabels,
  partyTypeFallbackLabel,
  partyTypeLabel,
  type IdentityStatusCode,
  type PartyRoleCode,
} from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

/** 类型筛选：全部、某个角色，或 NONE——一个角色都没有的「往来单位」。 */
export type BusinessPartyTypeFilter = CodeFilter<PartyRoleCode> | 'NONE';

export interface BusinessPartyFilter {
  search: string;
  status: BusinessPartyStatusFilter;
  /** 按持有的角色筛；缺省不筛。 */
  type?: BusinessPartyTypeFilter;
}

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const businessPartyStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 参与方标识 → 它持有的角色（去重、按词表顺序）。 */
export type PartyRoleIndex = ReadonlyMap<string, readonly PartyRoleCode[]>;

export const emptyPartyRoleIndex: PartyRoleIndex = new Map();

// 在用的两格才算「持有」：到期、撤销、替代都是边界之后不再依据这段关系（CONTEXT），一家只剩过期合同的单位今天就不是客户。
const roleBearingStatuses = new Set(['CANDIDATE', 'EFFECTIVE']);

/**
 * 按关系册给每个参与方算它持有的角色。只数持有方那一侧：角色由「哪一方对哪一方持有」表达，相对方是被持有的一方
 * （我们的公司主体、或授权交付时的客户），它不因此得到这个角色。词表之外的角色码不进索引——它没有中文可显，
 * 也不该冒充某个已知类型。关系册没取到传 null，索引为空，所有行按「往来单位」显。
 */
export function indexPartyRoles(relationships: readonly PartyRelationshipRecord[] | null): PartyRoleIndex {
  if (relationships === null) return emptyPartyRoleIndex;
  const index = new Map<string, Set<PartyRoleCode>>();
  for (const row of relationships) {
    if (!roleBearingStatuses.has(row.status)) continue;
    if (!(row.role in partyRoleLabels)) continue;
    const roles = index.get(row.holderId) ?? new Set<PartyRoleCode>();
    roles.add(row.role as PartyRoleCode);
    index.set(row.holderId, roles);
  }
  const order = Object.keys(partyRoleLabels) as PartyRoleCode[];
  return new Map([...index].map(([partyId, roles]) => [partyId, order.filter((role) => roles.has(role))]));
}

export function rolesOfParty(index: PartyRoleIndex, partyId: string): readonly PartyRoleCode[] {
  return index.get(partyId) ?? [];
}

/** 表头可排的四列。 */
export type BusinessPartySortKey = 'party' | 'party-name' | 'effective-from' | 'registered-at';

/** 默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultBusinessPartySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/**
 * 搜索是包含匹配、不分大小写，命中参与方标识 / 名称 / 状态原名与中文 / 类型中文任一格；类型筛选精确到角色码，
 * NONE 只留一个角色都没有的。
 */
export function filterBusinessParties(
  rows: readonly BusinessPartyRecord[],
  filter: BusinessPartyFilter,
  roles: PartyRoleIndex = emptyPartyRoleIndex,
): BusinessPartyRecord[] {
  const type = filter.type ?? 'ALL';
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    const held = rolesOfParty(roles, row.partyId);
    if (type === 'NONE' && held.length > 0) return false;
    if (type !== 'ALL' && type !== 'NONE' && !held.includes(type)) return false;
    return matchesSearch(filter.search, [
      row.partyId,
      row.partyName,
      row.status,
      identityStatusLabels[row.status as IdentityStatusCode] ?? '',
      partyTypeLabel(held),
    ]);
  });
}

export interface BusinessPartyCounts {
  total: number;
  byStatus: Record<IdentityStatusCode, number>;
  /** 持有每个角色的参与方数；一个参与方持有两个角色时两格各记一次。 */
  byRole: Record<PartyRoleCode, number>;
  /** 一个角色都没有的（「往来单位」）。 */
  withoutRole: number;
}

/**
 * 命令头指标与状态胶囊的计数，数的是已取回的这一页。本册没有「待补」一格：参与方册是名称的来源，没有转写不到的格；
 * 身份层两格是法人册的事。所以按状态与角色分格。
 */
export function countBusinessParties(
  rows: readonly BusinessPartyRecord[],
  roles: PartyRoleIndex = emptyPartyRoleIndex,
): BusinessPartyCounts {
  const byStatus: Record<IdentityStatusCode, number> = { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 };
  const byRole = Object.fromEntries(Object.keys(partyRoleLabels).map((role) => [role, 0])) as Record<PartyRoleCode, number>;
  let withoutRole = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as IdentityStatusCode] += 1;
    const held = rolesOfParty(roles, row.partyId);
    if (held.length === 0) withoutRole += 1;
    for (const role of held) byRole[role] += 1;
  }
  return { total: rows.length, byStatus, byRole, withoutRole };
}

/** 类型下拉的选项：全部、词表里每个角色、往来单位。 */
export const businessPartyTypeFilterOptions: readonly { value: BusinessPartyTypeFilter; label: string }[] = [
  ...codeFilterOptions(partyRoleLabels, '全部类型'),
  { value: 'NONE', label: partyTypeFallbackLabel },
];

/**
 * 工具条右端的计数摘要（票 01 裁决 1）：总数与当前显示数分开报。页面态里的空态说的是「登记册为空」，
 * 筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，后者改条件）——所以筛空时这里照显
 * 「共 N 家，当前显示 0 家」，表格区另显 businessPartyNoMatchNote，不把页面切成空态。
 */
export function businessPartyCountSummary(total: number, visible: number): string {
  return `共 ${total} 家单位，当前显示 ${visible} 家`;
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const businessPartyNoMatchNote = '当前筛选条件下没有匹配的单位';

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
