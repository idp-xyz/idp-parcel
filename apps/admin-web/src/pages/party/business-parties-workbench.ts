// 往来单位页新版式的列表纯逻辑（票 business-party-redesign/02）：类型芯片与状态下拉上的数、「合作关系」一格、行首图标块取哪一种、
// 视图签右端待办的数、表底那两句。全部是纯函数，node:test 钉着；组件只照着摆。
//
// 数从哪来与此前一样：往来单位册与推类型用的册都读全时由页面数，读不全的降级在 business-party-list.ts 的
// partyTypeSourcesComplete 一组函数里。这里新定的只有一条规矩：筛选项上的数随「别的条件」走、不随自己那一项走——点下去会得到
// 几行，上面写的就是几。册读全时，读口按当前条件答回的那一页就是当前条件下的全部行（整册一页装得下，它的子集也装得下），
// 所以芯片按它数。

import { cursorTotalPages } from '../../templates/cursor-pagination';
import type { BusinessPartyRecord, PartyRelationshipRecord } from './api';
import {
  businessPartyStatusFilterOptions,
  ourCompanyLabel,
  type BusinessPartyCounts,
  type BusinessPartyStatusFilter,
  type BusinessPartyTypeFilter,
} from './business-party-list';
import { liveRelationshipStatuses } from './party-overview';
import { partyRoleLabels, partyTypeFallbackLabel, type PartyRoleCode } from './presentation';

/** 一家单位名下在用的关系有几段：作为持有方的与作为相对方的分开记，其中候选（待确认）的另记。 */
export interface PartyRelationshipTally {
  held: number;
  heldCandidates: number;
  toward: number;
  towardCandidates: number;
}

export type PartyRelationshipTallies = ReadonlyMap<string, PartyRelationshipTally>;

/** 按关系册给每家单位记数；只数在用的（候选与已生效），判据同类型列。关系册没取到交空表。 */
export function tallyPartyRelationships(relationships: readonly PartyRelationshipRecord[] | null): PartyRelationshipTallies {
  const tallies = new Map<string, PartyRelationshipTally>();
  const of = (partyId: string) => {
    let tally = tallies.get(partyId);
    if (!tally) {
      tally = { held: 0, heldCandidates: 0, toward: 0, towardCandidates: 0 };
      tallies.set(partyId, tally);
    }
    return tally;
  };
  for (const row of relationships ?? []) {
    if (!liveRelationshipStatuses.has(row.status)) continue;
    const candidate = row.status === 'CANDIDATE';
    const holder = of(row.holderId);
    holder.held += 1;
    if (candidate) holder.heldCandidates += 1;
    const counterparty = of(row.counterpartyId);
    counterparty.toward += 1;
    if (candidate) counterparty.towardCandidates += 1;
  }
  return tallies;
}

export interface PartyRelationshipCell {
  count: number;
  candidates: number;
}

/**
 * 列表「合作关系」一格。与类型列同一口径：只数它担任角色的在用关系——只当别人关系里相对方的单位，类型是「暂无合作关系」，
 * 这一格也空着，两格不打架。我方公司不靠角色定类型，两侧都数。一段都没有交 null。
 */
export function partyRelationshipCellOf(tallies: PartyRelationshipTallies, partyId: string, ours: boolean): PartyRelationshipCell | null {
  const tally = tallies.get(partyId);
  if (!tally) return null;
  const count = tally.held + (ours ? tally.toward : 0);
  if (count === 0) return null;
  return { count, candidates: tally.heldCandidates + (ours ? tally.towardCandidates : 0) };
}

/** 行首图标块有哪几种：每个角色一种，我方公司一种，都不是的一种。 */
export type PartyTileKind = PartyRoleCode | 'OURS' | 'NONE';

/** 这一行的图标块取哪一种：我方公司优先；否则取它持有的角色里词表序最前的一个；一个都没有、或码不在词表里的是 NONE。 */
export function partyTileKindOf(ours: boolean, roles: readonly string[]): PartyTileKind {
  if (ours) return 'OURS';
  const known = (Object.keys(partyRoleLabels) as PartyRoleCode[]).find((role) => roles.includes(role));
  return known ?? 'NONE';
}

export interface PartyTypeChip {
  type: BusinessPartyTypeFilter;
  label: string;
  /** 当前条件下有几家；null 即当前条件的那一问还没答出，不摆数——不拿 0 占位。 */
  count: number | null;
  /** 整册里这一类有几家。估芯片宽度用它：它不随检索词与状态变，芯片与下拉之间不会边筛边换。 */
  registerCount: number;
  active: boolean;
  /** 当前条件下一家都没有、又不是选中的那一枚：留在原位，点不动。 */
  disabled: boolean;
  /** 摆的时候角色一组、我方公司与「暂无合作关系」一组，中间隔一道。 */
  group: 'all' | 'role' | 'other';
}

/**
 * 类型芯片。摆哪几枚看整册（whole）：整册里一家都没有的类型不摆，摆出来的那几枚不随检索词与状态增减，免得边打字边跳；
 * 每枚上的数看当前条件下答回的行（conditioned），那一问还没答出（null）就不摆数。选中的那一枚总在。次序：全部、各角色按
 * 词表序、我方公司、暂无合作关系。
 */
export function partyTypeChipsOf(
  whole: BusinessPartyCounts,
  conditioned: BusinessPartyCounts | null,
  selected: BusinessPartyTypeFilter,
): PartyTypeChip[] {
  const chip = (
    type: Exclude<BusinessPartyTypeFilter, 'ALL'>,
    label: string,
    registerCount: number,
    count: number | null,
    group: 'role' | 'other',
  ): PartyTypeChip[] =>
    registerCount > 0 || selected === type
      ? [{ type, label, count, registerCount, active: selected === type, disabled: count === 0 && selected !== type, group }]
      : [];
  return [
    {
      type: 'ALL',
      label: '全部',
      count: conditioned?.total ?? null,
      registerCount: whole.total,
      active: selected === 'ALL',
      disabled: false,
      group: 'all',
    },
    ...(Object.keys(partyRoleLabels) as PartyRoleCode[]).flatMap((role) =>
      chip(role, partyRoleLabels[role], whole.byRole[role], conditioned?.byRole[role] ?? null, 'role'),
    ),
    ...chip('OURS', ourCompanyLabel, whole.ours, conditioned?.ours ?? null, 'other'),
    ...chip('NONE', partyTypeFallbackLabel, whole.withoutRole, conditioned?.withoutRole ?? null, 'other'),
  ];
}

/**
 * 状态筛选项上的数，名录与合作关系两个视图同一条规矩：随检索词与别的筛选走、不随状态自己走。
 * - 没有检索词：按整册（whole）数，只数合别的筛选的（matchesOthers）；
 * - 有检索词、状态是「全部」：读口答回的就是这个检索词下各状态的行（conditioned），按它数；
 * - 有检索词、又选了某个状态：读口只答回那一个状态的行，别的状态此刻数不出，只给选中的那一项数。
 * 整册没读全（whole 为 null）一律不给数；要用到读口答回的行而那一问还没答出（conditioned 为 null）也不给。检索词的匹配
 * 只由读口做，这里不在整册上再写一份。交出的是「某个状态值 → 数」；「全部」的值是 'ALL'。
 */
export function statusCountsUnderConditions<Row extends { status: string }>(input: {
  whole: readonly Row[] | null;
  conditioned: readonly Row[] | null;
  keyword: string;
  status: string;
  matchesOthers: (row: Row) => boolean;
}): (value: string) => number | null {
  const { whole, conditioned, keyword, status, matchesOthers } = input;
  const searching = keyword.trim() !== '';
  const base = whole === null ? null : searching ? conditioned : whole;
  if (base === null) return () => null;
  const source = base.filter(matchesOthers);
  if (searching && status !== 'ALL') return (value) => (value === status ? source.length : null);
  return (value) => (value === 'ALL' ? source.length : source.filter((row) => row.status === value).length);
}

export interface PartyStatusOption {
  value: BusinessPartyStatusFilter;
  label: string;
  /** null 即这一项此刻数不出，只写词。 */
  count: number | null;
}

/** 名录状态下拉的各项与它们的数（statusCountsUnderConditions）；「别的筛选」是类型。 */
export function partyStatusOptionsOf(input: {
  whole: readonly BusinessPartyRecord[] | null;
  conditioned: readonly BusinessPartyRecord[] | null;
  keyword: string;
  status: BusinessPartyStatusFilter;
  ofType: (row: BusinessPartyRecord) => boolean;
}): PartyStatusOption[] {
  const countOf = statusCountsUnderConditions({ ...input, matchesOthers: input.ofType });
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
 * 表底左边那一句，给带着「页面上筛的那一项」的视图用——名录的类型、合作关系的待查，都是读口筛不了、页面在到手的行上筛的。
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

/**
 * 视图签右端一项待办的数（几段合作关系待确认、几家单位尚未生效）。册读全了从手上的行数；读不全时取那一问只带状态条件的读
 * 答的总数——状态是这两册读口的筛选维，总数精确。两样都没有交 null：不摆，不拿 0 冒充「没有」。
 */
export function partyAttentionCount<Row>(
  whole: readonly Row[] | null,
  matches: (row: Row) => boolean,
  statusTotal: number | null,
): number | null {
  return whole !== null ? whole.filter(matches).length : statusTotal;
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
