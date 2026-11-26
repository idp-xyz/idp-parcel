import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import {
  canRecordPartialShipmentConsent,
  partialShipmentLabel,
  partialShipmentOptions,
  partialShipmentSourceLabel,
  partialShipmentSummary,
} from './partial-shipment';

// Covers: 三个词各有中文；没写明答未配置，不当成其中任何一种。
test('部分发货规则三个词各有中文，没写明答未配置', () => {
  deepEqual(
    partialShipmentOptions.map((option) => option.value),
    ['SHIP_COMPLETE', 'SHORT_SHIP', 'SPLIT_SHIP'],
  );
  equal(partialShipmentLabel('SHIP_COMPLETE'), '整单发');
  equal(partialShipmentLabel('SHORT_SHIP'), '少发');
  equal(partialShipmentLabel('SPLIT_SHIP'), '拆单发');
  equal(partialShipmentLabel(undefined), '未配置');
  equal(partialShipmentLabel('SOMETHING_NEW'), 'SOMETHING_NEW');
  equal(partialShipmentSourceLabel('OWNER'), '货主登记的默认');
  equal(partialShipmentSourceLabel(undefined), '');
});

// Covers: 哪一层都没写明时如实答未配置，并说清两种处置都做不了。
test('三层都没写明：未配置，不摆默认', () => {
  const summary = partialShipmentSummary({ configured: false });
  equal(summary?.configured, false);
  equal(summary?.headline, '未配置');
  ok(summary?.source.includes('不能少发'));
  deepEqual(summary?.layers, []);
});

// Covers: 适用的那一层说出处，被盖过的几层照实摆出来。
test('代录的同意盖过单上写的与货主默认，被盖过的两层也摆出来', () => {
  const summary = partialShipmentSummary({
    configured: true,
    rule: 'SPLIT_SHIP',
    source: 'ORDER_CONSENT',
    order: 'SHIP_COMPLETE',
    owner: 'SHORT_SHIP',
    consent: { rule: 'SPLIT_SHIP', basis: 'B-1', operator: 'op', recordedAt: '2026-11-26T00:00:00Z' },
  });
  equal(summary?.headline, '拆单发');
  equal(summary?.source, '受理后代录的货主同意');
  deepEqual(summary?.layers, ['提交时写明 整单发', '货主默认 少发']);
});

test('适用货主默认时不把它再列成被盖过的一层', () => {
  const summary = partialShipmentSummary({ configured: true, rule: 'SHORT_SHIP', source: 'OWNER', owner: 'SHORT_SHIP' });
  equal(summary?.source, '货主登记的默认');
  deepEqual(summary?.layers, []);
});

test('读口没答这一格时不猜', () => {
  equal(partialShipmentSummary(undefined), null);
});

// Covers: 走完了的单不摆代录同意的钮。
test('已经出库、取消、拒收、拆开或交给别处履行的单不再收代录的同意', () => {
  for (const status of ['SHIPPED', 'CANCELLED', 'REJECTED', 'SPLIT', 'HANDED_OFF', 'FORWARDED', 'AWAITING_RETURN']) {
    equal(canRecordPartialShipmentConsent(status), false, status);
  }
  for (const status of ['RESERVED', 'PARTIALLY_RESERVED', 'PENDING_POLICY', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED']) {
    equal(canRecordPartialShipmentConsent(status), true, status);
  }
});
