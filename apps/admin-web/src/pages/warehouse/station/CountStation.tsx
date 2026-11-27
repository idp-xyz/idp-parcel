import type { WorkDeviceRecord } from '../../work-devices/api';
import { ScanBox } from './ScanBox';
import { ScanFeedback } from './ScanFeedback';
import { StationFrame, stationActionClass, stationPrimaryClass } from './StationFrame';
import { pendingLeave } from './station-leave';
import { useCountSession } from './use-count-session';
import { useStationSound } from './use-station-sound';

const states = [
  { value: 'GOOD', label: '良品' },
  { value: 'PENDING_INSPECTION', label: '待检' },
  { value: 'DEFECTIVE', label: '次品' },
];

export function CountStation({ device, countId, onChangeDevice }: { device: WorkDeviceRecord; countId: string | null; onChangeDevice?: () => void }) {
  const sound = useStationSound();
  const session = useCountSession({ deviceId: device.deviceId, warehouse: device.workScopeRef, countId, cue: sound.cue });
  const state = session.state;
  const busy = state.phase === 'opening' || state.phase === 'submitting';
  const left = state.sheet ? state.sheet.locations.length - state.sheet.counted.length : 0;
  return (
    <StationFrame
      title="盘点清点"
      device={{ id: device.deviceId, warehouse: device.workScopeRef }}
      onChangeDevice={onChangeDevice}
      context={state.sheet ? state.sheet.countId : '还没选定盘点单'}
      progress={state.sheet ? `已清点 ${state.sheet.counted.length} / ${state.sheet.locations.length}` : undefined}
      sound={{ on: sound.on, onToggle: sound.toggle }}
      confirmLeave={pendingLeave(state.phase, state.lines.reduce((sum, line) => sum + line.quantity, 0)) ?? undefined}
      left={
        <div className="flex flex-col gap-4">
          <ScanFeedback kind={state.feedback.tone} text={`${state.feedback.headline}。${state.feedback.detail}`} />
          <ScanBox onScan={session.scan} placeholder={state.phase === 'goods' ? '扫这个库位里的货' : '扫库位或盘点单号'} busy={busy} suspended={false} tone={state.feedback.tone} scanned={state.scanned} />
          {state.phase === 'goods' ? (
            <div className="flex flex-wrap gap-2">
              {states.map((option) => (
                <button key={option.value} type="button" className={state.stockState === option.value ? stationPrimaryClass : stationActionClass} onClick={() => session.chooseState(option.value)}>
                  {option.label}
                </button>
              ))}
            </div>
          ) : null}
          {state.lines.map((line, index) => (
            <label key={`${line.goodsCode ?? line.observed}|${index}`} className="type-body">
              {line.goodsCode ?? line.observed}
              <input className="ml-2 h-10 w-24 rounded border border-idpxyz-border bg-idpxyz-editor px-2" inputMode="numeric" value={String(line.quantity)} onChange={(event) => session.setQuantity(index, Number(event.target.value))} />
            </label>
          ))}
          <div className="flex flex-wrap gap-2">
            {state.phase === 'goods' ? <button type="button" className={stationPrimaryClass} onClick={session.submit}>提交这个库位</button> : null}
            {state.phase === 'goods' ? <button type="button" className={stationActionClass} onClick={session.empty}>这个库位是空的</button> : null}
            {state.phase === 'goods' ? <button type="button" className={stationActionClass} onClick={session.unknown}>认不出的货</button> : null}
            {state.phase === 'unsent' ? <button type="button" className={stationPrimaryClass} onClick={session.resend}>再送一次</button> : null}
          </div>
        </div>
      }
      right={
        state.sheet ? (
          <div className="flex flex-col gap-2">
            <div className="type-body font-semibold">盘点范围 · 还差 {left}</div>
            {state.sheet.locations.map((location) => (
              <div key={location} className="type-body">{location} · {state.location === location ? '正在点' : state.sheet?.counted.includes(location) ? '已清点' : '还没点'}</div>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="type-caption text-idpxyz-textMuted">{session.queueNote}</div>
            {session.queue.map((count) => (
              <button key={count.countId} type="button" className={`${stationActionClass} h-auto justify-between py-2 text-left`} onClick={() => session.openCount(count.countId)}>
                <span>{count.countId}</span>
                <span className="type-caption">{count.locations.length} 个库位</span>
              </button>
            ))}
          </div>
        )
      }
    />
  );
}
