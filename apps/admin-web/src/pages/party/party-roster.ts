// 往来单位页主从分栏的纯逻辑（票 business-party-redesign/03）：名录栏一行名称下那一句类型、栏底的页码、右栏此刻显哪一行。
// 纯函数，node:test 钉着；PartyRoster.tsx 与页面只照着摆。

import { cursorTotalPages } from '../../templates/cursor-pagination';
import type { BusinessPartyRecord } from './api';
import { ourCompanyLabel, partyTypeAbsenceOf } from './business-party-list';
import { labelOf, partyRoleLabels, partyTypeFallbackLabel } from './presentation';

export interface PartyRosterType {
  text: string;
  /** 这一句说的是「没读到」，不是类型：调用方给它配上说明。 */
  unread: boolean;
}

/** 名录栏里「没读到」的那一句。栏里一行只有一句类型，单摆一个「—」认不出缺的是什么，所以写成字。 */
export const partyRosterTypeUnreadText = '类型没读到';

/**
 * 名录栏一行名称下的那一句类型：我方公司在前、角色照给的次序，用「 · 」连；一个都没有是「暂无合作关系」。推类型用的册没读全、
 * 这一行又推不出来时说「没读到」，不拿「暂无合作关系」冒充——判据与类型列是同一个（business-party-list.ts 的 partyTypeAbsenceOf）。
 */
export function partyRosterTypeOf(ours: boolean, roles: readonly string[], sourcesComplete: boolean): PartyRosterType {
  const absence = partyTypeAbsenceOf(ours, roles, sourcesComplete);
  if (absence === 'unread') return { text: partyRosterTypeUnreadText, unread: true };
  if (absence === 'none') return { text: partyTypeFallbackLabel, unread: false };
  return { text: [...(ours ? [ourCompanyLabel] : []), ...roles.map((role) => labelOf(partyRoleLabels, role))].join(' · '), unread: false };
}

/** 名录栏底翻页钮之间的页码：「2 / 7」；读口不给总数时只有「2」。栏窄，「第」「页」两个字由读屏的说明念。 */
export function partyRosterPageText(page: number, size: number, total: number | null): string {
  const pages = cursorTotalPages(size, total);
  return pages === null ? String(page) : `${page} / ${pages}`;
}

/** 选中的那家单位最后一次在列表里读到的那一行。 */
export interface RememberedParty {
  row: BusinessPartyRecord;
  /** 它名下有写入落册、之后还没重新读到：这一行可能已经不是册上的样子。 */
  stale: boolean;
}

/**
 * 页面按新开页重挂之后取回来的那一行。选中的那一家按标签保留（templates/workspace-view-state.tsx），离开这张标签满新鲜期再回来
 * 时页面整个重挂、列表重读；离开的那段时间里这一行可能已经变了——从右栏去对象页更正了名称、停用了，或者别人动过——所以取回来
 * 的这一份一律按「可能已经不是册上的样子」对待：重新读到之前先拿它显着，右栏不闪；读回来的两处都没有它就收起右栏
 * （rememberedPartyExpired），不拿一行没有重读过的旧样子接着显。
 */
export function rememberedPartyOnReturn(remembered: RememberedParty | null): RememberedParty | null {
  return remembered === null ? null : { ...remembered, stale: true };
}

/**
 * 右栏显哪一行。选中的那家不一定在当前这一页上——翻了页、换了条件把它筛掉了——右栏不因此关，名录栏也不为它跳页。先在当前
 * 这一页里找，再在不带条件的那一页里找（两处都是刚读到的），都没有才用记着的那一行。
 */
export function selectedPartyRowOf(
  selectedId: string | null,
  pageRows: readonly BusinessPartyRecord[],
  known: readonly BusinessPartyRecord[] | null,
  remembered: RememberedParty | null,
): BusinessPartyRecord | null {
  if (selectedId === null) return null;
  const sameParty = (row: BusinessPartyRecord) => row.partyId === selectedId;
  return pageRows.find(sameParty) ?? known?.find(sameParty) ?? (remembered !== null && sameParty(remembered.row) ? remembered.row : null);
}

/**
 * 记着的那一行还能不能接着显。写入落册之后它可能已经变了（停用了、改了名）：当前这一页答回来了、它不在上面，不带条件的那一页
 * 里也没有，就没有一处读得到它的新样子——不拿旧的一行接着显，调用方收起右栏。还在途时先留着，免得右栏闪一下。
 */
export function rememberedPartyExpired(
  selectedId: string | null,
  pageAnswered: boolean,
  pageRows: readonly BusinessPartyRecord[],
  known: readonly BusinessPartyRecord[] | null,
  remembered: RememberedParty | null,
): boolean {
  if (selectedId === null || remembered === null || !remembered.stale || !pageAnswered) return false;
  const sameParty = (row: BusinessPartyRecord) => row.partyId === selectedId;
  return !pageRows.some(sameParty) && !(known?.some(sameParty) ?? false);
}
