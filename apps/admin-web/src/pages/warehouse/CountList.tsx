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
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listCounts, type StockCountRecord } from './api';
import { codesPreview, countNextStep, countStatusLabel, countWaitingSince } from './count-flow';
import { countKind, countsAdjustmentsModule as moduleId } from './counts-adjustments-address';
import { DocumentLink } from './document-page';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一份盘点往下走的先后：待开始与盘点中是要人动手的两队，都数。已盘点与已作废只增不减，数出来的不是待办，不数；
// 「全部」放最后。已盘点那一队里还有差异没批完的，读口没有这一维可以筛，所以每行的下一步写明还有几条待批。
const queueStatuses = ['PENDING', 'COUNTING'] as const;
const settledStatuses = ['COUNTED', 'VOID'] as const;

const newestFirst = '-createdAt';
const oldestFirst = 'createdAt';

const goWorkstation = () => {
  window.location.hash = '#/warehouse-workstation';
};

/** 盘点任务：创建后开始锁定范围，到作业台录入清点数，结束时出账实差异。点盘点号打开盘点单。 */
export function CountList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待开始或盘点中这样的队，按创建先后排，放得最久的在最上面；看全部或已了结的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(filters.status);
  const effectiveSort = filters.sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: filters.status === 'ALL' ? baseFilters : { ...baseFilters, status: [filters.status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCounts);
  const rows = answer?.kind === 'outcome' ? answer.body.counts : [];
  const counts = useStatusCounts(listCounts, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<StockCountRecord>[] = [
    {
      id: 'id',
      header: '盘点单号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[countKind, row.countId]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={countStatusLabel(row.status)} /> },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount || '不限货主' },
    {
      id: 'scope',
      header: '范围',
      className: 'whitespace-nowrap',
      render: (row) => `${row.locations.length} 个库位 · ${row.goods.length > 0 ? `${row.goods.length} 种商品` : '全部商品'}`,
    },
    { id: 'entries', header: '清点笔数', align: 'right', className: mono, render: (row) => String(row.entryCount) },
    {
      id: 'diff',
      header: '待批 / 差异',
      align: 'right',
      className: mono,
      // 差异是结束盘点时才出的，还没结束的不写 0 / 0。
      render: (row) =>
        row.status === 'COUNTED' ? (
          <span className={row.pendingDifferences > 0 ? 'text-amber-600 dark:text-amber-400' : undefined}>{`${row.pendingDifferences} / ${row.differenceCount}`}</span>
        ) : (
          '—'
        ),
    },
    { id: 'at', header: '创建时刻', className: mono, render: (row) => formatInstant(row.createdAt) },
    {
      id: 'waiting',
      header: '已停留',
      align: 'right',
      className: 'whitespace-nowrap',
      render: (row) => elapsedLabel(countWaitingSince(row), now) || '—',
    },
    {
      id: 'next',
      header: '下一步',
      className: 'whitespace-nowrap',
      render: (row) => <NextStepCell row={row} drawer={drawer} />,
    },
  ];

  const inspectorOf = (row: StockCountRecord): InspectorContent => {
    const audit: InspectorField[] = [{ label: '创建', value: `${row.operator} · ${formatInstant(row.createdAt)}`, mono: true }];
    if (row.startedAt) audit.push({ label: '开始', value: `${row.startedBy ?? ''} · ${formatInstant(row.startedAt)}`, mono: true });
    if (row.status === 'COUNTED' && row.completedAt) {
      audit.push({ label: '结束', value: `${row.completedBy ?? ''} · ${formatInstant(row.completedAt)}`, mono: true });
    }
    if (row.status === 'VOID' && row.voidedAt) {
      audit.push({ label: '作废', value: `${row.voidedBy ?? ''} · ${formatInstant(row.voidedAt)}`, mono: true });
    }
    const actions: InspectorAction[] = [{ label: '打开盘点单', onRun: () => openDocument(moduleId, [countKind, row.countId]) }];
    if (row.status === 'PENDING' && drawer.has('count-start')) {
      actions.push({ label: '开始盘点', onRun: () => drawer.open('count-start', { countId: row.countId }) });
    }
    if (row.status === 'COUNTING') actions.push({ label: '去作业台录数', onRun: goWorkstation });
    if (row.status === 'COUNTING' && drawer.has('count-complete')) {
      actions.push({ label: '结束并出差异', onRun: () => drawer.open('count-complete', { countId: row.countId }) });
    }
    if ((row.status === 'PENDING' || row.status === 'COUNTING') && drawer.has('count-void')) {
      actions.push({ label: '作废盘点', onRun: () => drawer.open('count-void', { countId: row.countId }) });
    }
    return {
      title: row.countId,
      subtitle: row.warehouseCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主', value: row.ownerAccount || '不限货主', mono: true },
            { label: '库位', value: codesPreview(row.locations, 4), mono: true },
            { label: '商品范围', value: codesPreview(row.goods, 4) || '范围内全部商品', mono: true },
            { label: '清点笔数', value: String(row.entryCount), mono: true },
            { label: '差异 / 待批', value: `${row.differenceCount} / ${row.pendingDifferences}`, mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: countStatusLabel(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<StockCountRecord>
        moduleId={moduleId}
        title="盘点任务"
        description="点状态看那一队，数量是当前筛选下还有几份；选了待开始或盘点中，放得最久的排在前面。点盘点号打开盘点单：看每个库位清点到哪、账实差异与批准。下一步一列写明该开始、录数、结束还是批差异。"
        headerActions={
          drawer.has('count-create') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('count-create')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              新建盘点任务
            </Button>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜盘点单号、仓、货主、备注或登记人' }}
        sort={{
          value: effectiveSort,
          onChange: filters.setSort,
          options: [
            { value: newestFirst, label: '最新创建的在前' },
            { value: oldestFirst, label: '放得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: countStatusLabel(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: countStatusLabel(code) })),
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
        rowKey={(row) => row.countId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [countKind, row.countId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有盘点任务"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-counts',
          emptyTitle: '还没有盘点任务',
          emptyDescription: '点右上角新建盘点，选定仓库与库位后即可下发。',
          narrowed: settled.trim() !== '' || filters.owner !== '' || filters.warehouse !== '' || filters.status !== 'ALL',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这份盘点此刻该做的一步。能就地做的是按钮，要去别处做的是链接，没有授予的只说在等什么。 */
function NextStepCell({ row, drawer }: { row: StockCountRecord; drawer: ActionDrawerControl }) {
  const next = countNextStep(row);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  const link = 'text-[color:var(--ds-text-link-hover)] hover:underline';
  if (next.kind === 'go') {
    return (
      <button
        type="button"
        className={link}
        onClick={(event) => {
          event.stopPropagation();
          goWorkstation();
        }}
      >
        {next.label}
      </button>
    );
  }
  if (next.kind === 'review') {
    // 批与驳回落在某一条差异上，要先打开盘点单；没有批准授予的人只看得到还剩几条。
    if (!drawer.has('count-approve') && !drawer.has('count-reject')) {
      return <span className="type-caption text-amber-600 dark:text-amber-400">{row.pendingDifferences} 条差异待批</span>;
    }
    return (
      <button
        type="button"
        className={link}
        onClick={(event) => {
          event.stopPropagation();
          openDocument(moduleId, [countKind, row.countId]);
        }}
      >
        {next.label}
      </button>
    );
  }
  const actionId = next.kind === 'start' ? 'count-start' : 'count-complete';
  if (!drawer.has(actionId)) return <span className="text-idpxyz-textMuted">{next.kind === 'start' ? '等开始' : '等结束'}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只开表单，不顺带把行选进右栏。
        event.stopPropagation();
        drawer.open(actionId, { countId: row.countId });
      }}
    >
      {next.label}
    </Button>
  );
}
