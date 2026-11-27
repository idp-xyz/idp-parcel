import { useMemo, useState, type ReactNode } from 'react';
import { TabletSmartphone } from 'lucide-react';
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
import { listInternalJobs, type InternalJobRecord } from './api';
import { ClearFilters } from './counts-list-chrome';
import { DocumentLink } from './document-page';
import { jobSignal } from './internal-job-flow';
import { InternalJobDocument } from './InternalJobDocument';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { internalJobKind, processingModule as moduleId } from './processing-address';
import { useWarehouseLookups } from './use-catalogues';
import type { QueueFilters } from './use-queue-filters';
import { labelOf, WmsBadge, WmsFilterSelect, WmsTwoLine } from './wms-ui';

const info = moduleInfoById[moduleId];
const receivedKey = 'receivedAt';
const noRows: InternalJobRecord[] = [];
const kindWords = ['INTERNAL_MOVE', 'FORM_CONVERSION', 'CONDITION_CONVERSION', 'SHORTAGE'] as const;

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 缺省最新收到的在上面。人点过表头就照他的。读口排的是收到时刻，不是现场发生的时刻。 */
function receivedSort(stored: string | null): WorkbenchSort {
  if (stored === receivedKey) return { key: receivedKey, dir: 1 };
  return { key: receivedKey, dir: -1 };
}

/**
 * 库内作业台账。已经发生的事实，没有状态队。
 * 点一行在右边开这笔作业，事实号仍进整页。读口按收到时刻排。
 */
export function InternalJobList({ filters }: { filters: QueueFilters }) {
  const lookups = useWarehouseLookups();
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.jobs.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const sort = receivedSort(filters.sort);
  const baseFilters: Record<string, string[]> = {};
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  if (filters.kind) baseFilters.kind = [filters.kind];
  const conditions: CatalogueConditions = { q: settled, sort: sort.dir === 1 ? receivedKey : `-${receivedKey}`, filters: baseFilters };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listInternalJobs);
  const held = heldCataloguePage(answer, previous, (body) => body.jobs.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.jobs : (held?.body.jobs ?? noRows);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const warehouseNames = useMemo(
    () => new Map(lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [lookups.warehouses],
  );
  const now = new Date();

  const columns: WorkbenchColumn<InternalJobRecord>[] = [
    {
      key: 'fact',
      header: '事实号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[internalJobKind, row.factId]} />,
    },
    { key: 'kind', header: '作业种类', width: fitColumnWidth, render: (row) => <WmsBadge code={row.kind} label={labelOf(row.kind)} tone="info" /> },
    {
      key: 'warehouse',
      header: '仓',
      width: fitColumnWidth,
      minListWidth: 860,
      render: (row) => {
        const name = nameOf(warehouseNames, row.warehouse);
        return name ? <WmsTwoLine primary={name} secondary={row.warehouse} code title={`${name} · ${row.warehouse}`} /> : <span className="whitespace-nowrap font-mono">{row.warehouse}</span>;
      },
    },
    {
      key: 'signal',
      header: '入账情况',
      render: (row) => nowrap(jobSignal(row.kind, row.lines).text),
    },
    {
      key: 'lines',
      header: nowrap('行数'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1080,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{row.lines.length}</span>,
    },
    {
      key: 'occurred',
      header: nowrap('发生'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1200,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.occurredAt)}>
          {shortInstant(row.occurredAt, now) || '—'}
        </span>
      ),
    },
    {
      key: 'received',
      header: nowrap('收到'),
      right: true,
      width: fitColumnWidth,
      sortKey: receivedKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.receivedAt)}>
          {shortInstant(row.receivedAt, now) || '—'}
        </span>
      ),
    },
  ];

  const narrowed = settled.trim() !== '' || filters.warehouse !== '' || filters.kind !== '' || pastFirstPage(pagination);
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setWarehouse('');
    filters.setKind('');
    filters.setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-internal-jobs',
        emptyTitle: '还没有库内作业记录',
        emptyDescription: '在仓内作业台提交移库或报短少后，记录会出现在这里。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <WorkbenchPageTemplate<InternalJobRecord>
      title="库内作业台账"
      viewStateKey={`${moduleId}.jobs`}
      subtitle="点一行在右边看这笔作业，逐行看账上入了多少"
      onRefresh={refresh}
      refreshing={answer === null}
      primaryAction={
        <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
          <TabletSmartphone className="mr-1 h-3.5 w-3.5" />
          打开作业台
        </Button>
      }
      search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜事实号或仓库' }}
      extraFilters={
        <>
          <WmsFilterSelect label="作业种类" value={filters.kind} onChange={filters.setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
          <WmsFilterSelect
            label="仓库"
            value={filters.warehouse}
            onChange={filters.setWarehouse}
            options={lookups.warehouses.map((warehouse) => ({
              value: warehouse.warehouseCode,
              label: warehouse.name ? `${warehouse.name} · ${warehouse.warehouseCode}` : warehouse.warehouseCode,
            }))}
          />
          {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
        </>
      }
      columns={columns}
      rows={rows}
      rowKey={(row) => row.factId}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        filters.setSort(next.dir === 1 ? receivedKey : `-${receivedKey}`);
      }}
      noMatchText="当前筛选下没有库内作业"
      onClearFilters={clearFilters}
      renderDetail={(key) => <InternalJobDocument factId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}
