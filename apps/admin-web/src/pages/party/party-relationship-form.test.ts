import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PartyRelationshipRecord } from './api';
import { partyRoleLabels } from './presentation';
import {
  emptyPartyRelationshipDraft,
  partyRelationshipFieldPaths,
  partyRelationshipLocalProblems,
  partyRelationshipPayloadOf,
  partyRoleOptions,
  suggestedPartyRelationshipRevision,
  type PartyRelationshipDraft,
} from './party-relationship-form';

// 本文件钉的是关系表单只做编码层的事：修订号编成整数、各墙钟时刻换成 RFC 3339、**可缺键缺席而不是零值**——
// partyRelationshipDocument 用指针表达 effectiveEndsAt 与 approval 的缺席（缺终点即开区间、缺批准即候选关系），
// 空串或零时刻顶上去在服务端就是另一个意思。载荷不带 tenantId、各格不裁首尾空白，同 business-party-form。

function draft(over: Partial<PartyRelationshipDraft>): PartyRelationshipDraft {
  return {
    ...emptyPartyRelationshipDraft(),
    relationshipId: 'SYN-REL-AGENT-07',
    revision: '1',
    holder: 'SYN-PARTY-AGENT-07',
    counterparty: 'SYN-PARTY-SHIPPER-01',
    role: 'CARRIER_AGENT',
    scope: 'SYN-SCOPE-CN-01',
    basis: 'SYN-REL-BASIS-07',
    effectiveStartsAt: '2026-01-02T08:00',
    effectiveEndsAt: '',
    approved: false,
    approvalReference: '',
    approvedAt: '',
    ...over,
  };
}

// Covers: 全格齐（含终点与批准）→ relationships 一项；各时刻按时区换 UTC；approval 是嵌套对象；顶层无 tenantId。
test('全格草稿组成载荷：终点与批准都在、各时刻换 UTC、无租户格', () => {
  const payload = partyRelationshipPayloadOf(
    draft({
      effectiveEndsAt: '2026-12-31T08:00',
      approved: true,
      approvalReference: 'SYN-APPROVAL-07',
      approvedAt: '2026-01-01T08:00',
    }),
    'Asia/Shanghai',
  );
  deepEqual(payload, {
    relationships: [
      {
        relationshipId: 'SYN-REL-AGENT-07',
        revision: 1,
        holder: 'SYN-PARTY-AGENT-07',
        counterparty: 'SYN-PARTY-SHIPPER-01',
        role: 'CARRIER_AGENT',
        scope: 'SYN-SCOPE-CN-01',
        basis: 'SYN-REL-BASIS-07',
        effectiveStartsAt: '2026-01-02T00:00:00Z',
        effectiveEndsAt: '2026-12-31T00:00:00Z',
        approval: { reference: 'SYN-APPROVAL-07', approvedAt: '2026-01-01T00:00:00Z' },
      },
    ],
  });
  equal('tenantId' in payload, false);
});

// Covers: 终点留空 → effectiveEndsAt 键缺席（开区间）；不勾「已批准」→ approval 键缺席（候选关系），即便批准那几格里
// 残留着字——勾选框才是「有没有批准事实」的声明，残字不是。
test('可缺键缺席：终点留空即开区间、不勾已批准即候选关系', () => {
  const item = partyRelationshipPayloadOf(
    draft({ effectiveEndsAt: '', approved: false, approvalReference: 'SYN-APPROVAL-07', approvedAt: '2026-01-01T08:00' }),
    'UTC',
  ).relationships[0];
  equal('effectiveEndsAt' in item, false);
  equal('approval' in item, false);
  deepEqual(item, {
    relationshipId: 'SYN-REL-AGENT-07',
    revision: 1,
    holder: 'SYN-PARTY-AGENT-07',
    counterparty: 'SYN-PARTY-SHIPPER-01',
    role: 'CARRIER_AGENT',
    scope: 'SYN-SCOPE-CN-01',
    basis: 'SYN-REL-BASIS-07',
    effectiveStartsAt: '2026-01-02T08:00:00Z',
  });
});

// Covers: 勾了已批准而批准时刻留空 → approval 在场、只带 reference，approvedAt 缺席——批准时刻是否可缺由服务端答
// （Approve 对零时刻拒），表单不替它补当前时刻。起点留空同理缺席。
test('勾了已批准但时刻留空：approval 只带引用，起点留空缺席', () => {
  const item = partyRelationshipPayloadOf(
    draft({ effectiveStartsAt: '', approved: true, approvalReference: 'SYN-APPROVAL-07', approvedAt: '' }),
    'UTC',
  ).relationships[0];
  equal('effectiveStartsAt' in item, false);
  deepEqual(item.approval, { reference: 'SYN-APPROVAL-07' });
});

// Covers: 首尾空白原样带、空串照送（含角色「未选」的空串——不在封闭集内由服务端点名，表单不预选）。
test('空白原样带、空串照送、角色未选照送空串', () => {
  const item = partyRelationshipPayloadOf(
    draft({ relationshipId: ' SYN-REL-AGENT-07 ', holder: '', role: '', scope: ' s ', basis: '' }),
    'UTC',
  ).relationships[0];
  equal(item.relationshipId, ' SYN-REL-AGENT-07 ');
  equal(item.holder, '');
  equal(item.role, '');
  equal(item.scope, ' s ');
  equal(item.basis, '');
});

// Covers: 本地只报编码层——修订号、各时刻换不出来时报在各自路径上；批准时刻只在勾了已批准时才判；
// 角色未选、双方为空、区间倒置都不是本地的话。
test('本地只报编码层问题', () => {
  deepEqual(partyRelationshipLocalProblems(draft({}), 'UTC'), {});
  deepEqual(partyRelationshipLocalProblems(draft({ revision: '2.0' }), 'UTC'), {
    'relationships[0].revision': ['修订号要填正整数'],
  });
  deepEqual(partyRelationshipLocalProblems(draft({ effectiveStartsAt: 'soon' }), 'UTC'), {
    'relationships[0].effectiveStartsAt': ['生效起点要是可解析的日期时间'],
  });
  deepEqual(partyRelationshipLocalProblems(draft({ effectiveEndsAt: '2026-13-01T00:00' }), 'UTC'), {
    'relationships[0].effectiveEndsAt': ['生效终点要是可解析的日期时间'],
  });
  deepEqual(
    partyRelationshipLocalProblems(draft({ approved: true, approvalReference: 'r', approvedAt: 'never' }), 'UTC'),
    { 'relationships[0].approval.approvedAt': ['批准时刻要是可解析的日期时间'] },
  );
  deepEqual(partyRelationshipLocalProblems(draft({ approved: false, approvedAt: 'never' }), 'UTC'), {});
  deepEqual(
    partyRelationshipLocalProblems(
      draft({ role: '', holder: '', counterparty: '', effectiveStartsAt: '2026-12-31T00:00', effectiveEndsAt: '2026-01-01T00:00' }),
      'UTC',
    ),
    {},
  );
});

// Covers: 建议修订号按 relationshipId 数关系册——在册取最新 + 1，不在册 / 列表 null / 标识空一律 1；按原串比不裁空白。
test('建议修订号取关系册最新修订加一，不在册或列表未取到为一', () => {
  const rows: PartyRelationshipRecord[] = [
    {
      tenantId: 'SYN-TENANT-01',
      relationshipId: 'SYN-REL-01',
      revision: 2,
      holderId: 'SYN-PARTY-AGENT-01',
      holderNameKnown: true,
      holderName: 'a',
      counterpartyId: 'SYN-PARTY-SHIPPER-01',
      counterpartyNameKnown: true,
      counterpartyName: 's',
      role: 'CARRIER_AGENT',
      scope: 'SYN-SCOPE-CN-01',
      basis: 'b',
      status: 'EFFECTIVE',
      effectiveStartsAt: '2026-01-02T00:00:00Z',
      registeredAt: '2026-01-02T00:00:00Z',
    },
  ];
  equal(suggestedPartyRelationshipRevision(rows, 'SYN-REL-01'), 3);
  equal(suggestedPartyRelationshipRevision(rows, 'SYN-REL-01 '), 1);
  equal(suggestedPartyRelationshipRevision(rows, 'SYN-REL-99'), 1);
  equal(suggestedPartyRelationshipRevision(rows, ''), 1);
  equal(suggestedPartyRelationshipRevision(null, 'SYN-REL-01'), 1);
});

// Covers: 角色下拉的选项只从 partyRoleLabels 派生——码是它的键、顺序同它，没有第二份封闭集写法；每项显中文与码。
test('角色选项从 partyRoleLabels 派生', () => {
  const options = partyRoleOptions();
  deepEqual(
    options.map((option) => option.value),
    Object.keys(partyRoleLabels),
  );
  deepEqual(options[0], { value: 'CUSTOMER', label: '客户 · CUSTOMER' });
});

// Covers: 认领的路径表与 partyRelationshipDocument 的键（含嵌套的 approval 各格）一一对应。
test('认领的路径表', () => {
  deepEqual([...partyRelationshipFieldPaths], [
    'relationships[0].relationshipId',
    'relationships[0].revision',
    'relationships[0].holder',
    'relationships[0].counterparty',
    'relationships[0].role',
    'relationships[0].scope',
    'relationships[0].basis',
    'relationships[0].effectiveStartsAt',
    'relationships[0].effectiveEndsAt',
    'relationships[0].approval.reference',
    'relationships[0].approval.approvedAt',
  ]);
});
