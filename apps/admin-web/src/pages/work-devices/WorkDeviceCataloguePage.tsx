import { useEffect, useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useOperatorAccessGrant } from '../use-operator-grants';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { labelFor } from '../../vocabulary/status';
import {
  deactivateWorkDevice,
  listWorkDevices,
  registerWorkDevice,
  type WorkDeviceListResponseBody,
  type WorkDeviceRecord,
} from './api';
import type { ApiResult } from '../catalogue-api';
import { completeRegisterRows } from '../complete-register';
import { StatusPill } from '../../components/StatusPill';
import { GuidedActionPanel, type GuidedActionTarget } from '../warehouse/GuidedActionPanel';
import type { GuidedFormSeed, GuidedFormValues } from '../warehouse/guided-forms';
import {
  WORK_DEVICE_SCOPE,
  countWorkDevices,
  workDeviceConditions,
  workDeviceStatusLabel,
  workDeviceViewState,
  type WorkDeviceStatusFilter,
} from './presentation';

const info = moduleInfoById['work-devices'];

// 列只放管设备的人要看的：设备号、在哪儿用（网点 / 场外 / 仓库 + 编号）、在不在用、哪天停的。状态与地点种类的原码
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

const statusFilters: ReadonlyArray<{ id: WorkDeviceStatusFilter; label: string }> = [
  { id: 'ALL', label: '全部' },
  { id: 'REGISTERED', label: workDeviceStatusLabel('REGISTERED') },
  { id: 'DEACTIVATED', label: workDeviceStatusLabel('DEACTIVATED') },
];

const deviceOutcomeLabels: Record<string, string> = {
  RECORDED: '已登记',
  ALREADY_REGISTERED: '已经在册，内容相同，没有重复写入',
  CONTENT_CONFLICT: '同一设备号已经在册且内容不同，没有覆盖',
  DEVICE_NOT_REGISTERED: '本租户设备册里没有这台设备',
};

function deviceProblemNote(code: string): string {
  switch (code) {
    case 'MALFORMED_REQUEST':
      return '报文不完整或含不允许的字段，未提交。';
    case 'NO_ANSWER_FORMED':
      return '系统暂时没能处理，结果还不确定；可稍后重试同一份。';
    default:
      return `这次没有成功（错误码 ${code}）。`;
  }
}

const deviceTargets: readonly GuidedActionTarget[] = [
  {
    id: 'register-device',
    label: '登记设备',
    title: '登记扫描设备或网页作业工位',
    endpoint: 'POST /work-device-registrations',
    snapshotHint:
      '实体 PDA 与无设备时的电脑/浏览器工位走同一套设备册。给仓配「仓内作业台」使用时，作业范围选仓库（WAREHOUSE）并填仓编码。',
    submit: registerWorkDevice,
    outcomeLabels: deviceOutcomeLabels,
    form: {
      fields: [
        { path: 'deviceId', label: '设备号', kind: 'text', required: true, placeholder: 'WEB-WH01-01', hint: '租户内唯一；停用后不再复用' },
        {
          path: 'workScope.kind',
          label: '作业范围种类',
          kind: 'select',
          required: true,
          options: [
            { value: 'WAREHOUSE', label: '仓库（WAREHOUSE，仓内作业台选此项）' },
            { value: 'NODE', label: '网点（NODE）' },
            { value: 'OFFSITE', label: '场外（OFFSITE）' },
          ],
        },
        { path: 'workScope.reference', label: '地点编号（仓编码或网点编号）', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
        { path: 'basis', label: '登记依据', kind: 'text', required: true, placeholder: 'WH-DESK-2026-01' },
      ],
    },
  },
  {
    id: 'deactivate-device',
    label: '停用设备',
    title: '停用已登记的作业设备',
    endpoint: 'POST /work-device-deactivations',
    snapshotHint: '停用是终局，自停用时刻起不可再用来提交作业事实；换机或重置请登记新的设备号。',
    submit: deactivateWorkDevice,
    outcomeLabels: deviceOutcomeLabels,
    form: {
      fields: [
        { path: 'deviceId', label: '设备号', kind: 'text', required: true, placeholder: 'WEB-WH01-01' },
        { path: 'deactivatedAt', label: '停用时刻', kind: 'datetime', required: true },
        { path: 'basis', label: '停用依据', kind: 'text', required: true, placeholder: 'DEACT-2026-01' },
      ],
    },
  },
];

function DeviceTable({
  mayAdminister,
  onPrefillDeactivate,
}: {
  mayAdminister: boolean;
  onPrefillDeactivate: (deviceId: string) => void;
}) {
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

  const inspectorOf = (row: WorkDeviceRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (mayAdminister) {
      if (row.status === 'REGISTERED') {
        actions.push({ label: '停用此设备', onRun: () => onPrefillDeactivate(row.deviceId) });
      } else {
        actions.push({ label: '停用此设备', disabledReason: '该设备已经停用，停用后不再复用' });
      }
    }
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
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        {
          kind: 'audit',
          fields: [
            { label: '状态码', value: row.status, mono: true },
            { label: '地点种类码', value: row.workScopeKind, mono: true },
          ],
        },
      ],
    };
  };

  return (
    <ListPageTemplate<WorkDeviceRecord>
      moduleId="work-devices"
      title={info.title}
      description="网点、场外和仓库在用的扫描设备或网页作业工位。持操作者接入管理授予的管理员可在「登记与停用」签直接增停。"
      search={{ value: search, onChange: setSearch, placeholder: '搜设备号、地点编号' }}
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

/**
 * 作业设备册：查阅、在线登记与停用（操作者接入管理授予）。
 * 实体 PDA 与无硬件时的网页作业工位共用同一本设备册，只登标识、范围与停用，不做设备管理。
 */
export function WorkDeviceCataloguePage() {
  const { mayAdminister } = useOperatorAccessGrant();
  const [tab, setTab] = useState('list');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);

  const openDeactivate = (deviceId: string) => {
    const values: GuidedFormValues = { deviceId };
    setSeed((current) => ({ targetId: 'deactivate-device', values, nonce: (current?.nonce ?? 0) + 1 }));
    setTab('manage');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="list">扫描设备</TabsTrigger>
          {mayAdminister ? <TabsTrigger value="manage">登记与停用</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="list" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeviceTable mayAdminister={mayAdminister} onPrefillDeactivate={openDeactivate} />
        </TabsContent>
        {mayAdminister ? (
          <TabsContent value="manage" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <GuidedActionPanel
              moduleId="work-devices"
              targets={deviceTargets}
              problemNote={deviceProblemNote}
              seed={seed}
            />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
