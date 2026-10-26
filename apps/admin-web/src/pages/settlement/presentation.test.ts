import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { scopePostingText } from './presentation';

test('资金事实在客户应收作用域上的记法：一个作用域除核到别处的都记，几个按核销，未分配停在哪照种类说', () => {
  equal(
    scopePostingText({ kind: 'RECEIPT_CONFIRMED', customerScopes: 0, unscopedUnappliedAmount: '0' }),
    '不入客户应收作用域',
  );
  equal(
    scopePostingText({ kind: 'RECEIPT_CONFIRMED', customerScopes: 1, unscopedUnappliedAmount: '0' }),
    '记入 1 个作用域（核到别处的除外）',
  );
  equal(
    scopePostingText({ kind: 'RECEIPT_CONFIRMED', customerScopes: 2, unscopedUnappliedAmount: '5000' }),
    '2 个作用域按核销记入；未分配 5000 未计入任何作用域，核到哪个作用域那里才记',
  );
  equal(
    scopePostingText({ kind: 'FUNDS_RETURNED', customerScopes: 2, unscopedUnappliedAmount: '2000' }),
    '2 个作用域按核销记入；未分配 2000 每个作用域都先扣，核到钱离开的那个账户上就只扣那一边',
  );
  equal(
    scopePostingText({ kind: 'RECEIPT_CONFIRMED', customerScopes: 3, unscopedUnappliedAmount: '0' }),
    '3 个作用域按核销记入',
  );
});
