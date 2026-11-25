import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { ActionDrawer, useDrawerOver } from './ActionDrawer';
import { getStockImportDetail, type StockImportCartonLineRecord, type StockImportGoodsLineRecord } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { stockImportStatusLabel } from './rows';
import { useStockImportTargets } from './stock-import-actions';
import { stockImportTotals, stockImportTrack } from './stock-import-flow';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn } from './wms-ui';

const moduleId = 'stock-imports';

/**
 * 建账导入页：期初库存按哪些行入账。提交只是登记，不改库存账；批准才入账，驳回就此结束。
 * 批准与驳回在头区做——批准的人在这里核完每一行再下手。
 */
export function StockImportDocument({ importId }: { importId: string }) {
  const drawer = useDrawerOver(useStockImportTargets());
  const back = useReturnToList(moduleId);
  const { answer, loading, reload } = useKeyedLoad(importId, getStockImportDetail);
  const record = answer?.kind === 'outcome' ? answer.body.stockImport : undefined;
  const failure =
    readFailureMessage(answer) ?? (answer?.kind === 'outcome' && !record ? `没有 ${importId} 这张建账导入。它可能属于别的公司，或导入号有误。` : null);

  const track = record ? stockImportTrack(record.status) : null;
  const totals = record ? stockImportTotals(record) : null;
  const pending = record?.status === 'PENDING';
  const mayDecide = drawer.has('approve');

  const goodsLines = record?.goodsLines ?? [];
  const cartonLines = record?.cartonLines ?? [];
  // 商品行与整箱行大多只有一种。两种都空的导入也要摆出一个空表，说清「这张单没有行」。
  const showGoods = goodsLines.length > 0 || cartonLines.length === 0;

  const goodsColumns: WmsSubColumn<StockImportGoodsLineRecord>[] = [
    { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
    {
      id: 'goods',
      header: '商品',
      // 商品是这位货主名下的商品档案。
      render: (line) => (record ? <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, line.goodsCode]} /> : line.goodsCode),
    },
    { id: 'state', header: '库存状态', render: (line) => <WmsBadge code={line.stockState} /> },
    { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
    { id: 'expiry', header: '效期', mono: true, render: (line) => line.expiryDate || '—' },
  ];
  // 贴标别名与申报入库时刻只有少数行才填：没有一行带它时不摆这一列。入库时刻决定库龄，批的人要看得到。
  if (goodsLines.some((line) => line.labelAlias)) {
    goodsColumns.push({ id: 'alias', header: '贴标别名', mono: true, render: (line) => line.labelAlias || '—' });
  }
  if (goodsLines.some((line) => line.declaredInboundAt)) {
    goodsColumns.push({ id: 'inbound', header: '申报入库时刻', mono: true, render: (line) => (line.declaredInboundAt ? formatInstant(line.declaredInboundAt) : '—') });
  }
  goodsColumns.push({ id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => `${line.quantity} ${line.unit}` });

  const cartonColumns: WmsSubColumn<StockImportCartonLineRecord>[] = [
    { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
    { id: 'carton', header: '箱号', mono: true, render: (line) => line.cartonNumber },
    { id: 'bill', header: '箱内', render: (line) => line.contents.map((item) => `${item.goodsCode}×${item.quantity}`).join('、') || '—' },
  ];
  if (cartonLines.some((line) => line.declaredInboundAt)) {
    cartonColumns.push({ id: 'inbound', header: '申报入库时刻', mono: true, render: (line) => (line.declaredInboundAt ? formatInstant(line.declaredInboundAt) : '—') });
  }

  return (
    <>
      <WmsDocumentPage
        kind="建账导入"
        title={record?.importId ?? importId}
        subtitle={record ? [`货主 ${record.ownerAccount}`, `仓 ${record.warehouseCode}`, `提交于 ${formatInstant(record.submittedAt)}`].join(' · ') : undefined}
        badge={record ? <WmsBadge code={record.status} label={stockImportStatusLabel(record.status)} /> : undefined}
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
          record && pending && mayDecide ? (
            <>
              <Button type="button" variant="outline" onClick={() => drawer.open('reject', { importId: record.importId })}>
                驳回
              </Button>
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('approve', { importId: record.importId })}>
                批准并入账
              </Button>
            </>
          ) : undefined
        }
        aside={
          record && (pending || record.status === 'POSTED') ? (
            <>
              {pending ? (
                <WmsAsideBlock title="批准会怎样">
                  <span>批准后按这些行入账，库存才增加；驳回不改账，这张单就此结束。</span>
                  <span className="type-caption text-idpxyz-textMuted">提交人能否自己批准，由租户登记的批准职责规则决定。</span>
                  {mayDecide ? null : (
                    <span className="type-caption text-amber-600 dark:text-amber-400">{hiddenDecisionHint('批准与驳回', '库存调整批准')}</span>
                  )}
                </WmsAsideBlock>
              ) : null}
              {record.status === 'POSTED' ? (
                <WmsAsideBlock title="入账">
                  {record.dutyRevision ? <span className="type-caption text-idpxyz-textMuted">按批准职责规则第 {record.dutyRevision} 修订批准</span> : null}
                  {record.goodsMovementId ? (
                    <span className="break-all font-mono type-caption text-idpxyz-textMuted">商品行入账流水 {record.goodsMovementId}</span>
                  ) : null}
                  {record.cartonMovementId ? (
                    <span className="break-all font-mono type-caption text-idpxyz-textMuted">整箱行入账流水 {record.cartonMovementId}</span>
                  ) : null}
                  {record.dutyRevision || record.goodsMovementId || record.cartonMovementId ? null : (
                    <span className="type-caption text-idpxyz-textMuted">已入账。</span>
                  )}
                </WmsAsideBlock>
              ) : null}
            </>
          ) : undefined
        }
      >
        {record && totals ? (
          <>
            <WmsKpiStrip
              items={[
                { label: '商品行', value: totals.goodsLines },
                { label: '整箱行', value: totals.cartonLines },
                { label: '涉及商品', value: `${totals.goods} 种` },
                { label: '涉及库位', value: `${totals.locations} 个` },
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '货主', value: record.ownerAccount, mono: true },
                { label: '仓库', value: record.warehouseCode, mono: true },
                { label: '建账原因', value: record.reason },
                { label: '提交人', value: record.submitter, mono: true },
                { label: '提交时刻', value: formatInstant(record.submittedAt), mono: true },
                ...(record.status === 'POSTED'
                  ? [
                      { label: '批准人', value: record.poster, mono: true },
                      { label: '入账时刻', value: record.postedAt ? formatInstant(record.postedAt) : undefined, mono: true },
                    ]
                  : []),
                ...(record.status === 'REJECTED'
                  ? [
                      { label: '驳回人', value: record.rejector, mono: true },
                      { label: '驳回时刻', value: record.rejectedAt ? formatInstant(record.rejectedAt) : undefined, mono: true },
                      { label: '驳回原因', value: record.rejectionNote },
                    ]
                  : []),
              ]}
            />
            {showGoods ? (
              <WmsSubTable
                title="商品行"
                count={goodsLines.length}
                emptyText="这张导入没有商品行，也没有整箱行。"
                rows={goodsLines}
                rowKey={(line, index) => `${line.locationCode}-${line.goodsCode}-${index}`}
                columns={goodsColumns}
              />
            ) : null}
            {cartonLines.length > 0 ? (
              <WmsSubTable
                title="整箱行"
                count={cartonLines.length}
                emptyText=""
                rows={cartonLines}
                rowKey={(line) => line.cartonNumber}
                columns={cartonColumns}
              />
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reload} />
    </>
  );
}
