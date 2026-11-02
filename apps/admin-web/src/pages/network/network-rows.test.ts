import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { adjustmentWindowOf, routeSummaryOf, timeZoneLabel, validityOf, versionWord } from './network-rows';
import { statusEntryFor } from '../../vocabulary/status';

const now = new Date('2026-10-28T12:00:00Z');

// Covers: 半开区间——起点前未生效、起点当刻起生效、截止当刻起已到期；起点缺席或写坏答 null 不猜。
test('版本按此刻落在区间哪一边', () => {
  equal(validityOf('2026-11-01T00:00:00Z', undefined, now), 'NOT_YET_EFFECTIVE');
  equal(validityOf('2026-10-28T12:00:00Z', undefined, now), 'IN_FORCE');
  equal(validityOf('2026-01-01T00:00:00Z', '2026-10-28T12:00:00Z', now), 'EXPIRED');
  equal(validityOf('2026-01-01T00:00:00Z', '2026-12-01T00:00:00Z', now), 'IN_FORCE');
  equal(validityOf(undefined, undefined, now), null);
  equal(validityOf('不是时间', undefined, now), null);
});

// Covers: 停运调整过了解除时刻叫「已解除」，不叫「已到期」；三种码都有中文。
test('停运与恢复的起止', () => {
  equal(adjustmentWindowOf('2026-07-15T00:00:00Z', '2026-07-17T00:00:00Z', now), 'LIFTED');
  equal(adjustmentWindowOf('2026-10-01T00:00:00Z', undefined, now), 'IN_FORCE');
  for (const code of ['IN_FORCE', 'NOT_YET_EFFECTIVE', 'EXPIRED', 'LIFTED']) {
    equal(/[A-Za-z]/.test(statusEntryFor(code).label), false, `${code} 没有中文`);
  }
});

// Covers: 路线首尾相接成一句；派送段的终点是区域引用，说成「末端派送」；没有段答空串。
test('路线一句话', () => {
  equal(
    routeSummaryOf([
      { reference: 'P#1', kind: 'BETWEEN_NODES', from: 'SHA', to: 'SZX', responsible: 'C1' },
      { reference: 'P#2', kind: 'BETWEEN_NODES', from: 'SZX', to: 'SIN', responsible: 'C2' },
      { reference: 'P#3', kind: 'DELIVERY', from: 'SIN', to: 'AREA-SG@1', responsible: 'AREA-SG@1' },
    ]),
    'SHA → SZX → SIN → 末端派送',
  );
  equal(routeSummaryOf([]), '');
  equal(routeSummaryOf(undefined), '');
});

// Covers: 时区取不出中文名就原样交回，不抛；版本号写成「第 N 版」。
test('时区与版本号的屏上写法', () => {
  equal(timeZoneLabel('Not/AZone'), 'Not/AZone');
  equal(timeZoneLabel(undefined), '—');
  equal(versionWord(2), '第 2 版');
  equal(versionWord(undefined), '');
});
