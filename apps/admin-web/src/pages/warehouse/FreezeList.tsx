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
import { listFreezes, type StockFreezeRecord } from './api';
import { DocumentLink } from './document-page';
import { freezeIsCarton, freezeNextStep, freezeQuantityLabel, freezeWaitingSince } from './freeze-flow';
import { freezeKind, processingModule as moduleId } from './processing-address';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一份冻结往下走的先后：冻结中是等人解冻的一队，数。已解冻只增不减，数出来的不是待办，不数；
// 「全部」放最后，页面打开就落在冻结中上。
const queueStatuses = ['FROZEN'] as const;
const settledStatuses = ['UNFROZEN'] as const;
// 冻的对象是读口声明的 target 筛选维，词表就这两个：按商品格冻，或按整箱冻。
const targetWords = ['GOODS', 'CARTON'] as const;

const newestFirst = '-frozenAt';
const oldestFirst = 'frozenAt';

/** 冻结：冻结中的商品格或整箱不能被出库分配。点冻结号打开冻结单。 */
export function FreezeList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  // 筛选态里的「种类」在这一页就是冻结的对象，读口的筛选维叫 target。
  if (filters.kind) baseFilters.target = [filters.kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了冻结中这一队，按冻结先后排，冻得最久的在最上面；看全部或已解冻的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(filters.status);
  const effectiveSort = filters.sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: filters.status === 'ALL' ? baseFilters : { ...baseFilters, status: [filters.status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listFreezes);
  const rows = answer?.kind === 'outcome' ? answer.body.freezes : [];
  const counts = useStatusCounts(listFreezes, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<StockFreezeRecord>[] = [
    {
      id: 'id',
      header: '冻结号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[freezeKind, row.freezeId]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    {
      id: 'subject',
      header: '冻的是什么',
      className: mono,
      // 按商品格冻的写库位与商品，按整箱冻的写箱号：库存页没有按库位或商品的地址，这里只写、不做链接。
      render: (row) => (freezeIsCarton(row) ? `整箱 ${row.cartonNumber || '—'}` : `${row.locationCode || '—'} · ${row.goodsCode || '—'}`),
    },
    { id: 'qty', header: '数量', align: 'right', className: mono, render: (row) => freezeQuantityLabel(row) },
    { id: 'reason', header: '原因', className: 'max-w-[18rem] truncate', render: (row) => row.reason },
    { id: 'at', header: '冻结时刻', className: mono, render: (row) => formatInstant(row.frozenAt) },
    {
      id: 'waiting',
      header: '已冻',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只有冻结中的还冻着；已解冻的不写冻了多久。
      render: (row) => elapsedLabel(freezeWaitingSince(row), now) || '—',
    },
    { id: 'next', header: '下一步', className: 'whitespace-nowrap', render: (row) => <NextStepCell row={row} drawer={drawer} /> },
  ];

  const inspectorOf = (row: StockFreezeRecord): InspectorContent => {
    const carton = freezeIsCarton(row);
    const actions: InspectorAction[] = [{ label: '打开冻结单', onRun: () => openDocument(moduleId, [freezeKind, row.freezeId]) }];
    if (freezeNextStep(row.status) !== null && drawer.has('stock-unfreeze')) {
      actions.push({ label: '解冻', onRun: () => drawer.open('stock-unfreeze', { freezeId: row.freezeId }) });
    }
    const audit: InspectorField[] = [{ label: '冻结', value: `${row.operator} · ${formatInstant(row.frozenAt)}`, mono: true }];
    if (row.status === 'UNFROZEN') {
      audit.push({
        label: '解冻',
        value: `${row.unfreezeOperator || '—'}${row.unfrozenAt ? ` · ${formatInstant(row.unfrozenAt)}` : ''}${row.unfreezeNote ? ` · ${row.unfreezeNote}` : ''}`,
        mono: true,
      });
    }
    return {
      title: row.freezeId,
      subtitle: row.reason,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '对象', value: labelOf(row.target) },
            ...(carton
              ? [{ label: '箱号', value: row.cartonNumber || '—', mono: true }]
              : [
                  { label: '库位', value: row.locationCode || '—', mono: true },
                  { label: '商品', value: row.goodsCode || '—', mono: true },
                ]),
            { label: '数量', value: freezeQuantityLabel(row), mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: labelOf(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<StockFreezeRecord>
        moduleId={moduleId}
        title="冻结与解冻"
        description="冻结中的商品格或整箱不能被出库分配。点状态看那一队，数量是当前筛选下还有几份，冻得最久的排在前面。点冻结号打开冻结单，冻结中的可以整份解冻。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            {drawer.has('condition-restore') ? (
              <Button type="button" variant="outline" onClick={() => drawer.open('condition-restore')}>
                次品转良品
              </Button>
            ) : null}
            {drawer.has('stock-freeze') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('stock-freeze')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                冻结库存
              </Button>
            ) : null}
          </div>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜冻结号、货主、仓、库位、商品、箱号或原因' }}
        sort={{
          value: effectiveSort,
          onChange: filters.setSort,
          options: [
            { value: newestFirst, label: '最新冻结的在前' },
            { value: oldestFirst, label: '冻得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: labelOf(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: labelOf(code) })),
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
            <WmsFilterSelect label="对象" value={filters.kind} onChange={filters.setKind} options={targetWords.map((word) => ({ value: word, label: labelOf(word) }))} />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.freezeId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [freezeKind, row.freezeId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有冻结记录"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-freezes',
          emptyTitle: filters.status === 'FROZEN' ? '当前没有冻结中的库存' : '还没有冻结记录',
          emptyDescription: '可在此冻结，或在实时库存对单个格子点冻结。',
          narrowed:
            settled.trim() !== '' ||
            filters.owner !== '' ||
            filters.warehouse !== '' ||
            filters.kind !== '' ||
            (filters.status !== 'FROZEN' && filters.status !== 'ALL'),
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：冻结中的一步是解冻。能就地做的是按钮，没有授予的只说在等什么，已解冻的没有下一步。 */
function NextStepCell({ row, drawer }: { row: StockFreezeRecord; drawer: ActionDrawerControl }) {
  const next = freezeNextStep(row.status);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('stock-unfreeze')) return <span className="text-idpxyz-textMuted">等解冻</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只开表单，不顺带把行选进右栏。
        event.stopPropagation();
        drawer.open('stock-unfreeze', { freezeId: row.freezeId });
      }}
    >
      {next.label}
    </Button>
  );
}
