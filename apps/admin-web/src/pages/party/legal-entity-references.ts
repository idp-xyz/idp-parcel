// 公司主体首屏指标带与「关联」签的判读（票 company-subject-ux/03）：打开一家公司时回答「谁在用它」——哪些供应商协议、
// 哪些结算政策以它为责任法人——以及首屏三格（生效日期、资料状态、被引用）各取什么值。做法照 party-overview.ts：只按法人
// 标识在已取回的行上关联，不下推查询参数、不改端点；取不到的那一册如实说读不到，不拿 0 冒充「没有」。纯函数，node:test
// 钉着；组件 LegalEntityReferencesSection.tsx 只负责摆。
//
// 客户合同与客户账户不在这里：两册的读口上都没有责任法人一格（customerContractBody 不透出法人，账户只钉客户参与方），
// 在已取回的行上关联不出来，列一行永远的「—」只是噪音。合同经结算政策的「合同」一格间接可见。

import type { ApiResult } from '../catalogue-api';
import type { LegalEntityProfileResolutionResponseBody, SettlementPolicyRecord, SupplierAgreementRecord } from './api';
import { byString } from './list-order';
import { legalEntityProfileStatusOf, type LegalEntityProfileStatus } from './legal-entity-profile';
import { overviewSectionHash, type OverviewSectionState } from './party-overview';

const byObjectVersion = (
  left: { objectId: string; version: string },
  right: { objectId: string; version: string },
) => byString(left.objectId, right.objectId) || byString(left.version, right.version);

/**
 * 责任法人是这家公司的协议版本。法人在正文里（领域 `SupplierAgreementBody.LegalEntity`），正文未登记的版本壳没有这一格，
 * 归不进任何公司——如实不列，与 supplierAgreementsOfParty 同一条理由。
 */
export function supplierAgreementsOfLegalEntity(
  legalEntityId: string,
  agreements: readonly SupplierAgreementRecord[],
): SupplierAgreementRecord[] {
  return agreements
    .filter((row) => row.contentRegistered && row.legalEntity === legalEntityId)
    .sort(byObjectVersion);
}

/** 责任法人是这家公司的结算政策版本（六维之一是责任法人，PC CONTEXT）；同一公司可同时有多份，全列。 */
export function settlementPoliciesOfLegalEntity(
  legalEntityId: string,
  policies: readonly SettlementPolicyRecord[],
): SettlementPolicyRecord[] {
  return policies.filter((row) => row.legalEntity === legalEntityId).sort(byObjectVersion);
}

/** 在用版本数：只看服务端答的状态是不是已生效，不在前端按时刻另算（口径同 version-groups.ts）。 */
export function inUseCount(rows: readonly { status: string }[]): number {
  return rows.filter((row) => row.status === 'EFFECTIVE').length;
}

/**
 * 一册的段首一行：「供应商协议 4 · 在用 2」。没有状态的册（结算政策）只报总数；读不到的册说「读不到」，不显 0——
 * 0 是「册上没有」，读不到是「不知道」，两句的续办相反。
 */
export function referenceLineText(title: string, state: OverviewSectionState<{ status?: string }>, withInUse: boolean): string {
  if (state.kind === 'loading') return `${title} …`;
  if (state.kind !== 'answered') return `${title} 读不到`;
  const total = `${title} ${state.rows.length}`;
  if (!withInUse) return total;
  return `${total} · 在用 ${inUseCount(state.rows.map((row) => ({ status: row.status ?? '' })))}`;
}

/**
 * 首屏「被引用」一格：供应商协议的在用版本数加结算政策的版本数。结算政策的读面没有状态（version-groups.ts：那几册只排不筛），
 * 「在用」判不了，登在册上的每一版都是一次引用，就按版本数计。任一册还没读到或读不到答 null，页面显「—」——
 * 少一册的和不是和。
 */
export function referencedCount(
  agreements: OverviewSectionState<SupplierAgreementRecord>,
  policies: OverviewSectionState<SettlementPolicyRecord>,
): number | null {
  if (agreements.kind !== 'answered' || policies.kind !== 'answered') return null;
  return inUseCount(agreements.rows) + policies.rows.length;
}

export function referencedCountText(count: number | null): string {
  return count === null ? '—' : String(count);
}

export type ReferenceRowKind = 'supplier-agreement' | 'settlement-policy';

/** 点一行的去处：对象所在的页，带上对象标识作检索词（机制同 party-overview.ts 的 overviewSectionHash）。 */
export function referenceRowHash(kind: ReferenceRowKind, objectId: string): string {
  return overviewSectionHash(kind === 'supplier-agreement' ? 'supplier-agreements' : 'commercial-policies', objectId);
}

/** 一册的段首「去「…」」的去处：带这家公司的标识作检索词，目标页筛到以它为责任法人的那几行。 */
export function referenceRegisterHash(kind: ReferenceRowKind, legalEntityId: string): string {
  return referenceRowHash(kind, legalEntityId);
}

/**
 * 主数据页的时间显示到日期（spec「统一用语」：生效 / 登记 / 停用显示到日期，完整时刻放 title）。按呈现时区取墙钟日，
 * 不拿 ISO 串的前十位——UTC 的 2026-01-01T20:00Z 在上海已是 1 月 2 日。解析不了的串原样交回，理由同 formatInstant。
 */
export function displayDateOf(value: string, timeZone: string): string {
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) return value;
  const parts = new Intl.DateTimeFormat('en-US', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(instant);
  const read = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? '';
  return `${read('year')}-${read('month')}-${read('day')}`;
}

/**
 * 首屏「资料状态」一格。词与色调由 legalEntityProfileStatusOf 出（票 company-subject-ux/04 改词，这里只调用）；
 * 还没读到显占位，读不到或答的不是这家公司显「—」——不拿别家的资料状态、也不拿读失败冒充「未登记资料」。
 */
export type ProfileStatusCell =
  | { kind: 'pending' }
  | { kind: 'unavailable' }
  | { kind: 'status'; status: LegalEntityProfileStatus };

export function profileStatusCellOf(
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null,
  legalEntityId: string,
): ProfileStatusCell {
  if (answer === null) return { kind: 'pending' };
  if (answer.kind !== 'outcome' || answer.body.legalEntityId !== legalEntityId) return { kind: 'unavailable' };
  return { kind: 'status', status: legalEntityProfileStatusOf(answer.body) };
}
