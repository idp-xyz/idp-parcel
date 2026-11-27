import type { RegistrationResponseBody } from '../../components/registration';
import { exchangeMasterData, postMasterData, postWorkFact, type ApiResult } from '../catalogue-api';
import type { CatalogueConditions, CataloguePage } from '../catalogue-query';
import { catalogueDefaultPageSize, cataloguePageSizes, catalogueRequestPath } from '../catalogue-query';
import { asCatalogueRegistration, asStockImportWrite, type CatalogueRegistrationBody, type StockImportWriteBody } from './answers';
import type { PartialShipmentAnswer, ShortPickDisposalAnswer } from './partial-shipment';
import type { AllocationExceptionRecord, DueWaveRulesBody, WaveRuleRecord, WaveRuleRunBody } from './wave-rule';
import {
  catalogueReadPath,
  catalogueWritePaths,
  type CatalogueFamily,
} from './rows';

export type { ApiResult } from '../catalogue-api';

export interface WarehouseRecord {
  warehouseCode: string;
  revision: number;
  name: string;
  kind: string;
  address: { country: string; text: string };
  businessTimeZone?: string;
  colocatedNode?: string;
  deactivated: boolean;
  registrant: string;
  registeredAt: string;
}

export interface ZoneRecord {
  warehouseCode: string;
  zoneCode: string;
  revision: number;
  name: string;
  usage: string;
  deactivated: boolean;
  registrant: string;
  registeredAt: string;
}

export interface LocationTypeRecord {
  locationTypeCode: string;
  revision: number;
  name: string;
  deactivated: boolean;
  registrant: string;
  registeredAt: string;
}

export interface LocationRecord {
  warehouseCode: string;
  locationCode: string;
  revision: number;
  zoneCode: string;
  locationTypeCode?: string;
  priority?: number;
  deactivated: boolean;
  registrant: string;
  registeredAt: string;
}

export interface PackagingRecord {
  packagingMaterialCode: string;
  revision: number;
  name: string;
  dimensions: { length: string; width: string; height: string; unit: string };
  weight: { value: string; unit: string };
  deactivated: boolean;
  registrant: string;
  registeredAt: string;
}

export interface CatalogueListBody {
  outcome: string;
  warehouses?: WarehouseRecord[];
  zones?: ZoneRecord[];
  locationTypes?: LocationTypeRecord[];
  locations?: LocationRecord[];
  packagingMaterials?: PackagingRecord[];
  page: CataloguePage;
}

export interface StockImportGoodsLineRecord {
  locationCode: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
  batchNumber?: string;
  expiryDate?: string;
  labelAlias?: string;
  declaredInboundAt?: string;
}

export interface StockImportCartonLineRecord {
  locationCode: string;
  cartonNumber: string;
  contents: ReadonlyArray<{ goodsCode: string; quantity: number }>;
  declaredInboundAt?: string;
}

export interface StockImportRecord {
  importId: string;
  ownerAccount: string;
  warehouseCode: string;
  reason: string;
  status: string;
  submitter: string;
  submittedAt: string;
  goodsLines: readonly StockImportGoodsLineRecord[];
  cartonLines: readonly StockImportCartonLineRecord[];
  poster?: string;
  dutyRevision?: number;
  postedAt?: string;
  goodsMovementId?: string;
  cartonMovementId?: string;
  rejector?: string;
  rejectedAt?: string;
  rejectionNote?: string;
}

export interface StockImportListBody {
  outcome: string;
  imports: StockImportRecord[];
  page: CataloguePage;
}

export interface UnclaimedArrivalRecord {
  arrivalId: string;
  factId: string;
  origin: string;
  kind: string;
  status: string;
  warehouseCode: string;
  device: string;
  operator: string;
  occurredAt: string;
  receivedAt: string;
  observation?: string;
  ownerClue?: string;
  preAdviceClue?: string;
  storageLocation?: string;
  /** 取消或完结之后又到的收货转来的那一次收货：货的内容只在这里，到货本身没有观察文字。 */
  divertedReceipt?: {
    factId: string;
    occurredAt: string;
    lines?: ReadonlyArray<{
      observed?: string;
      goodsCode?: string;
      cartonNumber?: string;
      unit?: string;
      stockState?: string;
      quantity?: number;
      batchNumber?: string;
      expiryDate?: string;
    }> | null;
  };
  postingReferences?: string[];
  claims?: ReadonlyArray<{
    claim: { decisionId: string; ownerAccount: string; preAdviceId?: string; operator: string; claimedAt: string };
    revocation?: { decisionId: string; reason?: string; operator: string; revokedAt: string };
  }>;
  disposition?: {
    method: string;
    ruleWarehouseCode: string;
    ruleRevision: number;
    note?: string;
    operator: string;
    disposedAt: string;
  };
}

export interface UnclaimedArrivalListBody {
  outcome: string;
  arrivals: UnclaimedArrivalRecord[];
  page: CataloguePage;
}

export interface UnclaimedArrivalOneBody {
  outcome: string;
  arrivalId: string;
  found: boolean;
  arrival?: UnclaimedArrivalRecord;
}

export interface DisposalRuleRecord {
  warehouseCode: string;
  revision: number;
  claimWindowSeconds: number;
  disposalMethods: string[];
  registrant: string;
  registeredAt: string;
}

export interface DisposalRuleListBody {
  outcome: string;
  disposalRules: DisposalRuleRecord[];
  page: CataloguePage;
}

export interface InternalJobLineRecord {
  ownerAccount: string;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  quantity?: number;
  sourceLocation?: string;
  targetLocation?: string;
  cartonNumber?: string;
  posted: number;
  postedInbound: number;
  shortOnBook: number;
  held: number;
  cartonMissing?: boolean;
  /** 这一行入账记下的流水；没入账的行没有。 */
  movementId?: string;
  /** 这一行没入账的部分形成的差异。 */
  discrepancyId?: string;
}

export interface InternalJobRecord {
  factId: string;
  kind: string;
  warehouse: string;
  occurredAt: string;
  deviceSequence: number;
  receivedAt: string;
  lines: InternalJobLineRecord[];
}

export interface InternalJobListBody {
  outcome: string;
  jobs: InternalJobRecord[];
  page: CataloguePage;
}

export interface InternalJobOneBody {
  outcome: string;
  factId: string;
  found: boolean;
  job?: InternalJobRecord;
}

export interface PreAdviceGoodsLineRecord {
  goodsCode: string;
  unit: string;
  quantity: number;
  batchNumber?: string;
  expiryDate?: string;
  cartonCount?: number;
}

export interface PreAdviceCartonRecord {
  cartonNumber: string;
  lines: ReadonlyArray<{ goodsCode: string; quantity: number }>;
}

export interface PreAdviceNoticeRecord {
  version: number;
  expectedAt: string;
  arrivalClue?: string;
  ownerReference?: string;
  submissionPath: string;
  evidenceReference?: string;
  registrant: string;
  recordedAt: string;
}

export interface PreAdviceReceiptRecord {
  factId: string;
  shape: string;
  outcome: string;
  device: string;
  operator: string;
  occurredAt: string;
  receivedAt: string;
  unclaimedArrivalId?: string;
  lines?: ReadonlyArray<{
    observed: string;
    goodsCode?: string;
    unit: string;
    stockState: string;
    quantity: number;
    locationCode: string;
    result?: string;
    unpostedReason?: string;
  }>;
  cartonLines?: ReadonlyArray<{
    cartonNumber: string;
    warehouseCartonNumber?: string;
    damaged?: boolean;
    locationCode: string;
    result?: string;
    unpostedReason?: string;
  }>;
  discrepancy?: {
    discrepancyId: string;
    lines: ReadonlyArray<{ kind: string; quantity?: number; goodsCode?: string; observed?: string; cartonNumber?: string }>;
    conclusion?: { verdict: string; note?: string; concludedAt: string; operator?: string };
  };
}

export interface PreAdviceRecord {
  preAdviceId: string;
  ownerAccount: string;
  warehouseCode: string;
  arrivalMode: string;
  inboundMode: string;
  status: string;
  documentNumber?: string;
  numberingAbsence?: string;
  submissionPath: string;
  evidenceReference?: string;
  registrant: string;
  acceptedAt: string;
  goodsLines?: readonly PreAdviceGoodsLineRecord[];
  cartons?: readonly PreAdviceCartonRecord[];
  currentNotice?: PreAdviceNoticeRecord;
  notices?: readonly PreAdviceNoticeRecord[];
  cumulativeReceived?: ReadonlyArray<{ goodsCode?: string; cartonNumber?: string; quantity: number }>;
  receipts?: readonly PreAdviceReceiptRecord[];
  closure?: {
    closedAt: string;
    operator?: string;
    note?: string;
    discrepancyId?: string;
    shortages?: ReadonlyArray<{ goodsCode?: string; cartonNumber?: string; quantity?: number }>;
  };
  unclaimedArrival?: boolean;
}

export interface PreAdviceOneBody {
  outcome: string;
  ownerAccount: string;
  preAdviceId: string;
  found: boolean;
  preAdvice?: PreAdviceRecord;
}

export interface PreAdviceListBody {
  outcome: string;
  ownerAccount: string;
  preAdvices: PreAdviceRecord[];
  page: CataloguePage;
}

export interface GoodsDimensionsRecord {
  length: string;
  width: string;
  height: string;
  unit: string;
}

export interface GoodsWeightRecord {
  value: string;
  unit: string;
}

export interface GoodsRecord {
  ownerAccount: string;
  goodsCode: string;
  revision: number;
  name: string;
  baseUnit: string;
  packagingLevels?: ReadonlyArray<{ name: string; factor: number }>;
  trackBatch: boolean;
  trackExpiry: boolean;
  aliases?: ReadonlyArray<{ code: string; type: string; platform?: string; validity?: { from?: string; to?: string } }>;
  composite: boolean;
  composition?: { outboundMode: string; components: ReadonlyArray<{ code: string; quantity: number }> };
  declaration?: { name?: string; hsCode?: string; originCountry?: string; unitPrice?: string; currency?: string };
  declaredMeasurements?: ReadonlyArray<{ level: string; dimensions?: GoodsDimensionsRecord; weight?: GoodsWeightRecord }>;
  warehouseMeasurements?: ReadonlyArray<{
    level: string;
    measured: boolean;
    current?: {
      factId: string;
      warehouseCode: string;
      device: string;
      operator: string;
      occurredAt: string;
      dimensions: GoodsDimensionsRecord;
      weight: GoodsWeightRecord;
    };
    declared?: { source: string; dimensions?: GoodsDimensionsRecord; weight?: GoodsWeightRecord };
  }>;
  submissionPath?: string;
  evidenceReference?: string;
  registrant: string;
  registeredAt: string;
}

export interface GoodsCurrentBody {
  outcome: string;
  ownerAccount: string;
  goodsCode: string;
  found: boolean;
  goods?: GoodsRecord;
}

export interface GoodsListBody {
  outcome: string;
  ownerAccount: string;
  goods: GoodsRecord[];
  page: CataloguePage;
}

export type StockViewKind = 'cells' | 'cartons' | 'summary' | 'holds' | 'movements';

export interface StockReadBody {
  outcome: string;
  cells?: ReadonlyArray<{
    warehouse: string;
    location?: string;
    goodsCode: string;
    unit: string;
    stockState: string;
    labelAlias?: string;
    batchNumber?: string;
    expiryDate?: string;
    inboundBatchId: string;
    inboundAt?: string;
    quantity: number;
    allocatable?: boolean;
    zoneUsage?: string;
    frozen: number;
    locked?: number;
    available: number;
    ageDays?: number;
    age?: string;
  }>;
  cartons?: ReadonlyArray<{
    warehouse: string;
    cartonNumber: string;
    location?: string;
    inboundAt: string;
    held: boolean;
    bill?: ReadonlyArray<{ goodsCode: string; quantity: number }>;
    ageDays?: number;
    age?: string;
  }>;
  summary?: ReadonlyArray<{
    warehouse: string;
    goodsCode: string;
    unit: string;
    stockState: string;
    onHand: number;
    allocatable: number;
    frozen: number;
    reserved: number;
    locked: number;
    available: number;
  }>;
  holds?: ReadonlyArray<{
    kind: string;
    warehouse: string;
    goodsCode?: string;
    unit?: string;
    stockState?: string;
    location?: string;
    cartonNumber?: string;
    quantity: number;
    reason?: string;
    operator?: string;
    frozenAt?: string;
    /** 冻结号或盘点号。 */
    reference?: string;
  }>;
  movements?: ReadonlyArray<{
    warehouse: string;
    reason: string;
    source?: string;
    occurredAt: string;
    postedAt?: string;
    lines?: ReadonlyArray<{ location?: string; goodsCode: string; unit: string; stockState: string; delta: number }>;
  }>;
  /** 只在看流水时有：流水按游标翻页。 */
  page?: CataloguePage;
  /** 库存格、整箱、汇总、占用按页取时有。不带页码的读口不答这一格，行是整份。 */
  book?: { page: number; pageSize: number; total: number };
}

/** 册别在条件的选择器里，请求路径按它选；选择器本身不进查询串。 */
export function listWarehouseCatalogue(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<CatalogueListBody>> {
  return exchangeMasterData<CatalogueListBody>(catalogueReadPath(conditions, after));
}

export function listStockImports(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<StockImportListBody>> {
  return exchangeMasterData<StockImportListBody>(
    catalogueRequestPath('/warehouse-fulfillment-stock-imports', conditions, after),
  );
}

export function listUnclaimedArrivals(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<UnclaimedArrivalListBody>> {
  return exchangeMasterData<UnclaimedArrivalListBody>(
    catalogueRequestPath('/warehouse-fulfillment-unclaimed-arrivals', conditions, after),
  );
}

export function listDisposalRules(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<DisposalRuleListBody>> {
  return exchangeMasterData<DisposalRuleListBody>(
    catalogueRequestPath('/warehouse-fulfillment-unclaimed-disposal-rules', conditions, after),
  );
}

export function listInternalJobs(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<InternalJobListBody>> {
  return exchangeMasterData<InternalJobListBody>(
    catalogueRequestPath('/warehouse-fulfillment-internal-jobs', conditions, after),
  );
}

export function getInternalJob(factId: string): Promise<ApiResult<InternalJobOneBody>> {
  const params = new URLSearchParams({ factId });
  return exchangeMasterData<InternalJobOneBody>(`/warehouse-fulfillment-internal-job?${params.toString()}`);
}

export function listOwnerPreAdvices(
  ownerAccount: string,
  keyword: string,
  after: string | null = null,
  pageSize?: number,
): Promise<ApiResult<PreAdviceListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  if (after !== null && after !== '') params.set('after', after);
  if (pageSize !== undefined) params.set('pageSize', String(pageSize));
  return exchangeMasterData<PreAdviceListBody>(`/warehouse-fulfillment-inbound-pre-advices?${params.toString()}`);
}

export function listOwnerGoods(
  ownerAccount: string,
  keyword: string,
  after: string | null = null,
  pageSize?: number,
  sort?: string,
): Promise<ApiResult<GoodsListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  if (after !== null && after !== '') params.set('after', after);
  if (pageSize !== undefined && pageSize > 0) params.set('pageSize', String(pageSize));
  if (sort !== undefined && sort !== '') params.set('sort', sort);
  return exchangeMasterData<GoodsListBody>(`/warehouse-fulfillment-goods?${params.toString()}`);
}

export function queryOwnerStock(
  ownerAccount: string,
  warehouseCode: string,
  view: StockViewKind,
  after: string | null = null,
  pageSize?: number,
  page?: number,
  keyword?: string,
  state?: string,
): Promise<ApiResult<StockReadBody>> {
  const params = new URLSearchParams({ ownerAccount, view });
  if (warehouseCode.trim() !== '') params.set('warehouseCode', warehouseCode.trim());
  if (keyword !== undefined && keyword.trim() !== '') params.set('q', keyword.trim());
  if (state !== undefined && state !== '') {
    if (view === 'cells' || view === 'summary') params.set('stockState', state);
    else if (view === 'cartons') params.set('held', state === 'HELD' ? 'true' : 'false');
    else if (view === 'holds') params.set('holdKind', state);
  }
  // 流水收 after 与 pageSize。其余四种收 page 与 pageSize，带了 after 会被拒。
  if (view === 'movements') {
    if (after !== null && after !== '') params.set('after', after);
    if (pageSize !== undefined) params.set('pageSize', String(pageSize));
  } else if (pageSize !== undefined) {
    params.set('pageSize', String(pageSize));
    params.set('page', String(page ?? 1));
  }
  return exchangeMasterData<StockReadBody>(`/warehouse-fulfillment-stock?${params.toString()}`);
}

export interface OutboundRequestLineRecord {
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
}

export interface OutboundRequestRecord {
  requestId: string;
  documentNumber?: string;
  numberingAbsence?: string;
  ownerAccount: string;
  warehouseCode: string;
  origin: string;
  requestKey: string;
  dispatch: string;
  service?: string;
  recipient: string;
  address: string;
  basis?: string;
  operator?: string;
  status: string;
  parentRequestId?: string;
  platformStocking: boolean;
  /** 这张单提交时写明的部分发货规则；没写明就不带。此刻适用哪一层看明细读口的 `partialShipment`。 */
  partialShipment?: string;
  prefetchShipmentId?: string;
  prefetchDeclaredParcelId?: string;
  receivedAt: string;
  lines: OutboundRequestLineRecord[];
  reservedQuantity: number;
  wantedQuantity: number;
  /** 只在这张单仍是已预占、最近一次放波次时分配不出被跳过时带；列表读口带在这里，明细读口带在答复的 `allocationException`。 */
  allocationException?: AllocationExceptionRecord;
}

export interface OutboundRequestListBody {
  outcome: string;
  requests: OutboundRequestRecord[];
  page: CataloguePage;
}

export interface WaveTaskRecord {
  taskId: string;
  requestId: string;
  /** 这条任务所属出库单的货主。商品编码只在一位货主之内唯一，合并同一种货时要靠它分开。 */
  ownerAccount?: string;
  locationCode: string;
  /** 这条任务所在库位此刻所属的库区；认不出不带。按库区派工靠它对上人。 */
  zoneCode?: string;
  stagingLocation?: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
  picked: number;
  status: string;
}

export interface WaveZoneAssigneeRecord {
  zoneCode: string;
  /** 操作者标识「发行方#sub」。 */
  assignee: string;
}

export interface WaveRecord {
  waveId: string;
  warehouseCode: string;
  operator: string;
  prefetch: boolean;
  /** RELEASED、PICKING、COMPLETED、CANCELLED，口径见 wave-flow.ts「波次的状态由读口答」。 */
  status: string;
  releasedAt: string;
  /** 单品单件波次：放出时每张出库单要拣的都只有一件。拣货时同库位同商品合成一格拿，复核打包时扫货就能配单。 */
  singleUnit?: boolean;
  /** 合并拣货的波次：放出时选的。货按库位、商品合在一起拿，拿回来再按出库单分拣。 */
  batchPick?: boolean;
  /** 放出这个波次的成波规则；由人点单放的波次不带。 */
  ruleCode?: string;
  /** 这个波次派给了谁拣（操作者标识，「发行方#sub」）；没派、没人领不带。 */
  assignee?: string;
  /** 单独派了人的库区；没单独派的库区照 `assignee` 走。一个都没有不带。 */
  zoneAssignees?: WaveZoneAssigneeRecord[];
  requests: string[];
  /** `requests` 里已经撤出的那几张：它们在这个波次里的任务作废了，请求回到了已预占。 */
  withdrawnRequests: string[];
  tasks: WaveTaskRecord[];
}

export interface WaveOneBody {
  outcome: string;
  /** 没有这个波次时不带。 */
  wave?: WaveRecord;
}

export interface WaveListBody {
  outcome: string;
  waves: WaveRecord[];
  page: CataloguePage;
}

export interface ChargeOccurrenceRecord {
  occurrenceId: string;
  version: number;
  supersedes?: number;
  correction: string;
  kind: string;
  ownerAccount: string;
  warehouseCode: string;
  sourceFact: string;
  businessTime: string;
  weightUnit?: string;
  volumeUnit?: string;
  measures: ReadonlyArray<{ unit: string; quantity?: string; unmeasured?: boolean; class?: string }>;
  attributes: ReadonlyArray<{ name: string; value: string }>;
  lineCount: number;
}

export interface ChargeOccurrenceListBody {
  outcome: string;
  occurrences: ChargeOccurrenceRecord[];
  page: CataloguePage;
}

export interface WarehouseAdoptionRecord {
  ownerAccount: string;
  warehouseCode: string;
  revision: number;
  effectiveAt: string;
  account: string;
  pricingScope: string;
  basis: string;
  registrant: string;
}

export interface WarehouseAdoptionListBody {
  outcome: string;
  adoptions: WarehouseAdoptionRecord[];
  page: CataloguePage;
}

export interface WarehouseChargeRequestRecord {
  occurrenceId: string;
  version: number;
  status: string;
  chargeId?: string;
  evaluationId?: string;
  requestId?: string;
  account?: string;
  settlementCurrency?: string;
  pricingScope?: string;
  pricingBasisAt?: string;
}

export interface WarehouseChargeRequestListBody {
  outcome: string;
  requests: WarehouseChargeRequestRecord[];
  page: CataloguePage;
}

export interface WarehousePlanRecord {
  id: string;
  version: string;
  scope: string;
  currency: string;
  declaredValue: string;
  effectiveFrom: string;
  groupCount: number;
  ruleCount: number;
}

export interface WarehousePlanListBody {
  outcome: string;
  plans: WarehousePlanRecord[];
  page: CataloguePage;
}

export function listOutboundRequests(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<OutboundRequestListBody>> {
  return exchangeMasterData<OutboundRequestListBody>(
    catalogueRequestPath('/warehouse-fulfillment-outbound-request-list', conditions, after),
  );
}

export function listWaves(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<WaveListBody>> {
  return exchangeMasterData<WaveListBody>(
    catalogueRequestPath('/warehouse-fulfillment-wave-list', conditions, after),
  );
}

export interface PickDifferenceRecord {
  differenceId: string;
  /** PENDING 待盘点，CLOSED 已由一份盘点了结。 */
  status: string;
  /** 少拣的数量。 */
  quantity: number;
  taskId: string;
  waveId: string;
  requestId: string;
  ownerAccount: string;
  warehouseCode: string;
  locationCode: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  /** 那条任务应拣与实拣的数量。 */
  taskQuantity: number;
  picked: number;
  recordedAt?: string;
  closedAt?: string;
  closedByCount?: string;
}

export interface PickDifferenceListBody {
  outcome: string;
  differences: PickDifferenceRecord[];
  page: CataloguePage;
}

/** 少拣留下的拣货差异。检索覆盖差异号、库位、商品、出库请求号与波次号。 */
export function listPickDifferences(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<PickDifferenceListBody>> {
  return exchangeMasterData<PickDifferenceListBody>(
    catalogueRequestPath('/warehouse-fulfillment-pick-differences', conditions, after),
  );
}

export interface ToteRecord {
  warehouseCode: string;
  toteCode: string;
  /** FREE 空着，IN_USE 绑着一张出库单，RETIRED 已停用。 */
  state: string;
  requestId?: string;
  boundAt?: string;
  registeredBy: string;
  registeredAt: string;
  retiredBy?: string;
  retiredAt?: string;
}

export interface ToteListBody {
  outcome: string;
  totes: ToteRecord[];
  page: CataloguePage;
}

/** 周转箱名册。检索覆盖箱号与绑着的出库请求号。 */
export function listTotes(conditions: CatalogueConditions, after: string | null = null): Promise<ApiResult<ToteListBody>> {
  return exchangeMasterData<ToteListBody>(catalogueRequestPath('/warehouse-fulfillment-totes', conditions, after));
}

export interface SingleUnitMatchBody {
  /** SINGLE_UNIT_MATCHED 配上了；SINGLE_UNIT_NOT_MATCHED 没有等复核的单品单件出库单要这种货。 */
  outcome: string;
  requestId?: string;
  documentNumber?: string;
  ownerAccount?: string;
  goodsCode?: string;
  /** 这位货主的这种货此刻还有几张单等着复核，含配上的这一张。 */
  waiting?: number;
}

/**
 * 复核打包台扫到一件货，给它配一张单品单件的出库单：本仓已拣完、等复核、要这种货的单里等得最久的那一张。
 * 码可以是商品编码，也可以是货主登记过的条码、平台商品码。这是读，配上的单没有被占住。
 */
export function matchSingleUnitOrder(warehouse: string, code: string): Promise<ApiResult<SingleUnitMatchBody>> {
  const params = new URLSearchParams({ warehouseCode: warehouse, code });
  return exchangeMasterData<SingleUnitMatchBody>(`/warehouse-fulfillment-single-unit-match?${params.toString()}`);
}

export interface SortPlanLineRecord {
  goodsCode: string;
  unit: string;
  /** 拣出来的件数、已经投放进箱的件数、拣货的人还没拿回来的件数，都按基本单位。 */
  picked: number;
  sorted: number;
  open: number;
}

export interface SortPlanRequestRecord {
  requestId: string;
  documentNumber?: string;
  ownerAccount: string;
  status: string;
  /** 这张出库单此刻绑着的周转箱；还没有箱是空数组。 */
  totes: string[];
  lines: SortPlanLineRecord[];
}

export interface SortPlanBody {
  /** SORT_PLAN_FOUND；这个波次不用分拣（逐单拣或单品单件）答 SORT_NOT_REQUIRED；没有这个波次答 WAVE_NOT_FOUND。 */
  outcome: string;
  waveId?: string;
  warehouseCode?: string;
  status?: string;
  /** 波次里还没打包、没取消的出库单；一张都没有时不带。 */
  requests?: SortPlanRequestRecord[];
}

/** 一个合并拣货的波次此刻的分拣情况：各张出库单绑着的箱，每种货拣出来几件、分了几件、还有几件没拣回来。 */
export function getSortPlan(waveId: string): Promise<ApiResult<SortPlanBody>> {
  const params = new URLSearchParams({ waveId });
  return exchangeMasterData<SortPlanBody>(`/warehouse-fulfillment-wave-sort?${params.toString()}`);
}

export interface WaveClaimBody {
  /** ASSIGNED 领到了；EXISTING_RESULT 本来就是自己的；WAVE_ASSIGNED_TO_OTHER 已经派给了别人。 */
  outcome: string;
  /** 这个波次此刻派给了谁。 */
  assignee?: string;
}

/**
 * 拣货的人在工位上领一个波次：没人领的归自己，之后别人拣它的任务后端不收。走作业事实那一面（设备与作业范围请求头），
 * 认的是拣货这一种作业事实的授予。
 */
export function claimWave(deviceId: string, warehouse: string, waveId: string): Promise<ApiResult<WaveClaimBody>> {
  return postWorkFact<WaveClaimBody>(
    '/warehouse-fulfillment-wave-claims',
    { warehouse, waveId },
    { deviceId, scopeKind: 'WAREHOUSE', scopeReference: warehouse },
  );
}

/** 按波次号读一张。逐字相等，只在本租户；没有这个波次答 `WAVE_NOT_FOUND`、不带 `wave`，不答 404。 */
export function getWave(waveId: string): Promise<ApiResult<WaveOneBody>> {
  const params = new URLSearchParams({ waveId });
  return exchangeMasterData<WaveOneBody>(`/warehouse-fulfillment-wave?${params.toString()}`);
}

export function listChargeOccurrences(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<ChargeOccurrenceListBody>> {
  return exchangeMasterData<ChargeOccurrenceListBody>(
    catalogueRequestPath('/warehouse-fulfillment-charge-occurrences', conditions, after),
  );
}

export function listWarehouseAdoptions(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<WarehouseAdoptionListBody>> {
  return exchangeMasterData<WarehouseAdoptionListBody>(
    catalogueRequestPath('/warehouse-fulfillment-charge-adoption-list', conditions, after),
  );
}

export function listWarehouseChargeRequests(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<WarehouseChargeRequestListBody>> {
  return exchangeMasterData<WarehouseChargeRequestListBody>(
    catalogueRequestPath('/warehouse-fulfillment-charge-requests', conditions, after),
  );
}

export function listWarehousePlans(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<WarehousePlanListBody>> {
  return exchangeMasterData<WarehousePlanListBody>(
    catalogueRequestPath('/pricing-warehouse-plan-list', conditions, after),
  );
}

async function postRegistration(path: string, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  const answer = await postMasterData<CatalogueRegistrationBody>(path, snapshot);
  if (answer.kind !== 'outcome') return answer;
  return { kind: 'outcome', status: answer.status, body: asCatalogueRegistration(answer.body) };
}

export function registerCatalogue(family: CatalogueFamily, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postRegistration(catalogueWritePaths[family], snapshot);
}

async function postStockImport(path: string, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  const answer = await postMasterData<StockImportWriteBody>(path, snapshot);
  if (answer.kind !== 'outcome') return answer;
  return { kind: 'outcome', status: answer.status, body: asStockImportWrite(answer.body) };
}

export function submitStockImport(snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postStockImport('/warehouse-fulfillment-stock-import-submissions', snapshot);
}

export function approveStockImport(snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postStockImport('/warehouse-fulfillment-stock-import-approvals', snapshot);
}

export function rejectStockImport(snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postStockImport('/warehouse-fulfillment-stock-import-rejections', snapshot);
}

// ============================================================================
// 仓配 360° 详情与全模块台账读口
// ============================================================================

export function getPreAdviceDetail(ownerAccount: string, preAdviceId: string): Promise<ApiResult<PreAdviceOneBody>> {
  const params = new URLSearchParams({ ownerAccount, preAdviceId });
  return exchangeMasterData<PreAdviceOneBody>(`/warehouse-fulfillment-inbound-pre-advice?${params.toString()}`);
}

export interface PreAdviceByNumberBody {
  outcome: string;
  documentNumber: string;
  found: boolean;
  preAdvice?: PreAdviceRecord;
}

/** 按单据号取一张入库预报。逐字相等，只在本租户；找不到答 found 为假，不答 404。 */
export function getPreAdviceByNumber(documentNumber: string): Promise<ApiResult<PreAdviceByNumberBody>> {
  const params = new URLSearchParams({ documentNumber });
  return exchangeMasterData<PreAdviceByNumberBody>(`/warehouse-fulfillment-inbound-pre-advice-by-number?${params.toString()}`);
}

export function getGoodsCurrent(ownerAccount: string, goodsCode: string): Promise<ApiResult<GoodsCurrentBody>> {
  const params = new URLSearchParams({ ownerAccount, goodsCode });
  return exchangeMasterData<GoodsCurrentBody>(`/warehouse-fulfillment-goods-current?${params.toString()}`);
}

export interface GoodsFoundBody {
  outcome: string;
  ownerAccount: string;
  code: string;
  found: boolean;
  goods?: GoodsRecord;
}

/**
 * 按一个码找一位货主名下的商品：码是商品编码，或是此刻有效的商品条码、平台商品码（别名按有效区间认，已过期与还没生效的不认）。
 * 找不到答 found 为假，不答 404；别的货主的商品同样答找不到。同一个码在同一刻指向了两个商品时后端答 500，调用方当没问成。
 */
export function findGoodsByCode(ownerAccount: string, code: string): Promise<ApiResult<GoodsFoundBody>> {
  const params = new URLSearchParams({ ownerAccount, code });
  return exchangeMasterData<GoodsFoundBody>(`/warehouse-fulfillment-goods-by-code?${params.toString()}`);
}

export interface StockImportOneBody {
  found: boolean;
  stockImport?: StockImportRecord;
}

/**
 * 按导入号取一张建账导入。这个读口答的是导入本身，没有 outcome 外壳；查无此号答 404 `STOCK_IMPORT_NOT_FOUND`。
 * 这里把它收成 `found: false`，与别的按号读口同一个形状，单据页按「没有这张单」说话，不必认问题码。别的非业务答复原样交回。
 */
export async function getStockImportDetail(importId: string): Promise<ApiResult<StockImportOneBody>> {
  const params = new URLSearchParams({ importId });
  const answer = await exchangeMasterData<StockImportRecord>(`/warehouse-fulfillment-stock-import?${params.toString()}`);
  if (answer.kind === 'outcome') return { kind: 'outcome', status: answer.status, body: { found: true, stockImport: answer.body } };
  if (answer.kind === 'callerProblem' && answer.status === 404 && answer.code === 'STOCK_IMPORT_NOT_FOUND') {
    return { kind: 'outcome', status: answer.status, body: { found: false } };
  }
  return answer;
}

export function getUnclaimedArrivalDetail(arrivalId: string): Promise<ApiResult<UnclaimedArrivalOneBody>> {
  const params = new URLSearchParams({ arrivalId });
  return exchangeMasterData<UnclaimedArrivalOneBody>(`/warehouse-fulfillment-unclaimed-arrival?${params.toString()}`);
}

export interface SettingsOverviewBody {
  outcome: string;
  allocationStrategy: { configured: boolean; revision?: number; operator?: string; registeredAt?: string };
  /** 两格各说各的：`kind` 是受理时库存不够怎么办，`partialShipment` 是拣不齐时怎么发；没登记的那格不带。 */
  shortagePolicies: ReadonlyArray<{
    ownerAccount: string;
    revision: number;
    kind?: string;
    partialShipment?: string;
    operator: string;
    registeredAt: string;
  }>;
  /** 登记过「拣货必须用周转箱」的仓各自最新的一版；没登记的仓不在里面，那是未配置。 */
  toteRequirements?: ReadonlyArray<{ warehouseCode: string; revision: number; required: boolean; operator: string; registeredAt: string }>;
  /** 各仓登记过的成波规则各自最新的一版，停用的也在里面；没登记规则的仓不在里面。按页取时只是这一页。 */
  waveRules?: ReadonlyArray<WaveRuleRecord>;
  /** 按页取成波规则时有。不带页码的总览不答这一格，waveRules 是整份。 */
  waveRuleBook?: { page: number; pageSize: number; total: number };
  approvalDuty: { configured: boolean; revision?: number; distinctSubjects?: boolean; operator?: string; registeredAt?: string };
  disposalRules: ReadonlyArray<DisposalRuleRecord>;
  chargeUnits: ReadonlyArray<{ warehouseCode: string; revision: number; weightUnit: string; volumeUnit: string; registeredAt: string }>;
  chargeAging: ReadonlyArray<{ warehouseCode: string; basis: string; registeredAt: string }>;
  chargeProjects: ReadonlyArray<{ warehouseCode: string; projectName: string; registeredAt: string }>;
  agentConnectors: ReadonlyArray<{ warehouseCode: string; registeredAt: string }>;
}

/**
 * 按一条成波规则把这个仓里圈得到的已预占出库单放成波次。`runId` 是这一次执行的号：没送到时用同一个号再送，不会放两遍。
 * 答复带放出的波次与放了几张、跳了几张、还有几张没轮到。
 */
export function runWaveRule(warehouse: string, ruleCode: string, runId: string): Promise<ApiResult<WaveRuleRunBody>> {
  return postMasterData<WaveRuleRunBody>('/warehouse-fulfillment-wave-rule-runs', { warehouse, ruleCode, runId });
}

/**
 * 放此刻已经到点的成波规则：今天（仓的业务日）已经过了、还没办过的时点各办一次。平常由部署方的定时任务来调；
 * 这里是给人手动点的——定时任务没配、或想马上补办时用。同一个时点只办一次，点几次都不会放两遍。
 * 不给仓是本租户各仓的规则都看。
 */
export function runDueWaveRules(warehouse?: string): Promise<ApiResult<DueWaveRulesBody>> {
  return postMasterData<DueWaveRulesBody>('/warehouse-fulfillment-wave-rule-due-runs', warehouse ? { warehouse } : {});
}

export function getSettingsOverview(query?: {
  page?: number;
  pageSize?: number;
  warehouse?: string;
}): Promise<ApiResult<SettingsOverviewBody>> {
  const params = new URLSearchParams();
  if (query?.page !== undefined) params.set('page', String(query.page));
  if (query?.pageSize !== undefined) params.set('pageSize', String(query.pageSize));
  if (query?.warehouse) params.set('warehouse', query.warehouse);
  const text = params.toString();
  return exchangeMasterData<SettingsOverviewBody>(text === '' ? '/warehouse-fulfillment-settings-overview' : `/warehouse-fulfillment-settings-overview?${text}`);
}

export interface OutboundRequestDetailBody {
  outcome: string;
  request?: OutboundRequestRecord;
  /** 这张单仍是已预占、最近一次放波次时分配不出被跳过的那一笔；没有是 null。 */
  allocationException?: AllocationExceptionRecord | null;
  partialShipment?: PartialShipmentAnswer;
  /** 拣不齐之后照部分发货规则办过的那一次；没办过是 null。 */
  shortPickDisposal?: ShortPickDisposalAnswer | null;
  /** 这张单绑过的周转箱，按绑上的先后；还绑着的不带 `releasedAt`。 */
  totes?: ReadonlyArray<{ toteCode: string; boundAt: string; releasedAt?: string }>;
  lines?: ReadonlyArray<{
    lineIndex: number;
    goodsCode: string;
    unit: string;
    stockState: string;
    labelAlias?: string;
    batchNumber?: string;
    expiryDate?: string;
    quantity: number;
  }>;
  reservations?: ReadonlyArray<{
    lineIndex: number;
    goodsCode: string;
    stockState: string;
    labelAlias?: string;
    batchNumber?: string;
    expiryDate?: string;
    quantity: number;
    fromUnlabeled: number;
    wanted: number;
  }>;
  waves?: readonly string[];
  pickTasks?: ReadonlyArray<{
    taskId: string;
    waveId: string;
    locationCode: string;
    stagingLocation?: string;
    goodsCode: string;
    unit: string;
    stockState: string;
    inboundBatchId: string;
    quantity: number;
    picked: number;
    status: string;
    factId?: string;
  }>;
  checks?: ReadonlyArray<{ factId: string; observed: number; matched: boolean; checkedAt: string }>;
  parcels?: ReadonlyArray<{
    parcelSequence: number;
    packagingMaterial: string;
    weightValue: string;
    weightUnit: string;
    lengthValue: string;
    widthValue: string;
    heightValue: string;
    dimensionUnit: string;
    shipmentRequestId?: string;
    confirmationState?: string;
    cutoffState?: string;
    ownerLabel?: string;
    lines: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
  }>;
  confirmations?: ReadonlyArray<{
    factId: string;
    parcelSequence: number;
    kind: string;
    offline: boolean;
    missing: string;
    externalNumber?: string;
    pickupParty?: string;
    pickupCredential?: string;
    occurredAt: string;
    correctionKind?: string;
    correctionStatus?: string;
  }>;
  returnTasks?: ReadonlyArray<{
    returnId: string;
    parcelSequence?: number;
    locationCode: string;
    goodsCode: string;
    unit: string;
    stockState: string;
    inboundBatchId: string;
    quantity: number;
    status: string;
    factId?: string;
  }>;
}

export function getOutboundRequestDetail(requestId: string): Promise<ApiResult<OutboundRequestDetailBody>> {
  const params = new URLSearchParams({ requestId });
  return exchangeMasterData<OutboundRequestDetailBody>(`/warehouse-fulfillment-outbound-request?${params.toString()}`);
}

export interface StockFreezeRecord {
  freezeId: string;
  decisionId: string;
  ownerAccount: string;
  warehouseCode: string;
  target: string;
  locationCode?: string;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  labelAlias?: string;
  inboundBatchId?: string;
  batchNumber?: string;
  expiryDate?: string;
  cartonNumber?: string;
  quantity: number;
  reason: string;
  operator: string;
  frozenAt: string;
  status: string;
  unfreezeOperator?: string;
  unfrozenAt?: string;
  unfreezeNote?: string;
}

export interface StockFreezeListBody {
  outcome: string;
  freezes: StockFreezeRecord[];
  page: CataloguePage;
}

export function listFreezes(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<StockFreezeListBody>> {
  return exchangeMasterData<StockFreezeListBody>(catalogueRequestPath('/warehouse-fulfillment-freezes', conditions, after));
}

/**
 * 冻结同样只有列表。检索覆盖冻结号、货主、仓、商品、库位、箱号与原因，按包含匹配（大小写不分）；冻结号是系统签发的，
 * 局限与库存调整一样：号恰好是另一批冻结号的一段、而那一批多到挤出第一页时认不出，认不出而后面还有页时 `truncated` 为真。
 */
export async function findFreeze(freezeId: string): Promise<ApiResult<FoundByNumber<StockFreezeRecord>>> {
  const answer = await listFreezes({ q: freezeId });
  if (answer.kind !== 'outcome') return answer;
  const { freezes, page } = answer.body;
  return {
    kind: 'outcome',
    status: answer.status,
    body: {
      record: freezes.find((item) => item.freezeId === freezeId),
      searched: freezes.length,
      truncated: page.next !== null,
    },
  };
}

export interface StockCountRecord {
  countId: string;
  decisionId: string;
  warehouseCode: string;
  ownerAccount?: string;
  note?: string;
  operator: string;
  createdAt: string;
  status: string;
  startedBy?: string;
  startedAt?: string;
  completedBy?: string;
  completedAt?: string;
  voidedBy?: string;
  voidedAt?: string;
  voidReason?: string;
  locations: string[];
  goods: string[];
  entryCount: number;
  differenceCount: number;
  pendingDifferences: number;
}

export interface StockCountListBody {
  outcome: string;
  counts: StockCountRecord[];
  page: CataloguePage;
}

export interface StockCountDetailBody {
  outcome: string;
  count?: StockCountRecord;
  entries?: ReadonlyArray<{
    factId: string;
    locationCode: string;
    occurredAt: string;
    deviceSequence: number;
    deviceId: string;
    operatorId: string;
    receivedAt: string;
    lines: ReadonlyArray<{
      ownerAccount?: string;
      goodsCode?: string;
      unit?: string;
      stockState?: string;
      labelAlias?: string;
      batchNumber?: string;
      expiryDate?: string;
      quantity: number;
      observed?: string;
    }>;
  }>;
  differences?: ReadonlyArray<{
    differenceId: string;
    countId: string;
    ownerAccount: string;
    warehouseCode: string;
    locationCode: string;
    goodsCode: string;
    unit: string;
    stockState: string;
    labelAlias?: string;
    batchNumber?: string;
    expiryDate?: string;
    bookQuantity: number;
    countedQuantity: number;
    status: string;
    approvedBy?: string;
    approvedAt?: string;
    approvalNote?: string;
    rejectedBy?: string;
    rejectedAt?: string;
    rejectReason?: string;
  }>;
}

export function listCounts(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<StockCountListBody>> {
  return exchangeMasterData<StockCountListBody>(catalogueRequestPath('/warehouse-fulfillment-counts', conditions, after));
}

export function getCountDetail(countId: string): Promise<ApiResult<StockCountDetailBody>> {
  const params = new URLSearchParams({ countId });
  return exchangeMasterData<StockCountDetailBody>(`/warehouse-fulfillment-count?${params.toString()}`);
}

export interface StockAdjustmentLineRecord {
  lineIndex: number;
  kind: string;
  locationCode: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  labelAlias?: string;
  batchNumber?: string;
  expiryDate?: string;
  inboundBatchId?: string;
  quantity: number;
  basis?: string;
}

export interface StockAdjustmentRecord {
  adjustmentId: string;
  ownerAccount: string;
  warehouseCode: string;
  reason: string;
  operator: string;
  submittedAt: string;
  status: string;
  approvedBy?: string;
  approvedAt?: string;
  approvalNote?: string;
  rejectedBy?: string;
  rejectedAt?: string;
  rejectReason?: string;
  lines: StockAdjustmentLineRecord[];
}

export interface StockAdjustmentListBody {
  outcome: string;
  adjustments: StockAdjustmentRecord[];
  page: CataloguePage;
}

export function listAdjustments(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<StockAdjustmentListBody>> {
  return exchangeMasterData<StockAdjustmentListBody>(catalogueRequestPath('/warehouse-fulfillment-adjustments', conditions, after));
}

export interface JobDiscrepancyRecord {
  discrepancyId: string;
  factId: string;
  linePosition: number;
  ownerAccount: string;
  warehouseCode: string;
  sourceLocation?: string;
  locationCode?: string;
  goodsCode?: string;
  unit?: string;
  stockState?: string;
  labelAlias?: string;
  batchNumber?: string;
  expiryDate?: string;
  inboundBatchId?: string;
  cartonNumber?: string;
  note?: string;
  quantity: number;
  shortOnBook: number;
  heldQuantity: number;
  cartonMissing: boolean;
  status: string;
  approvedBy?: string;
  approvedAt?: string;
  approvalNote?: string;
  rejectedBy?: string;
  rejectedAt?: string;
  rejectReason?: string;
}

export interface JobDiscrepancyListBody {
  outcome: string;
  discrepancies: JobDiscrepancyRecord[];
  page: CataloguePage;
}

export function listJobDiscrepancies(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<JobDiscrepancyListBody>> {
  return exchangeMasterData<JobDiscrepancyListBody>(
    catalogueRequestPath('/warehouse-fulfillment-job-discrepancies', conditions, after),
  );
}

/** 按号检索列表之后认出的那一张：认不出时 record 缺席，searched 是答回来的行数，truncated 说后面还有页没翻。 */
export interface FoundByNumber<T> {
  record?: T;
  searched: number;
  truncated: boolean;
}

/**
 * 库存调整没有按号取一张的读口，只有列表：按调整号检索，再在答回来的那一页里认出号完全相同的那一张。检索是按包含匹配的
 * （大小写不分），调整号又是调用方自己给的，号恰好是另一批调整号的一段、而那一批多到挤出第一页时会认不出；认不出而后面还有页时
 * `truncated` 为真，由页面如实说「没翻到」，不说成没有这张单。
 */
export async function findAdjustment(adjustmentId: string): Promise<ApiResult<FoundByNumber<StockAdjustmentRecord>>> {
  const answer = await listAdjustments({ q: adjustmentId });
  if (answer.kind !== 'outcome') return answer;
  const { adjustments, page } = answer.body;
  return {
    kind: 'outcome',
    status: answer.status,
    body: {
      record: adjustments.find((item) => item.adjustmentId === adjustmentId),
      searched: adjustments.length,
      truncated: page.next !== null,
    },
  };
}

/**
 * 作业差异同样只有列表。检索覆盖差异号、作业事实号、货主、仓、商品、库位与箱号，差异号是系统随机签发的、彼此不是对方的一段，
 * 所以实际上不会认不出；局限与库存调整一样，认不出而后面还有页时 `truncated` 为真。
 */
export async function findJobDiscrepancy(discrepancyId: string): Promise<ApiResult<FoundByNumber<JobDiscrepancyRecord>>> {
  const answer = await listJobDiscrepancies({ q: discrepancyId });
  if (answer.kind !== 'outcome') return answer;
  const { discrepancies, page } = answer.body;
  return {
    kind: 'outcome',
    status: answer.status,
    body: {
      record: discrepancies.find((item) => item.discrepancyId === discrepancyId),
      searched: discrepancies.length,
      truncated: page.next !== null,
    },
  };
}

export interface InstructionRecord {
  instructionId: string;
  ownerAccount: string;
  warehouseCode: string;
  kind: string;
  status: string;
  origin: string;
  goodsCode: string;
  unit?: string;
  quantity: number;
  doneQuantity: number;
  fromAlias?: string;
  toAlias?: string;
  batchNumber?: string;
  expiryDate?: string;
  reason?: string;
  basis?: string;
  operator?: string;
  fixedRevision: number;
  voidReason?: string;
  voidedAt?: string;
  receivedAt: string;
  components: ReadonlyArray<{ goodsCode: string; quantity: number }>;
  executions: ReadonlyArray<{
    factId: string;
    deviceId: string;
    operatorId: string;
    occurredAt: string;
    deviceSequence: number;
    doneQuantity: number;
    movementId: string;
  }>;
}

export interface InstructionListBody {
  outcome: string;
  instructions: InstructionRecord[];
  page: CataloguePage;
}

export function listInstructions(
  conditions: CatalogueConditions,
  after: string | null = null,
): Promise<ApiResult<InstructionListBody>> {
  return exchangeMasterData<InstructionListBody>(catalogueRequestPath('/warehouse-fulfillment-instructions', conditions, after));
}

/**
 * 加工、拆解与换标指令共用一份指令册、一个读口，也只有列表。检索覆盖指令号、货主、仓、商品、新别名与原因，按包含匹配
 * （大小写不分）；局限同库存调整：认不出而后面还有页时 `truncated` 为真，由页面如实说「没翻到」，不说成没有这条指令。
 */
export async function findInstruction(instructionId: string): Promise<ApiResult<FoundByNumber<InstructionRecord>>> {
  const answer = await listInstructions({ q: instructionId });
  if (answer.kind !== 'outcome') return answer;
  const { instructions, page } = answer.body;
  return {
    kind: 'outcome',
    status: answer.status,
    body: {
      record: instructions.find((item) => item.instructionId === instructionId),
      searched: instructions.length,
      truncated: page.next !== null,
    },
  };
}

export interface ReturnPreAdviceRecord {
  adviceId: string;
  documentNumber?: string;
  numberingAbsence?: string;
  ownerAccount: string;
  warehouseCode: string;
  origin: string;
  trackingNumber: string;
  basis?: string;
  operator?: string;
  receivedAt: string;
  lines: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
  /** 按页取的列表才带：待到仓、已到仓、已处置。按一张取时没有，状态由到仓记录对出来。 */
  status?: 'AWAITING_ARRIVAL' | 'ARRIVED' | 'DISPOSED';
}

export interface ReturnReceiptRecord {
  factId: string;
  warehouseCode: string;
  locationCode: string;
  trackingNumber: string;
  ownerAccount?: string;
  adviceId?: string;
  arrivalId?: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
  outcome: string;
  occurredAt: string;
  disposition?: {
    decisionId: string;
    destination: string;
    basis?: string;
    operator?: string;
    origin: string;
    decidedAt: string;
  };
}

export interface ReturnsOverviewBody {
  outcome: string;
  preAdvices: ReturnPreAdviceRecord[];
  receipts: ReturnReceiptRecord[];
}

export function getReturnsOverview(): Promise<ApiResult<ReturnsOverviewBody>> {
  return exchangeMasterData<ReturnsOverviewBody>('/warehouse-fulfillment-returns-overview');
}

export { cataloguePageSizes as returnsPageSizes, catalogueDefaultPageSize as returnsDefaultPageSize };

export interface ReturnsPageBody {
  outcome: string;
  view: 'advices' | 'receipts' | 'advice' | 'receipt';
  preAdvices: ReturnPreAdviceRecord[];
  receipts: ReturnReceiptRecord[];
  page?: { page: number; pageSize: number; total: number };
  counts?: Record<string, number>;
}

export interface ReturnsListQuery {
  view: 'advices' | 'receipts';
  page: number;
  pageSize: number;
  status: string;
  q: string;
  owner: string;
  warehouse: string;
  sort: 'newest' | 'oldest';
}

function returnsPath(params: Record<string, string | number>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== '') search.set(key, String(value));
  }
  return `/warehouse-fulfillment-returns-overview?${search.toString()}`;
}

/** 退货预报或到仓记录的一页。状态数量在 counts 里，不含状态这一维的筛选。 */
export function listReturnsPage(query: ReturnsListQuery): Promise<ApiResult<ReturnsPageBody>> {
  return exchangeMasterData<ReturnsPageBody>(
    returnsPath({
      view: query.view,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status,
      q: query.q,
      owner: query.owner,
      warehouse: query.warehouse,
      sort: query.sort,
    }),
  );
}

/** 按预报号或预报标识取一张，连同对上的到仓记录。认不出就是空数组。 */
export function loadReturnAdvice(key: string): Promise<ApiResult<ReturnsOverviewBody>> {
  return exchangeMasterData<ReturnsOverviewBody>(returnsPath({ view: 'advice', key }));
}

/** 按作业事实号取一笔，连同同一退件单号的其他到仓记录和对上的预报。 */
export function loadReturnReceipt(key: string): Promise<ApiResult<ReturnsOverviewBody>> {
  return exchangeMasterData<ReturnsOverviewBody>(returnsPath({ view: 'receipt', key }));
}

export interface AgentOverviewBody {
  outcome: string;
  connectors: ReadonlyArray<{ warehouseCode: string; registeredAt: string }>;
  statements: ReadonlyArray<{
    sourceId: string;
    statedAt: string;
    validUntil: string;
    warehouseCode: string;
    ownerAccount: string;
    goodsCode: string;
    unit: string;
    quantity: number;
    freshness: string;
  }>;
  results: ReadonlyArray<{
    resultId: string;
    requestId: string;
    warehouseCode: string;
    kind: string;
    weightValue?: string;
    weightUnit?: string;
    lengthValue?: string;
    widthValue?: string;
    heightValue?: string;
    dimensionUnit?: string;
    reportedAt: string;
  }>;
  /** 按页取时有，三张表各一页。不带页码的总览不答这一格。 */
  pages?: {
    connectors: { page: number; pageSize: number; total: number };
    statements: { page: number; pageSize: number; total: number };
    results: { page: number; pageSize: number; total: number };
  };
}

export function getAgentOverview(query?: {
  pageSize: number;
  connectorsPage: number;
  statementsPage: number;
  resultsPage: number;
}): Promise<ApiResult<AgentOverviewBody>> {
  const params = new URLSearchParams();
  if (query) {
    params.set('pageSize', String(query.pageSize));
    params.set('connectorsPage', String(query.connectorsPage));
    params.set('statementsPage', String(query.statementsPage));
    params.set('resultsPage', String(query.resultsPage));
  }
  const text = params.toString();
  return exchangeMasterData<AgentOverviewBody>(text === '' ? '/warehouse-fulfillment-agent-overview' : `/warehouse-fulfillment-agent-overview?${text}`);
}
