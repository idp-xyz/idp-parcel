import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { liveRelationshipCountOf, panelHintOf, panelPrimaryActionOf } from './business-party-panel-first-screen';
import type { PartyRelationshipView } from './party-overview';

// 本文件钉的是右栏首屏那两句话与主动作怎么选（票 business-party-ux/08）：提示只在待生效、或已生效却没有在用关系时出现，
// 我方公司不提「没有关系」；没有在用关系时主动作换成「登记合作关系」，其余照旧。

function view(over: Partial<PartyRelationshipView>): PartyRelationshipView {
  return {
    relationshipId: 'R1',
    revision: 1,
    side: 'HOLDER',
    role: 'CUSTOMER',
    otherPartyId: 'SYN-PARTY-CN-SZ-01',
    otherPartyNameKnown: true,
    scope: 'ALL',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 在用 = 候选与已生效，两侧都算；到期、撤销、替代不算。
test('liveRelationshipCountOf 只数候选与已生效，两侧都算', () => {
  equal(
    liveRelationshipCountOf([
      view({}),
      view({ relationshipId: 'R2', status: 'CANDIDATE', side: 'COUNTERPARTY' }),
      view({ relationshipId: 'R3', status: 'EXPIRED' }),
      view({ relationshipId: 'R4', status: 'REVOKED' }),
      view({ relationshipId: 'R5', status: 'SUPERSEDED' }),
    ]),
    2,
  );
  equal(liveRelationshipCountOf([]), 0);
});

// Covers: 待生效提示带生效时点；已生效且没有在用关系才提「还没有合作关系」；我方公司不提；关系册没答出不提；已停用不提（横幅在说）。
test('panelHintOf 按身份状态与在用关系数选提示', () => {
  deepEqual(panelHintOf({ status: 'REGISTERED', effectiveFrom: '2030-01-01T00:00:00Z' }, false, 0), {
    kind: 'pending',
    effectiveFrom: '2030-01-01T00:00:00Z',
  });
  deepEqual(panelHintOf({ status: 'EFFECTIVE', effectiveFrom: '2026-01-01T00:00:00Z' }, false, 0), { kind: 'no-relationship' });
  equal(panelHintOf({ status: 'EFFECTIVE', effectiveFrom: '2026-01-01T00:00:00Z' }, false, 1), null);
  equal(panelHintOf({ status: 'EFFECTIVE', effectiveFrom: '2026-01-01T00:00:00Z' }, true, 0), null);
  equal(panelHintOf({ status: 'EFFECTIVE', effectiveFrom: '2026-01-01T00:00:00Z' }, false, null), null);
  equal(panelHintOf({ status: 'DEACTIVATED', effectiveFrom: '2026-01-01T00:00:00Z' }, false, 0), null);
});

// Covers: 没有写授予没有主动作；没有在用关系时主动作是「登记合作关系」；我方公司（公司主体钉着）照旧不摆「更新资料」；
// 法人册没答出（null）照今天的做法先摆「更新资料」。
test('panelPrimaryActionOf 随提示与授予选主动作', () => {
  equal(panelPrimaryActionOf(false, false, { kind: 'no-relationship' }), null);
  equal(panelPrimaryActionOf(true, false, { kind: 'no-relationship' }), 'register-relationship');
  equal(panelPrimaryActionOf(true, false, null), 'update-profile');
  equal(panelPrimaryActionOf(true, false, { kind: 'pending', effectiveFrom: '2030-01-01T00:00:00Z' }), 'update-profile');
  equal(panelPrimaryActionOf(true, true, null), null);
  equal(panelPrimaryActionOf(true, null, null), 'update-profile');
});
