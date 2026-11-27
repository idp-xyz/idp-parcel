import { useEffect, useState } from 'react';
import { Plus, Settings } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueDefaultPageSize, cataloguePageSizes, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { getAgentOverview, listOutboundRequests, type AgentOverviewBody, type OutboundRequestRecord } from './api';
import type { ApiResult } from '../catalogue-api';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { DocumentLink } from './document-page';
import { openDocument } from './use-document';
import { labelOf, WmsBadge, WmsDetailModal, WmsKpiStrip, WmsPagerBar, WmsSubTable } from './wms-ui';

// 代理仓的单也是出库单：明细在出库单页上看，这一页只管转出与回录代理仓的结果。
const outboundModule = 'warehouse-outbound';

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
  {
    id: 'id',
    header: '出库单号',
    className: 'whitespace-nowrap',
    render: (row) => (
      <DocumentLink moduleId={outboundModule} parts={[row.requestId]}>
        {row.documentNumber ?? row.requestId}
      </DocumentLink>
    ),
  },
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
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const [ledger, setLedger] = useState<ApiResult<AgentOverviewBody> | null>(null);
  const [ledgerSize, setLedgerSize] = useState(catalogueDefaultPageSize);
  const [connectorsPage, setConnectorsPage] = useState(1);
  const [statementsPage, setStatementsPage] = useState(1);
  const [resultsPage, setResultsPage] = useState(1);
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
    void getAgentOverview({ pageSize: ledgerSize, connectorsPage, statementsPage, resultsPage }).then((next) => {
      if (!cancelled) setLedger(next);
    });
    return () => {
      cancelled = true;
    };
  }, [ledgerSize, connectorsPage, statementsPage, resultsPage]);

  const overview = ledger?.kind === 'outcome' ? ledger.body : null;
  const ledgerPages = overview?.pages;
  const changeLedgerSize = (next: number) => {
    setLedgerSize(next);
    setConnectorsPage(1);
    setStatementsPage(1);
    setResultsPage(1);
  };

  const inspectorOf = (row: OutboundRequestRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '打开出库单', onRun: () => openDocument(outboundModule, [row.requestId]) }];
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
                ? `连接器 ${ledgerPages?.connectors.total ?? overview.connectors.length} · 声明 ${ledgerPages?.statements.total ?? overview.statements.length} · 回传 ${ledgerPages?.results.total ?? overview.results.length}`
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
        onRowOpen={(row) => openDocument(outboundModule, [row.requestId])}
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
            { label: '连接器', value: ledgerPages?.connectors.total ?? overview?.connectors.length ?? '—', tone: (ledgerPages?.connectors.total ?? overview?.connectors.length ?? 0) > 0 ? 'success' : 'warning' },
            { label: '库存声明', value: ledgerPages?.statements.total ?? overview?.statements.length ?? '—' },
            { label: '履约回传', value: ledgerPages?.results.total ?? overview?.results.length ?? '—' },
          ]}
        />
        <WmsSubTable
          title="连接器"
          count={ledgerPages?.connectors.total ?? overview?.connectors.length ?? 0}
          emptyText="还没有代理仓连接器。到仓配规则设置里登记。"
          rows={overview?.connectors ?? []}
          rowKey={(row) => row.warehouseCode}
          columns={[
            { id: 'wh', header: '代理仓', mono: true, render: (row) => row.warehouseCode },
            { id: 'at', header: '登记时刻', mono: true, render: (row) => formatInstant(row.registeredAt) },
          ]}
        />
        {ledgerPages ? (
          <WmsPagerBar
            summary={`第 ${ledgerPages.connectors.page} / ${Math.max(1, Math.ceil(ledgerPages.connectors.total / ledgerPages.connectors.pageSize))} 页 · 共 ${ledgerPages.connectors.total} 条`}
            canPrevious={connectorsPage > 1}
            canNext={connectorsPage * ledgerSize < ledgerPages.connectors.total}
            onPrevious={() => setConnectorsPage((current) => current - 1)}
            onNext={() => setConnectorsPage((current) => current + 1)}
            pageSize={{ value: ledgerSize, options: cataloguePageSizes, onChange: changeLedgerSize }}
          />
        ) : null}
        <WmsSubTable
          title="库存声明"
          count={ledgerPages?.statements.total ?? overview?.statements.length ?? 0}
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
        {ledgerPages ? (
          <WmsPagerBar
            summary={`第 ${ledgerPages.statements.page} / ${Math.max(1, Math.ceil(ledgerPages.statements.total / ledgerPages.statements.pageSize))} 页 · 共 ${ledgerPages.statements.total} 条`}
            canPrevious={statementsPage > 1}
            canNext={statementsPage * ledgerSize < ledgerPages.statements.total}
            onPrevious={() => setStatementsPage((current) => current - 1)}
            onNext={() => setStatementsPage((current) => current + 1)}
          />
        ) : null}
        <WmsSubTable
          title="履约回传"
          count={ledgerPages?.results.total ?? overview?.results.length ?? 0}
          emptyText="还没有代理仓回传打包、交接或拒单。"
          rows={overview?.results ?? []}
          rowKey={(row) => row.resultId}
          columns={[
            { id: 'req', header: '出库单', render: (row) => <DocumentLink moduleId={outboundModule} parts={[row.requestId]} /> },
            { id: 'kind', header: '结果', render: (row) => labelOf(row.kind) },
            { id: 'wh', header: '仓', mono: true, render: (row) => row.warehouseCode },
            { id: 'at', header: '回传时刻', mono: true, render: (row) => formatInstant(row.reportedAt) },
          ]}
        />
        {ledgerPages ? (
          <WmsPagerBar
            summary={`第 ${ledgerPages.results.page} / ${Math.max(1, Math.ceil(ledgerPages.results.total / ledgerPages.results.pageSize))} 页 · 共 ${ledgerPages.results.total} 条`}
            canPrevious={resultsPage > 1}
            canNext={resultsPage * ledgerSize < ledgerPages.results.total}
            onPrevious={() => setResultsPage((current) => current - 1)}
            onNext={() => setResultsPage((current) => current + 1)}
          />
        ) : null}
      </WmsDetailModal>
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
