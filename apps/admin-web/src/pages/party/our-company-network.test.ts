import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { matchesOurCompanySearch, ourCompanyNetworkRows } from './our-company-network';
import type { GroupLegalEntityRecord, PartyRelationshipRecord } from './api';

// 本文件钉「我方关联」一册怎么数：只列在用公司主体钉着的我方公司，按侧、按角色数还在起作用的段，待确认另数，关系多的在前。

function entity(over: Partial<GroupLegalEntityRecord>): GroupLegalEntityRecord {
  return {
    tenantId: 'T',
    legalEntityId: 'LE-1',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'OURS-1',
    partyName: '我方一号',
    partyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'B',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    identityLayerRegistered: false,
    ...over,
  } as GroupLegalEntityRecord;
}

function relationship(over: Partial<PartyRelationshipRecord>): PartyRelationshipRecord {
  return {
    tenantId: 'T',
    relationshipId: 'R1',
    revision: 1,
    holderId: 'SHIPPER-1',
    holderNameKnown: true,
    holderName: '货主',
    counterpartyId: 'OURS-1',
    counterpartyNameKnown: true,
    counterpartyName: '我方一号',
    role: 'CUSTOMER',
    scope: 'S',
    basis: 'B',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 每家在用公司主体钉着的我方公司一行；按侧、按角色数在用的段；待确认另数；已结束不数但留在 views 里给图用；已停用公司主体不列。
test('ourCompanyNetworkRows 按我方公司数关系', () => {
  const rows = ourCompanyNetworkRows(
    [entity({}), entity({ legalEntityId: 'LE-2', partyId: 'OURS-2', partyName: '我方二号' }), entity({ legalEntityId: 'LE-3', partyId: 'OURS-3', status: 'DEACTIVATED' })],
    [
      relationship({ relationshipId: 'R1' }),
      relationship({ relationshipId: 'R2', holderId: 'SHIPPER-2', status: 'CANDIDATE' }),
      relationship({ relationshipId: 'R3', holderId: 'AGENT-1', role: 'CARRIER_AGENT' }),
      relationship({ relationshipId: 'R4', holderId: 'SHIPPER-3', status: 'REVOKED' }),
      relationship({ relationshipId: 'R5', holderId: 'OURS-1', counterpartyId: 'CARRIER-1', role: 'ACCOUNT_HOLDER' }),
    ],
  );
  deepEqual(
    rows.map((row) => row.partyId),
    ['OURS-1', 'OURS-2'],
  );
  const first = rows[0];
  deepEqual(first.legalEntityIds, ['LE-1']);
  deepEqual(first.towardIt, [
    { role: 'CUSTOMER', count: 2 },
    { role: 'CARRIER_AGENT', count: 1 },
  ]);
  deepEqual(first.fromIt, [{ role: 'ACCOUNT_HOLDER', count: 1 }]);
  equal(first.live, 4);
  equal(first.candidates, 1);
  equal(first.views.length, 5);
  equal(rows[1].live, 0);
});

// Covers: 法人册没答出交空表；检索按名称、参与方标识、公司主体标识包含匹配。
test('ourCompanyNetworkRows 法人册没答出与检索', () => {
  deepEqual(ourCompanyNetworkRows(null, []), []);
  const [row] = ourCompanyNetworkRows([entity({})], []);
  equal(matchesOurCompanySearch(row, ''), true);
  equal(matchesOurCompanySearch(row, '一号'), true);
  equal(matchesOurCompanySearch(row, 'le-1'), true);
  equal(matchesOurCompanySearch(row, '二号'), false);
});
