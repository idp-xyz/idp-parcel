import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { versionHistoryOf, type HistoryFacts } from './version-history';

const base = { objectId: 'OBJ', scope: 'S' };
const v1: HistoryFacts = { ...base, version: 'v1', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z', retiredAt: '2026-10-26T00:00:00Z', successorVersion: 'v2' };
const v2: HistoryFacts = { ...base, version: 'v2', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' };
const v3: HistoryFacts = { ...base, version: 'v3', status: 'EFFECTIVE', effectiveStartsAt: '2026-06-01T00:00:00Z' };
const v10: HistoryFacts = { ...base, version: 'v10', status: 'EFFECTIVE', effectiveStartsAt: '2026-06-01T00:00:00Z' };

test('历史：同一对象同一范围的各版，起点晚的在前，同起点按版本号自然序（v10 在 v3 之上）', () => {
  const other = { ...v3, objectId: 'OTHER' };
  const otherScope = { ...v3, version: 'v9', scope: 'S2' };
  const entries = versionHistoryOf([v1, v2, v3, v10, other, otherScope], v2);
  deepEqual(entries.map((entry) => entry.version), ['v10', 'v3', 'v2', 'v1']);
});

test('历史：被退役的写让位给谁与退役时刻，被截止的写截止点，没收尾的没有说明', () => {
  const cut: HistoryFacts = { ...base, version: 'v0', status: 'EFFECTIVE', effectiveStartsAt: '2025-01-01T00:00:00Z', selectionEndsAt: '2026-01-01T00:00:00Z' };
  const entries = versionHistoryOf([cut, v1, v2], v2);
  const byVersion = Object.fromEntries(entries.map((entry) => [entry.version, entry]));
  equal(byVersion.v1.closure?.startsWith('已退役：'), true);
  equal(byVersion.v1.closure?.includes('让位给 v2'), true);
  equal(byVersion.v0.closure?.startsWith('已截止：选用区间止于'), true);
  equal(byVersion.v2.closure, undefined);
});

test('历史：原区间照原样列（持续有效），读面没有状态的册状态为空', () => {
  const noStatus: HistoryFacts = { objectId: 'CP', version: 'v1', effectiveStartsAt: '2026-01-01T00:00:00Z' };
  const [entry] = versionHistoryOf([noStatus], noStatus);
  equal(entry.status, undefined);
  equal(entry.interval.endsWith('持续有效'), true);
});

test('历史：范围缺省的两条（读面没有范围的册）按同一范围算', () => {
  const a: HistoryFacts = { objectId: 'CP', version: 'v1', effectiveStartsAt: '2026-01-01T00:00:00Z' };
  const b: HistoryFacts = { objectId: 'CP', version: 'v2', effectiveStartsAt: '2026-02-01T00:00:00Z' };
  deepEqual(versionHistoryOf([a, b], a).map((entry) => entry.version), ['v2', 'v1']);
});
