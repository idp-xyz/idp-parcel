import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { disputedOnlyOf, disputedStatementsHash, reconciliationHash } from './reconciliation-address';

test('地址里没写时不只看争议', () => {
  equal(disputedOnlyOf('#/reconciliation'), false);
});

test('首页待对账那一格的地址只看挂着未了结异议的', () => {
  equal(disputedStatementsHash, '#/reconciliation?only=disputed');
  equal(disputedOnlyOf(disputedStatementsHash), true);
});

test('只看争议写进地址、取消时清掉，别的键原样', () => {
  equal(reconciliationHash('#/reconciliation?q=SYN', true), '#/reconciliation?q=SYN&only=disputed');
  equal(reconciliationHash('#/reconciliation?q=SYN&only=disputed', false), '#/reconciliation?q=SYN');
});
