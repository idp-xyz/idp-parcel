// 摘要条「挂起评价数」那一格的判读（ADR-0105 Decision 五；票 pricing-reference-series-operations/05 第 2 项）。
//
// 与 coverage-rows 同一条理由放进纯函数：要钉的是**「没问到」与「真零」不得折成同一个 0**——
// 摘要条上一个 0 会被读成「今天没有评价被挂起」，而 403 / 传输故障 / 还没回来时它其实是「今天没问到」。
// 组件只负责摆。

import type { ApiResult } from '../catalogue-api';
import type { PendingSeriesEvaluationsResponseBody } from './api';

/**
 * 三种答案。**三种不是两种**：
 *
 * - `unasked`——请求还没回来。不摆数字，也不摆 0。
 * - `unavailable`——问了但没得到答案（403 未配置 / 服务端无法作答 / 传输故障）。同样不摆 0；`reason`
 *   分开，因为续办动作不同：未配置去登记接入渠道，其余是故障。
 * - `counted`——服务端作答了。`total` 为 0 时才是真零，措辞才允许说「没有」。
 */
export type PendingEvaluationsCell =
  | { state: 'unasked' }
  | { state: 'unavailable'; reason: 'unconfigured' | 'failed' }
  | {
      state: 'counted';
      asOf: string;
      total: number;
      byKind: { kind: string; evaluationCount: number }[];
    };

export function pendingEvaluationsCellOf(
  answer: ApiResult<PendingSeriesEvaluationsResponseBody> | null,
): PendingEvaluationsCell {
  if (answer === null) return { state: 'unasked' };
  if (answer.kind === 'unconfigured') return { state: 'unavailable', reason: 'unconfigured' };
  if (answer.kind !== 'outcome') return { state: 'unavailable', reason: 'failed' };
  const byKind = answer.body.counts.map((count) => ({
    kind: count.kind,
    evaluationCount: count.evaluationCount,
  }));
  return {
    state: 'counted',
    asOf: answer.body.asOf,
    // 服务端按种类 COUNT(DISTINCT evaluation_id)；一份评价可能同时挂在两种序列上，各种类之和因此
    // 可能大于评价总数。这里的 total 如实是各种类之和，措辞里说「按种类计」而不说「共几份评价」。
    total: byKind.reduce((sum, count) => sum + count.evaluationCount, 0),
    byKind,
  };
}

/** 三种答案三句话；只有 `counted` 且真零那一句允许说「没有」。 */
export function pendingEvaluationsNote(cell: PendingEvaluationsCell): string {
  switch (cell.state) {
    case 'unasked':
      return '挂起评价数：尚未取到。';
    case 'unavailable':
      return cell.reason === 'unconfigured'
        ? '挂起评价数未取到：接入渠道未配置（403）。这是「今天没问到」，不是「没有挂起评价」。'
        : '挂起评价数未取到；这是「今天没问到」，不是「没有挂起评价」。';
    case 'counted':
      return cell.total === 0
        ? '因序列未解析而待判断的评价：没有。'
        : `因序列未解析而待判断的评价（按序列种类计）：${cell.total}`;
  }
}
