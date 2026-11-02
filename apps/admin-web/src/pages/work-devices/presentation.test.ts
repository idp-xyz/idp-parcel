import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { moduleInfoById } from '../../navigation';
import { countWorkDevices, filterWorkDevices, workDeviceStatusLabel, workDeviceViewState, workScopeText } from './presentation';

test('状态与作业范围经词典译成一线的词，集外取值原样显示', () => {
  equal(workDeviceStatusLabel('REGISTERED'), '在用');
  equal(workDeviceStatusLabel('DEACTIVATED'), '已停用');
  equal(workDeviceStatusLabel('ONLINE'), 'ONLINE');
  equal(workScopeText('NODE', 'SYN-NODE-SZX-01'), '网点 SYN-NODE-SZX-01');
  equal(workScopeText('OFFSITE', 'SYN-OFFSITE-SZX-01'), '场外 SYN-OFFSITE-SZX-01');
  equal(workScopeText('SITE', 'X'), 'SITE X');
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
    equal(state.title, '还没有登记扫描设备');
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

test('胶囊筛按状态，检索认设备号、地点编号与地点种类的中文词', () => {
  const devices = [
    { deviceId: 'SYN-DEVICE-SHA-HUB-01', workScopeKind: 'NODE', workScopeRef: 'SYN-NODE-SHA-HUB', status: 'REGISTERED' },
    {
      deviceId: 'SYN-DEVICE-OFFSITE-01',
      workScopeKind: 'OFFSITE',
      workScopeRef: 'SYN-OFFSITE-CN-EAST',
      status: 'DEACTIVATED',
      deactivatedAt: '2026-10-01T00:00:00Z',
    },
  ];
  equal(filterWorkDevices(devices, { search: '', status: 'DEACTIVATED' }).length, 1);
  equal(filterWorkDevices(devices, { search: '场外', status: 'ALL' })[0].deviceId, 'SYN-DEVICE-OFFSITE-01');
  equal(filterWorkDevices(devices, { search: 'sha-hub', status: 'ALL' }).length, 1);
  const counts = countWorkDevices(devices);
  equal(counts.ALL, 2);
  equal(counts.REGISTERED, 1);
  equal(counts.DEACTIVATED, 1);
});
