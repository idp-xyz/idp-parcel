import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  countTrackingByStatus,
  currentEffectiveEntries,
  currentStatusLabelOf,
  defaultTrackingSort,
  filterTrackingRows,
  progressPhrasesOf,
  sortTrackingRows,
  trackingRowOf,
} from './tracking-list';
import type { ProjectionEntryRecord, TrackingProjectionRecord } from './api';

// 本文件钉物流动态工作台的纯逻辑：替代关系只在同源同事实的版本间成立；进度按服务链顺序折成中文短语、缺的源不出现、
// 连续同词只留一个；当前状态经词典、归不上的如实「未归类」；计数按词。

const entry = (overrides: Partial<ProjectionEntryRecord>): ProjectionEntryRecord => ({
  source: 'PARCEL_SHIPMENT',
  fact: 'F-1',
  kind: 'final-outcome-adopted',
  factVersion: 'v1',
  occurredAt: '2026-10-28T10:00:00Z',
  effectiveAt: '2026-10-28T10:00:00Z',
  receivedAt: '2026-10-28T10:00:00Z',
  mappingVersion: 'M-1',
  ...overrides,
});

const record = (entries: ProjectionEntryRecord[], overrides: Partial<TrackingProjectionRecord> = {}): TrackingProjectionRecord => ({
  version: 'PRJ-1',
  parcel: 'DPCL-1',
  derivedAt: '2026-10-28T12:00:00Z',
  entries,
  ...overrides,
});

test('被同源同事实的后版指名为前身的条目不算当前有效；别的源指名不算', () => {
  const entries = [
    entry({ fact: 'F-1', factVersion: 'v1' }),
    entry({ fact: 'F-1', factVersion: 'v2', supersedes: 'v1' }),
    // 别的源指名「v1」为前身：那是它自己那条事实的 v1，不碰托运那条 F-1 的 v1。
    entry({ source: 'NODE_OPERATIONS', fact: 'F-9', factVersion: 'v2', supersedes: 'v1', kind: 'node-intake' }),
  ];
  deepEqual(
    currentEffectiveEntries(entries).map((item) => `${item.source}/${item.factVersion}`),
    ['PARCEL_SHIPMENT/v2', 'NODE_OPERATIONS/v2'],
  );
});

test('进度按发生先后折成中文短语（同时刻按服务链），缺的源不出现，连续同词只留一个', () => {
  const row = trackingRowOf(
    record([
      entry({ source: 'CUSTOMS_COMPLIANCE', fact: 'C-1', kind: 'external-result-release-full' }),
      entry({ source: 'NODE_OPERATIONS', fact: 'N-1', kind: 'node-intake' }),
      entry({ source: 'TRANSPORT_FULFILLMENT', fact: 'T-1', kind: 'node-intake' }),
    ]),
  );
  deepEqual(progressPhrasesOf(row), ['已收寄', '已放行']);
  deepEqual(row.latestBySource.map((item) => item.source), ['NODE_OPERATIONS', 'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE']);
  equal(row.supersededCount, 0);
  equal(row.currentCount, 3);

  // 托运段的终局最后发生：按时间排它落在末尾，按服务链排会把「已完结」排到最前。
  const late = trackingRowOf(
    record([
      entry({ source: 'PARCEL_SHIPMENT', fact: 'F-1', kind: 'final-outcome-adopted', occurredAt: '2026-10-28T20:00:00Z' }),
      entry({ source: 'NODE_OPERATIONS', fact: 'N-1', kind: 'node-intake', occurredAt: '2026-10-28T10:00:00Z' }),
      entry({ source: 'TRANSPORT_FULFILLMENT', fact: 'T-1', kind: 'effective-delivery', occurredAt: '2026-10-28T18:00:00Z' }),
    ]),
  );
  deepEqual(progressPhrasesOf(late), ['已收寄', '已签收', '已完结']);
});

test('当前状态取最后一条已归类条目的里程碑经词典；一条都归不上就是未归类', () => {
  const delivered = trackingRowOf(record([entry({ milestone: 'PICKED_UP' }), entry({ fact: 'F-2', milestone: 'DELIVERED' })]));
  equal(currentStatusLabelOf(delivered), '已签收');
  equal(currentStatusLabelOf(trackingRowOf(record([entry({})]))), '未归类');
});

test('搜索命中运单号或投影版本；状态胶囊按词筛；默认更新时间新→旧', () => {
  const rows = [
    trackingRowOf(record([entry({ milestone: 'DELIVERED' })], { parcel: 'DPCL-A', version: 'PRJ-A', derivedAt: '2026-10-28T10:00:00Z' })),
    trackingRowOf(record([entry({ milestone: 'IN_TRANSIT' })], { parcel: 'DPCL-B', version: 'PRJ-B', derivedAt: '2026-10-28T11:00:00Z' })),
  ];
  equal(filterTrackingRows(rows, { search: 'prj-b', status: 'ALL' }).length, 1);
  equal(filterTrackingRows(rows, { search: '', status: '已签收' })[0].parcelId, 'DPCL-A');
  deepEqual(defaultTrackingSort, { key: 'derived-at', dir: -1 });
  deepEqual(sortTrackingRows(rows, defaultTrackingSort).map((row) => row.parcelId), ['DPCL-B', 'DPCL-A']);
  deepEqual(sortTrackingRows(rows, { key: 'parcel', dir: 1 }).map((row) => row.parcelId), ['DPCL-A', 'DPCL-B']);
  deepEqual(countTrackingByStatus(rows), [
    { label: '已签收', count: 1 },
    { label: '运输中', count: 1 },
  ]);
});
