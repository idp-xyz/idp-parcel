import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
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
    emptyDescription: '网点和场外作业用的扫描设备登记后在这里出现；登记与停用由管理员用登记工具完成，本页只看。',
  });
}
