import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  serviceProductFormHash,
  serviceProductFormTargetOf,
  serviceProductsViewHash,
  serviceProductsViewOf,
} from './service-products-address';

// Covers: 缺省与认不得的值都是目录；两种登记视图各认各的；进出只改这一个键，检索词原样；回目录时键不写。
test('服务产品页的视图住在查询串上', () => {
  equal(serviceProductsViewOf('#/service-products'), 'versions');
  equal(serviceProductsViewOf('#/service-products?tab=publish'), 'publish');
  equal(serviceProductsViewOf('#/service-products?q=SYN&tab=register'), 'register');
  equal(serviceProductsViewOf('#/service-products?tab=json'), 'versions');
  equal(serviceProductsViewHash('#/service-products?q=SYN', 'publish'), '#/service-products?q=SYN&tab=publish');
  equal(serviceProductsViewHash('#/service-products?q=SYN&tab=publish', 'versions'), '#/service-products?q=SYN');
});

test('未登记带这一版进登记视图，离开时把产品、版本、适用范围清掉', () => {
  const entered = serviceProductFormHash('#/service-products?q=SYN', {
    productId: 'SYN-PROD',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
  });
  equal(entered, '#/service-products?q=SYN&tab=register&product=SYN-PROD&version=v1&scope=SYN-SCOPE-01');
  equal(serviceProductsViewOf(entered), 'register');
  equal(serviceProductFormTargetOf(entered)?.version, 'v1');
  equal(serviceProductFormTargetOf('#/service-products?tab=register'), null);
  equal(serviceProductsViewHash(entered, 'versions'), '#/service-products?q=SYN');
});
