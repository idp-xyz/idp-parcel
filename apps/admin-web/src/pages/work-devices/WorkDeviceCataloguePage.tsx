import { ListPageTemplate, type ListColumn, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import type { WorkDeviceRecord } from './api';
import { labelOf, workDeviceStatusLabels, workScopeText } from './presentation';

const info = moduleInfoById['work-devices'];

const columns: ListColumn<WorkDeviceRecord>[] = [
  {
    id: 'device',
    header: '设备标识',
    className: 'font-mono',
    render: (row) => row.deviceId,
  },
  {
    id: 'scope',
    header: '作业范围',
    render: (row) => workScopeText(row.workScopeKind, row.workScopeRef),
  },
  {
    id: 'status',
    header: '状态',
    render: (row) => labelOf(workDeviceStatusLabels, row.status),
  },
  {
    id: 'deactivated-at',
    header: '停用时刻',
    className: 'font-mono text-[12px]',
    render: (row) => (row.deactivatedAt ? formatInstant(row.deactivatedAt) : ''),
  },
];

// 端点表待接：读口与查阅处理器已在，parcel-api 端点表那一行等 operator-channel/10 余下半边进 main 再接。
// 在那之前本页不发请求。未上表的路径会得到 404，而 404 与渠道未配置不是同一格。
const endpointRowPending: TemplateViewState = {
  kind: 'error',
  title: '端点表待接',
  description:
    '作业设备册的按租户读口与只读查阅处理器已经在。parcel-api 端点表那一行还没接，本页因此不发请求。接上之后，租户只取自信封：没配发行方答未配置，没有查阅读授予答未授予，空册答空列表。',
};

/**
 * 作业设备册：只看已登记的行（ADR-0149 决定二、越权风险点 2）。
 * 不做设备管理，不扫描现场。登记与停用走受控登记口。
 */
export function WorkDeviceCataloguePage() {
  return (
    <ListPageTemplate<WorkDeviceRecord>
      moduleId="work-devices"
      title={info.title}
      description={`${info.owner}——只查阅已登记的设备行。登记与停用走受控登记口，本页不登记、不停用。`}
      columns={columns}
      rows={[]}
      rowKey={(row) => row.deviceId}
      viewState={endpointRowPending}
    />
  );
}
