import type { DirectOperatingDispositionRecord, OperatingResultListResponseBody, OperatingResultRecord } from './api';

/** 经营结果表的一行：有组成项的快照，或答不出毛利的当前答案。两种不能并成「毛利为空」。 */
export type OperatingMetricsRow =
  | { kind: 'result'; record: OperatingResultRecord }
  | { kind: 'disposition'; record: DirectOperatingDispositionRecord };

export type OperatingMetricsClassification =
  | { kind: 'unconfigured' }
  | { kind: 'listed'; rows: OperatingMetricsRow[] };

// 没登记采用是整页一态。已经采用时，快照与答不出毛利的答案一起成行；两者都没有才是「还没有结果」。
export function classifyOperatingMetrics(body: OperatingResultListResponseBody): OperatingMetricsClassification {
  if (body.outcome === 'OPERATING_BASIS_UNCONFIGURED') {
    return { kind: 'unconfigured' };
  }
  const rows: OperatingMetricsRow[] = [
    ...body.results.map((record) => ({ kind: 'result' as const, record })),
    ...body.dispositions.map((record) => ({ kind: 'disposition' as const, record })),
  ];
  return { kind: 'listed', rows };
}

export function operatingMetricsRowKey(row: OperatingMetricsRow): string {
  if (row.kind === 'result') {
    return [row.record.scope, row.record.period, row.record.basis, row.record.version].join(':');
  }
  return [row.record.scope, row.record.basis, row.record.disposition].join(':');
}
