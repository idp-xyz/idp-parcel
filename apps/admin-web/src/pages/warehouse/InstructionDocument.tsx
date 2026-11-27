import { useEffect, useRef } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { findInstruction } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import {
  instructionGrantLabel,
  instructionHeadline,
  instructionMaterials,
  instructionNextStep,
  instructionShortfall,
  instructionStatusLabel,
  instructionTitle,
  instructionTrack,
  instructionVoidActionId,
  instructionWaitingSince,
  noAliasWord,
  relabelPlan,
  type MaterialRow,
} from './instruction-flow';
import { instructionKind, processingModule } from './processing-address';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn, type WmsTone } from './wms-ui';

const moduleId = processingModule;
const mutedText = 'type-caption text-idpxyz-textMuted';

/**
 * 指令页：一份加工、拆解或换标指令要做什么、现场做了没有。三种指令共用一份指令册与一个读口，这一页按种类显示不同的内容：
 * 加工与拆解给出用料与产出，换标给出原别名与新别名。执行本身在作业台做，页面只指路——头区的主动作是去作业台，
 * 还没执行的可以作废；已执行的不能作废。
 *
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一行留着上一张，新的读到再换。栏里做完一个动作告诉列表重取，
 * 列表重取会让 `revision` 变，这张单跟着重读。
 */
export function InstructionDocument({
  instructionId,
  panel,
}: {
  instructionId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = instructionId !== '';
  const { answer, loading, reload, held, reloading } = useKeyedLoad(complete ? instructionId : null, findInstruction, {
    holdPrevious: panel !== undefined,
  });
  const shownId = useRef(instructionId);
  useEffect(() => {
    if (shownId.current === instructionId) return;
    shownId.current = instructionId;
    drawer.close();
  }, [instructionId, drawer]);
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const found = answer?.kind === 'outcome' ? answer.body : undefined;
  const record = found?.record;
  const failure =
    (complete ? null : '地址里缺指令号：加工与换标指令的地址是「指令 / 指令号」两段。') ??
    readFailureMessage(answer) ??
    (found && !record
      ? found.truncated
        ? `按「${instructionId}」检索到的结果多于一页，前 ${found.searched} 条里没有它。请回列表，用检索词找。`
        : `没有 ${instructionId} 这条指令。它可能属于别的公司，或指令号有误。`
      : null);

  const now = new Date();
  const track = record ? instructionTrack(record.status) : null;
  const next = record ? instructionNextStep(record.status) : null;
  const voidId = record ? instructionVoidActionId(record.kind) : 'processing-void';
  const canVoid = record !== undefined && record.status === 'PENDING' && drawer.has(voidId);
  const materials = record ? instructionMaterials(record) : [];
  const relabel = record ? relabelPlan(record) : null;
  const executions = record?.executions ?? [];
  const shortfall = record ? instructionShortfall(record) : 0;
  const waiting = record ? elapsedLabel(instructionWaitingSince(record), now) : '';

  const kpis: Array<{ label: string; value: string | number; note?: string; tone?: WmsTone }> = [];
  if (record) {
    kpis.push({ label: '计划数量', value: record.quantity, note: record.unit || undefined });
    kpis.push({
      label: '完成数量',
      value: record.doneQuantity,
      note: shortfall > 0 ? `比计划少 ${shortfall}，按完成数量入账，不再执行` : record.status === 'PENDING' ? '还没执行' : undefined,
      tone: record.status === 'DONE' ? (shortfall > 0 ? 'warning' : 'success') : undefined,
    });
    if (record.status === 'PENDING') {
      kpis.push({ label: '已等', value: waiting || '刚受理', note: `受理于 ${formatInstant(record.receivedAt)}` });
    }
  }

  const materialColumns: WmsSubColumn<MaterialRow>[] = [
    {
      id: 'direction',
      header: '方向',
      render: (row) => <WmsBadge label={row.direction === 'consume' ? '用料' : '产出'} tone={row.direction === 'consume' ? 'warning' : 'success'} />,
    },
    {
      id: 'goods',
      header: '商品',
      render: (row) => (
        <span className="inline-flex items-baseline gap-2">
          {record ? <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, row.goodsCode]} /> : row.goodsCode}
          <span className={mutedText}>{row.finished ? '成品' : '组件'}</span>
        </span>
      ),
    },
    { id: 'per', header: '每 1 个成品', align: 'right', mono: true, render: (row) => String(row.perFinished) },
    { id: 'total', header: '按计划共', align: 'right', mono: true, render: (row) => String(row.total) },
  ];

  return (
    <>
      <WmsDocumentPage
        kind={instructionTitle(record?.kind)}
        measure="条"
        title={record?.instructionId ?? instructionId}
        subtitle={
          record
            ? [labelOf(record.kind), `货主 ${record.ownerAccount}`, `仓 ${record.warehouseCode}`, `受理于 ${formatInstant(record.receivedAt)}`].join(' · ')
            : undefined
        }
        badge={record ? <WmsBadge code={record.status} label={instructionStatusLabel(record.status)} /> : undefined}
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={
          panel
            ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [instructionKind, record?.instructionId ?? instructionId]) }
            : undefined
        }
        stepper={
          track ? (
            <WmsStepper
              steps={track.steps}
              currentId={track.currentId}
              currentLabel={track.currentLabel}
              failedLabel={track.halt?.label}
              failedTone={track.halt?.tone}
            />
          ) : undefined
        }
        actions={
          record && next ? (
            <>
              {canVoid ? (
                <Button type="button" variant="outline" onClick={() => drawer.open(voidId, { instructionId: record.instructionId })}>
                  作废
                </Button>
              ) : null}
              <Button
                type="button"
                className={primaryActionClass}
                onClick={() => {
                  window.location.hash = `#/${next.moduleId}`;
                }}
              >
                {next.label}
              </Button>
            </>
          ) : undefined
        }
        aside={
          record && record.status === 'PENDING' ? (
            <>
              <WmsAsideBlock title="怎么执行">
                <span>
                  {record.kind === 'RELABEL'
                    ? '在作业台的「商品换标执行」做：扫码取出要换标的库存，系统按这份指令的新别名同笔换标入账。'
                    : '在作业台的「加工 / 拆分执行」做：录入用掉的组件与产出的成品；库存不够时整笔留在待执行。'}
                </span>
                <span className={mutedText}>一份指令只执行一次，按实际完成的数量入账；完成数量可以少于计划，但不会再执行第二次。</span>
              </WmsAsideBlock>
              <WmsAsideBlock title="作废会怎样">
                <span>作废后这份指令不再执行，也不动库存账。已执行的指令不能作废。</span>
                {canVoid ? null : (
                  <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('作废', instructionGrantLabel(record.kind))}</span>
                )}
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {record ? (
          <>
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">要做什么</h3>
              <div className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3 type-body text-idpxyz-textBright">
                <p>{instructionHeadline(record)}</p>
                {relabel && relabel.scope.length > 0 ? <p className={`mt-1 ${mutedText}`}>只换这一批的库存：{relabel.scope.join('、')}。</p> : null}
              </div>
            </section>
            <WmsKpiStrip items={kpis} />
            <WmsInfoGrid
              items={[
                { label: '货主', value: record.ownerAccount, mono: true },
                { label: '仓库', value: record.warehouseCode, mono: true },
                { label: '种类', value: labelOf(record.kind) },
                { label: '商品', value: <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, record.goodsCode]} /> },
                { label: '单位', value: record.unit || '—' },
                ...(record.kind === 'RELABEL'
                  ? [
                      { label: '原别名', value: record.fromAlias || noAliasWord, mono: true },
                      { label: '新别名', value: record.toAlias || noAliasWord, mono: true },
                      { label: '批号', value: record.batchNumber || '—', mono: true },
                      { label: '效期', value: record.expiryDate || '—', mono: true },
                    ]
                  : []),
                { label: '来源', value: labelOf(record.origin) },
                { label: '原因', value: record.reason || '—' },
                ...(record.basis ? [{ label: '依据', value: record.basis, mono: true }] : []),
                { label: '下发人', value: record.operator || '—', mono: true },
                { label: '受理时刻', value: formatInstant(record.receivedAt), mono: true },
                { label: '固定的商品修订', value: `r${record.fixedRevision}`, mono: true },
                ...(record.status === 'VOID'
                  ? [
                      { label: '作废时刻', value: record.voidedAt ? formatInstant(record.voidedAt) : '—', mono: true },
                      { label: '作废原因', value: record.voidReason || '—' },
                    ]
                  : []),
              ]}
            />
            {materials.length > 0 ? (
              <WmsSubTable
                title="用料与产出"
                count={materials.length}
                extra={<span className={mutedText}>组成在受理时固定下来，之后的商品修订不回改它；数量按基本单位算，现场实际用料以执行为准</span>}
                emptyText=""
                rows={materials}
                rowKey={(row) => `${row.direction}-${row.goodsCode}`}
                columns={materialColumns}
              />
            ) : null}
            {executions.length > 0 ? (
              <WmsSubTable
                title="执行记录"
                count={executions.length}
                extra={<span className={mutedText}>加工与换标的执行事实记在指令上，不进库内作业台账</span>}
                emptyText=""
                rows={executions}
                rowKey={(line) => line.factId}
                columns={[
                  { id: 'fact', header: '事实号', mono: true, render: (line) => line.factId },
                  { id: 'qty', header: '完成数量', align: 'right', mono: true, render: (line) => String(line.doneQuantity) },
                  { id: 'who', header: '操作者', mono: true, render: (line) => line.operatorId },
                  { id: 'device', header: '设备', mono: true, render: (line) => line.deviceId },
                  { id: 'at', header: '时刻', mono: true, render: (line) => formatInstant(line.occurredAt) },
                  { id: 'movement', header: '入账流水', mono: true, render: (line) => line.movementId || '—' },
                ]}
              />
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={panel ? panel.onChanged : reload} />
    </>
  );
}
