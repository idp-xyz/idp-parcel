import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import { invalidateWhenAnswered } from '../../query/answers';

// 操作者册的读面与三个写面（ADR-0363；票 operator-access-administration/02）。形状以票 01 的契约为准：租户只取自信封，载荷不带
// tenantId；答复沿用 OperatorRegistrationOutcome 代数，RECORDED 答 201、其余 200。

export const accessAnswersDomain = 'access';

export interface OperatorGrantRevocationRecord {
  revokedAt: string;
  basis: string;
}

export interface OperatorGrantRecord {
  grantId: string;
  capabilityFace: string;
  decisionKind?: string;
  workScope?: { kind: string; reference: string };
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  basis: string;
  recordedAt: string;
  revocation?: OperatorGrantRevocationRecord;
}

export interface OperatorRecord {
  issuer: string;
  subject: string;
  basis: string;
  recordedAt: string;
  grants: OperatorGrantRecord[];
}

export interface OperatorListResponseBody {
  outcome: 'OPERATORS_LISTED';
  /** 本进程信任的发行方：新增操作者时预填，不让人抄。 */
  issuer: string;
  /** 产品词表：可授能力面、决定种类、作业范围种类；页面据此摆选项，不抄第二份。 */
  faces: string[];
  decisionKinds: string[];
  workScopeKinds: string[];
  operators: OperatorRecord[];
}

export interface OperatorWriteResponseBody {
  outcome: string;
}

export function listOperators(): Promise<ApiResult<OperatorListResponseBody>> {
  return exchangeMasterData<OperatorListResponseBody>('/operators');
}

/** 写口答到之后整域作废（ADR-0360 决定三的做法）：操作者页的列表随之重问。授予自查（use-operator-grants.ts）是一页一问的模块级缓存，不在此域，换授予要整页重载。 */
function afterAccessWrite<Answer extends { kind: string }>(request: Promise<Answer>): Promise<Answer> {
  return invalidateWhenAnswered([accessAnswersDomain], request);
}

export interface OperatorRegistrationPayload {
  issuer: string;
  subject: string;
  basis: string;
}

export function registerOperator(payload: OperatorRegistrationPayload): Promise<ApiResult<OperatorWriteResponseBody>> {
  return afterAccessWrite(postMasterData<OperatorWriteResponseBody>('/operator-registrations', payload));
}

export interface OperatorGrantPayload {
  grantId: string;
  issuer: string;
  subject: string;
  capabilityFace: string;
  decisionKind?: string;
  workScope?: { kind: string; reference: string };
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  basis: string;
}

export function registerOperatorGrant(payload: OperatorGrantPayload): Promise<ApiResult<OperatorWriteResponseBody>> {
  return afterAccessWrite(postMasterData<OperatorWriteResponseBody>('/operator-grant-registrations', payload));
}

export interface OperatorGrantRevocationPayload {
  grantId: string;
  revokedAt: string;
  basis: string;
}

export function revokeOperatorGrant(payload: OperatorGrantRevocationPayload): Promise<ApiResult<OperatorWriteResponseBody>> {
  return afterAccessWrite(postMasterData<OperatorWriteResponseBody>('/operator-grant-revocations', payload));
}
