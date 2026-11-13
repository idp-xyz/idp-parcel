import { equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { bookingAnswerEntryEndpoint, bookingAnswerEntryHint, bookingAnswerEntryOutcomeLabels } from './booking-answer-entry';

test('录入订舱答复的提示说清五种答复、原件必填、答复方不用填', () => {
  equal(bookingAnswerEntryEndpoint, '/transport-fulfillment-booking-answer-manual-entries');
  for (const word of ['booking', 'answer', 'ACCEPTED', 'REFUSED', 'EXPIRED', 'WITHDRAWN', 'WAITLISTED', 'original', '答复方不用填']) {
    ok(bookingAnswerEntryHint.includes(word), word);
  }
  ok(bookingAnswerEntryOutcomeLabels.ANSWER_CONFLICT.includes('没有覆盖'));
});
