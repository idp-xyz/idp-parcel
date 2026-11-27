import { useEffect, useRef } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { findAdjustment, type StockAdjustmentLineRecord } from './api';
import { lotLabel } from './count-flow';
import { adjustmentKind, countsAdjustmentsModule } from './counts-adjustments-address';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { adjustmentStatusLabel, adjustmentTotals, adjustmentTrack, kindQuantities, signedAdjustmentQuantity } from './stock-approval-flow';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn } from './wms-ui';

const moduleId = countsAdjustmentsModule;

/**
 * 库存调整页：为什么调、调哪些库位的哪些商品、各调多少。提交只是登记，不改库存账；批准才按这些行入账，驳回就此结束。
 * 批准与驳回在头区做——批准的人在这里核完每一行再下手。
 *
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一行留着上一张，新的读到再换。栏里做完一个动作告诉列表重取，
 * 列表重取会让 `revision` 变，这张单跟着重读。
 */
export function AdjustmentDocument({
  adjustmentId,
  panel,
}: {
  adjustmentId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = adjustmentId !== '';
  const { answer, loading, reload, held, reloading } = useKeyedLoad(complete ? adjustmentId : null, findAdjustment, {
    holdPrevious: panel !== undefined,
  });
  const shownId = useRef(adjustmentId);
  useEffect(() => {
    if (shownId.current === adjustmentId) return;
    shownId.current = adjustmentId;
    drawer.close();
  }, [adjustmentId, drawer]);
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
    (complete ? null : '地址里缺调整号：库存调整的地址是「调整 / 调整号」两段。') ??
    readFailureMessage(answer) ??
    (found && !record
      ? found.truncated
        ? `按「${adjustmentId}」检索到的结果多于一页，前 ${found.searched} 条里没有它。请回列表，用检索词找。`
        : `没有 ${adjustmentId} 这张库存调整单。它可能属于别的公司，或调整号有误。`
      : null);

  const lines = record?.lines ?? [];
  const totals = adjustmentTotals(lines);
  const track = record ? adjustmentTrack(record.status) : null;
  const pending = record?.status === 'PENDING';
  const canApprove = drawer.has('adjustment-approve');
  const canReject = drawer.has('adjustment-reject');

  const lineColumns: WmsSubColumn<StockAdjustmentLineRecord>[] = [
    { id: 'line', header: '行', mono: true, render: (line) => String(line.lineIndex + 1) },
    { id: 'kind', header: '种类', render: (line) => labelOf(line.kind) },
    { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
    {
      id: 'goods',
      header: '商品',
      // 商品是这位货主名下的商品档案。
      render: (line) => (record ? <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, line.goodsCode]} /> : line.goodsCode),
    },
    { id: 'state', header: '库存状态', render: (line) => <WmsBadge code={line.stockState} /> },
  ];
  // 批号、效期、贴标与指名的入库批次只有少数行才填：没有一行带它时不摆这一列。
  if (lines.some((line) => lotLabel(line) !== '')) {
    lineColumns.push({ id: 'lot', header: '批次', render: (line) => lotLabel(line) || '—' });
  }
  if (lines.some((line) => line.inboundBatchId)) {
    lineColumns.push({ id: 'inbound', header: '入库批次', mono: true, render: (line) => line.inboundBatchId || '—' });
  }
  lineColumns.push({
    id: 'qty',
    header: '数量',
    align: 'right',
    mono: true,
    render: (line) => `${signedAdjustmentQuantity(line)} ${line.unit}`,
  });
  if (lines.some((line) => line.basis)) {
    lineColumns.push({ id: 'basis', header: '依据', render: (line) => line.basis || '—' });
  }

  return (
    <>
      <WmsDocumentPage
        kind="库存调整单"
        title={record?.adjustmentId ?? adjustmentId}
        subtitle={record ? [`货主 ${record.ownerAccount}`, `仓 ${record.warehouseCode}`, `提交于 ${formatInstant(record.submittedAt)}`].join(' · ') : undefined}
        badge={record ? <WmsBadge code={record.status} label={adjustmentStatusLabel(record.status)} /> : undefined}
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={
          panel
            ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [adjustmentKind, record?.adjustmentId ?? adjustmentId]) }
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
          record && pending && (canApprove || canReject) ? (
            <>
              {canReject ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('adjustment-reject', { adjustmentId: record.adjustmentId })}>
                  驳回
                </Button>
              ) : null}
              {canApprove ? (
                <Button type="button" className={primaryActionClass} onClick={() => drawer.open('adjustment-approve', { adjustmentId: record.adjustmentId })}>
                  批准入账
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          record && pending ? (
            <WmsAsideBlock title="批准会怎样">
              <span>提交不改库存账。批准后才按这些行入账：调增增加库存，调减与销毁扣减库存；驳回不改账，这张调整就此结束。</span>
              <span className="type-caption text-idpxyz-textMuted">提交人能否自己批准，由租户登记的批准职责规则决定。</span>
              {canApprove || canReject ? null : (
                <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('批准与驳回', '库存调整批准')}</span>
              )}
            </WmsAsideBlock>
          ) : undefined
        }
      >
        {record ? (
          <>
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">原因</h3>
              <p className="whitespace-pre-wrap rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3 type-body text-idpxyz-textBright">
                {record.reason || '提交时没有写原因。'}
              </p>
            </section>
            <WmsKpiStrip
              items={[
                { label: '调整行', value: totals.lines, note: `${totals.goods} 种商品 · ${totals.locations} 个库位` },
                ...totals.kinds.map((kind) => ({ label: labelOf(kind.kind), value: kindQuantities(kind), note: `${kind.rows} 行` })),
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '货主', value: record.ownerAccount, mono: true },
                { label: '仓库', value: record.warehouseCode, mono: true },
                { label: '提交人', value: record.operator, mono: true },
                { label: '提交时刻', value: formatInstant(record.submittedAt), mono: true },
                ...(record.status === 'POSTED'
                  ? [
                      { label: '批准人', value: record.approvedBy, mono: true },
                      { label: '批准时刻', value: record.approvedAt ? formatInstant(record.approvedAt) : undefined, mono: true },
                      { label: '批准说明', value: record.approvalNote },
                    ]
                  : []),
                ...(record.status === 'REJECTED'
                  ? [
                      { label: '驳回人', value: record.rejectedBy, mono: true },
                      { label: '驳回时刻', value: record.rejectedAt ? formatInstant(record.rejectedAt) : undefined, mono: true },
                      { label: '驳回原因', value: record.rejectReason },
                    ]
                  : []),
              ]}
            />
            <WmsSubTable
              title="调整行"
              count={lines.length}
              emptyText="这张调整单没有行。"
              rows={lines}
              rowKey={(line) => String(line.lineIndex)}
              columns={lineColumns}
            />
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={panel ? panel.onChanged : reload} />
    </>
  );
}
