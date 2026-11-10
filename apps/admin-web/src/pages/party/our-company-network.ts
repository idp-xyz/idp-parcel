// 往来单位页「我方关联」一册的判读（票 business-party-relationship-graph/02）。纯函数，node:test 钉着；页面与
// OurCompanyNetworkPanel.tsx 只负责摆。
//
// 这一册以我方公司为主体：每一家我方公司一行，数出谁对它担任什么角色、它又对谁担任什么角色，选中一行在右栏看以它为中心的
// 关系图。「我方公司」与往来单位列表类型格同一口径（business-party-list.ts 的 ourCompanyPartyIds：法人册上在用的公司主体钉着的
// 参与方）；关系只数还在起作用的段（party-overview.ts 的 liveRelationshipStatuses），与关系图同一条。只是读取时的组合，不是第二本册。

import type { GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import { ourCompanyPartyIds } from './business-party-list';
import { liveRelationshipStatuses, relationshipsOfParty, type PartyRelationshipView } from './party-overview';
import { partyRoleLabels } from './presentation';

export interface RoleCount {
  role: string;
  count: number;
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
  const counts = new Map<string, number>();
  for (const view of views) counts.set(view.role, (counts.get(view.role) ?? 0) + 1);
  const rank = (role: string) => (roleOrder.includes(role) ? roleOrder.indexOf(role) : roleOrder.length);
  return [...counts.entries()]
    .map(([role, count]) => ({ role, count }))
    .sort((left, right) => rank(left.role) - rank(right.role) || left.role.localeCompare(right.role));
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
