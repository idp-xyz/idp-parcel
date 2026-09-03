import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ReferenceSeriesRecord } from './api';
import { visibleSeriesRows } from './catalogue-filter';

// 本文件钉的是「只看这条序列」与「搜索词」是两种不同的收窄，不得折成一种：前者精确到
// （标识 + 种类），后者是包含匹配。折成一种（把只看做成往搜索框里填标识）会让前缀同名的
// 兄弟序列混进来，而看的人以为自己只看着一条。

function record(over: Partial<ReferenceSeriesRecord>): ReferenceSeriesRecord {
  return {
    seriesId: 'SYN-PRC-FUEL',
    seriesVersion: 'v1',
    kind: 'FUEL_RATE',
    sourceIdentifier: 'src-a',
    registrant: 'ops-a',
    effectiveFrom: '2026-08-01T00:00:00Z',
    evidenceGrade: 'S',
    canonicalization: 'c14n-1',
    contentDigest: 'digest',
    registeredAt: '2026-08-01T00:00:00Z',
    ...over,
  };
}

const rows: ReferenceSeriesRecord[] = [
  record({ seriesId: 'SYN-PRC-FUEL', seriesVersion: 'v1' }),
  record({ seriesId: 'SYN-PRC-FUEL', seriesVersion: 'v2', registrant: 'ops-b' }),
  record({ seriesId: 'SYN-PRC-FUEL-WEEKLY', seriesVersion: 'v1' }),
  record({ seriesId: 'SYN-PRC-FUEL', seriesVersion: 'v1', kind: 'EXCHANGE_RATE' }),
  record({ seriesId: 'SYN-FX-USD', seriesVersion: 'v1', kind: 'EXCHANGE_RATE' }),
];

const keyOf = (row: ReferenceSeriesRecord) => `${row.seriesId}/${row.kind}@${row.seriesVersion}`;

// Covers: 只看按（标识 + 种类）精确匹配——同一序列的各版本都留下；前缀同名的兄弟序列不混进来；
// 同标识另一种类也不混进来，因为覆盖读口本来就把它们当两行。
test('只看一条序列按标识加种类精确匹配，前缀同名与同名异类都不混进来', () => {
  const visible = visibleSeriesRows(rows, {
    focus: { seriesId: 'SYN-PRC-FUEL', kind: 'FUEL_RATE' },
    keyword: '',
  });
  deepEqual(visible.map(keyOf), ['SYN-PRC-FUEL/FUEL_RATE@v1', 'SYN-PRC-FUEL/FUEL_RATE@v2']);
});

// Covers: 不只看时不收窄——null 是「没选」，不是「选了空串」。
test('没有只看时全部可见', () => {
  equal(visibleSeriesRows(rows, { focus: null, keyword: '' }).length, rows.length);
});

// Covers: 搜索词是包含匹配、不分大小写、扫标识 / 来源 / 登记责任方三格——这是页面原有行为，
// 搬进纯函数时原样钉住。
test('搜索词按包含匹配三格且不分大小写', () => {
  equal(visibleSeriesRows(rows, { focus: null, keyword: 'fuel' }).length, 4);
  equal(visibleSeriesRows(rows, { focus: null, keyword: 'OPS-B' }).length, 1);
  equal(visibleSeriesRows(rows, { focus: null, keyword: '  src-a ' }).length, rows.length);
});

// Covers: 两种收窄叠加，先只看再搜——在这条序列里还能按责任方再筛，而不是二选一。
test('只看与搜索词叠加而不是二选一', () => {
  const visible = visibleSeriesRows(rows, {
    focus: { seriesId: 'SYN-PRC-FUEL', kind: 'FUEL_RATE' },
    keyword: 'ops-b',
  });
  deepEqual(visible.map(keyOf), ['SYN-PRC-FUEL/FUEL_RATE@v2']);
});
