import type { RegistrationResponseBody } from '../../components/registration';
import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { CatalogueConditions, CataloguePage } from '../catalogue-query';
import { catalogueRequestPath } from '../catalogue-query';
import { asCatalogueRegistration, asStockImportWrite, type CatalogueRegistrationBody, type StockImportWriteBody } from './answers';
import type { PartialShipmentAnswer, ShortPickDisposalAnswer } from './partial-shipment';
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
    frozen: number;
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
  }>;
  movements?: ReadonlyArray<{
    warehouse: string;
    reason: string;
    source?: string;
    occurredAt: string;
    postedAt?: string;
    lines?: ReadonlyArray<{ location?: string; goodsCode: string; unit: string; stockState: string; delta: number }>;
  }>;
  /** 只在看流水时有：五种看法里只有流水翻页，其余答整份。 */
  page?: CataloguePage;
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
): Promise<ApiResult<PreAdviceListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  if (after !== null && after !== '') params.set('after', after);
  return exchangeMasterData<PreAdviceListBody>(`/warehouse-fulfillment-inbound-pre-advices?${params.toString()}`);
}

export function listOwnerGoods(ownerAccount: string, keyword: string, after: string | null = null): Promise<ApiResult<GoodsListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  if (after !== null && after !== '') params.set('after', after);
  return exchangeMasterData<GoodsListBody>(`/warehouse-fulfillment-goods?${params.toString()}`);
}

export function queryOwnerStock(
  ownerAccount: string,
  warehouseCode: string,
  view: StockViewKind,
  after: string | null = null,
): Promise<ApiResult<StockReadBody>> {
  const params = new URLSearchParams({ ownerAccount, view });
  if (warehouseCode.trim() !== '') params.set('warehouseCode', warehouseCode.trim());
  // 读口只在看流水时收 after，别的看法带了它会被拒。
  if (view === 'movements' && after !== null && after !== '') params.set('after', after);
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
}

export interface OutboundRequestListBody {
  outcome: string;
  requests: OutboundRequestRecord[];
  page: CataloguePage;
}

export interface WaveTaskRecord {
  taskId: string;
  requestId: string;
  locationCode: string;
  stagingLocation?: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  quantity: number;
  picked: number;
  status: string;
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
  approvalDuty: { configured: boolean; revision?: number; distinctSubjects?: boolean; operator?: string; registeredAt?: string };
  disposalRules: ReadonlyArray<DisposalRuleRecord>;
  chargeUnits: ReadonlyArray<{ warehouseCode: string; revision: number; weightUnit: string; volumeUnit: string; registeredAt: string }>;
  chargeAging: ReadonlyArray<{ warehouseCode: string; basis: string; registeredAt: string }>;
  chargeProjects: ReadonlyArray<{ warehouseCode: string; projectName: string; registeredAt: string }>;
  agentConnectors: ReadonlyArray<{ warehouseCode: string; registeredAt: string }>;
}

export function getSettingsOverview(): Promise<ApiResult<SettingsOverviewBody>> {
  return exchangeMasterData<SettingsOverviewBody>('/warehouse-fulfillment-settings-overview');
}

export interface OutboundRequestDetailBody {
  outcome: string;
  request?: OutboundRequestRecord;
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
}

export function getAgentOverview(): Promise<ApiResult<AgentOverviewBody>> {
  return exchangeMasterData<AgentOverviewBody>('/warehouse-fulfillment-agent-overview');
}
