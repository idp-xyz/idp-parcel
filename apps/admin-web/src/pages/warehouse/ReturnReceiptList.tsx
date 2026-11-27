import { useMemo, useState, type ReactNode } from 'react';
import { TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { returnsDefaultPageSize, returnsPageSizes, type ReturnReceiptRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf } from './list-cells';
import { returnReceiptKind, returnsModule as moduleId } from './return-address';
import { ReceiptStatusBadge } from './return-badges';
import { isReturnOrder, returnReceiptNextStep, returnReceiptStatus, returnReceiptWaitingSince, type ReturnReceiptStatus } from './return-flow';
import { ReturnReceiptDocument } from './ReturnReceiptDocument';
import type { QueueFilters } from './use-queue-filters';
import { useReturnsPage } from './use-returns-overview';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsPagerBar, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const occurredKey = 'occurredAt';
const allQueues = 'ALL';
const homeQueue = 'AWAITING_DISPOSITION';

// 签的先后：待处置是要人动手的一队，排第一；已处置、转待认领各是一个终点。数量是这一页的检索、货主、仓之下各状态有几笔。
const statusCodes: readonly ReturnReceiptStatus[] = ['AWAITING_DISPOSITION', 'DISPOSED', 'DIVERTED'];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/**
 * 退件到仓与回库处置：退件扫进暂存位后出现在这里。点一行在右边看这笔到仓，作业事实号仍进整页。列表按页向读口要，
 * 每页多少条在 20、50、100、200 里选；行数多少都留着翻页条。看全部时没处置的仍排在前面。
 */
export function ReturnReceiptList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-returns.receipt.open', null);
  const [storedSize, setPageSize] = useTabViewState('warehouse-returns.receipt.pageSize', returnsDefaultPageSize);
  const pageSize = (returnsPageSizes as readonly number[]).includes(storedSize) ? storedSize : returnsDefaultPageSize;
  // 选了待处置这一队，按到仓先后排，等得最久的在最上面；看别的，最新的在上面。人点过表头就照他的。
  const defaultOrder = filters.status === homeQueue ? 'oldest' : 'newest';
  const order = isReturnOrder(filters.sort) ? filters.sort : defaultOrder;
  const sliceKey = `${filters.keyword}\n${filters.status}\n${filters.owner}\n${filters.warehouse}\n${order}\n${pageSize}`;
  const [paging, setPaging] = useState({ key: sliceKey, page: 1 });
  if (paging.key !== sliceKey) setPaging({ key: sliceKey, page: 1 });
  const page = paging.key === sliceKey ? paging.page : 1;
  const { answer, reload, reloading } = useReturnsPage({
    view: 'receipts',
    page,
    pageSize,
    status: filters.status === allQueues ? '' : filters.status,
    q: filters.keyword.trim(),
    owner: filters.owner,
    warehouse: filters.warehouse,
    sort: order,
  });
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const counts = body?.counts ?? {};
  const rows = body?.receipts ?? [];
  const loaded = body !== null;
  const total = body?.page?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const bookEmpty =
    loaded && total === 0 && statusCodes.every((code) => (counts[code] ?? 0) === 0) && filters.keyword.trim() === '' && filters.owner === '' && filters.warehouse === '';
  const sort: WorkbenchSort = { key: occurredKey, dir: order === 'oldest' ? 1 : -1 };
  const now = new Date();
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

  // 开了详情栏之后列表变窄，次要的列收起：作业事实号、回库处置、已等与下一步一直在。
  const columns: WorkbenchColumn<ReturnReceiptRecord>[] = [
    {
      key: 'fact',
      header: '作业事实号',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={<DocumentLink moduleId={moduleId} parts={[returnReceiptKind, row.factId]} />}
          secondary={row.trackingNumber}
          code
          title={`${row.factId}\n退件单号 ${row.trackingNumber}\n到仓于 ${formatInstant(row.occurredAt)}`}
        />
      ),
    },
    { key: 'disp', header: '回库处置', width: fitColumnWidth, render: (row) => <ReceiptStatusBadge receipt={row} /> },
    {
      key: 'owner',
      header: '货主',
      width: fitColumnWidth,
      minListWidth: 900,
      render: (row) => (row.ownerAccount ? named(ownerNames, row.ownerAccount) : <span className="text-idpxyz-textMuted">未认出</span>),
    },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1060, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'goods',
      header: '商品',
      minListWidth: 760,
      render: (row) => <WmsTwoLine fill primary={<span className="font-mono">{row.goodsCode}</span>} title={row.goodsCode} />,
    },
    {
      key: 'qty',
      header: nowrap('数量'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1100,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.quantity} ${row.unit}`}</span>,
    },
    { key: 'state', header: '入库状态', width: fitColumnWidth, minListWidth: 1160, render: (row) => <WmsBadge code={row.stockState} /> },
    {
      key: 'waiting',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      sortKey: occurredKey,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{elapsedLabel(returnReceiptWaitingSince(row), now) || '—'}</span>,
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <NextStepCell row={row} drawer={drawer} /> },
  ];

  const queueStats: WmsQueueStat[] = statusCodes.map((code) => ({
    id: code,
    label: labelOf(code),
    count: loaded ? (counts[code] ?? 0) : null,
    active: filters.status === code,
    onClick: () => filters.setStatus(filters.status === code ? allQueues : code),
  }));
  const narrowed =
    filters.keyword.trim() !== '' || filters.owner !== '' || filters.warehouse !== '' || filters.status !== homeQueue || filters.sort !== null;
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setStatus(homeQueue);
    filters.setSort(null);
  };
  const [revision, setRevision] = useState(0);
  const refresh = () => {
    reload();
    setRevision((count) => count + 1);
  };
  const viewState: TemplateViewState = catalogueViewState(answer, bookEmpty ? 0 : 1, refresh, {
    module: info,
    endpoint: 'GET /warehouse-fulfillment-returns-overview',
    emptyTitle: '还没有退件到仓记录',
    emptyDescription: '在作业台扫描退件到仓后，记录会出现在这里。',
  });

  return (
    <>
      <WorkbenchPageTemplate<ReturnReceiptRecord>
        title="到仓与回库处置"
        viewStateKey="warehouse-returns-receipts"
        subtitle="点一行在右边看这笔到仓、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={reloading}
        primaryAction={
          <>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
              <TabletSmartphone className="mr-1 h-3.5 w-3.5" />
              去作业台扫描到仓
            </Button>
            {drawer.has('return-disposition') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('return-disposition')}>
                代录回库处置
              </Button>
            ) : null}
          </>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜事实号、退件单号、商品或货主' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={filters.status === allQueues ? '' : filters.status}
              onChange={(next) => filters.setStatus(next === '' ? allQueues : next)}
              options={statusCodes.map((code) => ({ value: code, label: labelOf(code) }))}
            />
            <WmsFilterSelect
              label="货主"
              value={filters.owner}
              onChange={filters.setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={filters.warehouse}
              onChange={filters.setWarehouse}
              options={drawer.lookups.warehouses.map((warehouse) => ({
                value: warehouse.warehouseCode,
                label: warehouse.name ? `${warehouse.name} · ${warehouse.warehouseCode}` : warehouse.warehouseCode,
              }))}
            />
            {narrowed ? (
              <button
                type="button"
                className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
                onClick={clearFilters}
              >
                清除
              </button>
            ) : null}
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
          filters.setSort(next.dir === 1 ? 'oldest' : 'newest');
        }}
        noMatchText="当前筛选下没有退件到仓记录"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <ReturnReceiptDocument factId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision }} />
        )}
        viewState={viewState}
        listFooter={
          viewState.kind === 'ready' ? (
            <WmsPagerBar
              summary={`第 ${page} / ${pageCount} 页 · 共 ${total} 条`}
              canPrevious={page > 1}
              canNext={page < pageCount}
              onPrevious={() => setPaging({ key: sliceKey, page: page - 1 })}
              onNext={() => setPaging({ key: sliceKey, page: page + 1 })}
              pageSize={{
                value: pageSize,
                options: returnsPageSizes,
                onChange: (next) => {
                  setPageSize(next);
                  setPaging({ key: sliceKey, page: 1 });
                },
              }}
            />
          ) : undefined
        }
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这笔到仓记录此刻该做的一步。能就地做的是一个钮，没有授予的只说在等什么。 */
function NextStepCell({ row, drawer }: { row: ReturnReceiptRecord; drawer: ActionDrawerControl }) {
  const next = returnReceiptNextStep(returnReceiptStatus(row));
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('return-disposition')) return <span className="text-idpxyz-textMuted">等处置</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        // 行上的单击是开详情：这一下只开表单，不顺带把详情栏开合一次。
        event.stopPropagation();
        drawer.open('return-disposition', { factId: row.factId });
      }}
    >
      {next.label}
    </Button>
  );
}
