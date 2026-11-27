import { useMemo, useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listJobDiscrepancies, type JobDiscrepancyRecord } from './api';
import { countsAdjustmentsModule as moduleId, discrepancyKind } from './counts-adjustments-address';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf } from './list-cells';
import { JobDiscrepancyDocument } from './JobDiscrepancyDocument';
import { discrepancySignals, discrepancyStatusLabel } from './stock-approval-flow';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const pending = 'PENDING';
const allQueues = 'ALL';
const noRows: JobDiscrepancyRecord[] = [];

const queueStatuses = [pending] as const;
const settledStatuses = ['APPROVED', 'REJECTED'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/**
 * 作业差异。打开落在待批。待批的数量在标题行上。
 * 点一行在右边开这条差异，差异号仍进整页。行上待批的只留批准入账；驳回在右栏。
 * 读口只能按差异号排，差异号是随机签发的，不反映先后，所以表头不能排序，也不写等了多久。
 */
export function JobDiscrepancyList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.discrepancies.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : pending;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const conditions: CatalogueConditions = {
    ...base,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listJobDiscrepancies);
  const held = heldCataloguePage(answer, previous, (body) => body.discrepancies.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.discrepancies : (held?.body.discrepancies ?? noRows);
  const counts = useStatusCounts(listJobDiscrepancies, base, 'status', queueStatuses, refreshes);
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

  const columns: WorkbenchColumn<JobDiscrepancyRecord>[] = [
    {
      key: 'id',
      header: '差异号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[discrepancyKind, row.discrepancyId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={discrepancyStatusLabel(row.status)} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 860, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'goods',
      header: '商品 / 整箱',
      render: (row) => {
        const goods = row.goodsCode || '';
        const carton = row.cartonNumber ? `整箱 ${row.cartonNumber}` : '';
        if (goods && carton) return <WmsTwoLine fill primary={<span className="font-mono">{goods}</span>} secondary={carton} title={`${goods}\n${carton}`} />;
        if (goods) return <span className="whitespace-nowrap font-mono">{goods}</span>;
        if (carton) return <span className="whitespace-nowrap font-mono">{carton}</span>;
        return <span className="text-idpxyz-textMuted">—</span>;
      },
    },
    {
      key: 'location',
      header: '库位',
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => <span className="whitespace-nowrap font-mono">{row.locationCode || '—'}</span>,
    },
    {
      key: 'qty',
      header: nowrap('差额'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{row.quantity}</span>,
    },
    {
      key: 'signal',
      header: '信号',
      width: fitColumnWidth,
      minListWidth: 1280,
      render: (row) => nowrap(discrepancySignals(row).join(' · ') || '—'),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <ApprovalStep row={row} drawer={drawer} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : pending);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: discrepancyStatusLabel(code),
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
        endpoint: 'GET /warehouse-fulfillment-job-discrepancies',
        emptyTitle: status === pending ? '没有待批的作业差异' : '还没有作业差异',
        emptyDescription: '一线在作业台报短少或库位对不上时，差异会出现在这里等待审批。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<JobDiscrepancyRecord>
        title="作业差异审批"
        viewStateKey={`${moduleId}.discrepancies`}
        subtitle="点一行在右边核对这条差异来自哪次库内作业"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜差异号、作业事实号、货主、仓、商品、库位或箱号' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: discrepancyStatusLabel(code) }))}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.discrepancyId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        noMatchText="当前筛选下没有作业差异"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <JobDiscrepancyDocument discrepancyId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function ApprovalStep({ row, drawer }: { row: JobDiscrepancyRecord; drawer: ActionDrawerControl }) {
  if (row.status !== pending) return <span className="text-idpxyz-textMuted">—</span>;
  const action = drawer.has('job-discrepancy-approve') ? 'job-discrepancy-approve' : drawer.has('job-discrepancy-reject') ? 'job-discrepancy-reject' : null;
  if (action === null) return <span className="whitespace-nowrap text-idpxyz-textMuted">等批准</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open(action, { discrepancyId: row.discrepancyId });
      }}
    >
      {action === 'job-discrepancy-approve' ? '批准入账' : '驳回'}
    </Button>
  );
}
