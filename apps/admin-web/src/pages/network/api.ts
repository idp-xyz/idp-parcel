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

// ——以下为路由判断两册的检索列面(GET /route-plans,票 admin-skeleton-closure-batch/03)。
// 两册按 ?register= 分派;计划本体、无路可走判断与改路决定住在判断快照(jsonb)内,
// 属各判断口的权威读法,端点不透出,这里也不虚构。

/** 计划适用性组;仅计划版本已有适用性登记时在场。basis/successor 逐态在场。 */
export interface RoutePlanApplicability {
  /** 封闭四态原词:CURRENTLY_EFFECTIVE/SUPERSEDED/LAPSED/CONCLUDED。 */
  state: string;
  transitionedAt: string;
  basis?: string;
  successor?: string;
}

export interface InitialRouteRecord {
  customerAccountId: string;
  shipmentRequestId: string;
  acceptanceBaseline: string;
  declaredParcelId: string;
  servicePurpose: string;
  /** 封闭两格原词:ROUTE_FORMED/NO_CURRENT_ROUTE。 */
  conclusion: string;
  /** 仅成计划行在场。 */
  planVersion?: string;
  applicability?: RoutePlanApplicability;
  recordedAt: string;
}

export interface InitialRouteListResponseBody {
  outcome: 'INITIAL_ROUTES_LISTED';
  judgments: InitialRouteRecord[];
}

export interface RouteReassessmentRecord {
  correlationId: string;
  customerAccountId: string;
  shipmentRequestId: string;
  acceptanceBaseline: string;
  declaredParcelId: string;
  servicePurpose: string;
  /** 封闭四格原词:STILL_APPLICABLE/PLAN_LAPSED/FIRST_PLAN_FORMED/REROUTED。 */
  conclusion: string;
  reviewedPlan?: string;
  lapseBasis?: string;
  /** 封闭三态原词;缺席即本走向不评估。 */
  candidateState?: string;
  /** 封闭三态原词;缺席即未评估改路。 */
  rerouteState?: string;
  reassessedAt: string;
  recordedAt: string;
}

export interface RouteReassessmentListResponseBody {
  outcome: 'ROUTE_REASSESSMENTS_LISTED';
  reassessments: RouteReassessmentRecord[];
}

export function listInitialRoutes(): Promise<ApiResult<InitialRouteListResponseBody>> {
  return exchangeMasterData<InitialRouteListResponseBody>('/route-plans?register=initial-route');
}

export function listRouteReassessments(): Promise<ApiResult<RouteReassessmentListResponseBody>> {
  return exchangeMasterData<RouteReassessmentListResponseBody>('/route-plans?register=reassessment');
}
