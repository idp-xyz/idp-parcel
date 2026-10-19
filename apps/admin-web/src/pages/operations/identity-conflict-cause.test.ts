import { test } from 'node:test';
import { equal, notEqual, ok } from 'node:assert/strict';
import { conflictReleasableByRecheck, identityConflictCauseCellOf } from './identity-conflict-cause';
import { describeRecheckAnswer, recheckAvailabilityOf, unidentifiedRowOf } from './identity-recheck';
import type { UnidentifiedItemRecord } from './records-api';

// 身份冲突的来由在复核页上的三处（票 no-reception-identity/07；ADR-0211）：冲突来由一列、待识别行的「重核」动作、重核答复。
// 最容易错的是：只因歧义信号的冲突被当成「只经处置」把重核按钮置灰；或者反过来，多候选、一码两物与来由未记的旧冲突行被放开
// 重核；以及仍带信号的那一格答复被读成「重核失败」或「查无此号」。

function contested(overrides: Partial<UnidentifiedItemRecord> = {}): UnidentifiedItemRecord {
  return {
    sourceId: 'SYN-DELIVERY-NRI07',
    unit: 'SYN-UNIT-NRI07',
    node: 'SYN-NODE-NRI07',
    candidates: ['SYN-PARCEL-NRI07'],
    identityConflict: true,
    identityConflictCause: 'AMBIGUITY_ONLY',
    receivedAt: '2026-10-19T08:00:00Z',
    recordedAt: '2026-10-19T08:00:01Z',
    judgmentVersion: 'SYN-INTAKEV-NRI07',
    sourcePreserved: true,
    externalMark: 'SYN-MARK-NRI07',
    identityCheck: 'CHECKED',
    ...overrides,
  };
}

test('冲突来由一列：三值各一句、来由缺席的冲突行如实写来由未记、不记冲突写无冲突、未收录的码原样示出', () => {
  const ambiguous = identityConflictCauseCellOf(contested());
  const many = identityConflictCauseCellOf(contested({ identityConflictCause: 'MULTIPLE_CANDIDATES' }));
  const twoUnits = identityConflictCauseCellOf(contested({ identityConflictCause: 'ONE_MARK_ON_TWO_UNITS' }));
  const unrecorded = identityConflictCauseCellOf(contested({ identityConflictCause: undefined }));

  ok(ambiguous.startsWith('只因歧义信号'), ambiguous);
  ok(ambiguous.includes('可再重核'), ambiguous);
  ok(many.startsWith('多候选') && many.includes('处置'), many);
  ok(twoUnits.startsWith('一码两物') && twoUnits.includes('处置'), twoUnits);
  ok(unrecorded.startsWith('来由未记') && unrecorded.includes('处置'), unrecorded);
  equal(identityConflictCauseCellOf(contested({ identityConflict: false, identityConflictCause: undefined })), '无冲突');
  ok(identityConflictCauseCellOf(contested({ identityConflictCause: 'SOMETHING_NEW' })).includes('SOMETHING_NEW'));
  for (const [left, right] of [
    [ambiguous, many],
    [many, twoUnits],
    [twoUnits, unrecorded],
  ]) {
    notEqual(left, right);
  }
});

test('只有来由为只因歧义信号的冲突行算可经重核解开；其余来由、来由未记与未收录的码都不算', () => {
  equal(conflictReleasableByRecheck(contested()), true);
  for (const cause of ['MULTIPLE_CANDIDATES', 'ONE_MARK_ON_TWO_UNITS', undefined, 'SOMETHING_NEW']) {
    equal(conflictReleasableByRecheck(contested({ identityConflictCause: cause })), false, String(cause));
  }
  equal(conflictReleasableByRecheck(contested({ identityConflict: false, identityConflictCause: undefined })), false);
});

test('重核动作：只因歧义信号的冲突行放开，带着它的判断版本；其余来由的冲突行不给按钮，理由指向处置', () => {
  const opened = recheckAvailabilityOf(contested());
  equal(opened.kind, 'available');
  ok(opened.kind === 'available' && opened.judgmentVersion === 'SYN-INTAKEV-NRI07');

  for (const cause of ['MULTIPLE_CANDIDATES', 'ONE_MARK_ON_TWO_UNITS', undefined]) {
    const closed = recheckAvailabilityOf(contested({ identityConflictCause: cause }));
    equal(closed.kind, 'unavailable', String(cause));
    ok(closed.kind === 'unavailable' && closed.reason.includes('处置') && closed.reason.includes('身份冲突'), JSON.stringify(closed));
  }
  // 只因歧义信号的冲突行也照旧过其余几道：已被接续的不是当前有效版本。
  equal(recheckAvailabilityOf(contested({ succeededBy: 'SYN-INTAKEV-NRI07-NEXT' })).kind, 'unavailable');
});

test('行转写带冲突来由一列，动作随来由走', () => {
  const ambiguous = unidentifiedRowOf(contested());
  const many = unidentifiedRowOf(contested({ identityConflictCause: 'MULTIPLE_CANDIDATES' }));
  ok(ambiguous.values.identityConflictCause.startsWith('只因歧义信号'));
  equal(ambiguous.recheck.kind, 'available');
  ok(many.values.identityConflictCause.startsWith('多候选'));
  equal(many.recheck.kind, 'unavailable');
});

test('重核答复仍带歧义信号：形成了的答案、没追加、不重读列表；说清是有未决的同号声明暂不能关联，不说成查无此号或失败', () => {
  const note = describeRecheckAnswer({
    outcome: 'STILL_AMBIGUOUS',
    recheckedVersion: 'SYN-INTAKEV-NRI07',
    candidates: ['SYN-PARCEL-NRI07'],
    identityConflict: true,
    identityConflictCause: 'AMBIGUITY_ONLY',
  });
  equal(note.tone, 'answered');
  equal(note.landed, false);
  ok(note.headline.includes('STILL_AMBIGUOUS') && note.headline.includes('同号声明'), note.headline);
  ok(!note.headline.includes('查无此号'), note.headline);
  ok(note.details.some((line) => line.includes('SYN-INTAKEV-NRI07') && line.includes('当前有效版本')), JSON.stringify(note.details));
  ok(note.details.some((line) => line.includes('拒绝或撤回')), JSON.stringify(note.details));
});

test('重核答复记着身份冲突与追加了冲突版：说的话随来由走——只因歧义信号的那一格不说「只经处置」', () => {
  const recorded = describeRecheckAnswer({
    outcome: 'IDENTITY_CONFLICT_RECORDED',
    recheckedVersion: 'SYN-INTAKEV-NRI07',
    identityConflict: true,
    identityConflictCause: 'ONE_MARK_ON_TWO_UNITS',
  });
  ok(recorded.details.some((line) => line.includes('一码两物') && line.includes('处置')), JSON.stringify(recorded.details));

  const appendedAmbiguous = describeRecheckAnswer({
    outcome: 'IDENTITY_CONFLICT',
    recheckedVersion: 'SYN-INTAKEV-NRI07',
    judgmentVersion: 'SYN-INTAKEV-NRI07-CONFLICT',
    candidates: ['SYN-PARCEL-NRI07'],
    identityConflict: true,
    identityConflictCause: 'AMBIGUITY_ONLY',
  });
  ok(appendedAmbiguous.details.some((line) => line.includes('只因歧义信号') && line.includes('拒绝或撤回')), JSON.stringify(appendedAmbiguous.details));
  ok(!appendedAmbiguous.details.some((line) => line.includes('再重核不会解开')), JSON.stringify(appendedAmbiguous.details));

  const appendedMany = describeRecheckAnswer({
    outcome: 'IDENTITY_CONFLICT',
    recheckedVersion: 'SYN-INTAKEV-NRI07',
    judgmentVersion: 'SYN-INTAKEV-NRI07-CONFLICT',
    candidates: ['SYN-PARCEL-NRI07-A', 'SYN-PARCEL-NRI07-B'],
    identityConflict: true,
    identityConflictCause: 'MULTIPLE_CANDIDATES',
  });
  ok(appendedMany.details.some((line) => line.includes('多候选') && line.includes('处置')), JSON.stringify(appendedMany.details));
});
