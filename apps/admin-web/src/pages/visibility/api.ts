// 本目录唯一的 fetch 出口:运营追踪查阅(GET /tracking-projections,ADR-0076)。
// 列表、单件当前版与按版本读回共用一个端点,按 parcel / version 查询参数分派;
// 形状以 internal/visibilityexception/adapters/http 传输层为准,此处只做镜像不虚构。
//
// 响应判读按 ADR-0022:HTTP 状态码只回答「服务端有没有形成答案」,业务判别一律在
// 响应体的 `outcome`,取封闭集合的原字符串,传输层不合并、不改名。五格判别与委托
// 查阅页同款,每格的下一步动作不同:
//   outcome        —— 2xx,答案已形成,按 outcome 呈现给操作员;
//   unconfigured   —— 403 + ACCESS_CHANNEL_NOT_CONFIGURED,接入渠道未配置,改请求
//                     或重试都不会好,要去登记渠道参数(ADR-0055 的「未配置即拒」格);
//   callerProblem  —— 其余 4xx,调用方的错,重发同样内容不会改变结果;
//   noAnswer       —— 5xx,服务端没形成答案,可退避重试;
//   transport      —— fetch 未通或响应不是 JSON:parcel-api 一律写 JSON,非 JSON
//                     说明请求根本没到它(前缀/代理未通),与 5xx 分开,恢复动作不同。
//
// 作用域与页大小由服务端接入面从认证与授权结果裁决(ADR-0076:运营查阅的授权边界
// 只有租户,页大小按渠道契约),本目录不送任何自报身份或分页参数。

// 路径前缀与委托查阅同一裁决:前端统一带 /api 发起,开发代理剥前缀转发;由装配侧
// 在 bootstrap 调 configureVisibilityApi({ basePrefix: '/api' }) 注入。
let apiBase = '';

export function configureVisibilityApi(options: { basePrefix: string }): void {
  apiBase = options.basePrefix;
}

// ---- 响应形状(与 internal/visibilityexception/adapters/http 的封闭响应一一对应) ----

/**
 * 一条已接受事实引用与其里程碑归类(projectionEntryBody 的镜像)。字段里只有引用与
 * 归类,没有源事实的内容拷贝——投影「不保存第二套源事实」的落点。
 */
export interface ProjectionEntryRecord {
  /** 源上下文,传输层封闭五元:PARCEL_SHIPMENT / NETWORK_ROUTING / NODE_OPERATIONS / TRANSPORT_FULFILLMENT / CUSTOMS_COMPLIANCE。 */
  source: string;
  /** 源事实引用。源事实本体归源上下文,这里只引用。 */
  fact: string;
  /** 源上下文拥有的事实类型,开放词表,原词转写,页面不译不并。 */
  kind: string;
  factVersion: string;
  /** 被本条取代的前身版本;缺席即首登事实。替代关系由源上下文指名,本上下文只登记。 */
  supersedes?: string;
  /** 业务发生、有效与接收三个时间分别保存(CONTEXT 硬句),合并就分不出迟到与更正。 */
  occurredAt: string;
  effectiveAt: string;
  receivedAt: string;
  mappingVersion: string;
  /** 缺席即「按该映射版本无法可靠归类」——未归类是真话不是缺陷,不为凑时间线补宽泛值。 */
  milestone?: string;
}

/** 一份投影版本(projectionBody 的镜像)。 */
export interface TrackingProjectionRecord {
  version: string;
  parcel: string;
  derivedAt: string;
  /** 重派生版本指回被替代的那一版;原版本连同条目仍可按版本读回(ADR-0065)。 */
  priorVersion?: string;
  entries: ProjectionEntryRecord[];
}

/** 列表:空结果仍是 PROJECTIONS_LISTED + 空数组(租户内尚无投影是正常业务答案)。 */
export interface ProjectionListResponseBody {
  outcome: 'PROJECTIONS_LISTED';
  projections: TrackingProjectionRecord[];
}

/**
 * 单件读法的封闭结果。「投影未形成」与「版本未留存」对租户内已授权的运营查阅是
 * 如实的 2xx 业务答案,不承袭客户面 VIEW_NOT_FOUND 的三义合并(ADR-0076 第四条);
 * 对外部客户面的合并义务不因此松动,那在 /customer-tracking-view 一侧照旧。
 */
export type ProjectionDetailOutcome =
  | 'CURRENT_PROJECTION'
  | 'PROJECTION_NOT_FORMED'
  | 'PROJECTION_VERSION'
  | 'VERSION_NOT_FOUND';

export interface ProjectionDetailResponseBody {
  outcome: ProjectionDetailOutcome;
  projection?: TrackingProjectionRecord;
}

// ---- 调用结果 ----

export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

export function listTrackingProjections(): Promise<ApiResult<ProjectionListResponseBody>> {
  return exchange<ProjectionListResponseBody>('/tracking-projections', { method: 'GET' });
}

export function findCurrentProjection(
  parcel: string,
): Promise<ApiResult<ProjectionDetailResponseBody>> {
  return exchange<ProjectionDetailResponseBody>(
    `/tracking-projections?parcel=${encodeURIComponent(parcel)}`,
    { method: 'GET' },
  );
}

export function findProjectionVersion(
  version: string,
): Promise<ApiResult<ProjectionDetailResponseBody>> {
  return exchange<ProjectionDetailResponseBody>(
    `/tracking-projections?version=${encodeURIComponent(version)}`,
    { method: 'GET' },
  );
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
