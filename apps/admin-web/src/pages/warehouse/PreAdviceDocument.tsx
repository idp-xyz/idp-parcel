import { Button } from '@idpxyz/ui-primitives';
import { useEffect, useRef } from 'react';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import { formatInstant } from '../catalogue-view';
import { adviceActionPrefill } from './advice-action';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getPreAdviceDetail, type PreAdviceRecord } from './api';
import { WmsAsideBlock, WmsDocumentPage } from './document-page';
import { preAdviceProgress } from './pre-advice-progress';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { inboundSheet } from './print-forms';
import { openPrint } from './print-sheet';
import { receiveStationHash } from './station/stations';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type StepItem } from './wms-ui';

const steps: readonly StepItem[] = [
  { id: 'ADVISED', label: '已预报' },
  { id: 'RECEIVING', label: '收货中' },
  { id: 'CLOSED', label: '已完结' },
];

// 读口要货主加预报标识两格；useKeyedLoad 只认一把字符串键，两格用换行拼起来。
function loadPreAdvice(key: string) {
  const splitAt = key.indexOf('\n');
  return getPreAdviceDetail(key.slice(0, splitAt), key.slice(splitAt + 1));
}

/**
 * 入库预报页：货主说要来什么、实际收了多少、每一次收货记了什么。收货本身在作业台做，这里看进度并处理预报
 * （更新告知、撤回、完结、给收货差异结论）。
 */
export function PreAdviceDocument({
  ownerAccount,
  preAdviceId,
  panel,
}: {
  ownerAccount: string;
  preAdviceId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const back = useReturnToList('warehouse-pre-advices');
  const adviceKey = `${ownerAccount}\n${preAdviceId}`;
  const { answer, loading, reload, held, reloading } = useKeyedLoad(adviceKey, loadPreAdvice, { holdPrevious: panel !== undefined });
  const shownKey = useRef(adviceKey);
  useEffect(() => {
    if (shownKey.current === adviceKey) return;
    shownKey.current = adviceKey;
    drawer.close();
  }, [adviceKey, drawer]);
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const afterWrite = panel ? panel.onChanged : reload;
  const record: PreAdviceRecord | undefined = answer?.kind === 'outcome' ? answer.body.preAdvice : undefined;
  const failure =
    readFailureMessage(answer) ??
    (answer?.kind === 'outcome' && !record ? `货主 ${ownerAccount} 名下没有 ${preAdviceId} 这张入库预报。` : null);

  const progress = record ? preAdviceProgress(record) : null;
  const open = record?.status === 'ADVISED' || record?.status === 'RECEIVING';
  const expectedAt = record?.currentNotice?.expectedAt;
  // 只有还没开始收的预报说得上迟到：收货中的已经到了。
  const late = record?.status === 'ADVISED' ? elapsedLabel(expectedAt, new Date()) : '';
  const receipts = record?.receipts ?? [];
  const openDiscrepancies = receipts.filter((receipt) => receipt.discrepancy && !receipt.discrepancy.conclusion);

  return (
    <>
      <WmsDocumentPage
        kind="入库预报"
        title={record?.documentNumber ?? preAdviceId}
        subtitle={
          record
            ? [
                `货主 ${record.ownerAccount}`,
                `仓 ${record.warehouseCode}`,
                `${labelOf(record.arrivalMode)} · ${labelOf(record.inboundMode)}入库`,
                expectedAt ? `预计 ${formatInstant(expectedAt)} 到` : '没有告知预计到达',
              ].join(' · ')
            : undefined
        }
        badge={
          record ? (
            <>
              <WmsBadge code={record.status} />
              {late ? <WmsBadge label={`已迟 ${late}`} tone="danger" /> : null}
            </>
          ) : undefined
        }
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument('warehouse-pre-advices', [ownerAccount, preAdviceId]) } : undefined}
        stepper={
          record ? (
            <WmsStepper
              steps={steps}
              currentId={record.status === 'CANCELLED' ? 'ADVISED' : record.status}
              failedLabel={record.status === 'CANCELLED' ? '已取消' : undefined}
              failedTone="neutral"
            />
          ) : undefined
        }
        actions={
          record ? (
            <>
              {open && drawer.has('pre-advice-notice') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('pre-advice-notice', adviceActionPrefill(record, false))}>
                  更新告知
                </Button>
              ) : null}
              {record.status === 'ADVISED' && drawer.has('pre-advice-withdrawal') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('pre-advice-withdrawal', adviceActionPrefill(record, false))}>
                  撤回
                </Button>
              ) : null}
              {record.status === 'RECEIVING' && drawer.has('pre-advice-close') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('pre-advice-close', adviceActionPrefill(record, true))}>
                  完结
                </Button>
              ) : null}
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  openPrint(
                    inboundSheet(record.documentNumber ?? record.preAdviceId, record.warehouseCode, (record.goodsLines ?? []).map((line) => ({
                      goodsCode: line.goodsCode,
                      unit: line.unit,
                      advised: line.quantity,
                      received: (record.cumulativeReceived ?? []).filter((item) => item.goodsCode === line.goodsCode).reduce((sum, item) => sum + item.quantity, 0),
                    }))),
                  )
                }
              >
                打印入库单
              </Button>
              {open ? (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => enterStation(receiveStationHash(record?.preAdviceId))}
                >
                  去收货
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          record ? (
            <>
              <WmsAsideBlock title="到货告知">
                {record.currentNotice ? (
                  <>
                    <span>预计到达 {formatInstant(record.currentNotice.expectedAt)}</span>
                    <span className="type-caption text-idpxyz-textMuted">到货线索 {record.currentNotice.arrivalClue || '没有'}</span>
                    <span className="break-all font-mono type-caption text-idpxyz-textMuted">
                      货主参考号 {record.currentNotice.ownerReference || '没有'}
                    </span>
                    <span className="type-caption text-idpxyz-textMuted">第 {record.currentNotice.version} 版告知</span>
                  </>
                ) : (
                  <span className="text-idpxyz-textMuted">还没有告知。</span>
                )}
              </WmsAsideBlock>
              <WmsAsideBlock title="来源">
                <span>{labelOf(record.submissionPath)}</span>
                <span className="break-all font-mono type-caption text-idpxyz-textMuted">登记人 {record.registrant}</span>
                <span className="type-caption text-idpxyz-textMuted">受理于 {formatInstant(record.acceptedAt)}</span>
                {record.evidenceReference ? (
                  <span className="break-all type-caption text-idpxyz-textMuted">依据 {record.evidenceReference}</span>
                ) : null}
                {record.documentNumber ? (
                  <span className="break-all font-mono type-caption text-idpxyz-textMuted">内部标识 {record.preAdviceId}</span>
                ) : (
                  <span className="type-caption text-idpxyz-textMuted">没有入库单号：{record.numberingAbsence ?? '编号规则没有给号'}</span>
                )}
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {record && progress ? (
          <>
            <WmsKpiStrip
              items={[
                { label: progress.byCarton ? '预报箱数' : '预报数量', value: progress.advised },
                {
                  label: '已收',
                  value: progress.received,
                  note: progress.outstanding > 0 ? `还差 ${progress.outstanding}` : progress.advised > 0 ? '已收齐' : undefined,
                  tone: progress.outstanding > 0 ? 'warning' : progress.advised > 0 ? 'success' : undefined,
                },
                { label: '收货次数', value: receipts.length },
                {
                  label: '没给结论的差异',
                  value: openDiscrepancies.length,
                  tone: openDiscrepancies.length > 0 ? 'danger' : undefined,
                },
              ]}
            />
            <WmsSubTable
              title="收货进度"
              count={progress.rows.length}
              emptyText="这张预报没有商品行，也没有整箱。"
              rows={progress.rows}
              rowKey={(row) => row.key}
              columns={[
                { id: 'what', header: progress.byCarton ? '箱号' : '商品', mono: true, render: (row) => row.key },
                { id: 'detail', header: progress.byCarton ? '箱内' : '单位', render: (row) => row.detail || '—' },
                { id: 'advised', header: '预报', align: 'right', mono: true, render: (row) => String(row.advised) },
                { id: 'received', header: '已收', align: 'right', mono: true, render: (row) => String(row.received) },
                {
                  id: 'gap',
                  header: '还差',
                  align: 'right',
                  mono: true,
                  render: (row) =>
                    row.advised === 0 ? (
                      <WmsBadge label="预报里没有" tone="warning" />
                    ) : row.received > row.advised ? (
                      <WmsBadge label={`多收 ${row.received - row.advised}`} tone="warning" />
                    ) : (
                      String(row.advised - row.received)
                    ),
                },
              ]}
            />
            {(record.goodsLines?.length ?? 0) > 0 ? (
              <WmsSubTable
                title="预报的商品行"
                count={record.goodsLines?.length ?? 0}
                emptyText=""
                rows={record.goodsLines ?? []}
                rowKey={(line, index) => `${line.goodsCode}-${index}`}
                columns={[
                  { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
                  { id: 'unit', header: '单位', render: (line) => line.unit },
                  { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
                  { id: 'cartons', header: '箱数', align: 'right', mono: true, render: (line) => (line.cartonCount ? String(line.cartonCount) : '—') },
                  { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
                  { id: 'expiry', header: '效期', mono: true, render: (line) => line.expiryDate || '—' },
                ]}
              />
            ) : null}
            {receipts.length > 0 ? (
              <WmsSubTable
                title="收货记录"
                count={receipts.length}
                emptyText=""
                rows={receipts}
                rowKey={(receipt) => receipt.factId}
                columns={[
                  { id: 'at', header: '时刻', mono: true, render: (receipt) => formatInstant(receipt.occurredAt) },
                  { id: 'outcome', header: '结果', render: (receipt) => labelOf(receipt.outcome) },
                  {
                    id: 'lines',
                    header: '行数',
                    align: 'right',
                    mono: true,
                    render: (receipt) => String((receipt.lines?.length ?? 0) + (receipt.cartonLines?.length ?? 0)),
                  },
                  { id: 'device', header: '设备', mono: true, render: (receipt) => receipt.device },
                  { id: 'who', header: '操作者', mono: true, render: (receipt) => receipt.operator },
                  {
                    id: 'gap',
                    header: '收货差异',
                    render: (receipt) =>
                      !receipt.discrepancy ? (
                        '—'
                      ) : receipt.discrepancy.conclusion ? (
                        <span>
                          <span className="font-mono">{receipt.discrepancy.discrepancyId}</span> · {receipt.discrepancy.conclusion.verdict}
                        </span>
                      ) : (
                        <span className="inline-flex flex-wrap items-center gap-2">
                          <span className="font-mono">{receipt.discrepancy.discrepancyId}</span>
                          {drawer.has('discrepancy-conclusion') ? (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => drawer.open('discrepancy-conclusion', { discrepancyId: receipt.discrepancy?.discrepancyId })}
                            >
                              给结论
                            </Button>
                          ) : (
                            <WmsBadge label="没给结论" tone="danger" />
                          )}
                        </span>
                      ),
                  },
                  { id: 'fact', header: '作业事实', mono: true, render: (receipt) => receipt.factId },
                ]}
              />
            ) : null}
            {(record.notices?.length ?? 0) > 1 ? (
              <WmsSubTable
                title="告知版本"
                count={record.notices?.length ?? 0}
                emptyText=""
                rows={record.notices ?? []}
                rowKey={(notice) => String(notice.version)}
                columns={[
                  { id: 'version', header: '版', mono: true, render: (notice) => String(notice.version) },
                  { id: 'expected', header: '预计到达', mono: true, render: (notice) => formatInstant(notice.expectedAt) },
                  { id: 'clue', header: '到货线索', render: (notice) => notice.arrivalClue || '—' },
                  { id: 'ref', header: '货主参考号', mono: true, render: (notice) => notice.ownerReference || '—' },
                  { id: 'path', header: '途径', render: (notice) => labelOf(notice.submissionPath) },
                  { id: 'at', header: '记下的时刻', mono: true, render: (notice) => formatInstant(notice.recordedAt) },
                ]}
              />
            ) : null}
            {record.closure ? (
              <section className="space-y-2">
                <h3 className="type-body font-semibold text-idpxyz-textBright">完结</h3>
                <WmsInfoGrid
                  items={[
                    { label: '完结时刻', value: formatInstant(record.closure.closedAt), mono: true },
                    { label: '完结人', value: record.closure.operator || '—', mono: true },
                    {
                      label: '少收',
                      value:
                        record.closure.shortages?.map((item) => `${item.goodsCode || item.cartonNumber || '项'}×${item.quantity ?? 1}`).join('、') ||
                        '没有少收',
                    },
                    { label: '说明', value: record.closure.note || '—' },
                  ]}
                />
              </section>
            ) : null}
            {record.unclaimedArrival ? (
              <p className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 type-body text-idpxyz-text">
                这张预报取消或完结之后还有货到，已经转成待认领到货，没有入库。到「待认领到货」里认领或处置。
              </p>
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer
        moduleId="warehouse-pre-advices"
        active={drawer.active}
        lookups={drawer.lookups}
        onClose={drawer.close}
        onSubmitted={afterWrite}
      />
    </>
  );
}
