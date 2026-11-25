// 这台浏览器绑的是哪台设备、本机的设备序号（纯 TypeScript，node:test 直接测）。
//
// 作业台与旧的仓内作业台页共用这一份：键与读写的规矩与旧页原先内联的四个函数逐字相同，所以两边读写的是同一台设备、同一份
// 序号——在旧页选了设备，作业台打开就是它，反过来也一样。存储作参数传进来（生产传 browserStorage()，测试传 Map 顶替），
// 读不了、写不了（禁用存储）一律当没有、不报错：绑定与序号只是本机的记忆，丢了不影响提交，影响的是下次要重选一次。
//
// 序号只要求本机单调递增、从 1 起，后端不要求连续，所以多铸几个、留下空号都不要紧（见 work-fact-submit.ts 为什么要这样）。

import type { CatalogueConditions } from '../../catalogue-query';

export const ACTIVE_DEVICE_KEY = 'idp.parcel.warehouse.activeDevice';

export const deviceSequenceKey = (deviceId: string): string => `idp.parcel.warehouse.deviceSeq.${deviceId}`;

/** localStorage 的最小子集。 */
export interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

/** 浏览器的本机存储；禁用存储时访问 window.localStorage 本身就会抛，所以包一层，拿不到当 null。 */
export function browserStorage(): StorageLike | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function readActiveDeviceId(store: StorageLike | null): string {
  try {
    return store?.getItem(ACTIVE_DEVICE_KEY) ?? '';
  } catch {
    return '';
  }
}

export function writeActiveDeviceId(store: StorageLike | null, deviceId: string): void {
  try {
    store?.setItem(ACTIVE_DEVICE_KEY, deviceId);
  } catch {
    // 禁用存储时忽略
  }
}

export function readDeviceSequence(store: StorageLike | null, deviceId: string): number {
  if (!deviceId) return 0;
  try {
    const raw = store?.getItem(deviceSequenceKey(deviceId));
    const parsed = raw ? Number.parseInt(raw, 10) : 0;
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
  } catch {
    return 0;
  }
}

/**
 * 把本机序号推进到 `toValue`（比现在大才推），没给就加一；交回推进后的值。存不下时照样交回，调用方这一笔用的序号不因此变。
 */
export function advanceDeviceSequence(store: StorageLike | null, deviceId: string, toValue?: number): number {
  const current = readDeviceSequence(store, deviceId);
  const next = toValue && toValue > current ? toValue : current + 1;
  try {
    store?.setItem(deviceSequenceKey(deviceId), String(next));
  } catch {
    // 禁用存储时忽略
  }
  return next;
}

/** 「登记在用」的设备：旧页与作业台读设备名册的同一道筛，作业范围是仓库的再由 warehouseDevicesOf 挑。 */
export const registeredDeviceConditions: CatalogueConditions = { filters: { status: ['REGISTERED'] } };

export interface WorkDeviceLike {
  deviceId: string;
  workScopeKind: string;
  workScopeRef: string;
}

/** 作业范围是仓库的设备；一台设备的作业范围引用就是它绑的仓编码。 */
export function warehouseDevicesOf<Device extends WorkDeviceLike>(devices: readonly Device[]): Device[] {
  return devices.filter((device) => device.workScopeKind === 'WAREHOUSE');
}

export type DeviceBinding<Device> =
  /** 名册里没有可用的仓库设备：说明去「扫描设备」登记。 */
  | { kind: 'none' }
  /** 有几台、本机又没有记着其中一台：让人选一次。 */
  | { kind: 'choose'; devices: readonly Device[] }
  /** 只有一台，或本机记着的那台还在：直接用。`remember` 为真表示本机记着的不是它，调用方要把它记下来。 */
  | { kind: 'bound'; device: Device; remember: boolean };

/**
 * 作业台第一次进来怎么定设备。只有一台直接用；有几台时，本机记着的那台还在册就用它，没有记着或记着的已不在，让人选，
 * 不替人挑第一台（旧页取第一台，作业台改成明确选一次：挑错设备，提交就记在别的仓名下）。
 */
export function bindDevice<Device extends WorkDeviceLike>(devices: readonly Device[], storedId: string): DeviceBinding<Device> {
  const candidates = warehouseDevicesOf(devices);
  if (candidates.length === 0) return { kind: 'none' };
  if (candidates.length === 1) return { kind: 'bound', device: candidates[0], remember: candidates[0].deviceId !== storedId };
  const stored = candidates.find((device) => device.deviceId === storedId);
  return stored ? { kind: 'bound', device: stored, remember: false } : { kind: 'choose', devices: candidates };
}
