import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type { PartyRelationshipListResponseBody, PartyRelationshipRecord } from './api';
import { mergePartyRelationshipAnswers, partyRelationshipsComplete } from './party-relationships-of';

// 本文件钉「一家单位名下的合作关系」两问怎么并：都答成才并，哪一问没答成就交哪一问；没读全看得出来。

function relationship(relationshipId: string): PartyRelationshipRecord {
  return {
    tenantId: 'T',
    relationshipId,
    revision: 1,
    holderId: 'H',
    holderNameKnown: true,
    counterpartyId: 'C',
    counterpartyNameKnown: true,
    role: 'CUSTOMER',
    scope: 'S',
    basis: 'B',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
  };
}

function listed(ids: string[], next: string | null = null, total: number | null = ids.length): ApiResult<PartyRelationshipListResponseBody> {
  return {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'PARTY_RELATIONSHIPS_LISTED', relationships: ids.map(relationship), page: { size: 200, next, total } },
  };
}

const idsOf = (answer: ApiResult<PartyRelationshipListResponseBody>) =>
  answer.kind === 'outcome' ? answer.body.relationships.map((row) => row.relationshipId) : null;

// Covers: 两问都答成——行并在一起，持有方那一问的在前；总数相加；都没有下一页就是读全了。
test('两问都答成时并成一份', () => {
  const merged = mergePartyRelationshipAnswers(listed(['R1', 'R2']), listed(['R3']));
  deepEqual(idsOf(merged), ['R1', 'R2', 'R3']);
  equal(merged.kind === 'outcome' ? merged.body.page.total : null, 3);
  equal(partyRelationshipsComplete(merged), true);
});

// Covers: 同一个编号两问里都有只留一行。
test('按关系编号去重', () => {
  deepEqual(idsOf(mergePartyRelationshipAnswers(listed(['R1', 'R2']), listed(['R2', 'R3']))), ['R1', 'R2', 'R3']);
});

// Covers: 哪一问还有下一页，并出来的就不算读全；有一问不给总数，总数也不给。
test('有一问没读全，并出来的就不算全', () => {
  equal(partyRelationshipsComplete(mergePartyRelationshipAnswers(listed(['R1'], 'cursor-1', 500), listed(['R2']))), false);
  equal(partyRelationshipsComplete(mergePartyRelationshipAnswers(listed(['R1']), listed(['R2'], 'cursor-2', 500))), false);
  const withoutTotal = mergePartyRelationshipAnswers(listed(['R1'], null, null), listed(['R2']));
  equal(withoutTotal.kind === 'outcome' ? withoutTotal.body.page.total : 0, null);
});

// Covers: 哪一问没答成就交哪一问的答复（先看持有方那一问），不拿另一问的半份冒充全部；没答成、还没答出都不算读全。
test('有一问没答成，交出那一问的答复', () => {
  const failed: ApiResult<PartyRelationshipListResponseBody> = { kind: 'noAnswer', status: 502, code: 'NO_ANSWER_FORMED' };
  const transport: ApiResult<PartyRelationshipListResponseBody> = { kind: 'transport', message: 'offline' };
  equal(mergePartyRelationshipAnswers(failed, listed(['R1'])), failed);
  equal(mergePartyRelationshipAnswers(listed(['R1']), transport), transport);
  equal(mergePartyRelationshipAnswers(failed, transport), failed);
  equal(partyRelationshipsComplete(failed), false);
  equal(partyRelationshipsComplete(null), false);
});
