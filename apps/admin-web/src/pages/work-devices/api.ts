// 作业设备册查阅（GET /work-devices）。形状镜像 internal/accessidentity/adapters/http 的查阅答复。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/** 线上状态词。页面显示登记 / 停用，不把这两个词再译一遍。 */
export type WorkDeviceStatus = 'REGISTERED' | 'DEACTIVATED';

export interface WorkDeviceRecord {
  deviceId: string;
  workScopeKind: string;
  workScopeRef: string;
  status: string;
  /** 只有已停用的行才有。缺席即尚未停用，不是时刻未知。 */
  deactivatedAt?: string;
}

export interface WorkDeviceListResponseBody {
  outcome: 'WORK_DEVICES_LISTED';
  devices: WorkDeviceRecord[];
}

export function listWorkDevices(): Promise<ApiResult<WorkDeviceListResponseBody>> {
  return exchangeMasterData<WorkDeviceListResponseBody>('/work-devices');
}
