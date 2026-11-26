import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { stockPageClamp, stockPageCount, stockPageSlice, stockPageSummary } from './stock-page';

const rows = Array.from({ length: 7 }, (_, index) => index + 1);

test('库存按页切行，末页只剩余下的几行', () => {
  equal(stockPageCount(7, 3), 3);
  deepEqual(stockPageSlice(rows, 1, 3), [1, 2, 3]);
  deepEqual(stockPageSlice(rows, 3, 3), [7]);
});

test('没有行也算一页', () => {
  equal(stockPageCount(0, 3), 1);
  deepEqual(stockPageSlice([], 1, 3), []);
});

test('行数少了之后，停在末页之外的页收回到末页', () => {
  equal(stockPageClamp(5, 7, 3), 3);
  equal(stockPageClamp(0, 7, 3), 1);
  deepEqual(stockPageSlice(rows, 5, 3), [7]);
});

test('整份快照说共多少条，有上限的流水说读到多少条', () => {
  equal(stockPageSummary(2, 7, true, 3), '第 2 / 3 页 · 共 7 条');
  equal(stockPageSummary(1, 7, false, 3), '第 1 / 3 页 · 读到 7 条');
});
