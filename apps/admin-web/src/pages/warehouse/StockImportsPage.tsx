import { useMemo, useState, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { ActionDrawer, useDrawerOver, type ActionDrawerControl } from './ActionDrawer';
import { listStockImports, type StockImportRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { stockImportStatusLabel } from './rows';
import { useStockImportTargets } from './stock-import-actions';
import { StockImportDocument } from './StockImportDocument';
import { useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'stock-imports';
const info = moduleInfoById[moduleId];
const submittedKey = 'submittedAt';
const pending = 'PENDING';
const allQueues = 'ALL';
const noRows: StockImportRecord[] = [];

// 待批是要人动手的那一队。已入账与已驳回只增不减，数出来的不是待办，不数。页面打开就落在待批上。
const queueStatuses = [pending] as const;
const settledStatuses = ['POSTED', 'REJECTED'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

function knownStatus(value: string): string {
  return (knownStatuses as readonly string[]).includes(value) ? value : pending;
}

/** 读口按提交时刻排。待批先提交的在上面；看全部或已了结的，最新提交的在上面。人点过表头就照他的。 */
function submittedSort(stored: WorkbenchSort | null, inQueue: boolean): WorkbenchSort {
  if (stored?.key === submittedKey) return stored;
  return { key: submittedKey, dir: inQueue ? 1 : -1 };
}

/** 建账导入：列表与每一张导入单各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function StockImportsPage() {
  const parts = useDocumentParts();
  return parts ? <StockImportDocument importId={parts[0]} /> : <StockImportList />;
}

function StockImportList() {
  const drawer = useDrawerOver(useStockImportTargets());
  const [keyword, setKeyword] = useTabViewState(`${moduleId}.keyword`, '');
  const [storedStatus, setStatus] = useTabViewState(`${moduleId}.status`, pending);
  const status = knownStatus(storedStatus);
  const [owner, setOwner] = useTabViewState(`${moduleId}.owner`, '');
  const [warehouse, setWarehouse] = useTabViewState(`${moduleId}.warehouse`, '');
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>(`${moduleId}.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.open`, null);
  const [refreshes, setRefreshes] = useState(0);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (owner) baseFilters.ownerAccount = [owner];
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const inQueue = status === pending;
  const sort = submittedSort(chosenSort, inQueue);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? submittedKey : `-${submittedKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listStockImports);
  const held = heldCataloguePage(answer, previous, (body) => body.imports.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.imports : (held?.body.imports ?? noRows);
  const counts = useStatusCounts(listStockImports, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };
  const now = new Date();

  const columns: WorkbenchColumn<StockImportRecord>[] = [
    {
      key: 'id',
      header: '导入号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[row.importId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={stockImportStatusLabel(row.status)} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 860, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1040, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'reason',
      header: '原因',
      render: (row) => (row.reason ? <WmsTwoLine fill primary={row.reason} title={row.reason} /> : <span className="text-idpxyz-textMuted">没写原因</span>),
    },
    {
      key: 'lines',
      header: nowrap('商品 / 整箱'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.goodsLines.length} / ${row.cartonLines.length}`}</span>,
    },
    {
      key: 'waiting',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums">{row.status === pending ? elapsedLabel(row.submittedAt, now) || '—' : '—'}</span>
      ),
    },
    {
      key: 'submitted',
      header: nowrap('提交'),
      right: true,
      width: fitColumnWidth,
      sortKey: submittedKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.submittedAt)}>
          {shortInstant(row.submittedAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <ApprovalStep row={row} drawer={drawer} /> },
  ];

  const chooseStatus = (next: string) => setStatus(knownStatus(next));
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: stockImportStatusLabel(code),
    count: counts[code] ?? null,
    active: status === code,
    attention: true,
    onClick: () => chooseStatus(status === code ? allQueues : code),
  }));
  const homeQueue = status === pending && chosenSort === null;
  const narrowed = settled.trim() !== '' || owner !== '' || warehouse !== '' || !homeQueue;
  const clearFilters = () => {
    setKeyword('');
    setOwner('');
    setWarehouse('');
    setStatus(pending);
    setChosenSort(null);
  };
  const emptyDescription = drawer.has('submit')
    ? '点右上角「新建建账导入」提交期初库存，批准后才真正入账。'
    : hiddenDecisionHint('提交', '建账导入提交');
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-stock-imports',
        emptyTitle: status === pending ? '没有待批的建账导入' : '还没有建账导入',
        emptyDescription,
        narrowed: settled.trim() !== '' || owner !== '' || warehouse !== '' || (status !== pending && status !== allQueues) || pastFirstPage(pagination),
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<StockImportRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边核这张导入单。提交不改账，批准才入账"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('submit') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('submit')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              新建建账导入
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜导入号、货主、仓或原因' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: stockImportStatusLabel(code) }))}
            />
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
              options={drawer.lookups.warehouses.map((item) => ({
                value: item.warehouseCode,
                label: item.name ? `${item.name} · ${item.warehouseCode}` : item.warehouseCode,
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
        rowKey={(row) => row.importId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          setChosenSort(next);
        }}
        noMatchText="当前筛选下没有建账导入"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <StockImportDocument importId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 待批的那一行只留批准。驳回在右栏，核完每一行再下手。没有批准授予时行上写等批准，不摆一个点了也做不成的钮。 */
function ApprovalStep({ row, drawer }: { row: StockImportRecord; drawer: ActionDrawerControl }) {
  if (row.status !== pending) return <span className="text-idpxyz-textMuted">—</span>;
  if (!drawer.has('approve')) return <span className="whitespace-nowrap text-idpxyz-textMuted">等批准</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open('approve', { importId: row.importId });
      }}
    >
      批准入账
    </Button>
  );
}
