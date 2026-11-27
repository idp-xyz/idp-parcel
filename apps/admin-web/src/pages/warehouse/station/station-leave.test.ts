import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { pendingLeave } from './station-leave';

test('没数、也没在送，离开不拦', () => {
  equal(pendingLeave('goods', 0), null);
  equal(pendingLeave('waiting', 0), null);
  equal(pendingLeave('location', 0), null);
});

test('已经数了还没送出，离开要问', () => {
  equal(pendingLeave('goods', 3), '已经数了 3 件，还没送出。离开这些数就没了。');
  equal(pendingLeave('checking', 1)?.includes('已经数了 1 件'), true);
});

test('这一笔还没送到，离开要问，免得丢掉事实号', () => {
  equal(pendingLeave('unsent', 0)?.includes('作业事实号'), true);
  equal(pendingLeave('submitting', 4)?.includes('作业事实号'), true);
  equal(pendingLeave('check-unsent', 2)?.includes('作业事实号'), true);
  equal(pendingLeave('pack-pending', 0)?.includes('作业事实号'), true);
});
