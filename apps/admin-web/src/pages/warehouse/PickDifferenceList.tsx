import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listPickDifferences, type PickDifferenceRecord } from './api';
import { countKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { DocumentLink } from './document-page';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 待盘点是要人动手的那一队，数出来；已了结只增不减，不数。
const queueStatuses = ['PENDING'] as const;

/** 拣货差异的状态词：待处理的在等一份盘点，所以叫待盘点。陌生的词原样示出。 */
export function pickDifferenceStatusLabel(status: string): string {
  if (status === 'PENDING') return '待盘点';
  if (status === 'CLOSED') return '已了结';
  return status;
}

/**
 * 少拣待盘点：拣货时某个库位拣不够数留下的差异。差异待盘点期间，那个库位上的这种货不再分给拣货任务；
 * 建一份盘点把那个库位盘完，差异就了结，账实差多少由那份盘点自己的差异去调。
 */
export function PickDifferenceList({ filters }: { filters: QueueFilters }) {
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
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listPickDifferences);
  const rows = answer?.kind === 'outcome' ? answer.body.differences : [];
  const counts = useStatusCounts(listPickDifferences, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const countThisLocation = (row: PickDifferenceRecord) =>
    // 范围收到这位货主的这种货：同一个库位上别的货、别的货主的预占不必为这一份盘点让路。
    drawer.open('count-create', {
      warehouse: row.warehouseCode,
      locations: row.locationCode,
      ownerAccount: row.ownerAccount,
      goods: row.goodsCode,
      note: `少拣差异 ${row.differenceId}`,
    });

  const columns: ListColumn<PickDifferenceRecord>[] = [
    { id: 'id', header: '差异号', className: mono, render: (row) => row.differenceId },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={pickDifferenceStatusLabel(row.status)} /> },
    { id: 'location', header: '库位', className: mono, render: (row) => row.locationCode },
    { id: 'goods', header: '商品', className: mono, render: (row) => row.goodsCode },
    { id: 'short', header: '少拣 / 应拣', align: 'right', className: mono, render: (row) => `${row.quantity} / ${row.taskQuantity} ${row.unit}` },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'wave', header: '波次', className: 'whitespace-nowrap', render: (row) => <DocumentLink moduleId="warehouse-waves" parts={[row.waveId]} /> },
    { id: 'at', header: '记下时刻', className: mono, render: (row) => (row.recordedAt ? formatInstant(row.recordedAt) : '—') },
  ];

  const inspectorOf = (row: PickDifferenceRecord): InspectorContent => {
    const audit: InspectorField[] = [];
    if (row.recordedAt) audit.push({ label: '记下', value: formatInstant(row.recordedAt), mono: true });
    if (row.closedByCount) {
      audit.push({ label: '了结', value: `盘点 ${row.closedByCount}${row.closedAt ? ` · ${formatInstant(row.closedAt)}` : ''}`, mono: true });
    }
    const actions: InspectorAction[] = [
      { label: '打开波次', onRun: () => openDocument('warehouse-waves', [row.waveId]) },
      { label: '打开出库单', onRun: () => openDocument('warehouse-outbound', [row.requestId]) },
    ];
    if (row.status === 'PENDING' && drawer.has('count-create')) {
      actions.unshift({ label: '为这个库位建盘点', onRun: () => countThisLocation(row) });
    }
    if (row.closedByCount) {
      actions.push({ label: '打开了结它的盘点', onRun: () => openDocument(moduleId, [countKind, row.closedByCount as string]) });
    }
    return {
      title: row.differenceId,
      subtitle: `${row.warehouseCode} · ${row.locationCode}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '商品', value: row.goodsCode, mono: true },
            { label: '库存状态', value: labelOf(row.stockState) },
            { label: '应拣 / 实拣 / 少拣', value: `${row.taskQuantity} / ${row.picked} / ${row.quantity} ${row.unit}`, mono: true },
            { label: '拣货任务', value: row.taskId, mono: true },
            { label: '出库单', value: row.requestId, mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: pickDifferenceStatusLabel(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<PickDifferenceRecord>
        moduleId={moduleId}
        title="少拣待盘点"
        description="拣货时某个库位拣不够数留下的差异。待盘点期间，那个库位上的这种货不再分给拣货任务。点一行，在右栏为那个库位建一份盘点。盘出来账实相符，差异随盘点完成了结；盘出差额的，等那份差额批准或驳回再了结。"
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜差异号、库位、商品、出库请求号或波次号' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: pickDifferenceStatusLabel(code), count: counts[code] })),
                { value: 'CLOSED', label: pickDifferenceStatusLabel('CLOSED') },
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
        rowKey={(row) => row.differenceId}
        inspector={inspectorOf}
        pagination={pagination}
        emptyRowsNote="当前条件下没有拣货差异"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-pick-differences',
          emptyTitle: filters.status === 'PENDING' ? '没有待盘点的少拣库位' : '还没有拣货差异',
          emptyDescription: '拣货的人在某个库位拣不够数时，差异会出现在这里，等一份盘点把那个库位盘清。',
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
