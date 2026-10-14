import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type { WorkDeviceListResponseBody } from './api';

/** 册上的原词。集外取值原样显示，不代折成已知格。 */
export const workDeviceStatusLabels: Record<string, string> = {
  REGISTERED: '登记',
  DEACTIVATED: '停用',
};

/** 作业范围种类的原词（ADR-0149 决定二：节点、场外作业范围）。 */
export const workScopeKindLabels: Record<string, string> = {
  NODE: '节点',
  OFFSITE: '场外作业范围',
};

export function labelOf(labels: Record<string, string>, value: string): string {
  return labels[value] ?? value;
}

export function workScopeText(kind: string, reference: string): string {
  return `${labelOf(workScopeKindLabels, kind)} ${reference}`;
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
    emptyTitle: '当前租户的作业设备册是空的',
    emptyDescription:
      '空列表是册上没有已登记设备，不是渠道未配置。登记与停用走受控登记口，本页不登记、不停用。',
  });
}
