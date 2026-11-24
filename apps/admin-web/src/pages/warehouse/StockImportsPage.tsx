import { useMemo, useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayMakeDecision } from '../use-operator-grants';
import { approveStockImport, listStockImports, rejectStockImport, submitStockImport, type StockImportRecord } from './api';
import { GuidedActionPanel, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormSeed, GuidedFormValues } from './guided-forms';
import { stockImportGuidedForms } from './guided-schemas';
import { problemNote, stockImportOutcomeLabels, stockImportSnapshotHints } from './presentation';
import { stockImportConditions, stockImportStatusLabel, type StockImportStatusFilter } from './rows';

const info = moduleInfoById['stock-imports'];

const submitDecision = 'STOCK_IMPORT_SUBMISSION';
const approvalDecision = 'STOCK_ADJUSTMENT_APPROVAL';

const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<StockImportRecord>[] = [
  { id: 'import', header: '导入号', className: mono, render: (row) => row.importId },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'status', header: '状态', render: (row) => stockImportStatusLabel(row.status) },
  { id: 'submitted', header: '提交时刻', className: mono, render: (row) => formatInstant(row.submittedAt) },
];

const statusFilters: ReadonlyArray<{ id: StockImportStatusFilter; label: string }> = [
  { id: 'ALL', label: '全部' },
  { id: 'PENDING', label: '待批' },
  { id: 'POSTED', label: '已入账' },
  { id: 'REJECTED', label: '已驳回' },
];

function ImportTable({
  maySubmit,
  mayDecide,
  onPrefill,
}: {
  maySubmit: boolean;
  mayDecide: boolean;
  onPrefill: (targetId: string, values: GuidedFormValues) => void;
}) {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<StockImportStatusFilter>('PENDING');
  const settled = useSettledKeyword(keyword);
  const conditions = stockImportConditions(settled, status);
  // 打开页就落在待批上：没有待批是这一页的空态，不是筛空。换到已入账或已驳回再答零行，才说当前条件没有。
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listStockImports);
  const rows = answer?.kind === 'outcome' ? answer.body.imports : [];
  const emptyDescription = maySubmit
    ? '在「处理」签里提交后，待批的会在这里出现。批准才入账。'
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

    const actions: InspectorAction[] = [];
    if (mayDecide) {
      if (row.status === 'PENDING') {
        actions.push(
          { label: '批准并入账', onRun: () => onPrefill('approve', { importId: row.importId }) },
          { label: '驳回导入', onRun: () => onPrefill('reject', { importId: row.importId }) },
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
    <ListPageTemplate<StockImportRecord>
      moduleId="stock-imports"
      title={info.title}
      description="已提交的建账导入。单击待批行可从右栏直接带出导入号批准入账或驳回；提交本身不改账。"
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
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的导入"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-stock-imports',
        emptyTitle: status === 'PENDING' ? '没有待批的建账导入' : '还没有建账导入',
        emptyDescription,
        narrowed: settled.trim() !== '' || (status !== 'PENDING' && status !== 'ALL'),
      })}
    />
  );
}

/**
 * 建账导入的查阅，以及引导式提交、批准、驳回。
 * 提交与批准不是同一种授予：没有哪一种，对应的那一册就不摆。
 */
export function StockImportsPage() {
  const maySubmit = useMayMakeDecision(submitDecision);
  const mayDecide = useMayMakeDecision(approvalDecision);
  const [tab, setTab] = useState('list');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);

  const targets = useMemo<GuidedActionTarget[]>(() => {
    const built: GuidedActionTarget[] = [];
    if (maySubmit) {
      built.push({
        id: 'submit',
        label: '提交',
        title: '提交建账导入',
        endpoint: 'POST /warehouse-fulfillment-stock-import-submissions',
        snapshotHint: stockImportSnapshotHints.submit,
        submit: submitStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: stockImportGuidedForms.submit,
      });
    }
    if (mayDecide) {
      built.push(
        {
          id: 'approve',
          label: '批准',
          title: '批准建账导入',
          endpoint: 'POST /warehouse-fulfillment-stock-import-approvals',
          snapshotHint: stockImportSnapshotHints.approve,
          submit: approveStockImport,
          outcomeLabels: stockImportOutcomeLabels,
          form: stockImportGuidedForms.approve,
        },
        {
          id: 'reject',
          label: '驳回',
          title: '驳回建账导入',
          endpoint: 'POST /warehouse-fulfillment-stock-import-rejections',
          snapshotHint: stockImportSnapshotHints.reject,
          submit: rejectStockImport,
          outcomeLabels: stockImportOutcomeLabels,
          form: stockImportGuidedForms.reject,
        },
      );
    }
    return built;
  }, [maySubmit, mayDecide]);

  const openPrefill = (targetId: string, values: GuidedFormValues) => {
    setSeed((current) => ({ targetId, values, nonce: (current?.nonce ?? 0) + 1 }));
    setTab('act');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="list">建账导入</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">处理</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="list" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ImportTable maySubmit={maySubmit} mayDecide={mayDecide} onPrefill={openPrefill} />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <GuidedActionPanel moduleId="stock-imports" targets={targets} problemNote={problemNote} seed={seed} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
