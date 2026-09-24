import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord, CustomerAccountRecord, GroupLegalEntityRecord } from './api';
import { identityKindLabels } from './presentation';
import {
  emptyIdentityDeactivationDraft,
  identityDeactivationFieldPaths,
  identityDeactivationLocalProblems,
  identityDeactivationPayloadOf,
  identityKindOptions,
  identityTargetOf,
  isIdentityKind,
  suggestedDeactivationRevision,
  type IdentityDeactivationDraft,
} from './identity-deactivation-form';

// 本文件钉的是停用表单只做编码层的事：修订号编成整数、停用时刻换成 RFC 3339、留空缺席；种类词只从
// identityKindLabels 派生；修订建议按种类去对应册上数。载荷镜像 deactivationDocument 且不带 tenantId、不裁空白。

function draft(over: Partial<IdentityDeactivationDraft>): IdentityDeactivationDraft {
  return {
    ...emptyIdentityDeactivationDraft(),
    kind: 'BUSINESS_PARTY',
    id: 'SYN-PARTY-AGENT-07',
    revision: '2',
    basis: 'SYN-DEACT-BASIS-07',
    at: '2026-03-01T08:00',
    ...over,
  };
}

const party = (partyId: string, revision: number): BusinessPartyRecord => ({
  tenantId: 'SYN-TENANT-01',
  partyId,
  partyName: 'x',
  status: 'EFFECTIVE',
  revision,
  basis: 'b',
  effectiveFrom: '2026-01-02T00:00:00Z',
  registeredAt: '2026-01-02T00:00:00Z',
});

const legalEntity = (legalEntityId: string, revision: number): GroupLegalEntityRecord => ({
  tenantId: 'SYN-TENANT-01',
  legalEntityId,
  kind: 'RESPONSIBLE_LEGAL_ENTITY',
  partyId: 'SYN-PARTY-OPERATOR-01',
  partyNameKnown: true,
  partyName: 'x',
  status: 'EFFECTIVE',
  revision,
  basis: 'b',
  effectiveFrom: '2026-01-02T00:00:00Z',
  registeredAt: '2026-01-02T00:00:00Z',
  identityLayerRegistered: false,
});

const account = (accountId: string, revision: number): CustomerAccountRecord => ({
  tenantId: 'SYN-TENANT-01',
  accountId,
  customerPartyId: 'SYN-PARTY-SHIPPER-01',
  customerPartyNameKnown: true,
  customerPartyName: 'x',
  status: 'EFFECTIVE',
  revision,
  basis: 'b',
  effectiveFrom: '2026-01-02T00:00:00Z',
  registeredAt: '2026-01-02T00:00:00Z',
});

// Covers: 各格齐 → deactivations 一项；revision 是 JSON 整数；at 按时区换成 UTC；顶层没有 tenantId。
test('草稿组成载荷：一项、整数修订、UTC 时刻、无租户格', () => {
  const payload = identityDeactivationPayloadOf(draft({}), 'Asia/Shanghai');
  deepEqual(payload, {
    deactivations: [
      {
        kind: 'BUSINESS_PARTY',
        id: 'SYN-PARTY-AGENT-07',
        revision: 2,
        basis: 'SYN-DEACT-BASIS-07',
        at: '2026-03-01T00:00:00Z',
      },
    ],
  });
  equal('tenantId' in payload, false);
});

// Covers: 首尾空白原样带、空串照送（含种类「未选」的空串）；停用时刻留空则键缺席，不编成零时刻。
test('空白原样带、空串照送、时刻留空缺席', () => {
  const item = identityDeactivationPayloadOf(draft({ kind: '', id: ' SYN-LE-01 ', basis: '', at: '' }), 'UTC')
    .deactivations[0];
  deepEqual(item, { kind: '', id: ' SYN-LE-01 ', revision: 2, basis: '' });
});

// Covers: 本地只报编码层——修订号编不进正整数、时刻填了却换不出来；种类未选、标识与依据为空不是本地的话。
test('本地只报编码层问题', () => {
  deepEqual(identityDeactivationLocalProblems(draft({}), 'UTC'), {});
  deepEqual(identityDeactivationLocalProblems(draft({ revision: '-1' }), 'UTC'), {
    'deactivations[0].revision': ['修订号要填正整数'],
  });
  deepEqual(identityDeactivationLocalProblems(draft({ at: 'now' }), 'UTC'), {
    'deactivations[0].at': ['停用时刻要是可解析的日期时间'],
  });
  deepEqual(identityDeactivationLocalProblems(draft({ kind: '', id: '', basis: '' }), 'UTC'), {});
});

// Covers: 修订号编不进整数时载荷不造假数——那一格缺席，由问题表拦住不送。
test('修订号编不进整数时载荷缺席该格', () => {
  equal('revision' in identityDeactivationPayloadOf(draft({ revision: 'two' }), 'UTC').deactivations[0], false);
});

// Covers: 种类下拉只从 identityKindLabels 派生——码是它的键、顺序同它；每项显中文与码。
test('种类选项从 identityKindLabels 派生', () => {
  const options = identityKindOptions();
  deepEqual(
    options.map((option) => option.value),
    Object.keys(identityKindLabels),
  );
  deepEqual(options[0], { value: 'BUSINESS_PARTY', label: '业务参与方 · BUSINESS_PARTY' });
});

// Covers: 按种类把对应册的一行投成「标识 + 修订」——参与方册按 partyId、法人册按 legalEntityId、客户账户册按 accountId。
// 三册体形各异，取键只在这一份投影里；组件那侧的 kindRegisters 按种类取到册后逐行调它，不各自再写一遍取键——
// 「种类 → 取哪一册」由组件那张表说，纯模块不另留一张（票 14 第 1 条）。
test('按种类把对应册的一行投成「标识 + 修订」', () => {
  deepEqual(identityTargetOf.BUSINESS_PARTY(party('SYN-PARTY-01', 3)), { id: 'SYN-PARTY-01', revision: 3 });
  deepEqual(identityTargetOf.LEGAL_ENTITY(legalEntity('SYN-LE-01', 2)), { id: 'SYN-LE-01', revision: 2 });
  deepEqual(identityTargetOf.CUSTOMER_ACCOUNT(account('SYN-ACC-01', 5)), { id: 'SYN-ACC-01', revision: 5 });
});

// Covers: 建议修订号 = 该身份最新修订 + 1（停用落点的修订号，isolated_write_intake.go 停用口注释）；不在册 / 册未取到 /
// 标识空一律 1；按原串比不裁空白。
test('建议修订号取该身份最新修订加一，不在册或册未取到为一', () => {
  const targets = [{ id: 'SYN-PARTY-01', revision: 3 }];
  equal(suggestedDeactivationRevision(targets, 'SYN-PARTY-01'), 4);
  equal(suggestedDeactivationRevision(targets, 'SYN-PARTY-01 '), 1);
  equal(suggestedDeactivationRevision(targets, 'SYN-PARTY-99'), 1);
  equal(suggestedDeactivationRevision(targets, ''), 1);
  equal(suggestedDeactivationRevision(null, 'SYN-PARTY-01'), 1);
});

// Covers: 票 13 第 2 条——按种类分派的表键在 identityKindLabels 这个封闭集上：词表每一格都在表里（投影与词表一起长，
// 漏一格在这里显）；集外的串与空串不是「另一种」，一律判不在集内。组件那张 kindRegisters 键在 Record<IdentityKind, …>
// 上，键集由 tsc 钉（少一格编不过、多一格是多余属性）——组件模块不进 node:test 的编译，这里钉得到的是纯模块这一份。
test('分派表与种类词表同键，集外不认', () => {
  for (const code of Object.keys(identityKindLabels)) {
    equal(isIdentityKind(code), true);
    equal(code in identityTargetOf, true);
  }
  equal(isIdentityKind(''), false);
  equal(isIdentityKind('RELATIONSHIP'), false);
  equal(isIdentityKind('business_party'), false);
  deepEqual(Object.keys(identityTargetOf).sort(), Object.keys(identityKindLabels).sort());
});

// Covers: 认领的路径表与 deactivationDocument 的键一一对应。
test('认领的路径表', () => {
  deepEqual([...identityDeactivationFieldPaths], [
    'deactivations[0].kind',
    'deactivations[0].id',
    'deactivations[0].revision',
    'deactivations[0].basis',
    'deactivations[0].at',
  ]);
});
