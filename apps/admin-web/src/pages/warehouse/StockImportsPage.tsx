import { useMemo, useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayMakeDecision } from '../use-operator-grants';
import { approveStockImport, listStockImports, rejectStockImport, submitStockImport, type StockImportRecord } from './api';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import type { GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { enrichedStockImportForms } from './guided-schemas';
import { stockImportOutcomeLabels, stockImportSnapshotHints } from './presentation';
import { stockImportConditions, stockImportStatusLabel, type StockImportStatusFilter } from './rows';
import { useWarehouseLookups } from './use-catalogues';
import { ImportDetailModal } from './document-modals';
import { WmsBadge } from './wms-ui';

const info = moduleInfoById['stock-imports'];
const submitDecision = 'STOCK_IMPORT_SUBMISSION';
const approvalDecision = 'STOCK_ADJUSTMENT_APPROVAL';
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<StockImportRecord>[] = [
  { id: 'import', header: '导入号', className: mono, render: (row) => row.importId },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={stockImportStatusLabel(row.status)} /> },
  { id: 'submitted', header: '提交时刻', className: mono, render: (row) => formatInstant(row.submittedAt) },
];

const statusFilters: ReadonlyArray<{ id: StockImportStatusFilter; label: string }> = [
  { id: 'PENDING', label: '待批' },
  { id: 'POSTED', label: '已入账' },
  { id: 'REJECTED', label: '已驳回' },
  { id: 'ALL', label: '全部' },
];

/** 建账导入页：单表呈现全部状态，右上角弹窗提交，单击待批行从右栏批准入账或驳回，完成后自动刷新列表。 */
export function StockImportsPage() {
  const maySubmit = useMayMakeDecision(submitDecision);
  const mayDecide = useMayMakeDecision(approvalDecision);
  const lookups = useWarehouseLookups();

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<StockImportStatusFilter>('PENDING');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [opened, setOpened] = useState<StockImportRecord | null>(null);
  const settled = useSettledKeyword(keyword);
  const conditions = stockImportConditions(settled, status);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listStockImports);
  const rows = answer?.kind === 'outcome' ? answer.body.imports : [];

  const targets = useMemo(() => {
    const map = new Map<string, GuidedActionTarget>();
    if (maySubmit) {
      map.set('submit', {
        id: 'submit',
        label: '提交建账导入',
        title: '提交建账导入',
        endpoint: 'POST /warehouse-fulfillment-stock-import-submissions',
        snapshotHint: stockImportSnapshotHints.submit,
        submit: submitStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.submit,
      });
    }
    if (mayDecide) {
      map.set('approve', {
        id: 'approve',
        label: '批准入账',
        title: '批准建账导入并入账',
        endpoint: 'POST /warehouse-fulfillment-stock-import-approvals',
        snapshotHint: stockImportSnapshotHints.approve,
        submit: approveStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.approve,
      });
      map.set('reject', {
        id: 'reject',
        label: '驳回导入',
        title: '驳回建账导入',
        endpoint: 'POST /warehouse-fulfillment-stock-import-rejections',
        snapshotHint: stockImportSnapshotHints.reject,
        submit: rejectStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.reject,
      });
    }
    return map;
  }, [maySubmit, mayDecide]);

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targets.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const emptyDescription = maySubmit
    ? '点右上角「新建建账导入」提交期初库存，批准后才真正入账。'
    : hiddenDecisionHint('提交', '建账导入提交');

  const inspectorOf = (row: StockImportRecord): InspectorContent => {
    const fields = [
      { label: '原因', value: row.reason },
      { label: '商品行 / 整箱行', value: `${row.goodsLines.length} / ${row.cartonLines.length}`, mono: true },
      { label: '提交人', value: row.submitter, mono: true },
    ];
    if (row.poster) fields.push({ label: '批准人', value: row.poster, mono: true });
    if (row.postedAt) fields.push({ label: '入账时刻', value: formatInstant(row.postedAt), mono: true });
    if (row.rejector) fields.push({ label: '驳回人', value: row.rejector, mono: true });
    if (row.rejectionNote) fields.push({ label: '驳回原因', value: row.rejectionNote });

    const actions: InspectorAction[] = [{ label: '查看导入明细', onRun: () => setOpened(row) }];
    if (mayDecide) {
      if (row.status === 'PENDING') {
        actions.push(
          { label: '批准并入账', onRun: () => openAction('approve', { importId: row.importId }) },
          { label: '驳回导入', onRun: () => openAction('reject', { importId: row.importId }) },
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
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<StockImportRecord>
        moduleId="stock-imports"
        title={info.title}
        description="期初建账导入台账。双击看商品行和整箱行；待批单可在明细里批准入账或驳回。"
        headerActions={
          maySubmit ? (
            <Button type="button" className={primaryActionClass} onClick={() => openAction('submit')}>
              <Plus className="h-3.5 w-3.5 mr-1" />
              新建建账导入
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜导入号、货主、仓或原因' }}
        filters={
          <>
            {statusFilters.map((filter) => (
              <button
                key={filter.id}
                type="button"
                className={chipClass(filter.id === status)}
                aria-pressed={filter.id === status}
                onClick={() => setStatus(filter.id)}
              >
                {filter.label}
              </button>
            ))}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.importId}
        inspector={inspectorOf}
        onRowOpen={setOpened}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的建账导入"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-stock-imports',
          emptyTitle: status === 'PENDING' ? '没有待批的建账导入' : '还没有建账导入',
          emptyDescription,
          narrowed: settled.trim() !== '' || (status !== 'PENDING' && status !== 'ALL'),
        })}
      />
      <ImportDetailModal
        row={opened}
        canDecide={mayDecide}
        onClose={() => setOpened(null)}
        onAction={(id, prefill) => {
          setOpened(null);
          openAction(id, prefill);
        }}
      />
      <ActionDrawer
        moduleId="stock-imports"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </>
  );
}
