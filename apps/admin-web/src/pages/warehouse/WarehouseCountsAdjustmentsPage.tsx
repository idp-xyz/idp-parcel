import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  getCountDetail,
  listAdjustments,
  listCounts,
  listJobDiscrepancies,
  type JobDiscrepancyRecord,
  type StockAdjustmentRecord,
  type StockCountDetailBody,
  type StockCountRecord,
} from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useWarehouseLookups } from './use-catalogues';
import { labelOf, WmsBadge, WmsDetailModal, WmsInfoGrid, WmsStatusChips, WmsStepper, WmsSubTable } from './wms-ui';

const info = moduleInfoById['warehouse-counts-adjustments'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const countStatuses = ['ALL', 'PENDING', 'COUNTING', 'COUNTED', 'VOID'] as const;
const adjustmentStatuses = ['ALL', 'PENDING', 'POSTED', 'REJECTED'] as const;
const discrepancyStatuses = ['ALL', 'PENDING', 'APPROVED', 'REJECTED'] as const;

const countSteps = [
  { id: 'PENDING', label: '待开始' },
  { id: 'COUNTING', label: '盘点中' },
  { id: 'COUNTED', label: '已出差异' },
] as const;

function conditionsOf(keyword: string, status: string): CatalogueConditions {
  return {
    q: keyword,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
}

/** 盘点、库存调整与作业差异：三张真实台账，双击看明细，待批行上直接批准或驳回。 */
export function WarehouseCountsAdjustmentsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);
  const [tab, setTab] = useState('counts');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);

  const [countKeyword, setCountKeyword] = useState('');
  const [countStatus, setCountStatus] = useState<string>('ALL');
  const [countId, setCountId] = useState<string | null>(null);
  const countSettled = useSettledKeyword(countKeyword);
  const counts = useCatalogueCursor(conditionsOf(countSettled, countStatus), listCounts);
  const countRows = counts.answer?.kind === 'outcome' ? counts.answer.body.counts : [];

  const [adjustmentKeyword, setAdjustmentKeyword] = useState('');
  const [adjustmentStatus, setAdjustmentStatus] = useState<string>('PENDING');
  const [adjustment, setAdjustment] = useState<StockAdjustmentRecord | null>(null);
  const adjustmentSettled = useSettledKeyword(adjustmentKeyword);
  const adjustments = useCatalogueCursor(conditionsOf(adjustmentSettled, adjustmentStatus), listAdjustments);
  const adjustmentRows = adjustments.answer?.kind === 'outcome' ? adjustments.answer.body.adjustments : [];

  const [jobKeyword, setJobKeyword] = useState('');
  const [jobStatus, setJobStatus] = useState<string>('PENDING');
  const [job, setJob] = useState<JobDiscrepancyRecord | null>(null);
  const jobSettled = useSettledKeyword(jobKeyword);
  const jobs = useCatalogueCursor(conditionsOf(jobSettled, jobStatus), listJobDiscrepancies);
  const jobRows = jobs.answer?.kind === 'outcome' ? jobs.answer.body.discrepancies : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setCountId(null);
    setAdjustment(null);
    setJob(null);
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const refreshAll = () => {
    counts.retry();
    adjustments.retry();
    jobs.retry();
  };

  const countColumns: ListColumn<StockCountRecord>[] = [
    { id: 'id', header: '盘点单号', className: mono, render: (row) => row.countId },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'scope', header: '库位 / 商品', className: mono, render: (row) => `${row.locations.length} / ${row.goods.length || '全仓'}` },
    { id: 'diff', header: '待批差异', className: mono, render: (row) => String(row.pendingDifferences) },
    { id: 'at', header: '创建时刻', className: mono, render: (row) => formatInstant(row.createdAt) },
  ];

  const adjustmentColumns: ListColumn<StockAdjustmentRecord>[] = [
    { id: 'id', header: '调整单号', className: mono, render: (row) => row.adjustmentId },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'lines', header: '行数', className: mono, render: (row) => String(row.lines?.length ?? 0) },
    { id: 'reason', header: '原因', render: (row) => row.reason },
    { id: 'at', header: '提交时刻', className: mono, render: (row) => formatInstant(row.submittedAt) },
  ];

  const jobColumns: ListColumn<JobDiscrepancyRecord>[] = [
    { id: 'id', header: '差异号', className: mono, render: (row) => row.discrepancyId },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'goods', header: '商品', className: mono, render: (row) => row.goodsCode || row.cartonNumber || '—' },
    { id: 'qty', header: '数量', className: mono, render: (row) => String(row.quantity) },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    {
      id: 'signal',
      header: '信号',
      render: (row) => {
        const bits = [
          row.shortOnBook > 0 ? `账面短少 ${row.shortOnBook}` : '',
          row.heldQuantity > 0 ? `占用 ${row.heldQuantity}` : '',
          row.cartonMissing ? '整箱缺失' : '',
        ].filter(Boolean);
        return bits.join(' · ') || '—';
      },
    },
  ];

  const countInspector = (row: StockCountRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看盘点明细', onRun: () => setCountId(row.countId) }];
    if (row.status === 'PENDING' && targetMap.has('count-start')) {
      actions.push({ label: '开始盘点', onRun: () => openAction('count-start', { countId: row.countId }) });
    }
    if ((row.status === 'PENDING' || row.status === 'COUNTING') && targetMap.has('count-void')) {
      actions.push({ label: '作废盘点', onRun: () => openAction('count-void', { countId: row.countId }) });
    }
    if (row.status === 'COUNTING' && targetMap.has('count-complete')) {
      actions.push({ label: '结束并出差异', onRun: () => openAction('count-complete', { countId: row.countId }) });
    }
    return {
      title: row.countId,
      subtitle: row.warehouseCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '状态', value: labelOf(row.status) },
            { label: '库位数', value: String(row.locations.length), mono: true },
            { label: '清点笔数', value: String(row.entryCount), mono: true },
            { label: '待批差异', value: String(row.pendingDifferences), mono: true },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  const adjustmentInspector = (row: StockAdjustmentRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看调整明细', onRun: () => setAdjustment(row) }];
    if (row.status === 'PENDING' && targetMap.has('adjustment-approve')) {
      actions.push({ label: '批准入账', onRun: () => openAction('adjustment-approve', { adjustmentId: row.adjustmentId }) });
    }
    if (row.status === 'PENDING' && targetMap.has('adjustment-reject')) {
      actions.push({ label: '驳回', onRun: () => openAction('adjustment-reject', { adjustmentId: row.adjustmentId }) });
    }
    return {
      title: row.adjustmentId,
      subtitle: row.reason,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '状态', value: labelOf(row.status) },
            { label: '行数', value: String(row.lines?.length ?? 0), mono: true },
            { label: '提交人', value: row.operator, mono: true },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  const jobInspector = (row: JobDiscrepancyRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看差异明细', onRun: () => setJob(row) }];
    if (row.status === 'PENDING' && targetMap.has('job-discrepancy-approve')) {
      actions.push({ label: '批准入账', onRun: () => openAction('job-discrepancy-approve', { discrepancyId: row.discrepancyId }) });
    }
    if (row.status === 'PENDING' && targetMap.has('job-discrepancy-reject')) {
      actions.push({ label: '驳回', onRun: () => openAction('job-discrepancy-reject', { discrepancyId: row.discrepancyId }) });
    }
    return {
      title: row.discrepancyId,
      subtitle: row.factId,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓 / 库位', value: `${row.warehouseCode} · ${row.locationCode || '—'}`, mono: true },
            { label: '商品', value: row.goodsCode || '—', mono: true },
            { label: '数量', value: String(row.quantity), mono: true },
            { label: '状态', value: labelOf(row.status) },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="counts">盘点任务</TabsTrigger>
          <TabsTrigger value="adjustments">库存调整</TabsTrigger>
          <TabsTrigger value="job-diffs">作业差异审批</TabsTrigger>
        </TabsList>

        <TabsContent value="counts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<StockCountRecord>
            moduleId="warehouse-counts-adjustments"
            title="盘点任务"
            description="创建后开始锁定范围，到作业台录入清点数，结束时出账实差异。双击一行看清点笔与待批差异。"
            headerActions={
              targetMap.has('count-create') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('count-create')}>
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  新建盘点任务
                </Button>
              ) : undefined
            }
            search={{ value: countKeyword, onChange: setCountKeyword, placeholder: '搜盘点单号、仓或登记人' }}
            filters={<WmsStatusChips value={countStatus} options={countStatuses} onChange={setCountStatus} />}
            columns={countColumns}
            rows={countRows}
            rowKey={(row) => row.countId}
            inspector={countInspector}
            onRowOpen={(row) => setCountId(row.countId)}
            pagination={counts.pagination}
            emptyRowsNote="当前条件下没有盘点任务"
            viewState={catalogueViewState(counts.answer, countRows.length, counts.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-counts',
              emptyTitle: '还没有盘点任务',
              emptyDescription: '点右上角新建盘点，选定仓库与库位后即可下发。',
              narrowed: countSettled.trim() !== '' || countStatus !== 'ALL',
            })}
          />
        </TabsContent>

        <TabsContent value="adjustments" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<StockAdjustmentRecord>
            moduleId="warehouse-counts-adjustments"
            title="库存调整"
            description="盘盈、盘亏与销毁申请。提交不改账；待批行可直接批准入账或驳回。双击看每一行的库位与数量。"
            headerActions={
              targetMap.has('adjustment-submit') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('adjustment-submit')}>
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  提交库存调整
                </Button>
              ) : undefined
            }
            search={{ value: adjustmentKeyword, onChange: setAdjustmentKeyword, placeholder: '搜调整单号、货主、仓或原因' }}
            filters={<WmsStatusChips value={adjustmentStatus} options={adjustmentStatuses} onChange={setAdjustmentStatus} />}
            columns={adjustmentColumns}
            rows={adjustmentRows}
            rowKey={(row) => row.adjustmentId}
            inspector={adjustmentInspector}
            onRowOpen={setAdjustment}
            pagination={adjustments.pagination}
            emptyRowsNote="当前条件下没有库存调整"
            viewState={catalogueViewState(adjustments.answer, adjustmentRows.length, adjustments.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-adjustments',
              emptyTitle: adjustmentStatus === 'PENDING' ? '没有待批的库存调整' : '还没有库存调整',
              emptyDescription: '点右上角提交调整。批准人受「仓配规则设置」里的批准职责约束。',
              narrowed: adjustmentSettled.trim() !== '' || adjustmentStatus !== 'PENDING',
            })}
          />
        </TabsContent>

        <TabsContent value="job-diffs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<JobDiscrepancyRecord>
            moduleId="warehouse-counts-adjustments"
            title="作业差异审批"
            description="上架、移库、形态转换与短少上报留下的账面差异。待批行可直接批准入账或驳回。"
            search={{ value: jobKeyword, onChange: setJobKeyword, placeholder: '搜差异号、事实号、仓或商品' }}
            filters={<WmsStatusChips value={jobStatus} options={discrepancyStatuses} onChange={setJobStatus} />}
            columns={jobColumns}
            rows={jobRows}
            rowKey={(row) => row.discrepancyId}
            inspector={jobInspector}
            onRowOpen={setJob}
            pagination={jobs.pagination}
            emptyRowsNote="当前条件下没有作业差异"
            viewState={catalogueViewState(jobs.answer, jobRows.length, jobs.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-job-discrepancies',
              emptyTitle: jobStatus === 'PENDING' ? '没有待批的作业差异' : '还没有作业差异',
              emptyDescription: '一线在作业台报短少或库位对不上时，差异会出现在这里等待审批。',
              narrowed: jobSettled.trim() !== '' || jobStatus !== 'PENDING',
            })}
          />
        </TabsContent>
      </Tabs>

      <CountDetailModal
        countId={countId}
        canStart={targetMap.has('count-start')}
        canVoid={targetMap.has('count-void')}
        canComplete={targetMap.has('count-complete')}
        canApprove={targetMap.has('count-approve')}
        canReject={targetMap.has('count-reject')}
        onClose={() => setCountId(null)}
        onAction={openAction}
      />
      <AdjustmentDetailModal
        row={adjustment}
        canApprove={targetMap.has('adjustment-approve')}
        canReject={targetMap.has('adjustment-reject')}
        onClose={() => setAdjustment(null)}
        onAction={openAction}
      />
      <JobDetailModal
        row={job}
        canApprove={targetMap.has('job-discrepancy-approve')}
        canReject={targetMap.has('job-discrepancy-reject')}
        onClose={() => setJob(null)}
        onAction={openAction}
      />
      <ActionDrawer
        moduleId="warehouse-counts-adjustments"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={refreshAll}
      />
    </div>
  );
}

function CountDetailModal({
  countId,
  canStart,
  canVoid,
  canComplete,
  canApprove,
  canReject,
  onClose,
  onAction,
}: {
  countId: string | null;
  canStart: boolean;
  canVoid: boolean;
  canComplete: boolean;
  canApprove: boolean;
  canReject: boolean;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
}) {
  const { answer, loading } = useKeyedLoad(countId, getCountDetail);
  const failure = readFailureMessage(answer);
  const body: StockCountDetailBody | undefined = answer?.kind === 'outcome' ? answer.body : undefined;
  const count = body?.count;
  const missing = body && !count ? '没有这张盘点单' : null;
  const entries = (body?.entries ?? []).flatMap((entry) =>
    (entry.lines.length > 0 ? entry.lines : [{ quantity: 0 }]).map((line, index) => ({
      key: `${entry.factId}-${index}`,
      locationCode: entry.locationCode,
      occurredAt: entry.occurredAt,
      operatorId: entry.operatorId,
      goodsCode: 'goodsCode' in line ? line.goodsCode : undefined,
      unit: 'unit' in line ? line.unit : undefined,
      stockState: 'stockState' in line ? line.stockState : undefined,
      quantity: line.quantity,
    })),
  );

  return (
    <WmsDetailModal
      open={countId !== null}
      title={count?.countId ?? countId ?? ''}
      subtitle={count ? `${count.warehouseCode} · ${count.operator}` : undefined}
      badge={count ? <WmsBadge code={count.status} /> : undefined}
      loading={loading}
      error={failure ?? missing}
      stepper={
        count ? (
          <WmsStepper
            steps={countSteps}
            currentId={count.status === 'VOID' ? 'PENDING' : count.status}
            failedLabel={count.status === 'VOID' ? '已作废' : undefined}
          />
        ) : undefined
      }
      actions={
        count ? (
          <>
            {count.status === 'PENDING' && canStart ? (
              <Button type="button" className={primaryActionClass} onClick={() => onAction('count-start', { countId: count.countId })}>
                开始盘点
              </Button>
            ) : null}
            {count.status === 'COUNTING' ? (
              <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
                去作业台录数
              </Button>
            ) : null}
            {count.status === 'COUNTING' && canComplete ? (
              <Button type="button" className={primaryActionClass} onClick={() => onAction('count-complete', { countId: count.countId })}>
                结束并出差异
              </Button>
            ) : null}
            {(count.status === 'PENDING' || count.status === 'COUNTING') && canVoid ? (
              <Button type="button" variant="outline" onClick={() => onAction('count-void', { countId: count.countId })}>
                作废
              </Button>
            ) : null}
          </>
        ) : undefined
      }
      onClose={onClose}
    >
      {count ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '仓库', value: count.warehouseCode, mono: true },
              { label: '货主范围', value: count.ownerAccount || '不限货主', mono: true },
              { label: '库位', value: count.locations.join('、') || '—', mono: true },
              { label: '商品范围', value: count.goods.join('、') || '范围内全部商品', mono: true },
              { label: '清点笔数', value: String(count.entryCount), mono: true },
              { label: '差异 / 待批', value: `${count.differenceCount} / ${count.pendingDifferences}`, mono: true },
              { label: '创建', value: `${count.operator} · ${formatInstant(count.createdAt)}` },
              { label: '备注', value: count.note || '—' },
            ]}
          />
          <WmsSubTable
            title="清点录入"
            count={entries.length}
            emptyText="还没有清点录入。开始盘点后到作业台按库位录入实数。"
            rows={entries}
            rowKey={(row) => row.key}
            columns={[
              { id: 'loc', header: '库位', mono: true, render: (row) => row.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (row) => row.goodsCode || '—' },
              { id: 'state', header: '状态', render: (row) => (row.stockState ? <WmsBadge code={row.stockState} /> : '—') },
              { id: 'qty', header: '实数', align: 'right', mono: true, render: (row) => String(row.quantity) },
              { id: 'who', header: '录入人', mono: true, render: (row) => row.operatorId },
              { id: 'at', header: '时刻', mono: true, render: (row) => formatInstant(row.occurredAt) },
            ]}
          />
          <WmsSubTable
            title="账实差异"
            count={body?.differences?.length ?? 0}
            emptyText="结束盘点后，账与实数不一致的格子会出现在这里。"
            rows={body?.differences ?? []}
            rowKey={(row) => row.differenceId}
            columns={[
              { id: 'id', header: '差异号', mono: true, render: (row) => row.differenceId },
              { id: 'loc', header: '库位', mono: true, render: (row) => row.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (row) => row.goodsCode },
              { id: 'book', header: '账面', align: 'right', mono: true, render: (row) => String(row.bookQuantity) },
              { id: 'counted', header: '实盘', align: 'right', mono: true, render: (row) => String(row.countedQuantity) },
              { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
              {
                id: 'act',
                header: '',
                render: (row) =>
                  row.status === 'PENDING' ? (
                    <span className="inline-flex gap-2">
                      {canApprove ? (
                        <button type="button" className="type-caption text-idpxyz-accent" onClick={() => onAction('count-approve', { differenceId: row.differenceId })}>
                          批准
                        </button>
                      ) : null}
                      {canReject ? (
                        <button type="button" className="type-caption text-rose-700 dark:text-rose-300" onClick={() => onAction('count-reject', { differenceId: row.differenceId })}>
                          驳回
                        </button>
                      ) : null}
                    </span>
                  ) : (
                    row.approvalNote || row.rejectReason || '—'
                  ),
              },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

function AdjustmentDetailModal({
  row,
  canApprove,
  canReject,
  onClose,
  onAction,
}: {
  row: StockAdjustmentRecord | null;
  canApprove: boolean;
  canReject: boolean;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
}) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.adjustmentId ?? ''}
      subtitle={row?.reason}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      actions={
        row && row.status === 'PENDING' ? (
          <>
            {canApprove ? (
              <Button type="button" className={primaryActionClass} onClick={() => onAction('adjustment-approve', { adjustmentId: row.adjustmentId })}>
                批准入账
              </Button>
            ) : null}
            {canReject ? (
              <Button type="button" variant="outline" onClick={() => onAction('adjustment-reject', { adjustmentId: row.adjustmentId })}>
                驳回
              </Button>
            ) : null}
          </>
        ) : undefined
      }
      onClose={onClose}
    >
      {row ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '货主', value: row.ownerAccount, mono: true },
              { label: '仓库', value: row.warehouseCode, mono: true },
              { label: '提交人', value: row.operator, mono: true },
              { label: '提交时刻', value: formatInstant(row.submittedAt), mono: true },
              { label: '批准人', value: row.approvedBy || '—' },
              { label: '驳回原因', value: row.rejectReason || '—' },
            ]}
          />
          <WmsSubTable
            title="调整行"
            count={row.lines?.length ?? 0}
            emptyText="这张调整单没有行。"
            rows={row.lines ?? []}
            rowKey={(line) => String(line.lineIndex)}
            columns={[
              { id: 'kind', header: '种类', render: (line) => labelOf(line.kind) },
              { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'state', header: '状态', render: (line) => <WmsBadge code={line.stockState} /> },
              { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
              { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

function JobDetailModal({
  row,
  canApprove,
  canReject,
  onClose,
  onAction,
}: {
  row: JobDiscrepancyRecord | null;
  canApprove: boolean;
  canReject: boolean;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
}) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.discrepancyId ?? ''}
      subtitle={row ? `作业事实 ${row.factId}` : undefined}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      actions={
        row && row.status === 'PENDING' ? (
          <>
            {canApprove ? (
              <Button type="button" className={primaryActionClass} onClick={() => onAction('job-discrepancy-approve', { discrepancyId: row.discrepancyId })}>
                批准入账
              </Button>
            ) : null}
            {canReject ? (
              <Button type="button" variant="outline" onClick={() => onAction('job-discrepancy-reject', { discrepancyId: row.discrepancyId })}>
                驳回
              </Button>
            ) : null}
          </>
        ) : undefined
      }
      onClose={onClose}
    >
      {row ? (
        <WmsInfoGrid
          items={[
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '仓库', value: row.warehouseCode, mono: true },
            { label: '来源库位', value: row.sourceLocation || '—', mono: true },
            { label: '所在库位', value: row.locationCode || '—', mono: true },
            { label: '商品', value: row.goodsCode || '—', mono: true },
            { label: '单位 / 状态', value: `${row.unit || '—'} · ${labelOf(row.stockState)}` },
            { label: '数量', value: String(row.quantity), mono: true },
            { label: '账面短少', value: String(row.shortOnBook), mono: true },
            { label: '占用', value: String(row.heldQuantity), mono: true },
            { label: '整箱', value: row.cartonNumber || (row.cartonMissing ? '整箱缺失' : '—'), mono: true },
            { label: '批号', value: row.batchNumber || '—', mono: true },
            { label: '说明', value: row.note || row.approvalNote || row.rejectReason || '—' },
          ]}
        />
      ) : null}
    </WmsDetailModal>
  );
}
