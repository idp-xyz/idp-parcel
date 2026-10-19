// 身份冲突处置命令口（POST /node-operations/identity-dispositions；UC-NO-004、ADR-0210 决定六，票 no-reception-identity/06）。
//
// 形状以 internal/nodeoperations/adapters/http 的 dispose_identity_conflict.go 为准，此处只做镜像不虚构。写行走 postMasterData：
// 它挂操作者渠道「运营决定」能力面的 Intake，决定种类「身份冲突处置」，与查阅页那一行不是同一个准入。

import { postMasterData, type ApiResult } from '../catalogue-api';
import type { IdentityDispositionRecord } from './records-api';

export type { ApiResult } from '../catalogue-api';

/** 一件连带实物：此刻关联到指定包裹的别的作业实物，带操作者看到的那一版当前有效版本。 */
export interface CoDisposedUnit {
  unit: string;
  version: string;
}

/**
 * 一次处置的载荷。**没有租户、没有操作者、没有时间**——身份从操作者信封来，形成时刻取服务端时刻；服务端对载荷里出现的身份键
 * 一律按未知键拒。parcel 与 coDisposedUnits 只在指定一件（DESIGNATED）时带。
 */
export interface IdentityDispositionRequest {
  unit: string;
  disposedVersion: string;
  result: 'DESIGNATED' | 'NONE_OF_CANDIDATES';
  parcel?: string;
  coDisposedUnits?: CoDisposedUnit[];
  basis: string;
}

/** 答复里的一版判断。excluded 只在处置版上非空，是这一版排除的那一组。 */
export interface JudgmentVersionBody {
  unit: string;
  judgmentVersion: string;
  predecessor?: string;
  kind: string;
  node?: string;
  association?: string;
  candidates?: string[];
  identityConflict?: boolean;
  excluded?: string[];
  serviceMarkers?: string[];
  serviceResultCheck?: string;
}

export interface AssociatedUnitBody {
  unit: string;
  currentVersions: JudgmentVersionBody[];
}

/**
 * 处置口的封闭响应形状。`outcome` 取处置结果枚举原名（IDENTITY_CONFLICT_DISPOSED / EXISTING_RESULT / ALREADY_DISPOSED /
 * VERSION_SUPERSEDED / NO_IDENTITY_CONFLICT / NOT_A_CANDIDATE / ASSOCIATED_ELSEWHERE）。unit 与 disposedVersion 恒在场；其余各格随
 * outcome 在场。
 */
export interface IdentityDispositionResponseBody {
  outcome: string;
  unit: string;
  disposedVersion: string;
  disposition?: IdentityDispositionRecord;
  versions?: JudgmentVersionBody[];
  currentVersions?: JudgmentVersionBody[];
  candidates?: string[];
  associatedUnits?: AssociatedUnitBody[];
  intentVersion?: string;
}

export function disposeIdentityConflict(
  request: IdentityDispositionRequest,
): Promise<ApiResult<IdentityDispositionResponseBody>> {
  return postMasterData<IdentityDispositionResponseBody>('/node-operations/identity-dispositions', request);
}
