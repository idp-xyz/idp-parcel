import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { returnReceiptKind, returnsModule as moduleId } from './return-address';
import { ReceiptStatusBadge } from './return-badges';
import {
  adviceLineNote,
  adviceLineRows,
  adviceRegistrant,
  findReturnAdvice,
  numberingAbsenceNote,
  receiptsOfAdvice,
  receiptTally,
  returnAdviceNextStep,
  returnAdviceNumber,
  returnAdviceStatus,
  returnAdviceTrack,
  type AdviceLineRow,
} from './return-flow';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { loadReturnsOverview } from './use-returns-overview';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn } from './wms-ui';

const mutedText = 'type-caption text-idpxyz-textMuted';

/**
 * 退货预报页：货主或运营事先说有什么要退回来、到仓了没有、到仓之后怎么处置。退货只有概览一个读口，没有按号取一张的，所以这一页
 * 读整份概览，在里面按预报号认出这一张（概览是租户全册，认不出就是没有）。预报自己没有状态，进度与状态都是拿到仓记录对出来的
 * （returnAdviceStatus）。收退件在作业台做，预报页只在还没到仓时指路；到仓之后的处置落在每一笔到仓记录上，从这里点进去做。
 */
export function ReturnAdviceDocument({ adviceKey }: { adviceKey: string }) {
  const back = useReturnToList(moduleId);
  const complete = adviceKey !== '';
  const { answer, loading, reload } = useKeyedLoad(complete ? adviceKey : null, loadReturnsOverview);
  const overview = answer?.kind === 'outcome' ? answer.body : undefined;
  const advice = overview ? findReturnAdvice(overview.preAdvices ?? [], adviceKey) : undefined;
  const failure =
    (complete ? null : '地址里缺预报号：退货预报的地址是「退货预报 / 预报号」两段。') ??
    readFailureMessage(answer) ??
    (overview && !advice ? `没有 ${adviceKey} 这张退货预报。它可能属于别的公司，或预报号有误。` : null);

  const receipts = advice && overview ? receiptsOfAdvice(advice, overview.receipts ?? []) : [];
  const status = returnAdviceStatus(receipts);
  const track = advice ? returnAdviceTrack(receipts) : null;
  const next = advice ? returnAdviceNextStep(status) : null;
  const tally = receiptTally(receipts);
  const lineRows = advice ? adviceLineRows(advice, receipts) : [];
  const advisedLines = lineRows.filter((row) => row.expected > 0).length;

  const lineColumns: WmsSubColumn<AdviceLineRow>[] = [
    {
      id: 'goods',
      header: '商品',
      render: (row) => (advice ? <DocumentLink moduleId="warehouse-goods" parts={[advice.ownerAccount, row.goodsCode]} /> : row.goodsCode),
    },
    { id: 'unit', header: '单位', render: (row) => row.unit },
    { id: 'expected', header: '预计数量', align: 'right', mono: true, render: (row) => String(row.expected) },
    { id: 'arrived', header: '已入暂存', align: 'right', mono: true, render: (row) => String(row.arrived) },
    {
      id: 'note',
      header: '到了没有',
      render: (row) => {
        const note = adviceLineNote(row);
        return <WmsBadge label={note.label} tone={note.tone} />;
      },
    },
  ];

  return (
    <WmsDocumentPage
      kind="退货预报"
      title={advice ? returnAdviceNumber(advice) : adviceKey}
      subtitle={
        advice
          ? [`货主 ${advice.ownerAccount}`, `仓 ${advice.warehouseCode}`, `退件单号 ${advice.trackingNumber}`, `受理于 ${formatInstant(advice.receivedAt)}`].join(' · ')
          : undefined
      }
      badge={
        advice ? (
          <>
            <WmsBadge code={status} />
            <WmsBadge label={labelOf(advice.origin)} tone="info" />
          </>
        ) : undefined
      }
      loading={loading}
      error={failure}
      onBack={back}
      onReload={reload}
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
        advice && next ? (
          <Button
            type="button"
            className={primaryActionClass}
            onClick={() => {
              window.location.hash = `#/${next.moduleId}`;
            }}
          >
            {next.label}
          </Button>
        ) : undefined
      }
      aside={
        advice ? (
          <>
            <WmsAsideBlock title="到仓记录">
              {receipts.length === 0 ? (
                <span className="text-idpxyz-textMuted">还没有到仓记录。退件在作业台扫码到仓后，会按退件单号对上这张预报。</span>
              ) : (
                receipts.map((receipt) => (
                  <div key={receipt.factId} className="flex flex-wrap items-baseline gap-x-2">
                    <DocumentLink moduleId={moduleId} parts={[returnReceiptKind, receipt.factId]} />
                    <span className={mutedText}>
                      {receipt.goodsCode} × {receipt.quantity}
                    </span>
                  </div>
                ))
              )}
            </WmsAsideBlock>
            <WmsAsideBlock title="来源">
              <span>{labelOf(advice.origin)}</span>
              <span className="break-all font-mono type-caption text-idpxyz-textMuted">登记人 {adviceRegistrant(advice)}</span>
              <span className={mutedText}>受理于 {formatInstant(advice.receivedAt)}</span>
              {advice.basis ? <span className={`break-all ${mutedText}`}>依据 {advice.basis}</span> : null}
            </WmsAsideBlock>
            <WmsAsideBlock title="退件怎么收">
              <span>在作业台扫码收：退件单号对得上预报、或认得出货主的，入退货暂存位；认不出的转成待认领到货，不入库存账。</span>
              <span className={mutedText}>预报里的商品只是预计，不入库存账；入了暂存才有库存。</span>
            </WmsAsideBlock>
          </>
        ) : undefined
      }
    >
      {advice ? (
        <>
          <WmsKpiStrip
            items={[
              { label: '预计商品行', value: advice.lines?.length ?? 0 },
              {
                label: '到仓记录',
                value: tally.total,
                note: tally.total === 0 ? '还没有退件到仓' : tally.diverted > 0 ? `其中 ${tally.diverted} 笔转了待认领` : undefined,
              },
              { label: '待处置', value: tally.awaiting, tone: tally.awaiting > 0 ? 'warning' : undefined },
              { label: '已处置', value: tally.disposed, tone: tally.disposed > 0 ? 'success' : undefined },
            ]}
          />
          <WmsInfoGrid
            items={[
              advice.documentNumber
                ? { label: '预报号', value: advice.documentNumber, mono: true }
                : { label: '预报号', value: numberingAbsenceNote(advice.numberingAbsence) },
              { label: '内部标识', value: advice.adviceId, mono: true },
              { label: '货主', value: advice.ownerAccount, mono: true },
              { label: '仓库', value: advice.warehouseCode, mono: true },
              { label: '退件单号', value: advice.trackingNumber, mono: true },
              { label: '来源', value: labelOf(advice.origin) },
              { label: '依据', value: advice.basis || '—', mono: true },
              { label: '登记人', value: adviceRegistrant(advice), mono: true },
              { label: '受理时刻', value: formatInstant(advice.receivedAt), mono: true },
            ]}
          />
          <WmsSubTable
            title="预计退回"
            count={advisedLines}
            extra={<span className={mutedText}>预计商品不入库存账；已入暂存的数量按对上这张预报的到仓记录加起来，转了待认领的不算</span>}
            emptyText="这张预报没有商品行。"
            rows={lineRows}
            rowKey={(row) => `${row.goodsCode}\n${row.unit}`}
            columns={lineColumns}
          />
          {receipts.length > 0 ? (
            <WmsSubTable
              title="到仓记录"
              count={receipts.length}
              extra={<span className={mutedText}>同一件退件里每种商品是一笔，各有自己的作业事实号</span>}
              emptyText=""
              rows={receipts}
              rowKey={(receipt) => receipt.factId}
              columns={[
                {
                  id: 'fact',
                  header: '作业事实号',
                  render: (receipt) => <DocumentLink moduleId={moduleId} parts={[returnReceiptKind, receipt.factId]} />,
                },
                { id: 'goods', header: '商品', mono: true, render: (receipt) => receipt.goodsCode },
                { id: 'qty', header: '数量', align: 'right', mono: true, render: (receipt) => `${receipt.quantity} ${receipt.unit}` },
                { id: 'state', header: '库存状态', render: (receipt) => <WmsBadge code={receipt.stockState} /> },
                { id: 'disp', header: '回库处置', render: (receipt) => <ReceiptStatusBadge receipt={receipt} /> },
                { id: 'at', header: '到仓时刻', mono: true, render: (receipt) => formatInstant(receipt.occurredAt) },
              ]}
            />
          ) : null}
        </>
      ) : null}
    </WmsDocumentPage>
  );
}
