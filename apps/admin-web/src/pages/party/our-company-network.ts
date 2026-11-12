// 往来单位页「我方关联」一册的判读（票 business-party-relationship-graph/02）。纯函数，node:test 钉着；页面与
// OurCompanyNetworkPanel.tsx 只负责摆。
//
// 这一册以我方公司为主体：每一家我方公司一行，数出谁对它担任什么角色、它又对谁担任什么角色，选中一行在右栏看以它为中心的
// 关系图。「我方公司」与往来单位列表类型格同一口径（business-party-list.ts 的 ourCompanyPartyIds：法人册上在用的公司主体钉着的
// 参与方）；关系只数还在起作用的段（party-overview.ts 的 liveRelationshipStatuses），与关系图同一条。只是读取时的组合，不是第二本册。
//
// 选中一家之后是主从分栏（票 business-party-redesign/04）：左边名录栏一行两句，右边是这家公司的关系图与两张按角色的汇总。
// 名录栏那一句、汇总里每行的小字、合作关系册读到什么程度，判读也在这里。

import type { GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import { formatCount } from './business-parties-workbench';
import { ourCompanyPartyIds } from './business-party-list';
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
  /** 名称转写不到时为 null。 */
  partyName: string | null;
  /** 钉着这家参与方的在用公司主体，按标识排。 */
  legalEntityIds: string[];
  /** 对它担任各角色的单位段数（它是相对方），按角色词表序。 */
  towardIt: RoleCount[];
  /** 它对其担任各角色的段数（它是持有方）。 */
  fromIt: RoleCount[];
  /** 还在起作用的段数，与其中待确认的段数。 */
  live: number;
  candidates: number;
  /** 这家我方公司名下的全部段（含已结束），右栏的关系图拿它画。 */
  views: PartyRelationshipView[];
}

const roleOrder = Object.keys(partyRoleLabels);

function countsByRole(views: readonly PartyRelationshipView[]): RoleCount[] {
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

/** 每家我方公司一行；关系多的在前，同数按名称。法人册没答出交空表，页面照法人册的状态说话。 */
export function ourCompanyNetworkRows(
  entities: readonly GroupLegalEntityRecord[] | null,
  relationships: readonly PartyRelationshipRecord[],
): OurCompanyNetworkRow[] {
  if (entities === null) return [];
  const ours = ourCompanyPartyIds(entities);
  const rows: OurCompanyNetworkRow[] = [];
  for (const partyId of ours) {
    const pinning = entities.filter((entity) => entity.partyId === partyId && ours.has(entity.partyId));
    const named = pinning.find((entity) => entity.partyNameKnown && entity.partyName);
    const views = relationshipsOfParty(partyId, relationships);
    const live = views.filter((view) => liveRelationshipStatuses.has(view.status));
    rows.push({
      partyId,
      partyName: named?.partyName ?? null,
      legalEntityIds: [...new Set(pinning.map((entity) => entity.legalEntityId))].sort(),
      towardIt: countsByRole(live.filter((view) => view.side === 'COUNTERPARTY')),
      fromIt: countsByRole(live.filter((view) => view.side === 'HOLDER')),
      live: live.length,
      candidates: live.filter((view) => view.status === 'CANDIDATE').length,
      views,
    });
  }
  const nameOf = (row: OurCompanyNetworkRow) => row.partyName ?? row.partyId;
  return rows.sort((left, right) => right.live - left.live || nameOf(left).localeCompare(nameOf(right), 'zh-CN'));
}

/** 列表检索：包含匹配名称、参与方标识与公司主体标识。这一册是页面组合出来的，读口没有它，只能在手上这几行里找。 */
export function matchesOurCompanySearch(row: OurCompanyNetworkRow, search: string): boolean {
  const needle = search.trim().toLowerCase();
  if (needle === '') return true;
  return [row.partyName ?? '', row.partyId, ...row.legalEntityIds].some((text) => text.toLowerCase().includes(needle));
}

/**
 * 合作关系册读到了什么程度。这一视图的段数全是从那一册数出来的，名录栏的那一句与右栏的说明都照它说话：整册读到了，数是真数；
 * 只读到第一页，数只是那一页里的；还在读；没读到（未配置、无权、出错）。后两种没有数可说，更不能说「没有」。
 */
export type OurCompanyRelationshipsRead = 'complete' | 'first-page' | 'pending' | 'unread';

/**
 * `answerKind` 是合作关系册不带条件那一问答复的 kind，还没答出传 null；`complete` 是那一问已是整册（complete-register.ts 的
 * completeRegisterRows 判过）。
 */
export function ourCompanyRelationshipsReadOf(answerKind: string | null, complete: boolean): OurCompanyRelationshipsRead {
  if (answerKind === null) return 'pending';
  if (answerKind !== 'outcome') return 'unread';
  return complete ? 'complete' : 'first-page';
}

/** 「10 段在用 · 1 段待确认」：名录栏一行名称下的那一句与右栏关系图段头上的数是同一句。没有待确认的不写后半句。 */
export function ourCompanyLiveText(row: Pick<OurCompanyNetworkRow, 'live' | 'candidates'>): string {
  return `${formatCount(row.live)} 段在用${row.candidates > 0 ? ` · ${formatCount(row.candidates)} 段待确认` : ''}`;
}

/**
 * 名录栏一行名称下的那一句。一段在用的都没有时，整册读到了才说「没有」；只读到第一页就说是第一页里没有——它的关系可能排在
 * 后面几页，把没读到说成没有，人会照着去重登一段已有的关系。
 */
export function ourCompanyRosterLineOf(row: Pick<OurCompanyNetworkRow, 'live' | 'candidates'>, read: OurCompanyRelationshipsRead): string {
  if (read === 'pending') return '正在读取…';
  if (read === 'unread') return '合作关系没读到';
  if (row.live > 0) return ourCompanyLiveText(row);
  return read === 'complete' ? '没有在用的合作关系' : '第一页里没有在用的合作关系';
}

/** 按角色的汇总里一行的小字：这个角色下有待确认的就报几段，没有就说都已生效——在用的只有待确认与已生效两种。 */
export function roleCountNoteOf(count: RoleCount): string {
  return count.candidates > 0 ? `其中 ${formatCount(count.candidates)} 段待确认` : '都已生效';
}
