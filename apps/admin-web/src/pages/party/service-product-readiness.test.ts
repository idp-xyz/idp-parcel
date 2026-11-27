import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  serviceProductDeliveryAbsentNote,
  serviceProductFollowOn,
  serviceProductFormAbsent,
  serviceProductFormAbsentNote,
} from './service-product-readiness';

test('形态缺席才算未登记，不把空串当成一种形态', () => {
  equal(serviceProductFormAbsent(undefined), true);
  equal(serviceProductFormAbsent(''), true);
  equal(serviceProductFormAbsent('  '), true);
  equal(serviceProductFormAbsent('NETWORK_SERVICE'), false);
});

test('空着的两句说的是后果，接着去的两册不报条数', () => {
  equal(serviceProductFormAbsentNote.includes('还不能按网络服务或面单渠道被选中'), true);
  equal(serviceProductDeliveryAbsentNote.includes('不约定末端交付'), true);
  equal(serviceProductFollowOn.map((item) => item.hash).join(','), '#/channel-product-catalog,#/party-contracts');
});
