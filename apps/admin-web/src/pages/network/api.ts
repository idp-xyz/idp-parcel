// 本目录 fetch 出口:网络目录运营查阅(GET /network-catalog,ADR-0077、票
// master-data-wiring/03)。七族按 ?family= 分派;管理台网络目录页只请求六族,
// 服务区域族由服务区域页固定 family=service-area 请求(MCP-4 终表与 MCP-3 裁决)。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

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

export function listNetworkCatalog(
  family: NetworkCatalogFamily,
): Promise<ApiResult<NetworkCatalogListResponseBody>> {
  return exchangeMasterData<NetworkCatalogListResponseBody>(
    `/network-catalog?family=${encodeURIComponent(family)}`,
  );
}
