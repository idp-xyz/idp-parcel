import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listOutboundRequests,
  listWaves,
  type OutboundRequestRecord,
  type WaveRecord,
} from './api';
import { outboundWaveActionSpecs } from './actions';
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';
import type { GuidedFormSeed, GuidedFormValues } from './guided-forms';

const info = moduleInfoById['warehouse-outbound'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const dispatchLabels: Record<string, string> = {
  PRODUCT_NETWORK: '本产品网络',
  OWNER_LABEL: '自备面单',
  PICKUP: '自提',
};

const outboundStatusLabels: Record<string, string> = {
  RESERVED: '已预占',
  PARTIALLY_RESERVED: '部分预占',
  REJECTED: '已拒单',
  PENDING_POLICY: '待预占',
  HANDED_OFF: '待代理仓转出',
  FORWARDED: '已转出代理仓',
  SPLIT: '已拆单',
  ALLOCATED: '已分配',
  PICKED: '已拣货',
  CHECKED: '已复核',
  PACKED: '已打包',
  SHIPPED: '已出库确认',
  CANCELLED: '已取消',
  AWAITING_RETURN: '待回库',
  PARTIALLY_CANCELLED: '部分取消',
};

const taskStatusLabels: Record<string, string> = {
  OPEN: '待拣',
  DONE: '已拣完',
  SHORT: '短拣',
  CANCELLED: '已取消',
  LATE: '迟到拣货',
};

const outboundColumns: ListColumn<OutboundRequestRecord>[] = [
  { id: 'id', header: '请求标识', className: mono, render: (row) => row.requestId },
  { id: 'number', header: '单据号', className: mono, render: (row) => row.documentNumber ?? '—' },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'dispatch', header: '发运方式', render: (row) => dispatchLabels[row.dispatch] ?? row.dispatch },
  { id: 'status', header: '状态', render: (row) => outboundStatusLabels[row.status] ?? row.status },
  { id: 'received', header: '受理时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

export function buildOutboundInspector(
  row: OutboundRequestRecord,
  available: ReadonlySet<string>,
  onPrefill: (targetId: string, values: GuidedFormValues) => void,
): InspectorContent {
  // 概要严格不超过 8 格（INSPECTOR_SUMMARY_LIMIT）。
  const lineSummary =
    row.lines.length === 0
      ? '—'
      : row.lines
          .slice(0, 3)
          .map((line) => `${line.goodsCode}×${line.quantity}${line.unit}`)
          .join('；') + (row.lines.length > 3 ? ` 等 ${row.lines.length} 行` : '');
  const fields: InspectorField[] = [
    { label: '单据号', value: row.documentNumber ?? row.numberingAbsence ?? '—', mono: true },
    { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
    {
      label: '发运与产品',
      value: `${dispatchLabels[row.dispatch] ?? row.dispatch}${row.service ? ` · ${row.service}` : ''}${row.platformStocking ? ' · 平台备货' : ''}`,
    },
    { label: '收件人与地址', value: `${row.recipient}（${row.address}）` },
    { label: '预占 / 需求', value: `${row.reservedQuantity} / ${row.wantedQuantity}`, mono: true },
    { label: '商品明细', value: lineSummary },
  ];
  if (row.prefetchShipmentId) {
    fields.push({
      label: '预取委托',
      value: row.prefetchDeclaredParcelId
        ? `${row.prefetchShipmentId}（包裹 ${row.prefetchDeclaredParcelId}）`
        : row.prefetchShipmentId,
      mono: true,
    });
  }
  if (row.parentRequestId) {
    fields.push({ label: '拆分原请求', value: row.parentRequestId, mono: true });
  }

  const actions: InspectorAction[] = [];
  if (row.status === 'RESERVED' && available.has('wave-release')) {
    actions.push({
      label: '放波次分配',
      onRun: () => onPrefill('wave-release', { warehouse: row.warehouseCode, requests: row.requestId }),
    });
  }
  if ((row.status === 'RESERVED' || row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY') && available.has('outbound-split')) {
    actions.push({
      label: '拆单',
      onRun: () => onPrefill('outbound-split', { requestId: row.requestId }),
    });
  }
  if (row.status === 'HANDED_OFF' && available.has('agent-result')) {
    actions.push({
      label: '转出给代理仓',
      onRun: () => onPrefill('agent-result', { requestId: row.requestId, kind: 'FORWARDED' }),
    });
  }
  if (row.status === 'FORWARDED' && available.has('agent-result')) {
    actions.push({
      label: '登记代理仓结果',
      onRun: () => onPrefill('agent-result', { requestId: row.requestId }),
    });
  }
  if (row.dispatch === 'OWNER_LABEL' && available.has('owner-label')) {
    actions.push({
      label: '登记货主面单',
      onRun: () => onPrefill('owner-label', { requestId: row.requestId, parcelSequence: '1' }),
    });
  }
  if (row.status !== 'SHIPPED' && row.status !== 'CANCELLED' && row.status !== 'REJECTED' && row.status !== 'SPLIT' && available.has('outbound-cutoff')) {
    actions.push({
      label: '截单取消',
      onRun: () => onPrefill('outbound-cutoff', { requestId: row.requestId }),
    });
  }

  return {
    title: row.requestId,
    subtitle: outboundStatusLabels[row.status] ?? row.status,
    sections: [
      { kind: 'summary', fields },
      { kind: 'status', items: [{ label: '状态', word: outboundStatusLabels[row.status] ?? row.status }] },
      ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
      {
        kind: 'audit',
        fields: [
          { label: '来源', value: `${row.origin} · ${row.requestKey}`, mono: true },
          { label: '受理时刻', value: formatInstant(row.receivedAt), mono: true },
        ],
      },
    ],
  };
}

const waveColumns: ListColumn<WaveRecord>[] = [
  { id: 'wave', header: '波次号', className: mono, render: (row) => row.waveId },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'prefetch', header: '预取', render: (row) => (row.prefetch ? '是' : '否') },
  { id: 'requests', header: '请求数', className: mono, render: (row) => String(row.requests.length) },
  { id: 'tasks', header: '拣货任务', className: mono, render: (row) => String(row.tasks.length) },
  { id: 'released', header: '放波时刻', className: mono, render: (row) => formatInstant(row.releasedAt) },
];

export function buildWaveInspector(
  row: WaveRecord,
  available: ReadonlySet<string>,
  onPrefill: (targetId: string, values: GuidedFormValues) => void,
): InspectorContent {
  // 概要严格不超过 8 格（INSPECTOR_SUMMARY_LIMIT）：头部 3 格 + 最多 5 格任务（超出汇总）。
  const fields: InspectorField[] = [
    { label: '仓', value: row.warehouseCode, mono: true },
    { label: '预取估算委托', value: row.prefetch ? '是' : '否' },
    { label: '包含请求', value: row.requests.join('、') || '—', mono: true },
  ];
  const visible = row.tasks.length > 5 ? row.tasks.slice(0, 4) : row.tasks;
  visible.forEach((task) => {
    fields.push({
      label: task.taskId,
      value: `${task.requestId} · ${task.locationCode} · ${task.goodsCode} ${task.picked}/${task.quantity}${task.unit}（${taskStatusLabels[task.status] ?? task.status}）`,
    });
  });
  if (row.tasks.length > 5) {
    fields.push({ label: '其余任务', value: `另有 ${row.tasks.length - 4} 条拣货任务` });
  }

  const actions: InspectorAction[] = available.has('wave-release')
    ? [{ label: '同仓再放一波次', onRun: () => onPrefill('wave-release', { warehouse: row.warehouseCode }) }]
    : [];

  return {
    title: row.waveId,
    subtitle: `${row.warehouseCode} · ${row.tasks.length} 条拣货任务`,
    sections: [
      { kind: 'summary', fields },
      ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
      {
        kind: 'audit',
        fields: [
          { label: '操作者', value: row.operator, mono: true },
          { label: '放波时刻', value: formatInstant(row.releasedAt), mono: true },
        ],
      },
    ],
  };
}

const statusChips = ['ALL', 'RESERVED', 'ALLOCATED', 'PICKED', 'PACKED', 'SHIPPED', 'HANDED_OFF', 'CANCELLED'] as const;

function OutboundTable({
  available,
  onPrefill,
}: {
  available: ReadonlySet<string>;
  onPrefill: (targetId: string, values: GuidedFormValues) => void;
}) {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<(typeof statusChips)[number]>('ALL');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];

  return (
    <ListPageTemplate<OutboundRequestRecord>
      moduleId="warehouse-outbound"
      title={info.title}
      description="已受理的出库请求列表。单击行可从右栏直接带出请求号去放波次、拆单、截单、登面单或代理仓转出。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜请求标识、单据号、货主、仓或收件人' }}
      filters={
        <>
          {statusChips.map((item) => (
            <button
              key={item}
              type="button"
              className={chipClass(item === status)}
              aria-pressed={item === status}
              onClick={() => setStatus(item)}
            >
              {item === 'ALL' ? '全部' : outboundStatusLabels[item]}
            </button>
          ))}
        </>
      }
      columns={outboundColumns}
      rows={rows}
      rowKey={(row) => row.requestId}
      inspector={(row) => buildOutboundInspector(row, available, onPrefill)}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的出库请求"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
        emptyTitle: '还没有出库请求',
        emptyDescription: '货主提交或在「处理」签代录出库请求后，会在这里出现。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

function WavesTable({
  available,
  onPrefill,
}: {
  available: ReadonlySet<string>;
  onPrefill: (targetId: string, values: GuidedFormValues) => void;
}) {
  const [keyword, setKeyword] = useState('');
  const [prefetch, setPrefetch] = useState<'all' | 'true' | 'false'>('all');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: prefetch === 'all' ? undefined : { prefetch: [prefetch] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWaves);
  const rows = answer?.kind === 'outcome' ? answer.body.waves : [];

  return (
    <ListPageTemplate<WaveRecord>
      moduleId="warehouse-outbound"
      title="波次与拣货任务"
      description="已放出的波次及其分配的拣货任务。放波次前须先采用参考分配策略。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜波次号、仓或操作者' }}
      filters={
        <>
          {([
            ['all', '全部'],
            ['true', '标了预取'],
            ['false', '未标预取'],
          ] as const).map(([value, label]) => (
            <button
              key={value}
              type="button"
              className={chipClass(value === prefetch)}
              aria-pressed={value === prefetch}
              onClick={() => setPrefetch(value)}
            >
              {label}
            </button>
          ))}
        </>
      }
      columns={waveColumns}
      rows={rows}
      rowKey={(row) => row.waveId}
      inspector={(row) => buildWaveInspector(row, available, onPrefill)}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的波次"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-wave-list',
        emptyTitle: '还没有放出的波次',
        emptyDescription: '采用分配策略并在「处理」签放波次后，波次与拣货任务会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/** 出库、波次与代理仓页：出库请求列表、波次列表与引导式运营决定面板。 */
export function WarehouseOutboundPage() {
  const targets = useWarehouseActionTargets(outboundWaveActionSpecs);
  const available = new Set(targets.map((target) => target.id));
  const [tab, setTab] = useState('requests');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);

  const openPrefill = (targetId: string, values: GuidedFormValues) => {
    setSeed((current) => ({ targetId, values, nonce: (current?.nonce ?? 0) + 1 }));
    setTab('act');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="requests">出库请求</TabsTrigger>
          <TabsTrigger value="waves">波次与拣货</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">处理</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="requests" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OutboundTable available={available} onPrefill={openPrefill} />
        </TabsContent>
        <TabsContent value="waves" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <WavesTable available={available} onPrefill={openPrefill} />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <WarehouseActionPanel moduleId="warehouse-outbound" targets={targets} seed={seed} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
