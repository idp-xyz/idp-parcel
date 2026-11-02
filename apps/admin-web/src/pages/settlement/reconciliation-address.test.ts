import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { disputedOnlyOf, disputedStatementsHash, reconciliationHash, reconciliationTabOf } from './reconciliation-address';

test('地址里没写签时落在客户对账单签、不只看争议', () => {
  equal(reconciliationTabOf('#/reconciliation'), 'customer');
  equal(disputedOnlyOf('#/reconciliation'), false);
});

test('首页待对账那一格的地址落在客户对账单签、只看挂着未了结异议的', () => {
  equal(disputedStatementsHash, '#/reconciliation?only=disputed');
  equal(reconciliationTabOf(disputedStatementsHash), 'customer');
  equal(disputedOnlyOf(disputedStatementsHash), true);
});

test('只看争议只跟着客户对账单签走；换到供应商签清掉，别的键原样', () => {
  equal(reconciliationHash('#/reconciliation?q=SYN&only=disputed', { tab: 'supplier', disputedOnly: true }), '#/reconciliation?q=SYN&tab=supplier');
  equal(reconciliationHash(disputedStatementsHash, { tab: 'customer', disputedOnly: false }), '#/reconciliation');
  equal(reconciliationTabOf('#/reconciliation?tab=other'), 'customer');
});
