// 本目录 fetch 出口:网络目录运营查阅(GET /network-catalog,ADR-0077、票
// master-data-wiring/03)。七族按 ?family= 分派;管理台网络目录页只请求六族,
// 服务区域族由服务区域页固定 family=service-area 请求(MCP-4 终表与 MCP-3 裁决)。

let apiBase = '';

export function configureNetworkApi(options: { basePrefix: string }): void {
  apiBase = options.basePrefix;
}

/** 与登记口 kind / 查询参数 family 封闭集同词。 */
export type NetworkCatalogFamily =
  | 'node'
  | 'connection'
  | 'line'
  | 'service-area'
  | 'service-calendar'
  | 'availability-adjustment'
  | 'route-strategy';

export type NetworkCatalogOutcome =
  | 'NODE_VERSIONS_LISTED'
  | 'CONNECTION_VERSIONS_LISTED'
  | 'LINE_VERSIONS_LISTED'
  | 'SERVICE_AREA_VERSIONS_LISTED'
  | 'SERVICE_CALENDAR_VERSIONS_LISTED'
  | 'AVAILABILITY_ADJUSTMENTS_LISTED'
  | 'ROUTE_STRATEGY_VERSIONS_LISTED';

export interface NetworkVersionRecord {
  code?: string;
  version: number;
  businessTimezone?: string;
  effectiveFrom?: string;
  effectiveTo?: string;
  fromNode?: string;
  toNode?: string;
  segments?: string[];
  applicableScope?: string;
  targetKind?: string;
  targetCode?: string;
  kind?: string;
  source?: string;
  effectiveAt?: string;
  liftedAt?: string;
}

export interface NetworkCatalogListResponseBody {
  outcome: NetworkCatalogOutcome;
  versions: NetworkVersionRecord[];
}

export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

export function listNetworkCatalog(
  family: NetworkCatalogFamily,
): Promise<ApiResult<NetworkCatalogListResponseBody>> {
  return exchange<NetworkCatalogListResponseBody>(
    `/network-catalog?family=${encodeURIComponent(family)}`,
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
