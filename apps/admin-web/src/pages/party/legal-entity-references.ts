// 公司主体首屏指标带与「关联」签的判读（票 company-subject-ux/03）：打开一家公司时回答「谁在用它」——哪些供应商协议、
// 结算政策、信用政策、接单规则包以它为责任法人——以及首屏「资料状态」「被引用」两格各取什么值。做法照 party-overview.ts：
// 只按法人标识在已取回的行上关联，不下推查询参数、不改端点；取不到的那一册如实说读不到，不拿 0 冒充「没有」。纯函数，
// node:test 钉着；组件 LegalEntityReferencesSection.tsx 只负责摆。
//
// 客户合同与客户账户不在这里：两册的读口上都没有责任法人一格（customerContractBody 不透出法人，账户只钉客户参与方），
// 在已取回的行上关联不出来，列一行永远的「—」只是噪音。合同经结算政策的「合同」一格间接可见；后端透出之后再接。

import type { ApiResult } from '../catalogue-api';
import type {
  BusinessPartyListResponseBody,
  CommercialPolicyKind,
  CreditPolicyRecord,
  LegalEntityProfileResolutionResponseBody,
  RulePackageRecord,
  SettlementPolicyRecord,
  SupplierAgreementRecord,
} from './api';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import { policiesHash } from './commercial-policies-address';
import { byString } from './list-order';
import { legalEntityProfileStatusOf, type LegalEntityProfileStatus } from './legal-entity-profile';
import { overviewSectionHash, type OverviewSectionState } from './party-overview';
import { updateOrdinalLabel } from './presentation';

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

/** 责任法人是这家公司的信用政策版本（授信以法人为单位，ADR-0129）。 */
export function creditPoliciesOfLegalEntity(
  legalEntityId: string,
  policies: readonly CreditPolicyRecord[],
): CreditPolicyRecord[] {
  return policies.filter((row) => row.legalEntity === legalEntityId).sort(byObjectVersion);
}

/** 责任法人是这家公司的接单规则包版本（规则包绑定合同与法人）。 */
export function rulePackagesOfLegalEntity(
  legalEntityId: string,
  packages: readonly RulePackageRecord[],
): RulePackageRecord[] {
  return packages.filter((row) => row.legalEntity === legalEntityId).sort(byObjectVersion);
}

/** 在用版本数：只看服务端答的状态是不是已生效，不在前端按时刻另算（口径同 version-groups.ts）。 */
export function inUseCount(rows: readonly { status: string }[]): number {
  return rows.filter((row) => row.status === 'EFFECTIVE').length;
}

/**
 * 一册的段首一行：「供应商协议 4 · 在用 2」。没有状态的册（三本政策册）只报总数；读不到的册说「读不到」，不显 0——
 * 0 是「册上没有」，读不到是「不知道」，两句的续办相反。
 */
export function referenceLineText(title: string, state: OverviewSectionState<{ status?: string }>, withInUse: boolean): string {
  if (state.kind === 'loading') return `${title} …`;
  if (state.kind !== 'answered') return `${title} 读不到`;
  const total = `${title} ${state.rows.length}`;
  if (!withInUse) return total;
  return `${total} · 在用 ${inUseCount(state.rows.map((row) => ({ status: row.status ?? '' })))}`;
}

/** 四册各自的段状态；首屏与「关联」签用同一份。 */
export interface LegalEntityReferenceStates {
  agreements: OverviewSectionState<SupplierAgreementRecord>;
  settlement: OverviewSectionState<SettlementPolicyRecord>;
  credit: OverviewSectionState<CreditPolicyRecord>;
  rulePackages: OverviewSectionState<RulePackageRecord>;
}

/**
 * 首屏「被引用」一格：供应商协议的在用版本数加三本政策册的版本数。政策册的读面没有状态（version-groups.ts：那几册只排不筛），
 * 「在用」判不了，登在册上的每一版都是一次引用，就按版本数计。任一册还没读到或读不到答 null，页面显「—」——
 * 少一册的和不是和。
 */
export function referencedCount(states: LegalEntityReferenceStates): number | null {
  const { agreements, settlement, credit, rulePackages } = states;
  if (
    agreements.kind !== 'answered' ||
    settlement.kind !== 'answered' ||
    credit.kind !== 'answered' ||
    rulePackages.kind !== 'answered'
  ) {
    return null;
  }
  return inUseCount(agreements.rows) + settlement.rows.length + credit.rows.length + rulePackages.rows.length;
}

export function referencedCountText(count: number | null): string {
  return count === null ? '—' : String(count);
}

/** 「被引用」格悬停那句，说清口径——三本政策册按版本数计，不是在用数。 */
export const referencedCountNote = '供应商协议的在用版本，加结算政策、信用政策、接单规则包的版本数；任一册读不到显「—」';

/** 按参与方标识查名称；查不到交回 undefined，行上照显标识。 */
export type PartyNameOf = (partyId: string) => string | undefined;

/**
 * 行上的供应商与客户是参与方标识，经往来单位册译成名称（票 company-subject-ux/07）。册没读到、读不到、或册上没有这个
 * 标识一律查不到——名称是帮人认的，译不到只是这一次没译成，不是引用错了，所以不报错、不显占位。名为空串的也当查不到：
 * 空名上屏等于把标识藏掉。
 */
export function partyNameOfAnswer(answer: ApiResult<BusinessPartyListResponseBody> | null): PartyNameOf {
  if (answer === null || answer.kind !== 'outcome') return () => undefined;
  const names = new Map(answer.body.parties.map((party) => [party.partyId, party.partyName]));
  return (partyId) => names.get(partyId) || undefined;
}

/**
 * 版本标识给人看的写法：`v1` 拆成「第 1 版」放状态旁，主键只留对象标识（它才是对象名）。版本标识在领域里只要求非空
 * （CommercialVersionLabel），不限 `v` 加数字；拆不出序数的照显原串，不硬编一个。
 */
export function versionOrdinalText(version: string): string {
  const match = /^v(\d+)$/i.exec(version.trim());
  return match ? updateOrdinalLabel(Number(match[1])) : version;
}

export type ReferenceRowKind = 'supplier-agreement' | 'settlement-policy' | 'credit-policy' | 'rule-package';

const policyRegisterOf: Partial<Record<ReferenceRowKind, CommercialPolicyKind>> = {
  'settlement-policy': 'SETTLEMENT_POLICY',
  'credit-policy': 'CREDIT_POLICY',
  'rule-package': 'ACCEPTANCE_RULE_PACKAGE',
};

/**
 * 点一行的去处：对象所在的页，带上对象标识作检索词（机制同 party-overview.ts 的 overviewSectionHash）。三本政策册都在
 * 商业规则页，地址按册定位再带词。
 */
export function referenceRowHash(kind: ReferenceRowKind, objectId: string): string {
  const register = policyRegisterOf[kind];
  if (register === undefined) return overviewSectionHash('supplier-agreements', objectId);
  return hashWithQueryValue(policiesHash('#/commercial-policies', { tab: 'registers', register }), ADDRESS_KEYWORD_PARAM, objectId);
}

/** 一册的段首「去「…」」的去处：带这家公司的标识作检索词，目标页筛到以它为责任法人的那几行。 */
export function referenceRegisterHash(kind: ReferenceRowKind, legalEntityId: string): string {
  return referenceRowHash(kind, legalEntityId);
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
