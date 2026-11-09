import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  priceCardListConditions,
  priceCardRowKey,
  priceCardsOfStatus,
  priceCardStatusFilterOptions,
  priceCardStatusFiltersPage,
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

// Covers: 「全部」不带 retired；「已退役」下推 retired=true；其余四态都在未退役的版本里，下推 retired=false；检索词照带。
test('状态胶囊下推成 retired 一维', () => {
  deepEqual(priceCardListConditions('SYN', 'ALL'), { q: 'SYN' });
  deepEqual(priceCardListConditions('SYN', 'RETIRED'), { q: 'SYN', filters: { retired: ['true'] } });
  for (const status of ['IN_FORCE', 'NOT_YET', 'EXPIRED', 'CUT_OFF'] as const) {
    deepEqual(priceCardListConditions('', status), { q: '', filters: { retired: ['false'] } }, status);
  }
});

// Covers: 页内只按那四态现算再筛；「全部」与「已退役」读口已筛完，原样交出。
test('页内状态筛选按此刻判，只管读口筛不了的那四态', () => {
  deepEqual(
    priceCardStatusFilterOptions().map((option) => [option.value, priceCardStatusFiltersPage(option.value)]),
    [
      ['ALL', false],
      ['IN_FORCE', true],
      ['NOT_YET', true],
      ['EXPIRED', true],
      ['CUT_OFF', true],
      ['RETIRED', false],
    ],
  );
  equal(priceCardsOfStatus(rows, 'ALL', now).length, 4);
  equal(priceCardsOfStatus(rows, 'RETIRED', now).length, 4);
  deepEqual(priceCardsOfStatus(rows, 'EXPIRED', now).map(priceCardRowKey), ['SYN-PLAN-B@v1']);
  deepEqual(priceCardsOfStatus(rows, 'NOT_YET', now).map(priceCardRowKey), ['SYN-PLAN-B@v2']);
  equal(priceCardsOfStatus(rows, 'CUT_OFF', now).length, 0);
});

// Covers: 胶囊旁那一句——「已退役」按整本册筛，不说只筛本页；其余时候点明除已退役外只筛本页；选了页内那四态且拿到答复才报数。
test('页内筛选的说明句', () => {
  equal(priceCardStatusScopeNote(true, 4, 4, 'ALL'), '除已退役外，状态只筛本页');
  equal(priceCardStatusScopeNote(true, 4, 4, 'RETIRED'), '');
  equal(priceCardStatusScopeNote(true, 4, 1, 'EXPIRED'), '已过期只筛本页：本页未退役的 4 张里有 1 张');
  equal(priceCardStatusScopeNote(false, 0, 0, 'EXPIRED'), '除已退役外，状态只筛本页');
});

test('状态选项', () => {
  deepEqual(
    priceCardStatusFilterOptions().map((option) => option.label),
    ['全部', '生效中', '未生效', '已过期', '已截止', '已退役'],
  );
});
