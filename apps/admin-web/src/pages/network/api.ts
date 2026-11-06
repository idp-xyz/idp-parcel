// 本目录 fetch 出口:网络目录运营查阅(GET /network-catalog,ADR-0077、票
// master-data-wiring/03)。七族按 ?family= 分派;管理台网络目录页只请求六族,
// 服务区域族由服务区域页固定 family=service-area 请求(MCP-4 终表与 MCP-3 裁决)。
// 读口已迁到 ADR-0144(票 catalogue-read-pagination/03):翻页走游标,检索与各族筛选维下推。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';
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

/** 服务区域覆盖：一个国家，可带邮编前缀与揽收（始发）、派送（目的）网点。 */
export interface ServiceAreaCoverage {
  country: string;
  postalPrefixes?: string[];
  originNodes?: string[];
  destinationNodes?: string[];
}

export interface NetworkVersionRecord {
  code?: string;
  version: number;
  /** 网点（物流节点）这一版的显示名称（ADR-0353）；没登即缺席，照旧以 code 称呼。只有网点族带这一格。 */
  displayName?: string;
  businessTimezone?: string;
  effectiveFrom?: string;
  effectiveTo?: string;
  fromNode?: string;
  toNode?: string;
  segments?: string[];
  applicableScope?: string;
  /** 路线策略的排序形态；登记方没声明即缺席。 */
  rankingForm?: string;
  /** 服务区域的地理覆盖；没登覆盖即缺席。 */
  coverage?: ServiceAreaCoverage;
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
  page: CataloguePage;
}

/**
 * 网络目录一问的条件。family 是册子选择器(ADR-0144 决定四):保持原义、不兼作筛选维,但与检索、筛选、排序并在同一组条件里——
 * 游标摘要覆盖它,页面的条件签名也就覆盖它,换族即回第一页。各族点名的可排维与筛选维各不相同,以读口的声明为准,这里不抄一份。
 */
export type NetworkCatalogConditions = CatalogueConditions & {
  readonly selectors: { readonly family: NetworkCatalogFamily };
};

export function networkCatalogConditions(
  family: NetworkCatalogFamily,
  conditions: Omit<CatalogueConditions, 'selectors'> = {},
): NetworkCatalogConditions {
  return { ...conditions, selectors: { family } };
}

/** 游标缺即第一页。 */
export function listNetworkCatalog(
  conditions: NetworkCatalogConditions,
  after: string | null = null,
): Promise<ApiResult<NetworkCatalogListResponseBody>> {
  return exchangeMasterData<NetworkCatalogListResponseBody>(
    catalogueRequestPath('/network-catalog', conditions, after),
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
 * 一样送得出去。所以这里与读口同形：族是封闭集里的一个参数（读口经 `networkCatalogConditions` 带上）。
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
  REGISTERED: '已保存',
  REFUSED: '没有保存：内容缺了或填错了一项（见下方原因），改好后重新提交；已有的版本不受影响',
};

/**
 * 受理门拒绝理由（`application.CatalogRefusalReason` 原名），逐格中文。
 *
 * 逐格分开呈现而不折成一句「内容不合法」：各格的续办动作不同——缺时区要去补时区，两端
 * 相同要改端点，段链缺环要重给顺序，折成一格会让登记方去补错东西。每句先说哪里错、再说怎么改。
 */
export const registrationRefusalReasonLabels: Record<string, string> = {
  TENANT_MISSING: '没写本公司的编号；补上后重新提交',
  IDENTITY_MISSING: '没写编码，看不出登的是哪一个；补上编码',
  VERSION_MISSING: '没写版本号；第一次登填 1，以后每改一次加 1',
  TIMEZONE_MISSING: '没写时区；网点、运输连接和线路都要写所在时区，如 Asia/Shanghai',
  ENDPOINT_MISSING: '运输连接缺出发网点或到达网点；两端都要写',
  ENDPOINTS_NOT_DISTINCT: '出发网点和到达网点是同一个；改成两个不同的网点',
  CONNECTION_CODE_CARRIES_BASIS_SEPARATOR: '连接编码里带了「+」；换一个不带「+」的编码',
  SEGMENTS_MISSING: '线路没写由哪些运输连接组成；按先后顺序列出',
  SEGMENT_BLANK: '线路的运输连接里有一项是空的；补上或删掉那一项',
  APPLICABLE_SCOPE_MISSING: '没写适用服务；补上这一版管哪类服务',
  EFFECTIVE_TIME_MISSING: '没写生效时间；补上从何时起生效',
  EFFECTIVE_RANGE_REVERSED: '截止时间不晚于生效时间；把截止时间改到生效时间之后，或不填截止时间',
  TARGET_KIND_UNKNOWN: '适用对象类别写错了；只能填 NODE（网点）、CONNECTION（运输连接）或 LINE（线路）',
  ADJUSTMENT_KIND_UNKNOWN:
    '调整种类写错了；只能填 SUSPENSION（临时停运）、CLOSURE（关闭）、RESUMPTION（恢复）或 SCOPE_ADJUSTMENT（范围调整）',
  SOURCE_MISSING: '停运或恢复没写消息来源；补上是谁通知的',
  RANKING_FORM_UNKNOWN: '排序方式写错了；填 COST_SINGLE_DIMENSION（成本最低优先），或不填',
  COVERAGE_MALFORMED: '覆盖范围写法不对：国家要用两位代码，邮编前缀要配国家一起写；改覆盖那一项',
  COVERAGE_COUNTRY_NOT_IN_CATALOGUE: '覆盖国家不在国家和地区目录里；换成目录里的两位代码，目录可在“基础数据”页查看',
  NODE_ROLE_BLANK: '揽收或派送网点里有一项是空的；补上网点编码',
  NODE_ROLE_DUPLICATED: '同一个网点在揽收或派送里写了两次；删掉重复的那一个',
  NODE_ROLES_WITHOUT_COVERAGE: '没写覆盖国家就写了揽收或派送网点；先写覆盖国家',
  CALENDAR_CONTENT_OUT_OF_RANGE: '日历里有数不对：截单时间要在当天之内，处理、衔接和派送时长不能是负数；改那个数',
  COST_BASIS_MALFORMED: '逐段成本写法不对：段号要在线路之内、同一段只写一条、种类与引用都要填；改那一行',
  LEG_ZONE_MALFORMED: '段计价区域写法不对：段号不能为负、同一段只写一次、区域与依据都要填；改那一行',
  LEG_ZONE_WITHOUT_COST_BASIS: '给一段写了计价区域，但这一段没挂成本，或线路版本还没登；先给这一段挂上成本',
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

/**
 * 成计划行上的一段（端点 plannedLegBody，ADR-0217）。kind 取 network-routing `domain.PlannedLegKind` 的原词（BETWEEN_NODES /
 * DELIVERY）；交付段的 to 是交付范围（服务区域版本引用），不是网点。各格都是字符串，时刻是 UTC 文本。
 */
export interface PlannedLegRecord {
  reference: string;
  kind: string;
  from: string;
  to: string;
  responsible: string;
  earliest: string;
  latest: string;
  windowBasis: string;
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
  /** 计划各段，按计划顺序；仅成计划行在场，无路可走行缺席。 */
  legs?: PlannedLegRecord[];
  applicability?: RoutePlanApplicability;
  recordedAt: string;
}

export interface InitialRouteListResponseBody {
  outcome: 'INITIAL_ROUTES_LISTED';
  judgments: InitialRouteRecord[];
  page: CataloguePage;
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
  page: CataloguePage;
}

export function listInitialRoutes(): Promise<ApiResult<InitialRouteListResponseBody>> {
  return exchangeMasterData<InitialRouteListResponseBody>('/route-plans?register=initial-route');
}

export function listRouteReassessments(): Promise<ApiResult<RouteReassessmentListResponseBody>> {
  return exchangeMasterData<RouteReassessmentListResponseBody>('/route-plans?register=reassessment');
}

// ——以下为改路建议读面与授权改路决定口（ADR-0284 决定一、六；票 docs-consistency/code/29）。形状以
// internal/networkrouting/adapters/http 的 query_reroute_suggestions.go 与 decide_reroute.go 为准，此处只做镜像不虚构。
// 写行走 postMasterData：它挂操作者渠道「运营决定」能力面的授权改路一种，与查阅那一行不是同一个准入。

/** 建议里的一条候选：结局与依据是建议形成那一刻的评估，不是此刻的。 */
export interface RerouteSuggestionCandidate {
  id: string;
  /** 封闭三格原词：QUALIFIED / ELIMINATED / EVIDENCE_UNKNOWN。 */
  outcome: string;
  reason?: string;
}

export interface RerouteSuggestionDecision {
  candidate: string;
  newPlanVersion: string;
  decidedBy: string;
  basis: string;
  decidedAt: string;
}

export interface RerouteSuggestionRecord {
  correlationId: string;
  customerAccountId: string;
  shipmentRequestId: string;
  acceptanceBaseline: string;
  declaredParcelId: string;
  servicePurpose: string;
  reviewedPlan?: string;
  lapseBasis?: string;
  blockers: string[];
  candidates: RerouteSuggestionCandidate[];
  suggestedAt: string;
  triggerOccurredAt?: string;
  /** 封闭三格原词：PENDING / DECIDED / SUPERSEDED。 */
  state: string;
  decision?: RerouteSuggestionDecision;
}

export interface RerouteSuggestionListResponseBody {
  outcome: 'REROUTE_SUGGESTIONS_LISTED';
  suggestions: RerouteSuggestionRecord[];
}

export function listRerouteSuggestions(): Promise<ApiResult<RerouteSuggestionListResponseBody>> {
  return exchangeMasterData<RerouteSuggestionListResponseBody>('/route-reroute-suggestions');
}

/** 一次授权改路决定的载荷。**没有租户、没有决定方**——身份从操作者信封来，服务端对载荷里出现的身份键一律按未知键拒。 */
export interface RerouteDecisionRequest {
  declaredParcelId: string;
  suggestion: string;
  candidate: string;
  basis: string;
}

export interface RerouteDecisionBody {
  suggestion: string;
  declaredParcelId: string;
  candidate: string;
  originalPlan: string;
  newPlanVersion: string;
  effectiveFromNode: string;
  effectiveFrom: string;
  mode: string;
  decidedBy: string;
  basis: string;
  decidedAt: string;
}

/** 决定口的封闭响应形状：decision 随成立与已决定出现；candidateReason 只随此刻重判不成立的三格；undecidedReason 只随未决。 */
export interface RerouteDecisionResponseBody {
  outcome: string;
  candidateReason?: string;
  undecidedReason?: string;
  continuationReference?: string;
  decision?: RerouteDecisionBody;
}

export function decideReroute(request: RerouteDecisionRequest): Promise<ApiResult<RerouteDecisionResponseBody>> {
  return postMasterData<RerouteDecisionResponseBody>('/route-reroute-decisions', request);
}

// ——以下为并列读面与并列裁决口（ADR-0309 决定二、六；票 docs-consistency/code/97）。形状以 internal/networkrouting/adapters/http 的
// decide_initial_route.go 为准，此处只做镜像不虚构。裁决口与授权改路同属操作者渠道「运营决定」能力面的授权改路一种授予。

/** 并列的一家与它打平的金额与币种。 */
export interface InitialRouteTieCost {
  candidate: string;
  amountMinor: number;
  currency: string;
}

export interface InitialRouteTieDecision {
  candidate: string;
  planVersion: string;
  decidedBy: string;
  basis: string;
  decidedAt: string;
}

export interface InitialRouteTieRecord {
  customerAccountId: string;
  shipmentRequestId: string;
  acceptanceBaseline: string;
  declaredParcelId: string;
  servicePurpose: string;
  /** 入账那一刻的评估，不是此刻的。 */
  candidates: RerouteSuggestionCandidate[];
  tied: InitialRouteTieCost[];
  acceptedAt: string;
  settledAt: string;
  /** 封闭三格原词：PENDING / DECIDED / FORMED_OTHERWISE。 */
  state: string;
  decision?: InitialRouteTieDecision;
}

export interface InitialRouteTieListResponseBody {
  outcome: 'INITIAL_ROUTE_TIES_LISTED';
  ties: InitialRouteTieRecord[];
}

export function listInitialRouteTies(): Promise<ApiResult<InitialRouteTieListResponseBody>> {
  return exchangeMasterData<InitialRouteTieListResponseBody>('/route-initial-route-ties');
}

/** 一次并列裁决的载荷。**没有租户、没有决定方**——身份从操作者信封来，服务端对载荷里出现的身份键一律按未知键拒。 */
export interface InitialRouteTieDecisionRequest {
  declaredParcelId: string;
  candidate: string;
  basis: string;
}

/** 裁决口的封闭响应形状：plan 只随成立；existing 随已有结果；registeredReason 只随没有并列登记且登记行存在。 */
export interface InitialRouteTieDecisionResponseBody {
  outcome: string;
  registeredReason?: string;
  candidateReason?: string;
  undecidedReason?: string;
  continuationReference?: string;
  plan?: { version: string; candidate: string; effectiveFrom: string; judgedAt: string };
  existing?: { conclusion: string; planVersion?: string };
}

export function decideInitialRouteTie(
  request: InitialRouteTieDecisionRequest,
): Promise<ApiResult<InitialRouteTieDecisionResponseBody>> {
  return postMasterData<InitialRouteTieDecisionResponseBody>('/route-initial-route-decisions', request);
}
