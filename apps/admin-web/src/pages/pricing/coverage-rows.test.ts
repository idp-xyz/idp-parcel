import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import type { ReferenceSeriesCoverageRecord } from './api';
import { coverageNote, coverageRowOf, remainingDaysBetween } from './coverage-rows';

// 本文件钉的不是渲染，是**哪两种状态不得折成一句**。后端 query_reference_series_coverage.go
// 刻意把在用那一组做成子对象加一个布尔，为的就是让「今天没有在用版本」与「在用但没有终点」
// 分得开；前端若在判读这一步折回去，后端那道设计就白做了。夹具取后端行体的字面形状。

const asOf = '2026-09-03T00:00:00Z';

function record(over: Partial<ReferenceSeriesCoverageRecord>): ReferenceSeriesCoverageRecord {
  return {
    seriesId: 'SYN-PRC-FUEL-WEEKLY',
    kind: 'FUEL_RATE',
    registeredVersionCount: 1,
    inForceResolved: false,
    unreviewedVersionCount: 0,
    returnedVersionCount: 0,
    ...over,
  };
}

// Covers: 三种覆盖答案各自成格——无终点不是「剩余很多天」，没有在用版本不是「剩余 0 天」。
// 后者两句若共用一个破折号，看的人会跑错方向：一个要去登记或复核，一个要去登记下一版。
test('无终点、有止点、没有在用版本是三种答案，不是两种', () => {
  const open = coverageRowOf(
    record({
      inForceResolved: true,
      inForce: { version: 'v2', effectiveFrom: '2026-08-01T00:00:00Z', openEnded: true },
    }),
    asOf,
  );
  equal(open.coverage.kind, 'open');
  equal(open.coverage.remainingDays, undefined);
  equal(coverageNote(open.coverage), '在用版本无终点');

  const bounded = coverageRowOf(
    record({
      inForceResolved: true,
      inForce: {
        version: 'v1',
        effectiveFrom: '2026-08-01T00:00:00Z',
        effectiveTo: '2026-09-13T00:00:00Z',
        openEnded: false,
      },
    }),
    asOf,
  );
  equal(bounded.coverage.kind, 'bounded');
  equal(bounded.coverage.remainingDays, 10);

  const none = coverageRowOf(record({ inForceResolved: false }), asOf);
  equal(none.coverage.kind, 'none');
  equal(coverageNote(none.coverage), '今天没有在用版本');
  // 三句话逐句不同——这一条就是本文件的题目。
  ok(coverageNote(open.coverage) !== coverageNote(none.coverage));
});

// Covers: 剩余天数可以为负，不夹逼到 0。已经过期与「今天到期」是两回事，而按 0 显示会把
// 前者伪装成后者，读的人以为还来得及。
test('在用版本已过期时剩余为负，并如实说过期了几天', () => {
  const expired = coverageRowOf(
    record({
      inForceResolved: true,
      inForce: {
        version: 'v1',
        effectiveFrom: '2026-07-01T00:00:00Z',
        effectiveTo: '2026-08-31T00:00:00Z',
        openEnded: false,
      },
    }),
    asOf,
  );
  equal(expired.coverage.remainingDays, -3);
  equal(coverageNote(expired.coverage), '在用版本已于 3 天前到期');
});

// Covers: 先看布尔再看子对象。布尔说有而子对象没序列化出来，是响应不合契约——按「今天没有
// 在用版本」如实作答，不拿一个半个字段拼出一版在用版本来。
test('布尔说有而子对象缺席时按没有作答，不拼出一版来', () => {
  const broken = coverageRowOf(record({ inForceResolved: true }), asOf);
  equal(broken.coverage.kind, 'none');
  equal(broken.coverage.inForceVersion, undefined);
});

// Covers: openEnded 为假而止点缺席（两者本该成对）同样退回 none，不按 0 天算——按 0 算会
// 把一次不合契约的响应显示成「今天到期」，那是页面凭空造出来的业务结论。
test('有止点却缺止点时退回没有在用版本，不按 0 天算', () => {
  const broken = coverageRowOf(
    record({
      inForceResolved: true,
      inForce: { version: 'v1', effectiveFrom: '2026-08-01T00:00:00Z', openEnded: false },
    }),
    asOf,
  );
  equal(broken.coverage.kind, 'none');
});

// Covers: 两个未复核计数分开保留——等复核人与等登记方更正的续办动作不同，合成一个数字
// 看的人就不知道该去找谁。attention 只回答「要不要有人看一眼」，不回答紧不紧急：紧急要
// 阈值，而阈值是租户参数（票面第 4 项，机制只把数摆出来）。
test('两个待办计数不合并，attention 只答要不要看一眼', () => {
  const both = coverageRowOf(
    record({ unreviewedVersionCount: 2, returnedVersionCount: 1 }),
    asOf,
  );
  equal(both.pending.unreviewed, 2);
  equal(both.pending.returned, 1);
  equal(both.pending.attention, true);

  const clean = coverageRowOf(record({}), asOf);
  equal(clean.pending.attention, false);
});

// Covers: 从未复核过的序列没有「最近一次复核」，那是如实缺席不是零值——显示成一个零时刻
// 会让人以为复核过。
test('从未复核过的序列不给最近复核，而不是给一个零值', () => {
  equal(coverageRowOf(record({}), asOf).lastReview, undefined);

  const reviewed = coverageRowOf(
    record({ lastReviewedAt: '2026-09-01T08:00:00Z', lastReviewDecision: 'APPROVED' }),
    asOf,
  );
  equal(reviewed.lastReview?.decision, 'APPROVED');
});

// Covers: 剩余天数按 UTC 同一条时间轴作差并向下取整。后端刻意不算这个数，理由是要挑时区与
// 舍入口径——这里挑了并钉住，免得它变成一个没人说得清口径的数字。
test('剩余天数按 UTC 作差并向下取整', () => {
  equal(remainingDaysBetween('2026-09-03T00:00:00Z', '2026-09-04T23:00:00Z'), 1);
  equal(remainingDaysBetween('2026-09-03T00:00:00Z', '2026-09-03T23:59:59Z'), 0);
  equal(remainingDaysBetween('not-a-time', '2026-09-04T00:00:00Z'), 0);
});
