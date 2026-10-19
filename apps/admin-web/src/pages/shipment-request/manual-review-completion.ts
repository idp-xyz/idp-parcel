import type { AcceptanceReviewQueueEntry, ManualReviewCompletionDraft } from './api';

// 版本取这一行自己带的那一份,不取别处的「当前版」:操作员复核的是打开页面时看到的那一版,
// 新版本形成之后旧页面仍要指名旧版本,服务端才答得出`版本已换代`(ADR-0132 决定二)。
export function manualReviewCompletionDraftOf(
  entry: AcceptanceReviewQueueEntry,
  reason: string,
): ManualReviewCompletionDraft {
  return {
    shipmentRequestId: entry.shipmentRequestId,
    submissionVersionId: entry.submissionVersionId,
    reason,
  };
}
