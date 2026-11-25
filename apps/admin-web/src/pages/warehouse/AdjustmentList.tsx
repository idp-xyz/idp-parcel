import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listAdjustments, type StockAdjustmentRecord } from './api';
import { adjustmentKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { DocumentLink } from './document-page';
import { adjustmentStatusLabel } from './stock-approval-flow';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一张调整单往下走的先后：待批在最前，它是要人动手的那一队。已批准入账与已驳回只增不减，数出来的不是待办，不数；
// 「全部」放最后，页面打开就落在待批上。
const queueStatuses = ['PENDING'] as const;
const settledStatuses = ['POSTED', 'REJECTED'] as const;

const newestFirst = '-submittedAt';
const oldestFirst = 'submittedAt';

/** 库存调整：盘盈、盘亏与销毁申请。提交不改账，批准才入账。点调整号打开库存调整单。 */
export function AdjustmentList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待批这一队，按提交先后排，等得最久的在最上面；看全部或已了结的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(filters.status);
  const effectiveSort = filters.sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: filters.status === 'ALL' ? baseFilters : { ...baseFilters, status: [filters.status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listAdjustments);
  const rows = answer?.kind === 'outcome' ? answer.body.adjustments : [];
  const counts = useStatusCounts(listAdjustments, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<StockAdjustmentRecord>[] = [
    {
      id: 'id',
      header: '调整单号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[adjustmentKind, row.adjustmentId]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={adjustmentStatusLabel(row.status)} /> },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'lines', header: '行数', align: 'right', className: mono, render: (row) => String(row.lines?.length ?? 0) },
    { id: 'reason', header: '原因', className: 'max-w-[18rem] truncate', render: (row) => row.reason },
    { id: 'at', header: '提交时刻', className: mono, render: (row) => formatInstant(row.submittedAt) },
    {
      id: 'waiting',
      header: '已等',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只有待批的在等人；已了结的单不写等了多久。
      render: (row) => (row.status === 'PENDING' ? elapsedLabel(row.submittedAt, now) || '—' : '—'),
    },
  ];

  const inspectorOf = (row: StockAdjustmentRecord): InspectorContent => {
    const audit: InspectorField[] = [{ label: '提交', value: `${row.operator} · ${formatInstant(row.submittedAt)}`, mono: true }];
    if (row.approvedBy) audit.push({ label: '批准', value: `${row.approvedBy}${row.approvedAt ? ` · ${formatInstant(row.approvedAt)}` : ''}`, mono: true });
    if (row.rejectedBy) audit.push({ label: '驳回', value: `${row.rejectedBy}${row.rejectedAt ? ` · ${formatInstant(row.rejectedAt)}` : ''}`, mono: true });
    const actions: InspectorAction[] = [{ label: '打开库存调整单', onRun: () => openDocument(moduleId, [adjustmentKind, row.adjustmentId]) }];
    if (row.status === 'PENDING' && drawer.has('adjustment-approve')) {
      actions.push({ label: '批准入账', onRun: () => drawer.open('adjustment-approve', { adjustmentId: row.adjustmentId }) });
    }
    if (row.status === 'PENDING' && drawer.has('adjustment-reject')) {
      actions.push({ label: '驳回', onRun: () => drawer.open('adjustment-reject', { adjustmentId: row.adjustmentId }) });
    }
    return {
      title: row.adjustmentId,
      subtitle: row.reason,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '调整行', value: String(row.lines?.length ?? 0), mono: true },
            { label: '提交人', value: row.operator, mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: adjustmentStatusLabel(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<StockAdjustmentRecord>
        moduleId={moduleId}
        title="库存调整"
        description="盘盈、盘亏与销毁申请。提交不改账，批准才入账；点状态看那一队，待批的数量是当前筛选下还有几张。点调整号打开库存调整单，核完每一行再批准或驳回。"
        headerActions={
          drawer.has('adjustment-submit') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('adjustment-submit')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              提交库存调整
            </Button>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜调整单号、货主、仓、原因或提交人' }}
        sort={{
          value: effectiveSort,
          onChange: filters.setSort,
          options: [
            { value: newestFirst, label: '最新提交的在前' },
            { value: oldestFirst, label: '等得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: adjustmentStatusLabel(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: adjustmentStatusLabel(code) })),
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
        rowKey={(row) => row.adjustmentId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [adjustmentKind, row.adjustmentId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有库存调整"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-adjustments',
          emptyTitle: filters.status === 'PENDING' ? '没有待批的库存调整' : '还没有库存调整',
          emptyDescription: '点右上角提交调整。批准人受「仓配规则设置」里的批准职责约束。',
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
