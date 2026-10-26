import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { publicationSummary } from './publication-summary';

// 票 party-commercial-uiux/11：预览通过后与发布确认框里复述的版本信息。
test('复述类别、对象、版本号、范围与本地加 UTC 两份生效起止', () => {
  deepEqual(
    publicationSummary(
      {
        kind: 'PRICE_RULE',
        objectId: 'SYN-PRICE-RULE-CN-SG',
        version: 'v2',
        scope: 'SYN-SCOPE-01',
        effectiveStartsAt: '2026-09-30T16:00:00Z',
        effectiveEndsAt: '2027-03-31T16:00:00Z',
      },
      'Asia/Shanghai',
    ),
    [
      { label: '类别', value: '商业价格政策' },
      { label: '对象', value: 'SYN-PRICE-RULE-CN-SG' },
      { label: '版本号', value: 'v2' },
      { label: '适用范围', value: 'SYN-SCOPE-01' },
      { label: '生效起点', value: '2026-10-01 00:00:00 UTC+8（UTC 2026-09-30T16:00:00Z）' },
      { label: '生效止点', value: '2027-04-01 00:00:00 UTC+8（UTC 2027-03-31T16:00:00Z）' },
    ],
  );
});

test('没有止点即持续有效', () => {
  const facts = publicationSummary(
    { kind: 'CREDIT_POLICY', objectId: 'CP', version: 'v1', scope: 'S', effectiveStartsAt: '2026-09-30T16:00:00Z' },
    'Asia/Shanghai',
  );
  deepEqual(facts[5], { label: '生效止点', value: '持续有效' });
});
