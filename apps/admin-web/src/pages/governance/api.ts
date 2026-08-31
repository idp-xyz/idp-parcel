// 治理登记册三册的 fetch 出口（GET /governance-registers?register=…，票
// admin-skeleton-closure-batch/02）。形状以 internal/pilotgovernance/adapters/http
// 传输层为准，此处只做镜像不虚构。
//
// 治理无租户维是设计不是缺列（ADR-0083）：请求上本来就不带租户，端点的准入形是
// 产品实例级注入（隔离读开关启用时由装配侧配置），前端与其他目录页共用同一套
// exchangeMasterData，不为治理另起第二种传输。恢复决定的盘点 jsonb 属详情读法，
// 端点不透出，这里也不虚构。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/** 生产权威区间一行。toAt 缺席即开放区间（当前权威），不编造「无限远」时刻。 */
export interface AuthorityIntervalRecord {
  objectScope: string;
  capability: string;
  factKind: string;
  authority: string;
  fromAt: string;
  toAt?: string;
  insertedAt: string;
}

export interface AuthorityIntervalListResponseBody {
  outcome: 'AUTHORITY_INTERVALS_LISTED';
  intervals: AuthorityIntervalRecord[];
}

/** 暂停决定一行。字段全部是脱敏引用与原词（登记 CLI 纪律保证）。 */
export interface SuspensionRecord {
  suspensionId: string;
  triggerSource: string;
  basis: string;
  evidence: string;
  scope: string;
  executedBy: string;
  occurredAt: string;
  effectiveAt: string;
  inTransitNote: string;
}

export interface SuspensionListResponseBody {
  outcome: 'SUSPENSIONS_LISTED';
  suspensions: SuspensionRecord[];
}

/** 恢复决定一行。suspensionId 指回被恢复的暂停；盘点内容住 jsonb，不在列面。 */
export interface ResumptionRecord {
  suspensionId: string;
  releaseEvidence: string;
  consistencyCheck: string;
  inventoryTakenAt: string;
  decidedBy: string;
  decidedAt: string;
  effectiveAt: string;
}

export interface ResumptionListResponseBody {
  outcome: 'RESUMPTIONS_LISTED';
  resumptions: ResumptionRecord[];
}

export function listAuthorityIntervals(): Promise<ApiResult<AuthorityIntervalListResponseBody>> {
  return exchangeMasterData<AuthorityIntervalListResponseBody>(
    '/governance-registers?register=authority-interval',
  );
}

export function listSuspensions(): Promise<ApiResult<SuspensionListResponseBody>> {
  return exchangeMasterData<SuspensionListResponseBody>(
    '/governance-registers?register=suspension',
  );
}

export function listResumptions(): Promise<ApiResult<ResumptionListResponseBody>> {
  return exchangeMasterData<ResumptionListResponseBody>(
    '/governance-registers?register=resumption',
  );
}
