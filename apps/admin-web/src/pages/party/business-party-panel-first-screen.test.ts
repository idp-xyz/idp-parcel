import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { deactivationLiveRelationshipsNote, panelHeaderActionsOf, panelHintOf } from './business-party-panel-first-screen';
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

// Covers: 停用对话框说明后面那一句——名下有在用的合作关系才说、报出段数；没有不说；数不知道（关系册没答出或没读全）也不说。
test('deactivationLiveRelationshipsNote 有在用的关系才报段数，不知道就不说', () => {
  equal(deactivationLiveRelationshipsNote(3), '它名下还有 3 段在用的合作关系。');
  equal(deactivationLiveRelationshipsNote(1286), '它名下还有 1,286 段在用的合作关系。');
  equal(deactivationLiveRelationshipsNote(0), null);
  equal(deactivationLiveRelationshipsNote(null), null);
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

// Covers: 没有写授予什么都不摆；没有在用关系时「登记合作关系」作主动作、「更新资料」退成描边；有在用关系的已生效单位两枚
// 都在、「登记合作关系」是描边；待生效与已停用的不摆「登记合作关系」；我方公司（公司主体钉着）两枚都不摆，只有提示是
// 「还没有合作关系」时留「登记合作关系」；法人册没答出（null）两枚都先摆，与对象页同一做法。登过停用的（末一个参数为假）
// 没有「更新资料」——已停用的两枚都不摆；停用时点还没到、状态仍是已生效的，「登记合作关系」照旧摆。
test('panelHeaderActionsOf 随提示、状态与授予摆头区动作', () => {
  const none = { kind: 'no-relationship' } as const;
  deepEqual(panelHeaderActionsOf(false, false, none, 'EFFECTIVE', true), { registerRelationship: null, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, false, none, 'EFFECTIVE', true), { registerRelationship: 'primary', updateProfile: 'outline' });
  deepEqual(panelHeaderActionsOf(true, false, null, 'EFFECTIVE', true), { registerRelationship: 'outline', updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, false, { kind: 'pending' }, 'REGISTERED', true), { registerRelationship: null, updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, false, null, 'DEACTIVATED', false), { registerRelationship: null, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, false, null, 'EFFECTIVE', false), { registerRelationship: 'outline', updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, false, none, 'EFFECTIVE', false), { registerRelationship: 'primary', updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, true, null, 'EFFECTIVE', true), { registerRelationship: null, updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, true, none, 'EFFECTIVE', true), { registerRelationship: 'primary', updateProfile: null });
  deepEqual(panelHeaderActionsOf(true, null, null, 'EFFECTIVE', true), { registerRelationship: 'outline', updateProfile: 'primary' });
  deepEqual(panelHeaderActionsOf(true, null, none, 'EFFECTIVE', true), { registerRelationship: 'primary', updateProfile: 'outline' });
});
