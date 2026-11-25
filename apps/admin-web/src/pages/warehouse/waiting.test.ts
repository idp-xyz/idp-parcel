import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { elapsedLabel } from './waiting';

const now = new Date('2026-11-25T08:00:00Z');

test('只取最大的一个单位', () => {
  equal(elapsedLabel('2026-11-25T07:59:30Z', now), '不到 1 分钟');
  equal(elapsedLabel('2026-11-25T07:20:00Z', now), '40 分钟');
  equal(elapsedLabel('2026-11-25T05:47:00Z', now), '2 小时');
  equal(elapsedLabel('2026-11-22T09:00:00Z', now), '2 天');
});

test('认不出的时刻与落在现在之后的时刻不写', () => {
  equal(elapsedLabel(undefined, now), '');
  equal(elapsedLabel('不是时刻', now), '');
  equal(elapsedLabel('2026-11-25T08:00:05Z', now), '');
});
