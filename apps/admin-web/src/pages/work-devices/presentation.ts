import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type { CatalogueConditions } from '../catalogue-query';
import type { WorkDeviceListResponseBody } from './api';

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

/** narrowed：这一问带了检索词或状态，或已翻过第一页——零行是「这组条件下没有」，不是册空（catalogueViewState 的 narrowed）。 */
export function workDeviceViewState(
  answer: ApiResult<WorkDeviceListResponseBody> | null,
  rowCount: number,
  retry: () => void,
  module: ModuleInfo,
  narrowed = false,
): TemplateViewState {
  return catalogueViewState(answer, rowCount, retry, {
    module,
    endpoint: 'GET /work-devices',
    emptyTitle: '还没有登记扫描设备',
    emptyDescription: '网点和场外作业用的扫描设备登记后会出现在这里。新增或停用设备请联系管理员。',
    narrowed,
  });
}

/** 胶囊筛：全部、在用、已停用。码取线上状态词，词经 work-device 分册。 */
export type WorkDeviceStatusFilter = 'ALL' | 'REGISTERED' | 'DEACTIVATED';

export interface WorkDeviceFilter {
  search: string;
  status: WorkDeviceStatusFilter;
}

/**
 * 屏上的检索词与状态胶囊 → 读口的条件（ADR-0144 决定四）：胶囊下推成 status 一维，检索词下推成 q。q 只认设备号与地点编号
 * （accessidentity 的 DeviceCataloguePages）；地点种类的中文词（网点、场外）不是这一册的列，搜不到，检索框的提示也只写这两格。
 */
export function workDeviceConditions(filter: WorkDeviceFilter): CatalogueConditions {
  return { q: filter.search, filters: filter.status === 'ALL' ? {} : { status: [filter.status] } };
}
