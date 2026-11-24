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

export interface StockImportRecord {
  importId: string;
  ownerAccount: string;
  warehouseCode: string;
  reason: string;
  status: string;
  submitter: string;
  submittedAt: string;
  goodsLines: readonly unknown[];
  cartonLines: readonly unknown[];
  poster?: string;
  postedAt?: string;
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
  registrant: string;
  acceptedAt: string;
  goodsLines?: readonly unknown[];
  cartons?: readonly unknown[];
}

export interface PreAdviceListBody {
  outcome: string;
  ownerAccount: string;
  preAdvices: PreAdviceRecord[];
  page: CataloguePage;
}

export interface GoodsRecord {
  ownerAccount: string;
  goodsCode: string;
  revision: number;
  name: string;
  baseUnit: string;
  trackBatch: boolean;
  trackExpiry: boolean;
  composite: boolean;
  registrant: string;
  registeredAt: string;
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
    inboundBatchId: string;
    quantity: number;
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
    location?: string;
    cartonNumber?: string;
    quantity: number;
    reason?: string;
    operator?: string;
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
