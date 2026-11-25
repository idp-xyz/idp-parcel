import type { ReactNode } from 'react';
import { statusToneBanner } from '../../../components/status-tone';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { StationFrame, stationButtonClass } from './StationFrame';
import type { StationDevice } from './use-station-device';

// 工位绑设备：绑好了才往下渲工位；还没绑好时在这里把话说清——读着、读不出、没有可用设备、要选一台。
// 判断（只有一台直接用、几台让人选、没有就去登记）在 device-binding.ts，这里只摆。

const workDevicesHash = '#/work-devices';

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex max-w-xl flex-col gap-3 rounded-lg border border-idpxyz-border bg-idpxyz-editor p-5">
      <h2 className="type-section font-semibold text-idpxyz-textBright">{title}</h2>
      {children}
    </div>
  );
}

export interface ReadyDevice {
  device: WorkDeviceRecord;
  /** 有几台可选才给；只有一台时没有可换的。 */
  onChangeDevice?: () => void;
}

export function DeviceGate({ device, children }: { device: StationDevice; children: (ready: ReadyDevice) => ReactNode }) {
  const { state } = device;
  if (state.phase === 'ready') {
    return <>{children({ device: state.device, onChangeDevice: state.devices.length > 1 ? device.change : undefined })}</>;
  }
  return (
    <StationFrame
      title="作业台"
      left={
        state.phase === 'loading' ? (
          <p role="status" className="type-body text-idpxyz-textMuted">
            正在读在用的仓库扫描设备…
          </p>
        ) : state.phase === 'error' ? (
          <Card title="读不出设备名册">
            <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.danger}`}>
              {state.message}。
            </p>
            <div>
              <button type="button" className={stationButtonClass} onClick={device.reload}>
                再试一次
              </button>
            </div>
          </Card>
        ) : state.phase === 'none' ? (
          <Card title="还没有在用的仓库工作设备">
            <p className="type-body text-idpxyz-textMuted">
              作业台要绑一台归属具体仓库、登记在用的设备。先去「扫描设备」登记一台网页工位设备，并确认操作者已持有该仓「作业事实登记」的授予，再回来。
            </p>
            <div>
              <a className={stationButtonClass} href={workDevicesHash}>
                去扫描设备登记
              </a>
            </div>
          </Card>
        ) : (
          <Card title="选这台电脑用的设备">
            <p className="type-body text-idpxyz-textMuted">
              选一次，这台浏览器记住它；之后这个工位只做那个仓的事。要换随时可以在顶条上点「换设备」。
            </p>
            <ul className="flex flex-col gap-2">
              {state.devices.map((candidate) => (
                <li key={candidate.deviceId}>
                  <button
                    type="button"
                    className={`${stationButtonClass} h-12 w-full justify-start px-4 ${
                      state.current?.deviceId === candidate.deviceId ? 'border-idpxyz-accent' : ''
                    }`}
                    onClick={() => device.choose(candidate.deviceId)}
                  >
                    <span className="font-mono">{candidate.deviceId}</span>
                    <span className="text-idpxyz-textMuted">仓 {candidate.workScopeRef}</span>
                    {state.current?.deviceId === candidate.deviceId ? <span className="text-idpxyz-textMuted">（现在用的）</span> : null}
                  </button>
                </li>
              ))}
            </ul>
            {state.current !== null ? (
              <div>
                <button type="button" className={stationButtonClass} onClick={device.cancelChange}>
                  不换了
                </button>
              </div>
            ) : null}
          </Card>
        )
      }
    />
  );
}
