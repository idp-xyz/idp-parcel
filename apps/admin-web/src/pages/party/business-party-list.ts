// 业务参与方页身份本体册的筛选、排序与计数纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成
// 工作台口径，与 legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 几件事都只在**已取回的行**上做（README 列表页上列通则：不下推成查询参数——那要改端点契约，
// 归票 04）。端点今天答的是每个参与方的最新修订、上限 isolatedReadLimit 一页，这里的排序与计数是对这一页，
// 不是对册；分页下推之前这条限制如实存在，页面不假装成全量。
//
// 「类型」一列不是册上的字段：参与方是客户还是承运商，由它在关系册上持有的角色说（CONTEXT：角色由关系表达，身份本体不带
// 角色）。这里按关系册给每个参与方算出它持有的角色集合，列表与筛选按它称呼；一个角色都没有的显「暂无合作关系」。客户角色
// 另认一处事实：未停用客户账户的所属客户也持有它（CONTEXT 参与方身份诸句，ADR-0367 决定四）——账户不要求先登客户关系。
// 「我方公司」同理不是册上的字段（票 company-subject-ux/02）：法人册上钉着这个参与方的责任法人在，它就是我们自己的公司；
// 按法人册算出参与方标识集合，类型列与筛选按它称呼，与角色并列显示。

import type { WorkbenchSort } from '../../templates/workbench';
import type { BusinessPartyRecord, CustomerAccountRecord, GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import {
  identityStatusLabels,
  labelOf,
  partyRoleLabels,
  partyTypeFallbackLabel,
  partyTypeLabel,
  type IdentityStatusCode,
  type PartyRoleCode,
} from './presentation';
import { byInstant, byString, codeFilterOptions, matchesSearch, type CodeFilter } from './list-order';
import { partyIdentificationSearchTerms } from './business-party-identification';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

/** 类型筛选：全部、某个角色、OURS——法人册上钉着的「我方公司」，或 NONE——既不是我方公司、一个角色也没有的（类型格显「暂无合作关系」）。 */
export type BusinessPartyTypeFilter = CodeFilter<PartyRoleCode> | 'NONE' | 'OURS';

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

// 未停用的客户账户才让所属客户持有客户角色：已登记待生效的算，与关系的「候选」同一条判据——登好了、等着用。
const customerBearingAccountStatuses = new Set(['REGISTERED', 'EFFECTIVE']);

/**
 * 按关系册给每个参与方算它持有的角色。只数持有方那一侧：角色由「哪一方对哪一方持有」表达，相对方是被持有的一方
 * （我们的公司主体、或授权交付时的客户），它不因此得到这个角色。词表之外的角色码不进索引——它没有中文可显，
 * 也不该冒充某个已知类型。客户账户册另给所属客户记一次客户角色（文件头）。哪一册没取到传 null，那一册不贡献角色。
 */
export function indexPartyRoles(
  relationships: readonly PartyRelationshipRecord[] | null,
  accounts: readonly CustomerAccountRecord[] | null = null,
): PartyRoleIndex {
  if (relationships === null && accounts === null) return emptyPartyRoleIndex;
  const index = new Map<string, Set<PartyRoleCode>>();
  const hold = (partyId: string, role: PartyRoleCode) => {
    const roles = index.get(partyId) ?? new Set<PartyRoleCode>();
    roles.add(role);
    index.set(partyId, roles);
  };
  for (const row of relationships ?? []) {
    if (!roleBearingStatuses.has(row.status)) continue;
    if (!(row.role in partyRoleLabels)) continue;
    hold(row.holderId, row.role as PartyRoleCode);
  }
  for (const account of accounts ?? []) {
    if (customerBearingAccountStatuses.has(account.status)) hold(account.customerPartyId, 'CUSTOMER');
  }
  const order = Object.keys(partyRoleLabels) as PartyRoleCode[];
  return new Map([...index].map(([partyId, roles]) => [partyId, order.filter((role) => roles.has(role))]));
}

export function rolesOfParty(index: PartyRoleIndex, partyId: string): readonly PartyRoleCode[] {
  return index.get(partyId) ?? [];
}

/** 法人册上钉着责任法人的参与方标识集合；法人册没取到即空集，所有行都不按「我方公司」显。 */
export type OurCompanyIndex = ReadonlySet<string>;

export const emptyOurCompanyIndex: OurCompanyIndex = new Set();

// 在用的两格才算「钉着」：已停用的法人不再是我们签约、开票用的公司，与角色只数在用关系同一条判据；待生效的算——它是登记好、
// 等着用的公司，列表上该让人看见它是我方的。
const ourCompanyBearingStatuses = new Set(['REGISTERED', 'EFFECTIVE']);

/**
 * 按法人册算哪些参与方是我方公司。法人钉的参与方就是行上的 partyId，不管名称转写到没到——转写不到是写入门失败的悬空，
 * 那一行仍钉着这个标识。法人册没取到传 null，集合为空。
 */
export function ourCompanyPartyIds(entities: readonly GroupLegalEntityRecord[] | null): OurCompanyIndex {
  if (entities === null) return emptyOurCompanyIndex;
  return new Set(entities.filter((row) => ourCompanyBearingStatuses.has(row.status)).map((row) => row.partyId));
}

/**
 * 法人册上有公司主体钉着的参与方，已停用的也算——与后端「我方公司不登号、不登参与方资料」那道门同一口径（ADR-0367 决定一、
 * ADR-0369 决定二）。对象页据它收起这两处写入口；列表类型格的「我方公司」只数在用的（ourCompanyPartyIds），两者用途不同。
 */
export function partiesPinnedByLegalEntities(entities: readonly GroupLegalEntityRecord[] | null): OurCompanyIndex {
  if (entities === null) return emptyOurCompanyIndex;
  return new Set(entities.map((row) => row.partyId));
}

/**
 * 列表「注册地」格要的公司主体那一半：参与方 → 钉着它的公司主体登的注册国家。我方公司不在往来单位上登识别信息（ADR-0367
 * 决定一），它的注册地登在公司主体上；已停用的公司主体也算——注册在哪儿不因停用而变。法人册没取到为空表。
 */
export function partyRegistrationCountriesOf(entities: readonly GroupLegalEntityRecord[] | null): ReadonlyMap<string, string> {
  const countries = new Map<string, string>();
  for (const row of entities ?? []) {
    if (row.identityLayerRegistered && row.registrationCountry) countries.set(row.partyId, row.registrationCountry);
  }
  return countries;
}

/** 「注册地」格：往来单位自己登了就用它，否则取公司主体那一份（partyRegistrationCountriesOf）；都没有为 null。 */
export function businessPartyRegistrationCountry(
  row: Pick<BusinessPartyRecord, 'partyId' | 'identityLayerRegistered' | 'registrationCountry'>,
  entityCountries: ReadonlyMap<string, string>,
): string | null {
  if (row.identityLayerRegistered && row.registrationCountry) return row.registrationCountry;
  return entityCountries.get(row.partyId) ?? null;
}

/** 往来单位详情栏的签，顺序即签条上的顺序。值存在视图状态里，换名会让旧状态失配。 */
export type BusinessPartyPanelTab = 'summary' | 'contacts' | 'profile' | 'identity' | 'overview';

/**
 * 详情栏摆哪几张签，判据同对象页：公司主体钉着的往来单位不收往来单位资料（ADR-0369 决定二），「联系人」「资料版本」两签不摆，
 * 概要里只剩一句指向公司主体的话。法人册还没答出（null）时照常摆——签条不因法人册晚到而先多后少，资料等它答出再读。
 */
export function businessPartyPanelTabsOf(pinned: boolean | null): readonly BusinessPartyPanelTab[] {
  return pinned === true ? ['summary', 'identity', 'overview'] : ['summary', 'contacts', 'profile', 'identity', 'overview'];
}

/** 记下的签这一行没有（翻到了我方公司那一行）就显概要；记下的值不动，翻回别的行仍停在那一签。 */
export function shownBusinessPartyPanelTab(
  stored: BusinessPartyPanelTab,
  tabs: readonly BusinessPartyPanelTab[],
): BusinessPartyPanelTab {
  return tabs.includes(stored) ? stored : 'summary';
}

/** 「我方公司」在类型列与筛选里的词；只在这一处。 */
export const ourCompanyLabel = '我方公司';

/**
 * 类型列的文字：我方公司排在它持有的角色前面（一家我方公司也可以在关系册上对别人担任客户），不是我方公司就照角色称呼，
 * 一个角色都没有的叫「往来单位」（partyTypeLabel）。
 */
export function businessPartyTypeLabel(ours: boolean, roles: readonly string[]): string {
  if (!ours) return partyTypeLabel(roles);
  return [ourCompanyLabel, ...roles.map((role) => labelOf(partyRoleLabels, role))].join(' / ');
}

/** 表头可排的四列。 */
export type BusinessPartySortKey = 'party' | 'party-name' | 'effective-from' | 'registered-at';

/** 默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultBusinessPartySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/**
 * 搜索是包含匹配、不分大小写，命中参与方标识 / 名称 / 状态原名与中文 / 类型中文 / 注册号 / 外部编码任一格；类型筛选精确到角色码，
 * OURS 只留我方公司，NONE 只留既不是我方公司、一个角色也没有的。
 */
export function filterBusinessParties(
  rows: readonly BusinessPartyRecord[],
  filter: BusinessPartyFilter,
  roles: PartyRoleIndex = emptyPartyRoleIndex,
  ours: OurCompanyIndex = emptyOurCompanyIndex,
): BusinessPartyRecord[] {
  const type = filter.type ?? 'ALL';
  return rows.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    const held = rolesOfParty(roles, row.partyId);
    const isOurs = ours.has(row.partyId);
    if (type === 'OURS' && !isOurs) return false;
    if (type === 'NONE' && (held.length > 0 || isOurs)) return false;
    if (type !== 'ALL' && type !== 'NONE' && type !== 'OURS' && !held.includes(type)) return false;
    return matchesSearch(filter.search, [
      row.partyId,
      row.partyName,
      row.status,
      identityStatusLabels[row.status as IdentityStatusCode] ?? '',
      businessPartyTypeLabel(isOurs, held),
      ...partyIdentificationSearchTerms(row),
    ]);
  });
}

export interface BusinessPartyCounts {
  total: number;
  byStatus: Record<IdentityStatusCode, number>;
  /** 持有每个角色的参与方数；一个参与方持有两个角色时两格各记一次。 */
  byRole: Record<PartyRoleCode, number>;
  /** 法人册上钉着的「我方公司」数；同时持有角色的也各记一次。 */
  ours: number;
  /** 既不是我方公司、一个角色都没有的（「往来单位」）。 */
  withoutRole: number;
}

/**
 * 命令头指标与状态胶囊的计数，数的是已取回的这一页。本册没有「待补」一格：参与方册是名称的来源，没有转写不到的格；
 * 身份层两格是法人册的事。所以按状态与角色分格。
 */
export function countBusinessParties(
  rows: readonly BusinessPartyRecord[],
  roles: PartyRoleIndex = emptyPartyRoleIndex,
  ours: OurCompanyIndex = emptyOurCompanyIndex,
): BusinessPartyCounts {
  const byStatus: Record<IdentityStatusCode, number> = { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 };
  const byRole = Object.fromEntries(Object.keys(partyRoleLabels).map((role) => [role, 0])) as Record<PartyRoleCode, number>;
  let oursCount = 0;
  let withoutRole = 0;
  for (const row of rows) {
    if (row.status in byStatus) byStatus[row.status as IdentityStatusCode] += 1;
    const held = rolesOfParty(roles, row.partyId);
    const isOurs = ours.has(row.partyId);
    if (isOurs) oursCount += 1;
    if (held.length === 0 && !isOurs) withoutRole += 1;
    for (const role of held) byRole[role] += 1;
  }
  return { total: rows.length, byStatus, byRole, ours: oursCount, withoutRole };
}

/** 类型下拉的选项：全部、我方公司、词表里每个角色、往来单位。我方公司排在角色前面，与类型列里的次序一样。 */
export const businessPartyTypeFilterOptions: readonly { value: BusinessPartyTypeFilter; label: string }[] = (() => {
  const [all, ...roles] = codeFilterOptions(partyRoleLabels, '全部类型');
  return [all, { value: 'OURS', label: ourCompanyLabel }, ...roles, { value: 'NONE', label: partyTypeFallbackLabel }];
})();

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
