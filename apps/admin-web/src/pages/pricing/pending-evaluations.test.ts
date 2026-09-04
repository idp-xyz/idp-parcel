import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import { pendingEvaluationsCellOf, pendingEvaluationsNote } from './pending-evaluations';

// 本文件钉的是**「没问到」与「真零」不得折成同一个 0**（票 05 MCP-4 那条 Comment 的理由）：
// 摘要条上一个 0 会被读成「今天没有评价被挂起」，而 403 / 故障 / 未回来时它其实是「今天没问到」。

const asOf = '2026-09-04T00:00:00Z';

// Covers: 未回来、未配置、故障三种都不摆数字；措辞里明说「没问到」，不说「没有」。
test('没问到的三种形态都不是 0，措辞不说「没有」', () => {
  const unasked = pendingEvaluationsCellOf(null);
  equal(unasked.state, 'unasked');
  ok(!/没有/.test(pendingEvaluationsNote(unasked)));

  const unconfigured = pendingEvaluationsCellOf({ kind: 'unconfigured' });
  deepEqual(unconfigured, { state: 'unavailable', reason: 'unconfigured' });
  match(pendingEvaluationsNote(unconfigured), /403/);
  match(pendingEvaluationsNote(unconfigured), /没问到/);

  const failed = pendingEvaluationsCellOf({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  deepEqual(failed, { state: 'unavailable', reason: 'failed' });
  match(pendingEvaluationsNote(failed), /没问到/);

  const transport = pendingEvaluationsCellOf({ kind: 'transport', message: 'down' });
  deepEqual(transport, { state: 'unavailable', reason: 'failed' });
});

// Covers: 服务端作答且空数组才是真零，措辞才允许说「没有」；asOf 随答案带出。
test('服务端答空数组才是真零', () => {
  const cell = pendingEvaluationsCellOf({
    kind: 'outcome',
    status: 200,
    body: { outcome: 'PENDING_SERIES_EVALUATIONS_COUNTED', asOf, counts: [] },
  });
  deepEqual(cell, { state: 'counted', asOf, total: 0, byKind: [] });
  match(pendingEvaluationsNote(cell), /没有/);
});

// Covers: 有数时按种类透出，总数是各种类之和（服务端按种类 COUNT(DISTINCT)，一份评价可挂两种序列）。
test('有数时逐种类透出，总数按种类相加', () => {
  const cell = pendingEvaluationsCellOf({
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'PENDING_SERIES_EVALUATIONS_COUNTED',
      asOf,
      counts: [
        { kind: 'EXCHANGE_RATE', evaluationCount: 1 },
        { kind: 'FUEL_RATE', evaluationCount: 2 },
      ],
    },
  });
  equal(cell.state, 'counted');
  if (cell.state !== 'counted') return;
  equal(cell.total, 3);
  deepEqual(cell.byKind, [
    { kind: 'EXCHANGE_RATE', evaluationCount: 1 },
    { kind: 'FUEL_RATE', evaluationCount: 2 },
  ]);
  match(pendingEvaluationsNote(cell), /按序列种类计.*3/);
});
