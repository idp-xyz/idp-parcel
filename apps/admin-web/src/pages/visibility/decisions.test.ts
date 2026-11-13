import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { exceptionDecisionEndpoints, exceptionDecisionHints, exceptionDecisionTargets, withUndecidedReason } from './decisions';

test('异常的十个运营决定口各有路径与填写提示，选册键不重复', () => {
  equal(exceptionDecisionTargets.length, 10);
  equal(new Set(exceptionDecisionTargets.map((target) => target.id)).size, 10);
  for (const kind of Object.keys(exceptionDecisionEndpoints) as (keyof typeof exceptionDecisionEndpoints)[]) {
    ok(exceptionDecisionEndpoints[kind].startsWith('/'), kind);
    ok(exceptionDecisionHints[kind].includes('请填'), kind);
  }
});

test('披露与通知三口都要求指名哪一期信号', () => {
  for (const kind of [
    'customer-disclosure-decision',
    'customer-disclosure-authorization',
    'customer-exception-notification',
    'customer-exception-notification-manual-record',
  ] as const) {
    ok(exceptionDecisionHints[kind].includes('episodeId'), kind);
  }
});

test('未决原因从 reason 挪到登记面板认的那一格，已有的不覆盖，别的答复原样', () => {
  const undecided = withUndecidedReason({ kind: 'outcome', status: 200, body: { outcome: 'UNDECIDED', reason: 'NOTIFICATION_POLICY_NOT_CONFIGURED' } } as never);
  deepEqual(undecided.kind === 'outcome' ? undecided.body.undecidedReason : null, 'NOTIFICATION_POLICY_NOT_CONFIGURED');
  const formed = { kind: 'outcome', status: 201, body: { outcome: 'TAKEN_UP' } } as never;
  equal(withUndecidedReason(formed), formed);
});

test('手工标记通知的提示要求通知方式与原件，并说明系统不发送', () => {
  const hint = exceptionDecisionHints['customer-exception-notification-manual-record'];
  for (const word of ['method', 'original', '不发送']) ok(hint.includes(word), word);
});

test('「已提交」在推送口与手工标记口各说各的', () => {
  const labelOf = (id: string) => exceptionDecisionTargets.find((target) => target.id === id)?.outcomeLabels.SUBMITTED;
  equal(labelOf('customer-exception-notification'), '已提交给通知渠道');
  equal(labelOf('customer-exception-notification-manual-record'), '已记为已通知客户');
});
