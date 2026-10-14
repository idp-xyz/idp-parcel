import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import {
  listWorkDevices,
  type WorkDeviceListResponseBody,
  type WorkDeviceRecord,
} from './api';
import { labelOf, workDeviceStatusLabels, workDeviceViewState, workScopeText } from './presentation';

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

/**
 * 作业设备册：只看已登记的行（ADR-0149 决定二、越权风险点 2）。
 * 不做设备管理，不扫描现场。登记与停用走受控登记口。
 */
export function WorkDeviceCataloguePage() {
  const [search, setSearch] = useState('');
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
  const needle = search.trim().toLowerCase();
  const visible = needle
    ? devices.filter((row) =>
        [row.deviceId, row.workScopeKind, row.workScopeRef, row.status].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : devices;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<WorkDeviceRecord>
      moduleId="work-devices"
      title={info.title}
      description={`${info.owner}——只查阅已登记的设备行。登记与停用走受控登记口，本页不登记、不停用。`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索设备标识 / 作业范围',
      }}
      filterSummary={answer?.kind === 'outcome' ? `当前返回 ${devices.length} 台设备` : undefined}
      columns={columns}
      rows={visible}
      rowKey={(row) => row.deviceId}
      viewState={workDeviceViewState(answer, devices.length, retry, info)}
    />
  );
}
