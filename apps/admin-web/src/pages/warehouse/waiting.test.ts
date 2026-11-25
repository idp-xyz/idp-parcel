import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { durationLabel, elapsedLabel } from './waiting';

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

test('规定时长能整除的取最大的单位，不能整除的退到更小的单位，不四舍五入', () => {
  equal(durationLabel(30 * 86_400), '30 天');
  equal(durationLabel(36 * 3_600), '36 小时');
  equal(durationLabel(90 * 60), '90 分钟');
  equal(durationLabel(90), '90 秒');
  equal(durationLabel(86_400 + 60), '1441 分钟');
});

test('规定时长不是正数就不写', () => {
  equal(durationLabel(0), '');
  equal(durationLabel(-60), '');
  equal(durationLabel(Number.NaN), '');
});
