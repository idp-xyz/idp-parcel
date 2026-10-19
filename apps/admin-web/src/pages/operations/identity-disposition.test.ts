import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { IdentityDispositionResponseBody } from './identity-disposition-api';
import {
  coDisposedUnitsFromEcho,
  describeDispositionAnswer,
  dispositionAvailabilityOf,
  dispositionProblemNote,
  dispositionRequestOf,
  identityConflictStateOf,
  identityReviewRowOf,
} from './identity-disposition';
import type { IdentityDispositionRecord, IdentityReviewHead, IdentityReviewUnit } from './records-api';

// 待识别实物页的「处置」动作（票 no-reception-identity/06；UC-NO-004）。最容易错的几处：一件实物几条链时冲突与处置候选从哪几版来；
// 「关联对象另有归属」的回显怎么带出须连带的实物；没落版的六格被读成失败；已处置的实物还给一个「处置」按钮。

function head(overrides: Partial<IdentityReviewHead> = {}): IdentityReviewHead {
  return {
    sourceId: 'SYN-DELIVERY-NRI06-B',
    kind: 'PENDING_IDENTIFICATION',
    node: 'SYN-NODE-NRI06',
    judgmentVersion: 'SYN-V-B1',
    candidates: ['SYN-PARCEL-NRI06-X'],
    identityConflict: true,
    receivedAt: '2026-10-19T08:00:00Z',
    recordedAt: '2026-10-19T08:00:01Z',
    ...overrides,
  };
}

function unitOf(...heads: IdentityReviewHead[]): IdentityReviewUnit {
  return { unit: 'SYN-UNIT-NRI06-B', heads };
}

const designated: IdentityDispositionRecord = {
  disposedVersion: 'SYN-V-B1',
  unit: 'SYN-UNIT-NRI06-B',
  result: 'DESIGNATED',
  parcel: 'SYN-PARCEL-NRI06-X',
  coDisposedUnits: [{ unit: 'SYN-UNIT-NRI06-A', version: 'SYN-V-A2' }],
  basis: 'SYN-DISPOSITION-BASIS-NRI06',
  operator: 'OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01',
  disposedAt: '2026-10-19T09:00:00Z',
};

test('冲突照服务端同一读法判：一版记冲突，或带关联的链头指向不同的包裹；处置候选是各链头的候选与关联合起来', () => {
  const twoChains = unitOf(
    head({ judgmentVersion: 'SYN-V-M1', candidates: ['SYN-PARCEL-NRI06-Z', 'SYN-PARCEL-NRI06-Y'] }),
    head({ judgmentVersion: 'SYN-V-M2', candidates: [], identityConflict: false, kind: 'INTAKE_FORMED', association: 'SYN-PARCEL-NRI06-W' }),
  );
  const state = identityConflictStateOf(twoChains);
  equal(state.conflicted, true);
  deepEqual(state.candidates, ['SYN-PARCEL-NRI06-W', 'SYN-PARCEL-NRI06-Y', 'SYN-PARCEL-NRI06-Z']);

  const split = unitOf(
    head({ judgmentVersion: 'SYN-V-1', kind: 'INTAKE_FORMED', association: 'SYN-PARCEL-NRI06-X', candidates: [], identityConflict: false }),
    head({ judgmentVersion: 'SYN-V-2', kind: 'INTAKE_FORMED', association: 'SYN-PARCEL-NRI06-Y', candidates: [], identityConflict: false }),
  );
  equal(identityConflictStateOf(split).conflicted, true);
  equal(identityConflictStateOf(unitOf(head({ identityConflict: false, candidates: [] }))).conflicted, false);
});

test('处置动作：冲突中的实物可发起，默认指名记冲突的那一版，带各链版本与处置候选', () => {
  const availability = dispositionAvailabilityOf(
    unitOf(head({ judgmentVersion: 'SYN-V-OTHER', identityConflict: false, candidates: [] }), head()),
  );
  deepEqual(availability, {
    kind: 'available',
    disposedVersion: 'SYN-V-B1',
    versions: ['SYN-V-OTHER', 'SYN-V-B1'],
    candidates: ['SYN-PARCEL-NRI06-X'],
  });
});

test('处置动作：已处置、从没冲突的实物各说各的为什么，不给按钮；行上写出处置了什么', () => {
  const disposedUnit = unitOf(
    head({ judgmentVersion: 'SYN-V-B2', kind: 'INTAKE_FORMED', association: 'SYN-PARCEL-NRI06-X', candidates: [], identityConflict: false, disposition: designated }),
  );
  const availability = dispositionAvailabilityOf(disposedUnit);
  ok(availability.kind === 'unavailable' && availability.reason.includes('已处置'), JSON.stringify(availability));
  const row = identityReviewRowOf(disposedUnit);
  equal(row.values.status, '冲突已处置');
  for (const fragment of ['指定 SYN-PARCEL-NRI06-X', 'SYN-UNIT-NRI06-A', 'OPERATOR/', 'SYN-DISPOSITION-BASIS-NRI06']) {
    ok(row.values.disposition.includes(fragment), `${fragment} 不在 ${row.values.disposition}`);
  }
  ok(row.values.heads.includes('处置版'));

  const quiet = dispositionAvailabilityOf(unitOf(head({ identityConflict: false, candidates: [] })));
  ok(quiet.kind === 'unavailable' && quiet.reason.includes('无可处置的冲突'), JSON.stringify(quiet));
});

test('载荷：选一件候选拼 DESIGNATED 带包裹与连带实物，选都不是拼 NONE_OF_CANDIDATES、不带包裹；依据空白不发', () => {
  const draft = {
    unit: 'SYN-UNIT-NRI06-B',
    disposedVersion: 'SYN-V-B1',
    parcel: 'SYN-PARCEL-NRI06-X',
    basis: 'SYN-BASIS',
    coDisposedUnits: [{ unit: 'SYN-UNIT-NRI06-A', version: 'SYN-V-A2' }],
  };
  deepEqual(dispositionRequestOf(draft), {
    kind: 'ready',
    request: {
      unit: 'SYN-UNIT-NRI06-B',
      disposedVersion: 'SYN-V-B1',
      result: 'DESIGNATED',
      parcel: 'SYN-PARCEL-NRI06-X',
      basis: 'SYN-BASIS',
      coDisposedUnits: [{ unit: 'SYN-UNIT-NRI06-A', version: 'SYN-V-A2' }],
    },
  });
  const none = dispositionRequestOf({ ...draft, parcel: '' });
  ok(none.kind === 'ready' && none.request.result === 'NONE_OF_CANDIDATES' && none.request.parcel === undefined && none.request.coDisposedUnits === undefined);
  const designatedAlone = dispositionRequestOf({ ...draft, coDisposedUnits: [] });
  ok(designatedAlone.kind === 'ready' && !('coDisposedUnits' in designatedAlone.request));
  equal(dispositionRequestOf({ ...draft, basis: '  ' }).kind, 'incomplete');
  for (const key of ['tenantId', 'operator', 'disposedAt']) {
    ok(none.kind === 'ready' && !(key in none.request), `载荷带了 ${key}`);
  }
});

test('关联对象另有归属：回显带出须连带的实物与它此刻的版本；那件还牵连别的包裹时不给一并提交', () => {
  const ready = coDisposedUnitsFromEcho('SYN-PARCEL-NRI06-X', [
    { unit: 'SYN-UNIT-NRI06-A', currentVersions: [{ unit: 'SYN-UNIT-NRI06-A', judgmentVersion: 'SYN-V-A2', kind: 'PENDING_IDENTIFICATION', candidates: ['SYN-PARCEL-NRI06-X'], identityConflict: true }] },
  ]);
  deepEqual(ready, { kind: 'ready', units: [{ unit: 'SYN-UNIT-NRI06-A', version: 'SYN-V-A2' }] });
  const blocked = coDisposedUnitsFromEcho('SYN-PARCEL-NRI06-X', [
    {
      unit: 'SYN-UNIT-NRI06-A',
      currentVersions: [
        { unit: 'SYN-UNIT-NRI06-A', judgmentVersion: 'SYN-V-A2', kind: 'PENDING_IDENTIFICATION', candidates: ['SYN-PARCEL-NRI06-X'], identityConflict: true },
        { unit: 'SYN-UNIT-NRI06-A', judgmentVersion: 'SYN-V-A9', kind: 'PENDING_IDENTIFICATION', candidates: ['SYN-PARCEL-NRI06-Y'], identityConflict: true },
      ],
    },
  ]);
  ok(blocked.kind === 'blocked' && blocked.reason.includes('SYN-UNIT-NRI06-A'), JSON.stringify(blocked));
});

function answer(overrides: Partial<IdentityDispositionResponseBody>): IdentityDispositionResponseBody {
  return { outcome: 'IDENTITY_CONFLICT_DISPOSED', unit: 'SYN-UNIT-NRI06-B', disposedVersion: 'SYN-V-B1', ...overrides };
}

test('首次形成：落了版，点明追加的各版、意图交给了哪一版、原判断与冲突版照留', () => {
  const note = describeDispositionAnswer(
    answer({
      disposition: designated,
      versions: [
        { unit: 'SYN-UNIT-NRI06-B', judgmentVersion: 'SYN-V-B2', predecessor: 'SYN-V-B1', kind: 'INTAKE_FORMED', association: 'SYN-PARCEL-NRI06-X' },
        { unit: 'SYN-UNIT-NRI06-A', judgmentVersion: 'SYN-V-A3', predecessor: 'SYN-V-A2', kind: 'PENDING_IDENTIFICATION', excluded: ['SYN-PARCEL-NRI06-X'] },
      ],
      intentVersion: 'SYN-V-B2',
    }),
  );
  equal(note.tone, 'answered');
  equal(note.landed, true);
  ok(note.details.some((line) => line.includes('SYN-V-A3') && line.includes('排除 SYN-PARCEL-NRI06-X')));
  ok(note.details.some((line) => line.includes('SYN-V-B2') && line.includes('parcel-shipment')));
  ok(note.details.some((line) => line.includes('照留')));
});

test('没落版的六格都是形成了的答案，不是失败，也不重读列表', () => {
  for (const outcome of [
    'EXISTING_RESULT',
    'ALREADY_DISPOSED',
    'VERSION_SUPERSEDED',
    'NO_IDENTITY_CONFLICT',
    'NOT_A_CANDIDATE',
    'ASSOCIATED_ELSEWHERE',
  ]) {
    const note = describeDispositionAnswer(
      answer({
        outcome,
        disposition: designated,
        currentVersions: [{ unit: 'SYN-UNIT-NRI06-B', judgmentVersion: 'SYN-V-B9', kind: 'PENDING_IDENTIFICATION' }],
        candidates: ['SYN-PARCEL-NRI06-X'],
        associatedUnits: [{ unit: 'SYN-UNIT-NRI06-A', currentVersions: [{ unit: 'SYN-UNIT-NRI06-A', judgmentVersion: 'SYN-V-A2', kind: 'PENDING_IDENTIFICATION' }] }],
      }),
    );
    equal(note.tone, 'answered', outcome);
    equal(note.landed, false, outcome);
    ok(note.details.length > 0, outcome);
  }
  const unknown = describeDispositionAnswer(answer({ outcome: 'SOMETHING_NEW' }));
  equal(unknown.tone, 'refused');
  ok(unknown.headline.includes('SOMETHING_NEW'));
});

test('问题码：404 指名不到与 403 不在准入范围各有说法', () => {
  ok(dispositionProblemNote('JUDGMENT_NOT_VISIBLE').includes('刷新列表'));
  ok(dispositionProblemNote('OUTSIDE_ADMISSION_SCOPE').includes('生产权威区间'));
  equal(dispositionProblemNote('SOMETHING_ELSE'), '服务端问题码：SOMETHING_ELSE');
});
