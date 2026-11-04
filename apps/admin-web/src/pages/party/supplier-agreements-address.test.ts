import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { supplierAgreementsViewHash, supplierAgreementsViewOf } from './supplier-agreements-address';

// Covers: 缺省与认不得的值都是目录；登记视图认 `publish`；进出只改这一个键，检索词原样。
test('供应商协议页的视图住在查询串上', () => {
  equal(supplierAgreementsViewOf('#/supplier-agreements'), 'catalogue');
  equal(supplierAgreementsViewOf('#/supplier-agreements?q=SYN&tab=publish'), 'publish');
  equal(supplierAgreementsViewOf('#/supplier-agreements?tab=x'), 'catalogue');
  equal(supplierAgreementsViewHash('#/supplier-agreements?q=SYN', 'publish'), '#/supplier-agreements?q=SYN&tab=publish');
  equal(supplierAgreementsViewHash('#/supplier-agreements?q=SYN&tab=publish', 'catalogue'), '#/supplier-agreements?q=SYN');
});
