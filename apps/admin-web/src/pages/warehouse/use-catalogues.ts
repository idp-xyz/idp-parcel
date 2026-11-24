import { useCallback, useEffect, useState } from 'react';
import { listCustomerAccounts, listServiceProducts, type CustomerAccountRecord, type ServiceProductRecord } from '../party/api';
import {
  listWarehouseCatalogue,
  type LocationRecord,
  type LocationTypeRecord,
  type PackagingRecord,
  type WarehouseRecord,
  type ZoneRecord,
} from './api';
import type { GuidedFormValues, GuidedSelectOption, WarehouseLookupKind } from './guided-forms';
import { warehouseKindLabel, zoneUsageLabel } from './rows';

export interface WarehouseLookups {
  warehouses: WarehouseRecord[];
  zones: ZoneRecord[];
  locationTypes: LocationTypeRecord[];
  locations: LocationRecord[];
  packaging: PackagingRecord[];
  owners: CustomerAccountRecord[];
  serviceProducts: ServiceProductRecord[];
  loaded: boolean;
  reload: () => void;
}

const emptyLookups: Omit<WarehouseLookups, 'reload'> = {
  warehouses: [],
  zones: [],
  locationTypes: [],
  locations: [],
  packaging: [],
  owners: [],
  serviceProducts: [],
  loaded: false,
};

/**
 * 仓配全局实时字典：拉取仓库、库区、库位、库位类型、包材、货主客户账户与服务产品，
 * 供全部引导表单做下拉选择与级联筛选（选了仓就只列该仓的库区与在用库位），不再靠手敲记编码。
 */
export function useWarehouseLookups(): WarehouseLookups {
  const [data, setData] = useState<Omit<WarehouseLookups, 'reload'>>(emptyLookups);
  const [tick, setTick] = useState(0);

  const reload = useCallback(() => setTick((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([
      listWarehouseCatalogue({ selectors: { family: 'warehouse' } }),
      listWarehouseCatalogue({ selectors: { family: 'zone' } }),
      listWarehouseCatalogue({ selectors: { family: 'location-type' } }),
      listWarehouseCatalogue({ selectors: { family: 'location' } }),
      listWarehouseCatalogue({ selectors: { family: 'packaging' } }),
      listCustomerAccounts(),
      listServiceProducts(),
    ]).then(([wh, zn, lt, loc, pkg, acct, prod]) => {
      if (cancelled) return;
      setData({
        warehouses: wh.kind === 'outcome' ? wh.body.warehouses ?? [] : [],
        zones: zn.kind === 'outcome' ? zn.body.zones ?? [] : [],
        locationTypes: lt.kind === 'outcome' ? lt.body.locationTypes ?? [] : [],
        locations: loc.kind === 'outcome' ? loc.body.locations ?? [] : [],
        packaging: pkg.kind === 'outcome' ? pkg.body.packagingMaterials ?? [] : [],
        owners: acct.kind === 'outcome' ? acct.body.accounts ?? [] : [],
        serviceProducts: prod.kind === 'outcome' ? prod.body.products ?? [] : [],
        loaded: true,
      });
    });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  return { ...data, reload };
}

function selectedWarehouseOf(values: GuidedFormValues): string {
  const raw = values.warehouseCode ?? values.warehouse;
  return typeof raw === 'string' ? raw.trim() : '';
}

/** 按字段的 lookup 种类与当前表单已选仓库，过滤出实时候选项。 */
export function resolveLookupOptions(
  kind: WarehouseLookupKind,
  lookups: WarehouseLookups | undefined,
  formValues: GuidedFormValues,
): GuidedSelectOption[] {
  if (!lookups) return [];
  const wh = selectedWarehouseOf(formValues);
  const zoneUsageByKey = new Map<string, { usage: string; deactivated: boolean }>();
  for (const z of lookups.zones) {
    zoneUsageByKey.set(`${z.warehouseCode}/${z.zoneCode}`, { usage: z.usage, deactivated: z.deactivated });
  }

  switch (kind) {
    case 'warehouse':
      return lookups.warehouses.map((w) => ({
        value: w.warehouseCode,
        label: `${w.warehouseCode} · ${w.name}（${warehouseKindLabel(w.kind)}${w.deactivated ? ' · 已停用' : ''}）`,
      }));
    case 'self-warehouse':
      return lookups.warehouses
        .filter((w) => w.kind === 'SELF_OPERATED')
        .map((w) => ({
          value: w.warehouseCode,
          label: `${w.warehouseCode} · ${w.name}${w.deactivated ? '（已停用）' : ''}`,
        }));
    case 'agent-warehouse':
      return lookups.warehouses
        .filter((w) => w.kind === 'AGENT')
        .map((w) => ({
          value: w.warehouseCode,
          label: `${w.warehouseCode} · ${w.name}${w.deactivated ? '（已停用）' : ''}`,
        }));
    case 'zone':
      return lookups.zones
        .filter((z) => !wh || z.warehouseCode === wh)
        .map((z) => ({
          value: z.zoneCode,
          label: `${z.zoneCode} · ${z.name}（${zoneUsageLabel(z.usage)}${z.deactivated ? ' · 已停用' : ''}）`,
        }));
    case 'location-type':
      return lookups.locationTypes
        .filter((t) => !t.deactivated)
        .map((t) => ({
          value: t.locationTypeCode,
          label: `${t.locationTypeCode} · ${t.name}`,
        }));
    case 'location':
    case 'receiving-location':
    case 'return-staging-location':
    case 'outbound-staging-location': {
      const targetUsage =
        kind === 'receiving-location'
          ? 'RECEIVING_STAGING'
          : kind === 'return-staging-location'
            ? 'RETURN_STAGING'
            : kind === 'outbound-staging-location'
              ? 'OUTBOUND_STAGING'
              : null;
      return lookups.locations
        .filter((loc) => {
          if (wh && loc.warehouseCode !== wh) return false;
          const zoneInfo = zoneUsageByKey.get(`${loc.warehouseCode}/${loc.zoneCode}`);
          if (targetUsage && zoneInfo && zoneInfo.usage !== targetUsage) return false;
          return true;
        })
        .map((loc) => {
          const zoneInfo = zoneUsageByKey.get(`${loc.warehouseCode}/${loc.zoneCode}`);
          const off = loc.deactivated || zoneInfo?.deactivated;
          const usage = zoneInfo ? zoneUsageLabel(zoneInfo.usage) : loc.zoneCode;
          return {
            value: loc.locationCode,
            label: `${loc.locationCode}（${loc.warehouseCode} · ${usage}${off ? ' · 已停用' : ''}）`,
          };
        });
    }
    case 'packaging':
      return lookups.packaging
        .filter((p) => !p.deactivated)
        .map((p) => ({
          value: p.packagingMaterialCode,
          label: `${p.packagingMaterialCode} · ${p.name}（${p.dimensions.length}×${p.dimensions.width}×${p.dimensions.height}${p.dimensions.unit} / ${p.weight.value}${p.weight.unit}）`,
        }));
    case 'owner':
      return lookups.owners.map((a) => ({
        value: a.accountId,
        label: a.customerPartyName ? `${a.accountId} · ${a.customerPartyName}` : a.accountId,
      }));
    case 'service-product': {
      const seen = new Set<string>();
      const out: GuidedSelectOption[] = [];
      for (const p of lookups.serviceProducts) {
        if (seen.has(p.objectId)) continue;
        seen.add(p.objectId);
        out.push({ value: p.objectId, label: `${p.objectId}（${p.scope}）` });
      }
      return out;
    }
  }
}
