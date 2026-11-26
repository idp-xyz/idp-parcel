import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import {
  awaitsShortPickDisposal,
  canRecordPartialShipmentConsent,
  partialShipmentLabel,
  partialShipmentOptions,
  partialShipmentSourceLabel,
  partialShipmentSummary,
  shortPickDisposalChoice,
  shortPickDisposalLine,
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

// Covers: 只有已预占、任务都有了结果、已有货拣进待出库的单算拣不齐等处置。
test('拣不齐等处置：已预占、没有待执行的任务、已有已拣', () => {
  equal(awaitsShortPickDisposal('RESERVED', [{ status: 'SHORT' }, { status: 'DONE' }], 6), true);
  equal(awaitsShortPickDisposal('RESERVED', [{ status: 'SHORT' }, { status: 'OPEN' }], 2), false);
  equal(awaitsShortPickDisposal('RESERVED', [], 0), false);
  equal(awaitsShortPickDisposal('RESERVED', [{ status: 'CANCELLED' }, { status: 'LATE' }], 0), false);
  equal(awaitsShortPickDisposal('ALLOCATED', [{ status: 'SHORT' }], 2), false);
  equal(awaitsShortPickDisposal('PICKED', [{ status: 'SHORT' }], 2), false);
});

// Covers: 只在货主许了少发时摆「按已拣数量发货」；别的三种说清为什么不摆。
test('拣不齐的处置只摆货主许了的那一个动作', () => {
  const short = shortPickDisposalChoice({ configured: true, rule: 'SHORT_SHIP', source: 'OWNER' }, 6, 10, false);
  equal(short.action, 'short-shipment');
  ok(short.note.includes('少的 4'));
  const complete = shortPickDisposalChoice({ configured: true, rule: 'SHIP_COMPLETE', source: 'ORDER' }, 6, 10, false);
  equal(complete.action, null);
  ok(complete.note.includes('整单发'));
  const split = shortPickDisposalChoice({ configured: true, rule: 'SPLIT_SHIP', source: 'ORDER_CONSENT' }, 6, 10, false);
  equal(split.action, null);
  ok(split.note.includes('拆单发'));
  const unset = shortPickDisposalChoice({ configured: false }, 6, 10, false);
  equal(unset.action, null);
  ok(unset.note.includes('没写明'));
  equal(shortPickDisposalChoice(undefined, 6, 10, false).action, null);
  // 预取过估算委托的单，货主许了少发也不摆。
  const prefetched = shortPickDisposalChoice({ configured: true, rule: 'SHORT_SHIP', source: 'OWNER' }, 6, 10, true);
  equal(prefetched.action, null);
  ok(prefetched.note.includes('预取'));
});

test('办过的处置说清要发、实发与照的是哪一层', () => {
  equal(
    shortPickDisposalLine({ rule: 'SHORT_SHIP', source: 'OWNER', wanted: 10, shipped: 6, operator: 'op', recordedAt: '2026-11-26T00:00:00Z' }),
    '已按已拣数量发货：要发 10，实发 6，照货主登记的默认',
  );
});
