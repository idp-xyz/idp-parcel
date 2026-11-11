import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  relationshipGraphColumnLimit,
  relationshipGraphDraftOf,
  relationshipGraphOf,
  relationshipGraphRoleLimit,
  relationshipGraphWithDraft,
} from './party-relationship-graph';
import type { PartyRelationshipView } from './party-overview';

// 本文件钉关系图的摆法：左右按本方站哪一侧分、结束的关系不上图只计数、同角色多了合成计数节点、一侧太长时整侧按角色合成。

function view(over: Partial<PartyRelationshipView>): PartyRelationshipView {
  return {
    relationshipId: 'R1',
    revision: 1,
    side: 'HOLDER',
    role: 'CUSTOMER',
    otherPartyId: 'P-OTHER',
    otherPartyName: '对方',
    otherPartyNameKnown: true,
    scope: 'S',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 它作持有方的段在右（它对谁担任），作相对方的段在左（谁对它担任）；结束的段不上图、只计数；名称未知交 null。
test('relationshipGraphOf 按侧分列，结束的不上图', () => {
  const graph = relationshipGraphOf([
    view({ relationshipId: 'R1', side: 'HOLDER', role: 'CUSTOMER', otherPartyId: 'OP-1', otherPartyName: '运营方' }),
    view({ relationshipId: 'R2', side: 'COUNTERPARTY', role: 'AUTHORIZED_DELIVERING_PARTY', otherPartyId: 'D-1', otherPartyNameKnown: false, status: 'CANDIDATE' }),
    view({ relationshipId: 'R3', side: 'HOLDER', role: 'CUSTOMER', otherPartyId: 'OP-2', status: 'REVOKED' }),
  ]);
  deepEqual(graph.fromIt, [{ kind: 'party', key: 'R1', partyId: 'OP-1', name: '运营方', role: 'CUSTOMER', status: 'EFFECTIVE' }]);
  deepEqual(graph.towardIt, [{ kind: 'party', key: 'R2', partyId: 'D-1', name: null, role: 'AUTHORIZED_DELIVERING_PARTY', status: 'CANDIDATE' }]);
  equal(graph.ended, 1);
});

// Covers: 同一侧同一角色超过上限合成一个计数节点，并数出其中待确认的段；没超的照样逐家摆，按角色词表序排。
test('relationshipGraphOf 同角色多了合成计数节点', () => {
  const customers = Array.from({ length: relationshipGraphRoleLimit + 1 }, (_, index) =>
    view({ relationshipId: `C${index}`, side: 'COUNTERPARTY', role: 'CUSTOMER', otherPartyId: `CU-${index}`, status: index === 0 ? 'CANDIDATE' : 'EFFECTIVE' }),
  );
  const graph = relationshipGraphOf([...customers, view({ relationshipId: 'A1', side: 'COUNTERPARTY', role: 'CARRIER_AGENT', otherPartyId: 'AG-1' })]);
  deepEqual(
    graph.towardIt.map((node) => (node.kind === 'group' ? `${node.role}×${node.count}/${node.candidates}` : node.partyId)),
    [`CUSTOMER×${relationshipGraphRoleLimit + 1}/1`, 'AG-1'],
  );
});

// Covers: 一侧逐家摆超过列上限时，这一侧凡多于一家的角色都合成计数节点。
test('relationshipGraphOf 一侧太长时按角色合成', () => {
  const roles = ['CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT'];
  const views = roles.flatMap((role) =>
    Array.from({ length: relationshipGraphRoleLimit }, (_, index) => view({ relationshipId: `${role}-${index}`, side: 'HOLDER', role, otherPartyId: `${role}-${index}` })),
  );
  equal(views.length > relationshipGraphColumnLimit, true);
  deepEqual(
    relationshipGraphOf(views).fromIt.map((node) => (node.kind === 'group' ? `${node.role}×${node.count}` : node.partyId)),
    ['CUSTOMER×3', 'SUPPLIER×3', 'CARRIER_AGENT×3'],
  );
});

// Covers: 没有关系时两侧都空、结束段为零。
test('relationshipGraphOf 没有关系', () => {
  deepEqual(relationshipGraphOf([]), { towardIt: [], fromIt: [], ended: 0 });
});

// Covers: 表单里的三格折成图上那一段（票 business-party-redesign/11）——中心是持有方时另一方在右、是相对方时在左；名称从候选里
// 认，认不出是 null；角色或某一方没选、双方是同一家、双方都不是中心，都画不出来；首尾空白不算。
test('relationshipGraphDraftOf 三格齐了且其中一方是中心才有这一段', () => {
  const nameOf = (partyId: string) => (partyId === 'OP-1' ? '运营方' : null);
  deepEqual(relationshipGraphDraftOf('ME', { holder: 'ME', counterparty: ' OP-1 ', role: 'CUSTOMER' }, nameOf), {
    side: 'HOLDER',
    otherPartyId: 'OP-1',
    otherPartyName: '运营方',
    role: 'CUSTOMER',
  });
  deepEqual(relationshipGraphDraftOf('ME', { holder: 'D-9', counterparty: 'ME', role: 'AUTHORIZED_DELIVERING_PARTY' }, nameOf), {
    side: 'COUNTERPARTY',
    otherPartyId: 'D-9',
    otherPartyName: null,
    role: 'AUTHORIZED_DELIVERING_PARTY',
  });
  equal(relationshipGraphDraftOf('ME', { holder: 'ME', counterparty: 'OP-1', role: '' }, nameOf), null);
  equal(relationshipGraphDraftOf('ME', { holder: 'ME', counterparty: '', role: 'CUSTOMER' }, nameOf), null);
  equal(relationshipGraphDraftOf('ME', { holder: '', counterparty: 'ME', role: 'CUSTOMER' }, nameOf), null);
  equal(relationshipGraphDraftOf('ME', { holder: 'ME', counterparty: 'ME', role: 'CUSTOMER' }, nameOf), null);
  equal(relationshipGraphDraftOf('ME', { holder: 'OP-1', counterparty: 'OP-2', role: 'CUSTOMER' }, nameOf), null);
});

// Covers: 正在填的那一段排在它那一侧的最后、另一侧不动，已在册的各段不换位置；同角色已合成计数节点时它也单独一枚；
// 没有草稿原样交回（同一个对象）。
test('relationshipGraphWithDraft 把正在填的一段排在那一侧的末尾', () => {
  const customers = Array.from({ length: relationshipGraphRoleLimit + 1 }, (_, index) =>
    view({ relationshipId: `C${index}`, side: 'HOLDER', role: 'CUSTOMER', otherPartyId: `OP-${index}` }),
  );
  const graph = relationshipGraphOf([...customers, view({ relationshipId: 'T1', side: 'COUNTERPARTY', role: 'CARRIER_AGENT', otherPartyId: 'AG-1' })]);
  equal(relationshipGraphWithDraft(graph, null), graph);

  const right = relationshipGraphWithDraft(graph, { side: 'HOLDER', otherPartyId: 'OP-NEW', otherPartyName: '新对方', role: 'CUSTOMER' });
  deepEqual(right.towardIt, graph.towardIt);
  deepEqual(
    right.fromIt.map((node) => (node.kind === 'group' ? `${node.role}×${node.count}` : `${node.kind}:${node.partyId}`)),
    [`CUSTOMER×${relationshipGraphRoleLimit + 1}`, 'draft:OP-NEW'],
  );
  deepEqual(right.fromIt[right.fromIt.length - 1], { kind: 'draft', key: 'draft', partyId: 'OP-NEW', name: '新对方', role: 'CUSTOMER' });

  const left = relationshipGraphWithDraft(graph, { side: 'COUNTERPARTY', otherPartyId: 'D-9', otherPartyName: null, role: 'SUPPLIER' });
  deepEqual(left.fromIt, graph.fromIt);
  deepEqual(left.towardIt.map((node) => `${node.kind}:${node.kind === 'group' ? node.role : node.partyId}`), ['party:AG-1', 'draft:D-9']);
  equal(left.ended, graph.ended);
});
