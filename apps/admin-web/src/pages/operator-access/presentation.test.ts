import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { effectiveGrantCount, grantQualifier, grantStandingAt, shortSubject } from './presentation';

const now = new Date('2026-11-04T12:00:00Z');
const grant = (over: Partial<Parameters<typeof grantStandingAt>[0]>) => ({
  grantId: 'G-1',
  capabilityFace: 'MASTER_DATA_AND_OPERATIONS_READ',
  effectiveStartsAt: '2026-01-01T00:00:00Z',
  basis: 'B',
  recordedAt: '2026-01-01T00:00:00Z',
  ...over,
});

// Covers: 区间含起点不含终点；撤销时刻到了才算已撤销、没到仍按区间；细分格只有决定种类与作业范围两种。
test('授予此刻的状态照区间与撤销判', () => {
  equal(grantStandingAt(grant({}), now), 'effective');
  equal(grantStandingAt(grant({ effectiveStartsAt: '2026-12-01T00:00:00Z' }), now), 'pending');
  equal(grantStandingAt(grant({ effectiveEndsAt: '2026-11-04T12:00:00Z' }), now), 'expired');
  equal(grantStandingAt(grant({ revocation: { revokedAt: '2026-11-04T11:59:59Z', basis: 'R' } }), now), 'revoked');
  equal(grantStandingAt(grant({ revocation: { revokedAt: '2026-11-05T00:00:00Z', basis: 'R' } }), now), 'effective');
  equal(grantQualifier({ decisionKind: 'LOAD_ASSIGNMENT' }), 'LOAD_ASSIGNMENT');
  equal(grantQualifier({ workScope: { kind: 'NODE', reference: 'SYN-NODE-01' } }), '网点 SYN-NODE-01');
  equal(grantQualifier({}), '');
});

test('操作者的生效授予数与主体标识的短写法', () => {
  const operator = { issuer: 'i', subject: 'ChZTWU4tT1BFUkFUT1ItQ09ORklHLTAxEgVsb2NhbA', basis: 'SYN-OP-01', recordedAt: '2026-01-01T00:00:00Z', grants: [grant({}), grant({ grantId: 'G-2', effectiveEndsAt: '2026-02-01T00:00:00Z' })] };
  equal(effectiveGrantCount(operator, now), 1);
  equal(shortSubject(operator.subject), 'ChZTWU4tT1…b2NhbA');
  equal(shortSubject('short'), 'short');
});
