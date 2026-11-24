import { useState } from 'react';
import { CalendarClock, Plus, Settings } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listChargeOccurrences,
  listWarehouseAdoptions,
  listWarehouseChargeRequests,
  listWarehousePlans,
  type ChargeOccurrenceRecord,
  type WarehouseAdoptionRecord,
  type WarehouseChargeRequestRecord,
  type WarehousePlanRecord,
} from './api';
import { pricingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-pricing'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const planColumns: ListColumn<WarehousePlanRecord>[] = [
  { id: 'id', header: '方案标识', className: mono, render: (row) => row.id },
  { id: 'version', header: '版本', className: mono, render: (row) => row.version },
  { id: 'scope', header: '定价口径', className: mono, render: (row) => row.scope },
  { id: 'currency', header: '币种', className: mono, render: (row) => row.currency },
  { id: 'counts', header: '分组 / 规则', className: mono, render: (row) => `${row.groupCount} / ${row.ruleCount}` },
  { id: 'from', header: '生效时刻', className: mono, render: (row) => formatInstant(row.effectiveFrom) },
];

const adoptionColumns: ListColumn<WarehouseAdoptionRecord>[] = [
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'scope', header: '定价口径', className: mono, render: (row) => row.pricingScope },
  { id: 'account', header: '结算账户', className: mono, render: (row) => row.account },
  { id: 'rev', header: '修订', className: mono, render: (row) => `r${row.revision}` },
  { id: 'eff', header: '生效时刻', className: mono, render: (row) => formatInstant(row.effectiveAt) },
];

const occurrenceColumns: ListColumn<ChargeOccurrenceRecord>[] = [
  { id: 'id', header: '发生项号', className: mono, render: (row) => `${row.occurrenceId} v${row.version}` },
  { id: 'kind', header: '收费类别', render: (row) => row.kind },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'correction', header: '原项 / 更正', render: (row) => row.correction },
  { id: 'time', header: '业务时刻', className: mono, render: (row) => formatInstant(row.businessTime) },
];

const requestColumns: ListColumn<WarehouseChargeRequestRecord>[] = [
  { id: 'req', header: '计费请求号', className: mono, render: (row) => row.requestId ?? '未发请求' },
  { id: 'occ', header: '收费发生项', className: mono, render: (row) => `${row.occurrenceId} v${row.version}` },
  { id: 'status', header: '状态', render: (row) => row.status },
  { id: 'account', header: '结算账户 / 币种', className: mono, render: (row) => `${row.account ?? '—'} · ${row.settlementCurrency ?? '—'}` },
  { id: 'charge', header: '计费结果', className: mono, render: (row) => row.chargeId ?? '—' },
];

function PlansTab({ refreshKey, onOpen }: { refreshKey: number; onOpen: (id: string, prefill?: GuidedFormValues) => void }) {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehousePlans);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const rows = answer?.kind === 'outcome' ? answer.body.plans : [];

  return (
    <ListPageTemplate<WarehousePlanRecord>
      moduleId="warehouse-pricing"
      title="仓储计费方案"
      description="操作费、仓租与增值服务价卡方案。仓重量体积单位、库龄口径与增值服务项目统一在「仓配规则设置」管理。"
      headerActions={
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
            <Settings className="h-3.5 w-3.5 mr-1" />
            计费口径设置
          </Button>
          <Button type="button" className={primaryActionClass} onClick={() => onOpen('warehouse-plan')}>
            <Plus className="h-3.5 w-3.5 mr-1" />
            登记计费方案
          </Button>
        </div>
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜方案标识或口径' }}
      columns={planColumns}
      rows={rows}
      rowKey={(row) => `${row.id}/${row.version}`}
      inspector={(row): InspectorContent => ({
        title: `${row.id} · ${row.version}`,
        subtitle: row.scope,
        sections: [
          {
            kind: 'summary',
            fields: [
              { label: '币种 / 声明值', value: `${row.currency} · ${row.declaredValue}`, mono: true },
              { label: '分组数 / 规则数', value: `${row.groupCount} / ${row.ruleCount}`, mono: true },
              { label: '生效时刻', value: formatInstant(row.effectiveFrom), mono: true },
            ],
          },
          {
            kind: 'actions',
            actions: [{ label: '为此方案登记货主采用', onRun: () => onOpen('charge-adoption', { pricingScope: row.scope }) }],
          },
        ],
      })}
      pagination={pagination}
      emptyRowsNote="当前条件下没有仓储计费方案"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /pricing-warehouse-plan-list',
        emptyTitle: '还没有登记仓储计费方案',
        emptyDescription: '点右上角「登记计费方案」为操作费、仓租或增值服务建价卡。',
        narrowed: settled.trim() !== '',
      })}
    />
  );
}

function AdoptionsTab({ refreshKey, onOpen }: { refreshKey: number; onOpen: (id: string, prefill?: GuidedFormValues) => void }) {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseAdoptions);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const rows = answer?.kind === 'outcome' ? answer.body.adoptions : [];

  return (
    <ListPageTemplate<WarehouseAdoptionRecord>
      moduleId="warehouse-pricing"
      title="货主计费采用"
      description="把货主在某个仓库的仓储收费绑定到结算账户与定价口径；采用后新发生的收费项会自动发出销售计费请求。"
      headerActions={
        <Button type="button" className={primaryActionClass} onClick={() => onOpen('charge-adoption')}>
          <Plus className="h-3.5 w-3.5 mr-1" />
          登记计费采用
        </Button>
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜货主、仓或定价口径' }}
      columns={adoptionColumns}
      rows={rows}
      rowKey={(row) => `${row.ownerAccount}/${row.warehouseCode}/${row.revision}`}
      inspector={(row): InspectorContent => ({
        title: `${row.ownerAccount} · ${row.warehouseCode}`,
        subtitle: `r${row.revision}`,
        sections: [
          {
            kind: 'summary',
            fields: [
              { label: '结算账户', value: row.account, mono: true },
              { label: '定价口径', value: row.pricingScope, mono: true },
              { label: '生效时刻', value: formatInstant(row.effectiveAt), mono: true },
              { label: '依据', value: row.basis },
            ],
          },
          {
            kind: 'actions',
            actions: [
              {
                label: `登记下一修订（r${row.revision + 1}）`,
                onRun: () =>
                  onOpen('charge-adoption', {
                    ownerAccount: row.ownerAccount,
                    warehouse: row.warehouseCode,
                    revision: String(row.revision + 1),
                    account: row.account,
                    pricingScope: row.pricingScope,
                  }),
              },
            ],
          },
          { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
        ],
      })}
      pagination={pagination}
      emptyRowsNote="当前条件下没有计费采用记录"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-adoption-list',
        emptyTitle: '还没有登记计费采用',
        emptyDescription: '点右上角「登记计费采用」把货主与仓绑定到仓储计费方案。',
        narrowed: settled.trim() !== '',
      })}
    />
  );
}

function OccurrencesTab({ refreshKey, onOpen }: { refreshKey: number; onOpen: (id: string, prefill?: GuidedFormValues) => void }) {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listChargeOccurrences);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const rows = answer?.kind === 'outcome' ? answer.body.occurrences : [];

  return (
    <ListPageTemplate<ChargeOccurrenceRecord>
      moduleId="warehouse-pricing"
      title="仓储收费发生项"
      description="入库、拣货、打包、发货、增值服务与日结仓租事实产出的可计费发生项。"
      headerActions={
        <Button type="button" className={primaryActionClass} onClick={() => onOpen('storage-day-close')}>
          <CalendarClock className="h-3.5 w-3.5 mr-1" />
          触发仓租日结
        </Button>
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜发生项号、货主或仓' }}
      columns={occurrenceColumns}
      rows={rows}
      rowKey={(row) => `${row.occurrenceId}/v${row.version}`}
      inspector={(row): InspectorContent => ({
        title: `${row.occurrenceId} · v${row.version}`,
        subtitle: row.kind,
        sections: [
          {
            kind: 'summary',
            fields: [
              { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
              { label: '来源事实', value: row.sourceFact, mono: true },
              { label: '业务时刻', value: formatInstant(row.businessTime), mono: true },
              {
                label: '计费量',
                value: row.measures.map((m) => `${m.unit}:${m.unmeasured ? '未量' : m.quantity ?? ''}`).join('，') || '—',
                mono: true,
              },
              {
                label: '附加属性',
                value: row.attributes.map((a) => `${a.name}=${a.value}`).join('，') || '—',
                mono: true,
              },
            ],
          },
        ],
      })}
      pagination={pagination}
      emptyRowsNote="当前条件下没有收费发生项"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-occurrences',
        emptyTitle: '还没有收费发生项',
        emptyDescription: '完成仓内收货/出库作业，或点右上角「触发仓租日结」后会生成收费发生项。',
        narrowed: settled.trim() !== '',
      })}
    />
  );
}

function RequestsTab({ refreshKey }: { refreshKey: number }) {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseChargeRequests);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];

  return (
    <ListPageTemplate<WarehouseChargeRequestRecord>
      moduleId="warehouse-pricing"
      title="销售计费请求"
      description="仓储收费发生项发往结算计费上下文的销售计费请求（前缀 WHREQ）与过账状态。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜请求号、发生项号或账户' }}
      columns={requestColumns}
      rows={rows}
      rowKey={(row) => `${row.occurrenceId}/v${row.version}`}
      inspector={(row): InspectorContent => ({
        title: row.requestId ?? `${row.occurrenceId} v${row.version}`,
        subtitle: row.status,
        sections: [
          {
            kind: 'summary',
            fields: [
              { label: '发生项', value: `${row.occurrenceId} v${row.version}`, mono: true },
              { label: '结算账户 / 币种', value: `${row.account ?? '—'} · ${row.settlementCurrency ?? '—'}`, mono: true },
              { label: '定价口径', value: row.pricingScope ?? '—', mono: true },
              { label: '定价基准时刻', value: row.pricingBasisAt ? formatInstant(row.pricingBasisAt) : '—', mono: true },
              { label: '计费结果 / 评估', value: `${row.chargeId ?? '—'} / ${row.evaluationId ?? '—'}`, mono: true },
            ],
          },
        ],
      })}
      pagination={pagination}
      emptyRowsNote="当前条件下没有销售计费请求"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-requests',
        emptyTitle: '还没有仓储计费请求',
        emptyDescription: '登记「货主计费采用」后，新产生的收费发生项会自动投递销售计费请求。',
        narrowed: settled.trim() !== '',
      })}
    />
  );
}

/** 仓储计费页：方案、采用、发生项与销售计费请求四签台账，操作通过右上角弹窗原地完成并刷新。 */
export function WarehousePricingPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(pricingActionSpecs);
  const [tab, setTab] = useState('plans');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="plans">计费方案</TabsTrigger>
          <TabsTrigger value="adoptions">货主采用</TabsTrigger>
          <TabsTrigger value="occurrences">收费发生项</TabsTrigger>
          <TabsTrigger value="requests">销售计费请求</TabsTrigger>
        </TabsList>
        <TabsContent value="plans" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <PlansTab refreshKey={refreshKey} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="adoptions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <AdoptionsTab refreshKey={refreshKey} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="occurrences" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OccurrencesTab refreshKey={refreshKey} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="requests" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RequestsTab refreshKey={refreshKey} />
        </TabsContent>
      </Tabs>
      <ActionDrawer
        moduleId="warehouse-pricing"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setRefreshKey((n) => n + 1)}
      />
    </div>
  );
}
