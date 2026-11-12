import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { formatMoney, isPositiveMinorAmount } from './money';

// 本文件钉金额的屏上写法：最小币单位的整数串只加千分位、不换单位，费用页与运单详情的受理前估价同一份。

test('最小币单位整数串只加千分位、带币种，不换算成主单位', () => {
  equal(formatMoney('1536', 'CNY'), '1,536 CNY');
  equal(formatMoney('1234567', 'USD'), '1,234,567 USD');
  equal(formatMoney('384', 'CNY'), '384 CNY');
  equal(formatMoney('-120000', 'CNY'), '-120,000 CNY');
});

test('币种空时只出数；金额缺席出空串；非整数串原样回显', () => {
  equal(formatMoney('1000', ''), '1,000');
  equal(formatMoney(undefined, 'CNY'), '');
  equal(formatMoney('384.00', 'CNY'), '384.00 CNY');
});

test('已扣金额是否为正：0 与全零串不算，非整数串不算', () => {
  equal(isPositiveMinorAmount('500'), true);
  equal(isPositiveMinorAmount('0'), false);
  equal(isPositiveMinorAmount('000'), false);
  equal(isPositiveMinorAmount(undefined), false);
  equal(isPositiveMinorAmount('5.00'), false);
});
