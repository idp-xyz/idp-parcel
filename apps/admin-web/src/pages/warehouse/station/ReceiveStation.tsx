import { useState } from 'react';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { labelOf } from '../wms-ui';
import { ScanBox } from './ScanBox';
import { ScanFeedback } from './ScanFeedback';
import { stationActionClass, stationButtonClass, stationPrimaryClass, StationFrame } from './StationFrame';
import { pendingLeave } from './station-leave';
import { useStationSound } from './use-station-sound';
import { goodsThisTime, receiptStockStates } from './receive-sheet';
import { receivesScan, type ReceiveState } from './receive-flow';
import { useReceiveSession, type ReceiveQueueItem } from './use-receive-session';

// 收货工位只摆。扫到的码算不算、该不该提交，都在 receive-flow.ts。

export function ReceiveStation({
  device,
  adviceId,
  onChangeDevice,
}: {
  device: WorkDeviceRecord;
  adviceId: string | null;
  onChangeDevice?: () => void;
}) {
  const sound = useStationSound();
  const session = useReceiveSession({
    deviceId: device.deviceId,
    warehouse: device.workScopeRef,
    adviceId,
    cue: sound.cue,
  });
  const state = session.state;
  const asking = state.phase === 'asking';

  return (
    <StationFrame
      title="收货"
      device={{ id: device.deviceId, warehouse: device.workScopeRef }}
      onChangeDevice={onChangeDevice}
      context={state.advice ? (state.advice.documentNumber ?? state.advice.preAdviceId) : '还没选定预报'}
      progress={progressOf(state)}
      sound={{ on: sound.on, onToggle: sound.toggle }}
      confirmLeave={
        pendingLeave(
          state.phase,
          state.goods.reduce((sum, line) => sum + line.quantity, 0) + state.cartons.length,
        ) ?? undefined
      }
      left={
        <div className="flex flex-col gap-4">
          <ScanFeedback kind={state.feedback.tone} text={`${state.feedback.headline}。${state.feedback.detail}`} />
          <ScanBox
            onScan={session.scan}
            placeholder={placeholderOf(state)}
            busy={!receivesScan(state.phase)}
            suspended={asking}
            tone={state.feedback.tone}
            scanned={state.scanned}
          />
          {state.phase === 'state' || (state.phase === 'goods' && state.advice?.inboundMode === 'BY_GOODS') ? (
            <div className="flex flex-wrap gap-2">
              {receiptStockStates.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={state.stockState === option.value ? stationPrimaryClass : stationButtonClass}
                  onClick={() => session.chooseState(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          ) : null}
          {asking && state.ask ? (
            <AskForm
              needUnit={state.ask.needUnit}
              needBatch={state.ask.needBatch}
              needExpiry={state.ask.needExpiry}
              onCancel={session.cancelAsk}
              onConfirm={session.confirmAsk}
            />
          ) : null}
          {state.stray ? (
            <div className="flex flex-wrap gap-2">
              {state.advice?.inboundMode === 'BY_GOODS' ? (
                <button type="button" className={stationActionClass} onClick={session.acceptStray}>
                  照收
                </button>
              ) : null}
              <button type="button" className={stationActionClass} onClick={session.registerStray}>
                登记成待认领
              </button>
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {state.advice?.inboundMode === 'BY_CARTON' && state.phase === 'goods' ? (
              <button type="button" className={state.damageNext ? stationPrimaryClass : stationActionClass} onClick={session.toggleDamage}>
                外箱破损
              </button>
            ) : null}
            {state.phase === 'goods' ? (
              <button type="button" className={stationPrimaryClass} onClick={session.submit}>
                提交这一次收货
              </button>
            ) : null}
            {state.phase === 'unsent' ? (
              <button type="button" className={stationPrimaryClass} onClick={session.resend}>
                再送一次
              </button>
            ) : null}
            {state.advice ? (
              <button type="button" className={stationActionClass} onClick={session.leave}>
                换一张预报
              </button>
            ) : null}
          </div>
        </div>
      }
      right={
        state.advice ? (
          <AdviceSide state={state} />
        ) : (
          <QueueSide note={session.queueNote} items={session.queue} onOpen={session.openAdvice} />
        )
      }
    />
  );
}

function progressOf(state: ReceiveState): string {
  if (!state.advice) return '';
  if (state.advice.inboundMode === 'BY_CARTON') return `这一次 ${state.cartons.length} 只箱`;
  const count = state.goods.reduce((sum, line) => sum + line.quantity, 0);
  return `这一次 ${count} 件`;
}

function placeholderOf(state: ReceiveState): string {
  if (state.phase === 'locating' || state.phase === 'verifying') return '扫收货暂存库位';
  if (state.phase === 'goods' && state.advice?.inboundMode === 'BY_CARTON') return '扫箱号';
  if (state.phase === 'goods') return '扫商品';
  return '扫入库单号';
}

function AskForm({
  needUnit,
  needBatch,
  needExpiry,
  onCancel,
  onConfirm,
}: {
  needUnit: boolean;
  needBatch: boolean;
  needExpiry: boolean;
  onCancel: () => void;
  onConfirm: (batch: string, expiry: string, unit: string) => void;
}) {
  const [unit, setUnit] = useState('');
  const [batch, setBatch] = useState('');
  const [expiry, setExpiry] = useState('');
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        onConfirm(batch, expiry, unit);
      }}
    >
      {needUnit ? <label className="type-body">计量单位 <input className="ml-2 h-10 rounded border border-idpxyz-border bg-idpxyz-editor px-2" value={unit} onChange={(event) => setUnit(event.target.value)} /></label> : null}
      {needBatch ? <label className="type-body">批号 <input className="ml-2 h-10 rounded border border-idpxyz-border bg-idpxyz-editor px-2" value={batch} onChange={(event) => setBatch(event.target.value)} /></label> : null}
      {needExpiry ? <label className="type-body">失效日 <input type="date" className="ml-2 h-10 rounded border border-idpxyz-border bg-idpxyz-editor px-2" value={expiry} onChange={(event) => setExpiry(event.target.value)} /></label> : null}
      <div className="flex gap-2">
        <button type="submit" className={stationPrimaryClass}>记下</button>
        <button type="button" className={stationActionClass} onClick={onCancel}>取消</button>
      </div>
    </form>
  );
}

function AdviceSide({ state }: { state: ReceiveState }) {
  const advice = state.advice;
  if (!advice) return null;
  return (
    <div className="flex flex-col gap-3">
      <div className="type-body font-semibold">{advice.inboundMode === 'BY_CARTON' ? '这张预报的箱' : '这张预报的货'}</div>
      {advice.late ? <div className="type-caption">{advice.late}</div> : null}
      {advice.inboundMode === 'BY_GOODS'
        ? advice.goods.map((line) => (
            <div key={`${line.goodsCode}|${line.unit}`} className="flex justify-between gap-3 type-body">
              <span>{line.goodsCode}</span>
              <span>预报 {line.advised}，已收 {line.received}，这一次 {goodsThisTime(state.goods, line.goodsCode)}</span>
            </div>
          ))
        : advice.cartons.map((carton) => (
            <div key={carton.cartonNumber} className="type-body">
              <div>{carton.cartonNumber}{state.cartons.some((line) => line.cartonNumber === carton.cartonNumber) ? ' · 这一次已扫' : ''}</div>
              <div className="type-caption text-idpxyz-textMuted">{carton.summary}</div>
            </div>
          ))}
    </div>
  );
}

function QueueSide({ note, items, onOpen }: { note: string; items: ReceiveQueueItem[]; onOpen: (id: string) => void }) {
  return (
    <div className="flex flex-col gap-2">
      <div className="type-caption text-idpxyz-textMuted">{note}</div>
      {items.slice(0, 50).map((item) => (
        <button key={item.preAdviceId} type="button" className={`${stationButtonClass} h-auto justify-between py-2 text-left`} onClick={() => onOpen(item.preAdviceId)}>
          <span>{item.documentNumber ?? item.preAdviceId}</span>
          <span className="type-caption">{labelOf(item.status)}{item.late ? ` · ${item.late}` : ''}</span>
        </button>
      ))}
    </div>
  );
}
