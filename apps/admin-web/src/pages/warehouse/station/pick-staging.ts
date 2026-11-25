// 拣货工位的放货位按设备记在本机（纯 TypeScript，node:test 直接测）：下次进来直接用，不再扫。
//
// 记的只是一个库位码。它此刻还能不能用（库位被停用了、库区改了用途）每次进来都要重新核，核不过就不用它，让人重扫——
// 所以这里不判：怎么核是 createStagingVerifier，核完怎么处置是 beginPick 与 stagingVerified。

import type { StorageLike } from './device-binding';

export const pickStagingKey = (deviceId: string): string => `idp.parcel.station.pick.staging.${deviceId}`;

/** 本机记着的这台设备的放货位；没有记着、设备号为空、存储读不了都是 null。 */
export function readPickStaging(store: StorageLike | null, deviceId: string): string | null {
  if (!deviceId) return null;
  try {
    const raw = store?.getItem(pickStagingKey(deviceId)) ?? null;
    return raw !== null && raw.trim() !== '' ? raw : null;
  } catch {
    return null;
  }
}

export function writePickStaging(store: StorageLike | null, deviceId: string, location: string): void {
  if (!deviceId) return;
  try {
    store?.setItem(pickStagingKey(deviceId), location);
  } catch {
    // 禁用存储时忽略：下次进来重扫一次
  }
}
