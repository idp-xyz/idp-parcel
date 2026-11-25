// 收货工位的收货暂存库位：按设备记在本机，核法与拣货放货位同一份（createStagingVerifier），只是库区用途换成收货暂存。
//
// 次序取自 application.ConfirmReceiptHandler.locationIssue：库位在这台设备绑的仓里、没停用，所在库区没停用且用途是
// RECEIVING_STAGING。不满足时收货确认答内容不合（CONTENT_INVALID），这里先在屏上说清是哪一条。预报已不能收货、这次要转待认领时，
// 后端不再核库位（divertUnscreened），屏上仍先核：转过去的到货也要有个地方放。

import { listWarehouseCatalogue } from '../api';
import {
  createStagingVerifier,
  MAX_STAGING_PAGES,
  type StagingUsage,
  type StagingVerifier,
} from './pick-staging-check';
import type { StorageLike } from './device-binding';

export const receivingUsage: StagingUsage = {
  usage: 'RECEIVING_STAGING',
  deactivated: (code) => `库位 ${code} 已停用，不能当收货暂存位。`,
  wrongUsage: (code, zoneCode, usageLabel) =>
    `库位 ${code} 所在的库区 ${zoneCode} 用途是「${usageLabel}」，不是收货暂存库区，货不能收在那里。`,
};

export function liveReceivingVerifier(): StagingVerifier {
  return createStagingVerifier(listWarehouseCatalogue, MAX_STAGING_PAGES, receivingUsage);
}

export const receiveLocationKey = (deviceId: string): string => `idp.parcel.station.receive.location.${deviceId}`;

export function readReceiveLocation(store: StorageLike | null, deviceId: string): string | null {
  if (!deviceId) return null;
  try {
    const raw = store?.getItem(receiveLocationKey(deviceId)) ?? null;
    return raw !== null && raw.trim() !== '' ? raw : null;
  } catch {
    return null;
  }
}

export function writeReceiveLocation(store: StorageLike | null, deviceId: string, location: string): void {
  if (!deviceId) return;
  try {
    store?.setItem(receiveLocationKey(deviceId), location);
  } catch {
    // 禁用存储时忽略：下次进来重扫一次
  }
}
