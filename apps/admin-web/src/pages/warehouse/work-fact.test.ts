import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { buildGuidedPayload } from './guided-forms';
import {
  mintWorkFactIdentity,
  workFactPrefillFor,
  workFactProblemNote,
  workFactSpecs,
} from './work-fact';

test('模拟扫码端覆盖仓配全部 18 个作业事实端点且报文不含设备号与操作者', () => {
  const endpoints = new Set(workFactSpecs.map((spec) => spec.endpoint));
  equal(endpoints.size, 18);
  const expected = [
    '/warehouse-fulfillment-receipt-confirmations',
    '/warehouse-fulfillment-warehouse-measurements',
    '/warehouse-fulfillment-unclaimed-arrival-registrations',
    '/warehouse-fulfillment-return-receipts',
    '/warehouse-fulfillment-internal-moves',
    '/warehouse-fulfillment-form-conversions',
    '/warehouse-fulfillment-condition-conversions',
    '/warehouse-fulfillment-shortage-registrations',
    '/warehouse-fulfillment-processing-executions',
    '/warehouse-fulfillment-relabel-executions',
    '/warehouse-fulfillment-unboxings',
    '/warehouse-fulfillment-count-entries',
    '/warehouse-fulfillment-picks',
    '/warehouse-fulfillment-outbound-checks',
    '/warehouse-fulfillment-packs',
    '/warehouse-fulfillment-outbound-confirmations',
    '/warehouse-fulfillment-pickup-handovers',
    '/warehouse-fulfillment-returns',
  ];
  for (const path of expected) {
    ok(endpoints.has(path), `缺少作业事实端点 ${path}`);
  }
  for (const spec of workFactSpecs) {
    for (const field of spec.form.fields) {
      ok(field.path !== 'deviceId' && field.path !== 'operator' && field.path !== 'tenantId', `${spec.id} 不得在正文带 ${field.path}`);
    }
  }
});

test('端侧签发事实标识与单调序号并按设备绑定仓预填', () => {
  const at = new Date('2026-11-24T08:15:30.123Z');
  const minted = mintWorkFactIdentity('WEB-WH01-01', 7, at);
  equal(minted.factId, 'PDA-WEB-WH01-01-20261124081530-7');
  equal(minted.occurredAt, '2026-11-24T08:15:30Z');
  equal(minted.deviceSequence, '7');

  const pickSpec = workFactSpecs.find((s) => s.id === 'pick')!;
  const prefill = workFactPrefillFor(pickSpec, 'WEB-WH01-01', 'SYN-WH-01', 7, at);
  equal(prefill.warehouse, 'SYN-WH-01');
  equal(prefill.factId, minted.factId);
  equal(prefill.deviceSequence, undefined);
  equal(prefill.occurredAt, undefined);
});

test('空库位盘点录入发出空数组 lines 且仓测校验对象分支', () => {
  const countSpec = workFactSpecs.find((s) => s.id === 'count-entry')!;
  const countBuilt = buildGuidedPayload(countSpec.form, {
    factId: 'PDA-1',
    warehouse: 'SYN-WH-01',
    occurredAt: '2026-11-24T08:00:00Z',
    deviceSequence: '1',
    countId: 'CNT-01',
    locationCode: 'SYN-LOC-STORE',
    lines: [],
  });
  equal(countBuilt.ok, true);
  if (countBuilt.ok) {
    deepEqual(countBuilt.payload.lines, []);
  }

  const measureSpec = workFactSpecs.find((s) => s.id === 'warehouse-measurement')!;
  const badMeasure = buildGuidedPayload(measureSpec.form, {
    factId: 'PDA-2',
    warehouse: 'SYN-WH-01',
    occurredAt: '2026-11-24T08:00:00Z',
    deviceSequence: '2',
    targetKind: 'CARTON',
    ownerAccount: 'SYN-OWNER-01',
    'dimensions.length': '20',
    'dimensions.width': '15',
    'dimensions.height': '10',
    'dimensions.unit': 'CM',
    'weight.value': '1.2',
    'weight.unit': 'KG',
  });
  equal(badMeasure.ok, false);
  if (!badMeasure.ok) ok(badMeasure.formError?.includes('箱号'));
});

test('设备未在用与不在准入范围给出清晰中文恢复说明', () => {
  ok(workFactProblemNote('DEVICE_NOT_IN_USE').includes('扫描设备'));
  ok(workFactProblemNote('OUTSIDE_ADMISSION_SCOPE').includes('生产权威区间'));
});
