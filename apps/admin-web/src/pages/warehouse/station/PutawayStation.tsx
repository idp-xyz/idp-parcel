import type { WorkDeviceRecord } from '../../work-devices/api';
import { ScanBox } from './ScanBox';
import { ScanFeedback } from './ScanFeedback';
import { StationFrame, stationActionClass, stationPrimaryClass } from './StationFrame';
import { usePutawaySession } from './use-putaway-session';
import { useStationSound } from './use-station-sound';

const stateWords: Record<string, string> = { GOOD: '良品', PENDING_INSPECTION: '待检', DEFECTIVE: '次品' };

export function PutawayStation({ device, onChangeDevice }: { device: WorkDeviceRecord; onChangeDevice?: () => void }) {
  const sound = useStationSound();
  const session = usePutawaySession({ deviceId: device.deviceId, warehouse: device.workScopeRef, cue: sound.cue });
  const state = session.state;
  const busy = state.phase === 'verifying' || state.phase === 'submitting';
  return (
    <StationFrame
      title="上架与移库"
      device={{ id: device.deviceId, warehouse: device.workScopeRef }}
      onChangeDevice={onChangeDevice}
      context={state.owner ? `货主 ${state.owner}` : '还没选货主'}
      progress={state.moved > 0 ? `这一趟已移 ${state.moved} 笔` : undefined}
      sound={{ on: sound.on, onToggle: sound.toggle }}
      left={
        <div className="flex flex-col gap-4">
          <ScanFeedback kind={state.feedback.tone} text={`${state.feedback.headline}。${state.feedback.detail}`} />
          {state.phase === 'owner' ? (
            <div className="flex flex-col gap-2">
              <div className="type-caption text-idpxyz-textMuted">{session.ownerNote}</div>
              {session.owners.map((owner) => (
                <button key={owner} type="button" className={stationActionClass} onClick={() => session.chooseOwner(owner)}>{owner}</button>
              ))}
            </div>
          ) : (
            <ScanBox
              onScan={session.scan}
              placeholder={state.phase === 'source' ? '扫源库位' : state.draft ? '扫目标库位，或再扫商品加一' : '扫商品或箱号'}
              busy={busy}
              suspended={false}
              tone={state.feedback.tone}
              scanned={state.scanned}
            />
          )}
          {state.draft && !state.draft.carton ? (
            <label className="type-body">
              数量
              <input
                className="ml-2 h-10 w-24 rounded border border-idpxyz-border bg-idpxyz-editor px-2"
                inputMode="numeric"
                value={String(state.draft.quantity)}
                onChange={(event) => session.setQuantity(Number(event.target.value))}
              />
            </label>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {state.phase === 'target' ? <button type="button" className={stationPrimaryClass} onClick={session.submit}>提交这一笔</button> : null}
            {state.phase === 'unsent' ? <button type="button" className={stationPrimaryClass} onClick={session.resend}>再送一次</button> : null}
            {state.owner && state.phase !== 'owner' ? <button type="button" className={stationActionClass} onClick={() => session.chooseOwner(state.owner as string)}>换货主</button> : null}
          </div>
        </div>
      }
      right={
        <div className="flex flex-col gap-2">
          <div className="type-body font-semibold">{state.source ? `源库位 ${state.source}` : '还没扫源库位'}</div>
          {state.cells.map((item) => (
            <div key={`${item.goodsCode}|${item.stockState}|${item.inboundBatchId ?? ''}`} className="type-body">
              {item.goodsCode} · {stateWords[item.stockState] ?? item.stockState} · 在库 {item.quantity}
            </div>
          ))}
          {state.cartons.map((item) => (
            <div key={item.cartonNumber} className="type-body">整箱 {item.cartonNumber}</div>
          ))}
        </div>
      }
    />
  );
}
