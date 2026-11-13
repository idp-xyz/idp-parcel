import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { trackingEntryEndpoint, trackingEntryHint, trackingEntryLines } from './tracking-entry';

test('录入轨迹的提示说清每条要填什么、原件必填、标识会加前缀', () => {
  equal(trackingEntryEndpoint, '/transport-fulfillment-external-tracking-manual-entries');
  for (const word of ['entries', 'source', 'credential', 'status', 'occurredAt', 'original', 'TRKMANUAL/', '200']) ok(trackingEntryHint.includes(word), word);
});

test('逐条答复折成人读的话，带原因、包裹与事件号', () => {
  deepEqual(
    trackingEntryLines({
      outcome: 'ENTRIES_PROCESSED',
      entries: [
        { index: 0, outcome: 'TRACKING_FACT_ADOPTED_PENDING_JUDGMENT', object: 'SYN-PARCEL-1', eventId: 'TRKMANUAL/EVT-1' },
        { index: 1, outcome: 'MATERIAL_LEFT_UNADOPTED', reason: 'CREDENTIAL_UNKNOWN', eventId: 'TRKMANUAL/EVT-2' },
        { index: 2, outcome: 'SOMETHING_NEW' },
      ],
    }),
    [
      '第 1 条：已录入，等待时间确认，包裹 SYN-PARCEL-1，事件号 TRKMANUAL/EVT-1',
      '第 2 条：没有录成事实，原因已记下（这个承运凭证没有登记到任何包裹上），事件号 TRKMANUAL/EVT-2',
      '第 3 条：SOMETHING_NEW',
    ],
  );
  deepEqual(trackingEntryLines({ outcome: 'ENTRIES_PROCESSED' }), []);
});
