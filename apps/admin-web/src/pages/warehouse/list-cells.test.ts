import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { linesSummary, nameOf, shortInstant, waitedSortParam } from './list-cells';

test('商品行摆出第一种，其余的说还有几种', () => {
  deepEqual(linesSummary([{ goodsCode: 'SKU-A', quantity: 2 }]), { first: 'SKU-A × 2', rest: '', all: 'SKU-A × 2' });
  deepEqual(
    linesSummary([
      { goodsCode: 'SKU-A', quantity: 2 },
      { goodsCode: 'SKU-B', quantity: 1 },
      { goodsCode: 'SKU-C', quantity: 5 },
    ]),
    { first: 'SKU-A × 2', rest: '另 2 种', all: 'SKU-A × 2\nSKU-B × 1\nSKU-C × 5' },
  );
  deepEqual(linesSummary([]), { first: '', rest: '', all: '' });
});

test('已等从大到小排，读口按受理时刻从早到晚排', () => {
  equal(waitedSortParam(-1), 'receivedAt');
  equal(waitedSortParam(1), '-receivedAt');
});

// 测试里没有配置呈现时区，按 UTC 显。
const now = new Date('2026-11-26T05:00:00Z');

test('今年的时刻不写年份，只到分钟', () => {
  equal(shortInstant('2026-11-20T06:00:07Z', now), '11-20 06:00');
});

test('不是今年的时刻带上年份', () => {
  equal(shortInstant('2025-12-31T23:59:00Z', now), '2025-12-31 23:59');
});

test('没有时刻答空串，认不出的串原样交回', () => {
  equal(shortInstant(undefined, now), '');
  equal(shortInstant('', now), '');
  equal(shortInstant('不是时刻', now), '不是时刻');
});

test('名册里有名称就答名称，没有就答空串', () => {
  const names = new Map([
    ['A-01', '一号货主'],
    ['A-02', '  '],
  ]);
  equal(nameOf(names, 'A-01'), '一号货主');
  equal(nameOf(names, 'A-02'), '');
  equal(nameOf(names, 'A-03'), '');
});
