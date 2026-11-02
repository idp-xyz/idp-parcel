import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  amountProblem,
  chargesOnStatement,
  localInputToInstant,
  newDisputeId,
  openDisputesOf,
  registrationAnswerText,
  resolutionAnswerText,
  textProblem,
} from './dispute-forms';
import type { CustomerChargeRecord } from './api';

const charge = (id: string, billedOn?: string): CustomerChargeRecord => ({
  charge: id,
  feeItem: 'SYN-FEE',
  evaluation: 'E',
  stage: 'CONFIRMED',
  originalCurrency: 'CNY',
  originalAmount: '9600',
  settlementCurrency: 'CNY',
  settlementAmount: '9600',
  formedAt: '2026-11-01T00:00:00Z',
  confirmationBases: [],
  billedOn,
});

test('一张单上的费用按读口的所在对账单认，没出账的与别的单上的不列', () => {
  deepEqual(
    chargesOnStatement([charge('A', 'S1'), charge('B', 'S2'), charge('C')], 'S1').map((option) => option.charge),
    ['A'],
  );
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
  equal(registrationAnswerText({ outcome: 'STATEMENT_VOIDED_TAKES_NO_DISPUTE' }).includes('作废'), true);
  equal(registrationAnswerText({ outcome: 'SOURCE_NOT_ACCEPTED' }).startsWith('没有登记'), true);
  equal(resolutionAnswerText({ outcome: 'DISPUTE_RESOLVED', resolution: 'REJECTED' }), '已处理：拒绝。');
  equal(resolutionAnswerText({ outcome: 'DISPUTE_ALREADY_RESOLVED', resolution: 'ACCEPTED' }), '这一项已经处理完了（接受），不能再改。');
});
