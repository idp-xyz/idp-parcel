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
import { listCounts, type StockCountRecord } from './api';
import { codesPreview, countNextStep, countStatusLabel, countWaitingSince } from './count-flow';
import { countKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { CountDocument } from './CountDocument';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const createdKey = 'createdAt';
const allQueues = 'ALL';
const noRows: StockCountRecord[] = [];

// 签的先后照一份盘点往下走的先后。待开始与盘点中是要人动手的两队，都数。已盘点与已作废只增不减，不数。
const queueStatuses = ['PENDING', 'COUNTING'] as const;
const settledStatuses = ['COUNTED', 'VOID'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const goWorkstation = () => {
  window.location.hash = '#/warehouse-workstation';
};

/** 读口按创建时刻排。待开始、盘点中先创建的在上面；看全部或已了结的，最新创建的在上面。人点过表头就照他的。 */
function createdSort(stored: string | null, inQueue: boolean): WorkbenchSort {
  if (stored === createdKey) return { key: createdKey, dir: 1 };
  if (stored === `-${createdKey}`) return { key: createdKey, dir: -1 };
  return { key: createdKey, dir: inQueue ? 1 : -1 };
}

/**
 * 盘点任务。打开看全部。待开始与盘点中的数量在标题行上，点一下看那一队。
 * 点一行在右边开这张盘点单，盘点号仍进整页。行上只留此刻该做的那一步。
 * 已停留从开始或结束算，读口排的是创建时刻，所以排序落在「创建」那一列，不落在「已停留」上。
 */
export function CountList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.counts.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : allQueues;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const inQueue = (queueStatuses as readonly string[]).includes(status);
  const sort = createdSort(filters.sort, inQueue);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? createdKey : `-${createdKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listCounts);
  const held = heldCataloguePage(answer, previous, (body) => body.counts.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.counts : (held?.body.counts ?? noRows);
  const counts = useStatusCounts(listCounts, base, 'status', queueStatuses, refreshes);
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

  const columns: WorkbenchColumn<StockCountRecord>[] = [
    {
      key: 'id',
      header: '盘点单',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[countKind, row.countId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={countStatusLabel(row.status)} /> },
    {
      key: 'owner',
      header: '货主',
      width: fitColumnWidth,
      minListWidth: 860,
      render: (row) => (row.ownerAccount ? named(ownerNames, row.ownerAccount) : nowrap('不限货主')),
    },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'scope',
      header: '范围',
      render: (row) => (
        <WmsTwoLine
          fill
          primary={`${row.locations.length} 个库位`}
          secondary={row.goods.length > 0 ? `${row.goods.length} 种商品` : '全部商品'}
          title={`${codesPreview(row.locations, 8)}${row.goods.length > 0 ? `\n${codesPreview(row.goods, 8)}` : ''}`}
        />
      ),
    },
    {
      key: 'entries',
      header: nowrap('清点'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{row.entryCount}</span>,
    },
    {
      key: 'diff',
      header: nowrap('待批'),
      right: true,
      width: fitColumnWidth,
      // 差异是结束盘点时才出的，还没结束的不写 0。
      render: (row) =>
        row.status === 'COUNTED' ? (
          <span className="whitespace-nowrap tabular-nums">
            {row.pendingDifferences}/{row.differenceCount}
          </span>
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    },
    {
      key: 'waiting',
      header: nowrap('已停留'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{elapsedLabel(countWaitingSince(row), now) || '—'}</span>,
    },
    {
      key: 'created',
      header: nowrap('创建'),
      right: true,
      width: fitColumnWidth,
      sortKey: createdKey,
      render: (row) => <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.createdAt)}>{shortInstant(row.createdAt, now) || '—'}</span>,
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <CountStep row={row} drawer={drawer} onOpen={setOpenKey} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : allQueues);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: countStatusLabel(code),
    count: counts[code] ?? null,
    active: status === code,
    attention: code === 'PENDING',
    onClick: () => chooseStatus(status === code ? allQueues : code),
  }));
  const narrowed = settled.trim() !== '' || filters.owner !== '' || filters.warehouse !== '' || status !== allQueues || pastFirstPage(pagination);
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setStatus(allQueues);
    filters.setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-counts',
        emptyTitle: '还没有盘点任务',
        emptyDescription: '点右上角新建盘点，选定仓库与库位后即可下发。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<StockCountRecord>
        title="盘点任务"
        viewStateKey={`${moduleId}.counts`}
        subtitle="点一行在右边看这张盘点单、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('count-create') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('count-create')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              新建盘点任务
            </Button>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜盘点单号、仓、货主、备注或登记人' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: countStatusLabel(code) }))}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.countId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? createdKey : `-${createdKey}`);
        }}
        noMatchText="当前筛选下没有盘点任务"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <CountDocument countId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这份盘点此刻该做的一步。审批要看每一条差异，所以这一步把详情栏打开，不另开整页。 */
function CountStep({ row, drawer, onOpen }: { row: StockCountRecord; drawer: ActionDrawerControl; onOpen: (countId: string) => void }) {
  const next = countNextStep(row);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (next.kind === 'review' && !drawer.has('count-approve') && !drawer.has('count-reject')) {
    return <span className="whitespace-nowrap text-idpxyz-textMuted">{row.pendingDifferences} 条差异待批</span>;
  }
  const actionId = next.kind === 'start' ? 'count-start' : next.kind === 'complete' ? 'count-complete' : null;
  if (actionId !== null && !drawer.has(actionId)) {
    return <span className="whitespace-nowrap text-idpxyz-textMuted">{next.kind === 'start' ? '等开始' : '等结束'}</span>;
  }
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        if (next.kind === 'go') goWorkstation();
        else if (next.kind === 'review') onOpen(row.countId);
        else if (actionId !== null) drawer.open(actionId, { countId: row.countId });
      }}
    >
      {next.label}
    </Button>
  );
}
