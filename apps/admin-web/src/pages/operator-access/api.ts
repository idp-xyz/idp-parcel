import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';

// 操作者册的读面与三个写面（ADR-0363；票 operator-access-administration/02）。形状以票 01 的契约为准：租户只取自信封，载荷不带
// tenantId；答复沿用 OperatorRegistrationOutcome 代数，RECORDED 答 201、其余 200。
//
// 读面已迁到 ADR-0144（票 catalogue-read-pagination-rollout/13）：翻页走游标，答复带 page；没有筛选维，q 只认主体标识与绑定
// 依据（accessidentity 的 OperatorCataloguePages）。列表按游标取、不经取数层（ADR-0360），写口答到之后由页面重取当前这一页。

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
  page: CataloguePage;
}

export interface OperatorWriteResponseBody {
  outcome: string;
}

/** 游标缺即第一页；不带条件即缺省序（登记先后）的第一页。 */
export function listOperators(
  conditions: CatalogueConditions = {},
  after: string | null = null,
): Promise<ApiResult<OperatorListResponseBody>> {
  return exchangeMasterData<OperatorListResponseBody>(catalogueRequestPath('/operators', conditions, after));
}

export interface OperatorRegistrationPayload {
  issuer: string;
  subject: string;
  basis: string;
}

export function registerOperator(payload: OperatorRegistrationPayload): Promise<ApiResult<OperatorWriteResponseBody>> {
  return postMasterData<OperatorWriteResponseBody>('/operator-registrations', payload);
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
  return postMasterData<OperatorWriteResponseBody>('/operator-grant-registrations', payload);
}

export interface OperatorGrantRevocationPayload {
  grantId: string;
  revokedAt: string;
  basis: string;
}

export function revokeOperatorGrant(payload: OperatorGrantRevocationPayload): Promise<ApiResult<OperatorWriteResponseBody>> {
  return postMasterData<OperatorWriteResponseBody>('/operator-grant-revocations', payload);
}
