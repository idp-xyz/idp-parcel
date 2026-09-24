import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  currentCursorAfter,
  cursorPageNumber,
  cursorPageSummary,
  cursorPagerControls,
  cursorTotalPages,
  cursorTrailFor,
  nextCursorPage,
  previousCursorPage,
  startCursorTrail,
} from './cursor-pagination';

// 本文件钉的是游标翻页的纯逻辑（票 catalogue-read-pagination/04）：压栈 / 出栈、换条件清栈、总数为 null、末页禁用下一页。
// 翻页条怎么摆在 node:test 里钉不到，用 scripts/dom-probe.mjs 一次性实测，结论写票面。

test('下一页压栈、上一页出栈；页号与要带的 after 跟着走', () => {
  let trail = startCursorTrail('q=');
  equal(cursorPageNumber(trail), 1);
  equal(currentCursorAfter(trail), null);

  trail = nextCursorPage(trail, 'c1');
  trail = nextCursorPage(trail, 'c2');
  equal(cursorPageNumber(trail), 3);
  equal(currentCursorAfter(trail), 'c2');

  trail = previousCursorPage(trail);
  equal(cursorPageNumber(trail), 2);
  equal(currentCursorAfter(trail), 'c1');

  trail = previousCursorPage(trail);
  equal(cursorPageNumber(trail), 1);
  equal(currentCursorAfter(trail), null);
  equal(previousCursorPage(trail), trail, '第一页再上一页原样返回');
});

test('末页：next 为 null 时不压栈，下一页钮不能按；第一页上一页钮不能按', () => {
  const trail = nextCursorPage(startCursorTrail('q='), 'c1');
  equal(nextCursorPage(trail, null), trail);
  deepEqual(cursorPagerControls(2, null), { canPrevious: true, canNext: false });
  deepEqual(cursorPagerControls(1, 'c9'), { canPrevious: false, canNext: true });
  deepEqual(cursorPagerControls(1, null), { canPrevious: false, canNext: false });
});

test('同一个 next 连压两次只算一次（取数期间的第二次点击）', () => {
  const once = nextCursorPage(startCursorTrail('q='), 'c1');
  const twice = nextCursorPage(once, 'c1');
  equal(twice, once);
  equal(cursorPageNumber(twice), 2);
  equal(cursorPageNumber(nextCursorPage(twice, 'c2')), 3, '新的 next 照压');
});

test('换条件清栈回第一页；条件没变原样返回', () => {
  const deep = nextCursorPage(nextCursorPage(startCursorTrail('sort=-registeredAt&q=甲'), 'c1'), 'c2');
  equal(cursorTrailFor(deep, 'sort=-registeredAt&q=甲'), deep);
  const reset = cursorTrailFor(deep, 'sort=-registeredAt&q=乙');
  equal(cursorPageNumber(reset), 1);
  equal(currentCursorAfter(reset), null);
  equal(reset.query, 'sort=-registeredAt&q=乙');
});

test('总页数由 size 与 total 算；total 为 null 不给总页数、翻页条只说第几页', () => {
  equal(cursorTotalPages(200, 812), 5);
  equal(cursorTotalPages(200, 800), 4);
  equal(cursorTotalPages(200, 0), 1, '空目录也算一页');
  equal(cursorTotalPages(200, null), null);
  equal(cursorTotalPages(0, 10), null, '页大小不正不算');
  equal(cursorPageSummary(2, 200, 812), '第 2 / 5 页 · 共 812 条');
  equal(cursorPageSummary(1, 200, 0), '第 1 / 1 页 · 共 0 条');
  equal(cursorPageSummary(3, 200, null), '第 3 页');
});
