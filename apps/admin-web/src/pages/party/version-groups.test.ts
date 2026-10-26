import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  groupedByObject,
  hasVersionStatus,
  matchesVersionStatus,
  versionCountSummary,
  versionStatusCounts,
  type VersionFacts,
} from './version-groups';

const id = (facts: VersionFacts) => facts;
const key = (facts: VersionFacts) => `${facts.objectId}@${facts.version}`;

// 演示种子里供应商协议的读口顺序：TRUNK 三版夹着 LASTMILE 一版。
const agreements: VersionFacts[] = [
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v1', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-LASTMILE', version: 'v1', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v3', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v2', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z' },
];

test('同一对象的各版连续出现，组内新版在上，组按对象标识排', () => {
  deepEqual(groupedByObject(agreements, id).map(key), [
    'SYN-SUPPLIER-LASTMILE@v1',
    'SYN-SUPPLIER-TRUNK@v3',
    'SYN-SUPPLIER-TRUNK@v2',
    'SYN-SUPPLIER-TRUNK@v1',
  ]);
});

test('组内先比生效起点，晚的在上；起点相同才比版本号，且按自然序', () => {
  const rows: VersionFacts[] = [
    { objectId: 'A', version: 'v9', effectiveStartsAt: '2026-03-01T00:00:00Z' },
    { objectId: 'A', version: 'v10', effectiveStartsAt: '2026-03-01T00:00:00Z' },
    { objectId: 'A', version: 'v11', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  ];
  deepEqual(groupedByObject(rows, id).map(key), ['A@v10', 'A@v9', 'A@v11']);
});

test('排列不改原数组', () => {
  const before = agreements.map(key);
  groupedByObject(agreements, id);
  deepEqual(agreements.map(key), before);
});

test('在用只认服务端答的已生效；其余是已发布未生效、已退役、已到期、已替代', () => {
  const rows: VersionFacts[] = ['EFFECTIVE', 'PUBLISHED', 'RETIRED', 'EXPIRED', 'SUPERSEDED'].map((status, index) => ({
    objectId: 'A',
    version: `v${index}`,
    status,
  }));
  deepEqual(rows.filter((row) => matchesVersionStatus(row, 'in-use')).map((row) => row.status), ['EFFECTIVE']);
  equal(rows.filter((row) => matchesVersionStatus(row, 'other')).length, 4);
  equal(rows.filter((row) => matchesVersionStatus(row, 'all')).length, 5);
});

test('计数：全部、在用、其余', () => {
  deepEqual(versionStatusCounts(agreements, id), { all: 4, inUse: 2, other: 2 });
});

test('没有状态的册不显在用筛选，空册也不显', () => {
  equal(hasVersionStatus(agreements, id), true);
  equal(hasVersionStatus([{ objectId: 'A', version: 'v1' }], id), false);
  equal(hasVersionStatus([], id), false);
});

test('右端计数跟着筛选走', () => {
  equal(versionCountSummary(4, 4, '份', '协议版本'), '共 4 份协议版本');
  equal(versionCountSummary(4, 1, '份', '协议版本'), '共 4 份协议版本，当前显示 1 份');
  equal(versionCountSummary(2, 1, '个', '版本'), '共 2 个版本，当前显示 1 个');
});
