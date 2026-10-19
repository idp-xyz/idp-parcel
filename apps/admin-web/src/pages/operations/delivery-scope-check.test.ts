import { test } from 'node:test';
import { equal, notEqual, ok } from 'node:assert/strict';
import {
  deliveryScopeBasisCellOf,
  deliveryScopeCheckCellOf,
  deliveryScopeRecoveryOf,
  pendingCauseOf,
} from './delivery-scope-check';
import { describeRecheckAnswer, unidentifiedRowOf } from './identity-recheck';
import type { UnidentifiedItemRecord } from './records-api';

// 待识别实物页的交付范围核对态一栏（票 ps-external-mark-relations/13；ADR-0212 决定五）。最容易错的是四件事在页面上长成一个样：
// 「缺范围」「范围成立但冲突」「零候选」与「未核对」候选列可以一样、冲突列可以一样，看的人分不出该去补证据、去处置还是等重核。

function pending(overrides: Partial<UnidentifiedItemRecord> = {}): UnidentifiedItemRecord {
  return {
    sourceId: 'SYN-DELIVERY-PSXM13',
    unit: 'SYN-UNIT-PSXM13',
    node: 'SYN-NODE-PSXM13',
    candidates: ['SYN-PARCEL-PSXM13/link-v1'],
    identityConflict: false,
    receivedAt: '2026-10-19T08:00:00Z',
    recordedAt: '2026-10-19T08:00:01Z',
    judgmentVersion: 'SYN-INTAKEV-PSXM13',
    sourcePreserved: true,
    externalMark: 'SYN-MARK-PSXM13',
    identityCheck: 'CHECKED',
    ...overrides,
  };
}

const unverified = (reason: string, inapplicability?: string): Partial<UnidentifiedItemRecord> => ({
  deliveryScopeCheck: 'UNVERIFIED',
  deliveryScopeUnverifiedReason: reason,
  ...(inapplicability ? { deliveryScopeInapplicability: inapplicability } : {}),
});

test('为何待识别：缺范围、范围成立但冲突、零候选与未核对各是一格，彼此分得开', () => {
  const missing = pendingCauseOf(pending(unverified('NO_BASIS')));
  const contested = pendingCauseOf(pending({ identityConflict: true, deliveryScopeCheck: 'VERIFIED', candidates: ['P-1', 'P-2'] }));
  const zero = pendingCauseOf(pending({ candidates: [], ...unverified('NO_BASIS') }));
  const unchecked = pendingCauseOf(pending({ identityCheck: 'UNCHECKED', candidates: [] }));

  equal(missing.cause, 'SCOPE_MISSING');
  equal(contested.cause, 'CONFLICT_WITHIN_SCOPE');
  equal(zero.cause, 'ZERO_CANDIDATES');
  equal(unchecked.cause, 'UNCHECKED');
  ok(missing.label.startsWith('缺范围'), missing.label);
  ok(contested.label.startsWith('范围成立但冲突'), contested.label);
  ok(zero.label.startsWith('零候选'), zero.label);
  ok(unchecked.label.startsWith('未核对'), unchecked.label);
  const labels = [missing, contested, zero, unchecked].map((cause) => cause.label);
  equal(new Set(labels).size, labels.length, labels.join(' | '));
});

test('为何待识别：其余几格——没有范围的冲突、未配置、范围成立却范围内无相容候选、没记核对态的旧行', () => {
  equal(pendingCauseOf(pending({ identityConflict: true, ...unverified('NO_BASIS') })).cause, 'CONFLICT');
  equal(pendingCauseOf(pending({ identityConflict: true })).cause, 'CONFLICT');
  equal(pendingCauseOf(pending({ deliveryScopeCheck: 'UNCONFIGURED' })).cause, 'SCOPE_UNCONFIGURED');
  equal(pendingCauseOf(pending({ deliveryScopeCheck: 'VERIFIED' })).cause, 'NO_COMPATIBLE_CANDIDATE_IN_SCOPE');
  equal(pendingCauseOf(pending()).cause, 'SCOPE_NOT_RECORDED');
  // 没带身份核对态、候选又空的旧行：查没查过不知道，不替它说「零候选」。
  equal(pendingCauseOf(pending({ identityCheck: undefined, candidates: [] })).cause, 'CHECK_NOT_RECORDED');
  // 零候选不看范围：范围成立也照样是零候选（ADR-0212 决定四「一个也没有」）。
  equal(pendingCauseOf(pending({ candidates: [], deliveryScopeCheck: 'VERIFIED' })).cause, 'ZERO_CANDIDATES');
});

test('交付范围核对：已证、未证附原因与细分、未配置、没记、未收录的码各写各的字', () => {
  equal(deliveryScopeCheckCellOf({ deliveryScopeCheck: 'VERIFIED' }), '已证');
  equal(deliveryScopeCheckCellOf(unverified('NO_BASIS')), '未证 · 没有依据');
  equal(deliveryScopeCheckCellOf(unverified('BASIS_NOT_APPLICABLE', 'NOT_A_HANDOVER_MEMBER')), '未证 · 依据不适用（不是交接单成员）');
  equal(deliveryScopeCheckCellOf(unverified('BASIS_NOT_APPLICABLE', 'OUTSIDE_WINDOW')), '未证 · 依据不适用（在时窗外）');
  equal(deliveryScopeCheckCellOf(unverified('BINDINGS_NOT_ALL_CONFIRMED')), '未证 · 交接单绑定未全部确认');
  ok(deliveryScopeCheckCellOf({ deliveryScopeCheck: 'UNCONFIGURED' }).startsWith('未配置'));
  ok(deliveryScopeCheckCellOf({}).startsWith('未记'));
  ok(deliveryScopeCheckCellOf({ deliveryScopeCheck: 'SOMETHING_NEW' }).includes('SOMETHING_NEW'));
  ok(deliveryScopeCheckCellOf(unverified('A_NEW_REASON')).includes('A_NEW_REASON'));
});

test('恢复动作：未配置指向租户登记时窗，未证按原因指向确认绑定或归入交接单，已证与没记不给动作', () => {
  ok(deliveryScopeRecoveryOf({ deliveryScopeCheck: 'UNCONFIGURED' }).includes('预报有效时窗'));
  ok(deliveryScopeRecoveryOf(unverified('BINDINGS_NOT_ALL_CONFIRMED')).includes('确认'));
  for (const reason of ['NO_BASIS', 'NUMBER_NOT_PRE_ADVISED']) {
    ok(deliveryScopeRecoveryOf(unverified(reason)).includes('归入'), reason);
  }
  for (const detail of ['NODE_MISMATCH', 'OUTSIDE_WINDOW', 'WITHDRAWN', 'NOT_A_HANDOVER_MEMBER']) {
    ok(deliveryScopeRecoveryOf(unverified('BASIS_NOT_APPLICABLE', detail)).includes('归入'), detail);
  }
  equal(deliveryScopeRecoveryOf({ deliveryScopeCheck: 'VERIFIED' }), '');
  equal(deliveryScopeRecoveryOf({}), '');
  notEqual(deliveryScopeRecoveryOf({ deliveryScopeCheck: 'UNCONFIGURED' }), deliveryScopeRecoveryOf(unverified('NO_BASIS')));
});

test('依据引用与此刻归入：来源里的依据、来源没带、来源未保全与归入各写各的字', () => {
  equal(
    deliveryScopeBasisCellOf(pending({ deliveryScopeBasis: { kind: 'DELIVERY_PRE_ADVICE', id: 'SYN-PREADV-1' } })),
    '送站预报 SYN-PREADV-1 · 未归入交接单',
  );
  equal(
    deliveryScopeBasisCellOf(pending({ assignedHandoverNote: 'SYN-HN-2' })),
    '来源没带依据 · 此刻归入 SYN-HN-2',
  );
  equal(
    deliveryScopeBasisCellOf(pending({ sourcePreserved: false, externalMark: undefined })),
    '来源未保全 · 未归入交接单',
  );
  equal(
    deliveryScopeBasisCellOf(pending({ deliveryScopeBasis: { kind: 'DELIVERY_HANDOVER_NOTE', id: 'SYN-HN-1' }, assignedHandoverNote: 'SYN-HN-2' })),
    '送站交接单 SYN-HN-1 · 此刻归入 SYN-HN-2',
  );
});

test('待识别行带上三格：为何待识别、交付范围核对（附恢复动作）与依据', () => {
  const row = unidentifiedRowOf(pending({ ...unverified('NO_BASIS'), assignedHandoverNote: 'SYN-HN-2' }));
  ok(row.values.pendingCause.startsWith('缺范围'), row.values.pendingCause);
  ok(row.values.deliveryScope.startsWith('未证 · 没有依据'), row.values.deliveryScope);
  ok(row.values.deliveryScope.includes('归入'), row.values.deliveryScope);
  equal(row.values.deliveryScopeBasis, '来源没带依据 · 此刻归入 SYN-HN-2');
  equal(unidentifiedRowOf(pending({ deliveryScopeCheck: 'VERIFIED', identityConflict: true })).values.deliveryScope, '已证');
});

test('重核答复：只因歧义信号的冲突解开成范围未证的待识别，是追加了一版的答案，带上这一版的核对态', () => {
  const note = describeRecheckAnswer({
    outcome: 'AMBIGUITY_GONE_SCOPE_UNVERIFIED',
    recheckedVersion: 'SYN-INTAKEV-PSXM13',
    judgmentVersion: 'SYN-INTAKEV-PSXM13-NEXT',
    candidates: ['SYN-PARCEL-PSXM13/link-v1'],
    deliveryScopeCheck: 'UNVERIFIED',
    deliveryScopeUnverifiedReason: 'NO_BASIS',
  });
  equal(note.tone, 'answered');
  equal(note.landed, true);
  ok(note.details.some((line) => line.includes('未证 · 没有依据')), note.details.join(' | '));
  const formed = describeRecheckAnswer({
    outcome: 'INTAKE_FORMED',
    recheckedVersion: 'SYN-INTAKEV-PSXM13',
    judgmentVersion: 'SYN-INTAKEV-PSXM13-NEXT',
    association: 'SYN-PARCEL-PSXM13/link-v1',
    deliveryScopeCheck: 'VERIFIED',
  });
  ok(formed.details.some((line) => line.includes('交付范围核对：已证')), formed.details.join(' | '));
});
