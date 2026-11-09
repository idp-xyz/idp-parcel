// 治理登记册四册的 fetch 出口（GET /governance-registers?register=…，票
// admin-skeleton-closure-batch/02；接管册 ADR-0155）。形状以
// internal/pilotgovernance/adapters/http 传输层为准，此处只做镜像不虚构。
// 阶段评审查阅仍未开，这里不补一个空请求。
//
// 治理无租户维是设计不是缺列（ADR-0083）：请求上本来就不带租户，端点的准入形是
// 产品实例级注入（隔离读开关启用时由装配侧配置），前端与其他目录页共用同一套
// exchangeMasterData，不为治理另起第二种传输。恢复决定的盘点 jsonb 属详情读法，
// 端点不透出，这里也不虚构。
//
// 读口已迁到 ADR-0144（票 catalogue-read-pagination-rollout/12）：register 是册子选择器，由各册的读口函数带上；翻页走游标，
// 答复带 page。各册点名的可排维与 q 覆盖的列以读口的声明为准（internal/pilotgovernance/ports 的 catalogue_pages.go），
// 这里不抄一份。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';

export type { ApiResult } from '../catalogue-api';

/** 治理登记册一问的条件；register 不在其中，由读口函数加上。 */
export type GovernanceRegisterConditions = Omit<CatalogueConditions, 'selectors'>;

type GovernanceRegister = 'authority-interval' | 'suspension' | 'resumption' | 'takeover';

function registerPath(register: GovernanceRegister, conditions: GovernanceRegisterConditions, after: string | null): string {
  return catalogueRequestPath('/governance-registers', { ...conditions, selectors: { register } }, after);
}

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
  page: CataloguePage;
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
  page: CataloguePage;
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
  page: CataloguePage;
}

/** 游标缺即第一页；不带条件即该册的缺省序第一页。 */
export function listAuthorityIntervals(
  conditions: GovernanceRegisterConditions = {},
  after: string | null = null,
): Promise<ApiResult<AuthorityIntervalListResponseBody>> {
  return exchangeMasterData<AuthorityIntervalListResponseBody>(registerPath('authority-interval', conditions, after));
}

export function listSuspensions(
  conditions: GovernanceRegisterConditions = {},
  after: string | null = null,
): Promise<ApiResult<SuspensionListResponseBody>> {
  return exchangeMasterData<SuspensionListResponseBody>(registerPath('suspension', conditions, after));
}

export function listResumptions(
  conditions: GovernanceRegisterConditions = {},
  after: string | null = null,
): Promise<ApiResult<ResumptionListResponseBody>> {
  return exchangeMasterData<ResumptionListResponseBody>(registerPath('resumption', conditions, after));
}

/** 对象级接管一行。盘点 jsonb 不在列面，只留盘点时刻。toAt 缺席即开放区间。 */
export interface TakeoverRecord {
  objectScope: string;
  capability: string;
  factKind: string;
  authority: string;
  fromAt: string;
  toAt?: string;
  stopEvidence: string;
  acceptedFacts: string;
  pendingExternals: string;
  actualControl: string;
  responsibilities: string;
  nextAction: string;
  inventoryTakenAt: string;
  effectiveAt: string;
}

export interface TakeoverListResponseBody {
  outcome: 'TAKEOVERS_LISTED';
  takeovers: TakeoverRecord[];
  page: CataloguePage;
}

export function listTakeovers(
  conditions: GovernanceRegisterConditions = {},
  after: string | null = null,
): Promise<ApiResult<TakeoverListResponseBody>> {
  return exchangeMasterData<TakeoverListResponseBody>(registerPath('takeover', conditions, after));
}
