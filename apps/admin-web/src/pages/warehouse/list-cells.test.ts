import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { fillerColumnId, fitContentClass, nameOf, packColumns, shortInstant } from './list-cells';

test('各列按内容定宽，末尾补一列空列占剩下的宽度', () => {
  const packed = packColumns<{ id: string }>([
    { id: 'number', header: '单号', render: (row) => row.id },
    { id: 'quantity', header: '件数', align: 'right', className: 'tabular-nums', render: () => '3' },
  ]);
  deepEqual(packed.map((column) => column.id), ['number', 'quantity', fillerColumnId]);
  equal(packed[0].className, fitContentClass);
  equal(packed[1].className, `${fitContentClass} tabular-nums`);
  equal(packed[1].align, 'right');
  equal(packed[2].className, 'w-full');
  equal(packed[2].render({ id: 'x' }), null);
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
