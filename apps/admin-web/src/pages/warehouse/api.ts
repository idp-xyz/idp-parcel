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
