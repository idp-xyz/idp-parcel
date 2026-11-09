import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { labelFor } from '../../vocabulary/status';
import { listWorkDevices, type WorkDeviceRecord } from './api';
import { StatusPill } from '../../components/StatusPill';
import {
  WORK_DEVICE_SCOPE,
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
 * 下推成 status 一维（presentation 的 workDeviceConditions）。胶囊不带计数、页头不再报「在用 N 台」——到手的只是一页，页上数出来的
 * 不是整册；整册条数在翻页条上（答复的 total，随胶囊与检索变）。
 */
export function WorkDeviceCataloguePage() {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<WorkDeviceStatusFilter>('ALL');
  const conditions = workDeviceConditions({ search: useSettledKeyword(search), status });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWorkDevices);
  const devices = answer?.kind === 'outcome' ? answer.body.devices : [];

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
                {filter.label}
              </button>
            ))}
          </>
        ) : undefined
      }
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
