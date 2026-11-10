import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { relationshipGraphColumnLimit, relationshipGraphOf, relationshipGraphRoleLimit } from './party-relationship-graph';
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
