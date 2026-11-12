// 业务参与方页身份本体册的列表纯逻辑（票 admin-web-group-legal-entities/09 立，票 business-parties-uiux/01 改成工作台口径，与
// legal-entity-list.ts 同形）。全部是纯函数，node:test 钉着；页面只负责摆。
//
// 检索、状态与类型筛选、表头排序都下推到目录读口（ADR-0144；读口的可排维与筛选维由后端 BusinessPartyPages 声明）：
// businessPartyListConditions 编条件，页面按游标翻页。读口按参与方标识、名称、注册号与外部编码检索；状态词与类型词不在其内，
// 要按状态、类型看用各自的筛选。
//
// 「类型」不是身份上的一格：参与方是客户还是承运商，由它在关系册上持有的角色说（CONTEXT：角色由关系表达，身份本体不带角色）；
// 客户角色另认未停用客户账户的所属客户（ADR-0367 决定四）；被未停用的责任法人钉着的另算我方公司。这条规则的判定在后端一处
// （ADR-0378）：读口在每一行上答 `categories`——它此刻持有的各个角色，加「我方公司」「无角色」两个词，永不为空——并可按它筛、
// 按它给分面计数。管理台只照着摆，自己不推（票 business-party-redesign/09）：此前是按合作关系、客户账户、公司主体各册的第一页
// 推的，往来单位一多就推不全，把没读到说成没有，操作者会照着去重登一段已有的关系。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
import type { BusinessPartyRecord, GroupLegalEntityRecord } from './api';
import { identityStatusLabels, labelOf, partyRoleLabels, partyTypeLabel, type IdentityStatusCode, type PartyRoleCode } from './presentation';
import { catalogueSortOf, codeFilterOptions, type CatalogueSortColumns, type CodeFilter } from './list-order';

/** 身份状态封闭三格（domain IdentityStatus 原名，从 identityStatusLabels 的键派生）加「全部」。 */
export type BusinessPartyStatusFilter = CodeFilter<IdentityStatusCode>;

/** 类型筛选：全部、某个角色、OURS——我方公司，或 NONE——既不是我方公司、一个角色也没有的（类型格显「未担任角色」）。 */
export type BusinessPartyTypeFilter = CodeFilter<PartyRoleCode> | 'NONE' | 'OURS';

// 胶囊的码与词都从 identityStatusLabels 派生——那份词表是 CONTEXT 原词在前端的唯一一处。
export const businessPartyStatusFilterOptions = codeFilterOptions(identityStatusLabels, '全部');

/** 读口 `categories` 里不是角色的两个词（ADR-0378 决定三）：被未停用的责任法人钉着；前面各项都不成立。 */
export const ourCompanyCategory = 'OUR_COMPANY';
export const noRoleCategory = 'NO_ROLE';

/** 这一行是不是我方公司：读口答的类别里有那个词。 */
export function partyIsOurCompany(row: Pick<BusinessPartyRecord, 'categories'>): boolean {
  return row.categories.includes(ourCompanyCategory);
}

/**
 * 这一行此刻持有的角色：读口答的类别里去掉不是角色的那两个词，按词表序排——类型标签、名录栏的那一句、图标块都照这个次序。
 * 词表之外的词（服务端新增的角色）排在后面原样留着：它是读口说的事实，没有中文可显也不该藏起来。
 */
export function partyRolesOf(row: Pick<BusinessPartyRecord, 'categories'>): string[] {
  const held = row.categories.filter((category) => category !== ourCompanyCategory && category !== noRoleCategory);
  const order = Object.keys(partyRoleLabels);
  const rank = (role: string) => (order.includes(role) ? order.indexOf(role) : order.length);
  return [...new Set(held)].sort((left, right) => rank(left) - rank(right));
}

/**
 * 法人册上有公司主体钉着的参与方，已停用的也算——与后端「我方公司不登号、不登参与方资料」那道门同一口径（ADR-0367 决定一、
 * ADR-0369 决定二）。对象页据它收起这两处写入口。它与类别里的「我方公司」不是一回事：那一个只认未停用的责任法人，这一个连
 * 已停用的也算，所以不能拿行上的类别顶。法人册没取到即空集。
 */
export function partiesPinnedByLegalEntities(entities: readonly GroupLegalEntityRecord[] | null): ReadonlySet<string> {
  return new Set((entities ?? []).map((row) => row.partyId));
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

/** 类型筛选的值对应读口 `categories` 维的哪个词：角色原样，我方公司与「未担任角色」各是读口的那两个词。 */
export function businessPartyCategoryOf(type: Exclude<BusinessPartyTypeFilter, 'ALL'>): string {
  if (type === 'OURS') return ourCompanyCategory;
  if (type === 'NONE') return noRoleCategory;
  return type;
}

/** 表头可排的列 → 读口声明的可排维。名称列不可排：读口不按名称排。 */
export const businessPartySortColumns: CatalogueSortColumns = {
  'effective-from': 'effectiveFrom',
  'registered-at': 'registeredAt',
};

/** 默认登记时间新→旧（票 09 裁决 1，判据同票 01 裁决 2：运营配置员最常看「刚登进去的那条」）。 */
export const defaultBusinessPartySort: WorkbenchSort = { key: 'registered-at', dir: -1 };

/**
 * 身份本体册一问的条件：检索词、状态与类型（「全部」即那一维缺席）与表头排序；登记时间新→旧是读口的缺省序，不写进查询串。
 * 类型下推成读口的 `categories` 维。
 */
export function businessPartyListConditions(
  keyword: string,
  status: BusinessPartyStatusFilter,
  type: BusinessPartyTypeFilter,
  sort: WorkbenchSort,
): CatalogueConditions {
  return {
    q: keyword,
    filters: {
      status: status === 'ALL' ? [] : [status],
      categories: type === 'ALL' ? [] : [businessPartyCategoryOf(type)],
    },
    sort: catalogueSortOf(sort, businessPartySortColumns, '-registeredAt'),
  };
}

/** 筛出为空时表格区那一行的话；措辞点明是「筛选条件」，与空态「尚无登记」分得开。 */
export const businessPartyNoMatchNote = '当前筛选条件下没有匹配的单位';
