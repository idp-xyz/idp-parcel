import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ReviewFinancialControlRecord } from './api';
import { controlItemText, financialControlConclusionOf, restrictionReasonText } from './financial-control';

// 本文件钉接受前财务控制结论的屏上写法（ADR-0379 决定三、四）：四种结论各自的说法、受限项逐条带中文原因、认不得的原因照原码，
// 以及复核页那一行挪进共享模块后输出不变。

const control = (outcome: string, items: ReviewFinancialControlRecord['items'] = []): ReviewFinancialControlRecord => ({
  outcome,
  resultId: 'FCR-1',
  asOfAt: '2026-11-11T02:00:00Z',
  items,
});

test('结算的三种限制原因译成中文，认不得的照原码', () => {
  equal(restrictionReasonText('AVAILABLE_BALANCE_INSUFFICIENT'), '可用余额不足');
  equal(restrictionReasonText('AVAILABLE_CREDIT_INSUFFICIENT'), '可用额度不足');
  equal(restrictionReasonText('ACCOUNT_OVERDUE'), '账户逾期');
  equal(restrictionReasonText('SOMETHING_NEW'), 'SOMETHING_NEW');
});

test('复核页的控制项一行不带受限原因，挪进共享模块后输出照旧', () => {
  equal(controlItemText({ kind: 'PREPAID_FREEZE', order: 1, conclusion: 'SATISFIED' }), '预付冻结:已通过');
  equal(
    controlItemText({
      kind: 'CREDIT_CHECK',
      order: 2,
      conclusion: 'RESTRICTED',
      basis: 'ACCOUNT_OVERDUE',
      failureDisposition: 'AUTHORIZED_DISPOSITION',
      responsibility: 'SYN-PARTY-01',
    }),
    '信用检查:受限 · 不通过时进入授权处置 · 责任方 SYN-PARTY-01',
  );
});

test('冻结资金通过：已冻结资金，逐项一行', () => {
  deepEqual(financialControlConclusionOf(control('HELD', [{ kind: 'PREPAID_FREEZE', order: 1, conclusion: 'SATISFIED' }])), {
    verdict: '通过',
    detail: '已冻结资金',
    tone: 'good',
    items: [{ key: 'PREPAID_FREEZE:1', text: '预付冻结:已通过' }],
  });
});

test('信用占用通过：已占用信用', () => {
  const conclusion = financialControlConclusionOf(control('CREDIT_EXPOSED', [{ kind: 'CREDIT_CHECK', order: 1, conclusion: 'SATISFIED' }]));
  equal(conclusion?.verdict, '通过');
  equal(conclusion?.detail, '已占用信用');
  equal(conclusion?.tone, 'good');
});

test('受限：受限项逐条带中文原因与采用的失败处置，通过的项照列', () => {
  const conclusion = financialControlConclusionOf(
    control('RESTRICTED', [
      { kind: 'PREPAID_FREEZE', order: 1, conclusion: 'RESTRICTED', basis: 'AVAILABLE_BALANCE_INSUFFICIENT', failureDisposition: 'REJECT' },
      { kind: 'CREDIT_CHECK', order: 2, conclusion: 'RESTRICTED', basis: 'AVAILABLE_CREDIT_INSUFFICIENT' },
      { kind: 'CREDIT_CHECK', order: 3, conclusion: 'RESTRICTED', basis: 'NEW_REASON' },
    ]),
  );
  equal(conclusion?.verdict, '受限');
  equal(conclusion?.tone, 'warn');
  deepEqual(
    conclusion?.items.map((item) => item.text),
    ['预付冻结:受限 · 可用余额不足 · 不通过时按策略拒绝', '信用检查:受限 · 可用额度不足', '信用检查:受限 · NEW_REASON'],
  );
});

test('不适用：合同约定无财务控制，没有项', () => {
  deepEqual(financialControlConclusionOf(control('NOT_APPLICABLE')), {
    verdict: '不适用',
    detail: '合同约定无财务控制',
    tone: 'muted',
    items: [],
  });
});

test('没判过答 null；认不得的结论照原码', () => {
  equal(financialControlConclusionOf(undefined), null);
  const unknown = financialControlConclusionOf(control('SOMETHING_ELSE'));
  equal(unknown?.verdict, 'SOMETHING_ELSE');
  equal(unknown?.tone, 'muted');
});
