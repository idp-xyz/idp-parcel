import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { moduleInfoById } from '../../navigation';
import { catalogueConditionsQuery } from '../catalogue-query';
import { workDeviceConditions, workDeviceStatusLabel, workDeviceViewState, workScopeText } from './presentation';

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
    { kind: 'outcome', status: 200, body: { outcome: 'WORK_DEVICES_LISTED', devices: [], page: { size: 200, next: null, total: 0 } } },
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
        page: { size: 200, next: null, total: 1 },
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

// Covers: 带检索词或状态的一问答出零行是「这组条件下没有」，仍报就绪，由表区说「没有匹配」；不带条件的零行才是册空。
test('带条件的零行不是册空', () => {
  const empty = {
    kind: 'outcome' as const,
    status: 200,
    body: { outcome: 'WORK_DEVICES_LISTED' as const, devices: [], page: { size: 200, next: null, total: 0 } },
  };
  const module = moduleInfoById['work-devices'];
  equal(workDeviceViewState(empty, 0, () => undefined, module).kind, 'empty');
  equal(workDeviceViewState(empty, 0, () => undefined, module, true).kind, 'ready');
});

// Covers: 胶囊「全部」不带 status；选一格即 status 的一个值；检索词下推成 q（去首尾空白由 catalogueRequestPath 做）。
test('状态胶囊与检索词拼成读口的条件', () => {
  equal(catalogueConditionsQuery(workDeviceConditions({ search: '', status: 'ALL' })), '');
  equal(catalogueConditionsQuery(workDeviceConditions({ search: ' sha-hub ', status: 'DEACTIVATED' })), 'status=DEACTIVATED&q=sha-hub');
});
