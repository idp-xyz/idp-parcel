import { useEffect, useRef } from 'react';
import { formatInstant } from '../catalogue-view';
import { getInternalJob, type InternalJobLineRecord } from './api';
import { countsAdjustmentsModule, discrepancyKind } from './counts-adjustments-address';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import {
  incompleteReasons,
  jobSummary,
  lineAccounting,
  lineLocations,
  lineOutcomeLabels,
  lineOutcomeTones,
  lineStory,
  plannedLabel,
  shortageKind,
} from './internal-job-flow';
import { internalJobKind, processingModule } from './processing-address';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsSubTable, type WmsSubColumn, type WmsTone } from './wms-ui';

const moduleId = processingModule;
const mutedText = 'type-caption text-idpxyz-textMuted';

// 四种作业各是什么：只说作业本身。没入账的部分怎么补，在它形成的作业差异上说，这里不重复。
const kindNotes: Readonly<Record<string, string>> = {
  INTERNAL_MOVE: '上架、移库与拣选位补货都记成库内移动：从来源库位取货，放进目标库位；整箱按箱整个搬。',
  FORM_CONVERSION: '包装形态转换：在原库位把一种包装单位按折合数换成另一种，已入目标账的数按转入的单位算。',
  CONDITION_CONVERSION: '库存状态转换：在原库位把库存从一种状态改成另一种。',
  SHORTAGE: '账实短少上报：现场发现实物比账少，只上报，不直接改账。',
};

/**
 * 库内作业页：作业台提交的一笔上架、移库、形态或状态转换，或一次短少上报。它是已经发生的事实，没有生命周期，所以不画进度条，
 * 状态位是作业种类。正文的重点是「账上入了多少」：每一行计划多少、实际入账多少、没入的是账上不足还是被占用挡住。
 * 没入账的部分形成作业差异，在右栏链过去；那里批准补账，补进去的数不回改这里。
 *
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一行留着上一张，新的读到再换。列表重取会让 `revision` 变，这笔作业跟着重读。
 */
export function InternalJobDocument({
  factId,
  panel,
}: {
  factId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const back = useReturnToList(moduleId);
  const complete = factId !== '';
  const { answer, loading, reload, held, reloading } = useKeyedLoad(complete ? factId : null, getInternalJob, { holdPrevious: panel !== undefined });
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const body = answer?.kind === 'outcome' ? answer.body : undefined;
  const job = body?.job;
  const failure =
    (complete ? null : '地址里缺作业事实号：库内作业的地址是「库内作业 / 作业事实号」两段。') ??
    readFailureMessage(answer) ??
    (body && !job ? `没有 ${factId} 这笔库内作业。它可能属于别的公司，或作业事实号有误。` : null);

  const lines = job?.lines ?? [];
  const kind = job?.kind ?? '';
  const shortage = kind === shortageKind;
  const summary = job ? jobSummary(job.kind, lines) : null;
  const reasons = job ? incompleteReasons(job.kind, lines) : [];
  const multiOwner = (summary?.owners.length ?? 0) > 1;
  const showDiscrepancy = lines.some((line) => line.discrepancyId);
  const showMovement = lines.some((line) => line.movementId);

  const kpis: Array<{ label: string; value: string | number; note?: string; tone?: WmsTone }> = [];
  if (summary) {
    if (shortage) {
      kpis.push({ label: '上报行', value: summary.lines, note: summary.cartonLines > 0 ? `其中整箱 ${summary.cartonLines} 行` : undefined });
    } else {
      kpis.push({ label: '作业行', value: summary.lines, note: summary.cartonLines > 0 ? `其中整箱 ${summary.cartonLines} 行` : undefined });
      kpis.push({ label: '全部入账', value: summary.full, note: `共 ${summary.lines} 行`, tone: summary.incomplete === 0 && summary.lines > 0 ? 'success' : undefined });
      kpis.push({ label: '没入完', value: summary.incomplete, note: reasons.join(' · ') || undefined, tone: summary.incomplete > 0 ? 'warning' : undefined });
    }
    kpis.push({
      label: '形成差异',
      value: summary.discrepancyIds.length,
      note: summary.discrepancyIds.length > 0 ? '等批准补账或驳回' : undefined,
      tone: summary.discrepancyIds.length > 0 ? 'warning' : undefined,
    });
  }

  const goodsCell = (line: InternalJobLineRecord) =>
    line.goodsCode ? (
      <DocumentLink moduleId="warehouse-goods" parts={[line.ownerAccount, line.goodsCode]} />
    ) : line.cartonNumber ? (
      <span className="font-mono">整箱 {line.cartonNumber}</span>
    ) : (
      '—'
    );
  // 为零的账上不足与被占用不写数字，免得一屏零里找不到有数的那几格。
  const shortCell = (value: number) => (value > 0 ? <span className="text-[color:var(--ds-status-warning-fg)]">{value}</span> : '—');

  const columns: WmsSubColumn<InternalJobLineRecord>[] = [{ id: 'line', header: '行', mono: true, render: (_line, index) => String(index + 1) }];
  // 一笔作业可以带几个货主；只有一个时它写在上面的信息格里，不占每行。
  if (multiOwner) columns.push({ id: 'owner', header: '货主', mono: true, render: (line) => line.ownerAccount });
  columns.push(
    { id: 'goods', header: '商品 / 整箱', render: goodsCell },
    { id: 'where', header: '库位', mono: true, render: (line) => lineLocations(line) },
  );
  if (shortage) {
    // 短少上报整张表都是「已上报」，没有入账可说，不另开结果一列。
    columns.push({ id: 'planned', header: '上报短少', align: 'right', mono: true, render: (line) => plannedLabel(line) });
  } else {
    columns.push(
      { id: 'planned', header: '计划', align: 'right', mono: true, render: (line) => plannedLabel(line) },
      { id: 'posted', header: '已入账', align: 'right', mono: true, render: (line) => String(line.posted) },
      { id: 'inbound', header: '已入目标账', align: 'right', mono: true, render: (line) => String(line.postedInbound) },
      { id: 'short', header: '账上不足', align: 'right', mono: true, render: (line) => shortCell(line.shortOnBook) },
      { id: 'held', header: '被占用挡住', align: 'right', mono: true, render: (line) => shortCell(line.held) },
      {
        id: 'result',
        header: '结果',
        render: (line) => {
          const acc = lineAccounting(kind, line);
          return (
            <div className="flex min-w-[12rem] flex-col items-start gap-0.5">
              <WmsBadge label={lineOutcomeLabels[acc.outcome]} tone={lineOutcomeTones[acc.outcome]} />
              {acc.outcome === 'full' ? null : <span className={mutedText}>{lineStory(acc)}</span>}
            </div>
          );
        },
      },
    );
  }
  if (showDiscrepancy) {
    columns.push({
      id: 'discrepancy',
      header: '差异',
      render: (line) => (line.discrepancyId ? <DocumentLink moduleId={countsAdjustmentsModule} parts={[discrepancyKind, line.discrepancyId]} /> : '—'),
    });
  }
  if (showMovement) columns.push({ id: 'movement', header: '入账流水', mono: true, render: (line) => line.movementId || '—' });

  return (
    <WmsDocumentPage
      kind="库内作业"
      measure="笔"
      title={job?.factId ?? factId}
      subtitle={job ? [`仓 ${job.warehouse}`, `发生于 ${formatInstant(job.occurredAt)}`, `设备序号 #${job.deviceSequence}`].join(' · ') : undefined}
      badge={
        job ? (
          <>
            <WmsBadge code={job.kind} label={labelOf(job.kind)} tone="info" />
            {summary && summary.incomplete > 0 ? <WmsBadge label={`${summary.incomplete} 行没入完`} tone="warning" /> : null}
          </>
        ) : undefined
      }
      loading={held ? false : loading}
      reloading={reloading}
      busy={held}
      error={failure}
      onBack={back}
      onReload={reload}
      panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [internalJobKind, job?.factId ?? factId]) } : undefined}
      aside={
        job && summary ? (
          <>
            <WmsAsideBlock title="这是什么作业">
              <span>{kindNotes[job.kind] ?? labelOf(job.kind)}</span>
            </WmsAsideBlock>
            {shortage ? null : (
              <WmsAsideBlock title="怎么读这些数">
                <span>计划：这一行要做的数量。</span>
                <span>已入账：从来源取走并入账的数量。</span>
                <span>已入目标账：放进目标的数量；包装形态转换换了单位，所以与已入账不相等。</span>
                <span>账上不足：账上本来就没有这么多。</span>
                <span>被占用挡住：账上有，但被冻结、盘点锁定、待拣分配或预占占着，取不走。</span>
              </WmsAsideBlock>
            )}
            {summary.discrepancyIds.length > 0 ? (
              <WmsAsideBlock title="形成的差异">
                <span className={mutedText}>没入账的数量不会自动补：每条差异由有批准职责的人批准补账，或驳回。</span>
                {summary.discrepancyIds.map((id) => (
                  <DocumentLink key={id} moduleId={countsAdjustmentsModule} parts={[discrepancyKind, id]} />
                ))}
              </WmsAsideBlock>
            ) : null}
          </>
        ) : undefined
      }
    >
      {job && summary ? (
        <>
          <WmsKpiStrip items={kpis} />
          <WmsInfoGrid
            items={[
              { label: '仓库', value: job.warehouse, mono: true },
              { label: '作业种类', value: labelOf(job.kind) },
              { label: '货主', value: summary.owners.join('、') || '—', mono: true },
              { label: '发生时刻', value: formatInstant(job.occurredAt), mono: true },
              { label: '收到时刻', value: formatInstant(job.receivedAt), mono: true },
              { label: '设备序号', value: `#${job.deviceSequence}`, mono: true },
            ]}
          />
          <WmsSubTable
            title={shortage ? '上报的短少' : '账上入了多少'}
            count={lines.length}
            extra={
              <span className={mutedText}>
                {shortage
                  ? '每一行形成一条作业差异，批准后才改账'
                  : '数是登记那一刻入的；没入账的部分形成作业差异，批准补账后记在差异与库存流水上，不回改这里'}
              </span>
            }
            emptyText="这笔作业没有行。"
            rows={lines}
            rowKey={(_line, index) => String(index)}
            columns={columns}
          />
        </>
      ) : null}
    </WmsDocumentPage>
  );
}
