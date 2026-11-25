import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { claimDeadline, claimPrefill, claimWindowElapsed, unclaimedNextStep, unclaimedTrack } from './unclaimed-next-step';

const now = new Date('2026-11-25T08:00:00Z');
const day = 86_400;

test('认领期限从到货发生的时刻起按整秒算，恰好到点就算走完', () => {
  equal(claimWindowElapsed('2026-10-26T08:00:00Z', 30 * day, now), true);
  equal(claimWindowElapsed('2026-10-26T08:00:01Z', 30 * day, now), false);
  equal(claimWindowElapsed('2026-10-25T08:00:00Z', 30 * day, now), true);
});

test('时刻认不出、发生在现在之后、期限不是正数或大到后端也不认，都答没走完', () => {
  equal(claimWindowElapsed('不是时刻', 30 * day, now), false);
  equal(claimWindowElapsed('2026-11-25T08:00:05Z', 1, now), false);
  equal(claimWindowElapsed('2026-01-01T00:00:00Z', 0, now), false);
  equal(claimWindowElapsed('2026-01-01T00:00:00Z', -5, now), false);
  equal(claimWindowElapsed('2026-01-01T00:00:00Z', 9_223_372_037, now), false);
});

test('认领期限到哪一刻为止', () => {
  equal(claimDeadline('2026-10-26T08:00:00Z', 30 * day)?.toISOString(), '2026-11-25T08:00:00.000Z');
  equal(claimDeadline('不是时刻', 30 * day), null);
  equal(claimDeadline('2026-10-26T08:00:00Z', 0), null);
});

const unclaimed = { status: 'UNCLAIMED', occurredAt: '2026-11-20T08:00:00Z' };
const rule = { claimWindowSeconds: 30 * day };

test('待认领的下一步是认领：没登记处置规则的仓不能把处置当下一步', () => {
  deepEqual(unclaimedNextStep(unclaimed, undefined, now), { kind: 'claim', label: '认领' });
  deepEqual(unclaimedNextStep({ ...unclaimed, occurredAt: '2026-01-01T00:00:00Z' }, undefined, now), { kind: 'claim', label: '认领' });
});

test('认领期限没走完仍是认领，走完仍没人认才改成处置', () => {
  deepEqual(unclaimedNextStep(unclaimed, rule, now), { kind: 'claim', label: '认领' });
  deepEqual(unclaimedNextStep({ ...unclaimed, occurredAt: '2026-10-01T00:00:00Z' }, rule, now), { kind: 'dispose', label: '处置' });
});

test('已认领的货还在暂存，下一步去作业台入账；货已入账的认领被用掉了，没有下一步', () => {
  deepEqual(unclaimedNextStep({ status: 'CLAIMED', occurredAt: unclaimed.occurredAt }, rule, now), {
    kind: 'go',
    label: '去作业台入账',
    moduleId: 'warehouse-workstation',
  });
  equal(unclaimedNextStep({ status: 'CLAIMED', occurredAt: unclaimed.occurredAt, postingReferences: ['SYN-FACT-1'] }, rule, now), null);
});

test('已处置与陌生状态没有下一步', () => {
  equal(unclaimedNextStep({ status: 'DISPOSED', occurredAt: unclaimed.occurredAt }, rule, now), null);
  equal(unclaimedNextStep({ status: 'ARCHIVED', occurredAt: unclaimed.occurredAt }, rule, now), null);
});

test('进度条：待认领与认领后待入账用待办色，入账走到末一步，处置是中性的结束', () => {
  const waiting = unclaimedTrack({ status: 'UNCLAIMED' });
  equal(waiting.currentId, 'UNCLAIMED');
  equal(waiting.currentLabel, '待认领');

  const claimed = unclaimedTrack({ status: 'CLAIMED' });
  equal(claimed.currentId, 'CLAIMED');
  equal(claimed.currentLabel, '已认领，待入账');

  const received = unclaimedTrack({ status: 'CLAIMED', postingReferences: ['SYN-FACT-1'] });
  equal(received.currentId, 'POSTED');
  equal(received.currentLabel, undefined);

  const disposed = unclaimedTrack({ status: 'DISPOSED' });
  deepEqual(disposed.halt, { label: '已处置', tone: 'neutral' });
  equal(disposed.currentLabel, undefined);
});

test('认领表单带出到货号与现场留的线索，没有线索的格是空串', () => {
  deepEqual(claimPrefill({ arrivalId: 'UCA-1', ownerClue: 'OWNER-1', preAdviceClue: 'IPA-1' }), {
    arrivalId: 'UCA-1',
    ownerAccount: 'OWNER-1',
    preAdviceId: 'IPA-1',
  });
  deepEqual(claimPrefill({ arrivalId: 'UCA-2' }), { arrivalId: 'UCA-2', ownerAccount: '', preAdviceId: '' });
});
