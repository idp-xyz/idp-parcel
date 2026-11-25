import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { monitorCursorCanPrevious, monitorCursorNext, monitorCursorPrevious, monitorCursorStart } from './monitor-page';

test('监控从第一页翻到下一页，再退回原来的游标', () => {
  const first = monitorCursorStart();
  equal(first.after, null);
  equal(monitorCursorCanPrevious(first), false);
  const second = monitorCursorNext(first, 'c2');
  equal(second.after, 'c2');
  equal(monitorCursorCanPrevious(second), true);
  deepEqual(monitorCursorPrevious(second), first);
});
