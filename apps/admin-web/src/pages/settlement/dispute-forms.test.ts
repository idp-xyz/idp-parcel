import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  amountProblem,
  chargesOnStatement,
  openedAtProblem,
  localInputToInstant,
  newDisputeId,
  openDisputesOf,
  registrationAnswerText,
  resolutionAnswerText,
  textProblem,
} from './dispute-forms';
test('一张单上的费用取单自己的行；单不在册就没有可选的', () => {
  deepEqual(
    chargesOnStatement({ outcome: 'STATEMENT_LINES_LISTED', currency: 'CNY', lines: [{ charge: 'A', amount: '9600' }] }),
    [{ charge: 'A', amount: '9600', currency: 'CNY' }],
  );
  deepEqual(chargesOnStatement({ outcome: 'STATEMENT_NOT_FOUND' }), []);
});

test('提出时间：不早于发出，不晚于作废；作废之前提出的照收', () => {
  const published = '2026-11-01T08:00:00Z';
  const voided = '2026-11-01T10:00:00Z';
  equal(openedAtProblem('2026-11-01T07:00:00Z', published, voided) !== null, true);
  equal(openedAtProblem('2026-11-01T09:00:00Z', published, voided), null);
  equal(openedAtProblem('2026-11-01T10:00:00Z', published, voided), null);
  equal(openedAtProblem('2026-11-01T11:00:00Z', published, voided), '这张单已作废，作废之后提出的异议不再登记在这张单上');
  equal(openedAtProblem('2026-11-01T11:00:00Z', published, undefined), null);
});

test('还在等人处理的争议：未处理与待复核；终局三种不列', () => {
  const statement = {
    disputes: [
      { dispute: 'D1', charge: 'A', disputedAmount: '1', reason: 'r', openedAt: 't' },
      { dispute: 'D2', charge: 'A', disputedAmount: '1', reason: 'r', openedAt: 't', resolution: 'PENDING_REVIEW' },
      { dispute: 'D3', charge: 'A', disputedAmount: '1', reason: 'r', openedAt: 't', resolution: 'REJECTED' },
    ],
  };
  deepEqual(openDisputesOf(statement).map((dispute) => dispute.dispute), ['D1', 'D2']);
});

test('争议编号由页面定下，带时间与一段随机串，只用字母数字', () => {
  equal(newDisputeId(new Date('2026-11-01T09:05:07Z'), 'a1-b2c3d4e5'), 'DSP-20261101-090507-A1B2C3');
});

test('金额：正整数、与单上同一写法、不超过这笔费用', () => {
  equal(amountProblem('', '9600'), '填上客户不认的金额');
  equal(amountProblem('16.00', '9600') !== null, true);
  equal(amountProblem('0', '9600') !== null, true);
  equal(amountProblem('9601', '9600'), '不能超过这笔费用的金额 9600');
  equal(amountProblem('1600', '9600'), null);
});

test('文字格：不空、至多 1000 字', () => {
  equal(textProblem(' ', '理由'), '写一句理由');
  equal(textProblem('长'.repeat(1001), '理由') !== null, true);
  equal(textProblem('客户说重量不对', '理由'), null);
});

test('本地时间到分换成 UTC 到秒；写坏的不猜', () => {
  equal(localInputToInstant('昨天'), null);
  equal(localInputToInstant('2026-11-01T09:05')?.endsWith(':00Z'), true);
});

test('答复译成一句人话，负向答案不说成功', () => {
  equal(registrationAnswerText({ outcome: 'DISPUTE_OPENED', dispute: 'D1' }).startsWith('已登记'), true);
  equal(registrationAnswerText({ outcome: 'STATEMENT_VOIDED_TAKES_NO_DISPUTE' }).includes('替代单'), false);
  equal(registrationAnswerText({ outcome: 'STATEMENT_VOIDED_TAKES_NO_DISPUTE' }).includes('作废之后提出'), true);
  equal(registrationAnswerText({ outcome: 'SOURCE_NOT_ACCEPTED' }).startsWith('没有登记'), true);
  equal(resolutionAnswerText({ outcome: 'DISPUTE_RESOLVED', resolution: 'REJECTED' }), '已处理：拒绝。');
  equal(resolutionAnswerText({ outcome: 'DISPUTE_ALREADY_RESOLVED', resolution: 'ACCEPTED' }), '这一项已经处理完了（接受），不能再改。');
});
