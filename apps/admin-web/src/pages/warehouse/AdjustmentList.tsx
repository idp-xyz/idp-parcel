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
import { listAdjustments, type StockAdjustmentRecord } from './api';
import { adjustmentKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { AdjustmentDocument } from './AdjustmentDocument';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { adjustmentStatusLabel } from './stock-approval-flow';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const submittedKey = 'submittedAt';
const pending = 'PENDING';
const allQueues = 'ALL';
const noRows: StockAdjustmentRecord[] = [];

const queueStatuses = [pending] as const;
const settledStatuses = ['POSTED', 'REJECTED'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 读口按提交时刻排。待批先提交的在上面；看全部或已了结的，最新提交的在上面。人点过表头就照他的。 */
function submittedSort(stored: string | null, inQueue: boolean): WorkbenchSort {
  if (stored === submittedKey) return { key: submittedKey, dir: 1 };
  if (stored === `-${submittedKey}`) return { key: submittedKey, dir: -1 };
  return { key: submittedKey, dir: inQueue ? 1 : -1 };
}

/**
 * 库存调整。打开落在待批。待批的数量在标题行上。
 * 点一行在右边开这张调整单，调整号仍进整页。行上待批的只留批准入账；驳回在右栏。
 */
export function AdjustmentList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.adjustments.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : pending;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const sort = submittedSort(filters.sort, status === pending);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? submittedKey : `-${submittedKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listAdjustments);
  const held = heldCataloguePage(answer, previous, (body) => body.adjustments.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.adjustments : (held?.body.adjustments ?? noRows);
  const counts = useStatusCounts(listAdjustments, base, 'status', queueStatuses, refreshes);
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

  const columns: WorkbenchColumn<StockAdjustmentRecord>[] = [
    {
      key: 'id',
      header: '调整单',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[adjustmentKind, row.adjustmentId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={adjustmentStatusLabel(row.status)} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 860, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'reason',
      header: '原因',
      render: (row) => (row.reason ? <WmsTwoLine fill primary={row.reason} title={row.reason} /> : <span className="text-idpxyz-textMuted">没写原因</span>),
    },
    {
      key: 'lines',
      header: nowrap('行数'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{row.lines?.length ?? 0}</span>,
    },
    {
      key: 'waiting',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums">{row.status === pending ? elapsedLabel(row.submittedAt, now) || '—' : '—'}</span>
      ),
    },
    {
      key: 'submitted',
      header: nowrap('提交'),
      right: true,
      width: fitColumnWidth,
      sortKey: submittedKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.submittedAt)}>
          {shortInstant(row.submittedAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <ApprovalStep row={row} drawer={drawer} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : pending);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: adjustmentStatusLabel(code),
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
        endpoint: 'GET /warehouse-fulfillment-adjustments',
        emptyTitle: status === pending ? '没有待批的库存调整' : '还没有库存调整',
        emptyDescription: '点右上角提交调整。批准人受「仓配规则设置」里的批准职责约束。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<StockAdjustmentRecord>
        title="库存调整"
        viewStateKey={`${moduleId}.adjustments`}
        subtitle="点一行在右边核这张调整单。提交不改账，批准才入账"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('adjustment-submit') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('adjustment-submit')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              提交库存调整
            </Button>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜调整单号、货主、仓、原因或提交人' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: adjustmentStatusLabel(code) }))}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.adjustmentId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? submittedKey : `-${submittedKey}`);
        }}
        noMatchText="当前筛选下没有库存调整"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <AdjustmentDocument adjustmentId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 待批的那一行只留批准。没有批准授予、但有驳回授予的，这一步改成驳回，免得行上什么都不能做。 */
function ApprovalStep({ row, drawer }: { row: StockAdjustmentRecord; drawer: ActionDrawerControl }) {
  if (row.status !== pending) return <span className="text-idpxyz-textMuted">—</span>;
  const action = drawer.has('adjustment-approve') ? 'adjustment-approve' : drawer.has('adjustment-reject') ? 'adjustment-reject' : null;
  if (action === null) return <span className="whitespace-nowrap text-idpxyz-textMuted">等批准</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open(action, { adjustmentId: row.adjustmentId });
      }}
    >
      {action === 'adjustment-approve' ? '批准入账' : '驳回'}
    </Button>
  );
}
