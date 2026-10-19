import type { AcceptanceReviewQueueEntry, ManualReviewCompletionDraft } from './api';

export function manualReviewCompletionDraftOf(
  entry: AcceptanceReviewQueueEntry,
  reason: string,
): ManualReviewCompletionDraft {
  return { shipmentRequestId: entry.shipmentRequestId, reason };
}
