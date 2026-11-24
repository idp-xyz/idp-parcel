import type { RegistrationResponseBody } from '../../components/registration';
import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { CatalogueConditions, CataloguePage } from '../catalogue-query';
import { catalogueRequestPath } from '../catalogue-query';
import { asCatalogueRegistration, asStockImportWrite, type CatalogueRegistrationBody, type StockImportWriteBody } from './answers';
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
  postingReferences?: string[];
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

export function listOwnerPreAdvices(
  ownerAccount: string,
  keyword: string,
): Promise<ApiResult<PreAdviceListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  return exchangeMasterData<PreAdviceListBody>(`/warehouse-fulfillment-inbound-pre-advices?${params.toString()}`);
}

export function listOwnerGoods(ownerAccount: string, keyword: string): Promise<ApiResult<GoodsListBody>> {
  const params = new URLSearchParams({ ownerAccount });
  if (keyword.trim() !== '') params.set('q', keyword.trim());
  return exchangeMasterData<GoodsListBody>(`/warehouse-fulfillment-goods?${params.toString()}`);
}

export function queryOwnerStock(
  ownerAccount: string,
  warehouseCode: string,
  view: StockViewKind,
): Promise<ApiResult<StockReadBody>> {
  const params = new URLSearchParams({ ownerAccount, view });
  if (warehouseCode.trim() !== '') params.set('warehouseCode', warehouseCode.trim());
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
  releasedAt: string;
  requests: string[];
  tasks: WaveTaskRecord[];
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

export function getGoodsCurrent(ownerAccount: string, goodsCode: string): Promise<ApiResult<GoodsCurrentBody>> {
  const params = new URLSearchParams({ ownerAccount, goodsCode });
  return exchangeMasterData<GoodsCurrentBody>(`/warehouse-fulfillment-goods-current?${params.toString()}`);
}

export interface SettingsOverviewBody {
  outcome: string;
  allocationStrategy: { configured: boolean; revision?: number; operator?: string; registeredAt?: string };
  shortagePolicies: ReadonlyArray<{ ownerAccount: string; revision: number; kind: string; operator: string; registeredAt: string }>;
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
