import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import type { InitialRouteTieRecord } from './api';
import {
  decidableTiedCandidatesOf,
  describeInitialRouteTieAnswer,
  initialRouteTieAvailabilityOf,
  initialRouteTieRequestOf,
  tiedAmountText,
} from './initial-route-tie';

function tie(overrides: Partial<InitialRouteTieRecord> = {}): InitialRouteTieRecord {
  return {
    customerAccountId: 'SYN-CUST-01',
    shipmentRequestId: 'SYN-REQ-01',
    acceptanceBaseline: 'SYN-REQ-01/v1',
    declaredParcelId: 'SYN-PARCEL-01',
    servicePurpose: 'NETWORK_SERVICE',
    candidates: [
      { id: 'SYN-CANDIDATE-01', outcome: 'QUALIFIED' },
      { id: 'SYN-CANDIDATE-02', outcome: 'QUALIFIED' },
      { id: 'SYN-CANDIDATE-03', outcome: 'ELIMINATED', reason: 'HARD_CONSTRAINT_RESTRICTION/X' },
    ],
    tied: [
      { candidate: 'SYN-CANDIDATE-01', amountMinor: 4200, currency: 'CNY' },
      { candidate: 'SYN-CANDIDATE-02', amountMinor: 4200, currency: 'CNY' },
    ],
    acceptedAt: '2026-10-26T01:00:00Z',
    settledAt: '2026-10-26T01:05:00Z',
    state: 'PENDING',
    ...overrides,
  };
}

// ADR-0309 决定二「所择不在并列里」：只给并列的那几家，不给并列之外的合格候选。
test('only the tied candidates can be chosen', () => {
  deepEqual(decidableTiedCandidatesOf(tie()), ['SYN-CANDIDATE-01', 'SYN-CANDIDATE-02']);
  match(tiedAmountText(tie()), /SYN-CANDIDATE-01 42\.00 CNY；SYN-CANDIDATE-02 42\.00 CNY/);
});

// ADR-0309 决定六：待裁决才给裁决动作；已裁决与另行形成说清为什么不给。
test('only a pending tie offers a decision', () => {
  deepEqual(initialRouteTieAvailabilityOf(tie()), {
    kind: 'available',
    candidates: ['SYN-CANDIDATE-01', 'SYN-CANDIDATE-02'],
  });
  const decided = initialRouteTieAvailabilityOf(
    tie({
      state: 'DECIDED',
      decision: {
        candidate: 'SYN-CANDIDATE-02',
        planVersion: 'SYN-RPV-64',
        decidedBy: 'OPERATOR/dex#ops',
        basis: 'SYN-APPROVAL-1',
        decidedAt: '2026-10-26T02:00:00Z',
      },
    }),
  );
  equal(decided.kind, 'unavailable');
  match(decided.kind === 'unavailable' ? decided.reason : '', /SYN-CANDIDATE-02/);
  equal(initialRouteTieAvailabilityOf(tie({ state: 'FORMED_OTHERWISE' })).kind, 'unavailable');
  equal(initialRouteTieAvailabilityOf(tie({ tied: [] })).kind, 'unavailable');
});

// ADR-0309 决定二：载荷只带包裹、所择候选与依据；候选不在并列里或依据空白不提交。
test('the request carries the parcel of the row, a tied candidate and a basis', () => {
  const row = tie();
  const ready = initialRouteTieRequestOf(row, { candidate: 'SYN-CANDIDATE-02', basis: '  SYN-APPROVAL-1 ' });
  ok(ready.kind === 'ready');
  deepEqual(ready.request, { declaredParcelId: 'SYN-PARCEL-01', candidate: 'SYN-CANDIDATE-02', basis: 'SYN-APPROVAL-1' });
  equal(initialRouteTieRequestOf(row, { candidate: 'SYN-CANDIDATE-03', basis: 'x' }).kind, 'incomplete');
  equal(initialRouteTieRequestOf(row, { candidate: 'SYN-CANDIDATE-01', basis: '   ' }).kind, 'incomplete');
});

// 成立与已有结果刷新列表；拒的几格带依据，不当作失败。
test('the answers are read by what they say, not as failures', () => {
  const decided = describeInitialRouteTieAnswer({
    outcome: 'INITIAL_ROUTE_DECIDED',
    plan: { version: 'SYN-RPV-64', candidate: 'SYN-CANDIDATE-02', effectiveFrom: '2026-10-26T02:00:00Z', judgedAt: '2026-10-26T02:00:00Z' },
  });
  ok(decided.landed);
  equal(decided.tone, 'answered');
  match(decided.details.join(' '), /SYN-RPV-64/);
  const existing = describeInitialRouteTieAnswer({ outcome: 'INITIAL_ROUTE_ALREADY_FORMED', existing: { conclusion: 'ROUTE_FORMED', planVersion: 'SYN-RPV-9' } });
  ok(existing.landed);
  equal(existing.tone, 'refused');
  const restricted = describeInitialRouteTieAnswer({ outcome: 'CANDIDATE_RESTRICTED', candidateReason: 'HARD_CONSTRAINT_RESTRICTION/X' });
  ok(!restricted.landed);
  match(restricted.details.join(' '), /HARD_CONSTRAINT_RESTRICTION/);
  const notTied = describeInitialRouteTieAnswer({ outcome: 'NO_TIE_REGISTERED', registeredReason: 'CANDIDATE_COSTS_PENDING' });
  match(notTied.details.join(' '), /CANDIDATE_COSTS_PENDING/);
  const undecided = describeInitialRouteTieAnswer({ outcome: 'UNDECIDED', undecidedReason: 'ROUTE_STORE_UNAVAILABLE', continuationReference: 'CONT-1' });
  ok(!undecided.landed);
  match(undecided.details.join(' '), /CONT-1/);
});
