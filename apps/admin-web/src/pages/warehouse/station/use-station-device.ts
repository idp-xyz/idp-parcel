import { useCallback, useEffect, useMemo, useState } from 'react';
import { listWorkDevices, type WorkDeviceRecord } from '../../work-devices/api';
import { bindDevice, browserStorage, readActiveDeviceId, registeredDeviceConditions, warehouseDevicesOf, writeActiveDeviceId } from './device-binding';
import { describeReadFailure } from './work-fact-submit';

// 工位绑的设备（判断在 device-binding.ts）：进来先读在用的仓库设备名册，只有一台直接用，多台让人选一次并记住，没有就说明去登记。
// 读的是同一个名册、记的是旧作业台页同一把键，所以在哪边换了设备，另一边下次进来就是它。

export type StationDeviceState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'none' }
  | { phase: 'choose'; devices: readonly WorkDeviceRecord[]; /** 已经绑着一台、是人主动要换：可以不换。 */ current: WorkDeviceRecord | null }
  | { phase: 'ready'; device: WorkDeviceRecord; devices: readonly WorkDeviceRecord[] };

export interface StationDevice {
  state: StationDeviceState;
  choose: (deviceId: string) => void;
  /** 人主动要换设备。 */
  change: () => void;
  cancelChange: () => void;
  reload: () => void;
}

export function useStationDevice(): StationDevice {
  const [devices, setDevices] = useState<WorkDeviceRecord[] | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [boundId, setBoundId] = useState(() => readActiveDeviceId(browserStorage()));
  const [changing, setChanging] = useState(false);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setDevices(null);
    setFailure(null);
    void listWorkDevices(registeredDeviceConditions).then((answer) => {
      if (cancelled) return;
      if (answer.kind !== 'outcome') {
        setFailure(describeReadFailure(answer));
        return;
      }
      setDevices(warehouseDevicesOf(answer.body.devices));
    });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  const binding = useMemo(() => (devices === null ? null : bindDevice(devices, boundId)), [devices, boundId]);

  // 只有一台、或本机记着的不是它时，把它记到本机；与旧页读到设备名册之后做的同一件事。
  useEffect(() => {
    if (binding?.kind === 'bound' && binding.remember) {
      writeActiveDeviceId(browserStorage(), binding.device.deviceId);
      setBoundId(binding.device.deviceId);
    }
  }, [binding]);

  const state = useMemo<StationDeviceState>(() => {
    if (failure !== null) return { phase: 'error', message: failure };
    if (devices === null || binding === null) return { phase: 'loading' };
    if (binding.kind === 'none') return { phase: 'none' };
    if (binding.kind === 'choose') return { phase: 'choose', devices: binding.devices, current: null };
    if (changing && devices.length > 1) return { phase: 'choose', devices, current: binding.device };
    return { phase: 'ready', device: binding.device, devices };
  }, [failure, devices, binding, changing]);

  const choose = useCallback((deviceId: string) => {
    writeActiveDeviceId(browserStorage(), deviceId);
    setBoundId(deviceId);
    setChanging(false);
  }, []);
  const change = useCallback(() => setChanging(true), []);
  const cancelChange = useCallback(() => setChanging(false), []);
  const reload = useCallback(() => setTick((value) => value + 1), []);

  return { state, choose, change, cancelChange, reload };
}
