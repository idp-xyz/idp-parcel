import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { manualReviewCompletionDraftOf } from './manual-review-completion';
import type { AcceptanceReviewQueueEntry } from './api';

// 本文件钉复核完成草案的形状(票 operator-channel/34)。期望值取服务端 Intake 收的线格式原词
// (OperatorDecisionIntake 的复核完成载荷),不从实现推。

function entryOf(overrides: Partial<AcceptanceReviewQueueEntry> = {}): AcceptanceReviewQueueEntry {
  return {
    shipmentRequestId: 'SYN-REQ-01',
    customerAccountId: 'SYN-ACCOUNT-01',
    source: 'SYN-SOURCE',
    sourceRequestKey: 'SYN-KEY-01',
    state: 'SUBMITTED',
    submissionVersionId: 'SYN-VERSION-01',
    declaredParcelCount: 1,
    submittedAt: '2026-10-18T08:00:00Z',
    reviewCompleted: false,
    ...overrides,
  };
}

// Covers: 票 operator-channel/34——复核签给操作员看过的那一份提交版本:草案带上队列行上的 submissionVersionId,
// 新版本形成之后按旧页面提交,服务端才答得出`版本已换代`(ADR-0132 决定二),而不是把旧版本的复核签到新版本头上。
// 整形相等同时钉住草案里没有复核人、授权依据或证据这类「谁在签」的格。
test('复核完成草案带上队列行的提交版本,只有委托、版本与理由三格', () => {
  deepEqual(manualReviewCompletionDraftOf(entryOf(), 'SYN-REVIEW-REASON-1'), {
    shipmentRequestId: 'SYN-REQ-01',
    submissionVersionId: 'SYN-VERSION-01',
    reason: 'SYN-REVIEW-REASON-1',
  });
});

// Covers: 同一份委托,开着旧页面的那一行仍是旧版本——草案取的是这一行自己的版本,不是别处的「当前版」。
test('旧页面那一行的复核草案指名那一行的版本', () => {
  const stale = manualReviewCompletionDraftOf(entryOf({ submissionVersionId: 'SYN-VERSION-01' }), 'r');
  const reloaded = manualReviewCompletionDraftOf(entryOf({ submissionVersionId: 'SYN-VERSION-02' }), 'r');
  deepEqual([stale, reloaded].map((draft) => (draft as { submissionVersionId?: string }).submissionVersionId), [
    'SYN-VERSION-01',
    'SYN-VERSION-02',
  ]);
});
