import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { formatInstant } from '../catalogue-view';
import { measurementRowOf } from './measurement-rows';
import type { ActualMeasurementRecord } from './records-api';

function measurement(overrides: Partial<ActualMeasurementRecord> = {}): ActualMeasurementRecord {
  return {
    sourceId: 'SYN-MEASUREMENT-1',
    objectKind: 'HANDLING_UNIT',
    objectId: 'SYN-HU-1',
    weightValue: '12.5',
    weightUnit: 'KG',
    length: '30',
    width: '20',
    height: '10',
    dimensionUnit: 'CM',
    occurredAt: '2026-10-19T02:00:00Z',
    recordedAt: '2026-10-19T02:00:01Z',
    location: 'SYN-NODE-1',
    basis: 'SYN-BASIS-1',
    performedBy: 'SYN-PARTY-1',
    voided: false,
    ...overrides,
  };
}

// Covers: 行上「对象种类」一格上屏的字——作业实物、集运单元各一词；词表外的种类原样露码，不猜词。
test('称重量方行上的对象种类显示作业实物与集运单元', () => {
  equal(measurementRowOf(measurement({ objectKind: 'HANDLING_UNIT' })).values.objectKind, '作业实物');
  equal(measurementRowOf(measurement({ objectKind: 'CONSOLIDATION_UNIT' })).values.objectKind, '集运单元');
  equal(measurementRowOf(measurement({ objectKind: 'SYN-UNSEEN-KIND' })).values.objectKind, 'SYN-UNSEEN-KIND');
});

// Covers: 行上「作废」一格上屏的字——false 是仍有效，true 是已作废；与册上筛选胶囊同词。
test('称重量方行上的作废一格显示仍有效与已作废', () => {
  equal(measurementRowOf(measurement({ voided: false })).values.voided, '仍有效');
  equal(measurementRowOf(measurement({ voided: true })).values.voided, '已作废');
});

// Covers: 其余各格照登记的原数转写；录入人与原件只在手工录入时出现，缺席转成空串。
test('称重量方行的其余各格照原数转写，手工录入的两格缺席即空', () => {
  const row = measurementRowOf(measurement());
  equal(row.key, 'SYN-MEASUREMENT-1');
  deepEqual(row.values, {
    operationalItem: 'SYN-HU-1',
    weight: '12.5 KG',
    dimensions: '30×20×10 CM',
    occurredAt: formatInstant('2026-10-19T02:00:00Z'),
    voided: '仍有效',
    sourceId: 'SYN-MEASUREMENT-1',
    objectKind: '作业实物',
    location: 'SYN-NODE-1',
    basis: 'SYN-BASIS-1',
    performedBy: 'SYN-PARTY-1',
    enteredBy: '',
    originalDocument: '',
    recordedAt: formatInstant('2026-10-19T02:00:01Z'),
  });
  const manual = measurementRowOf(measurement({ enteredBy: 'SYN-OPERATOR-1', originalDocument: 'SYN-DOC-1' }));
  equal(manual.values.enteredBy, 'SYN-OPERATOR-1');
  equal(manual.values.originalDocument, 'SYN-DOC-1');
});
