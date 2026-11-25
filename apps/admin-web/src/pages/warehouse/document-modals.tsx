import { Button } from '@idpxyz/ui-primitives';
import { formatInstant } from '../catalogue-view';
import { getGoodsCurrent, type GoodsRecord, type StockImportRecord, type UnclaimedArrivalRecord } from './api';
import type { GuidedFormValues } from './guided-forms';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { labelOf, WmsBadge, WmsDetailModal, WmsInfoGrid, WmsSubTable } from './wms-ui';

// 出库单、波次与入库预报已经各有自己的单据页（OutboundDocument、WaveDocument、PreAdviceDocument）。
// 这里余下的三种还是弹窗，跟着仓配界面重构的下一笔改成单据页。

function loadGoods(key: string) {
  const splitAt = key.indexOf('\n');
  return getGoodsCurrent(key.slice(0, splitAt), key.slice(splitAt + 1));
}

export function GoodsDetailModal({
  ownerAccount,
  goodsCode,
  onClose,
  onRevise,
}: {
  ownerAccount: string;
  goodsCode: string | null;
  onClose: () => void;
  onRevise?: (goods: GoodsRecord) => void;
}) {
  const key = goodsCode ? `${ownerAccount}\n${goodsCode}` : null;
  const { answer, loading } = useKeyedLoad(key, loadGoods);
  const failure = readFailureMessage(answer);
  const goods = answer?.kind === 'outcome' ? answer.body.goods : undefined;
  const missing = answer?.kind === 'outcome' && answer.body.found === false ? '没有这个商品' : null;

  return (
    <WmsDetailModal
      open={goodsCode !== null}
      title={goods?.goodsCode ?? goodsCode ?? ''}
      subtitle={goods?.name}
      badge={goods ? <WmsBadge label={`修订 r${goods.revision}`} tone="info" /> : undefined}
      loading={loading}
      error={failure ?? missing}
      actions={
        goods && onRevise ? (
          <Button type="button" variant="outline" onClick={() => onRevise(goods)}>
            代录新修订
          </Button>
        ) : undefined
      }
      onClose={onClose}
    >
      {goods ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '货主', value: goods.ownerAccount, mono: true },
              { label: '名称', value: goods.name },
              { label: '基本单位', value: goods.baseUnit },
              { label: '批次', value: goods.trackBatch ? '记批' : '不记批' },
              { label: '效期', value: goods.trackExpiry ? '记效期' : '不记效期' },
              { label: '组合', value: goods.composite ? labelOf(goods.composition?.outboundMode) : '否' },
              { label: '申报品名', value: goods.declaration?.name || '—' },
              { label: '登记', value: `${goods.registrant} · ${formatInstant(goods.registeredAt)}` },
            ]}
          />
          <WmsSubTable
            title="包装层级"
            count={goods.packagingLevels?.length ?? 0}
            emptyText="没有登记包装层级。"
            rows={goods.packagingLevels ?? []}
            rowKey={(line) => line.name}
            columns={[
              { id: 'name', header: '层级', render: (line) => line.name },
              { id: 'factor', header: '含基本单位', align: 'right', mono: true, render: (line) => String(line.factor) },
            ]}
          />
          <WmsSubTable
            title="别名"
            count={goods.aliases?.length ?? 0}
            emptyText="没有登记条码或平台码。"
            rows={goods.aliases ?? []}
            rowKey={(line, index) => `${line.code}-${index}`}
            columns={[
              { id: 'code', header: '编码', mono: true, render: (line) => line.code },
              { id: 'type', header: '种类', render: (line) => labelOf(line.type) },
              { id: 'platform', header: '平台', render: (line) => line.platform || '—' },
            ]}
          />
          <WmsSubTable
            title="组合组件"
            count={goods.composition?.components.length ?? 0}
            emptyText="不是组合商品。"
            rows={goods.composition?.components ?? []}
            rowKey={(line) => line.code}
            columns={[
              { id: 'code', header: '组件', mono: true, render: (line) => line.code },
              { id: 'qty', header: '每件用量', align: 'right', mono: true, render: (line) => String(line.quantity) },
            ]}
          />
          <WmsSubTable
            title="申报与实测"
            count={goods.warehouseMeasurements?.length ?? goods.declaredMeasurements?.length ?? 0}
            emptyText="没有尺寸重量。"
            rows={goods.warehouseMeasurements ?? []}
            rowKey={(line, index) => `${line.level}-${index}`}
            columns={[
              { id: 'level', header: '层级', render: (line) => line.level },
              { id: 'measured', header: '已实测', render: (line) => (line.measured ? '是' : '否') },
              {
                id: 'size',
                header: '当前尺寸 / 重量',
                render: (line) =>
                  line.current
                    ? `${line.current.dimensions.length}×${line.current.dimensions.width}×${line.current.dimensions.height} ${line.current.dimensions.unit} · ${line.current.weight.value} ${line.current.weight.unit}`
                    : '—',
              },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

export function ImportDetailModal({
  row,
  canDecide,
  onClose,
  onAction,
}: {
  row: StockImportRecord | null;
  canDecide: boolean;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
}) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.importId ?? ''}
      subtitle={row?.reason}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      actions={
        row && row.status === 'PENDING' && canDecide ? (
          <>
            <Button type="button" onClick={() => onAction('approve', { importId: row.importId })}>
              批准并入账
            </Button>
            <Button type="button" variant="outline" onClick={() => onAction('reject', { importId: row.importId })}>
              驳回
            </Button>
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
              { label: '提交人', value: row.submitter, mono: true },
              { label: '提交时刻', value: formatInstant(row.submittedAt), mono: true },
              { label: '批准人', value: row.poster || '—' },
              { label: '入账时刻', value: row.postedAt ? formatInstant(row.postedAt) : '—', mono: true },
              { label: '驳回人', value: row.rejector || '—' },
              { label: '驳回原因', value: row.rejectionNote || '—' },
            ]}
          />
          <WmsSubTable
            title="商品行"
            count={row.goodsLines.length}
            emptyText="没有按商品的建账行。"
            rows={row.goodsLines}
            rowKey={(line, index) => `${line.locationCode}-${line.goodsCode}-${index}`}
            columns={[
              { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'state', header: '状态', render: (line) => <WmsBadge code={line.stockState} /> },
              { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
              { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => `${line.quantity} ${line.unit}` },
            ]}
          />
          <WmsSubTable
            title="整箱行"
            count={row.cartonLines.length}
            emptyText="没有整箱建账行。"
            rows={row.cartonLines}
            rowKey={(line) => line.cartonNumber}
            columns={[
              { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
              { id: 'carton', header: '箱号', mono: true, render: (line) => line.cartonNumber },
              { id: 'bill', header: '箱内', render: (line) => line.contents.map((item) => `${item.goodsCode}×${item.quantity}`).join('、') },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

export function UnclaimedDetailModal({ row, onClose }: { row: UnclaimedArrivalRecord | null; onClose: () => void }) {
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.arrivalId ?? ''}
      subtitle={row?.factId}
      badge={row ? <WmsBadge code={row.status} /> : undefined}
      onClose={onClose}
    >
      {row ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '仓库', value: row.warehouseCode, mono: true },
              { label: '种类', value: labelOf(row.kind) },
              { label: '现场观察', value: row.observation || '—' },
              { label: '货主线索', value: row.ownerClue || '—', mono: true },
              { label: '预报线索', value: row.preAdviceClue || '—', mono: true },
              { label: '暂存库位', value: row.storageLocation || '—', mono: true },
              { label: '登记设备', value: row.device, mono: true },
              { label: '登记人', value: row.operator, mono: true },
              { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
              { label: '收到时刻', value: formatInstant(row.receivedAt), mono: true },
              { label: '入账引用', value: row.postingReferences?.join('、') || '—', mono: true },
            ]}
          />
          <WmsSubTable
            title="认领记录"
            count={row.claims?.length ?? 0}
            emptyText="还没有人认领过这件到货。"
            rows={row.claims ?? []}
            rowKey={(entry) => entry.claim.decisionId}
            columns={[
              { id: 'owner', header: '认领货主', mono: true, render: (entry) => entry.claim.ownerAccount },
              { id: 'advice', header: '认给预报', mono: true, render: (entry) => entry.claim.preAdviceId || '—' },
              { id: 'who', header: '认领人', mono: true, render: (entry) => entry.claim.operator },
              { id: 'at', header: '认领时刻', mono: true, render: (entry) => formatInstant(entry.claim.claimedAt) },
              {
                id: 'revoked',
                header: '撤销',
                render: (entry) =>
                  entry.revocation
                    ? `${entry.revocation.operator} · ${formatInstant(entry.revocation.revokedAt)}${entry.revocation.reason ? ` · ${entry.revocation.reason}` : ''}`
                    : '—',
              },
            ]}
          />
          {row.disposition ? (
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">处置</h3>
              <WmsInfoGrid
                items={[
                  { label: '处置方式', value: labelOf(row.disposition.method) },
                  { label: '依据规则', value: `${row.disposition.ruleWarehouseCode} r${row.disposition.ruleRevision}`, mono: true },
                  { label: '处置人', value: row.disposition.operator, mono: true },
                  { label: '处置时刻', value: formatInstant(row.disposition.disposedAt), mono: true },
                  { label: '说明', value: row.disposition.note || '—' },
                ]}
              />
            </section>
          ) : null}
        </>
      ) : null}
    </WmsDetailModal>
  );
}
