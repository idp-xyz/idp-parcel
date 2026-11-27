import { useMemo, useState, type ReactNode } from 'react';
import { Plus, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
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
  catalogueFamilyOf,
  catalogueReadPaths,
  catalogueWritePaths,
  warehouseKindLabel,
  zoneUsageLabel,
  type ActivityFilter,
  type CatalogueFamily,
} from './rows';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { useWarehouseLookups, type WarehouseLookups } from './use-catalogues';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';
import { useStatusCounts } from './use-status-counts';

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

const moduleId = 'warehouse-catalogues';

function moduleKey(family: CatalogueFamily): string {
  return `${moduleId}.${family}`;
}

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

const noRows: CatalogueRow[] = [];
const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

// 五册各自只开一个可排维，缺省都是从低到高。库区与库位排的是仓编码：编码在仓内才唯一，单按它排没有稳定的次序。
const sortFieldOf: Record<CatalogueFamily, string> = {
  warehouse: 'warehouseCode',
  zone: 'warehouseCode',
  location: 'warehouseCode',
  'location-type': 'locationTypeCode',
  packaging: 'packagingMaterialCode',
};

function codeSort(stored: string | null, field: string): WorkbenchSort {
  if (stored === `-${field}`) return { key: field, dir: -1 };
  return { key: field, dir: 1 };
}

const activityFilters: ReadonlyArray<{ id: ActivityFilter; label: string }> = [
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
  const [keyword, setKeyword] = useTabViewState(`${moduleKey(family)}.keyword`, '');
  const [activity, setActivity] = useTabViewState<ActivityFilter>(`${moduleKey(family)}.activity`, 'all');
  const [warehouseFilter, setWarehouseFilter] = useTabViewState(`${moduleKey(family)}.warehouse`, '');
  const [storedSort, setSort] = useTabViewState<string | null>(`${moduleKey(family)}.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleKey(family)}.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  const supportsWarehouseFilter = family === 'zone' || family === 'location';
  const sortField = sortFieldOf[family];
  const sort = codeSort(storedSort, sortField);
  const sortByPlace = family === 'zone' || family === 'location';

  const conditions = useMemo(() => {
    const filters: Record<string, readonly string[]> = {};
    if (activity !== 'all') filters.deactivated = [activity === 'deactivated' ? 'true' : 'false'];
    if (supportsWarehouseFilter && warehouseFilter !== '') filters.warehouseCode = [warehouseFilter];
    return {
      selectors: { family },
      q: settled,
      sort: sort.dir === 1 ? sortField : `-${sortField}`,
      filters: Object.keys(filters).length > 0 ? filters : undefined,
    };
  }, [family, settled, activity, supportsWarehouseFilter, warehouseFilter, sort.dir, sortField]);
  const countBase = useMemo(() => {
    const filters: Record<string, readonly string[]> = {};
    if (supportsWarehouseFilter && warehouseFilter !== '') filters.warehouseCode = [warehouseFilter];
    return { selectors: { family }, q: settled, filters: Object.keys(filters).length > 0 ? filters : undefined };
  }, [family, settled, supportsWarehouseFilter, warehouseFilter]);

  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listWarehouseCatalogue);
  const [lastRefresh, setLastRefresh] = useState(refreshKey);
  if (refreshKey !== lastRefresh) {
    setLastRefresh(refreshKey);
    retry();
    setRefreshes((count) => count + 1);
  }
  const held = heldCataloguePage(answer, previous, (body) => rowsOf(family, body).length > 0);
  const shown = answer?.kind === 'outcome' ? answer.body : held?.body;
  const rows = shown ? rowsOf(family, shown) : noRows;
  const counts = useStatusCounts(listWarehouseCatalogue, countBase, 'deactivated', ['false', 'true'], refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const label = familyLabels[family];
  const warehouseNames = useMemo(
    () => new Map(lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [lookups.warehouses],
  );
  const now = new Date();

  const placeOf = (row: CatalogueRow) => {
    if (row.family !== 'zone' && row.family !== 'location') return nowrap(row.place || '—');
    const name = nameOf(warehouseNames, row.place);
    return name ? <WmsTwoLine primary={name} secondary={row.place} code title={`${name} · ${row.place}`} /> : <span className="whitespace-nowrap font-mono">{row.place}</span>;
  };
  const columns: WorkbenchColumn<CatalogueRow>[] = [
    {
      key: 'name',
      header: family === 'location' ? '库位' : '名称',
      width: fitColumnWidth,
      sortKey: sortByPlace ? undefined : sortField,
      render: (row) =>
        row.name === row.code ? (
          <span className="whitespace-nowrap font-mono">{row.code}</span>
        ) : (
          <WmsTwoLine primary={row.name} secondary={row.code} code title={`${row.name}\n${row.code}`} />
        ),
    },
    {
      key: 'status',
      header: '状态',
      width: fitColumnWidth,
      render: (row) => <WmsBadge code={row.deactivated ? 'DEACTIVATED' : 'ACTIVE'} label={activityLabel(row.deactivated)} />,
    },
    {
      key: 'place',
      header: '所属',
      width: fitColumnWidth,
      minListWidth: 820,
      sortKey: sortByPlace ? sortField : undefined,
      render: placeOf,
    },
    {
      key: 'note',
      header: '说明',
      render: (row) => (row.note ? <WmsTwoLine fill primary={row.note} title={row.note} /> : <span className="text-idpxyz-textMuted">—</span>),
    },
    {
      key: 'revision',
      header: '修订',
      width: fitColumnWidth,
      minListWidth: 1080,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`r${row.revision}`}</span>,
    },
    {
      key: 'at',
      header: nowrap('登记'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1220,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.registeredAt)}>
          {shortInstant(row.registeredAt, now) || '—'}
        </span>
      ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => <CatalogueStep row={row} mayWrite={mayWrite} onOpenForm={onOpenForm} />,
    },
  ];

  const chooseActivity = (next: ActivityFilter) => setActivity(next);
  const queueStats: WmsQueueStat[] = [
    {
      id: 'active',
      label: '在用',
      count: counts['false'] ?? null,
      active: activity === 'active',
      onClick: () => chooseActivity(activity === 'active' ? 'all' : 'active'),
    },
    {
      id: 'deactivated',
      label: '已停用',
      count: counts['true'] ?? null,
      active: activity === 'deactivated',
      onClick: () => chooseActivity(activity === 'deactivated' ? 'all' : 'deactivated'),
    },
  ];
  const narrowed = catalogueConditionsNarrow(conditions) || pastFirstPage(pagination);
  const clearFilters = () => {
    setKeyword('');
    setActivity('all');
    setWarehouseFilter('');
    setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: `GET ${catalogueReadPaths[family]}`,
        emptyTitle: `还没有登记${label}`,
        emptyDescription: entryHint(
          mayWrite,
          `点右上角「新增${label}」即可逐格填写登记，也可以用受控批量命令行 parcel-warehouse-register 导入。`,
        ),
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <WorkbenchPageTemplate<CatalogueRow>
      title={`${label}名册`}
      viewStateKey={moduleKey(family)}
      subtitle="点一行在右边看。改资料、停用或加下级都在右边"
      titleAside={<WmsQueueStats items={queueStats} />}
      onRefresh={refresh}
      refreshing={answer === null}
      primaryAction={
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
            <Plus className="mr-1 h-3.5 w-3.5" />
            {`新增${label}`}
          </Button>
        ) : undefined
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: `搜${label}编码或名称` }}
      extraFilters={
        <>
          <WmsFilterSelect
            label="状态"
            value={activity === 'all' ? '' : activity}
            onChange={(next) => chooseActivity(next === 'active' || next === 'deactivated' ? next : 'all')}
            options={activityFilters.map((filter) => ({ value: filter.id, label: filter.label }))}
          />
          {supportsWarehouseFilter ? (
            <WmsFilterSelect
              label="仓库"
              value={warehouseFilter}
              onChange={setWarehouseFilter}
              options={lookups.warehouses.map((warehouse) => ({
                value: warehouse.warehouseCode,
                label: warehouse.name ? `${warehouse.name} · ${warehouse.warehouseCode}` : warehouse.warehouseCode,
              }))}
            />
          ) : null}
          {narrowed ? (
            <button
              type="button"
              className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
              onClick={clearFilters}
            >
              清除
            </button>
          ) : null}
        </>
      }
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        setSort(next.dir === 1 ? sortField : `-${sortField}`);
      }}
      noMatchText={`当前条件下没有匹配的${label}`}
      onClearFilters={clearFilters}
      renderDetail={(key) => {
        const row = rows.find((item) => item.key === key);
        return row ? (
          <CatalogueDetail row={row} mayWrite={mayWrite} onOpenForm={onOpenForm} onClose={() => setOpenKey(null)} />
        ) : (
          <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这一条。</p>
        );
      }}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}

function CatalogueStep({
  row,
  mayWrite,
  onOpenForm,
}: {
  row: CatalogueRow;
  mayWrite: boolean;
  onOpenForm: (targetId: CatalogueFamily, values?: GuidedFormValues) => void;
}) {
  if (!mayWrite) return <span className="text-idpxyz-textMuted">—</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        onOpenForm(row.family, row.toggleActiveSeed);
      }}
    >
      {row.deactivated ? '重新启用' : '停用'}
    </Button>
  );
}

function CatalogueDetail({
  row,
  mayWrite,
  onOpenForm,
  onClose,
}: {
  row: CatalogueRow;
  mayWrite: boolean;
  onOpenForm: (targetId: CatalogueFamily, values?: GuidedFormValues) => void;
  onClose: () => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">{familyLabels[row.family]}</span>
            <span className="font-mono">{row.code}</span>
            <WmsBadge code={row.deactivated ? 'DEACTIVATED' : 'ACTIVE'} label={activityLabel(row.deactivated)} />
          </div>
          <p className="mt-1 type-caption text-idpxyz-textMuted">{row.name === row.code ? row.place || '—' : row.name}</p>
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">
        <p>{row.note || '没有规格说明'}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">修订 r{row.revision}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">
          登记 {row.registrant} · {formatInstant(row.registeredAt)}
        </p>
        {mayWrite ? (
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant="outline" onClick={() => onOpenForm(row.family, row.nextRevisionSeed)}>
              {`修改资料（登 r${row.revision + 1}）`}
            </Button>
            <CatalogueStep row={row} mayWrite={mayWrite} onOpenForm={onOpenForm} />
            {row.childAction ? (
              <Button type="button" size="sm" variant="outline" onClick={() => onOpenForm(row.childAction!.targetId, row.childAction!.values)}>
                {row.childAction.label}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** 仓库与库位五册。没有各自的单据页：点一行在右边看，行上只留停用或重新启用。 */
export function WarehouseCataloguesPage() {
  const mayWrite = useMayWriteRegistry();
  const lookups = useWarehouseLookups();
  const [storedFamily, setFamily] = useTabViewState('warehouse-catalogues.tab', 'warehouse');
  const family = catalogueFamilyOf(storedFamily);
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
        onValueChange={(next) => setFamily(catalogueFamilyOf(next))}
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
            key={family}
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
