import { useMemo, useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listPickDifferences, type PickDifferenceRecord } from './api';
import { countKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const recordedKey = 'recordedAt';
const pending = 'PENDING';
const allQueues = 'ALL';
const noRows: PickDifferenceRecord[] = [];

const queueStatuses = [pending] as const;
const knownStatuses = [allQueues, pending, 'CLOSED'];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 拣货差异的状态词：待处理的在等一份盘点，所以叫待盘点。陌生的词原样示出。 */
export function pickDifferenceStatusLabel(status: string): string {
  if (status === 'PENDING') return '待盘点';
  if (status === 'CLOSED') return '已了结';
  return status;
}

/** 缺省按记下的时刻从新到旧，和读口的缺省序一样。人点过表头就照他的。 */
function recordedSort(stored: string | null): WorkbenchSort {
  if (stored === recordedKey) return { key: recordedKey, dir: 1 };
  return { key: recordedKey, dir: -1 };
}

/**
 * 少拣待盘点。打开落在待盘点。待盘点的数量在标题行上。
 * 这一条没有自己的单据页：点一行在右边看。行上只留为这个库位建盘点，范围收到这位货主的这种货。
 */
export function PickDifferenceList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.pick-diffs.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : pending;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const sort = recordedSort(filters.sort);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? recordedKey : `-${recordedKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listPickDifferences);
  const held = heldCataloguePage(answer, previous, (body) => body.differences.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.differences : (held?.body.differences ?? noRows);
  const counts = useStatusCounts(listPickDifferences, base, 'status', queueStatuses, refreshes);
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

  const columns: WorkbenchColumn<PickDifferenceRecord>[] = [
    {
      key: 'spot',
      header: '库位',
      width: fitColumnWidth,
      render: (row) => <WmsTwoLine primary={<span className="font-mono">{row.locationCode}</span>} secondary={row.goodsCode} code title={`${row.locationCode}\n${row.goodsCode}`} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={pickDifferenceStatusLabel(row.status)} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 860, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'short',
      header: nowrap('少拣 / 应拣'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.quantity} / ${row.taskQuantity} ${row.unit}`}</span>,
    },
    {
      key: 'wave',
      header: '波次',
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => <DocumentLink moduleId="warehouse-waves" parts={[row.waveId]} />,
    },
    {
      key: 'recorded',
      header: nowrap('记下'),
      right: true,
      width: fitColumnWidth,
      sortKey: recordedKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={row.recordedAt ? formatInstant(row.recordedAt) : undefined}>
          {shortInstant(row.recordedAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <PickStep row={row} drawer={drawer} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : pending);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: pickDifferenceStatusLabel(code),
    count: counts[code] ?? null,
    active: status === code,
    attention: true,
    onClick: () => chooseStatus(status === code ? allQueues : code),
  }));
  const narrowed =
    settled.trim() !== '' || filters.owner !== '' || filters.warehouse !== '' || (status !== pending && status !== allQueues) || pastFirstPage(pagination);
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setStatus(pending);
    filters.setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-pick-differences',
        emptyTitle: status === pending ? '没有待盘点的少拣库位' : '还没有拣货差异',
        emptyDescription: '拣货的人在某个库位拣不够数时，差异会出现在这里，等一份盘点把那个库位盘清。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<PickDifferenceRecord>
        title="少拣待盘点"
        viewStateKey={`${moduleId}.pick-diffs`}
        subtitle="点一行在右边看。待盘点期间，那个库位上的这种货不再分给拣货任务"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜差异号、库位、商品、出库请求号或波次号' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[
                { value: pending, label: pickDifferenceStatusLabel(pending) },
                { value: 'CLOSED', label: pickDifferenceStatusLabel('CLOSED') },
              ]}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.differenceId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? recordedKey : `-${recordedKey}`);
        }}
        noMatchText="当前筛选下没有拣货差异"
        onClearFilters={clearFilters}
        renderDetail={(key) => {
          const row = rows.find((item) => item.differenceId === key);
          return row ? (
            <PickDetail row={row} drawer={drawer} onClose={() => setOpenKey(null)} />
          ) : (
            <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这条差异。</p>
          );
        }}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function countThisLocation(drawer: ActionDrawerControl, row: PickDifferenceRecord) {
  // 范围收到这位货主的这种货：同一个库位上别的货、别的货主的预占不必为这一份盘点让路。
  drawer.open('count-create', {
    warehouse: row.warehouseCode,
    locations: row.locationCode,
    ownerAccount: row.ownerAccount,
    goods: row.goodsCode,
    note: `少拣差异 ${row.differenceId}`,
  });
}

function PickStep({ row, drawer }: { row: PickDifferenceRecord; drawer: ActionDrawerControl }) {
  if (row.status !== pending) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('count-create')) return <span className="whitespace-nowrap text-idpxyz-textMuted">等建盘点</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        countThisLocation(drawer, row);
      }}
    >
      为这个库位建盘点
    </Button>
  );
}

function PickDetail({ row, drawer, onClose }: { row: PickDifferenceRecord; drawer: ActionDrawerControl; onClose: () => void }) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">少拣</span>
            <span className="font-mono">{row.locationCode}</span>
            <WmsBadge code={row.status} label={pickDifferenceStatusLabel(row.status)} />
          </div>
          <p className="mt-1 type-caption text-idpxyz-textMuted">
            {row.warehouseCode} · {row.goodsCode}
          </p>
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">
        <p className="tabular-nums">
          应拣 {row.taskQuantity} · 实拣 {row.picked} · 少拣 {row.quantity} {row.unit}
        </p>
        <p>库存状态 {labelOf(row.stockState)}</p>
        <p>
          波次 <DocumentLink moduleId="warehouse-waves" parts={[row.waveId]} />
        </p>
        <p>
          出库单 <DocumentLink moduleId="warehouse-outbound" parts={[row.requestId]} />
        </p>
        <p className="font-mono type-caption text-idpxyz-textMuted">拣货任务 {row.taskId}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">记下 {row.recordedAt ? formatInstant(row.recordedAt) : '—'}</p>
        {row.closedByCount ? (
          <p>
            了结于 <DocumentLink moduleId={moduleId} parts={[countKind, row.closedByCount]} />
            {row.closedAt ? <span className="font-mono type-caption text-idpxyz-textMuted"> · {formatInstant(row.closedAt)}</span> : null}
          </p>
        ) : (
          <p className="type-caption text-idpxyz-textMuted">盘出来账实相符，差异随盘点完成了结；盘出差额的，等那份差额批准或驳回再了结。</p>
        )}
        <PickStep row={row} drawer={drawer} />
      </div>
    </div>
  );
}
