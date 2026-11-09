// 业务参与方页身份本体册的列表纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成工作台口径，与
// legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 检索、状态筛选与表头排序下推到目录读口（ADR-0144；读口的可排维与筛选维由后端 BusinessPartyPages 声明）：businessPartyListConditions
// 编条件，页面按游标翻页。读口按参与方标识、名称、注册号与外部编码检索；状态词与类型词不在其内，要按状态看用状态胶囊。
//
// 「类型」一列不是册上的字段：参与方是客户还是承运商，由它在关系册上持有的角色说（CONTEXT：角色由关系表达，身份本体不带
// 角色）。这里按关系册给每个参与方算出它持有的角色集合，列表按它称呼；一个角色都没有的显「暂无合作关系」。客户角色另认一处
// 事实：未停用客户账户的所属客户也持有它（CONTEXT 参与方身份诸句，ADR-0367 决定四）——账户不要求先登客户关系。
// 「我方公司」同理不是册上的字段（票 company-subject-ux/02）：法人册上钉着这个参与方的责任法人在，它就是我们自己的公司；
// 按法人册算出参与方标识集合，类型列按它称呼，与角色并列显示。类型由这几本册算出，读口筛不了，所以类型筛选只筛已取回的这一页
// （filterBusinessPartiesByType），控件上如实标出；这几本册各只取不带条件的第一页，角色与我方公司也只按那一页算。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
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
import { catalogueSortOf, codeFilterOptions, type CatalogueSortColumns, type CodeFilter, type SelectOption } from './list-order';
import { liveRelationshipStatuses } from './party-overview';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

/** 类型筛选：全部、某个角色、OURS——法人册上钉着的「我方公司」，或 NONE——既不是我方公司、一个角色也没有的（类型格显「暂无合作关系」）。 */
export type BusinessPartyTypeFilter = CodeFilter<PartyRoleCode> | 'NONE' | 'OURS';

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const businessPartyStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 参与方标识 → 它持有的角色（去重、按词表顺序）。 */
export type PartyRoleIndex = ReadonlyMap<string, readonly PartyRoleCode[]>;

export const emptyPartyRoleIndex: PartyRoleIndex = new Map();

// 在用的关系才算「持有」（party-overview.ts 的 liveRelationshipStatuses）：一家只剩过期合同的单位今天就不是客户。
const roleBearingStatuses = liveRelationshipStatuses;

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

/** 类型筛选的选项：我方公司在前、角色按词表序、「暂无合作关系」在最后，与类型列里的次序一样。 */
export const businessPartyTypeFilterOptions: readonly SelectOption<BusinessPartyTypeFilter>[] = [
  { value: 'ALL', label: '全部类型' },
  { value: 'OURS', label: ourCompanyLabel },
  ...(Object.keys(partyRoleLabels) as PartyRoleCode[]).map((role) => ({ value: role, label: partyRoleLabels[role] })),
  { value: 'NONE', label: partyTypeFallbackLabel },
];

/** 表头可排的列 → 读口声明的可排维。名称列不可排：读口不按名称排。 */
export const businessPartySortColumns: CatalogueSortColumns = {
  'effective-from': 'effectiveFrom',
  'registered-at': 'registeredAt',
};

/** 默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultBusinessPartySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/** 身份本体册一问的条件：检索词、状态（「全部」即这一维缺席）与表头排序；登记时间新→旧是读口的缺省序，不写进查询串。 */
export function businessPartyListConditions(
  keyword: string,
  status: BusinessPartyStatusFilter,
  sort: WorkbenchSort,
): CatalogueConditions {
  return {
    q: keyword,
    filters: status === 'ALL' ? {} : { status: [status] },
    sort: catalogueSortOf(sort, businessPartySortColumns, '-registeredAt'),
  };
}

/**
 * 类型筛选，只筛已取回的这一页：类型精确到角色码，OURS 只留我方公司，NONE 只留既不是我方公司、一个角色也没有的。
 */
export function filterBusinessPartiesByType(
  rows: readonly BusinessPartyRecord[],
  type: BusinessPartyTypeFilter,
  roles: PartyRoleIndex = emptyPartyRoleIndex,
  ours: OurCompanyIndex = emptyOurCompanyIndex,
): BusinessPartyRecord[] {
  if (type === 'ALL') return [...rows];
  return rows.filter((row) => {
    const held = rolesOfParty(roles, row.partyId);
    const isOurs = ours.has(row.partyId);
    if (type === 'OURS') return isOurs;
    if (type === 'NONE') return held.length === 0 && !isOurs;
    return held.includes(type);
  });
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const businessPartyNoMatchNote = '当前筛选条件下没有匹配的单位';
