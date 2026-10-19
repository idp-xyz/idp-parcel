// 人工重核命令口（POST /node-operations/identity-rechecks；ADR-0199 决定三，票 no-reception-source/03）。
//
// 形状以 internal/nodeoperations/adapters/http 的 recheck_pending_identity.go 为准，此处只做镜像不虚构。写行走
// postMasterData：它挂操作者渠道「运营决定」能力面的 Intake，与查阅页那一行不是同一个准入。

import { postMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/**
 * 一次人工重核的载荷：指名一版判断。**没有租户、没有操作者**——身份从操作者信封来，服务端对载荷里出现的身份键一律
 * 按未知键拒。
 */
export interface IdentityRecheckRequest {
  judgmentVersion: string;
}

/**
 * 重核口的封闭响应形状。`outcome` 取重核结果枚举原名（INTAKE_FORMED / IDENTITY_CONFLICT / STILL_UNIDENTIFIED /
 * EXISTING_RESULT / IDENTITY_CONFLICT_RECORDED / SOURCE_NOT_PRESERVED / NOT_PENDING_IDENTIFICATION）。
 * recheckedVersion 恒在场；judgmentVersion 只随追加了一版的两格出现；currentVersion 与 recordedOutcome 只随已有结果
 * 出现。其余几格说的是答复所指的那一版。
 */
export interface IdentityRecheckResponseBody {
  outcome: string;
  recheckedVersion: string;
  judgmentVersion?: string;
  currentVersion?: string;
  recordedOutcome?: string;
  unit?: string;
  node?: string;
  association?: string;
  candidates?: string[];
  identityConflict?: boolean;
  serviceMarkers?: string[];
  serviceResultCheck?: string;
}

export function recheckPendingIdentity(
  request: IdentityRecheckRequest,
): Promise<ApiResult<IdentityRecheckResponseBody>> {
  return postMasterData<IdentityRecheckResponseBody>('/node-operations/identity-rechecks', request);
}
