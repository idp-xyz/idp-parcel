// 本目录唯一的 fetch 出口:UC-PS-001 提交、UC-PS-005 决定前撤回、UC-PS-006 接受后
// 取消,以及委托查阅(GET /shipment-request-views,列表与单份共用一个端点、按
// shipmentRequestId 分派)。
//
// 响应判读按 ADR-0022:HTTP 状态码只回答「服务端有没有形成答案」,业务判别一律在
// 响应体的 `outcome`,取应用结果枚举的原字符串,传输层不合并、不改名。因此这里把
// 一次调用分成五格交给页面,每格的下一步动作不同:
//   outcome        —— 2xx,答案已形成,按 outcome 呈现给操作员(201 表示新建成立);
//   unconfigured   —— 403 + ACCESS_CHANNEL_NOT_CONFIGURED,接入渠道未配置,改请求
//                     或重试都不会好,要去提供渠道参数(ADR-0055 的「未配置即拒」格);
//   callerProblem  —— 其余 4xx,调用方的错,重发同样内容不会改变结果;
//   noAnswer       —— 5xx,服务端没形成答案,可退避重试;
//   transport      —— fetch 未通或响应不是 JSON:parcel-api 一律写 JSON,非 JSON
//                     说明请求根本没到它(前缀/代理未通),与 5xx 分开,恢复动作不同。
//
// 单份查阅多一枚要认的 4xx code:SHIPMENT_REQUEST_NOT_VISIBLE(404)。它是终局业务
// 答案而不是故障——「统一不可见结果」不区分「不存在」与「越权/他租户对象」,页面按
// callerProblem 里的这枚 code 专门呈现,不得当错误重试,也不得替它补出第二种说法。

// 路径前缀已裁决:前端统一带 /api 发起,开发代理剥前缀转发;由装配侧在 bootstrap
// 调 configureShipmentRequestApi({ basePrefix: '/api' }) 注入。默认空串只是未装配
// 时的中性起点,不是第二套前缀约定;本目录其余代码不感知前缀。
let apiBase = '';

export function configureShipmentRequestApi(options: { basePrefix: string }): void {
  apiBase = options.basePrefix;
}

// ---- 响应形状(与 internal/parcelshipment/adapters/http 的封闭响应一一对应) ----

export type SubmitOutcome =
  | 'SUBMITTED'
  | 'EXISTING_RESULT'
  | 'INGRESS_CONFLICT'
  | 'INPUT_NOT_ACCEPTED'
  | 'OTHER_PRODUCTION_AUTHORITY'
  | 'OWNERSHIP_UNRESOLVED'
  | 'ADMISSION_PAUSED'
  | 'PRIOR_REQUEST_NOT_FOUND'
  | 'LINK_INELIGIBLE';

// 归属决定本身而不只是结论字符串:非本产品归属要报当前权威方与交接结果,归属未决
// 要报缺口与安全续办引用,少了它们调用方不知道该找治理还是找客户。
export interface ProductionOwnershipView {
  decisionId: string;
  authority: string;
  admissionControl: string;
  ruleVersion: string;
  revision: string;
  otherAuthority?: string;
  handoffReference?: string;
  unresolvedReason?: string;
  continuationReference?: string;
  suspensionReference?: string;
}

export interface SubmitResponseBody {
  outcome: SubmitOutcome;
  shipmentRequestId?: string;
  productionOwnership?: ProductionOwnershipView;
  gateBlockReasons?: string[];
}

export type WithdrawalOutcome =
  | 'FORMED'
  | 'NOT_AUTHORIZED'
  | 'DECISION_ALREADY_FORMED'
  | 'UNDECIDED'
  | 'SOURCE_CONFLICT';

export interface WithdrawalResponseBody {
  outcome: WithdrawalOutcome;
  requestState?: string;
  withdrawalId?: string;
  decisionKind?: 'ACCEPTED' | 'REJECTED';
  pendingReason?: string;
  continuationReference?: string;
  compensationReference?: string;
}

// UC-PS-006 接受后取消(POST /shipment-requests/parcel-cancellations)。词取
// application.CancelParcelOutcome 的原字符串:三种已提交走向(取消成立、待处置、
// 拒绝)加未决、重复、冲突、不受理。REQUEST_NOT_ACCEPTED 兼收「委托或包裹在当前
// 客户范围内查不到」——统一不可见在编排内作答(AT-PS-090),不走 4xx,是终局业务答案。
export type CancellationOutcome =
  | 'PARCEL_CANCELLED'
  | 'DISPOSITION_PENDING'
  | 'CANCELLATION_REFUSED'
  | 'CANCELLATION_UNDECIDED'
  | 'EXISTING_RESULT'
  | 'REQUEST_CONFLICT'
  | 'REQUEST_NOT_ACCEPTED';

// 与 adapters/http 的 cancellationResponse 一一对应:三种已提交走向各带自己的凭据
// ——取消成立带取消决定标识、待处置带越过的收寄版本(调用方要知道输给了哪次收寄)、
// 拒绝带规则依据;未决带原因与续办引用;取消成立而发布意图未交出时带重发引用
// (重放同一请求会重发同一份意图,决定本身不受影响)。
export interface CancellationResponseBody {
  outcome: CancellationOutcome;
  cancellationId?: string;
  intakeVersion?: string;
  refusalBasis?: string;
  decidedAt?: string;
  pendingReason?: string;
  continuationReference?: string;
  handoffReference?: string;
}

// ---- 委托查阅响应形状(与 internal/parcelshipment/adapters/http 的封闭响应一一对应) ----
//
// 这是已落地的查询契约的镜像,不再是页面侧草案:字段跟着服务端读模型走,读模型没有
// 的东西(客户委托参考、服务产品、寄收件关系等)这里不虚构。作用域与页大小由服务端
// 接入面从认证与授权结果裁决(CONTEXT「授权查询作用域」),本目录不送任何自报身份参数。

/** 统一不可见结果的传输层 code(单份查阅 404 携带,终局业务答案)。 */
export const REQUEST_NOT_VISIBLE_CODE = 'SHIPMENT_REQUEST_NOT_VISIBLE';

export interface ShipmentRequestSummary {
  shipmentRequestId: string;
  customerAccountId: string;
  source: string;
  sourceRequestKey: string;
  state: string;
  submissionVersionId: string;
  declaredParcelCount: number;
  submittedAt: string;
}

export interface ParcelDimensions {
  length: string;
  width: string;
  height: string;
  unit: string;
}

/** 声明包裹(读模型粒度:内部包裹标识 + 声明测量;值原样保全,不规范化)。 */
export interface DeclaredParcelRecord {
  parcelId: string;
  declaredWeightValue?: string;
  declaredWeightUnit?: string;
  dimensions?: ParcelDimensions;
}

/** 接受判断任务查阅面:任务阶段 + 最近一次没能推进的处理记录(有过才在场)。 */
export interface AcceptanceTaskRecord {
  state: string;
  lastAttemptReason?: string;
  lastAttemptContinuation?: string;
  lastAttemptedAt?: string;
}

/** 已形成的接受/拒绝决定(kind 词汇与撤回端点的 decisionKind 同一套)。 */
export interface DecisionRecord {
  decisionId: string;
  kind: 'ACCEPTED' | 'REJECTED';
  decidedAt: string;
}

export interface ShipmentRequestDetail extends ShipmentRequestSummary {
  batchId: string;
  occurredAt: string;
  receivedAt: string;
  declaredParcels: DeclaredParcelRecord[];
  priorVersionCount: number;
  acceptanceTask: AcceptanceTaskRecord;
  decision?: DecisionRecord;
}

/** 列表:空结果仍是 LISTED + 空数组(空列表不泄露任何存在性,是正常业务答案)。 */
export interface ViewsListResponseBody {
  outcome: 'LISTED';
  requests: ShipmentRequestSummary[];
}

export interface ViewDetailResponseBody {
  outcome: 'REQUEST_VIEW';
  request: ShipmentRequestDetail;
}

// ---- 复核队列响应形状(票 admin-skeleton-closure-batch/09) ----
//
// 队列是委托查阅面的子集视图,复用同一套作用域与页大小裁决,因此这里只加复核语境
// 特有的那几格:任务停在哪、复核留痕录了没、判断表上已记录的权威判断说了什么。
// 委托本身的字段一律复用上面的 ShipmentRequestSummary / ShipmentRequestDetail,
// 不另抄一份——抄第二份就得在两处约定哪一份是准的。

/** 队列一行:委托摘要 + 最近一次未推进的处理记录 + 复核留痕(录了才在场)。 */
export interface AcceptanceReviewQueueEntry extends ShipmentRequestSummary {
  lastAttemptReason?: string;
  lastAttemptContinuation?: string;
  lastAttemptedAt?: string;
  /** 复核已录完成但尚未续办的行照列不出队——出队与否由下一轮判断决定,不由读面折叠。 */
  reviewCompleted: boolean;
  reviewAuthority?: string;
  reviewReviewer?: string;
  reviewEvidence?: string;
  reviewCompletedAt?: string;
}

export interface AcceptanceReviewQueueListResponseBody {
  outcome: 'REVIEW_QUEUE_LISTED';
  entries: AcceptanceReviewQueueEntry[];
}

/**
 * 任务文档上的等待与留痕。waitingOn 取 ResumePath 原词(MANUAL_REVIEW 等),缺席即
 * 决定已形成或任务已完结——详情不按它过滤,深链一份已续办的委托如实呈现当前状态。
 */
export interface ReviewStatusRecord {
  waitingOn?: string;
  completed: boolean;
  authority?: string;
  reviewer?: string;
  evidence?: string;
  completedAt?: string;
}

/** 可达性判断一行。`不适用`带依据说明这个问题为什么不该问,其余三值带判断标识。 */
export interface ReviewReachabilityRecord {
  parcelId: string;
  value: string;
  judgmentId?: string;
  basis?: string;
  asOfAt?: string;
  asOfSemantics?: string;
  asOfPolicyVersion?: string;
}

export interface ReviewFinancialControlRecord {
  outcome: string;
  resultId?: string;
  basis?: string;
  asOfAt?: string;
  asOfSemantics?: string;
  asOfPolicyVersion?: string;
}

/** 财务控制与采用解析尚未形成时整格缺席——不造「空结果」冒充判断过。 */
export interface RecordedJudgmentsRecord {
  reachability: ReviewReachabilityRecord[];
  financialControl?: ReviewFinancialControlRecord;
  adoptedResolutionId?: string;
}

export interface AcceptanceReviewCaseResponseBody {
  outcome: 'REVIEW_CASE';
  request: ShipmentRequestDetail;
  review: ReviewStatusRecord;
  recordedJudgments: RecordedJudgmentsRecord;
}

// ---- 面单交易查阅响应形状(票 admin-skeleton-closure-batch/08,ADR-0084 决定七) ----
//
// 行粒度是交易 × 包裹:一笔交易覆盖几件包裹就摊几行,摊开在服务端读侧完成,页面不再
// 二次组装。作用域只有租户维——覆盖包裹可以跨委托,按客户账户过滤会把一笔跨客户的交易
// 归给其中一个客户,所以这是运营查阅面而不是客户面。

/**
 * 面单交易查阅的一行。
 *
 * hasParcelResult 与 parcelAccepted 是两格不是一格:结果未回时 parcelAccepted 为
 * false,而把它读成「未受理」就是把 CONTEXT 禁止的「结果不确定按失败处理」搬到页面上。
 * 呈现时必须先看 hasParcelResult。
 */
export interface LabelTransactionRow {
  transactionId: string;
  parcelId: string;
  channelAccount: string;
  /** 渠道账号持有人。与渠道服务方、合同与结算相对方是三个独立角色,不合并。 */
  accountHolder: string;
  channelServicer: string;
  settlementCounterparty: string;
  contract: string;
  rate: string;
  responsibilityBasis: string;
  /** 交易级结果(状态枚举原词)。交易级失败不能推导包裹失败。 */
  transactionResult: string;
  /** 面单交易定案:交易级结果已落在成功/部分成功/失败之一,派生而非存储列。 */
  finalized: boolean;
  hasParcelResult: boolean;
  parcelAccepted: boolean;
  parcelIdentifier?: string;
  parcelResultReason?: string;
  /** 作用到本件包裹的后续动作种类(渠道作废/退款/替代),追加式,不改写原结果。 */
  followUpKinds: string[];
  /** 包裹级继续尝试判断:开放 / 受控关闭。派生依据见 continuedAttemptBasis。 */
  continuedAttemptOpen: boolean;
  /**
   * 这件包裹的继续尝试决定登记册里有没有任何决定。与 continuedAttemptOpen 并列而不折进去:
   * 「开放」既可能来自「没有人作过决定」,也可能来自「关过又重开」——判断值只有两格
   * (CONTEXT 原词,不加第三格),两种来源的现场处置却相反,页面靠这一格分开说。
   */
  continuedAttemptDecided: boolean;
  establishedAt: string;
  submittedAt?: string;
  resultObservedAt?: string;
  priorTransactionId?: string;
  priorLinkKind?: string;
}

/**
 * 列表:空册仍是 LISTED + 空数组。渠道墙未降前登记零行是设计,不是缺陷。
 *
 * continuedAttemptBasis 是「继续尝试判断」那一列的派生依据代码,页头要如实转述它——
 * 这一列是按 CONTEXT 规则由决定登记册与当前有效终局现算出来的,不是存下来的状态,
 * 不写明就会被读成「有人逐件核对过」。
 */
export interface LabelTransactionsListResponseBody {
  outcome: 'LABEL_TRANSACTIONS_LISTED';
  continuedAttemptBasis: string;
  rows: LabelTransactionRow[];
}

/** 继续尝试判断由决定登记册与当前有效终局现算时服务端交回的依据代码。 */
export const CONTINUED_ATTEMPT_BASIS_REGISTER_AND_CURRENT_FINAL =
  'DERIVED_FROM_DECISION_REGISTER_AND_CURRENT_FINAL';

/** 复核完成命令的封闭结果。`已有完成`带先到那份的留痕:操作员要知道签的是谁。 */
export type ManualReviewCompletionOutcome =
  | 'RECORDED'
  | 'ALREADY_COMPLETED'
  | 'TASK_CONCLUDED'
  | 'VERSION_SUPERSEDED';

export interface ManualReviewCompletionResponseBody {
  outcome: ManualReviewCompletionOutcome;
  requestState?: string;
  reviewer?: string;
  authority?: string;
  evidence?: string;
  completedAt?: string;
  decisionKind?: 'ACCEPTED' | 'REJECTED';
  currentVersion?: string;
}

/** 主动拒绝命令的封闭结果。撞上既有决定时交回先到那一个,可能是接受。 */
export interface ActiveRejectionResponseBody {
  outcome: string;
  requestState?: string;
  decisionKind?: 'ACCEPTED' | 'REJECTED';
  pendingReason?: string;
  continuationReference?: string;
  compensationReference?: string;
}

// ---- 请求草案形状 ----
//
// 真实线格式不是本目录能定的:动作端点当前一律挂「未配置即拒」Intake,不读请求体;
// 渠道字段格式、枚举与必填条件属 PAR-INT-01 待登记。下面的草案只承载各用例
// 输入语义契约里「客户可声明」的各组,键名是页面侧暂定,真渠道接线时以渠道契约
// 为准重谈,不得反过来把这里当成已发布的渠道 Schema。

export interface DeclaredParcelDraft {
  /** 客户侧包裹引用。 */
  customerParcelReference: string;
  /** 声明测量:毛重必备,数值原样保全("1.50" 不规范化),单位是客户引用、无默认。 */
  declaredWeightValue: string;
  declaredWeightUnit: string;
  /** 外廓可整体缺席,但不可半截(长宽高与单位要么齐全要么全空)。 */
  declaredLength?: string;
  declaredWidth?: string;
  declaredHeight?: string;
  declaredDimensionsUnit?: string;
  /** 申报原始资料:具体必填字段由真实产品和关务区域决定,页面不发明必填。 */
  goodsDescription?: string;
  quantity?: string;
  declaredValue?: string;
  currency?: string;
  originCountry?: string;
  /** 货物和服务资料的补充说明。 */
  serviceNotes?: string;
}

export interface ShipmentRequestDraft {
  /** 客户委托参考。 */
  customerShipmentReference: string;
  /** 请求的服务产品或服务要求(真实产品目录未登记,只收客户话语,不做下拉)。 */
  requestedServiceProduct: string;
  /**
   * 客户请求生效时间(requestEffectiveAt)。缺失与显式存在本身进入内容摘要,
   * 因此留空时必须整个键缺席,不能送空串冒充「填了」。
   */
  requestEffectiveAt?: string;
  senderRelation: string;
  senderAddress: string;
  recipientRelation: string;
  recipientAddress: string;
  /** 目的服务范围。 */
  destinationServiceScope: string;
  parcels: DeclaredParcelDraft[];
}

export interface WithdrawalDraft {
  /** 产生委托的那次提交的来源请求标识,用来定位目标委托。 */
  originalRequestKey: string;
  /** 委托标识(提交成立时返回的 shipmentRequestId)。 */
  shipmentRequestId: string;
  /** 当前提交版本标识,客户端未必持有,留空由服务端按当前版本裁决。 */
  submissionVersionId?: string;
  /** 请求方引用。撤回授权的真实角色属 PAR-COM-14,页面只收引用不判授权。 */
  requesterReference: string;
  /** 撤回原因引用。 */
  reasonReference: string;
}

// 取消草案一次只指名一件包裹:端点一次受理一件的取消请求,批量只归组、不拥有共同
// 状态(UC-PS-006 步骤 1),逐件分发归页面,部分成功由逐件请求自然表达。与撤回不同,
// 取消没有自己的请求信封:编排以原提交的来源身份定位委托,同一来源身份加包裹就是
// 取消请求的幂等键,故这里不设「取消请求标识」字段。
export interface CancellationDraft {
  /** 原提交的来源请求标识。委托按来源身份加编号双重指名,缺一即统一不可见。 */
  originalRequestKey: string;
  /** 委托标识(提交成立时返回的 shipmentRequestId)。 */
  shipmentRequestId: string;
  /** 目标包裹标识(委托当前提交版本的成员,出界同答统一不可见)。 */
  parcelId: string;
  /** 请求方引用。取消授权的真实规则属 PAR-COM-17,页面只收引用不判授权。 */
  requesterReference: string;
  /** 取消原因引用。原因目录属待登记参数,按引用填写。 */
  reasonReference: string;
}

/**
 * 复核完成草案。**故意只有这两个字段。**
 *
 * 复核人、授权依据与证据引用不在这里:那三样是「谁在签」,由 PAR-INT-01 的接入面从
 * 已认证的操作员身份翻译出来(ManualReviewCompletionIntake 的注释:采信自报的复核人
 * 等于让任何调用方替任何角色签复核)。页面把它们填成任何值都是伪造采信身份,哪怕
 * 填的是「开发用」占位。这里送的两样都是页面自己手上的事实:审的是哪一份、理由是什么。
 */
export interface ManualReviewCompletionDraft {
  shipmentRequestId: string;
  /** 复核理由。模板保证非空白后才提交。 */
  reason: string;
}

/** 主动拒绝草案。决定人同样不在这里,理由同上——授权由编排去问 party-commercial。 */
export interface ActiveRejectionDraft {
  shipmentRequestId: string;
  reason: string;
}

// ---- 调用结果 ----

export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

export function submitShipmentRequest(
  draft: ShipmentRequestDraft,
): Promise<ApiResult<SubmitResponseBody>> {
  return post<SubmitResponseBody>('/shipment-requests', draft);
}

export function withdrawShipmentRequest(
  draft: WithdrawalDraft,
): Promise<ApiResult<WithdrawalResponseBody>> {
  return post<WithdrawalResponseBody>('/shipment-requests/withdrawals', draft);
}

export function cancelParcel(
  draft: CancellationDraft,
): Promise<ApiResult<CancellationResponseBody>> {
  return post<CancellationResponseBody>('/shipment-requests/parcel-cancellations', draft);
}

export function listShipmentRequestViews(): Promise<ApiResult<ViewsListResponseBody>> {
  return exchange<ViewsListResponseBody>('/shipment-request-views', { method: 'GET' });
}

export function findShipmentRequestView(
  shipmentRequestId: string,
): Promise<ApiResult<ViewDetailResponseBody>> {
  return exchange<ViewDetailResponseBody>(
    `/shipment-request-views?shipmentRequestId=${encodeURIComponent(shipmentRequestId)}`,
    { method: 'GET' },
  );
}

export function listAcceptanceReviewQueue(): Promise<
  ApiResult<AcceptanceReviewQueueListResponseBody>
> {
  return exchange<AcceptanceReviewQueueListResponseBody>('/acceptance-review-queue', {
    method: 'GET',
  });
}

export function findAcceptanceReviewCase(
  shipmentRequestId: string,
): Promise<ApiResult<AcceptanceReviewCaseResponseBody>> {
  return exchange<AcceptanceReviewCaseResponseBody>(
    `/acceptance-review-queue?shipmentRequestId=${encodeURIComponent(shipmentRequestId)}`,
    { method: 'GET' },
  );
}

export function listLabelTransactions(): Promise<ApiResult<LabelTransactionsListResponseBody>> {
  return exchange<LabelTransactionsListResponseBody>('/label-transactions', { method: 'GET' });
}

export function completeManualReview(
  draft: ManualReviewCompletionDraft,
): Promise<ApiResult<ManualReviewCompletionResponseBody>> {
  return post<ManualReviewCompletionResponseBody>(
    '/shipment-requests/manual-review-completions',
    draft,
  );
}

export function rejectShipmentRequest(
  draft: ActiveRejectionDraft,
): Promise<ApiResult<ActiveRejectionResponseBody>> {
  return post<ActiveRejectionResponseBody>('/shipment-requests/rejections', draft);
}

function post<Body>(path: string, payload: unknown): Promise<ApiResult<Body>> {
  return exchange<Body>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

async function exchange<Body>(path: string, init: RequestInit): Promise<ApiResult<Body>> {
  let response: Response;
  try {
    response = await fetch(apiBase + path, init);
  } catch (cause) {
    return {
      kind: 'transport',
      message: cause instanceof Error ? cause.message : String(cause),
    };
  }

  let parsed: unknown;
  try {
    parsed = await response.json();
  } catch {
    return {
      kind: 'transport',
      message: `响应不是 JSON(HTTP ${response.status}),请求可能未到达 parcel-api`,
    };
  }

  if (response.ok) {
    return { kind: 'outcome', status: response.status, body: parsed as Body };
  }

  const code =
    (parsed as { error?: { code?: string } } | null)?.error?.code ?? 'UNKNOWN';
  if (response.status === 403 && code === 'ACCESS_CHANNEL_NOT_CONFIGURED') {
    return { kind: 'unconfigured' };
  }
  if (response.status >= 500) {
    return { kind: 'noAnswer', status: response.status, code };
  }
  return { kind: 'callerProblem', status: response.status, code };
}
