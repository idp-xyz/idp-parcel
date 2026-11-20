import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { classifyOperatingMetrics } from './operating-metrics';
import type { OperatingResultListResponseBody } from './api';

const page = { size: 0, next: null, total: 0 };

function body(partial: Partial<OperatingResultListResponseBody> & Pick<OperatingResultListResponseBody, 'outcome'>): OperatingResultListResponseBody {
  return { results: [], dispositions: [], page, ...partial };
}

test('没采用口径不是空的结果列表', () => {
  const classified = classifyOperatingMetrics(body({ outcome: 'OPERATING_BASIS_UNCONFIGURED' }));
  equal(classified.kind, 'unconfigured');
});

test('已经采用但还没有结果与未配置分开', () => {
  const classified = classifyOperatingMetrics(body({ outcome: 'OPERATING_RESULTS_LISTED' }));
  equal(classified.kind, 'listed');
  if (classified.kind === 'listed') equal(classified.rows.length, 0);
});

test('判不出、成本不全与带组成项的结果各成一行', () => {
  const classified = classifyOperatingMetrics(
    body({
      outcome: 'OPERATING_RESULTS_LISTED',
      results: [
        {
          scope: 'SYN-PARCEL-LABEL',
          period: '2026-11-20',
          basis: 'ESTIMATED',
          currency: 'CNY',
          margin: '60.00',
          version: 'direct-1',
          asOf: '2026-11-20T08:00:00Z',
          recordedAt: '2026-11-20T08:00:00Z',
          components: [
            { source: 'chg', role: 'CUSTOMER_ESTIMATE', effect: 'INCREASES', amount: '100.00' },
            { source: 'cost', role: 'SUPPLIER_EXPECTED_COST', effect: 'DECREASES', amount: '40.00' },
          ],
        },
      ],
      dispositions: [
        { scope: 'SYN-PARCEL-ONE', basis: 'ESTIMATED', disposition: 'INDETERMINATE', recordedAt: '2026-11-20T08:00:00Z' },
        { scope: 'SYN-PARCEL-NET', basis: 'CONFIRMED', disposition: 'INCOMPLETE_COST', recordedAt: '2026-11-20T08:00:00Z' },
      ],
    }),
  );
  if (classified.kind !== 'listed') {
    throw new Error('应列出');
  }
  deepEqual(
    classified.rows.map((row) => (row.kind === 'result' ? row.record.components.length : row.record.disposition)),
    [2, 'INDETERMINATE', 'INCOMPLETE_COST'],
  );
});
