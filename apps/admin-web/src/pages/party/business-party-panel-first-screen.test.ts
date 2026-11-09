import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { liveRelationshipCountOf, panelHeaderActionsOf, panelHintOf } from './business-party-panel-first-screen';
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

// Covers: 待生效就提示（日期在事实行，不进提示）；已生效、没有在用关系、也没有别处认出的角色才提「还没有合作关系」；只凭客户账户
// 就是客户的不提；我方公司不提；关系册或客户账户册没答出不提；已停用不提（横幅在说）。
test('panelHintOf 按身份状态、在用关系数与持有角色数选提示', () => {
  const effective = { status: 'EFFECTIVE' };
  deepEqual(panelHintOf({ status: 'REGISTERED' }, false, 0, 0), { kind: 'pending' });
  deepEqual(panelHintOf({ status: 'REGISTERED' }, false, 3, 1), { kind: 'pending' });
  deepEqual(panelHintOf(effective, false, 0, 0), { kind: 'no-relationship' });
  equal(panelHintOf(effective, false, 1, 1), null);
  equal(panelHintOf(effective, false, 0, 1), null);
  equal(panelHintOf(effective, true, 0, 0), null);
  equal(panelHintOf(effective, false, null, 0), null);
  equal(panelHintOf(effective, false, 0, null), null);
  equal(panelHintOf({ status: 'DEACTIVATED' }, false, 0, 0), null);
});

// Covers: 没有写授予什么都不摆；没有在用关系时「登记合作关系」作主动作、「更新资料」退成描边；我方公司（公司主体钉着）
// 不摆「更新资料」、也不让它以描边出现；法人册没答出（null）先摆「更新资料」，与对象页同一做法。
test('panelHeaderActionsOf 随提示与授予摆头区动作', () => {
  const none = { kind: 'no-relationship' } as const;
  deepEqual(panelHeaderActionsOf(false, false, none), { registerRelationship: false, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, false, none), { registerRelationship: true, updateProfile: 'outline' });
  deepEqual(panelHeaderActionsOf(true, false, null), { registerRelationship: false, updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, false, { kind: 'pending' }), { registerRelationship: false, updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, true, null), { registerRelationship: false, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, true, none), { registerRelationship: true, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, null, null), { registerRelationship: false, updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, null, none), { registerRelationship: true, updateProfile: 'outline' });
});
