import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import type { RerouteSuggestionRecord } from './api';
import {
  decidableCandidatesOf,
  describeRerouteDecisionAnswer,
  rerouteDecisionAvailabilityOf,
  rerouteDecisionRequestOf,
} from './reroute-decision';

function suggestion(overrides: Partial<RerouteSuggestionRecord> = {}): RerouteSuggestionRecord {
  return {
    correlationId: 'handover/13:SYN-TENANT-01/13:SYN-PARCEL-01',
    customerAccountId: 'SYN-CUST-01',
    shipmentRequestId: 'SYN-REQ-01',
    acceptanceBaseline: 'SYN-REQ-01/v1',
    declaredParcelId: 'SYN-PARCEL-01',
    servicePurpose: 'NETWORK_SERVICE',
    reviewedPlan: 'SYN-RPV-63',
    blockers: ['NOT_AT_CONTROLLED_NODE'],
    candidates: [
      { id: 'SYN-CANDIDATE-01', outcome: 'ELIMINATED', reason: 'HARD_CONSTRAINT_RESTRICTION/X' },
      { id: 'SYN-CANDIDATE-02', outcome: 'QUALIFIED' },
    ],
    suggestedAt: '2026-10-26T01:00:00Z',
    state: 'PENDING',
    ...overrides,
  };
}

// ADR-0284 决定二「不在建议之列」：建议里被淘汰或证据未知的候选不给选。
test('only the qualified candidates of a suggestion can be chosen', () => {
  deepEqual(decidableCandidatesOf(suggestion()), ['SYN-CANDIDATE-02']);
});

// ADR-0284 决定六：待决定才给决定动作；已决定与已过时说清为什么不给。
test('only a pending suggestion with a qualified candidate offers a decision', () => {
  deepEqual(rerouteDecisionAvailabilityOf(suggestion()), { kind: 'available', candidates: ['SYN-CANDIDATE-02'] });
  const decided = rerouteDecisionAvailabilityOf(
    suggestion({
      state: 'DECIDED',
      decision: {
        candidate: 'SYN-CANDIDATE-02',
        newPlanVersion: 'SYN-RPV-64',
        decidedBy: 'OPERATOR/dex#ops',
        basis: 'SYN-APPROVAL-1',
        decidedAt: '2026-10-26T02:00:00Z',
      },
    }),
  );
  equal(decided.kind, 'unavailable');
  match(decided.kind === 'unavailable' ? decided.reason : '', /SYN-CANDIDATE-02/);
  equal(rerouteDecisionAvailabilityOf(suggestion({ state: 'SUPERSEDED' })).kind, 'unavailable');
  equal(
    rerouteDecisionAvailabilityOf(suggestion({ candidates: [{ id: 'SYN-CANDIDATE-01', outcome: 'ELIMINATED' }] })).kind,
    'unavailable',
  );
});

// ADR-0284 决定一：载荷只带包裹、建议关联、所择候选与依据，包裹与关联取自那一行；候选不在合格之列或依据空白不提交。
test('the request carries the row identity, a qualified candidate and a non-blank basis', () => {
  deepEqual(rerouteDecisionRequestOf(suggestion(), { candidate: 'SYN-CANDIDATE-02', basis: '  SYN-APPROVAL-1 ' }), {
    kind: 'ready',
    request: {
      declaredParcelId: 'SYN-PARCEL-01',
      suggestion: 'handover/13:SYN-TENANT-01/13:SYN-PARCEL-01',
      candidate: 'SYN-CANDIDATE-02',
      basis: 'SYN-APPROVAL-1',
    },
  });
  equal(rerouteDecisionRequestOf(suggestion(), { candidate: 'SYN-CANDIDATE-01', basis: 'x' }).kind, 'incomplete');
  equal(rerouteDecisionRequestOf(suggestion(), { candidate: 'SYN-CANDIDATE-02', basis: '   ' }).kind, 'incomplete');
});

// 成立与已决定都算已有决定在册；拒的几格带依据、未决带原因，都不算落了决定。
test('the answer note tells a landed decision from a refusal', () => {
  const decided = describeRerouteDecisionAnswer({
    outcome: 'REROUTE_DECIDED',
    decision: {
      suggestion: 's',
      declaredParcelId: 'SYN-PARCEL-01',
      candidate: 'SYN-CANDIDATE-02',
      originalPlan: 'SYN-RPV-63',
      newPlanVersion: 'SYN-RPV-64',
      effectiveFromNode: 'SYN-NODE-SHA-HUB',
      effectiveFrom: '2026-10-26T02:00:00Z',
      mode: 'AUTHORIZED_ROLE',
      decidedBy: 'OPERATOR/dex#ops',
      basis: 'SYN-APPROVAL-1',
      decidedAt: '2026-10-26T02:00:00Z',
    },
  });
  equal(decided.landed, true);
  equal(decided.tone, 'answered');
  ok(decided.details.some((line) => line.includes('SYN-RPV-64')));

  const restricted = describeRerouteDecisionAnswer({
    outcome: 'CANDIDATE_RESTRICTED',
    candidateReason: 'HARD_CONSTRAINT_RESTRICTION/X',
  });
  equal(restricted.landed, false);
  equal(restricted.tone, 'refused');
  ok(restricted.details.includes('依据：HARD_CONSTRAINT_RESTRICTION/X'));

  const already = describeRerouteDecisionAnswer({ outcome: 'SUGGESTION_ALREADY_DECIDED' });
  equal(already.landed, true);
  equal(already.tone, 'refused');

  const undecided = describeRerouteDecisionAnswer({
    outcome: 'UNDECIDED',
    undecidedReason: 'ACCEPTED_PARCEL_UNREADABLE',
    continuationReference: 'CONT-1',
  });
  equal(undecided.landed, false);
  ok(undecided.details.some((line) => line.includes('小包托运')));
});
