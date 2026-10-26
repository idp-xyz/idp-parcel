import { test } from 'node:test';
import { equal, deepEqual } from 'node:assert/strict';
import { formatInstant } from '../catalogue-view';
import {
  answerLineOf,
  bookingRowOf,
  bookingStandingLabels,
  bounceBasisLabels,
  bounceRowOf,
  transportRecordsPathOf,
} from './booking-registers';
import type { BookingAnswerBounceRecord, BookingRequestRecord } from './records-api';

function booking(overrides: Partial<BookingRequestRecord> = {}): BookingRequestRecord {
  return {
    booking: 'SYN-BOOKING-1',
    commission: 'SYN-COMMISSION-1',
    schedule: 'SYN-SCHEDULE-1',
    quantity: 10,
    unit: 'KG',
    requestedAt: '2026-10-20T08:00:00Z',
    recordedAt: '2026-10-20T08:00:01Z',
    standing: 'PENDING_CONFIRMATION',
    answers: [],
    ...overrides,
  };
}

test('状态词七格各有中文，且不在集合内的词原样示文', () => {
  deepEqual(
    Object.keys(bookingStandingLabels),
    ['PENDING_CONFIRMATION', 'WAITLISTED', 'CONFIRMED', 'PARTIALLY_CONFIRMED', 'REFUSED', 'EXPIRED', 'WITHDRAWN'],
  );
  for (const [word, label] of Object.entries(bookingStandingLabels)) {
    equal(bookingRowOf(booking({ standing: word })).values.standing, label, word);
  }
  equal(bookingRowOf(booking({ standing: 'SOMETHING_NEW' })).values.standing, 'SOMETHING_NEW');
});

test('没有答复：当前答复三格空，名下答复 0 份', () => {
  const row = bookingRowOf(booking());
  equal(row.values.currentAnswer, '');
  equal(row.values.acceptedQuantity, '');
  equal(row.values.currentAnsweredAt, '');
  equal(row.values.answerCount, '0');
  equal(row.values.requested, '10 KG');
});

test('当前答复示答复值、接受量、答复时刻；名下全部答复含候补与后继', () => {
  const waitlist = { booking: 'SYN-BOOKING-1', answer: 'A1', outcome: 'WAITLISTED', decidedAt: '2026-10-21T01:00:00Z', recordedAt: '2026-10-21T01:00:01Z' };
  const accepted = { booking: 'SYN-BOOKING-1', answer: 'A2', outcome: 'ACCEPTED', quantity: 6, decidedAt: '2026-10-22T01:00:00Z', recordedAt: '2026-10-22T01:00:01Z' };
  const row = bookingRowOf(booking({ standing: 'PARTIALLY_CONFIRMED', currentAnswer: accepted, answers: [waitlist, accepted] }));
  equal(row.values.standing, '部分确认');
  equal(row.values.currentAnswer, '接受');
  equal(row.values.acceptedQuantity, '6');
  equal(row.values.currentAnsweredAt, formatInstant(accepted.decidedAt));
  equal(row.values.answerCount, '2');
  equal(row.values.answers.split('\n').length, 2);
  equal(answerLineOf(waitlist).startsWith('候补 · 标识 A1'), true);
});

test('取消时刻单列，不并进状态词；提交人、取消人有就示、缺席为空', () => {
  const cancelled = bookingRowOf(
    booking({ cancelledAt: '2026-10-23T02:00:00Z', submittedBy: 'OPERATOR/a#1', cancelledBy: 'OPERATOR/b#2' }),
  );
  equal(cancelled.values.standing, '待确认');
  equal(cancelled.values.cancelledAt, formatInstant('2026-10-23T02:00:00Z'));
  equal(cancelled.values.submittedBy, 'OPERATOR/a#1');
  equal(cancelled.values.cancelledBy, 'OPERATOR/b#2');
  const plain = bookingRowOf(booking());
  equal(plain.values.cancelledAt, '');
  equal(plain.values.submittedBy, '');
  equal(plain.values.cancelledBy, '');
});

test('挡回两种依据照 CONTEXT 词条示文', () => {
  const record = (basis: string): BookingAnswerBounceRecord => ({
    source: 'SYN-CARRIER-1', booking: 'SYN-BOOKING-1', answer: 'A1', basis,
    firstBouncedAt: '2026-10-24T01:00:00Z', lastBouncedAt: '2026-10-25T01:00:00Z', count: '3',
  });
  equal(bounceBasisLabels.BOOKING_NOT_REGISTERED, '订舱不在册');
  equal(bounceBasisLabels.SOURCE_NOT_SERVICE_PROVIDER, '不是服务提供方');
  equal(bounceRowOf(record('BOOKING_NOT_REGISTERED')).values.basis, '订舱不在册');
  const row = bounceRowOf(record('SOURCE_NOT_SERVICE_PROVIDER'));
  equal(row.values.basis, '不是服务提供方');
  equal(row.values.count, '3');
  equal(row.values.firstBouncedAt, formatInstant('2026-10-24T01:00:00Z'));
  equal(row.values.lastBouncedAt, formatInstant('2026-10-25T01:00:00Z'));
  equal(bounceRowOf(record('NEW_BASIS')).values.basis, 'NEW_BASIS');
});

test('筛选参数拼装：缺席与空白不发、两端空白去掉、值转义、只挡回册带', () => {
  const bounce = 'booking-answer-bounce';
  equal(transportRecordsPathOf(bounce), '/transport-fulfillment-records?registry=booking-answer-bounce');
  equal(transportRecordsPathOf(bounce, { booking: '  ', source: '' }), '/transport-fulfillment-records?registry=booking-answer-bounce');
  equal(
    transportRecordsPathOf(bounce, { booking: ' B 1 ', source: 'OPERATOR/x#1' }),
    '/transport-fulfillment-records?registry=booking-answer-bounce&booking=B%201&source=OPERATOR%2Fx%231',
  );
  equal(transportRecordsPathOf(bounce, { source: 'S' }), '/transport-fulfillment-records?registry=booking-answer-bounce&source=S');
  equal(transportRecordsPathOf('booking-request', { booking: 'B' }), '/transport-fulfillment-records?registry=booking-request');
});
