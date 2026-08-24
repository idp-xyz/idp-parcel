// 本目录唯一的 fetch 出口:UC-PS-001 提交、UC-PS-005 决定前撤回,以及委托查阅
// (GET /shipment-request-views,列表与单份共用一个端点、按 shipmentRequestId 分派)。
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

// ---- 请求草案形状 ----
//
// 真实线格式不是本目录能定的:提交与撤回两个动作端点当前挂「未配置即拒」Intake,不读请求体;
// 渠道字段格式、枚举与必填条件属 PAR-INT-01 待登记。下面的草案只承载 UC-PS-001
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
