import { useMemo, useState } from 'react';
import { Plus, Send } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listOutboundRequests, type OutboundRequestRecord } from './api';
import { DocumentLink } from './document-page';
import { linesSummary, nameOf, packColumns, shortInstant, wmsSurfaceClass } from './list-cells';
import { OutboundDocument } from './OutboundDocument';
import { outboundNextStep, outboundSettled } from './outbound-next-step';
import { openDocument, useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueTabs, WmsTwoLine } from './wms-ui';

const moduleId = 'warehouse-outbound';
const info = moduleInfoById[moduleId];

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

  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((w) => [w.warehouseCode, w.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  // 人认名称，编码放在下面那一行；名册里没有名称的只摆编码。
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="font-mono">{code}</span>;
  };

  const columns = packColumns<OutboundRequestRecord>([
    {
      id: 'id',
      header: '出库单',
      render: (row) => (
        <WmsTwoLine
          primary={
            <DocumentLink moduleId={moduleId} parts={[row.requestId]}>
              {row.documentNumber ?? row.requestId}
            </DocumentLink>
          }
          secondary={`${labelOf(row.origin)} · ${shortInstant(row.receivedAt, now)}`}
          title={`受理于 ${formatInstant(row.receivedAt)}`}
        />
      ),
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'owner', header: '货主', render: (row) => named(ownerNames, row.ownerAccount) },
    { id: 'warehouse', header: '仓库', render: (row) => named(warehouseNames, row.warehouseCode) },
    { id: 'dispatch', header: '发运', render: (row) => labelOf(row.dispatch) },
    {
      id: 'recipient',
      header: '收件人',
      render: (row) => <WmsTwoLine fill primary={row.recipient} secondary={row.address} title={`${row.recipient}\n${row.address}`} />,
    },
    {
      id: 'lines',
      header: '商品',
      render: (row) => {
        const summary = linesSummary(row.lines);
        if (summary.first === '') return <span className="text-idpxyz-textMuted">没有商品行</span>;
        return <WmsTwoLine fill primary={<span className="font-mono">{summary.first}</span>} secondary={summary.rest} title={summary.all} />;
      },
    },
    {
      id: 'qty',
      header: '件数',
      align: 'right',
      // 只在卡在预占的两种状态上说占到多少：拣出去的那部分预占已经放开，拣完之后的 0 不是缺货。
      render: (row) => {
        const wanted = row.lines.reduce((sum, line) => sum + line.quantity, 0);
        const stuck = row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY';
        return (
          <div className="leading-tight">
            <div className="tabular-nums">{wanted}</div>
            {stuck ? <div className="mt-0.5 type-caption text-[color:var(--ds-status-warning-fg)]">只占到 {row.reservedQuantity}</div> : null}
          </div>
        );
      },
    },
    {
      id: 'age',
      header: '已等',
      align: 'right',
      className: 'tabular-nums',
      // 已了结的单不写等了多久：它不在等谁。受理的时刻在单号下面那一行。
      render: (row) => (outboundSettled(row.status) ? <span className="text-idpxyz-textMuted">—</span> : elapsedLabel(row.receivedAt, now) || '—'),
    },
    {
      id: 'next',
      header: '下一步',
      render: (row) => <NextStepCell row={row} agentWarehouse={agentWarehouses.has(row.warehouseCode)} drawer={drawer} />,
    },
  ], ['recipient', 'lines']);

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
            { label: '货主', value: [nameOf(ownerNames, row.ownerAccount), row.ownerAccount].filter(Boolean).join(' · ') },
            { label: '仓库', value: [nameOf(warehouseNames, row.warehouseCode), row.warehouseCode].filter(Boolean).join(' · ') },
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
        tabs={
          <WmsQueueTabs
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
        }
        filters={
          <div className="flex items-center gap-3">
            <WmsFilterSelect
              label="货主"
              value={owner}
              onChange={setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: w.name ? `${w.name} · ${w.warehouseCode}` : w.warehouseCode }))}
            />
            <WmsFilterSelect
              label="发运"
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
        surfaceClassName={wmsSurfaceClass}
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

/** 每行末尾的那一格：这一单此刻该做的一步。能做的是一个钮，等人定的只说在等什么。 */
function NextStepCell({
  row,
  agentWarehouse,
  drawer,
}: {
  row: OutboundRequestRecord;
  agentWarehouse: boolean;
  drawer: ActionDrawerControl;
}) {
  const next = outboundNextStep(row.status, agentWarehouse, undefined, row.requestId);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (next.kind === 'wait') return <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{next.label}</span>;
  if (next.kind === 'wave' && !drawer.has('wave-release')) return <span className="text-idpxyz-textMuted">等放波次</span>;
  // 能做的一步在每一行上是同一个样子的钮，不论它是就地开表单还是去别的页：操作者要认的是「这一单现在该做什么」，
  // 不是这一步落在哪里。
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      // 字用链接色：表里除了状态徽章只有能点的东西带颜色，灰描边灰字的钮在一排灰字里找不出来。
      className="text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        // 行上的单击是预览：这一下只做这一步，不顺带把行选进右栏。
        event.stopPropagation();
        if (next.kind === 'wave') drawer.open('wave-release', { warehouse: row.warehouseCode, requests: row.requestId });
        else if (next.kind === 'station') enterStation(next.hash);
        else window.location.hash = `#/${next.moduleId}`;
      }}
    >
      {next.label}
    </Button>
  );
}
