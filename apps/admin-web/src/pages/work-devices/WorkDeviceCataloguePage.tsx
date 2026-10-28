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
import { StatusPill } from '../../components/StatusPill';
import { WORK_DEVICE_SCOPE, workDeviceViewState, workScopeText } from './presentation';

const info = moduleInfoById['work-devices'];

const columns: ListColumn<WorkDeviceRecord>[] = [
  {
    id: 'device',
    header: '设备号',
    className: 'whitespace-nowrap font-mono text-xs',
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
    align: 'center',
    render: (row) => <StatusPill code={row.status} scope={WORK_DEVICE_SCOPE} />,
  },
  {
    id: 'deactivated-at',
    header: '停用时间',
    className: 'whitespace-nowrap font-mono text-xs',
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
      title="扫描设备"
      description="网点和场外作业在用的扫描设备，以及各自归哪个作业范围"
      search={{ value: search, onChange: setSearch, placeholder: '搜设备号、作业范围' }}
      filterSummary={answer?.kind === 'outcome' ? `${devices.length} 台 · 已停用 ${devices.filter((row) => row.status === 'DEACTIVATED').length} 台` : undefined}
      emptyRowsNote="没有匹配的设备"
      columns={columns}
      rows={visible}
      rowKey={(row) => row.deviceId}
      viewState={workDeviceViewState(answer, devices.length, retry, info)}
    />
  );
}
