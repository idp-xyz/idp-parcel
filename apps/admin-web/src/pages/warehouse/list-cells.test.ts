import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { fillerColumnId, fitContentClass, flexColumnClass, linesSummary, nameOf, packColumns, shortInstant } from './list-cells';

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

test('点名吃宽度的列拿多出来的宽度，这时不再补空列', () => {
  const packed = packColumns<{ id: string }>(
    [
      { id: 'number', header: '单号', render: (row) => row.id },
      { id: 'address', header: '地址', render: () => '某路 1 号' },
      { id: 'next', header: '下一步', render: () => '去拣货' },
    ],
    ['address'],
  );
  deepEqual(packed.map((column) => column.id), ['number', 'address', 'next']);
  equal(packed[0].className, fitContentClass);
  equal(packed[1].className, flexColumnClass(1));
  equal(packed[2].className, fitContentClass);
});

test('几列一起吃宽度时各占一份，加起来正好一整份', () => {
  const packed = packColumns<{ id: string }>(
    [
      { id: 'address', header: '地址', render: () => '某路 1 号' },
      { id: 'goods', header: '商品', render: () => 'SKU-A × 2' },
    ],
    ['address', 'goods'],
  );
  equal(packed[0].className, flexColumnClass(2));
  equal(packed[1].className, flexColumnClass(2));
  equal(flexColumnClass(1).startsWith('w-full '), true);
  equal(flexColumnClass(2).startsWith('w-1/2 '), true);
  equal(flexColumnClass(3).startsWith('w-1/3 '), true);
  equal(flexColumnClass(7), flexColumnClass(3));
});

test('点名的列不在表里，照没有点名办', () => {
  const packed = packColumns<{ id: string }>([{ id: 'number', header: '单号', render: (row) => row.id }], ['address']);
  deepEqual(packed.map((column) => column.id), ['number', fillerColumnId]);
});

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
