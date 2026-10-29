import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  countPriceCards,
  filterPriceCards,
  priceCardCountSummary,
  priceCardRowKey,
  priceCardStatusFilterOptions,
  type PriceCardListRow,
} from './price-card-list';

const now = new Date('2026-06-01T00:00:00Z');

const rows: PriceCardListRow[] = [
  { planId: 'SYN-PLAN-A', planVersion: 'v1', scope: 'SYN-SCOPE-01', effectiveFrom: '2026-01-01T00:00:00Z' },
  { planId: 'SYN-PLAN-B', planVersion: 'v1', scope: 'SYN-SCOPE-02', effectiveFrom: '2026-01-01T00:00:00Z', effectiveTo: '2026-03-01T00:00:00Z' },
  { planId: 'SYN-PLAN-B', planVersion: 'v2', scope: 'SYN-SCOPE-02', effectiveFrom: '2026-07-01T00:00:00Z' },
  { planId: 'SYN-PLAN-C', planVersion: 'v1', scope: 'SYN-SCOPE-03', effectiveFrom: '2026-01-01T00:00:00Z', retiredAt: '2026-05-01T00:00:00Z' },
];

test('行键是方案标识加版本', () => {
  equal(priceCardRowKey(rows[2]), 'SYN-PLAN-B@v2');
});

test('按状态计数，状态按此刻判', () => {
  deepEqual(countPriceCards(rows, now), {
    total: 4,
    byStatus: { IN_FORCE: 1, NOT_YET: 1, EXPIRED: 1, CUT_OFF: 0, RETIRED: 1 },
  });
});

test('筛选：状态与检索词（方案标识、版本、适用范围）叠加，检索不分大小写', () => {
  equal(filterPriceCards(rows, { search: '', status: 'ALL' }, now).length, 4);
  deepEqual(
    filterPriceCards(rows, { search: '', status: 'EXPIRED' }, now).map(priceCardRowKey),
    ['SYN-PLAN-B@v1'],
  );
  deepEqual(
    filterPriceCards(rows, { search: 'syn-plan-b', status: 'ALL' }, now).map(priceCardRowKey),
    ['SYN-PLAN-B@v1', 'SYN-PLAN-B@v2'],
  );
  deepEqual(filterPriceCards(rows, { search: 'scope-03', status: 'ALL' }, now).map(priceCardRowKey), ['SYN-PLAN-C@v1']);
  deepEqual(filterPriceCards(rows, { search: 'plan-b', status: 'NOT_YET' }, now).map(priceCardRowKey), ['SYN-PLAN-B@v2']);
  equal(filterPriceCards(rows, { search: '没有', status: 'ALL' }, now).length, 0);
});

test('计数句与状态选项', () => {
  equal(priceCardCountSummary(4, 4), '共 4 张价卡');
  equal(priceCardCountSummary(4, 1), '显示 1 / 4 张价卡');
  deepEqual(
    priceCardStatusFilterOptions().map((option) => option.label),
    ['全部', '生效中', '未生效', '已过期', '已截止', '已退役'],
  );
});
