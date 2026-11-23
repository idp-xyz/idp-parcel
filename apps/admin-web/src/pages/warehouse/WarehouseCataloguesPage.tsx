import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
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
import {
  catalogueOutcomeLabels,
  catalogueRefusalLabels,
  familyLabels,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import {
  activityLabel,
  catalogueConditions,
  catalogueFamilies,
  catalogueReadPaths,
  catalogueWritePaths,
  warehouseKindLabel,
  zoneUsageLabel,
  type ActivityFilter,
  type CatalogueFamily,
} from './rows';

const info = moduleInfoById['warehouse-catalogues'];

interface CatalogueRow {
  key: string;
  code: string;
  name: string;
  place: string;
  note: string;
  deactivated: boolean;
  revision: number;
  registrant: string;
  registeredAt: string;
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
  return {
    key: row.warehouseCode,
    code: row.warehouseCode,
    name: row.name,
    place: row.address.country,
    note: warehouseKindLabel(row.kind),
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
  };
}

function zoneRow(row: ZoneRecord): CatalogueRow {
  return {
    key: `${row.warehouseCode}/${row.zoneCode}`,
    code: row.zoneCode,
    name: row.name,
    place: row.warehouseCode,
    note: zoneUsageLabel(row.usage),
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
  };
}

function locationTypeRow(row: LocationTypeRecord): CatalogueRow {
  return {
    key: row.locationTypeCode,
    code: row.locationTypeCode,
    name: row.name,
    place: '',
    note: '',
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
  };
}

function locationRow(row: LocationRecord): CatalogueRow {
  const type = row.locationTypeCode ?? '未指定类型';
  const priority = row.priority === undefined ? '' : ` · 优先级 ${row.priority}`;
  return {
    key: `${row.warehouseCode}/${row.locationCode}`,
    code: row.locationCode,
    name: row.locationCode,
    place: row.warehouseCode,
    note: `库区 ${row.zoneCode} · ${type}${priority}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
  };
}

function packagingRow(row: PackagingRecord): CatalogueRow {
  const size = row.dimensions;
  return {
    key: row.packagingMaterialCode,
    code: row.packagingMaterialCode,
    name: row.name,
    place: '',
    note: `${size.length}×${size.width}×${size.height} ${size.unit} · ${row.weight.value} ${row.weight.unit}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
  };
}

const columns: ListColumn<CatalogueRow>[] = [
  { id: 'name', header: '名称', render: (row) => row.name },
  { id: 'code', header: '编码', className: mono, render: (row) => row.code },
  { id: 'place', header: '所在', className: mono, render: (row) => row.place },
  { id: 'note', header: '说明', render: (row) => row.note },
  { id: 'activity', header: '状态', render: (row) => activityLabel(row.deactivated) },
  { id: 'revision', header: '修订', className: mono, render: (row) => String(row.revision) },
];

function inspectorOf(row: CatalogueRow): InspectorContent {
  return {
    title: row.name,
    subtitle: row.code,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '所在', value: row.place || '—' },
          { label: '说明', value: row.note || '—' },
          { label: '修订', value: String(row.revision), mono: true },
          { label: '登记时刻', value: formatInstant(row.registeredAt), mono: true },
        ],
      },
      { kind: 'status', items: [{ label: '状态', word: activityLabel(row.deactivated) }] },
      { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
    ],
  };
}

const activityFilters: ReadonlyArray<{ id: ActivityFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'active', label: '在用' },
  { id: 'deactivated', label: '已停用' },
];

function CatalogueTable() {
  const [family, setFamily] = useState<CatalogueFamily>('warehouse');
  const [keyword, setKeyword] = useState('');
  const [activity, setActivity] = useState<ActivityFilter>('all');
  const settled = useSettledKeyword(keyword);
  const conditions = catalogueConditions(family, settled, activity);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseCatalogue);
  const mayWrite = useMayWriteRegistry();
  const rows = answer?.kind === 'outcome' ? rowsOf(family, answer.body) : [];
  const label = familyLabels[family];

  return (
    <ListPageTemplate<CatalogueRow>
      moduleId="warehouse-catalogues"
      title={info.title}
      description="各仓、库区、库位类型、库位和包材的当前修订。改资料是登下一个修订，不覆盖这一笔。"
      search={{ value: keyword, onChange: setKeyword, placeholder: `搜${label}的编码或名称` }}
      filters={
        <>
          {catalogueFamilies.map((item) => (
            <button key={item} type="button" className={chipClass(item === family)} aria-pressed={item === family} onClick={() => setFamily(item)}>
              {familyLabels[item]}
            </button>
          ))}
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
      emptyRowsNote="当前条件下没有匹配的修订"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET ${catalogueReadPaths[family]}`,
        emptyTitle: `还没有登记${label}`,
        emptyDescription: entryHint(mayWrite, `在「新增或更新」签里登记${label}后，会在这里出现。`),
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

const registrationTargets: RegistrationTarget[] = catalogueFamilies.map((family) => ({
  id: family,
  label: familyLabels[family],
  title: registrationTitles[family],
  endpoint: `POST ${catalogueWritePaths[family]}`,
  snapshotHint: registrationSnapshotHints[family],
  submit: (snapshot) => registerCatalogue(family, snapshot),
  outcomeLabels: catalogueOutcomeLabels,
  refusalReasonLabels: catalogueRefusalLabels,
}));

/**
 * 五个名册的当前修订，外加登记签。
 * 登记签收的是与受控批量口同一份报文。写入口按登记册配置写授予收起。
 */
export function WarehouseCataloguesPage() {
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">仓库与库位</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">新增或更新</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalog" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CatalogueTable />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <MultiRegistrationPanel moduleId="warehouse-catalogues" targets={registrationTargets} problemNote={problemNote} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
