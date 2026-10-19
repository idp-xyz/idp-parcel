import { test } from 'node:test';
import { equal, notEqual, ok } from 'node:assert/strict';
import { identityCheckCellOf } from './identity-check';
import { describeRecheckAnswer, unidentifiedRowOf } from './identity-recheck';
import type { ReceptionRecord, UnidentifiedItemRecord } from './records-api';
import { receptionRowOf } from './reception-rows';

// 身份核对态在管理台上的两处（票 no-reception-source/05；ADR-0199 决定五）。最容易错的是：未核对与零候选的已核对两条待识别
// 候选同为空，页面不写核对态就把「没问到」读成「查无此号」。

function pending(overrides: Partial<UnidentifiedItemRecord> = {}): UnidentifiedItemRecord {
  return {
    sourceId: 'SYN-DELIVERY-NRS05',
    unit: 'SYN-UNIT-NRS05',
    node: 'SYN-NODE-NRS05',
    candidates: [],
    identityConflict: false,
    receivedAt: '2026-10-19T08:00:00Z',
    recordedAt: '2026-10-19T08:00:01Z',
    judgmentVersion: 'SYN-INTAKEV-NRS05',
    sourcePreserved: true,
    externalMark: 'SYN-MARK-NRS05',
    ...overrides,
  };
}

function pendingReception(identityCheck?: string): ReceptionRecord {
  return {
    sourceId: 'SYN-DELIVERY-NRS05',
    kind: 'PENDING_IDENTIFICATION',
    unit: 'SYN-UNIT-NRS05',
    node: 'SYN-NODE-NRS05',
    deliveredBy: 'SYN-PARTY-NRS05',
    receivedAt: '2026-10-19T08:00:00Z',
    controlKind: 'NODE_INTAKE',
    controlEstablishedAt: '2026-10-19T08:00:00Z',
    serviceMarkers: [],
    recordedAt: '2026-10-19T08:00:01Z',
    ...(identityCheck === undefined ? {} : { identityCheck }),
  };
}

test('身份核对一格：未核对、已核对、未带核对态、未收录的码各写各的字，未核对写明候选缺席不代表查无此号', () => {
  const unchecked = identityCheckCellOf({ identityCheck: 'UNCHECKED' });
  const checked = identityCheckCellOf({ identityCheck: 'CHECKED' });
  equal(unchecked, '未核对：候选缺席不代表查无此号');
  equal(checked, '已核对');
  equal(identityCheckCellOf({}), '未带核对态：查没查过不知道');
  equal(identityCheckCellOf({ identityCheck: 'MAYBE' }), '核对态 MAYBE（页面未收录）');
  notEqual(unchecked, checked);
});

test('待识别实物区：两条候选同为空的待识别，核对态一列把未核对与查无此号分开', () => {
  const unchecked = unidentifiedRowOf(pending({ identityCheck: 'UNCHECKED' }));
  const checked = unidentifiedRowOf(pending({ identityCheck: 'CHECKED' }));
  equal(unchecked.values.candidates, checked.values.candidates);
  equal(unchecked.values.identityCheck, '未核对：候选缺席不代表查无此号');
  equal(checked.values.identityCheck, '已核对');
  // 未核对那一件正是重核要续上的：动作照给。
  equal(unchecked.recheck.kind, 'available');
});

test('节点收寄区：待识别行的种类一格带上未核对，已核对与不带核对态的照旧只写待识别', () => {
  equal(receptionRowOf(pendingReception('UNCHECKED')).values.kind, '待识别 · 身份未核对');
  equal(receptionRowOf(pendingReception('CHECKED')).values.kind, '待识别');
  equal(receptionRowOf(pendingReception()).values.kind, '待识别');
});

test('重核把未核对那一件问到了、仍查无此号：追加了一版待识别 · 已核对，要重读列表', () => {
  const note = describeRecheckAnswer({
    outcome: 'IDENTITY_NOW_CHECKED',
    recheckedVersion: 'SYN-INTAKEV-NRS05',
    judgmentVersion: 'SYN-INTAKEV-NRS05-CHECKED',
    identityCheck: 'CHECKED',
  });
  equal(note.tone, 'answered');
  equal(note.landed, true);
  ok(note.details.some((line) => line.includes('SYN-INTAKEV-NRS05-CHECKED') && line.includes('SYN-INTAKEV-NRS05')));
  ok(note.headline.includes('IDENTITY_NOW_CHECKED'));
});
