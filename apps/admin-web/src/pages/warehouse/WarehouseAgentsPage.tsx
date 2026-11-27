import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Plus, Settings } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueDefaultPageSize, cataloguePageSizes, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { getAgentOverview, listOutboundRequests, type AgentOverviewBody, type OutboundRequestRecord } from './api';
import type { ApiResult } from '../catalogue-api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant, waitedSortParam } from './list-cells';
import { OutboundDocument } from './OutboundDocument';
import { queueCount } from './outbound-queues';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsDetailModal, WmsFilterSelect, WmsKpiStrip, WmsPagerBar, WmsQueueStats, WmsSubTable, WmsTwoLine, type WmsQueueStat } from './wms-ui';

// 代理仓的单也是出库单：右边开的就是出库单那一页，这一页只管转出与回录代理仓的结果。
const outboundModule = 'warehouse-outbound';
const moduleId = 'warehouse-agents';
const info = moduleInfoById[moduleId];

const agentQueues = ['HANDED_OFF', 'FORWARDED', 'PACKED', 'SHIPPED'] as const;
/** 还在等代理仓往下做的三队：等得最久的排最前。已交接的只增不减，最新受理的在上面。 */
const waitingQueues = ['HANDED_OFF', 'FORWARDED', 'PACKED'] as const;

const queueLabel: Record<string, string> = {
  HANDED_OFF: '待转出',
  FORWARDED: '已转出',
  PACKED: '已打包',
  SHIPPED: '已交接',
};

const longestWaitFirst: WorkbenchSort = { key: 'waited', dir: -1 };
const newestFirst: WorkbenchSort = { key: 'waited', dir: 1 };
const noRows: OutboundRequestRecord[] = [];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const knownQueue = (value: string) => ((agentQueues as readonly string[]).includes(value) ? value : 'HANDED_OFF');

/**
 * 代理仓履约：出库单里走代理仓的那几队。打开就落在待转出。点一行在右边开这张出库单；
 * 行上只有这一步最常做的那一个动作，拒单、缺货在单据页里。
 */
export function WarehouseAgentsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(outboundWaveActionSpecs);

  const [keyword, setKeyword] = useTabViewState('warehouse-agents.keyword', '');
  const [storedQueue, setQueue] = useTabViewState('warehouse-agents.status', 'HANDED_OFF');
  const queue = knownQueue(storedQueue);
  const [warehouse, setWarehouse] = useTabViewState('warehouse-agents.warehouse', '');
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>('warehouse-agents.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-agents.open', null);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const [ledger, setLedger] = useState<ApiResult<AgentOverviewBody> | null>(null);
  const [ledgerSize, setLedgerSize] = useState(catalogueDefaultPageSize);
  const [connectorsPage, setConnectorsPage] = useState(1);
  const [statementsPage, setStatementsPage] = useState(1);
  const [resultsPage, setResultsPage] = useState(1);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);

  const agentCodes = useMemo(
    () => lookups.warehouses.filter((item) => item.kind === 'AGENT').map((item) => item.warehouseCode).sort(),
    [lookups.warehouses],
  );
  const agentCodeSet = useMemo(() => new Set(agentCodes), [agentCodes]);
  const ownerNames = useMemo(
    () => new Map(lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [lookups.warehouses],
  );

  // 名册到了才按代理仓筛：没到之前不把仓写成空，免得待转出那一队被问成零。
  const warehouseFilter = warehouse !== '' ? [warehouse] : agentCodes;
  const baseFilters: Record<string, string[]> = {};
  if (warehouseFilter.length > 0) baseFilters.warehouseCode = warehouseFilter;
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const sort = chosenSort ?? ((waitingQueues as readonly string[]).includes(queue) ? longestWaitFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: waitedSortParam(sort.dir),
    filters: { ...baseFilters, status: [queue] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const held = heldCataloguePage(answer, previous, (body) => body.requests.length > 0);
  const pageRows = answer?.kind === 'outcome' ? answer.body.requests : (held?.body.requests ?? noRows);
  const rows = pageRows.filter((row) => belongsOnAgentPage(row, agentCodeSet, lookups.loaded));
  const counts = useStatusCounts(listOutboundRequests, base, 'status', agentQueues, refreshes);
  const shownCounts = lookups.loaded && (agentCodes.length > 0 || warehouse !== '') ? counts : { ...counts, PACKED: null, SHIPPED: null };
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

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
  }, [ledgerSize, connectorsPage, statementsPage, resultsPage, refreshes]);

  const overview = ledger?.kind === 'outcome' ? ledger.body : null;
  const ledgerPages = overview?.pages;
  const changeLedgerSize = (next: number) => {
    setLedgerSize(next);
    setConnectorsPage(1);
    setStatementsPage(1);
    setResultsPage(1);
  };

  const now = new Date();
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };

  const columns: WorkbenchColumn<OutboundRequestRecord>[] = [
    {
      key: 'id',
      header: '出库单',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={
            <DocumentLink moduleId={outboundModule} parts={[row.requestId]}>
              {row.documentNumber ?? row.requestId}
            </DocumentLink>
          }
          secondary={`${labelOf(row.origin)} · ${shortInstant(row.receivedAt, now)}`}
          title={`受理于 ${formatInstant(row.receivedAt)}`}
        />
      ),
    },
    {
      key: 'status',
      header: '状态',
      width: fitColumnWidth,
      render: (row) => <WmsBadge code={row.status} label={queueLabel[row.status]} />,
    },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 860, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '代理仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'recipient',
      header: '收件人',
      render: (row) => <WmsTwoLine fill primary={row.recipient} secondary={row.address} title={`${row.recipient}\n${row.address}`} />,
    },
    {
      key: 'age',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      sortKey: 'waited',
      render: (row) =>
        row.status === 'SHIPPED' ? (
          <span className="text-idpxyz-textMuted">—</span>
        ) : (
          <span className="whitespace-nowrap tabular-nums">{elapsedLabel(row.receivedAt, now) || '—'}</span>
        ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => <AgentStep row={row} canRecord={targetMap.has('agent-result')} onAct={openAction} />,
    },
  ];

  const queueStats: WmsQueueStat[] = agentQueues.map((id) => ({
    id,
    label: queueLabel[id],
    count: queueCount(shownCounts, id),
    active: queue === id,
    attention: id === 'HANDED_OFF',
    onClick: () => setQueue(id),
  }));

  const narrowed = settled.trim() !== '' || warehouse !== '';
  const clearFilters = () => {
    setKeyword('');
    setWarehouse('');
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
        emptyTitle: '这一队还没有代理仓的单',
        emptyDescription: '向类型为「代理仓」的仓库下出库单后，单会进待转出。',
        narrowed: narrowed || queue !== 'HANDED_OFF' || pastFirstPage(pagination),
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<OutboundRequestRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边看这张单、做代理仓这一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          <>
            <Button type="button" variant="outline" onClick={() => setLedgerOpen(true)}>
              {overview
                ? `台账 ${ledgerPages?.connectors.total ?? overview.connectors.length}/${ledgerPages?.statements.total ?? overview.statements.length}/${ledgerPages?.results.total ?? overview.results.length}`
                : '台账'}
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
              <Settings className="mr-1 h-3.5 w-3.5" />
              连接器
            </Button>
            {targetMap.has('agent-statement') ? (
              <Button type="button" className={primaryActionClass} onClick={() => openAction('agent-statement')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                登记库存声明
              </Button>
            ) : null}
          </>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜单号、代理仓或货主' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="代理仓"
              value={warehouse}
              onChange={setWarehouse}
              options={lookups.warehouses
                .filter((item) => item.kind === 'AGENT')
                .map((item) => ({ value: item.warehouseCode, label: item.name ? `${item.name} · ${item.warehouseCode}` : item.warehouseCode }))}
            />
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
        rowKey={(row) => row.requestId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => setChosenSort(toggleWorkbenchSort(sort, key))}
        noMatchText="这一队里没有对得上的单"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <OutboundDocument requestId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <AgentLedger
        open={ledgerOpen}
        overview={overview}
        ledgerPages={ledgerPages}
        ledgerSize={ledgerSize}
        connectorsPage={connectorsPage}
        statementsPage={statementsPage}
        resultsPage={resultsPage}
        onClose={() => setLedgerOpen(false)}
        onSize={changeLedgerSize}
        onConnectorsPage={setConnectorsPage}
        onStatementsPage={setStatementsPage}
        onResultsPage={setResultsPage}
      />
      <ActionDrawer
        moduleId={moduleId}
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={refresh}
      />
    </>
  );
}

/** 待转出、已转出只会发生在代理仓。打包和交接自营仓也有，名册到了才按仓留下。 */
function belongsOnAgentPage(row: OutboundRequestRecord, agentCodes: ReadonlySet<string>, loaded: boolean): boolean {
  if (row.status === 'HANDED_OFF' || row.status === 'FORWARDED') return true;
  if (!loaded) return false;
  return agentCodes.has(row.warehouseCode);
}

function AgentStep({
  row,
  canRecord,
  onAct,
}: {
  row: OutboundRequestRecord;
  canRecord: boolean;
  onAct: (id: string, prefill?: GuidedFormValues) => void;
}) {
  const step = agentStep(row.status);
  if (step === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!canRecord) return <span className="whitespace-nowrap text-idpxyz-textMuted">{step.label}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        onAct('agent-result', { requestId: row.requestId, kind: step.kind });
      }}
    >
      {step.label}
    </Button>
  );
}

/** 一行只摆最常做的那一步。拒单、缺货仍在单据页里，不跟这一步抢。 */
function agentStep(status: string): { label: string; kind: string } | null {
  switch (status) {
    case 'HANDED_OFF':
      return { label: '转出', kind: 'FORWARDED' };
    case 'FORWARDED':
      return { label: '登记打包', kind: 'PACKED' };
    case 'PACKED':
      return { label: '登记交接', kind: 'HANDED_OVER' };
    default:
      return null;
  }
}

function AgentLedger({
  open,
  overview,
  ledgerPages,
  ledgerSize,
  connectorsPage,
  statementsPage,
  resultsPage,
  onClose,
  onSize,
  onConnectorsPage,
  onStatementsPage,
  onResultsPage,
}: {
  open: boolean;
  overview: AgentOverviewBody | null;
  ledgerPages: AgentOverviewBody['pages'];
  ledgerSize: number;
  connectorsPage: number;
  statementsPage: number;
  resultsPage: number;
  onClose: () => void;
  onSize: (next: number) => void;
  onConnectorsPage: (next: number) => void;
  onStatementsPage: (next: number) => void;
  onResultsPage: (next: number) => void;
}) {
  return (
    <WmsDetailModal open={open} title="代理仓台账" subtitle="连接器、库存声明和履约回传" onClose={onClose}>
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
          onPrevious={() => onConnectorsPage(connectorsPage - 1)}
          onNext={() => onConnectorsPage(connectorsPage + 1)}
          pageSize={{ value: ledgerSize, options: cataloguePageSizes, onChange: onSize }}
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
          onPrevious={() => onStatementsPage(statementsPage - 1)}
          onNext={() => onStatementsPage(statementsPage + 1)}
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
          onPrevious={() => onResultsPage(resultsPage - 1)}
          onNext={() => onResultsPage(resultsPage + 1)}
        />
      ) : null}
    </WmsDetailModal>
  );
}
