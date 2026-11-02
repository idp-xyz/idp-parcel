import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { formatInstant } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import { labelFor } from '../../vocabulary/status';
import {
  listWorkDevices,
  type WorkDeviceListResponseBody,
  type WorkDeviceRecord,
} from './api';
import { StatusPill } from '../../components/StatusPill';
import {
  WORK_DEVICE_SCOPE,
  countWorkDevices,
  filterWorkDevices,
  workDeviceStatusLabel,
  workDeviceViewState,
  type WorkDeviceStatusFilter,
} from './presentation';

const info = moduleInfoById['work-devices'];

// 列只放管设备的人要看的：设备号、在哪儿用（网点 / 场外作业点 + 编号）、在不在用、哪天停的。状态与地点种类的原码
// 进右栏「审计」。
const columns: ListColumn<WorkDeviceRecord>[] = [
  {
    id: 'device',
    header: '设备号',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => row.deviceId,
  },
  {
    id: 'scope-kind',
    header: '使用地点',
    render: (row) => labelFor(row.workScopeKind),
  },
  {
    id: 'scope-ref',
    header: '地点编号',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => row.workScopeRef,
  },
  {
    id: 'status',
    header: '状态',
    render: (row) => <StatusPill code={row.status} scope={WORK_DEVICE_SCOPE} />,
  },
  {
    id: 'deactivated-at',
    header: '停用时间',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => (row.deactivatedAt ? formatInstant(row.deactivatedAt) : ''),
  },
];

function inspectorOf(row: WorkDeviceRecord): InspectorContent {
  return {
    title: '扫描设备',
    subtitle: row.deviceId,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '使用地点', value: labelFor(row.workScopeKind) },
          { label: '地点编号', value: row.workScopeRef, mono: true },
          ...(row.deactivatedAt ? [{ label: '停用时间', value: formatInstant(row.deactivatedAt), mono: true }] : []),
        ],
      },
      { kind: 'status', items: [{ label: '状态', word: workDeviceStatusLabel(row.status) }] },
      {
        kind: 'audit',
        fields: [
          { label: '状态码', value: row.status, mono: true },
          { label: '地点种类码', value: row.workScopeKind, mono: true },
        ],
      },
    ],
  };
}

const statusFilters: ReadonlyArray<{ id: WorkDeviceStatusFilter; label: string }> = [
  { id: 'ALL', label: '全部' },
  { id: 'REGISTERED', label: workDeviceStatusLabel('REGISTERED') },
  { id: 'DEACTIVATED', label: workDeviceStatusLabel('DEACTIVATED') },
];

/**
 * 作业设备册：只看已登记的行（ADR-0149 决定二、越权风险点 2）。
 * 不做设备管理，不扫描现场。登记与停用走受控登记口，本页没有写入口。
 */
export function WorkDeviceCataloguePage() {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<WorkDeviceStatusFilter>('ALL');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<WorkDeviceListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listWorkDevices().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const devices = answer?.kind === 'outcome' ? answer.body.devices : [];
  const counts = countWorkDevices(devices);
  const visible = filterWorkDevices(devices, { search, status });
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<WorkDeviceRecord>
      moduleId="work-devices"
      title={info.title}
      description="网点和场外作业点在用的扫描设备，各在哪儿用、还在不在用。新增或停用设备请联系管理员。"
      search={{ value: search, onChange: setSearch, placeholder: '搜设备号、地点编号' }}
      filters={
        devices.length > 0 ? (
          <>
            {statusFilters.map((filter) => (
              <button
                key={filter.id}
                type="button"
                className={chipClass(filter.id === status)}
                aria-pressed={filter.id === status}
                onClick={() => setStatus(filter.id === status ? 'ALL' : filter.id)}
              >
                {filter.label} {counts[filter.id]}
              </button>
            ))}
          </>
        ) : undefined
      }
      filterSummary={answer?.kind === 'outcome' ? `共 ${counts.ALL} 台，在用 ${counts.REGISTERED} 台` : undefined}
      emptyRowsNote="当前筛选条件下没有匹配的设备"
      columns={columns}
      rows={visible}
      rowKey={(row) => row.deviceId}
      inspector={inspectorOf}
      viewState={workDeviceViewState(answer, devices.length, retry, info)}
    />
  );
}
