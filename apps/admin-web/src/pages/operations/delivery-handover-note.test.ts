import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { formatInstant } from '../catalogue-view';
import type { BusinessPartyRecord } from '../party/api';
import {
  claimRowsOf,
  confirmationProblemNote,
  confirmationRequestOf,
  deliveringParticipantOptionsOf,
  describeConfirmationAnswer,
  handoverMemberRowOf,
  handoverNoteRowOf,
} from './delivery-handover-note';
import type { DeliveryHandoverConfirmationResponseBody, DeliveryHandoverNoteRecord } from './delivery-scope-api';

// 送站交接单区与详情上的「确认」（票 ps-external-mark-relations/13「做什么」的「查阅读口」「确认绑定动作」两项；UC-NO-006 步骤 5B）。最容易错的几格：没下结论的
// 账户与否认的账户长成一样、确认按钮出现在已有结论的账户上；载荷里漏了交付方参与方或依据引用；「未配置」与「不在授权交付关系内」被当成
// 失败，或两句写成同一个恢复动作。

function note(overrides: Partial<DeliveryHandoverNoteRecord> = {}): DeliveryHandoverNoteRecord {
  return {
    noteId: 'SYN-HN-PSXM13-1',
    node: 'SYN-NODE-PSXM13',
    deliveringParty: 'SYN-DRIVER-PSXM13',
    openedAt: '2026-10-20T09:00:00Z',
    closed: false,
    memberCount: 0,
    claims: [{ customerAccount: 'SYN-ACCOUNT-01' }],
    scopeEstablished: false,
    ...overrides,
  };
}

const confirmedByOperator = {
  customerAccount: 'SYN-ACCOUNT-01',
  conclusion: 'CONFIRMED',
  route: 'OPERATION_DECISION',
  concluder: 'OPERATOR/https://id.syn.example/dex#syn-operator',
  concludedAt: '2026-10-20T11:00:00Z',
  deliveringParticipant: 'SYN-PARTY-DELIVERER-01',
  basis: 'SYN-BASIS-1',
};
const deniedByCustomer = {
  customerAccount: 'SYN-ACCOUNT-02',
  conclusion: 'DENIED',
  route: 'CUSTOMER_CHANNEL',
  concluder: 'https://id.syn.example/dex#syn-client',
  concludedAt: '2026-10-20T12:00:00Z',
};

test('交接单行：开放与关闭、成员数、绑定进度与范围成立各写各的字', () => {
  deepEqual(handoverNoteRowOf(note({ claims: [{ customerAccount: 'SYN-ACCOUNT-01' }, { customerAccount: 'SYN-ACCOUNT-03' }] })), {
    key: 'handover-note:SYN-HN-PSXM13-1',
    noteId: 'SYN-HN-PSXM13-1',
    values: {
      noteId: 'SYN-HN-PSXM13-1',
      node: 'SYN-NODE-PSXM13',
      deliveringParty: 'SYN-DRIVER-PSXM13',
      openedAt: formatInstant('2026-10-20T09:00:00Z'),
      state: '开放',
      members: '0',
      claims: 'SYN-ACCOUNT-01、SYN-ACCOUNT-03',
      bindings: '已确认 0 · 已否认 0 · 待确认 2',
      scope: '未成立：还有账户待确认',
    },
  });
  const established = handoverNoteRowOf(
    note({
      closed: true,
      closedAt: '2026-10-20T10:00:00Z',
      memberCount: 2,
      claims: [confirmedByOperator],
      scopeEstablished: true,
      scopeEstablishedAt: '2026-10-20T11:00:00Z',
    }),
  );
  equal(established.values.state, `已关闭 · ${formatInstant('2026-10-20T10:00:00Z')}`);
  equal(established.values.scope, `已成立 · ${formatInstant('2026-10-20T11:00:00Z')}`);
  equal(established.values.bindings, '已确认 1 · 已否认 0 · 待确认 0');
  const refused = handoverNoteRowOf(note({ claims: [{ customerAccount: 'SYN-ACCOUNT-01' }, deniedByCustomer] }));
  ok(refused.values.scope.startsWith('不成立'), refused.values.scope);
  notEqual(refused.values.scope, handoverNoteRowOf(note()).values.scope);
});

test('声称的账户：待确认的给「确认」，已确认、已否认的不给并说为什么；运营决定的带交付方参与方与依据', () => {
  const rows = claimRowsOf(note({ claims: [{ customerAccount: 'SYN-ACCOUNT-03' }, confirmedByOperator, deniedByCustomer] }));
  deepEqual(rows[0].confirm, { kind: 'available', noteId: 'SYN-HN-PSXM13-1', customerAccount: 'SYN-ACCOUNT-03' });
  equal(rows[0].values.conclusion, '待确认');
  equal(rows[0].values.route, '');
  equal(rows[1].values.conclusion, '已确认');
  equal(rows[1].values.route, '运营决定');
  equal(rows[1].values.evidence, '交付方参与方 SYN-PARTY-DELIVERER-01 · 依据 SYN-BASIS-1');
  equal(rows[1].values.concluded, `OPERATOR/https://id.syn.example/dex#syn-operator · ${formatInstant('2026-10-20T11:00:00Z')}`);
  equal(rows[1].confirm.kind, 'unavailable');
  equal(rows[2].values.conclusion, '已否认');
  equal(rows[2].values.route, '客户渠道');
  equal(rows[2].values.evidence, '');
  ok(rows[2].confirm.kind === 'unavailable' && rows[2].confirm.reason.includes('否认'), JSON.stringify(rows[2].confirm));
  ok(rows[1].confirm.kind === 'unavailable' && rows[1].confirm.reason.includes('已确认'), JSON.stringify(rows[1].confirm));
});

test('成员行：形成方式写原词，事实身份与业务时间照列', () => {
  deepEqual(handoverMemberRowOf({ factId: 'SYN-SCAN-1', unit: 'SYN-UNIT-1', membership: 'RECEPTION', formedAt: '2026-10-20T09:10:00Z' }), {
    key: 'member:RECEPTION:SYN-SCAN-1',
    values: { unit: 'SYN-UNIT-1', membership: '收寄带来', factId: 'SYN-SCAN-1', formedAt: formatInstant('2026-10-20T09:10:00Z') },
  });
  equal(handoverMemberRowOf({ factId: 'F', unit: 'U', membership: 'ASSIGNMENT', formedAt: '2026-10-20T09:10:00Z' }).values.membership, '归入');
  equal(handoverMemberRowOf({ factId: 'F', unit: 'U', membership: 'NEW_KIND', formedAt: '2026-10-20T09:10:00Z' }).values.membership, 'NEW_KIND');
});

test('确认载荷：四格齐了才发，交付方参与方没选、依据引用空白各说一句；载荷不带租户与操作者', () => {
  const draft = { noteId: 'SYN-HN-PSXM13-1', customerAccount: 'SYN-ACCOUNT-01', deliveringParticipant: 'SYN-PARTY-DELIVERER-01', basis: 'SYN-BASIS-1' };
  deepEqual(confirmationRequestOf(draft), { kind: 'ready', request: draft });
  const noParticipant = confirmationRequestOf({ ...draft, deliveringParticipant: '' });
  ok(noParticipant.kind === 'incomplete' && noParticipant.reason.includes('交付方参与方'), JSON.stringify(noParticipant));
  const blankBasis = confirmationRequestOf({ ...draft, basis: '   ' });
  ok(blankBasis.kind === 'incomplete' && blankBasis.reason.includes('依据引用'), JSON.stringify(blankBasis));
  const ready = confirmationRequestOf(draft);
  ok(ready.kind === 'ready');
  deepEqual(Object.keys(ready.request).sort(), ['basis', 'customerAccount', 'deliveringParticipant', 'noteId']);
});

function answer(overrides: Partial<DeliveryHandoverConfirmationResponseBody>): DeliveryHandoverConfirmationResponseBody {
  return { outcome: 'BINDING_CONFIRMED', ...overrides };
}

test('确认答复：落了结论才算 landed；范围成立与否照答复写；已有结果与已有结论说的是在册那一份', () => {
  const binding = { customerAccount: 'SYN-ACCOUNT-01', conclusion: 'CONFIRMED', route: 'OPERATION_DECISION', concludedAt: '2026-10-20T11:00:00Z', deliveringParticipant: 'SYN-PARTY-DELIVERER-01', basis: 'SYN-BASIS-1' };
  const established = describeConfirmationAnswer(answer({ binding, scopeEstablishedAt: '2026-10-20T11:00:00Z' }));
  equal(established.tone, 'answered');
  equal(established.landed, true);
  ok(established.details.some((line) => line.includes('已成立')), established.details.join(' | '));
  const partial = describeConfirmationAnswer(answer({ binding }));
  ok(partial.details.some((line) => line.includes('尚未成立')), partial.details.join(' | '));

  const replay = describeConfirmationAnswer(answer({ outcome: 'EXISTING_RESULT', binding }));
  equal(replay.landed, false);
  equal(replay.tone, 'answered');
  const conflicting = describeConfirmationAnswer(answer({ outcome: 'EXISTING_CONCLUSION', binding: { ...binding, conclusion: 'DENIED', route: 'CUSTOMER_CHANNEL' } }));
  equal(conflicting.landed, false);
  ok(conflicting.details.some((line) => line.includes('已否认')), conflicting.details.join(' | '));
});

test('确认答复：「不在授权交付关系内」与「未配置」是答案不是失败，各写明恢复动作，两句不同', () => {
  const outside = describeConfirmationAnswer(answer({ outcome: 'NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP' }));
  const unconfigured = describeConfirmationAnswer(answer({ outcome: 'AUTHORIZED_DELIVERY_NOT_CONFIGURED' }));
  for (const note of [outside, unconfigured]) {
    equal(note.tone, 'answered');
    equal(note.landed, false);
    ok(note.details.some((line) => line.includes('未落结论')), note.details.join(' | '));
    ok(note.details.some((line) => line.includes('授权交付关系')), note.details.join(' | '));
  }
  ok(outside.details.some((line) => line.includes('换')), outside.details.join(' | '));
  ok(unconfigured.details.some((line) => line.includes('一条') && line.includes('登记')), unconfigured.details.join(' | '));
  notEqual(outside.details.join(), unconfigured.details.join());
  const unknown = describeConfirmationAnswer(answer({ outcome: 'SOMETHING_NEW' }));
  equal(unknown.tone, 'refused');
  ok(unknown.headline.includes('SOMETHING_NEW'));
});

test('确认口问题码：看不见、准入范围、坏载荷与没形成答案各有一句', () => {
  ok(confirmationProblemNote('DELIVERY_HANDOVER_NOTE_NOT_VISIBLE').includes('声称'));
  ok(confirmationProblemNote('OUTSIDE_ADMISSION_SCOPE').includes('区间'));
  ok(confirmationProblemNote('MALFORMED_REQUEST').includes('载荷'));
  ok(confirmationProblemNote('NO_ANSWER_FORMED').includes('授权交付判断'));
  ok(confirmationProblemNote('SOMETHING_NEW').includes('SOMETHING_NEW'));
});

test('交付方参与方：取参与方目录，按标识排；停用与未生效的照列并标出，不替授权交付判断筛', () => {
  const party = (partyId: string, partyName: string, status: string): BusinessPartyRecord => ({
    tenantId: 'SYN-TENANT-01', partyId, partyName, status, revision: 1, basis: 'SYN-BASIS', effectiveFrom: '2026-01-01T00:00:00Z', registeredAt: '2026-01-01T00:00:00Z',
  });
  deepEqual(
    deliveringParticipantOptionsOf([
      party('SYN-PARTY-SHIPPER-01', 'SYN 货主', 'EFFECTIVE'),
      party('SYN-PARTY-DELIVERER-01', 'SYN 交付方', 'EFFECTIVE'),
      party('SYN-PARTY-OLD', 'SYN 旧交付方', 'DEACTIVATED'),
    ]),
    [
      { value: 'SYN-PARTY-DELIVERER-01', label: 'SYN 交付方（SYN-PARTY-DELIVERER-01）' },
      { value: 'SYN-PARTY-OLD', label: 'SYN 旧交付方（SYN-PARTY-OLD）· 已停用' },
      { value: 'SYN-PARTY-SHIPPER-01', label: 'SYN 货主（SYN-PARTY-SHIPPER-01）' },
    ],
  );
});
