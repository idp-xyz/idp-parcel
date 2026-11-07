import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  priceCardRowKey,
  priceCardsOfStatus,
  priceCardStatusFilterOptions,
  priceCardStatusScopeNote,
} from './price-card-list';
import type { PriceCardStatusInput } from './price-card-content';

const now = new Date('2026-06-01T00:00:00Z');

type Row = PriceCardStatusInput & { planId: string; planVersion: string };

const rows: Row[] = [
  { planId: 'SYN-PLAN-A', planVersion: 'v1', effectiveFrom: '2026-01-01T00:00:00Z' },
  { planId: 'SYN-PLAN-B', planVersion: 'v1', effectiveFrom: '2026-01-01T00:00:00Z', effectiveTo: '2026-03-01T00:00:00Z' },
  { planId: 'SYN-PLAN-B', planVersion: 'v2', effectiveFrom: '2026-07-01T00:00:00Z' },
  { planId: 'SYN-PLAN-C', planVersion: 'v1', effectiveFrom: '2026-01-01T00:00:00Z', retiredAt: '2026-05-01T00:00:00Z' },
];

test('行键是方案标识加版本', () => {
  equal(priceCardRowKey(rows[2]), 'SYN-PLAN-B@v2');
});

// Covers: 状态按此刻现算、只在这一页的行上筛；全部即原样。
test('页内状态筛选按此刻判', () => {
  equal(priceCardsOfStatus(rows, 'ALL', now).length, 4);
  deepEqual(priceCardsOfStatus(rows, 'EXPIRED', now).map(priceCardRowKey), ['SYN-PLAN-B@v1']);
  deepEqual(priceCardsOfStatus(rows, 'NOT_YET', now).map(priceCardRowKey), ['SYN-PLAN-B@v2']);
  equal(priceCardsOfStatus(rows, 'CUT_OFF', now).length, 0);
});

// Covers: 胶囊旁那一句总说「只筛本页」；选了状态且拿到答复才报这一页筛出几张，没答到不报数。
test('页内筛选的说明句', () => {
  equal(priceCardStatusScopeNote(true, 4, 4, 'ALL'), '状态只筛本页');
  equal(priceCardStatusScopeNote(true, 4, 1, 'EXPIRED'), '状态只筛本页：本页 4 张里有 1 张');
  equal(priceCardStatusScopeNote(false, 0, 0, 'EXPIRED'), '状态只筛本页');
});

test('状态选项', () => {
  deepEqual(
    priceCardStatusFilterOptions().map((option) => option.label),
    ['全部', '生效中', '未生效', '已过期', '已截止', '已退役'],
  );
});
