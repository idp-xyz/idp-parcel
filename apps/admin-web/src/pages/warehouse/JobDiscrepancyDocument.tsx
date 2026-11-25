import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { findJobDiscrepancy, getInternalJob, type InternalJobLineRecord } from './api';
import { lotLabel } from './count-flow';
import { countsAdjustmentsModule, discrepancyKind } from './counts-adjustments-address';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { internalJobKind, processingModule } from './processing-address';
import { discrepancyEffect, discrepancyStatusLabel, discrepancyTrack, originLineIndex } from './stock-approval-flow';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn } from './wms-ui';

const moduleId = countsAdjustmentsModule;
const mutedText = 'type-caption text-idpxyz-textMuted';

/**
 * 作业差异页：库内作业（上架、移库、形态与状态转换、短少上报）里没能入账的那一部分，等人批准补账或驳回。
 * 差异本身不带作业种类，所以页面按作业事实号再读一次那次作业，读不到不拦正文，只少了种类与作业各行。
 * 作业事实号链到它自己的库内作业页，那里写这次作业账上入了多少。批准与驳回在头区做。
 */
export function JobDiscrepancyDocument({ discrepancyId }: { discrepancyId: string }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = discrepancyId !== '';
  const { answer, loading, reload } = useKeyedLoad(complete ? discrepancyId : null, findJobDiscrepancy);
  const found = answer?.kind === 'outcome' ? answer.body : undefined;
  const record = found?.record;
  const failure =
    (complete ? null : '地址里缺差异号：作业差异的地址是「作业差异 / 差异号」两段。') ??
    readFailureMessage(answer) ??
    (found && !record
      ? found.truncated
        ? `按「${discrepancyId}」检索到的结果多于一页，前 ${found.searched} 条里没有它。请回列表，用检索词找。`
        : `没有 ${discrepancyId} 这条作业差异。它可能属于别的公司，或差异号有误。`
      : null);

  const jobLoad = useKeyedLoad(record?.factId ?? null, getInternalJob);
  const job = jobLoad.answer?.kind === 'outcome' ? jobLoad.answer.body.job : undefined;
  const jobFailure = readFailureMessage(jobLoad.answer);
  const reloadAll = () => {
    reload();
    jobLoad.reload();
  };

  const track = record ? discrepancyTrack(record.status) : null;
  const pending = record?.status === 'PENDING';
  const canApprove = drawer.has('job-discrepancy-approve');
  const canReject = drawer.has('job-discrepancy-reject');
  const jobLines = job?.lines ?? [];
  const originIndex = record ? originLineIndex(jobLines, record.discrepancyId, record.linePosition) : -1;

  const jobColumns: WmsSubColumn<InternalJobLineRecord>[] = [
    { id: 'line', header: '行', mono: true, render: (_line, index) => String(index + 1) },
    {
      id: 'goods',
      header: '商品 / 整箱',
      render: (line) =>
        line.goodsCode ? (
          <DocumentLink moduleId="warehouse-goods" parts={[line.ownerAccount, line.goodsCode]} />
        ) : line.cartonNumber ? (
          <span className="font-mono">整箱 {line.cartonNumber}</span>
        ) : (
          '—'
        ),
    },
    {
      id: 'move',
      header: '库位',
      mono: true,
      // 上架与移库有来源与目标两头；短少只在一个库位上，写一个就行。
      render: (line) =>
        line.sourceLocation && line.targetLocation
          ? `${line.sourceLocation} → ${line.targetLocation}`
          : line.sourceLocation || line.targetLocation || '—',
    },
    { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => (line.quantity ? `${line.quantity} ${line.unit ?? ''}`.trim() : '—') },
    { id: 'posted', header: '已入账', align: 'right', mono: true, render: (line) => String(line.posted) },
    { id: 'short', header: '账面短少', align: 'right', mono: true, render: (line) => String(line.shortOnBook) },
    { id: 'held', header: '占用', align: 'right', mono: true, render: (line) => String(line.held) },
    {
      id: 'discrepancy',
      header: '差异',
      render: (line, index) =>
        index === originIndex ? (
          <WmsBadge label="这一条" tone="info" />
        ) : line.discrepancyId ? (
          <DocumentLink moduleId={moduleId} parts={[discrepancyKind, line.discrepancyId]} />
        ) : line.cartonMissing ? (
          '整箱缺失'
        ) : (
          '—'
        ),
    },
  ];

  return (
    <>
      <WmsDocumentPage
        kind="作业差异"
        measure="条"
        title={record?.discrepancyId ?? discrepancyId}
        subtitle={
          record
            ? [`货主 ${record.ownerAccount}`, `仓 ${record.warehouseCode}`, job ? labelOf(job.kind) : '', `作业事实 ${record.factId}`]
                .filter((part) => part !== '')
                .join(' · ')
            : undefined
        }
        badge={record ? <WmsBadge code={record.status} label={discrepancyStatusLabel(record.status)} /> : undefined}
        loading={loading}
        error={failure}
        onBack={back}
        onReload={reloadAll}
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
                <Button type="button" variant="outline" onClick={() => drawer.open('job-discrepancy-reject', { discrepancyId: record.discrepancyId })}>
                  驳回
                </Button>
              ) : null}
              {canApprove ? (
                <Button type="button" className={primaryActionClass} onClick={() => drawer.open('job-discrepancy-approve', { discrepancyId: record.discrepancyId })}>
                  批准入账
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          record ? (
            <>
              {pending ? (
                <WmsAsideBlock title="批准会怎样">
                  <span>{discrepancyEffect(job?.kind, record)}</span>
                  <span className={mutedText}>账上做不成这一笔（货不够、被冻结或正在盘点）时批准不成立，账不动。驳回不改账，这条差异就此结束。</span>
                  <span className={mutedText}>作业的操作者能否自己批准，由租户登记的批准职责规则决定。</span>
                  {canApprove || canReject ? null : (
                    <span className="type-caption text-amber-600 dark:text-amber-400">{hiddenDecisionHint('批准与驳回', '库存调整批准')}</span>
                  )}
                </WmsAsideBlock>
              ) : null}
              <WmsAsideBlock title="来自的库内作业">
                <DocumentLink moduleId={processingModule} parts={[internalJobKind, record.factId]} />
                {job ? (
                  <>
                    <span>{labelOf(job.kind)}</span>
                    <span className={mutedText}>发生于 {formatInstant(job.occurredAt)}</span>
                    <span className={mutedText}>收到于 {formatInstant(job.receivedAt)}</span>
                  </>
                ) : jobLoad.loading ? (
                  <span className={mutedText}>正在读这次作业……</span>
                ) : jobFailure ? (
                  <span className="type-caption text-amber-600 dark:text-amber-400">没能读到这次作业：{jobFailure}</span>
                ) : (
                  <span className={mutedText}>没有读到这次作业，只能看差异本身。</span>
                )}
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {record ? (
          <>
            <WmsKpiStrip
              items={[
                { label: '差额数量', value: record.quantity, note: record.unit || undefined },
                { label: '账面短少', value: record.shortOnBook, note: '账上数量不够', tone: record.shortOnBook > 0 ? 'warning' : undefined },
                { label: '占用', value: record.heldQuantity, note: '因占用取不走', tone: record.heldQuantity > 0 ? 'warning' : undefined },
                ...(record.cartonNumber || record.cartonMissing
                  ? [
                      {
                        label: '整箱',
                        value: record.cartonNumber || '—',
                        note: record.cartonMissing ? '整箱对不上账' : undefined,
                        tone: record.cartonMissing ? ('warning' as const) : undefined,
                      },
                    ]
                  : []),
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '货主', value: record.ownerAccount, mono: true },
                { label: '仓库', value: record.warehouseCode, mono: true },
                { label: '来源库位', value: record.sourceLocation || '—', mono: true },
                { label: '所在库位', value: record.locationCode || '—', mono: true },
                {
                  label: '商品',
                  value: record.goodsCode ? <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, record.goodsCode]} /> : '—',
                },
                { label: '单位 / 库存状态', value: `${record.unit || '—'} · ${labelOf(record.stockState)}` },
                { label: '批次', value: lotLabel(record) || '—' },
                { label: '整箱', value: record.cartonNumber || (record.cartonMissing ? '整箱缺失' : '—'), mono: true },
                { label: '说明', value: record.note || '—' },
                ...(record.status === 'APPROVED'
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
            {jobLines.length > 0 ? (
              <WmsSubTable
                title="这次库内作业的各行"
                count={jobLines.length}
                extra={<span className={mutedText}>{labelOf(job?.kind)} · 逐行看入账了多少、短少与占用多少</span>}
                emptyText=""
                rows={jobLines}
                rowKey={(_line, index) => String(index)}
                columns={jobColumns}
              />
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reloadAll} />
    </>
  );
}
