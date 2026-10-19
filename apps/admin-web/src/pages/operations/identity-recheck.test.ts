import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import type { IdentityRecheckResponseBody } from './identity-recheck-api';
import {
  describeRecheckAnswer,
  externalMarkCellOf,
  recheckAvailabilityOf,
  recheckProblemNote,
  unidentifiedRowOf,
} from './identity-recheck';
import type { UnidentifiedItemRecord } from './records-api';

// 待识别实物区的外部标识观察列与「重核」动作（票 no-reception-source/03）。最容易错的两处：三种「看不见号」在页面上长成
// 同一片空白；不重核的三格被读成失败、或被读成「已经重核过」。

function pending(overrides: Partial<UnidentifiedItemRecord> = {}): UnidentifiedItemRecord {
  return {
    sourceId: 'SYN-DELIVERY-NRS03',
    unit: 'SYN-UNIT-NRS03',
    node: 'SYN-NODE-NRS03',
    candidates: [],
    identityConflict: false,
    receivedAt: '2026-10-19T08:00:00Z',
    recordedAt: '2026-10-19T08:00:01Z',
    judgmentVersion: 'SYN-INTAKEV-NRS03',
    sourcePreserved: true,
    externalMark: 'SYN-MARK-NRS03',
    ...overrides,
  };
}

test('外部标识观察：来源未保全、设备未报号、号带不可见字符三格各写各的字，不留空白', () => {
  const unpreserved = externalMarkCellOf({ sourcePreserved: false });
  const blank = externalMarkCellOf({ sourcePreserved: true, externalMark: '' });
  const tabbed = externalMarkCellOf({ sourcePreserved: true, externalMark: 'SYN-MARK-NRS03\t' });

  equal(unpreserved, '来源未保全');
  equal(blank, '（设备未报号）');
  equal(tabbed, '"SYN-MARK-NRS03\\t"');
  equal(externalMarkCellOf({ sourcePreserved: true, externalMark: ' SYN-MARK ' }), '" SYN-MARK "');
  equal(externalMarkCellOf({ sourcePreserved: true, externalMark: 'SYN-MARK-NRS03' }), 'SYN-MARK-NRS03');
  for (const cell of [unpreserved, blank, tabbed]) notEqual(cell.trim(), '');
  notEqual(unpreserved, blank);
});

test('重核动作：当前有效、没记冲突、来源已保全的待识别才可发起，带着它的判断版本', () => {
  deepEqual(recheckAvailabilityOf(pending()), { kind: 'available', judgmentVersion: 'SYN-INTAKEV-NRS03' });
});

test('重核动作：记身份冲突、来源未保全、已被接续、没有判断版本各说各的为什么，不给按钮', () => {
  const cases: Array<[Partial<UnidentifiedItemRecord>, string]> = [
    [{ identityConflict: true }, '身份冲突'],
    [{ sourcePreserved: false, externalMark: undefined }, '来源未保全'],
    [{ succeededBy: 'SYN-INTAKEV-NRS03-NEXT' }, 'SYN-INTAKEV-NRS03-NEXT'],
    [{ judgmentVersion: undefined }, '判断版本'],
  ];
  for (const [overrides, expected] of cases) {
    const availability = recheckAvailabilityOf(pending(overrides));
    equal(availability.kind, 'unavailable');
    ok(availability.kind === 'unavailable' && availability.reason.includes(expected), JSON.stringify(availability));
  }
});

test('行转写带外部标识观察列与判断版本，动作随行走', () => {
  const row = unidentifiedRowOf(pending({ sourcePreserved: false, externalMark: undefined }));
  equal(row.values.externalMark, '来源未保全');
  equal(row.values.judgmentVersion, 'SYN-INTAKEV-NRS03');
  equal(row.recheck.kind, 'unavailable');
});

function answer(overrides: Partial<IdentityRecheckResponseBody>): IdentityRecheckResponseBody {
  return { outcome: 'INTAKE_FORMED', recheckedVersion: 'SYN-INTAKEV-NRS03', ...overrides };
}

test('重核成立：落了新版，点明接续哪一版、关联哪件包裹、业务时间沿用原收寄时刻', () => {
  const note = describeRecheckAnswer(
    answer({ judgmentVersion: 'SYN-INTAKEV-NRS03-RECHECK', association: 'SYN-PARCEL-NRS03', serviceResultCheck: 'CHECKED' }),
  );
  equal(note.tone, 'answered');
  equal(note.landed, true);
  ok(note.details.some((line) => line.includes('SYN-INTAKEV-NRS03-RECHECK') && line.includes('SYN-PARCEL-NRS03')));
  ok(note.details.some((line) => line.includes('原收寄时刻')));
});

test('不重核的三格与已有结果、仍查无此号都是形成了的答案，不是失败，也不重读列表', () => {
  for (const outcome of [
    'STILL_UNIDENTIFIED',
    'EXISTING_RESULT',
    'IDENTITY_CONFLICT_RECORDED',
    'SOURCE_NOT_PRESERVED',
    'NOT_PENDING_IDENTIFICATION',
  ]) {
    const note = describeRecheckAnswer(answer({ outcome, currentVersion: 'SYN-INTAKEV-NRS03-NEXT', recordedOutcome: 'INTAKE_FORMED' }));
    equal(note.tone, 'answered', outcome);
    equal(note.landed, false, outcome);
    ok(note.details.length > 0, outcome);
  }
  const existing = describeRecheckAnswer(answer({ outcome: 'EXISTING_RESULT', currentVersion: 'SYN-INTAKEV-NRS03-NEXT', recordedOutcome: 'INTAKE_FORMED' }));
  ok(existing.details[0].includes('SYN-INTAKEV-NRS03-NEXT') && existing.details[0].includes('已形成收寄'), existing.details[0]);
});

test('未收录的 outcome 原样示码，不归进既有说法', () => {
  const note = describeRecheckAnswer(answer({ outcome: 'SOMETHING_NEW' }));
  equal(note.tone, 'refused');
  ok(note.headline.includes('SOMETHING_NEW'));
});

test('问题码：404 指名不到与 403 不在准入范围各有说法', () => {
  ok(recheckProblemNote('JUDGMENT_NOT_VISIBLE').includes('刷新列表'));
  ok(recheckProblemNote('OUTSIDE_ADMISSION_SCOPE').includes('生产权威区间'));
  equal(recheckProblemNote('SOMETHING_ELSE'), '服务端问题码：SOMETHING_ELSE');
});
