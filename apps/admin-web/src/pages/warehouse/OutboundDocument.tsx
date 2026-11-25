import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import { formatInstant } from '../catalogue-view';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getOutboundRequestDetail, type OutboundRequestDetailBody } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { outboundNextStep, outboundReservationOpen } from './outbound-next-step';
import { outboundTrack } from './outbound-track';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable } from './wms-ui';

const shipmentModule = 'shipment-request-inquiry';

function uniq(values: readonly (string | undefined)[]): string[] {
  return [...new Set(values.filter((value): value is string => value !== undefined && value !== ''))];
}

/** 读口的行号从 0 起，给人看的从 1 起。 */
const lineNumber = (lineIndex: number) => String(lineIndex + 1);

/**
 * 出库单页。一张出库单从受理到出库的全部：要多少、占了多少、拣货任务、复核、包裹、出库确认与回库。
 *
 * 没发生过的那几块不画（还没放波次就没有拣货任务那一块）：走到哪一步由头区下面的进度条说，正文只摆已经有的东西。
 * 明细行例外，它是单据本身。
 */
export function OutboundDocument({ requestId }: { requestId: string }) {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const back = useReturnToList('warehouse-outbound');
  const { answer, loading, reload } = useKeyedLoad(requestId, getOutboundRequestDetail);
  const body: OutboundRequestDetailBody | undefined = answer?.kind === 'outcome' ? answer.body : undefined;
  const request = body?.request;
  const failure = readFailureMessage(answer) ?? (body && !request ? `没有 ${requestId} 这张出库单。它可能属于别的公司，或标识有误。` : null);

  const agentWarehouse =
    request !== undefined && drawer.lookups.warehouses.some((w) => w.warehouseCode === request.warehouseCode && w.kind === 'AGENT');
  const track = request ? outboundTrack(request.status, agentWarehouse) : null;
  // 去拣货只做这张单所在的那个波次；一张单放进过几个波次时不替人挑，按本仓全部波次
  const onlyWave = body?.waves?.length === 1 ? body.waves[0] : undefined;
  const next = request ? outboundNextStep(request.status, agentWarehouse, onlyWave) : null;

  const reservations = body?.reservations ?? [];
  const reservedByLine = new Map<number, number>();
  for (const line of reservations) reservedByLine.set(line.lineIndex, (reservedByLine.get(line.lineIndex) ?? 0) + line.quantity);
  // 明细读口的行带行号、批号与效期；它没答时退到列表行上那几格。
  const lines =
    body?.lines ??
    (request?.lines ?? []).map((line, index) => ({ ...line, lineIndex: index, batchNumber: undefined, expiryDate: undefined }));
  const pickTasks = body?.pickTasks ?? [];
  const checks = body?.checks ?? [];
  const parcels = body?.parcels ?? [];
  const confirmations = body?.confirmations ?? [];
  const returnTasks = body?.returnTasks ?? [];
  const picked = pickTasks.reduce((sum, task) => sum + task.picked, 0);
  // 要发按明细行合计：代理仓的单不预占，读口答的预占需求是 0。
  const wanted = lines.reduce((sum, line) => sum + line.quantity, 0);
  // 拣出去的那部分预占已经放开，账上不再占着；所以「占没占够」只在拣完之前问，并把已拣的算进去。
  const reserved = request?.reservedQuantity ?? 0;
  const reservationOpen = request !== undefined && outboundReservationOpen(request.status);
  const short = reservationOpen && !agentWarehouse ? Math.max(0, wanted - reserved - picked) : 0;
  const heldReservations = reservations.filter((line) => line.quantity > 0);
  const shipments = uniq([request?.prefetchShipmentId, ...parcels.map((parcel) => parcel.shipmentRequestId)]);

  return (
    <>
      <WmsDocumentPage
        kind="出库单"
        title={request?.documentNumber ?? requestId}
        subtitle={
          request
            ? [
                `货主 ${request.ownerAccount}`,
                `仓 ${request.warehouseCode}`,
                labelOf(request.dispatch),
                `受理于 ${formatInstant(request.receivedAt)}`,
              ].join(' · ')
            : undefined
        }
        badge={request ? <WmsBadge code={request.status} /> : undefined}
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
          request ? (
            <>
              {request.dispatch === 'OWNER_LABEL' && drawer.has('owner-label') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('owner-label', { requestId: request.requestId })}>
                  登记面单
                </Button>
              ) : null}
              {(request.status === 'RESERVED' || request.status === 'PARTIALLY_RESERVED' || request.status === 'PENDING_POLICY') &&
              drawer.has('outbound-split') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('outbound-split', { requestId: request.requestId })}>
                  拆单
                </Button>
              ) : null}
              {request.status !== 'SHIPPED' && request.status !== 'CANCELLED' && drawer.has('outbound-cutoff') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('outbound-cutoff', { requestId: request.requestId })}>
                  截单
                </Button>
              ) : null}
              {next?.kind === 'wave' && drawer.has('wave-release') ? (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => drawer.open('wave-release', { warehouse: request.warehouseCode, requests: request.requestId })}
                >
                  {next.label}
                </Button>
              ) : null}
              {next?.kind === 'go' ? (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => {
                    window.location.hash = `#/${next.moduleId}`;
                  }}
                >
                  {next.label}
                </Button>
              ) : null}
              {next?.kind === 'station' ? (
                <Button type="button" className={primaryActionClass} onClick={() => enterStation(next.hash)}>
                  {next.label}
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          request ? (
            <>
              <WmsAsideBlock title="关联单据">
                {(body?.waves ?? []).map((waveId) => (
                  <div key={waveId} className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">波次</span>
                    <DocumentLink moduleId="warehouse-waves" parts={[waveId]} />
                  </div>
                ))}
                {request.parentRequestId ? (
                  <div className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">拆自</span>
                    <DocumentLink moduleId="warehouse-outbound" parts={[request.parentRequestId]} />
                  </div>
                ) : null}
                {shipments.map((shipment) => (
                  <div key={shipment} className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">运单委托</span>
                    <DocumentLink moduleId={shipmentModule} parts={[shipment]} />
                  </div>
                ))}
                {(body?.waves ?? []).length === 0 && !request.parentRequestId && shipments.length === 0 ? (
                  <span className="text-idpxyz-textMuted">还没有波次，也没有提交运单委托。</span>
                ) : null}
              </WmsAsideBlock>
              <WmsAsideBlock title="来源">
                <span>{labelOf(request.origin)}</span>
                <span className="break-all font-mono type-caption text-idpxyz-textMuted">来源键 {request.requestKey}</span>
                {request.operator ? <span className="break-all font-mono type-caption text-idpxyz-textMuted">代录人 {request.operator}</span> : null}
                {request.basis ? <span className="break-all type-caption text-idpxyz-textMuted">依据 {request.basis}</span> : null}
                {request.documentNumber ? (
                  <span className="break-all font-mono type-caption text-idpxyz-textMuted">内部标识 {request.requestId}</span>
                ) : (
                  <span className="type-caption text-idpxyz-textMuted">没有出库单号：{request.numberingAbsence ?? '编号规则没有给号'}</span>
                )}
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {request ? (
          <>
            <WmsKpiStrip
              items={[
                { label: '要发', value: wanted, note: `${lines.length} 行` },
                {
                  label: '预占中',
                  value: reserved,
                  note: agentWarehouse
                    ? '代理仓的单不预占'
                    : short > 0
                      ? `还有 ${short} 没占到`
                      : picked > 0
                        ? `另有 ${picked} 已拣出`
                        : reservationOpen
                          ? '已占满'
                          : undefined,
                  tone: short > 0 ? 'warning' : undefined,
                },
                { label: '已拣', value: picked, note: pickTasks.length > 0 ? `${pickTasks.length} 个拣货任务` : '还没有拣货任务' },
                { label: '包裹', value: parcels.length, note: confirmations.length > 0 ? `${confirmations.length} 次出库确认` : '还没有出库确认' },
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '收件人', value: request.recipient },
                { label: '地址', value: request.address },
                { label: '发运方式', value: labelOf(request.dispatch) },
                { label: '服务产品', value: request.service || '—', mono: true },
                { label: '平台备货', value: request.platformStocking ? '是，全部包裹打包后交一份委托' : '否' },
              ]}
            />
            <WmsSubTable
              title="明细行"
              count={lines.length}
              emptyText="这张出库单没有商品行。"
              rows={lines}
              rowKey={(line) => String(line.lineIndex)}
              columns={[
                { id: 'line', header: '行', mono: true, render: (line) => lineNumber(line.lineIndex) },
                { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
                { id: 'unit', header: '单位', render: (line) => line.unit },
                { id: 'state', header: '库存状态', render: (line) => <WmsBadge code={line.stockState} /> },
                { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
                { id: 'expiry', header: '效期', mono: true, render: (line) => line.expiryDate || '—' },
                { id: 'wanted', header: '要发', align: 'right', mono: true, render: (line) => String(line.quantity) },
                { id: 'reserved', header: '预占中', align: 'right', mono: true, render: (line) => String(reservedByLine.get(line.lineIndex) ?? 0) },
              ]}
            />
            {heldReservations.length > 0 ? (
              <WmsSubTable
                title="预占中的批次"
                count={heldReservations.length}
                emptyText=""
                rows={heldReservations}
                rowKey={(line, index) => `${line.lineIndex}-${index}`}
                columns={[
                  { id: 'line', header: '行', mono: true, render: (line) => lineNumber(line.lineIndex) },
                  { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
                  { id: 'state', header: '库存状态', render: (line) => <WmsBadge code={line.stockState} /> },
                  { id: 'alias', header: '贴标别名', mono: true, render: (line) => line.labelAlias || '—' },
                  { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
                  { id: 'expiry', header: '效期', mono: true, render: (line) => line.expiryDate || '—' },
                  { id: 'qty', header: '预占', align: 'right', mono: true, render: (line) => String(line.quantity) },
                ]}
              />
            ) : null}
            {pickTasks.length > 0 ? (
              <WmsSubTable
                title="拣货任务"
                count={pickTasks.length}
                emptyText=""
                rows={pickTasks}
                rowKey={(task) => task.taskId}
                columns={[
                  { id: 'task', header: '任务', mono: true, render: (task) => task.taskId },
                  { id: 'wave', header: '波次', render: (task) => <DocumentLink moduleId="warehouse-waves" parts={[task.waveId]} /> },
                  { id: 'loc', header: '库位', mono: true, render: (task) => task.locationCode },
                  { id: 'stage', header: '暂存位', mono: true, render: (task) => task.stagingLocation || '—' },
                  { id: 'goods', header: '商品', mono: true, render: (task) => task.goodsCode },
                  { id: 'qty', header: '已拣 / 应拣', align: 'right', mono: true, render: (task) => `${task.picked} / ${task.quantity}` },
                  { id: 'status', header: '状态', render: (task) => <WmsBadge code={task.status} /> },
                ]}
              />
            ) : null}
            {checks.length > 0 ? (
              <WmsSubTable
                title="复核"
                count={checks.length}
                emptyText=""
                rows={checks}
                rowKey={(check) => check.factId}
                columns={[
                  { id: 'at', header: '时刻', mono: true, render: (check) => formatInstant(check.checkedAt) },
                  { id: 'seen', header: '点到的数量', align: 'right', mono: true, render: (check) => String(check.observed) },
                  {
                    id: 'ok',
                    header: '与应拣',
                    render: (check) => <WmsBadge label={check.matched ? '相符' : '不符'} tone={check.matched ? 'success' : 'danger'} />,
                  },
                  { id: 'fact', header: '作业事实', mono: true, render: (check) => check.factId },
                ]}
              />
            ) : null}
            {parcels.length > 0 ? (
              <WmsSubTable
                title="包裹"
                count={parcels.length}
                emptyText=""
                rows={parcels}
                rowKey={(parcel) => String(parcel.parcelSequence)}
                columns={[
                  { id: 'seq', header: '序号', mono: true, render: (parcel) => String(parcel.parcelSequence) },
                  { id: 'pack', header: '包材', mono: true, render: (parcel) => parcel.packagingMaterial },
                  { id: 'weight', header: '重量', mono: true, render: (parcel) => `${parcel.weightValue} ${parcel.weightUnit}` },
                  {
                    id: 'size',
                    header: '尺寸',
                    mono: true,
                    render: (parcel) => `${parcel.lengthValue}×${parcel.widthValue}×${parcel.heightValue} ${parcel.dimensionUnit}`,
                  },
                  { id: 'lines', header: '内件', render: (parcel) => parcel.lines.map((item) => `${item.goodsCode}×${item.quantity}`).join('、') || '—' },
                  {
                    id: 'shipment',
                    header: '运单委托 / 面单',
                    render: (parcel) =>
                      parcel.shipmentRequestId ? (
                        <DocumentLink moduleId={shipmentModule} parts={[parcel.shipmentRequestId]} />
                      ) : (
                        <span className="font-mono">{parcel.ownerLabel || '—'}</span>
                      ),
                  },
                  {
                    id: 'state',
                    header: '出库确认',
                    // 截单拆掉的包裹先说已截单：它不会再有出库确认。
                    render: (parcel) =>
                      parcel.cutoffState ? (
                        <WmsBadge label="已截单" tone="neutral" />
                      ) : parcel.confirmationState ? (
                        <WmsBadge code={parcel.confirmationState} />
                      ) : (
                        <WmsBadge label="未确认" tone="warning" />
                      ),
                  },
                ]}
              />
            ) : null}
            {confirmations.length > 0 ? (
              <WmsSubTable
                title="出库确认"
                count={confirmations.length}
                emptyText=""
                rows={confirmations}
                rowKey={(confirmation) => confirmation.factId}
                columns={[
                  { id: 'at', header: '时刻', mono: true, render: (confirmation) => formatInstant(confirmation.occurredAt) },
                  { id: 'parcel', header: '包裹', mono: true, render: (confirmation) => String(confirmation.parcelSequence) },
                  { id: 'kind', header: '种类', render: (confirmation) => labelOf(confirmation.kind) },
                  { id: 'offline', header: '离线记的', render: (confirmation) => (confirmation.offline ? '是' : '否') },
                  { id: 'number', header: '外部单号', mono: true, render: (confirmation) => confirmation.externalNumber || '—' },
                  { id: 'fix', header: '更正', render: (confirmation) => (confirmation.correctionStatus ? labelOf(confirmation.correctionStatus) : '—') },
                  { id: 'fact', header: '作业事实', mono: true, render: (confirmation) => confirmation.factId },
                ]}
              />
            ) : null}
            {returnTasks.length > 0 ? (
              <WmsSubTable
                title="回库任务"
                count={returnTasks.length}
                emptyText=""
                rows={returnTasks}
                rowKey={(task) => task.returnId}
                columns={[
                  { id: 'id', header: '回库号', mono: true, render: (task) => task.returnId },
                  { id: 'loc', header: '所在库位', mono: true, render: (task) => task.locationCode },
                  { id: 'goods', header: '商品', mono: true, render: (task) => task.goodsCode },
                  { id: 'qty', header: '数量', align: 'right', mono: true, render: (task) => `${task.quantity} ${task.unit}` },
                  { id: 'status', header: '状态', render: (task) => <WmsBadge code={task.status} /> },
                ]}
              />
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer
        moduleId="warehouse-outbound"
        active={drawer.active}
        lookups={drawer.lookups}
        onClose={drawer.close}
        onSubmitted={reload}
      />
    </>
  );
}
