import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { GroupLegalEntityRecord } from './api';
import { legalEntityLifecycle, lifecycleConnectorReached } from './legal-entity-lifecycle';

function record(over: Partial<GroupLegalEntityRecord>): GroupLegalEntityRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'SYN-PARTY-OPERATOR-01',
    partyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-LE-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    identityLayerRegistered: true,
    ...over,
  };
}

const states = (row: GroupLegalEntityRecord) => legalEntityLifecycle(row)?.map((stage) => stage.state) ?? null;

// Covers: 当前段之前走过、之后未到。
test('已登记与已生效的三段', () => {
  deepEqual(states(record({ status: 'REGISTERED' })), ['current', 'pending', 'pending']);
  deepEqual(states(record({ status: 'EFFECTIVE' })), ['done', 'current', 'pending']);
});

// Covers: 已停用时「已生效」按时点判——生效后停用算走过，未到生效就停用算跳过；时点解析不了按走过。
test('已停用时生效段按时点判走过或跳过', () => {
  deepEqual(states(record({ status: 'DEACTIVATED', deactivatedAt: '2026-06-01T00:00:00Z' })), ['done', 'done', 'current']);
  deepEqual(
    states(record({ status: 'DEACTIVATED', effectiveFrom: '2030-01-01T00:00:00Z', deactivatedAt: '2026-06-01T00:00:00Z' })),
    ['done', 'skipped', 'current'],
  );
  deepEqual(states(record({ status: 'DEACTIVATED', deactivatedAt: 'not-a-time' })), ['done', 'done', 'current']);
});

// Covers: 连线只在前段走过、后段走过或正在时画成走过——连进、连出被跳过的那段都不算。
test('lifecycleConnectorReached 不把跳过的那段画成走过', () => {
  const connectors = (row: GroupLegalEntityRecord) => {
    const stages = legalEntityLifecycle(row)!;
    return [lifecycleConnectorReached(stages[0], stages[1]), lifecycleConnectorReached(stages[1], stages[2])];
  };
  deepEqual(connectors(record({ status: 'EFFECTIVE' })), [true, false]);
  deepEqual(connectors(record({ status: 'DEACTIVATED', deactivatedAt: '2026-06-01T00:00:00Z' })), [true, true]);
  deepEqual(
    connectors(record({ status: 'DEACTIVATED', effectiveFrom: '2030-01-01T00:00:00Z', deactivatedAt: '2026-06-01T00:00:00Z' })),
    [false, false],
  );
});

// Covers: 三格之外的状态码不画——不把服务端新增的一格硬塞进旧三段。
test('词表之外的状态码答 null', () => {
  equal(legalEntityLifecycle(record({ status: 'SOMETHING_NEW' })), null);
});
