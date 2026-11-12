// 往来单位页「我方关联」一册的判读（票 business-party-relationship-graph/02）。纯函数，node:test 钉着；页面与
// OurCompanyNetworkPanel.tsx 只负责摆。
//
// 这一册以我方公司为主体：每一家我方公司一行，数出谁对它担任什么角色、它又对谁担任什么角色，选中一行在右栏看以它为中心的
// 关系图。哪些单位是我方公司由读口说（票 business-party-redesign/09）：行来自往来单位读口按类别「我方公司」筛出的那一问，
// 名称与在用的段数也取那一行上答的——不再拿法人册自己推一遍。按角色分的段数读口不在行上答，仍从合作关系册数：只数还在起作用
// 的段（party-overview.ts 的 liveRelationshipStatuses），与关系图同一条。只是读取时的组合，不是第二本册。
//
// 选中一家之后是主从分栏（票 business-party-redesign/04）：左边名录栏一行两句，右边是这家公司的关系图与两张按角色的汇总。
// 名录栏那一句、汇总里每行的小字，判读也在这里。

import type { BusinessPartyRecord, GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import { formatCount, partyLiveRelationshipsOf } from './business-parties-workbench';
import { liveRelationshipStatuses, relationshipsOfParty, type PartyRelationshipView } from './party-overview';
import { partyRoleLabels } from './presentation';

export interface RoleCount {
  role: string;
  /** 这个角色下还在起作用的段数。 */
  count: number;
  /** 其中待确认的段数。 */
  candidates: number;
}

export interface OurCompanyNetworkRow {
  partyId: string;
  partyName: string;
  /** 钉着这家参与方的未停用公司主体，按标识排；法人册没读到时为空。 */
  legalEntityIds: string[];
  /** 对它担任各角色的单位段数（它是相对方），按角色词表序。从合作关系册到手的行里数。 */
  towardIt: RoleCount[];
  /** 它对其担任各角色的段数（它是持有方）。 */
  fromIt: RoleCount[];
  /** 还在起作用的段数，与其中待确认的段数：读口在这一行上答的，不随合作关系册读到第几页而变。 */
  live: number;
  candidates: number;
  /** 合作关系册到手的行里这家公司名下的段（含已结束），右栏的关系图拿它画。 */
  views: PartyRelationshipView[];
}

const roleOrder = Object.keys(partyRoleLabels);

/** 把一批关系按角色数出段数与其中待确认的段数，按角色词表序；只数还在起作用的由调用方先筛。 */
export function roleCountsOf(views: readonly PartyRelationshipView[]): RoleCount[] {
  const counts = new Map<string, RoleCount>();
  for (const view of views) {
    const entry = counts.get(view.role) ?? { role: view.role, count: 0, candidates: 0 };
    entry.count += 1;
    if (view.status === 'CANDIDATE') entry.candidates += 1;
    counts.set(view.role, entry);
  }
  const rank = (role: string) => (roleOrder.includes(role) ? roleOrder.indexOf(role) : roleOrder.length);
  return [...counts.values()].sort((left, right) => rank(left.role) - rank(right.role) || left.role.localeCompare(right.role));
}

/** 一家单位名下还在起作用的段，按它站在哪一侧分开数角色：对它担任角色的（它是相对方）、它对别人担任的（它是持有方）。 */
export function roleCountsBySide(views: readonly PartyRelationshipView[]): { towardIt: RoleCount[]; fromIt: RoleCount[] } {
  const live = views.filter((view) => liveRelationshipStatuses.has(view.status));
  return {
    towardIt: roleCountsOf(live.filter((view) => view.side === 'COUNTERPARTY')),
    fromIt: roleCountsOf(live.filter((view) => view.side === 'HOLDER')),
  };
}

/**
 * 每家我方公司一行；关系多的在前，同数按名称。`ourParties` 是往来单位读口按类别「我方公司」筛出的行，那一问没答出交空表，
 * 页面照它的状态说话。`entities` 只用来列出钉着它的公司主体编号（已停用的不列），没读到就不列。
 */
export function ourCompanyNetworkRows(
  ourParties: readonly BusinessPartyRecord[] | null,
  entities: readonly GroupLegalEntityRecord[] | null,
  relationships: readonly PartyRelationshipRecord[],
): OurCompanyNetworkRow[] {
  if (ourParties === null) return [];
  const rows = ourParties.map((party) => {
    const pinning = (entities ?? []).filter((entity) => entity.partyId === party.partyId && entity.status !== 'DEACTIVATED');
    const views = relationshipsOfParty(party.partyId, relationships);
    return {
      partyId: party.partyId,
      partyName: party.partyName,
      legalEntityIds: [...new Set(pinning.map((entity) => entity.legalEntityId))].sort(),
      ...roleCountsBySide(views),
      ...partyLiveRelationshipsOf(party),
      views,
    };
  });
  return rows.sort((left, right) => right.live - left.live || left.partyName.localeCompare(right.partyName, 'zh-CN'));
}

/** 列表检索：包含匹配名称、参与方标识与公司主体标识。这一册是页面组合出来的，读口没有它，只能在手上这几行里找。 */
export function matchesOurCompanySearch(row: OurCompanyNetworkRow, search: string): boolean {
  const needle = search.trim().toLowerCase();
  if (needle === '') return true;
  return [row.partyName, row.partyId, ...row.legalEntityIds].some((text) => text.toLowerCase().includes(needle));
}

/** 「10 段在用 · 1 段待确认」：名录栏一行名称下的那一句与右栏关系图段头上的数是同一句。没有待确认的不写后半句。 */
export function ourCompanyLiveText(row: Pick<OurCompanyNetworkRow, 'live' | 'candidates'>): string {
  return `${formatCount(row.live)} 段在用${row.candidates > 0 ? ` · ${formatCount(row.candidates)} 段待确认` : ''}`;
}

/** 名录栏一行名称下的那一句。段数是读口在行上答的真数，一段在用的都没有就照实说没有。 */
export function ourCompanyRosterLineOf(row: Pick<OurCompanyNetworkRow, 'live' | 'candidates'>): string {
  return row.live > 0 ? ourCompanyLiveText(row) : '没有在用的合作关系';
}

/** 按角色的汇总里一行的小字：这个角色下有待确认的就报几段，没有就说都已生效——在用的只有待确认与已生效两种。 */
export function roleCountNoteOf(count: RoleCount): string {
  return count.candidates > 0 ? `其中 ${formatCount(count.candidates)} 段待确认` : '都已生效';
}
