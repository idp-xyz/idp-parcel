// 外部承运轨迹事实有效时间判断页的读写面（票 label-channel/21）：
// GET /transport-fulfillment-external-tracking-facts?source=…&view=pending|current 与
// POST /transport-fulfillment-effective-time-judgments。
//
// 形状以 internal/transportfulfillment/adapters/http 的 query_external_tracking_facts.go 与
// judge_effective_time.go 为准，此处只做镜像不虚构。传输与五格判读收敛在共享 catalogue-api；
// 读行与写行分成两个函数，读走 exchangeMasterData、写走 postMasterData——两行的 Intake 不是同一个，
// 隔离读准入（ADR-0078）换得了读行换不了写行，调用点长得一样会让这条区别在阅读时消失。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/** 视图封闭两格，与传输层 ?view= 同词：待判断的当前版 / 该源全部当前版。 */
export type ExternalTrackingFactView = 'pending' | 'current';

/**
 * 一条当前版。三个时间三键归属不同（ADR-0102）：occurredAt 源给、receivedAt 本上下文铸、
 * effectiveAt 只在判断过时在场——缺席就是「还没有人判」，页面不拿另两个时间顶上。
 * effectiveRule 两键只随按规则判断在场；supersedes 首版缺席；sourceEvent 源未给即缺席。
 * status 是源的原始状态词，原样示出不解释（ADR-0102 决定五）。
 */
export interface ExternalTrackingFactRecord {
  fact: string;
  version: string;
  source: string;
  credential: string;
  object: string;
  sourceEvent?: string;
  status: string;
  occurredAt: string;
  receivedAt: string;
  effectiveBasis: string;
  effectiveAt?: string;
  effectiveRule?: string;
  effectiveRuleVersion?: string;
  supersedes?: string;
  origin: string;
  recordedAt: string;
}

export type ExternalTrackingFactListResponseBody =
  | { outcome: 'PENDING_EFFECTIVE_TIME_FACTS_LISTED'; facts: ExternalTrackingFactRecord[] }
  | { outcome: 'CURRENT_EXTERNAL_TRACKING_FACTS_LISTED'; facts: ExternalTrackingFactRecord[] };

/** 源与视图都是必备维——缺席在服务端是 400，本函数不替调用方兜。 */
export function listExternalTrackingFacts(
  source: string,
  view: ExternalTrackingFactView,
): Promise<ApiResult<ExternalTrackingFactListResponseBody>> {
  return exchangeMasterData<ExternalTrackingFactListResponseBody>(
    `/transport-fulfillment-external-tracking-facts?source=${encodeURIComponent(source)}&view=${view}`,
  );
}

/**
 * 一次判断的载荷：指名哪条事实、从何时起有效（RFC 3339）。**没有租户、没有操作者**——身份从
 * ADR-0100 的操作者信封来，服务端对载荷里出现的身份键一律按未知键拒。
 */
export interface EffectiveTimeJudgmentRequest {
  fact: string;
  effectiveAt: string;
}

/**
 * 判断口的封闭响应形状。`outcome` 取应用结果枚举原名（EFFECTIVE_TIME_JUDGED / ALREADY_JUDGED_AS_GIVEN /
 * INPUT_NOT_ACCEPTED / JUDGMENT_UNDECIDED）；带记录的两格把那一版原样带回——`已按同值判过`带回的是
 * 既有判断版本及其依据；handoffReference 只在新版本已登记而意图没交出去时在场。
 */
export interface EffectiveTimeJudgmentResponseBody {
  outcome: string;
  undecidedReason?: string;
  continuationReference?: string;
  handoffReference?: string;
  fact?: string;
  version?: string;
  source?: string;
  credential?: string;
  object?: string;
  sourceEvent?: string;
  status?: string;
  occurredAt?: string;
  receivedAt?: string;
  effectiveBasis?: string;
  effectiveAt?: string;
  effectiveRule?: string;
  effectiveRuleVersion?: string;
  supersedes?: string;
  recordedAt?: string;
}

export function judgeEffectiveTime(
  request: EffectiveTimeJudgmentRequest,
): Promise<ApiResult<EffectiveTimeJudgmentResponseBody>> {
  return postMasterData<EffectiveTimeJudgmentResponseBody>(
    '/transport-fulfillment-effective-time-judgments',
    request,
  );
}
