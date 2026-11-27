import { useMemo, useState, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listFreezes, type StockFreezeRecord } from './api';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { DocumentLink } from './document-page';
import { freezeIsCarton, freezeNextStep, freezeQuantityLabel, freezeWaitingSince } from './freeze-flow';
import { FreezeDocument } from './FreezeDocument';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { freezeKind, processingModule as moduleId } from './processing-address';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const frozenKey = 'frozenAt';
const frozen = 'FROZEN';
const allQueues = 'ALL';
const noRows: StockFreezeRecord[] = [];

const queueStatuses = [frozen] as const;
const settledStatuses = ['UNFROZEN'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];
const targetWords = ['GOODS', 'CARTON'] as const;

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 读口按冻结时刻排。冻结中先冻上的在上面；看全部或已解冻的，最新冻结的在上面。人点过表头就照他的。 */
function frozenSort(stored: string | null, inQueue: boolean): WorkbenchSort {
  if (stored === frozenKey) return { key: frozenKey, dir: 1 };
  if (stored === `-${frozenKey}`) return { key: frozenKey, dir: -1 };
  return { key: frozenKey, dir: inQueue ? 1 : -1 };
}

/**
 * 冻结与解冻。打开落在冻结中。冻结中的数量在标题行上。
 * 点一行在右边开这张冻结单，冻结号仍进整页。行上冻结中的只留解冻。
 */
export function FreezeList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.freeze.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : frozen;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  // 筛选态里的「种类」在这一页就是冻结的对象，读口的筛选维叫 target。
  if (filters.kind) baseFilters.target = [filters.kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const sort = frozenSort(filters.sort, status === frozen);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? frozenKey : `-${frozenKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listFreezes);
  const held = heldCataloguePage(answer, previous, (body) => body.freezes.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.freezes : (held?.body.freezes ?? noRows);
  const counts = useStatusCounts(listFreezes, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };
  const now = new Date();

  const columns: WorkbenchColumn<StockFreezeRecord>[] = [
    {
      key: 'id',
      header: '冻结号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[freezeKind, row.freezeId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 900, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1080, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'subject',
      header: '冻的是什么',
      render: (row) =>
        freezeIsCarton(row) ? (
          <WmsTwoLine fill primary="整箱" secondary={row.cartonNumber || '—'} code title={row.cartonNumber || ''} />
        ) : (
          <WmsTwoLine
            fill
            primary={<span className="font-mono">{row.locationCode || '—'}</span>}
            secondary={row.goodsCode || '—'}
            code
            title={`${row.locationCode || ''}\n${row.goodsCode || ''}`}
          />
        ),
    },
    {
      key: 'qty',
      header: nowrap('数量'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{freezeQuantityLabel(row)}</span>,
    },
    {
      key: 'reason',
      header: '原因',
      minListWidth: 1240,
      render: (row) => (row.reason ? <WmsTwoLine fill primary={row.reason} title={row.reason} /> : <span className="text-idpxyz-textMuted">没写原因</span>),
    },
    {
      key: 'waiting',
      header: nowrap('已冻'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{elapsedLabel(freezeWaitingSince(row), now) || '—'}</span>,
    },
    {
      key: 'frozen',
      header: nowrap('冻结'),
      right: true,
      width: fitColumnWidth,
      sortKey: frozenKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.frozenAt)}>
          {shortInstant(row.frozenAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <FreezeStep row={row} drawer={drawer} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : frozen);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: labelOf(code),
    count: counts[code] ?? null,
    active: status === code,
    attention: true,
    onClick: () => chooseStatus(status === code ? allQueues : code),
  }));
  const narrowed =
    settled.trim() !== '' ||
    filters.owner !== '' ||
    filters.warehouse !== '' ||
    filters.kind !== '' ||
    (status !== frozen && status !== allQueues) ||
    pastFirstPage(pagination);
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setKind('');
    filters.setStatus(frozen);
    filters.setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-freezes',
        emptyTitle: status === frozen ? '当前没有冻结中的库存' : '还没有冻结记录',
        emptyDescription: '可在此冻结，或在实时库存对单个格子点冻结。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;
  const canFreeze = drawer.has('stock-freeze');
  const canRestore = drawer.has('condition-restore');

  return (
    <>
      <WorkbenchPageTemplate<StockFreezeRecord>
        title="冻结与解冻"
        viewStateKey={`${moduleId}.freeze`}
        subtitle="点一行在右边看这张冻结单。冻着的不能被出库分配"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          canFreeze || canRestore ? (
            <>
              {canRestore ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('condition-restore')}>
                  次品转良品
                </Button>
              ) : null}
              {canFreeze ? (
                <Button type="button" className={primaryActionClass} onClick={() => drawer.open('stock-freeze')}>
                  <Plus className="mr-1 h-3.5 w-3.5" />
                  冻结库存
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜冻结号、货主、仓、库位、商品、箱号或原因' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: labelOf(code) }))}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            <WmsFilterSelect label="对象" value={filters.kind} onChange={filters.setKind} options={targetWords.map((word) => ({ value: word, label: labelOf(word) }))} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.freezeId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? frozenKey : `-${frozenKey}`);
        }}
        noMatchText="当前筛选下没有冻结记录"
        onClearFilters={clearFilters}
        renderDetail={(key) => <FreezeDocument freezeId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function FreezeStep({ row, drawer }: { row: StockFreezeRecord; drawer: ActionDrawerControl }) {
  const next = freezeNextStep(row.status);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('stock-unfreeze')) return <span className="whitespace-nowrap text-idpxyz-textMuted">等解冻</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open('stock-unfreeze', { freezeId: row.freezeId });
      }}
    >
      {next.label}
    </Button>
  );
}
