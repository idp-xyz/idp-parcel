import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { hiddenInBasket, mergeWaveBlocker, refreshedBasket, waveReleaseTookEffect } from './outbound-basket';

interface Row {
  id: string;
  warehouseCode: string;
  status: string;
}

const keyOf = (row: Row) => row.id;
const row = (id: string, status = 'RESERVED', warehouseCode = 'WH-1'): Row => ({ id, warehouseCode, status });

test('眼前这一页重取之后，篮子里也在这一页上的行换成新取回的', () => {
  const kept = row('B');
  const basket = new Map([
    ['A', row('A')],
    ['B', kept],
  ]);
  const fresh = row('A', 'ALLOCATED');
  const next = refreshedBasket(basket, [fresh, row('C')], keyOf);
  equal(next.get('A'), fresh);
  equal(next.get('B'), kept);
  equal(next.has('C'), false);
  equal(next.size, 2);
});

test('没有哪一行换了，交回原来那一份篮子', () => {
  const a = row('A');
  const basket = new Map([['A', a]]);
  equal(refreshedBasket(basket, [a, row('C')], keyOf), basket);
  equal(refreshedBasket(basket, [], keyOf), basket);
});

test('篮子里不在眼前这一页上的单数得出来', () => {
  const basket = new Map([
    ['A', row('A')],
    ['B', row('B')],
    ['C', row('C')],
  ]);
  equal(hiddenInBasket(basket, [row('A'), row('X')], keyOf), 2);
  equal(hiddenInBasket(basket, [row('A'), row('B'), row('C')], keyOf), 0);
  equal(hiddenInBasket(new Map<string, Row>(), [row('A')], keyOf), 0);
});

test('同一个仓、都已预占的几张单可以合成一个波次', () => {
  equal(mergeWaveBlocker([row('A'), row('B')]), null);
});

test('不在同一个仓、或有不是已预占的，合不成，并说为什么', () => {
  equal(mergeWaveBlocker([]), '还没有勾选出库单');
  equal(mergeWaveBlocker([row('A'), row('B', 'RESERVED', 'WH-2')]), '勾选的单不在同一个仓，一个波次只放一个仓的单');
  equal(mergeWaveBlocker([row('A'), row('B', 'ALLOCATED')]), '勾选的单里有不是已预占的，只有已预占的单能放波次');
});

test('放成了才清篮子：新放成的与此前已放成的都算，别的答复不算', () => {
  equal(waveReleaseTookEffect('ALLOCATED'), true);
  equal(waveReleaseTookEffect('EXISTING_RESULT'), true);
  equal(waveReleaseTookEffect('STATUS_MISMATCH'), false);
  equal(waveReleaseTookEffect('CONTENT_CONFLICT'), false);
  equal(waveReleaseTookEffect('NOT_CONFIGURED'), false);
});
