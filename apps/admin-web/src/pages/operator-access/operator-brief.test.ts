import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { OperatorGrantRecord, OperatorRecord } from './api';
import { decisionBundles, operatorBrief, unbundledDecisionKinds } from './operator-brief';

const now = new Date('2026-11-27T12:00:00Z');

function grant(over: Partial<OperatorGrantRecord> & Pick<OperatorGrantRecord, 'grantId' | 'capabilityFace'>): OperatorGrantRecord {
  return {
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    basis: 'B',
    recordedAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

function person(grants: OperatorGrantRecord[]): OperatorRecord {
  return { issuer: 'http://iss', subject: 'sub', basis: 'B', recordedAt: '2026-01-01T00:00:00Z', grants };
}

const read = grant({ grantId: 'READ', capabilityFace: 'MASTER_DATA_AND_OPERATIONS_READ' });

test('五组互不重叠，组外的种类还能找回来', () => {
  const seen = new Set<string>();
  for (const bundle of decisionBundles) {
    for (const kind of bundle.kinds) {
      equal(seen.has(kind), false, kind);
      seen.add(kind);
    }
  }
  equal(decisionBundles.reduce((sum, bundle) => sum + bundle.kinds.length, 0), 71);
  deepEqual(unbundledDecisionKinds(['MANUAL_REVIEW_COMPLETION', 'CANCELLATION_ORDER_CONFLICT_RESOLUTION']), ['CANCELLATION_ORDER_CONFLICT_RESOLUTION']);
});

test('只读、管人、客服受理和看得见的范围按生效授予说', () => {
  equal(operatorBrief(person([read]), now).duty, '只读');
  equal(operatorBrief(person([read]), now).visibility, '看不见业务数据');
  const admin = operatorBrief(
    person([read, grant({ grantId: 'ADMIN', capabilityFace: 'OPERATOR_ACCESS_ADMINISTRATION' })]),
    now,
  );
  equal(admin.duty, '管人');
  equal(admin.visibility, '看不见业务数据');
  const acceptance = decisionBundles[0].kinds;
  const suwan = operatorBrief(
    person([
      read,
      ...acceptance.map((kind) => grant({ grantId: kind, capabilityFace: 'OPERATION_DECISION', decisionKind: kind })),
      grant({
        grantId: 'VIS',
        capabilityFace: 'DATA_VISIBILITY',
        dataVisibility: { kind: 'CUSTOMER_ACCOUNT', reference: 'IDP-ACCOUNT-QINGHE' },
      }),
    ]),
    now,
  );
  equal(suwan.duty, '客服受理');
  equal(suwan.visibility, '客户只有 IDP-ACCOUNT-QINGHE · 仓没有');
  const partial = operatorBrief(
    person([
      read,
      grant({ grantId: 'D1', capabilityFace: 'OPERATION_DECISION', decisionKind: acceptance[0] }),
      grant({ grantId: 'D2', capabilityFace: 'OPERATION_DECISION', decisionKind: acceptance[1] }),
    ]),
    now,
  );
  equal(partial.duty, '客服受理 2/13');
  const viewer = operatorBrief(
    person([
      read,
      grant({ grantId: 'C', capabilityFace: 'DATA_VISIBILITY', dataVisibility: { kind: 'CUSTOMER_ACCOUNT', reference: '*' } }),
      grant({ grantId: 'W', capabilityFace: 'DATA_VISIBILITY', dataVisibility: { kind: 'WAREHOUSE', reference: '*' } }),
    ]),
    now,
  );
  equal(viewer.duty, '只读');
  equal(viewer.visibility, '客户全部 · 仓全部');
  const revoked = operatorBrief(
    person([
      read,
      grant({
        grantId: 'OLD',
        capabilityFace: 'OPERATION_DECISION',
        decisionKind: acceptance[0],
        revocation: { revokedAt: '2026-11-01T00:00:00Z', basis: 'R' },
      }),
    ]),
    now,
  );
  equal(revoked.duty, '只读');
  const manySites = operatorBrief(
    person([
      read,
      grant({ grantId: 'W1', capabilityFace: 'WORK_FACT_REGISTRATION', workScope: { kind: 'NODE', reference: 'A' } }),
      grant({ grantId: 'W2', capabilityFace: 'WORK_FACT_REGISTRATION', workScope: { kind: 'NODE', reference: 'B' } }),
      grant({ grantId: 'W3', capabilityFace: 'WORK_FACT_REGISTRATION', workScope: { kind: 'WAREHOUSE', reference: 'C' } }),
    ]),
    now,
  );
  equal(manySites.duty, '现场作业 3 处');
});
