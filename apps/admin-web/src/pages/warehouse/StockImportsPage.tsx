import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { ActionDrawer, useDrawerOver } from './ActionDrawer';
import { listStockImports, type StockImportRecord } from './api';
import { DocumentLink } from './document-page';
import { stockImportStatusLabel } from './rows';
import { useStockImportTargets } from './stock-import-actions';
import { StockImportDocument } from './StockImportDocument';
import { openDocument, useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const moduleId = 'stock-imports';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一张导入单往下走的先后：待批在最前，它是要人动手的那一队。已入账与已驳回只增不减，数出来的不是待办，不数；
// 「全部」放最后，页面打开就落在待批上。
const queueStatuses = ['PENDING'] as const;
const settledStatuses = ['POSTED', 'REJECTED'] as const;

const newestFirst = '-submittedAt';
const oldestFirst = 'submittedAt';

/** 建账导入：列表与每一张导入单各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function StockImportsPage() {
  const parts = useDocumentParts();
  return parts ? <StockImportDocument importId={parts[0]} /> : <StockImportList />;
}

function StockImportList() {
  const drawer = useDrawerOver(useStockImportTargets());
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('PENDING');
  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [sort, setSort] = useState<string | null>(null);
  const [refreshes, setRefreshes] = useState(0);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (owner) baseFilters.ownerAccount = [owner];
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待批这一队，按提交先后排，等得最久的在最上面；看全部或已了结的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(status);
  const effectiveSort = sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: status === 'ALL' ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listStockImports);
  const rows = answer?.kind === 'outcome' ? answer.body.imports : [];
  const counts = useStatusCounts(listStockImports, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<StockImportRecord>[] = [
    { id: 'import', header: '导入号', className: 'whitespace-nowrap', render: (row) => <DocumentLink moduleId={moduleId} parts={[row.importId]} /> },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={stockImportStatusLabel(row.status)} /> },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'size', header: '商品行 / 整箱行', align: 'right', className: mono, render: (row) => `${row.goodsLines.length} / ${row.cartonLines.length}` },
    { id: 'submitted', header: '提交时刻', className: mono, render: (row) => formatInstant(row.submittedAt) },
    {
      id: 'waiting',
      header: '已等',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只有待批的在等人；已了结的单不写等了多久。
      render: (row) => (row.status === 'PENDING' ? elapsedLabel(row.submittedAt, now) || '—' : '—'),
    },
  ];

  const emptyDescription = drawer.has('submit')
    ? '点右上角「新建建账导入」提交期初库存，批准后才真正入账。'
    : hiddenDecisionHint('提交', '建账导入提交');

  const inspectorOf = (row: StockImportRecord): InspectorContent => {
    const fields: InspectorField[] = [
      { label: '原因', value: row.reason },
      { label: '商品行 / 整箱行', value: `${row.goodsLines.length} / ${row.cartonLines.length}`, mono: true },
      { label: '提交人', value: row.submitter, mono: true },
    ];
    if (row.poster) fields.push({ label: '批准人', value: row.poster, mono: true });
    if (row.postedAt) fields.push({ label: '入账时刻', value: formatInstant(row.postedAt), mono: true });
    if (row.rejector) fields.push({ label: '驳回人', value: row.rejector, mono: true });
    if (row.rejectionNote) fields.push({ label: '驳回原因', value: row.rejectionNote });

    const actions: InspectorAction[] = [{ label: '打开建账导入', onRun: () => openDocument(moduleId, [row.importId]) }];
    if (drawer.has('approve')) {
      if (row.status === 'PENDING') {
        actions.push(
          { label: '批准并入账', onRun: () => drawer.open('approve', { importId: row.importId }) },
          { label: '驳回导入', onRun: () => drawer.open('reject', { importId: row.importId }) },
        );
      } else {
        actions.push({ label: '批准 / 驳回', disabledReason: '只有待批状态的建账导入才能批准或驳回' });
      }
    }

    return {
      title: row.importId,
      subtitle: stockImportStatusLabel(row.status),
      sections: [
        { kind: 'summary', fields },
        { kind: 'status', items: [{ label: '状态', word: stockImportStatusLabel(row.status) }] },
        { kind: 'actions', actions },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<StockImportRecord>
        moduleId={moduleId}
        title={info.title}
        description="点状态看那一队，待批的数量是当前筛选下还有几张。点导入号打开建账导入，核完每一行再批准入账或驳回。"
        headerActions={
          drawer.has('submit') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('submit')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              新建建账导入
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜导入号、货主、仓或原因' }}
        sort={{
          value: effectiveSort,
          onChange: setSort,
          options: [
            { value: newestFirst, label: '最新提交的在前' },
            { value: oldestFirst, label: '等得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={status}
              onChange={setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: stockImportStatusLabel(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: stockImportStatusLabel(code) })),
                { value: 'ALL', label: '全部' },
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
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.importId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.importId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的建账导入"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-stock-imports',
          emptyTitle: status === 'PENDING' ? '没有待批的建账导入' : '还没有建账导入',
          emptyDescription,
          narrowed: settled.trim() !== '' || owner !== '' || warehouse !== '' || (status !== 'PENDING' && status !== 'ALL'),
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
