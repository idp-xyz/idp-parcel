import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { workFactSpecs } from '../work-fact';
import { moreJobIds } from './more-jobs';

const stationJobIds = [
  'receipt-goods',
  'receipt-carton',
  'internal-move',
  'count-entry',
  'pick',
  'outbound-check',
  'pack',
  'outbound-confirmation',
  'pickup-handover',
];

test('每种作业要么有全屏工位，要么在更多作业里，没有遗漏也没有重复', () => {
  const covered = new Set<string>([...stationJobIds, ...moreJobIds]);
  equal(covered.size, stationJobIds.length + moreJobIds.length);
  for (const spec of workFactSpecs) ok(covered.has(spec.id), spec.id);
  equal(covered.size, workFactSpecs.length);
});
