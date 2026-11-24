import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
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
import { GuidedActionPanel, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormSeed, GuidedFormValues } from './guided-forms';
import { catalogueGuidedForms } from './guided-schemas';
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
  return {
    key: row.warehouseCode,
    family: 'warehouse',
    code: row.warehouseCode,
    name: row.name,
    place: row.address.country,
    note: warehouseKindLabel(row.kind),
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: {
      warehouseCode: row.warehouseCode,
      revision: String(row.revision + 1),
      name: row.name,
      kind: row.kind,
      'address.country': row.address.country,
      'address.text': row.address.text,
      businessTimeZone: row.businessTimeZone ?? '',
      colocatedNode: row.colocatedNode ?? '',
      deactivated: row.deactivated,
    },
    childAction: {
      label: '在此仓新增库区',
      targetId: 'zone',
      values: { warehouseCode: row.warehouseCode, revision: '1' },
    },
  };
}

function zoneRow(row: ZoneRecord): CatalogueRow {
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
    nextRevisionSeed: {
      warehouseCode: row.warehouseCode,
      zoneCode: row.zoneCode,
      revision: String(row.revision + 1),
      name: row.name,
      usage: row.usage,
      deactivated: row.deactivated,
    },
    childAction: {
      label: '在此库区新增库位',
      targetId: 'location',
      values: { warehouseCode: row.warehouseCode, zoneCode: row.zoneCode, revision: '1' },
    },
  };
}

function locationTypeRow(row: LocationTypeRecord): CatalogueRow {
  return {
    key: row.locationTypeCode,
    family: 'location-type',
    code: row.locationTypeCode,
    name: row.name,
    place: '',
    note: '',
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: {
      locationTypeCode: row.locationTypeCode,
      revision: String(row.revision + 1),
      name: row.name,
      deactivated: row.deactivated,
    },
  };
}

function locationRow(row: LocationRecord): CatalogueRow {
  const type = row.locationTypeCode ?? '未指定类型';
  const priority = row.priority === undefined ? '' : ` · 优先级 ${row.priority}`;
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
    nextRevisionSeed: {
      warehouseCode: row.warehouseCode,
      locationCode: row.locationCode,
      revision: String(row.revision + 1),
      zoneCode: row.zoneCode,
      locationTypeCode: row.locationTypeCode ?? '',
      priority: row.priority === undefined ? '' : String(row.priority),
      deactivated: row.deactivated,
    },
  };
}

function packagingRow(row: PackagingRecord): CatalogueRow {
  const size = row.dimensions;
  return {
    key: row.packagingMaterialCode,
    family: 'packaging',
    code: row.packagingMaterialCode,
    name: row.name,
    place: '',
    note: `${size.length}×${size.width}×${size.height} ${size.unit} · ${row.weight.value} ${row.weight.unit}`,
    deactivated: row.deactivated,
    revision: row.revision,
    registrant: row.registrant,
    registeredAt: row.registeredAt,
    nextRevisionSeed: {
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
    },
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

const activityFilters: ReadonlyArray<{ id: ActivityFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'active', label: '在用' },
  { id: 'deactivated', label: '已停用' },
];

const registrationTargets: GuidedActionTarget[] = catalogueFamilies.map((family) => ({
  id: family,
  label: familyLabels[family],
  title: registrationTitles[family],
  endpoint: `POST ${catalogueWritePaths[family]}`,
  snapshotHint: registrationSnapshotHints[family],
  submit: (snapshot) => registerCatalogue(family, snapshot),
  outcomeLabels: catalogueOutcomeLabels,
  refusalReasonLabels: catalogueRefusalLabels,
  form: catalogueGuidedForms[family],
}));

function CatalogueTable({
  mayWrite,
  onPrefill,
}: {
  mayWrite: boolean;
  onPrefill: (targetId: CatalogueFamily, values: GuidedFormValues) => void;
}) {
  const [family, setFamily] = useState<CatalogueFamily>('warehouse');
  const [keyword, setKeyword] = useState('');
  const [activity, setActivity] = useState<ActivityFilter>('all');
  const settled = useSettledKeyword(keyword);
  const conditions = catalogueConditions(family, settled, activity);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseCatalogue);
  const rows = answer?.kind === 'outcome' ? rowsOf(family, answer.body) : [];
  const label = familyLabels[family];

  const inspectorOf = (row: CatalogueRow): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (mayWrite) {
      actions.push({
        label: `登记下一修订（r${row.revision + 1}）`,
        onRun: () => onPrefill(row.family, row.nextRevisionSeed),
      });
      if (row.childAction) {
        const child = row.childAction;
        actions.push({
          label: child.label,
          onRun: () => onPrefill(child.targetId, child.values),
        });
      }
    }
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
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
      ],
    };
  };

  return (
    <ListPageTemplate<CatalogueRow>
      moduleId="warehouse-catalogues"
      title={info.title}
      description="各仓、库区、库位类型、库位和包材的当前修订。单击行可从右栏直接带出资料登记下一修订。"
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
      emptyRowsNote={`当前条件下没有匹配的${label}`}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET ${catalogueReadPaths[family]}`,
        emptyTitle: `还没有登记${label}`,
        emptyDescription: entryHint(
          mayWrite,
          `可以在「新增或更新」签逐格填写登记${label}，也可以用受控批量命令行 parcel-warehouse-register 一次登记多条。`,
        ),
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/** 仓库与库位五册的查阅与引导式登记。没有登记册配置写授予的操作者只看查阅签。 */
export function WarehouseCataloguesPage() {
  const mayWrite = useMayWriteRegistry();
  const [tab, setTab] = useState('catalog');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);

  const openPrefill = (targetId: CatalogueFamily, values: GuidedFormValues) => {
    setSeed((current) => ({ targetId, values, nonce: (current?.nonce ?? 0) + 1 }));
    setTab('register');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">仓库与库位</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">新增或更新</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalog" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CatalogueTable mayWrite={mayWrite} onPrefill={openPrefill} />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <GuidedActionPanel
              moduleId="warehouse-catalogues"
              targets={registrationTargets}
              problemNote={problemNote}
              seed={seed}
            />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
