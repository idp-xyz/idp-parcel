import { useEffect, useState } from 'react';
import { Plus, Settings } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { getAgentOverview, listOutboundRequests, type AgentOverviewBody, type OutboundRequestRecord } from './api';
import type { ApiResult } from '../catalogue-api';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { OutboundDetailModal } from './document-modals';
import { labelOf, WmsBadge, WmsDetailModal, WmsKpiStrip, WmsSubTable } from './wms-ui';

const info = moduleInfoById['warehouse-agents'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const statusLabels: Record<string, string> = {
  HANDED_OFF: '待转出代理仓',
  FORWARDED: '已转出代理仓',
  PACKED: '代理仓已打包',
  SHIPPED: '代理仓已交接',
};

const agentStatusFilters = ['HANDED_OFF', 'FORWARDED', 'PACKED', 'SHIPPED'] as const;

const columns: ListColumn<OutboundRequestRecord>[] = [
  { id: 'id', header: '出库单号', className: mono, render: (row) => row.documentNumber ?? row.requestId },
  { id: 'warehouse', header: '代理仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'status', header: '履约进展', render: (row) => <WmsBadge code={row.status} label={statusLabels[row.status]} /> },
  { id: 'recipient', header: '收件人', render: (row) => row.recipient },
  { id: 'received', header: '受理时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

/** 代理仓履约页：把代理仓外发、回执（打包/交接/拒单）与库存声明独立成清晰队列，不再和自营仓波次混在一页。 */
export function WarehouseAgentsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(outboundWaveActionSpecs);

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('HANDED_OFF');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const [ledger, setLedger] = useState<ApiResult<AgentOverviewBody> | null>(null);
  const settled = useSettledKeyword(keyword);

  const conditions: CatalogueConditions = {
    q: settled,
    filters: { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const agentWarehouseCodes = new Set(lookups.warehouses.filter((w) => w.kind === 'AGENT').map((w) => w.warehouseCode));
  const rows =
    answer?.kind === 'outcome'
      ? answer.body.requests.filter(
          (r) => r.status === 'HANDED_OFF' || r.status === 'FORWARDED' || agentWarehouseCodes.has(r.warehouseCode),
        )
      : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  useEffect(() => {
    let cancelled = false;
    void getAgentOverview().then((next) => {
      if (!cancelled) setLedger(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const overview = ledger?.kind === 'outcome' ? ledger.body : null;

  const inspectorOf = (row: OutboundRequestRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看出库明细', onRun: () => setDetailId(row.requestId) }];
    if (row.status === 'HANDED_OFF' && targetMap.has('agent-result')) {
      actions.push({ label: '转出给代理仓', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'FORWARDED' }) });
    }
    if (row.status === 'FORWARDED' && targetMap.has('agent-result')) {
      actions.push(
        { label: '登记代理仓打包', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'PACKED' }) },
        { label: '登记代理仓交接出库', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'HANDED_OVER' }) },
        { label: '登记代理仓拒单', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'REJECTED' }) },
        { label: '登记代理仓缺货', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'SHORT' }) },
      );
    }
    if (row.status === 'PACKED' && targetMap.has('agent-result')) {
      actions.push({ label: '登记代理仓交接出库', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'HANDED_OVER' }) });
    }

    return {
      title: row.documentNumber ?? row.requestId,
      subtitle: `${row.warehouseCode} · ${statusLabels[row.status] ?? row.status}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '出库请求号', value: row.requestId, mono: true },
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '收件人', value: row.recipient },
            { label: '收件地址', value: row.address },
            { label: '需求数量', value: `${row.wantedQuantity}（${row.lines.length} 行）`, mono: true },
          ],
        },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '受理时刻', value: formatInstant(row.receivedAt), mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<OutboundRequestRecord>
        moduleId="warehouse-agents"
        title={info.title}
        description="代理仓不走自营波次与拣货车：在此转出指令、回录打包/交接/拒单，以及登记代理仓库存声明。代理仓连接器在「仓配规则设置」管理。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setLedgerOpen(true)}
            >
              {overview
                ? `连接器 ${overview.connectors.length} · 声明 ${overview.statements.length} · 回传 ${overview.results.length}`
                : '连接器、声明与回传'}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                window.location.hash = '#/warehouse-settings';
              }}
            >
              <Settings className="h-3.5 w-3.5 mr-1" />
              配置代理仓连接器
            </Button>
            {targetMap.has('agent-statement') ? (
              <Button type="button" className={primaryActionClass} onClick={() => openAction('agent-statement')}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                登记代理仓库存声明
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜单号、代理仓或货主' }}
        filters={
          <>
            {agentStatusFilters.map((item) => (
              <button
                key={item}
                type="button"
                className={chipClass(item === status)}
                aria-pressed={item === status}
                onClick={() => setStatus(item)}
              >
                {statusLabels[item]}
              </button>
            ))}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.requestId}
        inspector={inspectorOf}
        onRowOpen={(row) => setDetailId(row.requestId)}
        pagination={pagination}
        emptyRowsNote="当前状态下没有代理仓履约订单"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
          emptyTitle: '当前状态没有代理仓订单',
          emptyDescription: '向类型为「代理仓」的仓库下出库单后，订单会直接进入「待转出代理仓」队列。',
          narrowed: settled.trim() !== '',
        })}
      />
      <WmsDetailModal
        open={ledgerOpen}
        title="代理仓台账"
        subtitle="连接器、库存声明和履约回传"
        onClose={() => setLedgerOpen(false)}
      >
        <WmsKpiStrip
          items={[
            { label: '连接器', value: overview?.connectors.length ?? '—', tone: (overview?.connectors.length ?? 0) > 0 ? 'success' : 'warning' },
            { label: '库存声明', value: overview?.statements.length ?? '—' },
            { label: '履约回传', value: overview?.results.length ?? '—' },
          ]}
        />
        <WmsSubTable
          title="连接器"
          count={overview?.connectors.length ?? 0}
          emptyText="还没有代理仓连接器。到仓配规则设置里登记。"
          rows={overview?.connectors ?? []}
          rowKey={(row) => row.warehouseCode}
          columns={[
            { id: 'wh', header: '代理仓', mono: true, render: (row) => row.warehouseCode },
            { id: 'at', header: '登记时刻', mono: true, render: (row) => formatInstant(row.registeredAt) },
          ]}
        />
        <WmsSubTable
          title="库存声明"
          count={overview?.statements.length ?? 0}
          emptyText="还没有代理仓库存声明。"
          rows={overview?.statements ?? []}
          rowKey={(row) => row.sourceId}
          columns={[
            { id: 'wh', header: '仓', mono: true, render: (row) => row.warehouseCode },
            { id: 'owner', header: '货主', mono: true, render: (row) => row.ownerAccount },
            { id: 'goods', header: '商品', mono: true, render: (row) => row.goodsCode },
            { id: 'qty', header: '数量', align: 'right', mono: true, render: (row) => `${row.quantity} ${row.unit}` },
            { id: 'fresh', header: '新鲜度', render: (row) => <WmsBadge code={row.freshness} /> },
            { id: 'until', header: '有效至', mono: true, render: (row) => formatInstant(row.validUntil) },
          ]}
        />
        <WmsSubTable
          title="履约回传"
          count={overview?.results.length ?? 0}
          emptyText="还没有代理仓回传打包、交接或拒单。"
          rows={overview?.results ?? []}
          rowKey={(row) => row.resultId}
          columns={[
            { id: 'req', header: '出库单', mono: true, render: (row) => row.requestId },
            { id: 'kind', header: '结果', render: (row) => labelOf(row.kind) },
            { id: 'wh', header: '仓', mono: true, render: (row) => row.warehouseCode },
            { id: 'at', header: '回传时刻', mono: true, render: (row) => formatInstant(row.reportedAt) },
          ]}
        />
      </WmsDetailModal>
      <OutboundDetailModal
        requestId={detailId}
        canWave={false}
        canSplit={targetMap.has('outbound-split')}
        canCutoff={targetMap.has('outbound-cutoff')}
        canLabel={targetMap.has('owner-label')}
        onClose={() => setDetailId(null)}
        onAction={(id, prefill) => {
          setDetailId(null);
          openAction(id, prefill);
        }}
      />
      <ActionDrawer
        moduleId="warehouse-agents"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </>
  );
}
