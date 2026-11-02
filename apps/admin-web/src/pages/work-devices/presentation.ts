import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type { WorkDeviceListResponseBody, WorkDeviceRecord } from './api';

import { labelFor } from '../../vocabulary/status';

/** 设备状态经词典的 work-device 分册译：REGISTERED 对设备是「在用」，不是等生效。 */
export const WORK_DEVICE_SCOPE = 'work-device';

export function workDeviceStatusLabel(status: string): string {
  return labelFor(status, WORK_DEVICE_SCOPE);
}

/** 作业范围一格：网点 / 场外加引用（ADR-0149 决定二：节点、场外作业范围）。种类码经词典译。 */
export function workScopeText(kind: string, reference: string): string {
  return `${labelFor(kind)} ${reference}`;
}

export function workDeviceViewState(
  answer: ApiResult<WorkDeviceListResponseBody> | null,
  rowCount: number,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  return catalogueViewState(answer, rowCount, retry, {
    module,
    endpoint: 'GET /work-devices',
    emptyTitle: '还没有登记扫描设备',
    emptyDescription: '网点和场外作业用的扫描设备登记后会出现在这里。新增或停用设备请联系管理员。',
  });
}

/** 胶囊筛：全部、在用、已停用。码取线上状态词，词经 work-device 分册。 */
export type WorkDeviceStatusFilter = 'ALL' | 'REGISTERED' | 'DEACTIVATED';

export interface WorkDeviceFilter {
  search: string;
  status: WorkDeviceStatusFilter;
}

/** 检索命中设备号、地点编号与地点种类的中文词；状态由胶囊筛，不进检索。 */
export function filterWorkDevices(devices: readonly WorkDeviceRecord[], filter: WorkDeviceFilter): WorkDeviceRecord[] {
  const needle = filter.search.trim().toLowerCase();
  return devices.filter((row) => {
    if (filter.status !== 'ALL' && row.status !== filter.status) return false;
    if (needle === '') return true;
    return [row.deviceId, row.workScopeRef, labelFor(row.workScopeKind)].some((value) => value.toLowerCase().includes(needle));
  });
}

export function countWorkDevices(devices: readonly WorkDeviceRecord[]): Record<WorkDeviceStatusFilter, number> {
  return {
    ALL: devices.length,
    REGISTERED: devices.filter((row) => row.status === 'REGISTERED').length,
    DEACTIVATED: devices.filter((row) => row.status === 'DEACTIVATED').length,
  };
}
