import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { etaPredictionRowOf, type ETAPredictionRecord } from './eta-predictions';

const sourced: ETAPredictionRecord = {
  parcel: 'SYN-PCL-1', milestone: 'DELIVERED', source: 'CARRIER_PROVIDED', version: 'ETAV-1', inputs: 'inputs-1', model: 'model/v1',
  rangeFrom: '2026-11-14T06:00:00Z', rangeTo: '2026-11-14T12:00:00Z', confidence: 'HIGH', predictedAt: '2026-11-13T06:00:00Z',
};

test('预测来源送来的当前版：来源写接口送达，不带录入人与录入时间；口径另占一格', () => {
  const row = etaPredictionRowOf(sourced);
  equal(row.key, 'ETAV-1');
  equal(row.basis, '承运商告知');
  equal(row.entryWord, '接口送达');
  equal(row.entry, '接口送达');
  equal(row.enteredAt, '');
  equal(row.priorVersion, '');
  ok(row.range.includes(' ～ '), row.range);
});

test('人工录入的当前版：带出谁、原件与录入时间；口径照登记的词，认不得的词原样显示', () => {
  const row = etaPredictionRowOf({
    ...sourced, version: 'ETAV-2', priorVersion: 'ETAV-1', source: 'SYN-UNKNOWN', model: 'MANUAL_ENTRY',
    manualEntry: { enteredBy: 'SYN-OP-6', original: '承运商到港通知.pdf', enteredAt: '2026-11-13T06:00:00Z' },
  });
  equal(row.entryWord, '人工录入');
  equal(row.entry, '人工录入｜SYN-OP-6 录入，原件：承运商到港通知.pdf');
  ok(row.enteredAt.startsWith('2026-11-13'), row.enteredAt);
  equal(row.priorVersion, 'ETAV-1');
  equal(row.basis, 'SYN-UNKNOWN');
});
