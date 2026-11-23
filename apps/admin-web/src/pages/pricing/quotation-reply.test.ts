import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { quotationReplyPayload } from './quotation-reply';

test('代录载荷不带代录人或登记时刻，接受不带原因，缺客户时刻就缺席', () => {
  const payload = quotationReplyPayload({
    quotationId: 'QTS-1',
    decision: 'ACCEPTED',
    repliedAt: '',
    evidenceIndex: 'SYN-EVIDENCE-01',
    rejectionReason: '不该出现',
    contentDigest: 'digest',
  });
  equal(payload.repliedAt, undefined);
  equal(payload.rejectionReason, '');
  equal('recorder' in payload, false);
  equal('recordedAt' in payload, false);
  equal('tenantId' in payload, false);
});

test('拒绝保留原因', () => {
  const payload = quotationReplyPayload({
    quotationId: 'QTS-1',
    decision: 'REJECTED',
    repliedAt: '2026-11-24T00:00',
    evidenceIndex: 'SYN-EVIDENCE-01',
    rejectionReason: '价格过高',
    contentDigest: 'digest',
  });
  equal(payload.repliedAt, '2026-11-24T00:00:00Z');
  equal(payload.rejectionReason, '价格过高');
});
