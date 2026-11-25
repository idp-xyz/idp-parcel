import { useMemo, useState } from 'react';
import { Plus, Send } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listOutboundRequests, type OutboundRequestRecord } from './api';
import { DocumentLink } from './document-page';
import { OutboundDocument } from './OutboundDocument';
import { outboundNextStep, outboundSettled } from './outbound-next-step';
import { openDocument, useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const moduleId = 'warehouse-outbound';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一张出库单往下走的先后；卡在预占的两种排在已预占前面，它们最需要人看。已出库与已取消放最后、不数：
// 那两队只增不减，数出来的不是待办。
const queueStatuses = ['PENDING_POLICY', 'PARTIALLY_RESERVED', 'RESERVED', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED'] as const;
const settledStatuses = ['SHIPPED', 'CANCELLED'] as const;
const dispatchWords = ['PRODUCT_NETWORK', 'OWNER_LABEL', 'PICKUP'] as const;

const newestFirst = '-receivedAt';
const oldestFirst = 'receivedAt';

/** 出库单：列表与每一张出库单各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function WarehouseOutboundPage() {
  const parts = useDocumentParts();
  return parts ? <OutboundDocument requestId={parts[0]} /> : <OutboundList />;
}

function OutboundList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('ALL');
  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [dispatch, setDispatch] = useState('');
  const [sort, setSort] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [refreshes, setRefreshes] = useState(0);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (owner) baseFilters.ownerAccount = [owner];
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  if (dispatch) baseFilters.dispatch = [dispatch];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了某一队之后按受理先后排，放得最久的在最上面；看全部或已了结的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(status);
  const effectiveSort = sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: status === 'ALL' ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];
  const counts = useStatusCounts(listOutboundRequests, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const agentWarehouses = useMemo(
    () => new Set(drawer.lookups.warehouses.filter((w) => w.kind === 'AGENT').map((w) => w.warehouseCode)),
    [drawer.lookups.warehouses],
  );
  const now = new Date();

  const selectedRecords = rows.filter((row) => selected.has(row.requestId));
  const sameWarehouse = new Set(selectedRecords.map((row) => row.warehouseCode)).size === 1;
  const allReserved = selectedRecords.every((row) => row.status === 'RESERVED');
  const canBulkWave = selectedRecords.length > 0 && sameWarehouse && allReserved;

  const columns: ListColumn<OutboundRequestRecord>[] = [
    {
      id: 'id',
      header: '出库单号',
      className: 'whitespace-nowrap',
      render: (row) => (
        <DocumentLink moduleId={moduleId} parts={[row.requestId]}>
          {row.documentNumber ?? row.requestId}
        </DocumentLink>
      ),
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'dispatch', header: '发运方式', className: 'whitespace-nowrap', render: (row) => labelOf(row.dispatch) },
    { id: 'recipient', header: '收件人', className: 'max-w-[14rem] truncate', render: (row) => row.recipient },
    {
      id: 'qty',
      header: '要发',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只在卡在预占的两种状态上说占到多少：拣出去的那部分预占已经放开，拣完之后的 0 不是缺货。
      render: (row) => {
        const wanted = row.lines.reduce((sum, line) => sum + line.quantity, 0);
        const stuck = row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY';
        return (
          <span>
            <span className="font-mono">{wanted}</span>
            {stuck ? <span className="ml-2 type-caption text-amber-600 dark:text-amber-400">只占到 {row.reservedQuantity}</span> : null}
          </span>
        );
      },
    },
    {
      id: 'age',
      header: '受理至今',
      align: 'right',
      className: 'whitespace-nowrap',
      // 已了结的单不写过了多久：它不在等谁。
      render: (row) => (outboundSettled(row.status) ? formatInstant(row.receivedAt) : elapsedLabel(row.receivedAt, now) || '—'),
    },
    {
      id: 'next',
      header: '下一步',
      className: 'whitespace-nowrap',
      render: (row) => <NextStepCell row={row} agentWarehouse={agentWarehouses.has(row.warehouseCode)} drawer={drawer} />,
    },
  ];

  const inspectorOf = (row: OutboundRequestRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '打开出库单', onRun: () => openDocument(moduleId, [row.requestId]) }];
    if (row.status === 'RESERVED' && drawer.has('wave-release')) {
      actions.push({ label: '放波次', onRun: () => drawer.open('wave-release', { warehouse: row.warehouseCode, requests: row.requestId }) });
    }
    if ((row.status === 'RESERVED' || row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY') && drawer.has('outbound-split')) {
      actions.push({ label: '拆单', onRun: () => drawer.open('outbound-split', { requestId: row.requestId }) });
    }
    if (row.status !== 'SHIPPED' && row.status !== 'CANCELLED' && drawer.has('outbound-cutoff')) {
      actions.push({ label: '截单', onRun: () => drawer.open('outbound-cutoff', { requestId: row.requestId }) });
    }
    if (row.dispatch === 'OWNER_LABEL' && drawer.has('owner-label')) {
      actions.push({ label: '登记货主面单', onRun: () => drawer.open('owner-label', { requestId: row.requestId }) });
    }
    return {
      title: row.documentNumber ?? row.requestId,
      subtitle: labelOf(row.status),
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓库', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '发运方式', value: labelOf(row.dispatch) },
            { label: '收件人', value: row.recipient },
            { label: '收件地址', value: row.address },
            { label: '明细', value: row.lines.map((line) => `${line.goodsCode}×${line.quantity}`).join('、') || '没有商品行', mono: true },
          ],
        },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '来源 / 受理时刻', value: `${labelOf(row.origin)} · ${formatInstant(row.receivedAt)}` }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<OutboundRequestRecord>
        moduleId={moduleId}
        title={info.title}
        description="点状态看那一队，数量是当前筛选下各队还有几张。点单号打开出库单；勾选同一个仓、已预占的几张可以合并放波次。"
        headerActions={
          drawer.has('outbound-request') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('outbound-request')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录出库单
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜出库单号、货主、仓、收件人、来源键' }}
        sort={{
          value: effectiveSort,
          onChange: setSort,
          options: [
            { value: newestFirst, label: '最新受理的在前' },
            { value: oldestFirst, label: '放得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={status}
              onChange={(next) => {
                setStatus(next);
                setSelected(new Set());
              }}
              tabs={[
                { value: 'ALL', label: '全部' },
                ...queueStatuses.map((code) => ({ value: code, label: labelOf(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: labelOf(code) })),
              ]}
            />
            <WmsFilterSelect
              label="货主"
              value={owner}
              onChange={setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.accountId} · ${account.customerPartyName}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
            <WmsFilterSelect
              label="发运方式"
              value={dispatch}
              onChange={setDispatch}
              options={dispatchWords.map((word) => ({ value: word, label: labelOf(word) }))}
            />
          </div>
        }
        selection={{ selected, onChange: setSelected }}
        bulkActions={{
          extra: drawer.has('wave-release') ? (
            <Button
              type="button"
              size="sm"
              className={primaryActionClass}
              disabled={!canBulkWave}
              title={canBulkWave ? '把勾选的出库单合成一个波次放出去' : '只有同一个仓、状态都是已预占的出库单才能合并放波次'}
              onClick={() =>
                drawer.open('wave-release', {
                  warehouse: selectedRecords[0]?.warehouseCode ?? '',
                  requests: selectedRecords.map((row) => row.requestId).join(', '),
                })
              }
            >
              <Send className="mr-1 h-3.5 w-3.5" />
              {`合并放波次（${selectedRecords.length} 张）`}
            </Button>
          ) : undefined,
        }}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.requestId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.requestId])}
        pagination={pagination}
        emptyRowsNote="当前筛选下没有出库单"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
          emptyTitle: '还没有出库单',
          emptyDescription: '点右上角「代录出库单」新建，或等货主经客户接口提交。',
          narrowed: settled.trim() !== '' || status !== 'ALL' || owner !== '' || warehouse !== '' || dispatch !== '',
        })}
      />
      <ActionDrawer
        moduleId={moduleId}
        active={drawer.active}
        lookups={drawer.lookups}
        onClose={drawer.close}
        onSubmitted={() => {
          setSelected(new Set());
          refresh();
        }}
      />
    </>
  );
}

/** 每行最右的那一格：这一单此刻该做的一步。能就地做的是按钮，要去别处做的是链接，等人定的只说在等什么。 */
function NextStepCell({
  row,
  agentWarehouse,
  drawer,
}: {
  row: OutboundRequestRecord;
  agentWarehouse: boolean;
  drawer: ActionDrawerControl;
}) {
  const next = outboundNextStep(row.status, agentWarehouse);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (next.kind === 'wait') return <span className="type-caption text-amber-600 dark:text-amber-400">{next.label}</span>;
  if (next.kind === 'wave') {
    if (!drawer.has('wave-release')) return <span className="text-idpxyz-textMuted">等放波次</span>;
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={(event) => {
          // 行上的单击是预览：这一下只开表单，不顺带把行选进右栏。
          event.stopPropagation();
          drawer.open('wave-release', { warehouse: row.warehouseCode, requests: row.requestId });
        }}
      >
        {next.label}
      </Button>
    );
  }
  return (
    <button
      type="button"
      className="text-[color:var(--ds-text-link-hover)] hover:underline"
      onClick={(event) => {
        event.stopPropagation();
        window.location.hash = `#/${next.moduleId}`;
      }}
    >
      {next.label}
    </button>
  );
}
