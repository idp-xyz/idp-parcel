import { RefreshCw, TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import type { ReturnReceiptRecord } from './api';
import { DocumentLink } from './document-page';
import { returnReceiptKind, returnsModule as moduleId } from './return-address';
import { ReceiptStatusBadge } from './return-badges';
import {
  awaitsDisposition,
  countByStatus,
  isReturnOrder,
  receiptInFilter,
  returnDispositionText,
  returnReceiptNextStep,
  returnReceiptStatus,
  returnReceiptWaitingSince,
  sortReceipts,
  type ReturnOrder,
  type ReturnReceiptStatus,
} from './return-flow';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import type { ReturnsOverviewState } from './use-returns-overview';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后：待处置是要人动手的一队，排第一；已处置、转待认领各是一个终点。概览整份都在手里，每个签都如实写数。
const statusCodes: readonly ReturnReceiptStatus[] = ['AWAITING_DISPOSITION', 'DISPOSED', 'DIVERTED'];

const goModule = (target: string) => {
  window.location.hash = `#/${target}`;
};

/** 认不出货主的转了待认领到货，到货号读口答了就直接打开那一件，没答就去列表。 */
function goUnclaimed(arrivalId: string | undefined) {
  if (arrivalId) openDocument('warehouse-unclaimed', [arrivalId]);
  else goModule('warehouse-unclaimed');
}

/**
 * 退件到仓与回库处置：退件扫进暂存位后出现在这里。点作业事实号打开退货到仓。概览读口不分页、不接筛选维，状态、数量、货主与仓的
 * 筛选都在已取回的整份概览上做；要看别处刚发生的变动（比如作业台刚收的退件），点刷新重读。
 */
export function ReturnReceiptList({ overview, filters }: { overview: ReturnsOverviewState; filters: QueueFilters }) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const { answer, reload } = overview;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const all = body?.receipts ?? [];
  const loaded = body !== null;

  // 数量是当前检索词、货主与仓之下各状态各有几笔；点哪个签只换看的那一队，不改数量。
  const inScope = all.filter((row) => receiptInFilter(row, filters));
  const counts = countByStatus(inScope, returnReceiptStatus);
  const shown = filters.status === 'ALL' ? inScope : inScope.filter((row) => returnReceiptStatus(row) === filters.status);
  // 选了待处置这一队，按到仓先后排，等得最久的在最上面；看别的，最新的在上面。看全部时没处置的排前。人改过排序就照他的。
  const defaultOrder: ReturnOrder = filters.status === 'AWAITING_DISPOSITION' ? 'oldest' : 'newest';
  const order = isReturnOrder(filters.sort) ? filters.sort : defaultOrder;
  const rows = sortReceipts(shown, order);
  const now = new Date();

  const columns: ListColumn<ReturnReceiptRecord>[] = [
    {
      id: 'fact',
      header: '作业事实号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[returnReceiptKind, row.factId]} />,
    },
    { id: 'disp', header: '回库处置', render: (row) => <ReceiptStatusBadge receipt={row} /> },
    { id: 'track', header: '退件单号', className: mono, render: (row) => row.trackingNumber },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount || '未认出' },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'goods', header: '商品', className: mono, render: (row) => row.goodsCode },
    { id: 'qty', header: '数量', className: mono, render: (row) => `${row.quantity} ${row.unit}` },
    { id: 'state', header: '入库状态', render: (row) => <WmsBadge code={row.stockState} /> },
    { id: 'at', header: '到仓时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
    {
      id: 'waiting',
      header: '已等',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只有待处置的在等人记去向；处置过的、转了待认领的不写等了多久。
      render: (row) => elapsedLabel(returnReceiptWaitingSince(row), now) || '—',
    },
    { id: 'next', header: '下一步', className: 'whitespace-nowrap', render: (row) => <NextStepCell row={row} drawer={drawer} /> },
  ];

  const inspectorOf = (row: ReturnReceiptRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '打开退货到仓', onRun: () => openDocument(moduleId, [returnReceiptKind, row.factId]) }];
    if (awaitsDisposition(row) && drawer.has('return-disposition')) {
      actions.push({ label: '回库处置', onRun: () => drawer.open('return-disposition', { factId: row.factId }) });
    }
    if (row.outcome === 'UNCLAIMED') {
      actions.push({ label: '去待认领到货', onRun: () => goUnclaimed(row.arrivalId) });
    }
    return {
      title: row.trackingNumber || row.factId,
      subtitle: row.goodsCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓 / 库位', value: `${row.warehouseCode} · ${row.locationCode}`, mono: true },
            { label: '数量', value: `${row.quantity} ${row.unit}`, mono: true },
            { label: '货主', value: row.ownerAccount || '未认出', mono: true },
            { label: '处置', value: returnDispositionText(row) },
          ],
        },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '作业事实号', value: row.factId, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<ReturnReceiptRecord>
        moduleId={moduleId}
        title="到仓与回库处置"
        description="退件扫进暂存位后出现在这里，数量是当前筛选下各状态有几笔，待处置的排在前面。还没处置的行可以做良品上架、转次品、销毁或退还货主，处置只记去向、不改库存账；认不出货主的转待认领，认领后另起一笔入库。点作业事实号打开退货到仓。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={reload}>
              <RefreshCw className="mr-1 h-3.5 w-3.5" />
              刷新
            </Button>
            <Button type="button" variant="outline" onClick={() => goModule('warehouse-workstation')}>
              <TabletSmartphone className="mr-1 h-3.5 w-3.5" />
              去作业台扫描到仓
            </Button>
            {drawer.has('return-disposition') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('return-disposition')}>
                代录回库处置
              </Button>
            ) : null}
          </div>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜事实号、退件单号、商品或货主' }}
        sort={{
          value: order,
          onChange: filters.setSort,
          options: [
            { value: 'newest', label: '最新到仓的在前' },
            { value: 'oldest', label: '等得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...statusCodes.map((code) => ({ value: code, label: labelOf(code), count: loaded ? (counts[code] ?? 0) : undefined })),
                { value: 'ALL', label: '全部', count: loaded ? inScope.length : undefined },
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
        rowKey={(row) => row.factId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [returnReceiptKind, row.factId])}
        emptyRowsNote="当前条件下没有退件到仓记录"
        // 概览是整册：整册都没有到仓记录才是空态，筛没了说的是「这组条件下没有」，由 emptyRowsNote 说。
        viewState={catalogueViewState(answer, all.length, reload, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-returns-overview',
          emptyTitle: '还没有退件到仓记录',
          emptyDescription: '在作业台扫描退件到仓后，记录会出现在这里。',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reload} />
    </>
  );
}

/** 每行最右的那一格：这笔到仓记录此刻该做的一步。能就地做的是按钮，没有授予的只说在等什么。 */
function NextStepCell({ row, drawer }: { row: ReturnReceiptRecord; drawer: ActionDrawerControl }) {
  const next = returnReceiptNextStep(returnReceiptStatus(row));
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('return-disposition')) return <span className="text-idpxyz-textMuted">等处置</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只开表单，不顺带把行选进右栏。
        event.stopPropagation();
        drawer.open('return-disposition', { factId: row.factId });
      }}
    >
      {next.label}
    </Button>
  );
}
