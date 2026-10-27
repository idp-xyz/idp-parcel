import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ProductChannelMappingRecord } from './api';
import { mappingHistoryOf } from './mapping-history';

function revision(n: number, channels: string[], over: Partial<ProductChannelMappingRecord> = {}): ProductChannelMappingRecord {
  return {
    tenantId: 'T',
    mappingId: 'M',
    revision: n,
    productObjectId: 'P',
    productVersionLabel: 'v1',
    channels,
    basis: `BASIS-${n}`,
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: `2026-0${n}-02T00:00:00Z`,
    ...over,
  };
}

test('映射历史：新的在前，每一笔写渠道、依据与登记时刻', () => {
  const entries = mappingHistoryOf([revision(1, ['CH-A']), revision(2, ['CH-B', 'CH-C'])]);
  deepEqual(entries.map((entry) => entry.version), ['2', '1']);
  deepEqual(entries.map((entry) => entry.label), ['第 2 修订', '第 1 修订']);
  equal(entries[0].note?.startsWith('渠道：CH-B、CH-C；依据：BASIS-2；登记于'), true);
  equal(entries[1].note?.startsWith('渠道：CH-A；依据：BASIS-1；'), true);
});

test('映射历史：显式未配置的一笔如实写出，不留空；有终点的区间照列', () => {
  const [entry] = mappingHistoryOf([revision(1, [], { effectiveEndsAt: '2027-01-01T00:00:00Z' })]);
  equal(entry.note?.includes('渠道：未配置（显式登记）'), true);
  equal(entry.interval.endsWith('持续有效'), false);
  equal(entry.interval.includes('2027-01-01'), true);
});
