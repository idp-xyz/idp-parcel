import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  matchesOurCompanySearch,
  ourCompanyLiveText,
  ourCompanyNetworkRows,
  ourCompanyRosterLineOf,
  roleCountNoteOf,
  roleCountsBySide,
} from './our-company-network';
import type { BusinessPartyRecord, GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import { relationshipsOfParty } from './party-overview';

// 本文件钉「我方关联」一册怎么组合：行来自读口按类别筛出的我方公司，在用的段数取那一行上答的；按侧、按角色的段数从合作关系册
// 到手的行里数，只数还在起作用的，待确认另数；关系多的在前。以及主从分栏里那几句话——名录栏一行名称下的那一句、按角色汇总里
// 的小字。

function party(partyId: string, partyName: string, held: [number, number] = [0, 0], faced: [number, number] = [0, 0]): BusinessPartyRecord {
  return {
    tenantId: 'T',
    partyId,
    partyName,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'B',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    categories: ['OUR_COMPANY'],
    relationshipCounts: { asHolder: { live: held[0], candidate: held[1] }, asCounterparty: { live: faced[0], candidate: faced[1] } },
  };
}

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

const relationships = [
  relationship({ relationshipId: 'R1' }),
  relationship({ relationshipId: 'R2', holderId: 'SHIPPER-2', status: 'CANDIDATE' }),
  relationship({ relationshipId: 'R3', holderId: 'AGENT-1', role: 'CARRIER_AGENT' }),
  relationship({ relationshipId: 'R4', holderId: 'SHIPPER-3', status: 'REVOKED' }),
  relationship({ relationshipId: 'R5', holderId: 'OURS-1', counterpartyId: 'CARRIER-1', role: 'ACCOUNT_HOLDER' }),
];

// Covers: 读口筛出的每家我方公司一行；名称与在用的段数取那一行上答的；按侧、按角色的段数从关系册数，每个角色另数其中待确认的；
// 已结束的不数但留在 views 里给图用；已停用的公司主体不列进编号；关系多的在前。
test('ourCompanyNetworkRows 按读口筛出的我方公司组合', () => {
  const rows = ourCompanyNetworkRows(
    [party('OURS-2', '我方二号'), party('OURS-1', '我方一号', [1, 0], [3, 1])],
    [entity({}), entity({ legalEntityId: 'LE-2', partyId: 'OURS-2' }), entity({ legalEntityId: 'LE-3', partyId: 'OURS-1', status: 'DEACTIVATED' })],
    relationships,
  );
  deepEqual(
    rows.map((row) => row.partyId),
    ['OURS-1', 'OURS-2'],
  );
  const first = rows[0];
  equal(first.partyName, '我方一号');
  deepEqual(first.legalEntityIds, ['LE-1']);
  deepEqual(first.towardIt, [
    { role: 'CUSTOMER', count: 2, candidates: 1 },
    { role: 'CARRIER_AGENT', count: 1, candidates: 0 },
  ]);
  deepEqual(first.fromIt, [{ role: 'ACCOUNT_HOLDER', count: 1, candidates: 0 }]);
  equal(first.live, 4);
  equal(first.candidates, 1);
  equal(first.views.length, 5);
  equal(rows[1].live, 0);
  deepEqual(rows[1].legalEntityIds, ['LE-2']);
});

// Covers: 在用的段数不随合作关系册读到多少而变——关系册一行都没到手，段数照行上答的；按角色的那两格这时是空的。
// 那一问没答出交空表；法人册没读到就不列公司主体编号。
test('段数取行上答的，不看关系册读到第几页', () => {
  const [row] = ourCompanyNetworkRows([party('OURS-1', '我方一号', [1, 0], [3, 1])], null, []);
  equal(row.live, 4);
  equal(row.candidates, 1);
  deepEqual(row.towardIt, []);
  deepEqual(row.fromIt, []);
  deepEqual(row.legalEntityIds, []);
  deepEqual(ourCompanyNetworkRows(null, [entity({})], relationships), []);
});

// Covers: 检索按名称、参与方标识、公司主体标识包含匹配。
test('matchesOurCompanySearch 在手上的行里找', () => {
  const [row] = ourCompanyNetworkRows([party('OURS-1', '我方一号')], [entity({})], []);
  equal(matchesOurCompanySearch(row, ''), true);
  equal(matchesOurCompanySearch(row, '一号'), true);
  equal(matchesOurCompanySearch(row, 'ours-1'), true);
  equal(matchesOurCompanySearch(row, 'le-1'), true);
  equal(matchesOurCompanySearch(row, '二号'), false);
});

// Covers: 一家单位名下的关系按它站在哪一侧分开数角色，只数还在起作用的；右栏按这家公司读全的关系也用这一个。
test('roleCountsBySide 按侧数角色', () => {
  const counts = roleCountsBySide(relationshipsOfParty('OURS-1', relationships));
  deepEqual(counts.towardIt, [
    { role: 'CUSTOMER', count: 2, candidates: 1 },
    { role: 'CARRIER_AGENT', count: 1, candidates: 0 },
  ]);
  deepEqual(counts.fromIt, [{ role: 'ACCOUNT_HOLDER', count: 1, candidates: 0 }]);
  deepEqual(roleCountsBySide([]), { towardIt: [], fromIt: [] });
});

// Covers: 名录栏那一句——有在用的报段数与待确认；一段都没有照实说没有（段数是读口答的真数）。
test('名录栏一行名称下的那一句', () => {
  equal(ourCompanyLiveText({ live: 10, candidates: 1 }), '10 段在用 · 1 段待确认');
  equal(ourCompanyLiveText({ live: 1286, candidates: 0 }), '1,286 段在用');
  equal(ourCompanyRosterLineOf({ live: 10, candidates: 1 }), '10 段在用 · 1 段待确认');
  equal(ourCompanyRosterLineOf({ live: 3, candidates: 0 }), '3 段在用');
  equal(ourCompanyRosterLineOf({ live: 0, candidates: 0 }), '没有在用的合作关系');
});

// Covers: 按角色汇总里一行的小字——有待确认的报几段，没有就是都已生效。
test('按角色汇总里的小字', () => {
  equal(roleCountNoteOf({ role: 'CUSTOMER', count: 9, candidates: 2 }), '其中 2 段待确认');
  equal(roleCountNoteOf({ role: 'CUSTOMER', count: 9, candidates: 0 }), '都已生效');
});
