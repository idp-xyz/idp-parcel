// 往来单位页新版式的列表纯逻辑（票 business-party-redesign/02）：类型芯片与状态下拉上的数、「合作关系」一格、行首图标块取哪一种、
// 表底那两句。全部是纯函数，node:test 钉着；组件只照着摆。
//
// 筛选项上的数取读口答复里的分面（ADR-0378 决定五，票 business-party-redesign/09）：读口按点了名的筛选维答各值的行数，条件取
// 这一问的检索词与其余各维、不含该维自己——点下去会得到几行，那一格上就是几。「数随别的条件走、不随自己那一项走」这条规矩因此
// 由读口保证，往来单位再多也数得出、筛得准。此前是页面在读全了的册上自己数，册一多就数不出，只能降级。

import { cursorTotalPages } from '../../templates/cursor-pagination';
import type { CatalogueFacets } from '../catalogue-query';
import type { BusinessPartyRecord } from './api';
import {
  businessPartyCategoryOf,
  businessPartyStatusFilterOptions,
  ourCompanyLabel,
  partyIsOurCompany,
  partyRolesOf,
  type BusinessPartyStatusFilter,
  type BusinessPartyTypeFilter,
} from './business-party-list';
import { partyRoleLabels, partyTypeFallbackLabel, type PartyRoleCode } from './presentation';

/** 分面里一维各值的行数。 */
export type FacetCounts = ReadonlyMap<string, number>;

/** 从答复的分面里取一维。这一维没有答（答复还没到、读口没给这一格）交 null——不拿一张空表冒充「各值都是零」。 */
export function facetCountsOf(facets: CatalogueFacets | null | undefined, dimension: string): FacetCounts | null {
  const entries = facets?.[dimension];
  return entries ? new Map(entries.map((entry) => [entry.value, entry.count])) : null;
}

/**
 * 单值维各值之和：一行只落在一个值上，和就是「这一维不筛时」当前别的条件下的行数，状态的「全部」那一项用它。多值维不能这么加
 * ——一家单位可以落在几个类别里（partyTypeAllCount）。
 */
export function facetTotal(counts: FacetCounts | null): number | null {
  if (counts === null) return null;
  let total = 0;
  for (const count of counts.values()) total += count;
  return total;
}

/**
 * 筛选项摆不摆：整册里一条都没有的值不摆（`whole` 是不带条件那一问的分面），摆出来的不随检索词与别的筛选增减，免得边筛边跳；
 * 选中的那一项总在。没有分面可看（null）就都摆：不知道哪些是零，不替它收，筛空了也要能点回「全部」。
 */
export function facetOptionShown(whole: FacetCounts | null, value: string, selected: string): boolean {
  return whole === null || value === selected || (whole.get(value) ?? 0) > 0;
}

/**
 * 状态筛选项上的数，名录的状态下拉与合作关系的状态芯片同一条规矩：取当前条件那一问的状态分面；「全部」（'ALL'）是各状态之和。
 * 那一问还没答出（null）一律不给数，只写词——不拿 0 占位。
 */
export function statusCountsOf(conditioned: FacetCounts | null): (value: string) => number | null {
  const total = facetTotal(conditioned);
  return (value) => (value === 'ALL' ? total : (conditioned?.get(value) ?? null));
}

export interface PartyRelationshipCell {
  count: number;
  candidates: number;
}

/**
 * 列表「合作关系」一格：这家单位名下在用的合作关系共几段、其中待确认几段，两侧都算（partyLiveRelationshipsOf），数取行上
 * 答回的。一段都没有交 null。与右栏「关联」签、停用对话框里的「名下还有几段」是同一个数。
 *
 * 此前不是我方公司的只数它担任角色的那一侧（票 business-party-redesign/15 改）：那是为了迁就类型列的旧词「暂无合作关系」——
 * 只当相对方的单位类型写着「暂无」，这一格就也得空着。类型列改说「未担任角色」之后两格各说各的，不必再少数一侧。
 */
export function partyRelationshipCellOf(row: Pick<BusinessPartyRecord, 'relationshipCounts'>): PartyRelationshipCell | null {
  const { live, candidates } = partyLiveRelationshipsOf(row);
  return live === 0 ? null : { count: live, candidates };
}

/**
 * 一家单位名下在用的合作关系共几段、其中待确认几段，两侧都算：列表「合作关系」一格、右栏那句「还没有合作关系」、停用对话框的
 * 说明与「我方关联」名录栏的那一句用它。数是读口在行上答的，不随合作关系册读到第几页而变。
 */
export function partyLiveRelationshipsOf(row: Pick<BusinessPartyRecord, 'relationshipCounts'>): { live: number; candidates: number } {
  const { asHolder, asCounterparty } = row.relationshipCounts;
  return { live: asHolder.live + asCounterparty.live, candidates: asHolder.candidate + asCounterparty.candidate };
}

/** 行首图标块有哪几种：每个角色一种，我方公司一种，都不是的一种。 */
export type PartyTileKind = PartyRoleCode | 'OURS' | 'NONE';

/** 这一行的图标块取哪一种：我方公司优先；否则取它持有的角色里词表序最前的一个；一个都没有、或码不在词表里的是 NONE。 */
export function partyTileKindOf(ours: boolean, roles: readonly string[]): PartyTileKind {
  if (ours) return 'OURS';
  const known = (Object.keys(partyRoleLabels) as PartyRoleCode[]).find((role) => roles.includes(role));
  return known ?? 'NONE';
}

/** 一行的图标块，照读口在这一行上答的类别取。 */
export function partyTileKindOfRow(row: Pick<BusinessPartyRecord, 'categories'>): PartyTileKind {
  return partyTileKindOf(partyIsOurCompany(row), partyRolesOf(row));
}

export interface PartyTypeChip {
  type: BusinessPartyTypeFilter;
  label: string;
  /** 当前条件下有几家；null 即此刻数不出（那一问还没答出，或「全部」凑不出来），不摆数——不拿 0 占位。 */
  count: number | null;
  /** 整册里这一类有几家。估芯片宽度用它：它不随检索词与状态变，芯片与下拉之间不会边筛边换。 */
  registerCount: number;
  active: boolean;
  /** 当前条件下一家都没有、又不是选中的那一枚：留在原位，点不动。 */
  disabled: boolean;
  /** 摆的时候角色一组、我方公司与「未担任角色」一组，中间隔一道。 */
  group: 'all' | 'role' | 'other';
}

/**
 * 类型芯片。摆哪几枚看不带条件那一问的类别分面（whole，facetOptionShown）；每枚上的数看当前条件那一问的类别分面
 * （conditioned），那一问还没答出就不摆数。次序：全部、各角色按词表序、我方公司、未担任角色。
 *
 * 「全部」上的数由调用方给（allCount，partyTypeAllCount）：分面只答各类的家数，而一家单位可以落在几类里，各类之和不是总家数。
 */
export function partyTypeChipsOf(input: {
  whole: FacetCounts | null;
  /** 整册共几家（不带条件那一问的总数），只用来估「全部」那一枚的宽度。 */
  wholeTotal: number | null;
  conditioned: FacetCounts | null;
  allCount: number | null;
  selected: BusinessPartyTypeFilter;
}): PartyTypeChip[] {
  const { whole, wholeTotal, conditioned, allCount, selected } = input;
  const chip = (type: Exclude<BusinessPartyTypeFilter, 'ALL'>, label: string, group: 'role' | 'other'): PartyTypeChip[] => {
    const category = businessPartyCategoryOf(type);
    if (!facetOptionShown(whole, category, selected === type ? category : '')) return [];
    const count = conditioned?.get(category) ?? null;
    return [{ type, label, count, registerCount: whole?.get(category) ?? 0, active: selected === type, disabled: count === 0 && selected !== type, group }];
  };
  return [
    { type: 'ALL', label: '全部', count: allCount, registerCount: wholeTotal ?? 0, active: selected === 'ALL', disabled: false, group: 'all' },
    ...(Object.keys(partyRoleLabels) as PartyRoleCode[]).flatMap((role) => chip(role, partyRoleLabels[role], 'role')),
    ...chip('OURS', ourCompanyLabel, 'other'),
    ...chip('NONE', partyTypeFallbackLabel, 'other'),
  ];
}

/**
 * 类型「全部」那一枚上的数：类型不筛时，当前检索词与状态下有几家。读口不直接答它，从手上的几样里凑：
 * - 类型此刻就没筛：当前这一问的总数就是它；
 * - 筛着类型、没有检索词：从不带条件那一问里取——状态也没筛是整册的总数，筛着状态是那个状态的家数；
 * - 筛着类型、又有检索词：凑不出，交 null，那一枚只写词。为这一个数另发一问不值当。
 */
export function partyTypeAllCount(input: {
  type: BusinessPartyTypeFilter;
  keyword: string;
  status: BusinessPartyStatusFilter;
  /** 当前条件那一问的总数；还没答出为 null。 */
  conditionedTotal: number | null;
  wholeTotal: number | null;
  /** 不带条件那一问的状态分面。 */
  wholeStatus: FacetCounts | null;
}): number | null {
  const { type, keyword, status, conditionedTotal, wholeTotal, wholeStatus } = input;
  if (type === 'ALL') return conditionedTotal;
  if (keyword.trim() !== '') return null;
  if (status === 'ALL') return wholeTotal;
  return wholeStatus?.get(status) ?? null;
}

export interface PartyStatusOption {
  value: BusinessPartyStatusFilter;
  label: string;
  /** null 即这一项此刻数不出，只写词。 */
  count: number | null;
}

/** 名录状态下拉的各项与它们的数（statusCountsOf）：随检索词与类型走、不随状态自己走。 */
export function partyStatusOptionsOf(conditioned: FacetCounts | null): PartyStatusOption[] {
  const countOf = statusCountsOf(conditioned);
  return businessPartyStatusFilterOptions.map((option) => ({ value: option.value, label: option.label, count: countOf(option.value) }));
}

export interface PartyColumnLayout {
  /** 各列定宽加行尾的箭头都放得下：余量全落在行尾。 */
  fits: boolean;
  /** 各列的宽；null 即不定宽、拿剩下的（放不下时的首列）。 */
  widths: (number | null)[];
  /** 表至少这么宽，再窄就横向滚；放得下时不用设。 */
  minWidth: number | null;
}

/**
 * 列表这么宽时各列怎么排。放得下（各列定宽加行尾箭头）就各按定宽。放不下时箭头不摆、首列拿剩下的；首列还够它的定宽时别的列
 * 不动，不够了别的列才按比例往各自的窄宽收——只收到补上缺口为止，不为省而省；都收到窄宽还不够，首列再让，让到 firstMin 为止，
 * 再窄就横向滚，不把名称挤没。
 */
export function partyColumnLayoutOf(
  listWidth: number | null,
  columns: readonly { width: number; narrowWidth?: number }[],
  trailingMin: number,
  firstMin: number,
): PartyColumnLayout {
  const full = columns.reduce((sum, column) => sum + column.width, 0);
  if (listWidth === null || full + trailingMin <= listWidth) {
    return { fits: true, widths: columns.map((column) => column.width), minWidth: null };
  }
  const others = columns.slice(1);
  const narrowOf = (column: { width: number; narrowWidth?: number }) => Math.min(column.width, column.narrowWidth ?? column.width);
  const slack = others.reduce((sum, column) => sum + column.width - narrowOf(column), 0);
  const ratio = slack === 0 ? 0 : Math.min(1, Math.max(0, full - listWidth) / slack);
  return {
    fits: false,
    widths: [null, ...others.map((column) => Math.round(column.width - (column.width - narrowOf(column)) * ratio))],
    minWidth: others.reduce((sum, column) => sum + narrowOf(column), 0) + firstMin,
  };
}

/**
 * 工具条那一行摆不摆得下类型芯片：摆不下就收成下拉，不折成第二行。按字数与位数估每枚的宽，加上同一行别的件（检索框、状态
 * 下拉、「类型」二字、清除筛选）与间距；估得偏宽一点，宁可早收成下拉，不让芯片被行尾裁掉。位数按整册的数估（它是当前
 * 条件下的数的上限），所以摆芯片还是下拉只随工具条宽度与整册变，不随检索词与状态变。
 */
export function partyTypeChipsFit(toolbarWidth: number | null, chips: readonly PartyTypeChip[]): boolean {
  if (toolbarWidth === null) return false;
  const others = 48 + 256 + 150 + 40 + 76 + 10 * 5;
  const chipsWidth = chips.reduce(
    (sum, chip) => sum + 22 + (chip.group === 'role' ? 12 : 0) + chip.label.length * 13 + formatCount(chip.registerCount).length * 8 + 6,
    0,
  );
  return others + chipsWidth + 12 <= toolbarWidth;
}

/**
 * 表底左边那一句，给带着「页面上筛的那一项」的视图用——合作关系的待查：读口筛不了、页面在到手的行上筛的。
 * 整册读得全时页面筛得准，说的是筛出来的条数；读不全时总数是读口答的（不含页面上筛的那一项），页面上筛了就另说一句本页
 * 筛出几条。
 */
export function partyFilteredFooterCountText(input: {
  /** 页面上筛的那一项是不是筛得准（要用的册都读全）。 */
  exact: boolean;
  /** 读口答的总数；null 即读口不给。 */
  total: number | null;
  /** 读口答回的这一页有几行。 */
  pageRows: number;
  /** 页面上筛完摆出来的行数。 */
  shown: number;
  /** 页面上筛的那一项此刻筛没筛。 */
  pageFiltered: boolean;
  unit: string;
}): string {
  const { exact, total, pageRows, shown, pageFiltered, unit } = input;
  if (exact) return partyFooterCountText(shown, shown, unit);
  const base = partyFooterCountText(total, pageRows, unit);
  return pageFiltered ? `${base}，本页筛出 ${formatCount(shown)} ${unit}` : base;
}

/** 千位分节的数：往来单位册上万家是常事。 */
export function formatCount(value: number): string {
  return value.toLocaleString('en-US');
}

/** 表底左边那一句：「共 1,286 家」；读口不给总数时只说得出本页几家。 */
export function partyFooterCountText(total: number | null, shown: number, unit: string): string {
  return total === null ? `本页 ${formatCount(shown)} ${unit}` : `共 ${formatCount(total)} ${unit}`;
}

/** 表底翻页钮之间的页码：「第 2 / 7 页」；读口不给总数时只说「第 2 页」。 */
export function partyFooterPageText(page: number, size: number, total: number | null): string {
  const pages = cursorTotalPages(size, total);
  return pages === null ? `第 ${page} 页` : `第 ${page} / ${pages} 页`;
}
