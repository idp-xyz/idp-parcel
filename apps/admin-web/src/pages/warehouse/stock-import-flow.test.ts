import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { stockImportTotals, stockImportTrack } from './stock-import-flow';

test('待批停在第一步，用待办色；入账走到第二步', () => {
  const pending = stockImportTrack('PENDING');
  equal(pending.currentId, 'PENDING');
  equal(pending.currentLabel, '待批');
  equal(pending.halt, undefined);

  const posted = stockImportTrack('POSTED');
  equal(posted.currentId, 'POSTED');
  equal(posted.currentLabel, undefined);
  equal(posted.halt, undefined);
});

test('驳回是流程停下：红色停止标记，不当作走完了哪一步', () => {
  const rejected = stockImportTrack('REJECTED');
  deepEqual(rejected.halt, { label: '已驳回', tone: 'danger' });
  equal(rejected.currentId, 'PENDING');
  equal(rejected.currentLabel, undefined);
});

test('陌生的状态词原样示在第一步，不猜它走到了哪', () => {
  const unknown = stockImportTrack('ARCHIVED');
  equal(unknown.currentId, 'PENDING');
  equal(unknown.currentLabel, 'ARCHIVED');
});

test('合计按商品与库位去重，整箱里装的商品也算', () => {
  const totals = stockImportTotals({
    goodsLines: [
      { locationCode: 'L1', goodsCode: 'G1' },
      { locationCode: 'L1', goodsCode: 'G2' },
      { locationCode: 'L2', goodsCode: 'G1' },
    ],
    cartonLines: [
      { locationCode: 'L3', contents: [{ goodsCode: 'G2' }, { goodsCode: 'G9' }] },
      { locationCode: 'L1', contents: [{ goodsCode: 'G9' }] },
    ],
  });
  deepEqual(totals, { goodsLines: 3, cartonLines: 2, goods: 3, locations: 3 });
});

test('没有任何行的导入合计都是 0', () => {
  deepEqual(stockImportTotals({ goodsLines: [], cartonLines: [] }), { goodsLines: 0, cartonLines: 0, goods: 0, locations: 0 });
});
