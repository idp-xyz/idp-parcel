import { equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import {
  bookingAnswerEntryEndpoint,
  bookingAnswerEntryHint,
  bookingAnswerEntryOutcomeLabels,
  bookingAnswerEntryProblemNote,
} from './booking-answer-entry';

test('录入订舱答复的提示说清五种答复、原件必填、答复方不用填', () => {
  equal(bookingAnswerEntryEndpoint, '/transport-fulfillment-booking-answer-manual-entries');
  for (const word of ['booking', 'answer', 'ACCEPTED', 'REFUSED', 'EXPIRED', 'WITHDRAWN', 'WAITLISTED', 'original', '答复方不用填']) {
    ok(bookingAnswerEntryHint.includes(word), word);
  }
  ok(bookingAnswerEntryOutcomeLabels.ANSWER_CONFLICT.includes('没有覆盖'));
});

test('坏报文的说明讲的是这一口的字段，未决有中文', () => {
  const note = bookingAnswerEntryProblemNote('MALFORMED_REQUEST');
  for (const word of ['original', 'decidedAt', '+08:00']) ok(note.includes(word), word);
  ok(bookingAnswerEntryOutcomeLabels.COMMISSION_UNDECIDED);
});
