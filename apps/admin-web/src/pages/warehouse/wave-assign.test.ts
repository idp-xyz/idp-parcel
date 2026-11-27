import { deepEqual, equal } from 'node:assert/strict';
import { test } from 'node:test';
import { assigneeOptions, waveAssignmentPayload, type AssignableOperator } from './wave-assign';

const now = new Date('2026-11-27T02:00:00Z');
const issuer = 'https://issuer.example.invalid';

const grant = (more: Partial<AssignableOperator['grants'][number]> = {}): AssignableOperator['grants'][number] => ({
  capabilityFace: 'WORK_FACT_REGISTRATION',
  workScope: { kind: 'WAREHOUSE', reference: 'WH-1' },
  effectiveStartsAt: '2026-01-01T00:00:00Z',
  ...more,
});

const operator = (subject: string, grants: AssignableOperator['grants']): AssignableOperator => ({ issuer, subject, grants });

// Covers: 可选的拣货人是操作者册上的人，标识是「发行方#sub」；此刻在这个仓有有效的作业事实登记授予的排在前面，
// 没有的照列但写明派了也拣不了——别的仓的授予、别的能力面、已撤销、还没生效、已到期的都不算。
test('可选的拣货人：在这个仓有作业授予的在前，没有的写明', () => {
  const options = assigneeOptions(
    [
      operator('zoe', [grant()]),
      operator('other-warehouse', [grant({ workScope: { kind: 'WAREHOUSE', reference: 'WH-2' } })]),
      operator('decision-only', [grant({ capabilityFace: 'OPERATION_DECISION', workScope: undefined })]),
      operator('revoked', [grant({ revocation: { revokedAt: '2026-06-01T00:00:00Z' } })]),
      operator('not-yet', [grant({ effectiveStartsAt: '2027-01-01T00:00:00Z' })]),
      operator('expired', [grant({ effectiveEndsAt: '2026-06-01T00:00:00Z' })]),
      operator('amy', [grant({ effectiveEndsAt: '2027-01-01T00:00:00Z' })]),
    ],
    'WH-1',
    now,
  );
  deepEqual(
    options.filter((option) => option.granted).map((option) => [option.value, option.label]),
    [
      [`${issuer}#amy`, 'amy'],
      [`${issuer}#zoe`, 'zoe'],
    ],
  );
  deepEqual(
    options.filter((option) => !option.granted).map((option) => option.value),
    ['decision-only', 'expired', 'not-yet', 'other-warehouse', 'revoked'].map((subject) => `${issuer}#${subject}`),
  );
  equal(options.find((option) => option.value.endsWith('#revoked'))?.label, 'revoked（在这个仓没有作业授予，派了也拣不了）');
  // 登记时写的依据摆在 sub 后面帮人认，太长就截
  const named = assigneeOptions([{ ...operator('sub-1', [grant()]), basis: '早班拣货员 小王' }, { ...operator('sub-2', [grant()]), basis: '一'.repeat(30) }], 'WH-1', now);
  equal(named[0].label, 'sub-1 · 早班拣货员 小王');
  equal(named[1].label, `sub-2 · ${'一'.repeat(24)}…`);
});

// Covers: 指派的报文：选了人带上他，不派不带 assignee（不带才是取消指派）。
test('指派的报文', () => {
  deepEqual(waveAssignmentPayload('W-1', `${issuer}#amy`), { waveId: 'W-1', assignee: `${issuer}#amy` });
  deepEqual(waveAssignmentPayload('W-1', ''), { waveId: 'W-1' });
  deepEqual(waveAssignmentPayload('W-1', '   '), { waveId: 'W-1' });
});
