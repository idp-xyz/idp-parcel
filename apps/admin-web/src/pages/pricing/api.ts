// 本目录 fetch 出口:价卡目录与计价参考序列查阅(ADR-0077、票 master-data-wiring/02)。
// 形状以 internal/parcelpricing/adapters/http 传输层为准,此处只做镜像不虚构。
//
// 响应判读按 ADR-0022:HTTP 状态码只回答「服务端有没有形成答案」,业务判别一律在
// 响应体的 `outcome`。五格判别与全程追踪页同款,传输实现收敛在共享 catalogue-api
// (装配侧只在 bootstrap 配置那一处前缀),本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, postMasterData } from '../catalogue-api';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

export type { ApiResult } from '../catalogue-api';

// ---- 登记写面（ADR-0085，票 admin-write-faces/01 切片 01b）----
//
// 两个登记端点已在装配表里，挂的是字面量 `UnconfiguredIntake{}`：写准入不另立形，与其余
// 命令面同等 `PAR-INT-01` 证据，隔离读准入换不了写行。因此**今天提交必然答 403
// ACCESS_CHANNEL_NOT_CONFIGURED**，那是诚实答案不是接线缺陷；墙降当天在装配点换真 Intake
// 即点亮，本目录一行不用改。
//
// **请求体形状此刻没有契约。** ADR-0085 Decision 三把「渠道原始载荷 → 登记快照」的翻译
// 划给渠道接入契约，随 `PAR-INT-01` 提供。所以这里不发明字段：页面收的是登记快照 JSON
// 本体（与受控登记 CLI `parcel-pricing-register -file` 吃的同一份形状，那是有文档的），
// 原样作为请求体送出。真渠道接线时以渠道契约为准重谈，不得反过来把这里当成已发布的
// Schema——判据与 shipment-request 草案那节同一条。

// 登记端点的封闭响应形状（`outcome` 取应用结果枚举原名，与登记 CLI 同源）随表单区一起
// 抽到 components/registration；本上下文的两个登记口不带拒绝理由，只用到 `outcome` 一格。

/**
 * 两类登记的答案代数（`application.RegisterPriceCardOutcome` /
 * `RegisterReferenceSeriesOutcome` 的原名），逐格中文。
 *
 * 治理答案不是失败：`CONTENT_CONFLICT` 与 `CANONICALIZATION_DIFFERS` 都是登记册给出的
 * 业务结论，原行不被顶替，续办属治理裁决——页面照实呈现，不折成「提交失败」。
 */
export const registrationOutcomeLabels: Record<string, string> = {
  RECORDED: '已入册',
  ALREADY_REGISTERED: '已在册（同版本同内容，幂等重放）',
  CONTENT_CONFLICT: '版本内容冲突（原行不被顶替，续办属治理裁决）',
  CANONICALIZATION_DIFFERS: '规范化版本不可比（既不是冲突也不是重放）',
  NOT_ACCEPTED: '请求不受理（登记本体立不起来，未到达登记册）',
  UNDECIDED: '未决（依赖故障，登记与否未知）',
};

export function registerPriceCard(
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>('/pricing-price-card-registrations', snapshot);
}

export function registerReferenceSeries(
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(
    '/pricing-reference-series-registrations',
    snapshot,
  );
}

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
