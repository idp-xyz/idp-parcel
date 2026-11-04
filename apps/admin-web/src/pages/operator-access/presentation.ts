// 操作者页的判读与文案（票 operator-access-administration/02）。纯函数，node:test 钉着；组件只摆。

import type { OperatorGrantRecord, OperatorRecord } from './api';

/** 能力面的中文，原词取自 accessidentity.CapabilityFace 各常量；词表之外的码原样显。 */
export const capabilityFaceLabels: Record<string, string> = {
  REGISTRY_CONFIGURATION_WRITE: '登记册配置写',
  MASTER_DATA_AND_OPERATIONS_READ: '主数据与运营查阅读',
  OPERATION_DECISION: '运营决定',
  WORK_FACT_REGISTRATION: '作业事实登记',
  OPERATOR_ACCESS_ADMINISTRATION: '操作者接入管理',
};

export const workScopeKindLabels: Record<string, string> = {
  NODE: '网点',
  OFFSITE: '场外',
};

export function faceLabel(face: string): string {
  return capabilityFaceLabels[face] ?? face;
}

/** 一条授予的细分格：运营决定按决定种类、作业事实按作业范围；别的面没有细分。 */
export function grantQualifier(grant: Pick<OperatorGrantRecord, 'decisionKind' | 'workScope'>): string {
  if (grant.decisionKind) return grant.decisionKind;
  if (grant.workScope) return `${workScopeKindLabels[grant.workScope.kind] ?? grant.workScope.kind} ${grant.workScope.reference}`;
  return '';
}

export type GrantStanding = 'effective' | 'pending' | 'expired' | 'revoked';

export const grantStandingLabels: Record<GrantStanding, string> = {
  effective: '生效中',
  pending: '未到生效起点',
  expired: '已到期',
  revoked: '已撤销',
};

/**
 * 授予此刻的状态，照 accessidentity 的 RecordedGrant.EffectiveAt 判：区间含起点不含终点；撤销自撤销时刻起不再生效。
 * 已撤销但撤销时刻还没到的，此刻仍按区间判；撤销时刻已过即「已撤销」。
 */
export function grantStandingAt(grant: OperatorGrantRecord, now: Date): GrantStanding {
  const at = now.getTime();
  if (grant.revocation && Date.parse(grant.revocation.revokedAt) <= at) return 'revoked';
  if (at < Date.parse(grant.effectiveStartsAt)) return 'pending';
  if (grant.effectiveEndsAt && at >= Date.parse(grant.effectiveEndsAt)) return 'expired';
  return 'effective';
}

export function effectiveGrantCount(operator: OperatorRecord, now: Date): number {
  return operator.grants.filter((grant) => grantStandingAt(grant, now) === 'effective').length;
}

/** 检索：主体、依据、各条授予的面与细分格都搜得到。 */
export function operatorSearchText(operator: OperatorRecord): string {
  return [operator.subject, operator.basis, ...operator.grants.flatMap((grant) => [grant.grantId, grant.capabilityFace, faceLabel(grant.capabilityFace), grantQualifier(grant)])]
    .join(' ')
    .toLowerCase();
}

/** 写口答复的中文；代数原词取自 accessidentity.OperatorRegistrationOutcome 与 ADR-0363 新增的那一格。 */
export const operatorWriteOutcomeLabels: Record<string, string> = {
  RECORDED: '已登记',
  ALREADY_REGISTERED: '已在册（同内容重放，未改动）',
  CONTENT_CONFLICT: '同一键已在册、内容不同：册上原样不动，要改请换标识',
  SUBJECT_BOUND_TO_ANOTHER_TENANT: '这个主体已绑定别的租户，不能再绑本租户',
  OPERATOR_NOT_REGISTERED: '操作者未在册：先登记操作者，再授予',
  GRANT_NOT_REGISTERED: '这条授予不在册，无法撤销',
  REVOCATION_WOULD_LOCK_OUT_CALLER: '不能撤销你自己此刻生效的操作者接入管理授予：那会把你自己锁在外面',
};

export function operatorWriteOutcomeLabel(outcome: string): string {
  return operatorWriteOutcomeLabels[outcome] ?? outcome;
}

/** 主体标识是不透明串，表里显头尾；全串在详情与悬停里。 */
export function shortSubject(subject: string): string {
  return subject.length <= 18 ? subject : `${subject.slice(0, 10)}…${subject.slice(-6)}`;
}
