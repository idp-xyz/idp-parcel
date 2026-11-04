import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { channelProductCatalogViewHash, channelProductCatalogViewOf } from './channel-product-catalog-address';

// Covers: 缺省与认不得的值都是目录；登记视图认 `register`；进出只改这一个键，检索词原样。
test('渠道产品目录页的视图住在查询串上', () => {
  equal(channelProductCatalogViewOf('#/channel-product-catalog'), 'catalogue');
  equal(channelProductCatalogViewOf('#/channel-product-catalog?q=SYN&tab=register'), 'register');
  equal(channelProductCatalogViewOf('#/channel-product-catalog?tab=x'), 'catalogue');
  equal(channelProductCatalogViewHash('#/channel-product-catalog?q=SYN', 'register'), '#/channel-product-catalog?q=SYN&tab=register');
  equal(channelProductCatalogViewHash('#/channel-product-catalog?q=SYN&tab=register', 'catalogue'), '#/channel-product-catalog?q=SYN');
});
