import type { CatalogueConditions } from '../catalogue-query';
import { catalogueRequestPath } from '../catalogue-query';

/** 五个名册。词与读口路径、受控批量口的 -kind 同一套。 */
export type CatalogueFamily = 'warehouse' | 'zone' | 'location-type' | 'location' | 'packaging';

export const catalogueFamilies: readonly CatalogueFamily[] = [
  'warehouse',
  'zone',
  'location-type',
  'location',
  'packaging',
];

export const catalogueReadPaths: Record<CatalogueFamily, string> = {
  warehouse: '/warehouse-fulfillment-warehouses',
  zone: '/warehouse-fulfillment-zones',
  'location-type': '/warehouse-fulfillment-location-types',
  location: '/warehouse-fulfillment-locations',
  packaging: '/warehouse-fulfillment-packaging-materials',
};

export const catalogueWritePaths: Record<CatalogueFamily, string> = {
  warehouse: '/warehouse-fulfillment-warehouse-registrations',
  zone: '/warehouse-fulfillment-zone-registrations',
  'location-type': '/warehouse-fulfillment-location-type-registrations',
  location: '/warehouse-fulfillment-location-registrations',
  packaging: '/warehouse-fulfillment-packaging-material-registrations',
};

const familySelector = 'family';

export function catalogueFamilyOf(value: string | undefined): CatalogueFamily {
  return catalogueFamilies.find((family) => family === value) ?? 'warehouse';
}

/**
 * 换册要让游标作废，所以册别放进选择器。读口不认这一维，发出去之前拿掉，否则整问答调用方式问题。
 */
export function catalogueReadPath(conditions: CatalogueConditions, after: string | null): string {
  const family = catalogueFamilyOf(conditions.selectors?.[familySelector]);
  const selectors = { ...(conditions.selectors ?? {}) };
  delete selectors[familySelector];
  const stripped: CatalogueConditions = {
    ...conditions,
    selectors: Object.keys(selectors).length === 0 ? undefined : selectors,
  };
  return catalogueRequestPath(catalogueReadPaths[family], stripped, after);
}

export type ActivityFilter = 'all' | 'active' | 'deactivated';

export function catalogueConditions(family: CatalogueFamily, keyword: string, activity: ActivityFilter): CatalogueConditions {
  return {
    selectors: { [familySelector]: family },
    q: keyword,
    filters: activity === 'all' ? undefined : { deactivated: [activity === 'deactivated' ? 'true' : 'false'] },
  };
}

export type StockImportStatusFilter = 'ALL' | 'PENDING' | 'POSTED' | 'REJECTED';

export function stockImportConditions(keyword: string, status: StockImportStatusFilter): CatalogueConditions {
  return {
    q: keyword,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
}

const warehouseKindLabels: Record<string, string> = {
  SELF_OPERATED: '自营',
  AGENT: '代理',
};

const zoneUsageLabels: Record<string, string> = {
  RECEIVING_STAGING: '收货暂存',
  STORAGE: '存储',
  PICKING: '拣货',
  DEFECTIVE: '不良',
  RETURN_STAGING: '退货暂存',
  OUTBOUND_STAGING: '出库暂存',
};

export const stockImportStatusLabels: Record<string, string> = {
  PENDING: '待批',
  POSTED: '已入账',
  REJECTED: '已驳回',
};

export function warehouseKindLabel(kind: string): string {
  return warehouseKindLabels[kind] ?? kind;
}

export function zoneUsageLabel(usage: string): string {
  return zoneUsageLabels[usage] ?? usage;
}

export function activityLabel(deactivated: boolean): string {
  return deactivated ? '已停用' : '在用';
}

export function stockImportStatusLabel(status: string): string {
  return stockImportStatusLabels[status] ?? status;
}
