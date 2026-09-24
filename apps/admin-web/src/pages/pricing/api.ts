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

// ---- 序列版本复核（ADR-0099 决定二；票 pricing-reference-series-operations/04 切片 04b）----
//
// **这一族的载荷形状是产品定的，不是发明的。** 上面登记那一段写着「请求体形状此刻没有
// 契约」，那句对本族不成立：[ADR-0101](docs/adr/0101-…) 决定一把「翻译属渠道接入契约」
// 的适用场景收窄为**客户渠道载荷**，并明定运营操作者面的载荷形状由产品定义、属机制半边，
// 各登记签按「登记频次 × 操作者角色 × 载荷结构」逐册裁形。复核是低频、结构极简（两格）
// 的治理动作，因此取逐字段表单——不是 JSON 快照口。

/**
 * 复核请求体。**只有内容，没有身份。**
 *
 * 复核责任方**不在这里**，也不该在这里：它是四眼门的一半（领域拒绝复核责任方等于登记
 * 责任方），从浏览器送一个上去就是自报身份。传输层 `ReferenceSeriesReviewIntake` 的注释
 * 原话是「从请求内容里铸一个出来就等于把那道门拆了」；身份的正当出处是 ADR-0100 的
 * `OperatorEnvelope`，由接入渠道给。今天渠道未配置，所以本请求必然答 403——那是诚实答案。
 *
 * 复核时刻同理不在这里。复核就是复核责任方此刻作出的确认，时刻取服务端时钟；页面替人挑
 * 一个时刻，等于让「什么时候确认的」变成前端说了算的事实。受控批量口补录历史复核时才显式
 * 带它（`parcel-pricing-register -kind reference-series-review`）。
 */
export interface SeriesReviewRequest {
  seriesId: string;
  seriesVersion: string;
  /** 封闭两格，取 `domain.SeriesReviewDecision` 原词。 */
  decision: 'APPROVED' | 'RETURNED';
  basis: string;
}

/**
 * 复核答案代数（`application.ReviewReferenceSeriesOutcome` 原名），逐格中文。
 *
 * **七格里有三格是治理答案不是调用方错误**，而它们的续办动作各不相同：`需换人复核`要换个
 * 人来、`版本不在册`要先去登记、`冲突`要另追加一条。折成一句「提交失败」会让操作者以为改
 * 字段重试就成。
 *
 * 与 `registrationOutcomeLabels` 分表而不合并：两套代数有重名格。`CONFLICT` 在登记那栏说的
 * 是同版本号异内容、改内容要发新版本；在这栏说的是同键复核结论或依据不同、改主意要另追加
 * 一条。合表会让其中一种顶着另一种的中文显示出来。
 */
export const seriesReviewOutcomeLabels: Record<string, string> = {
  RECORDED: '复核已追加（结论为通过时，该版本自复核时刻起在用）',
  ALREADY_RECORDED: '同键同内容已在册（幂等重放，没有造第二条复核）',
  CONFLICT: '同键在册而结论或依据不同（原行不顶替；改主意请另追加一条）',
  VERSION_UNKNOWN: '被复核的版本不在册（先去登记该版本）',
  NEEDS_ANOTHER_REVIEWER: '四眼门拒：复核责任方就是登记责任方（换一个人来，不是改字段）',
  NOT_ACCEPTED: '请求不受理（结论不在封闭集或缺依据；未到达复核册）',
  UNDECIDED: '未决（依赖故障，记录与否未知，可重试）',
};

export function reviewReferenceSeries(
  request: SeriesReviewRequest,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>('/pricing-reference-series-reviews', request);
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

/**
 * 一期取值（目录行与预览差异两处同一形状）。`endsAt` 缺席即无上界（只许末期），`evidenceRef`
 * 缺席即该期只有断言强度——两处都是缺键表达「没有」，不是空串。
 */
export interface SeriesPeriodRecord {
  startsAt: string;
  endsAt?: string;
  value: string;
  evidenceRef?: string;
}

/**
 * 目录行。票 pricing-reference-series-operations/08 加的两组：
 *
 * - 复核事实：两计数总在场（无复核是 0 条不是缺键），`lastReviewedAt` 与 `lastReviewDecision`
 *   成对在场或成对缺席。**这里没有「在用」**——在用是相对评价形成时刻派生的结论，目录页没有
 *   那个时刻，它归覆盖摘要条（票 04 的 owner 裁决）。
 * - `periods`：该版全部期次，供「更正此版本」预填——不给就得让人重敲一遍，那正好制造更正要
 *   防的那类错误。
 *
 * `referenceDigest` 是登记时声明的**版本引用指纹**（ADR-0108：可选、不进摘要、自身引用今天一律
 * 留空），与 `contentDigest`（PRS 内容摘要）是两回事。更正版本的回指要带的是**前版的 `contentDigest`**
 * 作指纹——那才是操作者手上有来源的东西。页面上**不把 `referenceDigest` 显示为「摘要」**，「内容
 * 摘要」一词只指 `contentDigest`。
 */
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
  referenceDigest: string;
  registeredAt: string;
  reviewCount: number;
  approvedReviewCount: number;
  lastReviewedAt?: string;
  lastReviewDecision?: string;
  periods: SeriesPeriodRecord[];
}

// ---- 逐字段登记表单与登记前预览（票 pricing-reference-series-operations/08；ADR-0101 决定一、四）----
//
// **这一族的载荷形状是产品定的**（决定一：运营操作者面的载荷由产品定义、属机制半边），镜像
// `internal/parcelpricing/adapters/http` 的 `ReferenceSeriesRegistrationPayload`，此处只镜像不虚构。
// 上面登记那一段说的「请求体形状此刻没有契约」对本族不成立；那一段的 JSON 快照口退为受控批量口
// 的在线镜像（「高级」签），运营配置员的主路径是这里。
//
// **载荷里只有内容，没有身份。** 租户与登记责任方由接入渠道的操作者信封（ADR-0100）给，表单不收
// 也不送；送一个上去服务端按未知键拒。今天渠道未配置，预览与登记都必然答 403——那是诚实答案。
//
// **前端不算摘要、不裁证据等级、不铸任何引用令牌**（票 04 红线、MCP-3 裁决四条之四）：三样都由
// 服务端答，页面只呈现。预览与登记收**同一份**载荷、走同一段解码，预览页上的内容摘要与登记册
// 记下的逐字节相等（决定四）。

/**
 * 更正回指：`priorFingerprint` 带目录透出的前版 `contentDigest` 作指纹（ADR-0108 Decision 五），
 * 缺则回指只带三元——服务端不再铸任何令牌顶替。
 */
export interface SeriesCorrectionPayload {
  priorVersion: string;
  priorFingerprint?: string;
  basis: string;
}

/** 口径引用：一版商业价格政策。`digest` 今天不送——PC 读口尚不透出 content_digest，服务端铸令牌。 */
export interface SeriesQuoteBasisPayload {
  policyId: string;
  policyVersion: string;
  digest?: string;
}

export interface SeriesPeriodPayload {
  startsAt: string;
  endsAt?: string;
  value: string;
  evidenceRef?: string;
}

export interface SeriesRegistrationPayload {
  seriesId: string;
  seriesVersion: string;
  kind: string;
  sourceIdentifier: string;
  quoteBasis?: SeriesQuoteBasisPayload;
  /** 只对按期公布金额的序列（PUBLISHED_AMOUNT，ADR-0110）在场：每期取值都是这个币种的金额。 */
  currency?: string;
  periods: SeriesPeriodPayload[];
  correction?: SeriesCorrectionPayload;
  /** 只对预览有意义：指名对照版本。登记忽略它。 */
  compareWithVersion?: string;
}

/** 一期的比对结果：两侧按在场与否给键，三个细项布尔总在场（只在 CHANGED 上为真）。 */
export interface SeriesPeriodChangeRecord {
  startsAt: string;
  kind: string;
  valueChanged: boolean;
  endChanged: boolean;
  evidenceChanged: boolean;
  base?: SeriesPeriodRecord;
  proposed?: SeriesPeriodRecord;
}

/**
 * 预览答复。`outcome` 为 `PREVIEWED` 时其余键在场；`NOT_ACCEPTED` 只有 `outcome`——没有摘要可透，
 * 服务端不给空串装样子。`comparison` 总在场：没要求比也要说「没要求」，读的人才分得开「没比」
 * 与「比了没差异」。
 */
export interface SeriesPreviewResponseBody {
  outcome: string;
  evidenceGrade?: string;
  canonicalization?: string;
  contentDigest?: string;
  comparison?: {
    outcome: string;
    baseVersion?: string;
    changes: SeriesPeriodChangeRecord[];
  };
}

export function previewReferenceSeries(
  payload: SeriesRegistrationPayload,
): Promise<ApiResult<SeriesPreviewResponseBody>> {
  return postMasterData<SeriesPreviewResponseBody>('/pricing-reference-series-previews', payload);
}

/**
 * 表单路径的登记：与「高级」JSON 口打同一个端点，送的是产品定义的载荷而不是领域折装快照。
 * 两种形状在同一端点上**要由真 Intake 分辨**（快照带 `canonicalization`/`contentDigest` 键，
 * 载荷没有）——那是接真 Intake 时的一条要求，今天挂的是字面量 `UnconfiguredIntake{}`，两条路
 * 都在同一堵墙前答 403，分辨逻辑尚无代码；服务端解码器（`ReferenceSeriesRegistrationPayload`）
 * 与登记命令的翻译已在，接上即用。
 */
export function registerReferenceSeriesPayload(
  payload: SeriesRegistrationPayload,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(
    '/pricing-reference-series-registrations',
    payload,
  );
}

/**
 * 口径候选：`GET /commercial-policies?kind=PRICE_POLICY` 的价格政策行，**只取本表单要的几格**。
 * party 页的 `PricePolicyRecord` 止于 `registeredAt`、没有口径节（票 04 那条 MCP-4 取证），
 * 这里另立一个窄类型而不去改那份——它归 party 页所有；两个类型都只是同一 Go 行体的镜像。
 *
 * `caliberDeclared` 可为假（0010 早于 0022，只有正文没口径的行合法）；`caliber.fx` 缺席即该
 * 政策不涉外币。汇率序列的口径必须选**声明了 fx 口径**的版本，选单把这两类分开就是为了这个。
 */
export interface QuoteBasisCandidate {
  objectId: string;
  version: string;
  direction: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  caliberDeclared: boolean;
  caliber?: {
    fx?: { quoteType: string; asOfSemantics: string; asOfPolicyVersion: string };
  };
}

export interface QuoteBasisCandidateListResponseBody {
  outcome: string;
  kind: string;
  policies: QuoteBasisCandidate[];
}

export function listQuoteBasisCandidates() {
  return exchangeMasterData<QuoteBasisCandidateListResponseBody>(
    '/commercial-policies?kind=PRICE_POLICY',
  );
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

// ---- 覆盖地平线（票 pricing-reference-series-operations/05 第 1、3 项）----
//
// 一条序列一行，不是一版一行——一版一行的问法归上面那个 `/pricing-reference-series`。

/**
 * 在用那一版的引用与适用期。**它是子对象而不是平铺的几个可缺席键，那是后端刻意防的一处
 * 折叠**：若把止点平铺上去，「这条序列没有在用版本」与「有在用版本但它没有上界」都表现为
 * 止点缺席，而前者是「今天没得用」、后者是「用着且不会到期」，续办动作相反。
 *
 * `openEnded` 为真时 `effectiveTo` 缺席，那是**没有终点**不是终点未知。
 */
export interface ReferenceSeriesInForce {
  version: string;
  effectiveFrom: string;
  effectiveTo?: string;
  openEnded: boolean;
}

/**
 * 一条序列的覆盖摘要。
 *
 * `inForceResolved` 与 `inForce` 成对，判据同 party-commercial 的 `caliberDeclared`：布尔让
 * 调用方分得开「服务端说没有」与「这个键没序列化出来」。
 *
 * **两个未复核计数不合并**：`unreviewedVersionCount` 是一条复核都还没有的（等复核人），
 * `returnedVersionCount` 是复核过但至今没通过的（等登记方更正）。续办动作不同的东西合成
 * 一个数字，看的人就不知道该去找谁。
 *
 * **没有「剩余多少天」这一格，那是后端有意不给的**：无上界时那个数既不是 0 也不是无穷，
 * 是「没有终点」；算差值要挑时区与舍入口径，属呈现面。页面拿 `asOf` 与 `effectiveTo` 自己算。
 */
export interface ReferenceSeriesCoverageRecord {
  seriesId: string;
  kind: string;
  registeredVersionCount: number;
  inForceResolved: boolean;
  inForce?: ReferenceSeriesInForce;
  lastReviewedAt?: string;
  lastReviewDecision?: string;
  unreviewedVersionCount: number;
  returnedVersionCount: number;
}

/**
 * 顶层的 `asOf` **不是装饰，页面必须显示它**。「在用」是只对某一刻成立的结论，不回显那一刻，
 * 页面上就会出现一个看起来永久的权威答案。同一条判据下，目录页的状态列被裁为不含「在用」
 * ——那一页没有正当的时刻源；本端点有，代价就是把它说出来。
 */
export interface ReferenceSeriesCoverageListResponseBody {
  outcome: 'REFERENCE_SERIES_COVERAGE_LISTED';
  asOf: string;
  series: ReferenceSeriesCoverageRecord[];
}

export function listReferenceSeriesCoverage() {
  return exchangeMasterData<ReferenceSeriesCoverageListResponseBody>(
    '/pricing-reference-series-coverage',
  );
}

/**
 * 被挂起评价联动（GET /pricing-pending-series-evaluations；ADR-0105 Decision 五；票 05 第 2 项）：
 * 因序列未解析而待判断的评价数，按序列种类分组。
 *
 * **它数的是问题项子表有行的评价**：子表随 ADR-0105 才落地且不回填，此前落册的待判断评价不在
 * 这个数里——页面文案要把这一点说出来，不把它包装成「全部挂起评价」。
 *
 * `asOf` 与覆盖端点同一时刻源；两次请求各回各的时刻，并排时以各自的 asOf 为准。
 */
export interface PendingSeriesEvaluationCount {
  /** 序列种类原词：FUEL_RATE / EXCHANGE_RATE / PUBLISHED_AMOUNT。 */
  kind: string;
  evaluationCount: number;
}

export interface PendingSeriesEvaluationsResponseBody {
  outcome: 'PENDING_SERIES_EVALUATIONS_COUNTED';
  asOf: string;
  /** 空数组是正常业务答案：租户内没有因序列未解析而待判断的评价。 */
  counts: PendingSeriesEvaluationCount[];
}

export function countPendingSeriesEvaluations() {
  return exchangeMasterData<PendingSeriesEvaluationsResponseBody>(
    '/pricing-pending-series-evaluations',
  );
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

// ---- 运营试算（UC-PP-001，ADR-0152；票 operator-workspace-gaps/06）----
//
// 端点在装配表里挂字面量 `UnconfiguredIntake{}`：真 Intake 随操作者渠道与回放同批换（operator-channel/06），**今天提交必然答 403
// ACCESS_CHANNEL_NOT_CONFIGURED**，那是诚实答案不是接线缺陷。载荷与响应形状逐字镜像传输层 `estimate_evaluations.go`，不发明字段；
// 载荷里没有租户——租户从操作者信封来，传输层按未知键拒。

export interface EstimatePayload {
  scope: string;
  direction: string;
  basisAt?: string;
  weight: { value: string; unit: string };
  dimensions?: { length: string; width: string; height: string; unit: string };
  zone?: string;
  postalRoute?: { origin: string; destination: string };
  settlementCurrency?: string;
}

export interface EstimateMoney {
  amount: string;
  currency: string;
}

/** 一份试算评价的全部结论。合计只在评价完成时出现，其余各态缺席而不是零（CONTEXT：不得以零金额表达不可计价）。 */
export interface EstimateEvaluationRecord {
  evaluationId: string;
  status: string;
  evidence: string;
  direction: string;
  purpose: string;
  semanticDigest: string;
  total?: EstimateMoney;
  chargeLines: { code: string; description: string; amount: EstimateMoney }[];
  issues: { code: string; message: string }[];
  explanation: string[];
  manifest: { kind: string; id: string; version: string }[];
}

export interface EstimatePlanReference {
  id: string;
  version: string;
}

/** 逐卡一格：`EVALUATED` 带评价，`INPUT_INCOMPLETE` 带缺项码。 */
export interface EstimateCandidateRecord {
  plan: EstimatePlanReference;
  answer: string;
  missing?: string[];
  evaluation?: EstimateEvaluationRecord;
}

/** `outcome` 只说编排；卡与卡之间不排序、不标首选（择优归网络与路由）。 */
export interface EstimateResponseBody {
  outcome: string;
  reason?: string;
  candidates: EstimateCandidateRecord[];
  conflict: EstimatePlanReference[];
}

export function formEstimate(payload: EstimatePayload): Promise<ApiResult<EstimateResponseBody>> {
  return postMasterData<EstimateResponseBody>('/pricing-estimates', payload);
}
