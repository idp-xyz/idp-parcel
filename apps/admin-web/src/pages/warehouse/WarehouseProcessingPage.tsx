import { useState } from 'react';
import { Plus, TabletSmartphone } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listFreezes,
  listInstructions,
  listInternalJobs,
  type InstructionRecord,
  type InternalJobRecord,
  type StockFreezeRecord,
} from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { labelOf, WmsBadge, WmsDetailModal, WmsInfoGrid, WmsStatusChips, WmsStepper, WmsSubTable } from './wms-ui';

const info = moduleInfoById['warehouse-processing'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const instructionStatuses = ['ALL', 'PENDING', 'DONE', 'VOID'] as const;
const freezeStatuses = ['ALL', 'FROZEN', 'UNFROZEN'] as const;
const instructionSteps = [
  { id: 'PENDING', label: '待执行' },
  { id: 'DONE', label: '已完成' },
] as const;

function conditionsOf(keyword: string, status: string): CatalogueConditions {
  return {
    q: keyword,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
}

function goWorkstation() {
  window.location.hash = '#/warehouse-workstation';
}

/** 加工换标、冻结与库内作业：指令和冻结是台账，执行仍去作业台。 */
export function WarehouseProcessingPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);
  const [tab, setTab] = useState('instructions');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);

  const [instructionKeyword, setInstructionKeyword] = useState('');
  const [instructionStatus, setInstructionStatus] = useState<string>('PENDING');
  const [instruction, setInstruction] = useState<InstructionRecord | null>(null);
  const instructionSettled = useSettledKeyword(instructionKeyword);
  const instructions = useCatalogueCursor(conditionsOf(instructionSettled, instructionStatus), listInstructions);
  const instructionRows = instructions.answer?.kind === 'outcome' ? instructions.answer.body.instructions : [];

  const [freezeKeyword, setFreezeKeyword] = useState('');
  const [freezeStatus, setFreezeStatus] = useState<string>('FROZEN');
  const [freeze, setFreeze] = useState<StockFreezeRecord | null>(null);
  const freezeSettled = useSettledKeyword(freezeKeyword);
  const freezes = useCatalogueCursor(conditionsOf(freezeSettled, freezeStatus), listFreezes);
  const freezeRows = freezes.answer?.kind === 'outcome' ? freezes.answer.body.freezes : [];

  const [jobKeyword, setJobKeyword] = useState('');
  const [job, setJob] = useState<InternalJobRecord | null>(null);
  const jobSettled = useSettledKeyword(jobKeyword);
  const jobs = useCatalogueCursor({ q: jobSettled }, listInternalJobs);
  const jobRows = jobs.answer?.kind === 'outcome' ? jobs.answer.body.jobs : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setInstruction(null);
    setFreeze(null);
    setJob(null);
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const refreshAll = () => {
    instructions.retry();
    freezes.retry();
    jobs.retry();
  };

  const voidInstruction = (row: InstructionRecord) => {
    const id = row.kind === 'RELABEL' ? 'relabel-void' : 'processing-void';
    openAction(id, { instructionId: row.instructionId });
  };

  const instructionColumns: ListColumn<InstructionRecord>[] = [
    { id: 'id', header: '指令号', className: mono, render: (row) => row.instructionId },
    { id: 'kind', header: '种类', render: (row) => labelOf(row.kind) },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'goods', header: '成品 / 目标', className: mono, render: (row) => row.goodsCode },
    { id: 'progress', header: '完成 / 计划', className: mono, render: (row) => `${row.doneQuantity} / ${row.quantity}` },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
  ];

  const freezeColumns: ListColumn<StockFreezeRecord>[] = [
    { id: 'id', header: '冻结号', className: mono, render: (row) => row.freezeId },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'target', header: '对象', render: (row) => labelOf(row.target) },
    { id: 'goods', header: '商品 / 箱号', className: mono, render: (row) => row.goodsCode || row.cartonNumber || '—' },
    { id: 'qty', header: '数量', className: mono, render: (row) => String(row.quantity) },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'at', header: '冻结时刻', className: mono, render: (row) => formatInstant(row.frozenAt) },
  ];

  const jobColumns: ListColumn<InternalJobRecord>[] = [
    { id: 'fact', header: '事实号', className: mono, render: (row) => row.factId },
    { id: 'kind', header: '作业种类', render: (row) => labelOf(row.kind) },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouse },
    { id: 'lines', header: '明细行', className: mono, render: (row) => String(row.lines.length) },
    {
      id: 'signal',
      header: '差异信号',
      render: (row) => {
        const issues = row.lines.filter((line) => line.shortOnBook > 0 || line.held > 0 || line.cartonMissing).length;
        return issues > 0 ? `${issues} 行带差异` : '正常';
      },
    },
    { id: 'occurred', header: '发生时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
  ];

  const instructionInspector = (row: InstructionRecord): InspectorContent => {
    const actions: InspectorAction[] = [
      { label: '查看指令明细', onRun: () => setInstruction(row) },
      { label: '去作业台执行', onRun: goWorkstation },
    ];
    if (row.status === 'PENDING') {
      const voidId = row.kind === 'RELABEL' ? 'relabel-void' : 'processing-void';
      if (targetMap.has(voidId)) actions.push({ label: '作废指令', onRun: () => voidInstruction(row) });
    }
    return {
      title: row.instructionId,
      subtitle: labelOf(row.kind),
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '商品', value: row.goodsCode, mono: true },
            { label: '完成 / 计划', value: `${row.doneQuantity} / ${row.quantity}`, mono: true },
            { label: '状态', value: labelOf(row.status) },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  const freezeInspector = (row: StockFreezeRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看冻结明细', onRun: () => setFreeze(row) }];
    if (row.status === 'FROZEN' && targetMap.has('stock-unfreeze')) {
      actions.push({ label: '解冻', onRun: () => openAction('stock-unfreeze', { freezeId: row.freezeId }) });
    }
    return {
      title: row.freezeId,
      subtitle: row.reason,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓 / 库位', value: `${row.warehouseCode} · ${row.locationCode || '—'}`, mono: true },
            { label: '对象', value: row.goodsCode || row.cartonNumber || '—', mono: true },
            { label: '数量', value: String(row.quantity), mono: true },
            { label: '状态', value: labelOf(row.status) },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  const jobInspector = (row: InternalJobRecord): InspectorContent => ({
    title: row.factId,
    subtitle: labelOf(row.kind),
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '仓库', value: row.warehouse, mono: true },
          { label: '行数', value: String(row.lines.length), mono: true },
          { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
          { label: '设备序号', value: `#${row.deviceSequence}`, mono: true },
        ],
      },
      { kind: 'actions', actions: [{ label: '查看作业明细', onRun: () => setJob(row) }] },
    ],
  });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="instructions">加工与换标指令</TabsTrigger>
          <TabsTrigger value="freeze">冻结与解冻</TabsTrigger>
          <TabsTrigger value="jobs">库内作业台账</TabsTrigger>
        </TabsList>

        <TabsContent value="instructions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<InstructionRecord>
            moduleId="warehouse-processing"
            title="加工与换标指令"
            description="下发组装、拆解或换标。现场执行在作业台完成，这里看进度、组件和已执行记录。双击打开明细。"
            headerActions={
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" onClick={goWorkstation}>
                  <TabletSmartphone className="h-3.5 w-3.5 mr-1" />
                  去作业台执行
                </Button>
                {targetMap.has('processing-instruction') ? (
                  <Button type="button" variant="outline" onClick={() => openAction('processing-instruction')}>
                    <Plus className="h-3.5 w-3.5 mr-1" />
                    下发加工
                  </Button>
                ) : null}
                {targetMap.has('relabel-instruction') ? (
                  <Button type="button" className={primaryActionClass} onClick={() => openAction('relabel-instruction')}>
                    <Plus className="h-3.5 w-3.5 mr-1" />
                    下发换标
                  </Button>
                ) : null}
              </div>
            }
            search={{ value: instructionKeyword, onChange: setInstructionKeyword, placeholder: '搜指令号、货主、仓或商品' }}
            filters={<WmsStatusChips value={instructionStatus} options={instructionStatuses} onChange={setInstructionStatus} />}
            columns={instructionColumns}
            rows={instructionRows}
            rowKey={(row) => row.instructionId}
            inspector={instructionInspector}
            onRowOpen={setInstruction}
            pagination={instructions.pagination}
            emptyRowsNote="当前条件下没有加工或换标指令"
            viewState={catalogueViewState(instructions.answer, instructionRows.length, instructions.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-instructions',
              emptyTitle: instructionStatus === 'PENDING' ? '没有待执行的指令' : '还没有加工或换标指令',
              emptyDescription: '点右上角下发加工或换标。执行请到仓内作业台。',
              narrowed: instructionSettled.trim() !== '' || instructionStatus !== 'PENDING',
            })}
          />
        </TabsContent>

        <TabsContent value="freeze" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<StockFreezeRecord>
            moduleId="warehouse-processing"
            title="冻结与解冻"
            description="冻结中的商品格或整箱不能被出库分配。双击看冻结范围，冻结中的行可直接解冻。"
            headerActions={
              <div className="flex flex-wrap gap-2">
                {targetMap.has('condition-restore') ? (
                  <Button type="button" variant="outline" onClick={() => openAction('condition-restore')}>
                    次品转良品
                  </Button>
                ) : null}
                {targetMap.has('stock-freeze') ? (
                  <Button type="button" className={primaryActionClass} onClick={() => openAction('stock-freeze')}>
                    <Plus className="h-3.5 w-3.5 mr-1" />
                    冻结库存
                  </Button>
                ) : null}
              </div>
            }
            search={{ value: freezeKeyword, onChange: setFreezeKeyword, placeholder: '搜冻结号、仓、商品或箱号' }}
            filters={<WmsStatusChips value={freezeStatus} options={freezeStatuses} onChange={setFreezeStatus} />}
            columns={freezeColumns}
            rows={freezeRows}
            rowKey={(row) => row.freezeId}
            inspector={freezeInspector}
            onRowOpen={setFreeze}
            pagination={freezes.pagination}
            emptyRowsNote="当前条件下没有冻结记录"
            viewState={catalogueViewState(freezes.answer, freezeRows.length, freezes.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-freezes',
              emptyTitle: freezeStatus === 'FROZEN' ? '当前没有冻结中的库存' : '还没有冻结记录',
              emptyDescription: '可在此冻结，或在实时库存对单个格子点冻结。',
              narrowed: freezeSettled.trim() !== '' || freezeStatus !== 'FROZEN',
            })}
          />
        </TabsContent>

        <TabsContent value="jobs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<InternalJobRecord>
            moduleId="warehouse-processing"
            title="库内作业台账"
            description="作业台提交的上架、移库、拆箱、状态调整与短少。双击看每一行的来源库位和入账数量。"
            headerActions={
              <Button type="button" variant="outline" onClick={goWorkstation}>
                <TabletSmartphone className="h-3.5 w-3.5 mr-1" />
                打开作业台
              </Button>
            }
            search={{ value: jobKeyword, onChange: setJobKeyword, placeholder: '搜事实号、仓库或作业种类' }}
            columns={jobColumns}
            rows={jobRows}
            rowKey={(row) => row.factId}
            inspector={jobInspector}
            onRowOpen={setJob}
            pagination={jobs.pagination}
            emptyRowsNote="当前条件下没有库内作业"
            viewState={catalogueViewState(jobs.answer, jobRows.length, jobs.retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-internal-jobs',
              emptyTitle: '还没有库内作业记录',
              emptyDescription: '在仓内作业台提交移库或报短少后，记录会出现在这里。',
              narrowed: jobSettled.trim() !== '',
            })}
          />
        </TabsContent>
      </Tabs>

      <InstructionDetailModal
        row={instruction}
        canVoid={
          instruction !== null &&
          instruction.status === 'PENDING' &&
          targetMap.has(instruction.kind === 'RELABEL' ? 'relabel-void' : 'processing-void')
        }
        onClose={() => setInstruction(null)}
        onVoid={() => {
          if (instruction) voidInstruction(instruction);
        }}
      />
      <FreezeDetailModal
        row={freeze}
        canUnfreeze={targetMap.has('stock-unfreeze')}
        onClose={() => setFreeze(null)}
        onUnfreeze={() => {
          if (freeze) openAction('stock-unfreeze', { freezeId: freeze.freezeId });
        }}
      />
      <JobDetailModal row={job} onClose={() => setJob(null)} />
      <ActionDrawer
        moduleId="warehouse-processing"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={refreshAll}
      />
    </div>
  );
}

function InstructionDetailModal({
  row,
  canVoid,
  onClose,
  onVoid,
}: {
  row: InstructionRecord | null;
  canVoid: boolean;
  onClose: () => void;
  onVoid: () => void;
}) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.instructionId ?? ''}
      subtitle={row ? `${labelOf(row.kind)} · ${row.ownerAccount}` : undefined}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      stepper={
        row ? (
          <WmsStepper
            steps={instructionSteps}
            currentId={row.status === 'VOID' ? 'PENDING' : row.status}
            failedLabel={row.status === 'VOID' ? '已作废' : undefined}
          />
        ) : undefined
      }
      actions={
        row ? (
          <>
            <Button type="button" variant="outline" onClick={goWorkstation}>
              去作业台执行
            </Button>
            {canVoid ? (
              <Button type="button" variant="outline" onClick={onVoid}>
                作废
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
              { label: '仓库', value: row.warehouseCode, mono: true },
              { label: '商品', value: row.goodsCode, mono: true },
              { label: '单位', value: row.unit || '—' },
              { label: '完成 / 计划', value: `${row.doneQuantity} / ${row.quantity}`, mono: true },
              { label: '原别名', value: row.fromAlias || '—', mono: true },
              { label: '新别名', value: row.toAlias || '—', mono: true },
              { label: '批号', value: row.batchNumber || '—', mono: true },
              { label: '来源', value: labelOf(row.origin) },
              { label: '原因', value: row.reason || '—' },
              { label: '受理', value: formatInstant(row.receivedAt), mono: true },
              { label: '作废原因', value: row.voidReason || '—' },
              { label: '组成版本', value: `r${row.fixedRevision}`, mono: true },
            ]}
          />
          <WmsSubTable
            title="组成"
            count={row.components?.length ?? 0}
            emptyText={row.kind === 'RELABEL' ? '换标不使用组成行。' : '这张指令没有组成行。'}
            rows={row.components ?? []}
            rowKey={(line) => line.goodsCode}
            columns={[
              { id: 'goods', header: '组件商品', mono: true, render: (line) => line.goodsCode },
              { id: 'qty', header: '每件用量', align: 'right', mono: true, render: (line) => String(line.quantity) },
            ]}
          />
          <WmsSubTable
            title="执行记录"
            count={row.executions?.length ?? 0}
            emptyText="还没有现场执行。到作业台扫码完成后会出现在这里。"
            rows={row.executions ?? []}
            rowKey={(line) => line.factId}
            columns={[
              { id: 'fact', header: '事实号', mono: true, render: (line) => line.factId },
              { id: 'qty', header: '完成数量', align: 'right', mono: true, render: (line) => String(line.doneQuantity) },
              { id: 'who', header: '操作者', mono: true, render: (line) => line.operatorId },
              { id: 'device', header: '设备', mono: true, render: (line) => line.deviceId },
              { id: 'at', header: '时刻', mono: true, render: (line) => formatInstant(line.occurredAt) },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

function FreezeDetailModal({
  row,
  canUnfreeze,
  onClose,
  onUnfreeze,
}: {
  row: StockFreezeRecord | null;
  canUnfreeze: boolean;
  onClose: () => void;
  onUnfreeze: () => void;
}) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.freezeId ?? ''}
      subtitle={row?.reason}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      actions={
        row && row.status === 'FROZEN' && canUnfreeze ? (
          <Button type="button" className={primaryActionClass} onClick={onUnfreeze}>
            解冻
          </Button>
        ) : undefined
      }
      onClose={onClose}
    >
      {row ? (
        <WmsInfoGrid
          items={[
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '仓库', value: row.warehouseCode, mono: true },
            { label: '对象', value: labelOf(row.target) },
            { label: '库位', value: row.locationCode || '—', mono: true },
            { label: '商品', value: row.goodsCode || '—', mono: true },
            { label: '箱号', value: row.cartonNumber || '—', mono: true },
            { label: '单位 / 状态', value: `${row.unit || '—'} · ${labelOf(row.stockState)}` },
            { label: '数量', value: String(row.quantity), mono: true },
            { label: '批号', value: row.batchNumber || '—', mono: true },
            { label: '入库批次', value: row.inboundBatchId || '—', mono: true },
            { label: '冻结', value: `${row.operator} · ${formatInstant(row.frozenAt)}` },
            { label: '解冻', value: row.unfrozenAt ? `${row.unfreezeOperator || ''} · ${formatInstant(row.unfrozenAt)}` : '—' },
          ]}
        />
      ) : null}
    </WmsDetailModal>
  );
}

function JobDetailModal({ row, onClose }: { row: InternalJobRecord | null; onClose: () => void }) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.factId ?? ''}
      subtitle={row ? labelOf(row.kind) : undefined}
      badge={row ? <WmsBadge code={row.kind} label={labelOf(row.kind)} tone="info" /> : undefined}
      onClose={onClose}
    >
      {row ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '仓库', value: row.warehouse, mono: true },
              { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
              { label: '收到时刻', value: formatInstant(row.receivedAt), mono: true },
              { label: '设备序号', value: `#${row.deviceSequence}`, mono: true },
            ]}
          />
          <WmsSubTable
            title="作业行"
            count={row.lines.length}
            emptyText="这笔作业没有行。"
            rows={row.lines}
            rowKey={(line, index) => `${line.goodsCode ?? line.cartonNumber ?? 'line'}-${index}`}
            columns={[
              { id: 'owner', header: '货主', mono: true, render: (line) => line.ownerAccount },
              { id: 'goods', header: '商品 / 箱', mono: true, render: (line) => line.goodsCode || line.cartonNumber || '—' },
              { id: 'from', header: '来源库位', mono: true, render: (line) => line.sourceLocation || '—' },
              { id: 'to', header: '目标库位', mono: true, render: (line) => line.targetLocation || '—' },
              { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity ?? 0) },
              { id: 'posted', header: '已入账', align: 'right', mono: true, render: (line) => String(line.posted) },
              {
                id: 'signal',
                header: '信号',
                render: (line) => {
                  const bits = [
                    line.shortOnBook > 0 ? `短少 ${line.shortOnBook}` : '',
                    line.held > 0 ? `占用 ${line.held}` : '',
                    line.cartonMissing ? '整箱缺失' : '',
                  ].filter(Boolean);
                  return bits.join(' · ') || '正常';
                },
              },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}
