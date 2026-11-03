import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type { LegalEntityProfileResolutionResponseBody, SettlementPolicyRecord, SupplierAgreementRecord } from './api';
import {
  displayDateOf,
  inUseCount,
  profileStatusCellOf,
  referenceLineText,
  referenceRegisterHash,
  referenceRowHash,
  referencedCount,
  referencedCountText,
  settlementPoliciesOfLegalEntity,
  supplierAgreementsOfLegalEntity,
} from './legal-entity-references';

// 本文件钉公司主体「关联」签与首屏三格（票 company-subject-ux/03）的判读：只按法人标识在已取回的行上关联，不下推查询；
// 在用只看服务端答的状态；读不到的册说读不到、不显 0；被引用少一册就是「—」。

const SELF = 'SYN-LE-01';

function agreement(over: Partial<SupplierAgreementRecord>): SupplierAgreementRecord {
  return {
    objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    publishedAt: '2025-12-01T00:00:00Z',
    contentRegistered: true,
    supplier: 'SYN-PARTY-CARRIER-TRUNK-01',
    legalEntity: SELF,
    ...over,
  };
}

function policy(over: Partial<SettlementPolicyRecord>): SettlementPolicyRecord {
  return {
    objectId: 'SYN-SETTLEMENT-PREPAID-01',
    version: 'v1',
    method: 'PREPAID',
    legalEntity: SELF,
    counterparty: 'SYN-PARTY-SHIPPER-01',
    contractLabel: 'SYN-CONTRACT-01@v1',
    chargeScope: 'SYN-SCOPE-01',
    currency: 'CNY',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2025-12-01T00:00:00Z',
    ...over,
  };
}

// Covers: 只收责任法人是本公司且正文已登记的版本；壳在正文缺的版本没有法人一格，不列；按对象标识再按版本排。
test('供应商协议按责任法人在已取回的行上关联，正文未登记的不列', () => {
  const rows = supplierAgreementsOfLegalEntity(SELF, [
    agreement({ objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01', version: 'v2' }),
    agreement({ objectId: 'SYN-SUPPLIER-LASTMILE-SG-01', version: 'v1' }),
    agreement({ objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01', version: 'v1' }),
    agreement({ objectId: 'SYN-SUPPLIER-OTHER-01', legalEntity: 'SYN-LE-02' }),
    agreement({ objectId: 'SYN-SUPPLIER-SHELL-01', contentRegistered: false, legalEntity: undefined }),
  ]);
  deepEqual(
    rows.map((row) => `${row.objectId}@${row.version}`),
    ['SYN-SUPPLIER-LASTMILE-SG-01@v1', 'SYN-SUPPLIER-TRUNK-CN-SG-01@v1', 'SYN-SUPPLIER-TRUNK-CN-SG-01@v2'],
  );
});

// Covers: 结算政策按责任法人一格关联，别家的不列；同一公司多份全列。
test('结算政策按责任法人关联', () => {
  const rows = settlementPoliciesOfLegalEntity(SELF, [
    policy({ objectId: 'SYN-SETTLEMENT-PREPAID-02' }),
    policy({ objectId: 'SYN-SETTLEMENT-OTHER', legalEntity: 'SYN-LE-02' }),
    policy({ objectId: 'SYN-SETTLEMENT-PREPAID-01' }),
  ]);
  deepEqual(
    rows.map((row) => row.objectId),
    ['SYN-SETTLEMENT-PREPAID-01', 'SYN-SETTLEMENT-PREPAID-02'],
  );
});

// Covers: 在用只数服务端答 EFFECTIVE 的版本；已发布未生效、已退役的都不算。
test('inUseCount 只数已生效', () => {
  equal(inUseCount([{ status: 'EFFECTIVE' }, { status: 'PUBLISHED' }, { status: 'RETIRED' }, { status: 'EFFECTIVE' }]), 2);
  equal(inUseCount([]), 0);
});

// Covers: 段首一行——有状态的册报「总数 · 在用」，没状态的册只报总数；读不到说读不到、不显 0；还没读到显占位。
test('referenceLineText 按册的读取状态与有无状态措辞', () => {
  equal(
    referenceLineText('供应商协议', { kind: 'answered', rows: [{ status: 'EFFECTIVE' }, { status: 'RETIRED' }] }, true),
    '供应商协议 2 · 在用 1',
  );
  equal(referenceLineText('结算政策', { kind: 'answered', rows: [{}, {}] }, false), '结算政策 2');
  equal(referenceLineText('结算政策', { kind: 'answered', rows: [] }, false), '结算政策 0');
  equal(referenceLineText('供应商协议', { kind: 'loading' }, true), '供应商协议 …');
  equal(referenceLineText('供应商协议', { kind: 'unconfigured' }, true), '供应商协议 读不到');
  equal(referenceLineText('结算政策', { kind: 'transport', message: 'x' }, false), '结算政策 读不到');
});

// Covers: 被引用 = 协议在用版本数 + 政策版本数；任一册没答到就是 null → 「—」，不拿读到的那一册冒充和。
test('referencedCount 两册都答到才有数', () => {
  const agreements = { kind: 'answered' as const, rows: [agreement({}), agreement({ version: 'v2', status: 'RETIRED' })] };
  const policies = { kind: 'answered' as const, rows: [policy({}), policy({ objectId: 'SYN-SETTLEMENT-PREPAID-02' })] };
  equal(referencedCount(agreements, policies), 3);
  equal(referencedCount({ kind: 'loading' }, policies), null);
  equal(referencedCount(agreements, { kind: 'notGranted' }), null);
  equal(referencedCountText(3), '3');
  equal(referencedCountText(0), '0');
  equal(referencedCountText(null), '—');
});

// Covers: 点行去对象所在的页并带对象标识作检索词；结算政策落在商业规则页的结算政策册上；段首链接带公司标识。
test('引用的去处带检索词', () => {
  equal(referenceRowHash('supplier-agreement', 'SYN-SUPPLIER-TRUNK-CN-SG-01'), '#/supplier-agreements?q=SYN-SUPPLIER-TRUNK-CN-SG-01');
  equal(
    referenceRowHash('settlement-policy', 'SYN-SETTLEMENT-PREPAID-01'),
    '#/commercial-policies?register=SETTLEMENT_POLICY&q=SYN-SETTLEMENT-PREPAID-01',
  );
  equal(referenceRegisterHash('supplier-agreement', SELF), '#/supplier-agreements?q=SYN-LE-01');
});

// Covers: 日期按呈现时区取墙钟日，不是 ISO 串的前十位；解析不了原样交回。
test('displayDateOf 按呈现时区取日', () => {
  equal(displayDateOf('2026-01-01T20:00:00Z', 'Asia/Shanghai'), '2026-01-02');
  equal(displayDateOf('2026-01-01T20:00:00Z', 'UTC'), '2026-01-01');
  equal(displayDateOf('not-a-time', 'UTC'), 'not-a-time');
});

// Covers: 资料状态格——还没读到是占位；读不到或答的是别家公司显「—」；答到了词与色调取 legalEntityProfileStatusOf。
test('profileStatusCellOf 只在答到这家公司时出状态', () => {
  deepEqual(profileStatusCellOf(null, SELF), { kind: 'pending' });
  const refused: ApiResult<LegalEntityProfileResolutionResponseBody> = { kind: 'notGranted' };
  deepEqual(profileStatusCellOf(refused, SELF), { kind: 'unavailable' });
  const other: ApiResult<LegalEntityProfileResolutionResponseBody> = {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'RESOLVED', legalEntityId: 'SYN-LE-02', at: '2026-01-01T00:00:00Z' },
  };
  deepEqual(profileStatusCellOf(other, SELF), { kind: 'unavailable' });
  const incomplete: ApiResult<LegalEntityProfileResolutionResponseBody> = {
    kind: 'outcome',
    status: 200,
    body: { outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_EFFECTIVE_REVISION', legalEntityId: SELF, at: '2026-01-01T00:00:00Z' },
  };
  const cell = profileStatusCellOf(incomplete, SELF);
  equal(cell.kind, 'status');
  if (cell.kind === 'status') equal(cell.status.tone, 'warn');
});
