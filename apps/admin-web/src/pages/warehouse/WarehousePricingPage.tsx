import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
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
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';

const info = moduleInfoById['warehouse-pricing'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const occurrenceKindLabels: Record<string, string> = {
  INBOUND_HANDLING: '入库作业',
  OUTBOUND_HANDLING: '出库作业',
  STORAGE: '仓租',
  RETURN_HANDLING: '退货处理',
  VALUE_ADDED: '增值服务',
  PROCESSING: '加工',
  DESTRUCTION: '销毁',
};

const chargeRequestStatusLabels: Record<string, string> = {
  FORMED: '已形成预估',
  CONFIRMED: '已确认',
  DIFFERENCE: '已按差额结清',
  ZERO: '合计为零',
  PENDING: '待判断',
  NOT_CONFIGURED: '未配置',
  AWAITING_PERSON: '待人处理',
  SUPERSEDED: '已被后版替代',
};

const planColumns: ListColumn<WarehousePlanRecord>[] = [
  { id: 'id', header: '方案标识', className: mono, render: (row) => row.id },
  { id: 'version', header: '版本', className: mono, render: (row) => row.version },
  { id: 'scope', header: '计价范围', className: mono, render: (row) => row.scope },
  { id: 'currency', header: '币种', className: mono, render: (row) => row.currency },
  { id: 'rules', header: '组 / 规则', className: mono, render: (row) => `${row.groupCount} / ${row.ruleCount}` },
  { id: 'effective', header: '生效时刻', className: mono, render: (row) => formatInstant(row.effectiveFrom) },
];

function planInspector(row: WarehousePlanRecord): InspectorContent {
  return {
    title: row.id,
    subtitle: `版本 ${row.version}`,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '计价范围', value: row.scope, mono: true },
          { label: '币种', value: row.currency, mono: true },
          { label: '申报值口径', value: row.declaredValue, mono: true },
          { label: '收费组数', value: String(row.groupCount), mono: true },
          { label: '规则条数', value: String(row.ruleCount), mono: true },
          { label: '生效时刻', value: formatInstant(row.effectiveFrom), mono: true },
        ],
      },
    ],
  };
}

const adoptionColumns: ListColumn<WarehouseAdoptionRecord>[] = [
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'revision', header: '修订', className: mono, render: (row) => String(row.revision) },
  { id: 'account', header: '结算账户', className: mono, render: (row) => row.account },
  { id: 'scope', header: '计价范围', className: mono, render: (row) => row.pricingScope },
  { id: 'effective', header: '生效时刻', className: mono, render: (row) => formatInstant(row.effectiveAt) },
];

function adoptionInspector(row: WarehouseAdoptionRecord): InspectorContent {
  return {
    title: `${row.ownerAccount} · ${row.warehouseCode}`,
    subtitle: `第 ${row.revision} 版`,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '结算账户', value: row.account, mono: true },
          { label: '计价范围', value: row.pricingScope, mono: true },
          { label: '依据', value: row.basis },
          { label: '生效时刻', value: formatInstant(row.effectiveAt), mono: true },
        ],
      },
      { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
    ],
  };
}

const occurrenceColumns: ListColumn<ChargeOccurrenceRecord>[] = [
  { id: 'id', header: '发生项', className: mono, render: (row) => row.occurrenceId },
  { id: 'version', header: '版本', className: mono, render: (row) => `v${row.version} (${row.correction})` },
  { id: 'kind', header: '种类', render: (row) => occurrenceKindLabels[row.kind] ?? row.kind },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'time', header: '业务时刻', className: mono, render: (row) => formatInstant(row.businessTime) },
];

function occurrenceInspector(row: ChargeOccurrenceRecord): InspectorContent {
  const fields: Array<{ label: string; value: string; mono?: boolean }> = [
    { label: '来源事实', value: row.sourceFact, mono: true },
    { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
    { label: '重量 / 体积单位', value: `${row.weightUnit || '—'} / ${row.volumeUnit || '—'}`, mono: true },
    { label: '明细行数', value: String(row.lineCount), mono: true },
    { label: '业务时刻', value: formatInstant(row.businessTime), mono: true },
  ];
  row.measures.forEach((measure, index) => {
    fields.push({
      label: `计量 ${index + 1}`,
      value: measure.unmeasured
        ? `${measure.unit}（未测量）`
        : `${measure.quantity ?? '0'} ${measure.unit}${measure.class ? ` [${measure.class}]` : ''}`,
      mono: true,
    });
  });
  row.attributes.forEach((attr) => {
    fields.push({ label: attr.name, value: attr.value });
  });
  return {
    title: row.occurrenceId,
    subtitle: `${occurrenceKindLabels[row.kind] ?? row.kind} · 第 ${row.version} 版`,
    sections: [{ kind: 'summary', fields }],
  };
}

const requestColumns: ListColumn<WarehouseChargeRequestRecord>[] = [
  { id: 'occurrence', header: '发生项', className: mono, render: (row) => `${row.occurrenceId} v${row.version}` },
  { id: 'request', header: '计费请求号', className: mono, render: (row) => row.requestId || '—' },
  { id: 'status', header: '状态', render: (row) => chargeRequestStatusLabels[row.status] ?? row.status },
  { id: 'account', header: '结算账户', className: mono, render: (row) => row.account || '—' },
  { id: 'charge', header: '客户费用号', className: mono, render: (row) => row.chargeId || '—' },
  { id: 'evaluation', header: '评价标识', className: mono, render: (row) => row.evaluationId || '—' },
];

function requestInspector(row: WarehouseChargeRequestRecord): InspectorContent {
  return {
    title: row.requestId || `${row.occurrenceId} v${row.version}`,
    subtitle: chargeRequestStatusLabels[row.status] ?? row.status,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '发生项与版本', value: `${row.occurrenceId} v${row.version}`, mono: true },
          { label: '结算账户', value: row.account || '—', mono: true },
          { label: '结算币种', value: row.settlementCurrency || '—', mono: true },
          { label: '计价范围', value: row.pricingScope || '—', mono: true },
          { label: '计价基准时点', value: row.pricingBasisAt ? formatInstant(row.pricingBasisAt) : '—', mono: true },
          { label: '评价标识', value: row.evaluationId || '—', mono: true },
          { label: '客户费用号', value: row.chargeId || '—', mono: true },
        ],
      },
      { kind: 'status', items: [{ label: '状态', word: chargeRequestStatusLabels[row.status] ?? row.status }] },
    ],
  };
}

function PlansTable() {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehousePlans);
  const rows = answer?.kind === 'outcome' ? answer.body.plans : [];

  return (
    <ListPageTemplate<WarehousePlanRecord>
      moduleId="warehouse-pricing"
      title={info.title}
      description="已登记的仓储计费方案。新增方案、登记采用、配置单位与库龄口径及仓租日结在「登记与日结」签。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜方案标识、版本、计价范围或币种' }}
      columns={planColumns}
      rows={rows}
      rowKey={(row) => `${row.id}/${row.version}`}
      inspector={planInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的仓储计费方案"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /pricing-warehouse-plan-list',
        emptyTitle: '还没有仓储计费方案',
        emptyDescription: '在「登记与日结」签登记仓储计费方案后，会在这里出现。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

function AdoptionsTable() {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseAdoptions);
  const rows = answer?.kind === 'outcome' ? answer.body.adoptions : [];

  return (
    <ListPageTemplate<WarehouseAdoptionRecord>
      moduleId="warehouse-pricing"
      title="仓储计费采用"
      description="各货主与仓采用的结算账户及计价范围修订。没有适用采用行的发生项答未配置。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜货主、仓、账户、范围或依据' }}
      columns={adoptionColumns}
      rows={rows}
      rowKey={(row) => `${row.ownerAccount}/${row.warehouseCode}/${row.revision}`}
      inspector={adoptionInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的采用行"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-adoption-list',
        emptyTitle: '还没有仓储计费采用',
        emptyDescription: '在「登记与日结」签登记货主与仓的仓储计费采用后，会在这里出现。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

const kindChips = ['ALL', 'INBOUND_HANDLING', 'OUTBOUND_HANDLING', 'STORAGE', 'RETURN_HANDLING', 'VALUE_ADDED', 'PROCESSING', 'DESTRUCTION'] as const;

function OccurrencesTable() {
  const [keyword, setKeyword] = useState('');
  const [kind, setKind] = useState<(typeof kindChips)[number]>('ALL');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: kind === 'ALL' ? undefined : { kind: [kind] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listChargeOccurrences);
  const rows = answer?.kind === 'outcome' ? answer.body.occurrences : [];

  return (
    <ListPageTemplate<ChargeOccurrenceRecord>
      moduleId="warehouse-pricing"
      title="仓储作业收费发生项"
      description="仓内作业或仓租日结形成的收费发生项（最新一版）。只记作业种类与计量，不带金额。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜发生项号、货主、仓或来源事实' }}
      filters={
        <>
          {kindChips.map((item) => (
            <button
              key={item}
              type="button"
              className={chipClass(item === kind)}
              aria-pressed={item === kind}
              onClick={() => setKind(item)}
            >
              {item === 'ALL' ? '全部' : occurrenceKindLabels[item]}
            </button>
          ))}
        </>
      }
      columns={occurrenceColumns}
      rows={rows}
      rowKey={(row) => row.occurrenceId}
      inspector={occurrenceInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的收费发生项"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-occurrences',
        emptyTitle: '还没有仓储作业收费发生项',
        emptyDescription: '收货、出库确认、退货、加工销毁或仓租日结之后，发生项会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

const requestStatusChips = ['ALL', 'CONFIRMED', 'FORMED', 'DIFFERENCE', 'ZERO', 'PENDING', 'NOT_CONFIGURED', 'AWAITING_PERSON'] as const;

function ChargeRequestsTable() {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<(typeof requestStatusChips)[number]>('ALL');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWarehouseChargeRequests);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];

  return (
    <ListPageTemplate<WarehouseChargeRequestRecord>
      moduleId="warehouse-pricing"
      title="仓储销售计费请求"
      description="结算消费发生项后记下的销售计费请求与结清状态，带结算账户、币种、范围、基准时点、评价与费用引用。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜发生项、请求号、费用号、评价或账户' }}
      filters={
        <>
          {requestStatusChips.map((item) => (
            <button
              key={item}
              type="button"
              className={chipClass(item === status)}
              aria-pressed={item === status}
              onClick={() => setStatus(item)}
            >
              {item === 'ALL' ? '全部' : chargeRequestStatusLabels[item]}
            </button>
          ))}
        </>
      }
      columns={requestColumns}
      rows={rows}
      rowKey={(row) => `${row.occurrenceId}/${row.version}`}
      inspector={requestInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的计费请求"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-requests',
        emptyTitle: '还没有仓储计费请求',
        emptyDescription: '派发消费收费发生项后，计费请求与对应的评价、费用会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/** 仓储计价与收费配置页：方案、采用、发生项、计费请求四张列表表加登记与日结面板。 */
export function WarehousePricingPage() {
  const targets = useWarehouseActionTargets(pricingActionSpecs);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="plans" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="plans">计费方案</TabsTrigger>
          <TabsTrigger value="adoptions">计费采用</TabsTrigger>
          <TabsTrigger value="occurrences">收费发生项</TabsTrigger>
          <TabsTrigger value="requests">计费请求</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">登记与日结</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="plans" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <PlansTable />
        </TabsContent>
        <TabsContent value="adoptions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <AdoptionsTable />
        </TabsContent>
        <TabsContent value="occurrences" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OccurrencesTable />
        </TabsContent>
        <TabsContent value="requests" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ChargeRequestsTable />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <WarehouseActionPanel moduleId="warehouse-pricing" targets={targets} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
