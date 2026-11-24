import { Button } from '@idpxyz/ui-primitives';
import { formatInstant } from '../catalogue-view';
import {
  getGoodsCurrent,
  getOutboundRequestDetail,
  getPreAdviceDetail,
  type GoodsRecord,
  type OutboundRequestDetailBody,
  type PreAdviceRecord,
  type StockImportRecord,
  type UnclaimedArrivalRecord,
  type WaveRecord,
} from './api';
import type { GuidedFormValues } from './guided-forms';
import { outboundTrack } from './outbound-track';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { labelOf, WmsBadge, WmsDetailModal, WmsInfoGrid, WmsStepper, WmsSubTable, type StepItem } from './wms-ui';

const preAdviceSteps: readonly StepItem[] = [
  { id: 'ADVISED', label: '已预报' },
  { id: 'RECEIVING', label: '收货中' },
  { id: 'CLOSED', label: '已完结' },
];

function loadPreAdvice(key: string) {
  const splitAt = key.indexOf('\n');
  return getPreAdviceDetail(key.slice(0, splitAt), key.slice(splitAt + 1));
}

function loadGoods(key: string) {
  const splitAt = key.indexOf('\n');
  return getGoodsCurrent(key.slice(0, splitAt), key.slice(splitAt + 1));
}

export function OutboundDetailModal({
  requestId,
  onClose,
  onAction,
  canWave,
  canSplit,
  canCutoff,
  canLabel,
  isAgentWarehouse,
}: {
  requestId: string | null;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
  canWave: boolean;
  canSplit: boolean;
  canCutoff: boolean;
  canLabel: boolean;
  isAgentWarehouse: (warehouseCode: string) => boolean;
}) {
  const { answer, loading } = useKeyedLoad(requestId, getOutboundRequestDetail);
  const failure = readFailureMessage(answer);
  const body: OutboundRequestDetailBody | undefined = answer?.kind === 'outcome' ? answer.body : undefined;
  const request = body?.request;
  const missing = body && !request ? '没有这张出库单' : null;
  const track = request ? outboundTrack(request.status, isAgentWarehouse(request.warehouseCode)) : null;
  const demandLines = (body?.lines ?? request?.lines ?? []).map((line) => ({
    goodsCode: line.goodsCode,
    unit: line.unit,
    stockState: line.stockState,
    quantity: line.quantity,
    batchNumber: 'batchNumber' in line ? line.batchNumber ?? '' : '',
  }));

  return (
    <WmsDetailModal
      open={requestId !== null}
      title={request?.documentNumber ?? request?.requestId ?? requestId ?? ''}
      subtitle={request?.requestId}
      badge={request ? <WmsBadge code={request.status} /> : undefined}
      loading={loading}
      error={failure ?? missing}
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
        request ? (
          <>
            {request.status === 'RESERVED' && canWave ? (
              <Button type="button" onClick={() => onAction('wave-release', { warehouse: request.warehouseCode, requests: request.requestId })}>
                单票放波次
              </Button>
            ) : null}
            {(request.status === 'RESERVED' || request.status === 'PARTIALLY_RESERVED' || request.status === 'PENDING_POLICY') && canSplit ? (
              <Button type="button" variant="outline" onClick={() => onAction('outbound-split', { requestId: request.requestId })}>
                拆单
              </Button>
            ) : null}
            {request.status !== 'SHIPPED' && request.status !== 'CANCELLED' && canCutoff ? (
              <Button type="button" variant="outline" onClick={() => onAction('outbound-cutoff', { requestId: request.requestId })}>
                截单
              </Button>
            ) : null}
            {request.dispatch === 'OWNER_LABEL' && canLabel ? (
              <Button type="button" variant="outline" onClick={() => onAction('owner-label', { requestId: request.requestId })}>
                登记面单
              </Button>
            ) : null}
          </>
        ) : undefined
      }
      onClose={onClose}
    >
      {request ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '货主', value: request.ownerAccount, mono: true },
              { label: '仓库', value: request.warehouseCode, mono: true },
              { label: '发货途径', value: labelOf(request.dispatch) },
              { label: '服务', value: request.service || '—' },
              { label: '收件人', value: request.recipient },
              { label: '地址', value: request.address },
              { label: '预占 / 需求', value: `${request.reservedQuantity} / ${request.wantedQuantity}`, mono: true },
              { label: '受理', value: formatInstant(request.receivedAt), mono: true },
            ]}
          />
          <WmsSubTable
            title="需求行"
            count={demandLines.length}
            emptyText="这张出库单没有商品行。"
            rows={demandLines}
            rowKey={(line, index) => `${line.goodsCode}-${index}`}
            columns={[
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'unit', header: '单位', render: (line) => line.unit },
              { id: 'state', header: '状态', render: (line) => <WmsBadge code={line.stockState} /> },
              { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
              { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
            ]}
          />
          <WmsSubTable
            title="预占"
            count={body?.reservations?.length ?? 0}
            emptyText="还没有预占到具体批次。"
            rows={body?.reservations ?? []}
            rowKey={(line, index) => `${line.goodsCode}-${line.lineIndex}-${index}`}
            columns={[
              { id: 'line', header: '行', mono: true, render: (line) => String(line.lineIndex) },
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'state', header: '状态', render: (line) => <WmsBadge code={line.stockState} /> },
              { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
              { id: 'qty', header: '预占', align: 'right', mono: true, render: (line) => String(line.quantity) },
              { id: 'wanted', header: '需求', align: 'right', mono: true, render: (line) => String(line.wanted) },
            ]}
          />
          <WmsSubTable
            title="拣货任务"
            count={body?.pickTasks?.length ?? 0}
            extra={body?.waves && body.waves.length > 0 ? <span className="type-caption text-idpxyz-textMuted">波次 {body.waves.join('、')}</span> : undefined}
            emptyText="还没有放入波次。"
            rows={body?.pickTasks ?? []}
            rowKey={(line) => line.taskId}
            columns={[
              { id: 'task', header: '任务', mono: true, render: (line) => line.taskId },
              { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'qty', header: '已拣 / 应拣', align: 'right', mono: true, render: (line) => `${line.picked} / ${line.quantity}` },
              { id: 'status', header: '状态', render: (line) => <WmsBadge code={line.status} /> },
            ]}
          />
          <WmsSubTable
            title="复核"
            count={body?.checks?.length ?? 0}
            emptyText="还没有复核记录。"
            rows={body?.checks ?? []}
            rowKey={(line) => line.factId}
            columns={[
              { id: 'fact', header: '事实号', mono: true, render: (line) => line.factId },
              { id: 'seen', header: '观察到', align: 'right', mono: true, render: (line) => String(line.observed) },
              { id: 'ok', header: '相符', render: (line) => (line.matched ? '相符' : '不符') },
              { id: 'at', header: '时刻', mono: true, render: (line) => formatInstant(line.checkedAt) },
            ]}
          />
          <WmsSubTable
            title="包裹"
            count={body?.parcels?.length ?? 0}
            emptyText="还没有打包。"
            rows={body?.parcels ?? []}
            rowKey={(line) => String(line.parcelSequence)}
            columns={[
              { id: 'seq', header: '序号', mono: true, render: (line) => String(line.parcelSequence) },
              { id: 'pack', header: '包材', render: (line) => line.packagingMaterial },
              { id: 'weight', header: '重量', mono: true, render: (line) => `${line.weightValue} ${line.weightUnit}` },
              { id: 'lines', header: '内件', render: (line) => line.lines.map((item) => `${item.goodsCode}×${item.quantity}`).join('、') || '—' },
              { id: 'label', header: '面单', mono: true, render: (line) => line.ownerLabel || line.shipmentRequestId || '—' },
            ]}
          />
          <WmsSubTable
            title="出库确认"
            count={body?.confirmations?.length ?? 0}
            emptyText="还没有出库确认。"
            rows={body?.confirmations ?? []}
            rowKey={(line) => line.factId}
            columns={[
              { id: 'fact', header: '事实号', mono: true, render: (line) => line.factId },
              { id: 'kind', header: '种类', render: (line) => labelOf(line.kind) },
              { id: 'parcel', header: '包裹', mono: true, render: (line) => String(line.parcelSequence) },
              { id: 'at', header: '时刻', mono: true, render: (line) => formatInstant(line.occurredAt) },
              { id: 'fix', header: '更正', render: (line) => line.correctionStatus ? labelOf(line.correctionStatus) : '—' },
            ]}
          />
          <WmsSubTable
            title="回库任务"
            count={body?.returnTasks?.length ?? 0}
            emptyText="没有待回库的截单或短拣库存。"
            rows={body?.returnTasks ?? []}
            rowKey={(line) => line.returnId}
            columns={[
              { id: 'id', header: '回库号', mono: true, render: (line) => line.returnId },
              { id: 'loc', header: '库位', mono: true, render: (line) => line.locationCode },
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
              { id: 'status', header: '状态', render: (line) => <WmsBadge code={line.status} /> },
            ]}
          />
        </>
      ) : null}
    </WmsDetailModal>
  );
}

export function PreAdviceDetailModal({
  ownerAccount,
  preAdviceId,
  onClose,
  onAction,
  canNotice,
  canWithdraw,
  canClose,
}: {
  ownerAccount: string;
  preAdviceId: string | null;
  onClose: () => void;
  onAction: (id: string, prefill?: GuidedFormValues) => void;
  canNotice: boolean;
  canWithdraw: boolean;
  canClose: boolean;
}) {
  const key = preAdviceId ? `${ownerAccount}\n${preAdviceId}` : null;
  const { answer, loading } = useKeyedLoad(key, loadPreAdvice);
  const failure = readFailureMessage(answer);
  const record: PreAdviceRecord | undefined = answer?.kind === 'outcome' ? answer.body.preAdvice : undefined;
  const missing = answer?.kind === 'outcome' && answer.body.found === false ? '没有这张入库预报' : null;

  return (
    <WmsDetailModal
      open={preAdviceId !== null}
      title={record?.documentNumber ?? record?.preAdviceId ?? preAdviceId ?? ''}
      subtitle={record?.preAdviceId}
      badge={record ? <WmsBadge code={record.status} /> : undefined}
      loading={loading}
      error={failure ?? missing}
      stepper={
        record ? (
          <WmsStepper
            steps={preAdviceSteps}
            currentId={record.status === 'CANCELLED' ? 'ADVISED' : record.status}
            failedLabel={record.status === 'CANCELLED' ? '已取消' : undefined}
          />
        ) : undefined
      }
      actions={
        record ? (
          <>
            {record.status === 'ADVISED' || record.status === 'RECEIVING' ? (
              <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
                去作业台收货
              </Button>
            ) : null}
            {canNotice && record.status !== 'CLOSED' && record.status !== 'CANCELLED' ? (
              <Button type="button" variant="outline" onClick={() => onAction('pre-advice-notice', { preAdviceId: record.preAdviceId })}>
                更新告知
              </Button>
            ) : null}
            {canWithdraw && record.status === 'ADVISED' ? (
              <Button type="button" variant="outline" onClick={() => onAction('pre-advice-withdrawal', { preAdviceId: record.preAdviceId })}>
                撤回
              </Button>
            ) : null}
            {canClose && record.status === 'RECEIVING' ? (
              <Button type="button" onClick={() => onAction('pre-advice-close', { preAdviceId: record.preAdviceId })}>
                完结
              </Button>
            ) : null}
          </>
        ) : undefined
      }
      onClose={onClose}
    >
      {record ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '货主', value: record.ownerAccount, mono: true },
              { label: '仓库', value: record.warehouseCode, mono: true },
              { label: '到仓方式', value: labelOf(record.arrivalMode) },
              { label: '入库形态', value: labelOf(record.inboundMode) },
              { label: '预计到达', value: record.currentNotice ? formatInstant(record.currentNotice.expectedAt) : '—', mono: true },
              { label: '到货线索', value: record.currentNotice?.arrivalClue || '—' },
              { label: '货主参考号', value: record.currentNotice?.ownerReference || '—', mono: true },
              { label: '受理', value: `${record.registrant} · ${formatInstant(record.acceptedAt)}` },
            ]}
          />
          <WmsSubTable
            title="商品行"
            count={record.goodsLines?.length ?? 0}
            emptyText="这张预报没有按商品的行。"
            rows={record.goodsLines ?? []}
            rowKey={(line, index) => `${line.goodsCode}-${index}`}
            columns={[
              { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
              { id: 'unit', header: '单位', render: (line) => line.unit },
              { id: 'qty', header: '预报数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
              { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
              { id: 'exp', header: '效期', mono: true, render: (line) => line.expiryDate || '—' },
            ]}
          />
          <WmsSubTable
            title="整箱"
            count={record.cartons?.length ?? 0}
            emptyText="这张预报没有整箱。"
            rows={record.cartons ?? []}
            rowKey={(line) => line.cartonNumber}
            columns={[
              { id: 'carton', header: '箱号', mono: true, render: (line) => line.cartonNumber },
              { id: 'lines', header: '箱内', render: (line) => line.lines.map((item) => `${item.goodsCode}×${item.quantity}`).join('、') },
            ]}
          />
          <WmsSubTable
            title="已收进度"
            count={record.cumulativeReceived?.length ?? 0}
            emptyText="还没有收货。"
            rows={record.cumulativeReceived ?? []}
            rowKey={(line, index) => `${line.goodsCode ?? line.cartonNumber ?? 'recv'}-${index}`}
            columns={[
              { id: 'goods', header: '商品 / 箱', mono: true, render: (line) => line.goodsCode || line.cartonNumber || '—' },
              { id: 'qty', header: '累计实收', align: 'right', mono: true, render: (line) => String(line.quantity) },
            ]}
          />
          <WmsSubTable
            title="收货事实"
            count={record.receipts?.length ?? 0}
            emptyText="还没有收货事实。"
            rows={record.receipts ?? []}
            rowKey={(line) => line.factId}
            columns={[
              { id: 'fact', header: '事实号', mono: true, render: (line) => line.factId },
              { id: 'shape', header: '形态', render: (line) => labelOf(line.shape) },
              { id: 'outcome', header: '结果', render: (line) => labelOf(line.outcome) },
              { id: 'lines', header: '行数', mono: true, render: (line) => String((line.lines?.length ?? 0) + (line.cartonLines?.length ?? 0)) },
              { id: 'at', header: '时刻', mono: true, render: (line) => formatInstant(line.occurredAt) },
            ]}
          />
          {record.closure ? (
            <WmsInfoGrid
              items={[
                { label: '完结时刻', value: formatInstant(record.closure.closedAt), mono: true },
                { label: '完结人', value: record.closure.operator || '—', mono: true },
                { label: '少收', value: record.closure.shortages?.map((item) => `${item.goodsCode || item.cartonNumber || '项'}×${item.quantity ?? 0}`).join('、') || '无' },
                { label: '说明', value: record.closure.note || '—' },
              ]}
            />
          ) : null}
        </>
      ) : null}
    </WmsDetailModal>
  );
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

export function WaveDetailModal({ row, onClose }: { row: WaveRecord | null; onClose: () => void }) {
  const done = row?.tasks.filter((task) => task.status !== 'OPEN').length ?? 0;
  return (
    <WmsDetailModal
      open={row !== null}
      title={row?.waveId ?? ''}
      subtitle={row ? `${row.warehouseCode} · ${row.requests.length} 张出库单` : undefined}
      badge={row ? <WmsBadge label={done === row.tasks.length && row.tasks.length > 0 ? '拣货完成' : '拣货中'} tone={done === row.tasks.length && row.tasks.length > 0 ? 'success' : 'info'} /> : undefined}
      actions={
        <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
          去作业台
        </Button>
      }
      onClose={onClose}
    >
      {row ? (
        <>
          <WmsInfoGrid
            items={[
              { label: '仓库', value: row.warehouseCode, mono: true },
              { label: '出库单', value: row.requests.join('、') || '—', mono: true },
              { label: '拣货进度', value: `${done} / ${row.tasks.length}`, mono: true },
              { label: '预取面单', value: row.prefetch ? '已开启' : '未开启' },
              { label: '释放人', value: row.operator, mono: true },
              { label: '释放时刻', value: formatInstant(row.releasedAt), mono: true },
            ]}
          />
          <WmsSubTable
            title="拣货任务"
            count={row.tasks.length}
            emptyText="这个波次没有拣货任务。"
            rows={row.tasks}
            rowKey={(task) => task.taskId}
            columns={[
              { id: 'task', header: '任务', mono: true, render: (task) => task.taskId },
              { id: 'req', header: '出库单', mono: true, render: (task) => task.requestId },
              { id: 'loc', header: '库位', mono: true, render: (task) => task.locationCode },
              { id: 'stage', header: '暂存', mono: true, render: (task) => task.stagingLocation || '—' },
              { id: 'goods', header: '商品', mono: true, render: (task) => task.goodsCode },
              { id: 'qty', header: '已拣 / 应拣', align: 'right', mono: true, render: (task) => `${task.picked} / ${task.quantity}` },
              { id: 'status', header: '状态', render: (task) => <WmsBadge code={task.status} /> },
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
