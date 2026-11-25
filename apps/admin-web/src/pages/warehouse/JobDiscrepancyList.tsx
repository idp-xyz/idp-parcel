import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listJobDiscrepancies, type JobDiscrepancyRecord } from './api';
import { countsAdjustmentsModule as moduleId, discrepancyKind } from './counts-adjustments-address';
import { DocumentLink } from './document-page';
import { discrepancySignals, discrepancyStatusLabel } from './stock-approval-flow';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一条差异往下走的先后：待批在最前，它是要人动手的那一队。已批准入账与已驳回只增不减，数出来的不是待办，不数；
// 「全部」放最后，页面打开就落在待批上。
const queueStatuses = ['PENDING'] as const;
const settledStatuses = ['APPROVED', 'REJECTED'] as const;

/**
 * 作业差异审批：上架、移库、形态转换与短少上报留下的账面差异。点差异号打开作业差异。
 * 读口只能按差异号排，差异号是随机签发的，不反映先后；差异本身也没有形成时刻，所以这一页没有排序，也不写等了多久。
 */
export function JobDiscrepancyList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const conditions: CatalogueConditions = {
    ...base,
    filters: filters.status === 'ALL' ? baseFilters : { ...baseFilters, status: [filters.status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listJobDiscrepancies);
  const rows = answer?.kind === 'outcome' ? answer.body.discrepancies : [];
  const counts = useStatusCounts(listJobDiscrepancies, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const columns: ListColumn<JobDiscrepancyRecord>[] = [
    {
      id: 'id',
      header: '差异号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[discrepancyKind, row.discrepancyId]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={discrepancyStatusLabel(row.status)} /> },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'goods', header: '商品 / 整箱', className: mono, render: (row) => row.goodsCode || row.cartonNumber || '—' },
    { id: 'qty', header: '差额数量', align: 'right', className: mono, render: (row) => String(row.quantity) },
    { id: 'signal', header: '信号', render: (row) => discrepancySignals(row).join(' · ') || '—' },
  ];

  const inspectorOf = (row: JobDiscrepancyRecord): InspectorContent => {
    const audit: InspectorField[] = [];
    if (row.approvedBy) audit.push({ label: '批准', value: `${row.approvedBy}${row.approvedAt ? ` · ${formatInstant(row.approvedAt)}` : ''}`, mono: true });
    if (row.rejectedBy) audit.push({ label: '驳回', value: `${row.rejectedBy}${row.rejectedAt ? ` · ${formatInstant(row.rejectedAt)}` : ''}`, mono: true });
    const actions: InspectorAction[] = [{ label: '打开作业差异', onRun: () => openDocument(moduleId, [discrepancyKind, row.discrepancyId]) }];
    if (row.status === 'PENDING' && drawer.has('job-discrepancy-approve')) {
      actions.push({ label: '批准入账', onRun: () => drawer.open('job-discrepancy-approve', { discrepancyId: row.discrepancyId }) });
    }
    if (row.status === 'PENDING' && drawer.has('job-discrepancy-reject')) {
      actions.push({ label: '驳回', onRun: () => drawer.open('job-discrepancy-reject', { discrepancyId: row.discrepancyId }) });
    }
    return {
      title: row.discrepancyId,
      subtitle: `作业事实 ${row.factId}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓 / 库位', value: `${row.warehouseCode} · ${row.locationCode || '—'}`, mono: true },
            { label: '商品', value: row.goodsCode || '—', mono: true },
            { label: '差额数量', value: String(row.quantity), mono: true },
            { label: '库存状态', value: labelOf(row.stockState) },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: discrepancyStatusLabel(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<JobDiscrepancyRecord>
        moduleId={moduleId}
        title="作业差异审批"
        description="上架、移库、形态转换与短少上报留下的账面差异。点状态看那一队，待批的数量是当前筛选下还有几条。点差异号打开作业差异，核对来自哪次库内作业再批准或驳回。"
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜差异号、作业事实号、货主、仓、商品、库位或箱号' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: discrepancyStatusLabel(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: discrepancyStatusLabel(code) })),
                { value: 'ALL', label: '全部' },
              ]}
            />
            <WmsFilterSelect
              label="货主"
              value={filters.owner}
              onChange={filters.setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.accountId} · ${account.customerPartyName}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={filters.warehouse}
              onChange={filters.setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.discrepancyId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [discrepancyKind, row.discrepancyId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有作业差异"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-job-discrepancies',
          emptyTitle: filters.status === 'PENDING' ? '没有待批的作业差异' : '还没有作业差异',
          emptyDescription: '一线在作业台报短少或库位对不上时，差异会出现在这里等待审批。',
          narrowed:
            settled.trim() !== '' ||
            filters.owner !== '' ||
            filters.warehouse !== '' ||
            (filters.status !== 'PENDING' && filters.status !== 'ALL'),
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
