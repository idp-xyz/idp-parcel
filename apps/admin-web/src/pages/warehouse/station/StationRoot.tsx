import { parseStationAddress } from '../../../shell/station-address';
import { DeviceGate } from './DeviceGate';
import { DispatchStation } from './DispatchStation';
import { PutawayStation } from './PutawayStation';
import { CountStation } from './CountStation';
import { MoreStation } from './MoreStation';
import { PackStation } from './PackStation';
import { ReceiveStation } from './ReceiveStation';
import { PickStation } from './PickStation';
import { SortStation } from './SortStation';
import { StationFrame } from './StationFrame';
import { StationHome } from './StationHome';
import { resolveStationRoute } from './stations';
import { useStationDevice } from './use-station-device';

// 作业台的入口：App 在作业台地址上不渲染外壳，渲染这里（整块经 App 动态引入，入口块不带它）。
// 先把设备绑好（DeviceGate），再按地址落到选工位页或某个工位。工位的状态机、扫描框、提交各在自己的文件里。

export function StationRoot({ hash }: { hash: string }) {
  const address = parseStationAddress(hash);
  const device = useStationDevice();
  // App 只在作业台地址上挂本组件；地址认不出时（不会发生）什么都不摆，不替它猜一页。
  if (address === null) return null;
  const route = resolveStationRoute(address);

  return (
    <DeviceGate device={device}>
      {({ device: bound, onChangeDevice }) =>
        route.kind === 'pick' ? (
          // 设备或波次一换就是新的一趟：状态、本趟的认码缓存都从头来
          <PickStation key={`${bound.deviceId}|${route.wave ?? ''}`} device={bound} wave={route.wave} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'sort' ? (
          <SortStation key={`${bound.deviceId}|${route.wave ?? ''}`} device={bound} wave={route.wave} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'pack' ? (
          // 设备或地址里的出库请求号一换也是新的一趟
          <PackStation key={`${bound.deviceId}|${route.request ?? ''}`} device={bound} request={route.request} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'receive' ? (
          <ReceiveStation key={`${bound.deviceId}|${route.advice ?? ''}`} device={bound} adviceId={route.advice} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'dispatch' ? (
          <DispatchStation key={`${bound.deviceId}|${route.request ?? ''}`} device={bound} requestId={route.request} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'putaway' ? (
          <PutawayStation key={bound.deviceId} device={bound} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'count' ? (
          <CountStation key={`${bound.deviceId}|${route.count ?? ''}`} device={bound} countId={route.count} onChangeDevice={onChangeDevice} />
        ) : route.kind === 'more' ? (
          <MoreStation key={bound.deviceId} device={bound} onChangeDevice={onChangeDevice} />
        ) : (
          <StationFrame
            title="作业台"
            device={{ id: bound.deviceId, warehouse: bound.workScopeRef }}
            onChangeDevice={onChangeDevice}
            left={
              <StationHome
                notice={
                  route.kind === 'unknown'
                    ? `没有「${route.station}」这个工位，从下面选一个。`
                    : route.kind === 'not-ready'
                      ? `${route.entry.name}还在仓内作业台里，全屏的还没做好。`
                      : undefined
                }
                highlight={route.kind === 'not-ready' ? route.entry.id : undefined}
              />
            }
          />
        )
      }
    </DeviceGate>
  );
}
