// 作业设备册查阅（GET /work-devices）。形状镜像 internal/accessidentity/adapters/http 的查阅答复。
//
// 读口已迁到 ADR-0144（票 catalogue-read-pagination-rollout/13）：翻页走游标，答复带 page；status 一维与 q 下推，各维与 q 覆盖
// 的列以读口的声明为准（accessidentity 的 DeviceCataloguePages），这里不抄一份。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';

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
  page: CataloguePage;
}

/** 游标缺即第一页；不带条件即缺省序（设备号）的第一页。 */
export function listWorkDevices(
  conditions: CatalogueConditions = {},
  after: string | null = null,
): Promise<ApiResult<WorkDeviceListResponseBody>> {
  return exchangeMasterData<WorkDeviceListResponseBody>(catalogueRequestPath('/work-devices', conditions, after));
}
