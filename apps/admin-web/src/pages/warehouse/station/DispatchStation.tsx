import { useState } from 'react';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { ScanBox } from './ScanBox';
import { ScanFeedback } from './ScanFeedback';
import { StationFrame, stationActionClass, stationPrimaryClass } from './StationFrame';
import { useDispatchSession } from './use-dispatch-session';
import { useStationSound } from './use-station-sound';

export function DispatchStation({
  device,
  requestId,
  onChangeDevice,
}: {
  device: WorkDeviceRecord;
  requestId: string | null;
  onChangeDevice?: () => void;
}) {
  const sound = useStationSound();
  const session = useDispatchSession({
    deviceId: device.deviceId,
    warehouse: device.workScopeRef,
    requestId,
    cue: sound.cue,
  });
  const state = session.state;
  const busy = state.phase === 'finding' || state.phase === 'opening' || state.phase === 'submitting';

  return (
    <StationFrame
      title="发货交接"
      device={{ id: device.deviceId, warehouse: device.workScopeRef }}
      onChangeDevice={onChangeDevice}
      context={state.sheet ? `${state.sheet.number} · ${state.sheet.dispatchLabel}` : '还没选定出库单'}
      progress={state.handed > 0 ? `这一趟已交 ${state.handed} 个包裹` : undefined}
      sound={{ on: sound.on, onToggle: sound.toggle }}
      left={
        <div className="flex flex-col gap-4">
          <ScanFeedback kind={state.feedback.tone} text={`${state.feedback.headline}。${state.feedback.detail}`} />
          <ScanBox onScan={session.scan} placeholder="扫出库单号" busy={busy} suspended={false} tone={state.feedback.tone} scanned={state.scanned} />
          {state.phase === 'handing' && state.sheet?.pickup ? <PickupFields onChange={session.setPickup} /> : null}
          <div className="flex flex-wrap gap-2">
            {state.phase === 'handing' ? (
              <button type="button" className={stationPrimaryClass} onClick={session.confirm}>交出这个包裹</button>
            ) : null}
            {state.phase === 'handing' ? (
              <button type="button" className={stationActionClass} onClick={session.offline}>离线已交出</button>
            ) : null}
            {state.phase === 'unsent' ? (
              <button type="button" className={stationPrimaryClass} onClick={session.resend}>再送一次</button>
            ) : null}
            {state.sheet ? (
              <button type="button" className={stationActionClass} onClick={session.leave}>换一张单</button>
            ) : null}
          </div>
        </div>
      }
      right={
        state.sheet ? (
          <div className="flex flex-col gap-2">
            <div className="type-body font-semibold">这张单的包裹</div>
            {state.sheet.parcels.map((parcel) => (
              <div key={parcel.sequence} className="type-body">
                包裹 {parcel.sequence}{parcel.done ? ' · 已交出' : ' · 等交出'}
                <div className="type-caption text-idpxyz-textMuted">{parcel.summary}</div>
              </div>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="type-caption text-idpxyz-textMuted">{session.queueNote}</div>
            {session.queue.slice(0, 50).map((row) => (
              <button key={row.requestId} type="button" className={`${stationActionClass} h-auto justify-between py-2 text-left`} onClick={() => session.openRequest(row.requestId)}>
                <span>{row.documentNumber ?? row.requestId}</span>
                <span className="type-caption">{row.dispatch === 'PICKUP' ? '到仓自提' : row.dispatch === 'OWNER_LABEL' ? '货主面单' : '本产品网络'}</span>
              </button>
            ))}
          </div>
        )
      }
    />
  );
}

function PickupFields({ onChange }: { onChange: (party: string, credential: string) => void }) {
  const [party, setParty] = useState('');
  const [credential, setCredential] = useState('');
  return (
    <div className="flex flex-col gap-2">
      <label className="type-body">提货方 <input className="ml-2 h-10 rounded border border-idpxyz-border bg-idpxyz-editor px-2" value={party} onChange={(event) => { setParty(event.target.value); onChange(event.target.value, credential); }} /></label>
      <label className="type-body">自提凭证 <input className="ml-2 h-10 rounded border border-idpxyz-border bg-idpxyz-editor px-2" value={credential} onChange={(event) => { setCredential(event.target.value); onChange(party, event.target.value); }} /></label>
    </div>
  );
}
