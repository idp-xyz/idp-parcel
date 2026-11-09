import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { labelFor } from '../../vocabulary/status';
import { listWorkDevices, type WorkDeviceListResponseBody, type WorkDeviceRecord } from './api';
import type { ApiResult } from '../catalogue-api';
import { completeRegisterRows } from '../complete-register';
import { StatusPill } from '../../components/StatusPill';
import {
  WORK_DEVICE_SCOPE,
  countWorkDevices,
  workDeviceConditions,
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
    className: 'whitespace-nowrap font-mono type-body leading-4',
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
    className: 'whitespace-nowrap font-mono type-body leading-4',
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
    className: 'whitespace-nowrap font-mono type-body leading-4',
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
 *
 * 按游标翻页（ADR-0144，票 catalogue-read-pagination-rollout/14），次序是读口的缺省序（设备号）：检索词停稳后下推成 q，状态胶囊
 * 下推成 status 一维（presentation 的 workDeviceConditions）。胶囊计数与页头「共 N 台，在用 N 台」只在整册一页装得下时摆
 * （complete-register.ts）：另取一次不带条件的第一页，它已是整册就按它数；装不下就不摆——到手的只是一页，页上数出来的不是整册，
 * 那时条数看翻页条（答复的 total，随胶囊与检索变）。
 */
export function WorkDeviceCataloguePage() {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<WorkDeviceStatusFilter>('ALL');
  const conditions = workDeviceConditions({ search: useSettledKeyword(search), status });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWorkDevices);
  const devices = answer?.kind === 'outcome' ? answer.body.devices : [];
  const [whole, setWhole] = useState<ApiResult<WorkDeviceListResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void listWorkDevices().then((next) => {
      if (!cancelled) setWhole(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  const complete = completeRegisterRows(whole?.kind === 'outcome' ? whole.body : null, (body) => body.devices);
  const counts = complete === null ? null : countWorkDevices(complete);

  return (
    <ListPageTemplate<WorkDeviceRecord>
      moduleId="work-devices"
      title={info.title}
      description="网点和场外作业点在用的扫描设备，各在哪儿用、还在不在用。新增或停用设备请联系管理员。"
      search={{ value: search, onChange: setSearch, placeholder: '搜设备号、地点编号' }}
      // 换胶囊之后那一问在途时胶囊照摆，不随加载闪掉；读口答了接入那一格或错误时不摆。
      filters={
        answer === null || answer.kind === 'outcome' ? (
          <>
            {statusFilters.map((filter) => (
              <button
                key={filter.id}
                type="button"
                className={chipClass(filter.id === status)}
                aria-pressed={filter.id === status}
                onClick={() => setStatus(filter.id === status ? 'ALL' : filter.id)}
              >
                {counts === null ? filter.label : `${filter.label} ${counts[filter.id]}`}
              </button>
            ))}
          </>
        ) : undefined
      }
      filterSummary={counts === null ? undefined : `共 ${counts.ALL} 台，在用 ${counts.REGISTERED} 台`}
      emptyRowsNote="当前筛选条件下没有匹配的设备"
      columns={columns}
      rows={devices}
      rowKey={(row) => row.deviceId}
      inspector={inspectorOf}
      pagination={pagination}
      viewState={workDeviceViewState(
        answer,
        devices.length,
        retry,
        info,
        catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      )}
    />
  );
}
