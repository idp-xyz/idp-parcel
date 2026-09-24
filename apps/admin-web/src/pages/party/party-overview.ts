// 参与方全景的判读（票 operator-workspace-gaps/01）：打开一个参与方时回答「它对我们是什么」——它在哪些关系里、有没有货主
// 客户账户、是不是本集团的责任法人、签了哪些供应商协议、适用哪些结算政策。各段只按参与方标识在已取回的行上关联，不下推
// 查询参数、不改端点（README 列表页上列通则）；全景只是读取时的组合，不是第二套口径。纯函数，node:test 钉着；组件
// PartyOverviewSection.tsx 只负责摆。

import type { ApiResult } from '../catalogue-api';
import type {
  CustomerAccountRecord,
  GroupLegalEntityRecord,
  PartyRelationshipRecord,
  SettlementPolicyRecord,
  SupplierAgreementRecord,
} from './api';
import { byInstant, byString } from './list-order';
import { partyRoleLabels } from './presentation';

/** 本方在一段关系里站哪一侧。方向由关系册「持有方 → 相对方」的字段次序表达（CONTEXT），这里只读次序，不解释成谁是谁的客户。 */
export type RelationshipSide = 'HOLDER' | 'COUNTERPARTY';

export interface PartyRelationshipView {
  relationshipId: string;
  revision: number;
  side: RelationshipSide;
  role: string;
  otherPartyId: string;
  otherPartyName?: string;
  otherPartyNameKnown: boolean;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
}

function viewOf(row: PartyRelationshipRecord, side: RelationshipSide): PartyRelationshipView {
  const holder = side === 'HOLDER';
  return {
    relationshipId: row.relationshipId,
    revision: row.revision,
    side,
    role: row.role,
    otherPartyId: holder ? row.counterpartyId : row.holderId,
    otherPartyName: holder ? row.counterpartyName : row.holderName,
    otherPartyNameKnown: holder ? row.counterpartyNameKnown : row.holderNameKnown,
    scope: row.scope,
    status: row.status,
    effectiveStartsAt: row.effectiveStartsAt,
    effectiveEndsAt: row.effectiveEndsAt,
  };
}

/** 本方作持有方或相对方的那些关系，按生效起点新→旧，同刻按关系标识——与关系签的默认排序同一口径。 */
export function relationshipsOfParty(
  partyId: string,
  relationships: readonly PartyRelationshipRecord[],
): PartyRelationshipView[] {
  const views: PartyRelationshipView[] = [];
  for (const row of relationships) {
    if (row.holderId === partyId) views.push(viewOf(row, 'HOLDER'));
    else if (row.counterpartyId === partyId) views.push(viewOf(row, 'COUNTERPARTY'));
  }
  return views.sort(
    (left, right) =>
      byInstant(right.effectiveStartsAt, left.effectiveStartsAt) || byString(left.relationshipId, right.relationshipId),
  );
}

export interface EffectiveRoles {
  /** 本方作持有方、状态为已生效的关系里出现的角色码。 */
  asHolder: string[];
  /** 本方作相对方、状态为已生效的关系里出现的角色码。 */
  asCounterparty: string[];
}

// 词表的键序就是 CONTEXT 列举角色的顺序；服务端新增一格时词表里没有它，排在后面原样保留，不归进某个已知角色。
const roleOrder: readonly string[] = Object.keys(partyRoleLabels);

function orderedRoles(roles: Iterable<string>): string[] {
  const rank = (role: string) => {
    const index = roleOrder.indexOf(role);
    return index < 0 ? roleOrder.length : index;
  };
  return [...new Set(roles)].sort((left, right) => rank(left) - rank(right) || byString(left, right));
}

/**
 * 「它对我们是什么」的一行答案：只看关系册登记为已生效的关系。关系状态是登记进来的事实，不随装载时钟走，所以这里不拿
 * 有效区间去推「此刻是否有效」；候选、到期、撤销、替代的关系照旧列在关系段里，只是不进这行摘要。
 */
export function effectiveRolesOf(views: readonly PartyRelationshipView[]): EffectiveRoles {
  const effective = views.filter((view) => view.status === 'EFFECTIVE');
  return {
    asHolder: orderedRoles(effective.filter((view) => view.side === 'HOLDER').map((view) => view.role)),
    asCounterparty: orderedRoles(effective.filter((view) => view.side === 'COUNTERPARTY').map((view) => view.role)),
  };
}

const byObjectVersion = (
  left: { objectId: string; version: string },
  right: { objectId: string; version: string },
) => byString(left.objectId, right.objectId) || byString(left.version, right.version);

/**
 * 供应商是本方的协议版本。供应商在正文里（领域 `SupplierAgreementBody.Supplier` 是 `PartyID`），正文未登记的版本壳没有这一格，
 * 归不进任何参与方——如实不列，不拿壳上别的格去猜它是谁的。
 */
export function supplierAgreementsOfParty(
  partyId: string,
  agreements: readonly SupplierAgreementRecord[],
): SupplierAgreementRecord[] {
  return agreements
    .filter((row) => row.contentRegistered && row.supplier === partyId)
    .sort(byObjectVersion);
}

/** 关联到本方客户参与方的货主客户账户（ADR-0003：账户必须显式关联其客户参与方）。停用的照列，状态由页面按词表显。 */
export function customerAccountsOfParty(
  partyId: string,
  accounts: readonly CustomerAccountRecord[],
): CustomerAccountRecord[] {
  return accounts
    .filter((row) => row.customerPartyId === partyId)
    .sort((left, right) => byString(left.accountId, right.accountId));
}

/** 钉着本方参与方身份的责任法人——即本方是不是本集团的责任法人（CONTEXT：责任法人同时具有业务参与方身份）。 */
export function legalEntitiesOfParty(
  partyId: string,
  entities: readonly GroupLegalEntityRecord[],
): GroupLegalEntityRecord[] {
  return entities
    .filter((row) => row.partyId === partyId)
    .sort((left, right) => byString(left.legalEntityId, right.legalEntityId));
}

/**
 * 客户相对方是本方的结算政策。结算政策六维里的客户相对方是承担结算责任的业务参与方、不是货主客户账户（PC CONTEXT），
 * 所以按 counterparty 关联；同一货主可以同时适用多份不重叠的政策，全列。
 */
export function settlementPoliciesOfParty(
  partyId: string,
  policies: readonly SettlementPolicyRecord[],
): SettlementPolicyRecord[] {
  return policies.filter((row) => row.counterparty === partyId).sort(byObjectVersion);
}

/**
 * 全景一段的状态。每段各取各的读口，一段未配置或出错不牵连别段；`mismatch` 是答案回显的册种与所问不符——丢弃，
 * 不拿它当「册上没有」，两句续办相反（前者查前后端版本，后者去登记）。
 */
export type OverviewSectionState<Row> =
  | { kind: 'loading' }
  | { kind: 'answered'; rows: Row[] }
  | { kind: 'mismatch' }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string; detail?: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

/**
 * 读口结果 → 段状态。`rowsOf` 在业务答案上做本段的关联，答案不回答本段所问时交回 null。
 */
export function sectionOf<Body, Row>(
  answer: ApiResult<Body> | null,
  rowsOf: (body: Body) => Row[] | null,
): OverviewSectionState<Row> {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind !== 'outcome') return answer;
  const rows = rowsOf(answer.body);
  return rows === null ? { kind: 'mismatch' } : { kind: 'answered', rows };
}

/** 段首与顶部摘要的计数：只在拿到业务答案后报数，零条报 0；其余各态报「—」，不拿 0 冒充「没有」（README 列表页上列通则第六条）。 */
export function countText(state: OverviewSectionState<unknown>): string {
  return state.kind === 'answered' ? String(state.rows.length) : '—';
}
