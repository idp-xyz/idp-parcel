import { useMemo, useState } from 'react';
import { Plus } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';
import {
  listWarehouseCatalogue,
  registerCatalogue,
  type CatalogueListBody,
  type LocationRecord,
  type LocationTypeRecord,
  type PackagingRecord,
  type WarehouseRecord,
  type ZoneRecord,
} from './api';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import type { GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { enrichedCatalogueForms } from './guided-schemas';
import {
  catalogueOutcomeLabels,
  catalogueRefusalLabels,
  familyLabels,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import {
  activityLabel,
  catalogueFamilies,
  catalogueReadPaths,
  catalogueWritePaths,
  warehouseKindLabel,
  zoneUsageLabel,
  type ActivityFilter,
  type CatalogueFamily,
} from './rows';
import { useWarehouseLookups, type WarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-catalogues'];

interface CatalogueRow {
  key: string;
  family: CatalogueFamily;
  code: string;
  name: string;
  place: string;
  note: string;
  deactivated: boolean;
  revision: number;
  registrant: string;
  registeredAt: string;
  nextRevisionSeed: GuidedFormValues;
  toggleActiveSeed: GuidedFormValues;
  childAction?: { label: string; targetId: CatalogueFamily; values: GuidedFormValues };
}

const mono = 'whitespace-nowrap font-mono type-body leading-4';

function rowsOf(family: CatalogueFamily, body: CatalogueListBody): CatalogueRow[] {
  switch (family) {
    case 'warehouse':
      return (body.warehouses ?? []).map(warehouseRow);
    case 'zone':
      return (body.zones ?? []).map(zoneRow);
    case 'location-type':
      return (body.locationTypes ?? []).map(locationTypeRow);
    case 'location':
      return (body.locations ?? []).map(locationRow);
    case 'packaging':
      return (body.packagingMaterials ?? []).map(packagingRow);
  }
}

function warehouseRow(row: WarehouseRecord): CatalogueRow {
  const base: GuidedFormValues = {
    warehouseCode: row.warehouseCode,
    revision: String(row.revision + 1),
    name: row.name,
    kind: row.kind,
    'address.country': row.address.country,
    'address.text': row.address.text,
    businessTimeZone: row.businessTimeZone ?? '',
    colocatedNode: row.colocatedNode ?? '',
    deactivated: row.deactivated,
  };
  return {
    key: row.warehouseCode,
    family: 'warehouse',
    code: row.warehouseCode,
    name: row.name,
    place: row.address.country,
    note: `${warehouseKindLabel(row.kind)}${row.businessTimeZone ? ` · ${row.businessTimeZone}` : ' · 时区未配置'}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: base,
    toggleActiveSeed: { ...base, deactivated: !row.deactivated },
    childAction: {
      label: '在此仓新增库区',
      targetId: 'zone',
      values: { warehouseCode: row.warehouseCode, revision: '1' },
    },
  };
}

function zoneRow(row: ZoneRecord): CatalogueRow {
  const base: GuidedFormValues = {
    warehouseCode: row.warehouseCode,
    zoneCode: row.zoneCode,
    revision: String(row.revision + 1),
    name: row.name,
    usage: row.usage,
    deactivated: row.deactivated,
  };
  return {
    key: `${row.warehouseCode}/${row.zoneCode}`,
    family: 'zone',
    code: row.zoneCode,
    name: row.name,
    place: row.warehouseCode,
    note: zoneUsageLabel(row.usage),
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: base,
    toggleActiveSeed: { ...base, deactivated: !row.deactivated },
    childAction: {
      label: '在此库区新增库位',
      targetId: 'location',
      values: { warehouseCode: row.warehouseCode, zoneCode: row.zoneCode, revision: '1' },
    },
  };
}

function locationTypeRow(row: LocationTypeRecord): CatalogueRow {
  const base: GuidedFormValues = {
    locationTypeCode: row.locationTypeCode,
    revision: String(row.revision + 1),
    name: row.name,
    deactivated: row.deactivated,
  };
  return {
    key: row.locationTypeCode,
    family: 'location-type',
    code: row.locationTypeCode,
    name: row.name,
    place: '全租户共用',
    note: '',
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: base,
    toggleActiveSeed: { ...base, deactivated: !row.deactivated },
  };
}

function locationRow(row: LocationRecord): CatalogueRow {
  const type = row.locationTypeCode ?? '未指定类型';
  const priority = row.priority === undefined ? '' : ` · 优先级 ${row.priority}`;
  const base: GuidedFormValues = {
    warehouseCode: row.warehouseCode,
    locationCode: row.locationCode,
    revision: String(row.revision + 1),
    zoneCode: row.zoneCode,
    locationTypeCode: row.locationTypeCode ?? '',
    priority: row.priority === undefined ? '' : String(row.priority),
    deactivated: row.deactivated,
  };
  return {
    key: `${row.warehouseCode}/${row.locationCode}`,
    family: 'location',
    code: row.locationCode,
    name: row.locationCode,
    place: row.warehouseCode,
    note: `库区 ${row.zoneCode} · ${type}${priority}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: base,
    toggleActiveSeed: { ...base, deactivated: !row.deactivated },
  };
}

function packagingRow(row: PackagingRecord): CatalogueRow {
  const size = row.dimensions;
  const base: GuidedFormValues = {
    packagingMaterialCode: row.packagingMaterialCode,
    revision: String(row.revision + 1),
    name: row.name,
    'dimensions.length': size.length,
    'dimensions.width': size.width,
    'dimensions.height': size.height,
    'dimensions.unit': size.unit,
    'weight.value': row.weight.value,
    'weight.unit': row.weight.unit,
    deactivated: row.deactivated,
  };
  return {
    key: row.packagingMaterialCode,
    family: 'packaging',
    code: row.packagingMaterialCode,
    name: row.name,
    place: '全租户共用',
    note: `${size.length}×${size.width}×${size.height} ${size.unit} · ${row.weight.value} ${row.weight.unit}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: base,
    toggleActiveSeed: { ...base, deactivated: !row.deactivated },
  };
}

const columns: ListColumn<CatalogueRow>[] = [
  { id: 'code', header: '编码', className: mono, render: (row) => row.code },
  { id: 'name', header: '名称', render: (row) => row.name },
  { id: 'place', header: '所属', className: mono, render: (row) => row.place },
  { id: 'note', header: '规格 / 说明', render: (row) => row.note || '—' },
  { id: 'activity', header: '状态', render: (row) => activityLabel(row.deactivated) },
  { id: 'revision', header: '修订', className: mono, render: (row) => `r${row.revision}` },
];

const activityFilters: ReadonlyArray<{ id: ActivityFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'active', label: '在用' },
  { id: 'deactivated', label: '已停用' },
];

const targetsByFamily: Record<CatalogueFamily, GuidedActionTarget> = Object.fromEntries(
  catalogueFamilies.map((family) => [
    family,
    {
      id: family,
      label: familyLabels[family],
      title: registrationTitles[family],
      endpoint: `POST ${catalogueWritePaths[family]}`,
      snapshotHint: registrationSnapshotHints[family],
      submit: (snapshot: unknown) => registerCatalogue(family, snapshot),
      outcomeLabels: catalogueOutcomeLabels,
      refusalReasonLabels: catalogueRefusalLabels,
      form: enrichedCatalogueForms[family],
    },
  ]),
) as Record<CatalogueFamily, GuidedActionTarget>;

function FamilyTable({
  family,
  mayWrite,
  lookups,
  refreshKey,
  onOpenForm,
}: {
  family: CatalogueFamily;
  mayWrite: boolean;
  lookups: WarehouseLookups;
  refreshKey: number;
  onOpenForm: (targetId: CatalogueFamily, values?: GuidedFormValues) => void;
}) {
  const [keyword, setKeyword] = useState('');
  const [activity, setActivity] = useState<ActivityFilter>('all');
  const [warehouseFilter, setWarehouseFilter] = useState('');
  const settled = useSettledKeyword(keyword);
  const supportsWarehouseFilter = family === 'zone' || family === 'location';

  const conditions = useMemo(() => {
    const filters: Record<string, readonly string[]> = {};
    if (activity !== 'all') filters.deactivated = [activity === 'deactivated' ? 'true' : 'false'];
    if (supportsWarehouseFilter && warehouseFilter !== '') filters.warehouseCode = [warehouseFilter];
    return {
      selectors: { family },
      q: settled,
      filters: Object.keys(filters).length > 0 ? filters : undefined,
    };
  }, [family, settled, activity, supportsWarehouseFilter, warehouseFilter]);

  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseCatalogue);
  const [lastRefresh, setLastRefresh] = useState(refreshKey);
  if (refreshKey !== lastRefresh) {
    setLastRefresh(refreshKey);
    retry();
  }

  const rows = answer?.kind === 'outcome' ? rowsOf(family, answer.body) : [];
  const label = familyLabels[family];

  const inspectorOf = (row: CatalogueRow): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (mayWrite) {
      actions.push(
        { label: `修改资料（登 r${row.revision + 1}）`, onRun: () => onOpenForm(row.family, row.nextRevisionSeed) },
        {
          label: row.deactivated ? `重新启用（登 r${row.revision + 1}）` : `停用（登 r${row.revision + 1}）`,
          onRun: () => onOpenForm(row.family, row.toggleActiveSeed),
        },
      );
      if (row.childAction) {
        const child = row.childAction;
        actions.push({ label: child.label, onRun: () => onOpenForm(child.targetId, child.values) });
      }
    }
    return {
      title: row.name,
      subtitle: row.code,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '所属', value: row.place || '—', mono: true },
            { label: '说明', value: row.note || '—' },
            { label: '修订', value: `r${row.revision}`, mono: true },
            { label: '登记时刻', value: formatInstant(row.registeredAt), mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: activityLabel(row.deactivated) }] },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
      ],
    };
  };

  return (
    <ListPageTemplate<CatalogueRow>
      moduleId="warehouse-catalogues"
      title={`${label}名册`}
      description={`本租户已登记的${label}当前修订。点右上角「新增${label}」原地弹窗登记；单击行可从右栏修改修订、停用或新增下级。`}
      headerActions={
        mayWrite ? (
          <Button
            type="button"
            className={primaryActionClass}
            onClick={() =>
              onOpenForm(family, {
                revision: '1',
                ...(supportsWarehouseFilter && warehouseFilter ? { warehouseCode: warehouseFilter } : {}),
              })
            }
          >
            <Plus className="h-3.5 w-3.5 mr-1" />
            {`新增${label}`}
          </Button>
        ) : undefined
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: `搜${label}编码或名称` }}
      filters={
        <>
          {supportsWarehouseFilter && lookups.warehouses.length > 0 ? (
            <select
              className="h-6 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-caption text-idpxyz-text"
              value={warehouseFilter}
              onChange={(event) => setWarehouseFilter(event.target.value)}
            >
              <option value="">全部仓库</option>
              {lookups.warehouses.map((w) => (
                <option key={w.warehouseCode} value={w.warehouseCode}>
                  {`${w.warehouseCode} · ${w.name}`}
                </option>
              ))}
            </select>
          ) : null}
          {activityFilters.map((filter) => (
            <button
              key={filter.id}
              type="button"
              className={chipClass(filter.id === activity)}
              aria-pressed={filter.id === activity}
              onClick={() => setActivity(filter.id)}
            >
              {filter.label}
            </button>
          ))}
        </>
      }
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      inspector={inspectorOf}
      pagination={pagination}
      emptyRowsNote={`当前条件下没有匹配的${label}`}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET ${catalogueReadPaths[family]}`,
        emptyTitle: `还没有登记${label}`,
        emptyDescription: entryHint(
          mayWrite,
          `点右上角「新增${label}」即可逐格填写登记，也可以用受控批量命令行 parcel-warehouse-register 导入。`,
        ),
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/** 仓库与库位五册：仓库 / 库区 / 库位 / 库位类型 / 包材 分签查阅，右上角弹窗新增、右栏快捷修订与级联新增。 */
export function WarehouseCataloguesPage() {
  const mayWrite = useMayWriteRegistry();
  const lookups = useWarehouseLookups();
  const [family, setFamily] = useState<CatalogueFamily>('warehouse');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const openForm = (targetId: CatalogueFamily, values?: GuidedFormValues) => {
    setDrawer((current) => ({
      target: targetsByFamily[targetId],
      prefill: values,
      nonce: (current?.nonce ?? 0) + 1,
    }));
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs
        value={family}
        onValueChange={(next) => setFamily(next as CatalogueFamily)}
        className="flex-1 flex flex-col overflow-hidden gap-0"
      >
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="warehouse">仓库</TabsTrigger>
          <TabsTrigger value="zone">库区</TabsTrigger>
          <TabsTrigger value="location">库位</TabsTrigger>
          <TabsTrigger value="location-type">库位类型</TabsTrigger>
          <TabsTrigger value="packaging">包材</TabsTrigger>
        </TabsList>
        <TabsContent value={family} className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <FamilyTable
            family={family}
            mayWrite={mayWrite}
            lookups={lookups}
            refreshKey={refreshKey}
            onOpenForm={openForm}
          />
        </TabsContent>
      </Tabs>
      <ActionDrawer
        moduleId="warehouse-catalogues"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => {
          setRefreshKey((n) => n + 1);
          lookups.reload();
        }}
      />
    </div>
  );
}
