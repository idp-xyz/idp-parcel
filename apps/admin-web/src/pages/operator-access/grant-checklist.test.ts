import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { planGrantChecklist } from './grant-checklist';
import type { OperatorGrantRecord } from './api';

const now = new Date('2026-11-27T08:00:00Z');
const draft = {
  prefix: 'SYN-G',
  basis: 'SYN-BASIS',
  effectiveStartsAt: '2026-11-27T00:00:00Z',
  issuer: 'http://iss',
  subject: 'sub',
};

function grant(partial: Partial<OperatorGrantRecord> & Pick<OperatorGrantRecord, 'grantId' | 'capabilityFace'>): OperatorGrantRecord {
  return {
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    basis: 'SYN-OLD',
    recordedAt: '2026-01-01T00:00:00Z',
    ...partial,
  };
}

test('不带细分的面各成一笔，标识是前缀加原词', () => {
  const plan = planGrantChecklist(
    { faces: ['REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ'], decisionKinds: [], workScopes: [] },
    draft,
    [],
    now,
  );
  equal(plan.problem, null);
  deepEqual(
    plan.payloads.map((payload) => payload.grantId),
    ['SYN-G-REGISTRY_CONFIGURATION_WRITE', 'SYN-G-MASTER_DATA_AND_OPERATIONS_READ'],
  );
  equal(plan.payloads[0].basis, 'SYN-BASIS');
  equal(plan.payloads[0].decisionKind, undefined);
});

test('运营决定与作业范围带上细分，不带细分的那两格被丢掉', () => {
  const plan = planGrantChecklist(
    {
      faces: ['OPERATION_DECISION', 'WORK_FACT_REGISTRATION'],
      decisionKinds: ['WAVE_RELEASE'],
      workScopes: [{ kind: 'NODE', reference: 'SYN-NODE-SHA-HUB' }],
    },
    draft,
    [],
    now,
  );
  equal(plan.problem, null);
  deepEqual(
    plan.payloads.map((payload) => [payload.capabilityFace, payload.decisionKind, payload.workScope?.reference, payload.grantId]),
    [
      ['OPERATION_DECISION', 'WAVE_RELEASE', undefined, 'SYN-G-WAVE_RELEASE'],
      ['WORK_FACT_REGISTRATION', undefined, 'SYN-NODE-SHA-HUB', 'SYN-G-NODE-SYN-NODE-SHA-HUB'],
    ],
  );
});

test('已经生效或未到的同一格跳过，已撤销的可以再授', () => {
  const plan = planGrantChecklist(
    { faces: ['REGISTRY_CONFIGURATION_WRITE'], decisionKinds: ['WAVE_RELEASE', 'SEGMENT_CLOSURE'], workScopes: [] },
    draft,
    [
      grant({ grantId: 'OLD-READ', capabilityFace: 'REGISTRY_CONFIGURATION_WRITE' }),
      grant({
        grantId: 'OLD-FUTURE',
        capabilityFace: 'OPERATION_DECISION',
        decisionKind: 'WAVE_RELEASE',
        effectiveStartsAt: '2027-01-01T00:00:00Z',
      }),
      grant({
        grantId: 'OLD-REVOKED',
        capabilityFace: 'OPERATION_DECISION',
        decisionKind: 'SEGMENT_CLOSURE',
        revocation: { revokedAt: '2026-06-01T00:00:00Z', basis: 'SYN-REVOKE' },
      }),
    ],
    now,
  );
  equal(plan.problem, null);
  equal(plan.skipped, 2);
  deepEqual(
    plan.payloads.map((payload) => payload.grantId),
    ['SYN-G-SEGMENT_CLOSURE'],
  );
});

test('前缀、依据、起点缺了就不生成', () => {
  equal(planGrantChecklist({ faces: ['REGISTRY_CONFIGURATION_WRITE'], decisionKinds: [], workScopes: [] }, { ...draft, prefix: '  ' }, [], now).problem, '授予标识前缀、依据和生效起点都要填。');
  equal(planGrantChecklist({ faces: [], decisionKinds: [], workScopes: [] }, draft, [], now).problem, '还没有勾选要授的。');
  equal(
    planGrantChecklist({ faces: [], decisionKinds: [], workScopes: [{ kind: 'NODE', reference: '  ' }] }, draft, [], now).problem,
    '作业范围的种类和引用都要填。',
  );
});

test('生成的标识与已有标识相撞时不提交', () => {
  const plan = planGrantChecklist(
    { faces: ['REGISTRY_CONFIGURATION_WRITE'], decisionKinds: [], workScopes: [] },
    draft,
    [grant({ grantId: 'SYN-G-REGISTRY_CONFIGURATION_WRITE', capabilityFace: 'MASTER_DATA_AND_OPERATIONS_READ', revocation: { revokedAt: '2026-01-02T00:00:00Z', basis: 'SYN-REVOKE' } })],
    now,
  );
  equal(plan.payloads.length, 0);
  equal(plan.problem, '授予标识 SYN-G-REGISTRY_CONFIGURATION_WRITE 已经用过，换一个前缀。');
});
