// 本目录 fetch 出口:网络目录运营查阅(GET /network-catalog,ADR-0077、票
// master-data-wiring/03)。七族按 ?family= 分派;管理台网络目录页只请求六族,
// 服务区域族由服务区域页固定 family=service-area 请求(MCP-4 终表与 MCP-3 裁决)。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

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

// ---- 目录登记写面（ADR-0085，票 admin-write-faces/02 切片 02a）----
//
// 登记端点与其余命令面同挂字面量 `UnconfiguredIntake{}`：写准入不另立形，隔离读准入
// （ADR-0078）换得了读行换不了写行。因此**墙降之前提交必然答 403
// ACCESS_CHANNEL_NOT_CONFIGURED**，那是诚实答案不是接线缺陷；登记参数（PAR-INT-01，
// 实例半边）到位后由装配点换真 Intake 即点亮，本文件一行不用改。
//
// **请求体形状此刻没有契约。** ADR-0085 Decision 三把「渠道原始载荷 → 登记快照」的翻译
// 划给渠道接入契约，随 `PAR-INT-01` 提供。所以这里不发明字段：页面收的是登记快照 JSON
// 本体，与受控登记口 `parcel-network-register -kind <族> -file` 吃的同一份形状，原样作
// 请求体送出。真渠道接线时以渠道契约为准重谈，不得反过来把这里当成已发布的 Schema。

/**
 * 逐族登记端点。路径由读口 `/network-catalog` 的册名与该族原词构成，族词与查询参数
 * `?family=`、登记口 `-kind` 逐字同一个——同一本册在读口、写口与 CLI 不换词。
 */
export const networkRegistrationEndpoints: Record<NetworkCatalogFamily, string> = {
  node: '/network-catalog-node-registrations',
  connection: '/network-catalog-connection-registrations',
  line: '/network-catalog-line-registrations',
  'service-area': '/network-catalog-service-area-registrations',
  'service-calendar': '/network-catalog-service-calendar-registrations',
  'availability-adjustment': '/network-catalog-availability-adjustment-registrations',
  'route-strategy': '/network-catalog-route-strategy-registrations',
};

/**
 * 一族一个端点，本函数按族取路径而不是按族分裂成一串同形包装。
 *
 * 传输层那边逐族各立一个端点构造函数，为的是让「把一族的译装接到另一族的端点上」在
 * 编译期就红；那条保护在这里没有落点——快照本体在前端是未翻译的 JSON，分不分函数都
 * 一样送得出去。所以这里与读口的 `listNetworkCatalog` 同形：族是封闭集里的一个参数。
 */
export function registerNetworkCatalogVersion(
  family: NetworkCatalogFamily,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(
    networkRegistrationEndpoints[family],
    snapshot,
  );
}

/**
 * 登记答案代数（`application.RegisterCatalogOutcome` 原名），逐格中文。
 *
 * 本口今天没有治理格：版本号重复由主键挡，撞上时落未决而不是一种业务答案（登记 CLI
 * 的退出码同样没有治理那一路）。所以两格之外的一切都不是登记册的判断。
 */
export const registrationOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记',
  REFUSED: '受理门拒绝（缺件已指名，补齐后重登；原行不被顶替）',
};

/**
 * 受理门拒绝理由（`application.CatalogRefusalReason` 原名），逐格中文。
 *
 * 逐格分开呈现而不折成一句「内容不合法」：各格的续办动作不同——缺时区要去补时区，两端
 * 相同要改端点，段链缺环要重给顺序，折成一格会让登记方去补错东西。
 */
export const registrationRefusalReasonLabels: Record<string, string> = {
  TENANT_MISSING: '缺租户——目录行只在租户内唯一，管辖没给就登不进',
  IDENTITY_MISSING: '缺身份码——没说清登的是哪个对象',
  VERSION_MISSING: '缺版本号——版本由登记方给，登记口不代拟',
  TIMEZONE_MISSING: '缺业务时区——节点、网络连接与适用线路必须明确业务时区',
  ENDPOINT_MISSING: '缺端点节点——网络连接要指出从哪到哪',
  ENDPOINTS_NOT_DISTINCT: '两端节点相同——没有方向可言，不成其为网络连接',
  SEGMENTS_MISSING: '缺组成段——线路由一个或多个网络连接按明确顺序组成',
  SEGMENT_BLANK: '段链里有空身份——序即语义，缺一环链就断',
  APPLICABLE_SCOPE_MISSING: '缺适用范围——这一版没说清自己管哪里',
  EFFECTIVE_TIME_MISSING: '缺生效时间——零时刻不是一个能用的生效边界',
  EFFECTIVE_RANGE_REVERSED: '生效区间倒序或为空——半开区间两端相等即不覆盖任何时点',
  TARGET_KIND_UNKNOWN: '适用对象类别不在封闭集（NODE / CONNECTION / LINE）',
  ADJUSTMENT_KIND_UNKNOWN:
    '调整种类不在封闭集（SUSPENSION / CLOSURE / RESUMPTION / SCOPE_ADJUSTMENT）',
  SOURCE_MISSING: '缺来源——调整必须记录来源，否则这条陈述说不出自己凭什么',
};

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
