// 本目录 fetch 出口:价卡目录与计价参考序列查阅(ADR-0077、票 master-data-wiring/02)。
// 形状以 internal/parcelpricing/adapters/http 传输层为准,此处只做镜像不虚构。
//
// 响应判读按 ADR-0022:HTTP 状态码只回答「服务端有没有形成答案」,业务判别一律在
// 响应体的 `outcome`。五格判别与全程追踪页同款,传输实现收敛在共享 catalogue-api
// (装配侧只在 bootstrap 配置那一处前缀),本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

export interface PriceCardRecord {
  planId: string;
  planVersion: string;
  direction: string;
  purpose: string;
  scope: string;
  rateTableId: string;
  rateTableVersion: string;
  effectiveFrom: string;
  effectiveTo?: string;
  canonicalization: string;
  contentDigest: string;
  sourceFileName: string;
  sourceFileSha256: string;
  authorizationId: string;
  authorizationVersion: string;
  publicationApprover: string;
  registeredAt: string;
}

export interface PriceCardListResponseBody {
  outcome: 'PRICE_CARDS_LISTED';
  cards: PriceCardRecord[];
}

export interface ReferenceSeriesRecord {
  seriesId: string;
  seriesVersion: string;
  kind: string;
  sourceIdentifier: string;
  registrant: string;
  quoteBasisId?: string;
  quoteBasisVersion?: string;
  effectiveFrom: string;
  effectiveTo?: string;
  evidenceGrade: string;
  priorVersion?: string;
  correctionBasis?: string;
  canonicalization: string;
  contentDigest: string;
  registeredAt: string;
}

export interface ReferenceSeriesListResponseBody {
  outcome: 'REFERENCE_SERIES_LISTED';
  series: ReferenceSeriesRecord[];
}

export function listPriceCards() {
  return exchangeMasterData<PriceCardListResponseBody>('/pricing-price-cards');
}

export function listReferenceSeries() {
  return exchangeMasterData<ReferenceSeriesListResponseBody>('/pricing-reference-series');
}

// 评价登记册检索列面(GET /pricing-evaluations,票 admin-skeleton-closure-batch/03)。
// 评价的语义细节(对象、方向、金额)住在快照内属详情读法,端点不透出,这里也不虚构。
export interface PricingEvaluationRecord {
  evaluationId: string;
  /** 封闭五格原词:COMPLETED/PENDING/CONFLICT/FAILED/UNRATABLE。 */
  status: string;
  semanticDigest: string;
  planContentDigest: string;
  canonicalization: string;
  recordedAt: string;
}

export interface PricingEvaluationListResponseBody {
  outcome: 'EVALUATIONS_LISTED';
  evaluations: PricingEvaluationRecord[];
}

export function listPricingEvaluations() {
  return exchangeMasterData<PricingEvaluationListResponseBody>('/pricing-evaluations');
}
