import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  allQueues,
  attentionQueue,
  countedStatuses,
  isWorkingQueue,
  knownQueue,
  queueCount,
  queueStatusesOf,
} from './outbound-queues';

test('待处理一队背后是卡在预占上的两种状态，其余一队一个状态', () => {
  deepEqual(queueStatusesOf(attentionQueue), ['PENDING_POLICY', 'PARTIALLY_RESERVED']);
  deepEqual(queueStatusesOf('RESERVED'), ['RESERVED']);
  deepEqual(queueStatusesOf('SHIPPED'), ['SHIPPED']);
  deepEqual(queueStatusesOf(allQueues), []);
});

test('要数的是还在往下走的各队背后的状态，已出库与已取消不数', () => {
  deepEqual(countedStatuses, ['PENDING_POLICY', 'PARTIALLY_RESERVED', 'RESERVED', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED']);
});

test('待处理的数量是两种状态相加', () => {
  equal(queueCount({ PENDING_POLICY: 1, PARTIALLY_RESERVED: 2 }, attentionQueue), 3);
  equal(queueCount({ PENDING_POLICY: 0, PARTIALLY_RESERVED: 0 }, attentionQueue), 0);
  equal(queueCount({ RESERVED: 4 }, 'RESERVED'), 4);
});

test('有一个状态没答出来，这一队不写数', () => {
  equal(queueCount({ PENDING_POLICY: 1, PARTIALLY_RESERVED: null }, attentionQueue), null);
  equal(queueCount({ PENDING_POLICY: 1 }, attentionQueue), null);
  equal(queueCount({}, 'PACKED'), null);
});

test('还在往下走的队与已了结的队分得开', () => {
  equal(isWorkingQueue(attentionQueue), true);
  equal(isWorkingQueue('PACKED'), true);
  equal(isWorkingQueue('SHIPPED'), false);
  equal(isWorkingQueue(allQueues), false);
});

test('记着的是合并之前单独的卡住状态，照待处理看；认不出的照不筛看', () => {
  equal(knownQueue('PENDING_POLICY'), attentionQueue);
  equal(knownQueue('PARTIALLY_RESERVED'), attentionQueue);
  equal(knownQueue('PICKED'), 'PICKED');
  equal(knownQueue('CANCELLED'), 'CANCELLED');
  equal(knownQueue(allQueues), allQueues);
  equal(knownQueue('NOT-A-QUEUE'), allQueues);
});
