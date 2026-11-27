import { Button } from '@idpxyz/ui-primitives';
import { useEffect, useRef } from 'react';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { returnReceiptOutcomeLabels } from './presentation';
import { returnAdviceKind, returnReceiptKind, returnsModule as moduleId } from './return-address';
import { ReceiptStatusBadge } from './return-badges';
import {
  advicesOfReceipt,
  findReturnReceipt,
  returnAdviceNumber,
  returnDestinationLabel,
  returnReceiptNextStep,
  returnReceiptStatus,
  returnReceiptTrack,
  returnReceiptWaitingSince,
  siblingReceipts,
} from './return-flow';
import { loadReturnReceipt } from './api';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { putawayStationHash } from './station/stations';
import { openDocument, useReturnToList } from './use-document';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsTone } from './wms-ui';

const mutedText = 'type-caption text-idpxyz-textMuted';
const noteBox = 'rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3 type-body text-idpxyz-textBright';

/**
 * 退货到仓页：一笔退件在作业台扫进暂存位之后留下的记录，认出了哪张预报、转没转待认领、记没记回库去向。按作业事实号取这一笔、
 * 同一退件单号下的其他到仓记录和对上的预报，认不出就是没有。
 * 回库处置在头区做，只记去向、不改库存账；认不出货主的转待认领，处置不落在这一笔上，页头改指向待认领到货。
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一行留着上一笔，新的读到再换。
 */
export function ReturnReceiptDocument({
  factId,
  panel,
}: {
  factId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = factId !== '';
  const { answer, loading, reload, held, reloading } = useKeyedLoad(complete ? factId : null, loadReturnReceipt, { holdPrevious: panel !== undefined });
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const afterWrite = panel ? panel.onChanged : reload;
  const shownId = useRef(factId);
  useEffect(() => {
    if (shownId.current === factId) return;
    shownId.current = factId;
    drawer.close();
  }, [factId, drawer]);
  const overview = answer?.kind === 'outcome' ? answer.body : undefined;
  const receipt = overview ? findReturnReceipt(overview.receipts ?? [], factId) : undefined;
  const failure =
    (complete ? null : '地址里缺作业事实号：退货到仓的地址是「退货到仓 / 作业事实号」两段。') ??
    readFailureMessage(answer) ??
    (overview && !receipt ? `没有 ${factId} 这笔退货到仓记录。它可能属于别的公司，或作业事实号有误。` : null);

  const status = receipt ? returnReceiptStatus(receipt) : null;
  const track = receipt ? returnReceiptTrack(receipt) : null;
  const next = status ? returnReceiptNextStep(status) : null;
  const canDispose = receipt !== undefined && next !== null && next.station === undefined && drawer.has('return-disposition');
  const canMove = next?.station === 'putaway';
  const advices = receipt && overview ? advicesOfReceipt(receipt, overview.preAdvices ?? []) : [];
  const siblings = receipt && overview ? siblingReceipts(receipt, overview.receipts ?? []) : [];
  const disposition = receipt?.disposition;
  const waiting = receipt ? elapsedLabel(returnReceiptWaitingSince(receipt), new Date()) : '';
  const diverted = status === 'DIVERTED';

  const kpis: Array<{ label: string; value: string | number; note?: string; tone?: WmsTone }> = [];
  if (receipt) {
    kpis.push({ label: '到仓数量', value: receipt.quantity, note: receipt.unit || undefined });
    if (status === 'AWAITING_DISPOSITION') {
      kpis.push({ label: '已等', value: waiting || '刚到', note: `到仓于 ${formatInstant(receipt.occurredAt)}`, tone: 'warning' });
    }
    if (disposition) {
      kpis.push({
        label: '处置去向',
        value: returnDestinationLabel(disposition.destination),
        note: `处置于 ${formatInstant(disposition.decidedAt)}，货还在退货暂存`,
        tone: 'success',
      });
    }
    if (diverted) kpis.push({ label: '到仓结果', value: '转待认领', note: '没有入库存账' });
  }

  const unclaimedLink = receipt?.arrivalId ? <DocumentLink moduleId="warehouse-unclaimed" parts={[receipt.arrivalId]} /> : null;
  const adviceLinks =
    advices.length > 0 ? (
      <span className="flex flex-col gap-1">
        {advices.map((advice) => (
          <DocumentLink key={advice.adviceId} moduleId={moduleId} parts={[returnAdviceKind, returnAdviceNumber(advice)]} />
        ))}
      </span>
    ) : (
      receipt?.adviceId || null
    );
  // 记着预报标识的是系统认出来的；没记的，是退件单号对得上才算上，两种说法不一样。一张也没对上时只说「认出的预报」。
  const adviceLabel = receipt?.adviceId || advices.length === 0 ? '认出的预报' : '退件单号对上的预报';

  return (
    <>
      <WmsDocumentPage
        kind="退货到仓"
        measure="笔"
        title={receipt?.factId ?? factId}
        subtitle={
          receipt
            ? [`退件单号 ${receipt.trackingNumber}`, `仓 ${receipt.warehouseCode}`, `到仓于 ${formatInstant(receipt.occurredAt)}`].join(' · ')
            : undefined
        }
        badge={receipt ? <ReceiptStatusBadge receipt={receipt} /> : undefined}
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [returnReceiptKind, factId]) } : undefined}
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
          receipt ? (
            <>
              {canDispose ? (
                <Button type="button" className={primaryActionClass} onClick={() => drawer.open('return-disposition', { factId: receipt.factId })}>
                  处置
                </Button>
              ) : null}
              {canMove ? (
                <Button type="button" className={primaryActionClass} onClick={() => enterStation(putawayStationHash())}>
                  移出退货暂存
                </Button>
              ) : null}
              {diverted ? (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => {
                    if (receipt.arrivalId) openDocument('warehouse-unclaimed', [receipt.arrivalId]);
                    else window.location.hash = '#/warehouse-unclaimed';
                  }}
                >
                  去待认领到货
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          receipt ? (
            <>
              <WmsAsideBlock title="关联单据">
                {advices.length === 0 && !receipt.adviceId && !unclaimedLink ? (
                  <span className="text-idpxyz-textMuted">没有对上的预报，也没有转成待认领到货。</span>
                ) : null}
                {advices.length > 0 || receipt.adviceId ? (
                  <div className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">{adviceLabel}</span>
                    {adviceLinks}
                  </div>
                ) : null}
                {unclaimedLink ? (
                  <div className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">待认领到货</span>
                    {unclaimedLink}
                  </div>
                ) : null}
              </WmsAsideBlock>
              {siblings.length > 0 ? (
                <WmsAsideBlock title="同一件退件的其他商品">
                  {siblings.map((sibling) => (
                    <div key={sibling.factId} className="flex flex-wrap items-baseline gap-x-2">
                      <DocumentLink moduleId={moduleId} parts={[returnReceiptKind, sibling.factId]} />
                      <span className={mutedText}>
                        {sibling.goodsCode} × {sibling.quantity}
                      </span>
                    </div>
                  ))}
                </WmsAsideBlock>
              ) : null}
              {diverted ? (
                <WmsAsideBlock title="转待认领会怎样">
                  <span>认不出货主，没有入库存账，这一笔上也就没有处置可做。</span>
                  <span className={mutedText}>货被认领之后，另起一笔退货入库，处置记在那一笔上。</span>
                </WmsAsideBlock>
              ) : status === 'AWAITING_DISPOSITION' || status === 'DISPOSED' ? (
                <WmsAsideBlock title="回库处置会怎样">
                  <span>处置只记下这件货的去向，不改库存账：良品上架与转次品的库内流水另在库内作业里做，销毁走库存调整的销毁行，退还货主走新的出库请求。</span>
                  <span className={mutedText}>货要还在退货暂存位，处置才成立；已经被挪走的，处置会被拒。</span>
                </WmsAsideBlock>
              ) : null}
            </>
          ) : undefined
        }
      >
        {receipt ? (
          <>
            <WmsKpiStrip items={kpis} />
            <WmsInfoGrid
              items={[
                { label: '退件单号', value: receipt.trackingNumber, mono: true },
                { label: '作业事实号', value: receipt.factId, mono: true },
                { label: '仓库', value: receipt.warehouseCode, mono: true },
                { label: '暂存库位', value: receipt.locationCode, mono: true },
                { label: '货主', value: receipt.ownerAccount || '未认出', mono: true },
                { label: '到仓结果', value: returnReceiptOutcomeLabels[receipt.outcome] ?? receipt.outcome },
                { label: '到仓时刻', value: formatInstant(receipt.occurredAt), mono: true },
                { label: adviceLabel, value: adviceLinks },
                { label: '待认领号', value: unclaimedLink },
              ]}
            />
            <WmsSubTable
              title="到仓商品"
              count={1}
              extra={diverted ? <span className={mutedText}>没有入库，这一行只记下现场扫到的</span> : undefined}
              emptyText=""
              rows={[receipt]}
              rowKey={(row) => row.factId}
              columns={[
                {
                  id: 'goods',
                  header: '商品',
                  render: (row) => (row.ownerAccount ? <DocumentLink moduleId="warehouse-goods" parts={[row.ownerAccount, row.goodsCode]} /> : row.goodsCode),
                },
                { id: 'unit', header: '单位', render: (row) => row.unit },
                { id: 'qty', header: '数量', align: 'right', mono: true, render: (row) => String(row.quantity) },
                { id: 'state', header: '库存状态', render: (row) => <WmsBadge code={row.stockState} /> },
                { id: 'location', header: '暂存库位', mono: true, render: (row) => row.locationCode },
              ]}
            />
            {disposition || diverted || next !== null ? (
              <section className="space-y-2">
                <h3 className="type-body font-semibold text-idpxyz-textBright">回库处置</h3>
                {disposition ? (
                  <WmsInfoGrid
                    items={[
                      { label: '处置去向', value: <ReceiptStatusBadge receipt={receipt} /> },
                      { label: '处置时刻', value: formatInstant(disposition.decidedAt), mono: true },
                      { label: '处置人', value: disposition.operator || (disposition.origin === 'CUSTOMER' ? '货主自己提交' : '—'), mono: true },
                      { label: '来源', value: labelOf(disposition.origin) },
                      { label: '依据', value: disposition.basis || '—', mono: true },
                      { label: '处置决定号', value: disposition.decisionId, mono: true },
                    ]}
                  />
                ) : diverted ? (
                  <p className={noteBox}>
                    这一笔认不出货主，转成了待认领到货，没有入库存账，所以没有处置可做：认领之后另起一笔退货入库，处置记在那一笔上。
                    {unclaimedLink ? <span className="ml-2">到货号 {unclaimedLink}</span> : null}
                  </p>
                ) : (
                  <>
                    <p className={noteBox}>
                      这件货入了退货暂存位 {receipt.locationCode}，还没记去向。点页头「处置」，记下它是良品上架、转次品、销毁还是退还货主。
                    </p>
                    {canDispose ? null : (
                      <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('处置', '退货代录')}</span>
                    )}
                  </>
                )}
              </section>
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={afterWrite} />
    </>
  );
}
