import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { groupedByObject, versionPoolOf, versionStatusFilters, type VersionFacts } from './version-groups';

const id = (facts: VersionFacts) => facts;
const key = (facts: VersionFacts) => `${facts.objectId}@${facts.version}`;

// 读口按对象号排之后的一页：LASTMILE 在前，TRUNK 三版连着、版本号升序。
const agreements: VersionFacts[] = [
  { objectId: 'SYN-SUPPLIER-LASTMILE', version: 'v1', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v1', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v2', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-SUPPLIER-TRUNK', version: 'v3', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' },
];

test('同一对象的各版连续出现、组内新版在上', () => {
  deepEqual(groupedByObject(agreements, id).map(key), [
    'SYN-SUPPLIER-LASTMILE@v1',
    'SYN-SUPPLIER-TRUNK@v3',
    'SYN-SUPPLIER-TRUNK@v2',
    'SYN-SUPPLIER-TRUNK@v1',
  ]);
});

// Covers: 组的先后照读口答的次序——读口按对象号逐字节排（A10 在 A9 前），页面不再按自然序把它们换过来。
test('组的先后照输入次序，不另按对象标识排', () => {
  const rows: VersionFacts[] = [
    { objectId: 'A10', version: 'v1' },
    { objectId: 'A9', version: 'v1' },
    { objectId: 'A10', version: 'v2' },
  ];
  deepEqual(groupedByObject(rows, id).map(key), ['A10@v2', 'A10@v1', 'A9@v1']);
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

// Covers: 「在用」下推成 status=EFFECTIVE；「其余」是词表里已生效以外的四格，不含读口会拒的草稿；「全部」这一维缺席。
test('在用与其余下推成读口的状态维', () => {
  deepEqual(versionStatusFilters('all'), {});
  deepEqual(versionStatusFilters('in-use'), { status: ['EFFECTIVE'] });
  deepEqual(versionStatusFilters('other'), { status: ['PUBLISHED', 'EXPIRED', 'RETIRED', 'SUPERSEDED'] });
});

// Covers: 筛过的一页缺同一对象的别的版本，并进不带条件的那一页；同一版只留一份，这一页的那份在前。
test('版本池并进不带条件的那一页，同一版只留一份', () => {
  const page: VersionFacts[] = [{ objectId: 'SYN-SUPPLIER-TRUNK', version: 'v3', status: 'EFFECTIVE' }];
  deepEqual(versionPoolOf(page, agreements, key).map(key), [
    'SYN-SUPPLIER-TRUNK@v3',
    'SYN-SUPPLIER-LASTMILE@v1',
    'SYN-SUPPLIER-TRUNK@v1',
    'SYN-SUPPLIER-TRUNK@v2',
  ]);
});
