import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { moduleInfoById } from '../../navigation';
import {
  labelOf,
  workDeviceStatusLabels,
  workDeviceViewState,
  workScopeKindLabels,
  workScopeText,
} from './presentation';

test('状态与作业范围用册上的原词，集外取值原样显示', () => {
  equal(labelOf(workDeviceStatusLabels, 'REGISTERED'), '登记');
  equal(labelOf(workDeviceStatusLabels, 'DEACTIVATED'), '停用');
  equal(labelOf(workDeviceStatusLabels, 'ONLINE'), 'ONLINE');
  equal(workScopeText('NODE', 'SYN-NODE-SZX-01'), '节点 SYN-NODE-SZX-01');
  equal(workScopeText('OFFSITE', 'SYN-OFFSITE-SZX-01'), '场外作业范围 SYN-OFFSITE-SZX-01');
  equal(labelOf(workScopeKindLabels, 'SITE'), 'SITE');
});

test('空册是空列表，不是渠道未配置', () => {
  const state = workDeviceViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'WORK_DEVICES_LISTED', devices: [] } },
    0,
    () => undefined,
    moduleInfoById['work-devices'],
  );
  equal(state.kind, 'empty');
  if (state.kind === 'empty') {
    equal(state.title, '当前租户的作业设备册是空的');
  }
});

test('有行则是就绪，未配置与未授予各自一格', () => {
  const module = moduleInfoById['work-devices'];
  const ready = workDeviceViewState(
    {
      kind: 'outcome',
      status: 200,
      body: {
        outcome: 'WORK_DEVICES_LISTED',
        devices: [
          {
            deviceId: 'SYN-DEVICE-A',
            workScopeKind: 'NODE',
            workScopeRef: 'SYN-NODE-SZX-01',
            status: 'REGISTERED',
          },
        ],
      },
    },
    1,
    () => undefined,
    module,
  );
  equal(ready.kind, 'ready');
  equal(workDeviceViewState({ kind: 'unconfigured' }, 0, () => undefined, module).kind, 'unconfigured');
  equal(workDeviceViewState({ kind: 'notGranted' }, 0, () => undefined, module).kind, 'notGranted');
});
