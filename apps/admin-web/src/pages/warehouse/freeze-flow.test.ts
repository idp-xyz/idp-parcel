import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { freezeIsCarton, freezeNextStep, freezeQuantityLabel, freezeTrack, freezeWaitingSince } from './freeze-flow';

test('进度条：冻结中卡在等人解冻、用待办色；解冻走到末一步；陌生状态原样示在第一步', () => {
  const frozen = freezeTrack('FROZEN');
  equal(frozen.currentId, 'FROZEN');
  equal(frozen.currentLabel, '冻结中');
  equal(frozen.halt, undefined);

  const unfrozen = freezeTrack('UNFROZEN');
  equal(unfrozen.currentId, 'UNFROZEN');
  equal(unfrozen.currentLabel, undefined);
  equal(unfrozen.halt, undefined);

  const unknown = freezeTrack('ARCHIVED');
  equal(unknown.currentId, 'FROZEN');
  equal(unknown.currentLabel, 'ARCHIVED');
});

test('下一步只有冻结中的有：解冻；已解冻与陌生状态没有', () => {
  equal(freezeNextStep('FROZEN')?.label, '解冻');
  equal(freezeNextStep('UNFROZEN'), null);
  equal(freezeNextStep('ARCHIVED'), null);
});

test('冻了多久从冻结时刻算，已解冻的不在冻着', () => {
  equal(freezeWaitingSince({ status: 'FROZEN', frozenAt: '2026-11-25T01:00:00Z' }), '2026-11-25T01:00:00Z');
  equal(freezeWaitingSince({ status: 'UNFROZEN', frozenAt: '2026-11-25T01:00:00Z' }), undefined);
});

test('冻的是整箱还是商品格：按读口的对象判，陌生的对象不当整箱', () => {
  equal(freezeIsCarton({ target: 'CARTON' }), true);
  equal(freezeIsCarton({ target: 'GOODS' }), false);
  equal(freezeIsCarton({ target: 'MYSTERY' }), false);
});

test('冻结数量连单位：整箱写箱，商品格写数量与单位，没有单位只写数量', () => {
  equal(freezeQuantityLabel({ target: 'CARTON', quantity: 1 }), '1 箱');
  equal(freezeQuantityLabel({ target: 'CARTON', quantity: 1, unit: '件' }), '1 箱');
  equal(freezeQuantityLabel({ target: 'GOODS', quantity: 5, unit: '件' }), '5 件');
  equal(freezeQuantityLabel({ target: 'GOODS', quantity: 5 }), '5');
});
