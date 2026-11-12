import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  matchesOurCompanySearch,
  ourCompanyLiveText,
  ourCompanyNetworkRows,
  ourCompanyRelationshipsReadOf,
  ourCompanyRosterLineOf,
  roleCountNoteOf,
} from './our-company-network';
import type { GroupLegalEntityRecord, PartyRelationshipRecord } from './api';

// 本文件钉「我方关联」一册怎么数：只列在用公司主体钉着的我方公司，按侧、按角色数还在起作用的段，待确认另数，关系多的在前；
// 以及主从分栏里那几句话——名录栏一行名称下的那一句、按角色汇总里的小字——在合作关系册读到不同程度时各说什么。

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
  // 每个角色另数其中待确认的段：R2 是待确认的客户关系。
  deepEqual(first.towardIt, [
    { role: 'CUSTOMER', count: 2, candidates: 1 },
    { role: 'CARRIER_AGENT', count: 1, candidates: 0 },
  ]);
  deepEqual(first.fromIt, [{ role: 'ACCOUNT_HOLDER', count: 1, candidates: 0 }]);
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

// Covers: 合作关系册读到什么程度由那一问的答复判——还没答出、没答成、只有第一页、整册。
test('ourCompanyRelationshipsReadOf 分四种', () => {
  equal(ourCompanyRelationshipsReadOf(null, false), 'pending');
  equal(ourCompanyRelationshipsReadOf('unconfigured', false), 'unread');
  equal(ourCompanyRelationshipsReadOf('transport', false), 'unread');
  equal(ourCompanyRelationshipsReadOf('outcome', false), 'first-page');
  equal(ourCompanyRelationshipsReadOf('outcome', true), 'complete');
  // 没答成时「整册」无从说起：传进来是真也不当读全了。
  equal(ourCompanyRelationshipsReadOf('noAnswer', true), 'unread');
});

// Covers: 名录栏那一句——有在用的报段数与待确认；一段都没有时，整册读到了才说没有，只读到第一页说是第一页里没有；
// 还在读、没读到各说各的，不拿「没有」顶。
test('名录栏一行名称下的那一句', () => {
  equal(ourCompanyLiveText({ live: 10, candidates: 1 }), '10 段在用 · 1 段待确认');
  equal(ourCompanyLiveText({ live: 1286, candidates: 0 }), '1,286 段在用');
  equal(ourCompanyRosterLineOf({ live: 10, candidates: 1 }, 'complete'), '10 段在用 · 1 段待确认');
  equal(ourCompanyRosterLineOf({ live: 3, candidates: 0 }, 'first-page'), '3 段在用');
  equal(ourCompanyRosterLineOf({ live: 0, candidates: 0 }, 'complete'), '没有在用的合作关系');
  equal(ourCompanyRosterLineOf({ live: 0, candidates: 0 }, 'first-page'), '第一页里没有在用的合作关系');
  equal(ourCompanyRosterLineOf({ live: 0, candidates: 0 }, 'pending'), '正在读取…');
  equal(ourCompanyRosterLineOf({ live: 0, candidates: 0 }, 'unread'), '合作关系没读到');
  // 没读到时手上的数是零，不照它说话。
  equal(ourCompanyRosterLineOf({ live: 4, candidates: 2 }, 'unread'), '合作关系没读到');
});

// Covers: 按角色汇总里一行的小字——有待确认的报几段，没有就是都已生效。
test('按角色汇总里的小字', () => {
  equal(roleCountNoteOf({ role: 'CUSTOMER', count: 9, candidates: 2 }), '其中 2 段待确认');
  equal(roleCountNoteOf({ role: 'CUSTOMER', count: 9, candidates: 0 }), '都已生效');
});
