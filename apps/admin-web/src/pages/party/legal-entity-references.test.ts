import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type {
  BusinessPartyListResponseBody,
  CreditPolicyRecord,
  LegalEntityProfileResolutionResponseBody,
  RulePackageRecord,
  SettlementPolicyRecord,
  SupplierAgreementRecord,
} from './api';
import {
  creditPoliciesOfLegalEntity,
  inUseCount,
  partyNameOfAnswer,
  profileStatusCellOf,
  referenceLineText,
  referenceRegisterHash,
  referenceRowHash,
  referencedCount,
  referencedCountText,
  rulePackagesOfLegalEntity,
  settlementPoliciesOfLegalEntity,
  supplierAgreementsOfLegalEntity,
  versionOrdinalText,
  type LegalEntityReferenceStates,
} from './legal-entity-references';

// 本文件钉公司主体「关联」签与首屏两格（票 company-subject-ux/03）的判读：只按法人标识在已取回的行上关联，不下推查询；
// 在用只看服务端答的状态；读不到的册说读不到、不显 0；被引用少一册就是「—」。行上的供应商与客户经往来单位册译名、
// 版本标识拆成「第 N 版」是票 company-subject-ux/07 加的。

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

function credit(over: Partial<CreditPolicyRecord>): CreditPolicyRecord {
  return {
    objectId: 'SYN-CREDIT-01',
    version: 'v1',
    legalEntity: SELF,
    authorityLevel: 'OPERATIONS',
    chargeType: 'FREIGHT',
    limitMinor: 100000,
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2025-12-01T00:00:00Z',
    ...over,
  };
}

function rulePackage(over: Partial<RulePackageRecord>): RulePackageRecord {
  return {
    objectId: 'SYN-RULEPKG-01',
    version: 'v1',
    serviceProduct: 'SYN-PROD-A',
    contract: 'SYN-CONTRACT-01',
    legalEntity: SELF,
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    declaredAt: '2025-12-01T00:00:00Z',
    rules: [],
    intakeQualificationDeclared: false,
    allowedIntakeSources: [],
    intakeQualificationRefs: [],
    finalRulesDeclared: false,
    finalRules: [],
    ...over,
  };
}

function answered<Row>(rows: Row[]): { kind: 'answered'; rows: Row[] } {
  return { kind: 'answered', rows };
}

function states(over: Partial<LegalEntityReferenceStates>): LegalEntityReferenceStates {
  return {
    agreements: answered([agreement({}), agreement({ version: 'v2', status: 'RETIRED' })]),
    settlement: answered([policy({}), policy({ objectId: 'SYN-SETTLEMENT-PREPAID-02' })]),
    credit: answered([credit({})]),
    rulePackages: answered([rulePackage({})]),
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

// Covers: 三本政策册都按责任法人一格关联，别家的不列；同一公司多份全列，按标识与版本排。
test('结算政策、信用政策、接单规则包按责任法人关联', () => {
  deepEqual(
    settlementPoliciesOfLegalEntity(SELF, [
      policy({ objectId: 'SYN-SETTLEMENT-PREPAID-02' }),
      policy({ objectId: 'SYN-SETTLEMENT-OTHER', legalEntity: 'SYN-LE-02' }),
      policy({ objectId: 'SYN-SETTLEMENT-PREPAID-01' }),
    ]).map((row) => row.objectId),
    ['SYN-SETTLEMENT-PREPAID-01', 'SYN-SETTLEMENT-PREPAID-02'],
  );
  deepEqual(
    creditPoliciesOfLegalEntity(SELF, [
      credit({ version: 'v2' }),
      credit({ objectId: 'SYN-CREDIT-OTHER', legalEntity: 'SYN-LE-02' }),
      credit({ version: 'v1' }),
    ]).map((row) => `${row.objectId}@${row.version}`),
    ['SYN-CREDIT-01@v1', 'SYN-CREDIT-01@v2'],
  );
  deepEqual(
    rulePackagesOfLegalEntity(SELF, [
      rulePackage({ objectId: 'SYN-RULEPKG-OTHER', legalEntity: 'SYN-LE-02' }),
      rulePackage({}),
    ]).map((row) => row.objectId),
    ['SYN-RULEPKG-01'],
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

// Covers: 被引用 = 协议在用版本数 + 三本政策册的版本数；任一册没答到就是 null → 「—」，不拿读到的那几册冒充和。
test('referencedCount 四册都答到才有数', () => {
  equal(referencedCount(states({})), 1 + 2 + 1 + 1);
  equal(referencedCount(states({ agreements: { kind: 'loading' } })), null);
  equal(referencedCount(states({ settlement: { kind: 'notGranted' } })), null);
  equal(referencedCount(states({ credit: { kind: 'unconfigured' } })), null);
  equal(referencedCount(states({ rulePackages: { kind: 'transport', message: 'x' } })), null);
  equal(referencedCount(states({ credit: answered([]), rulePackages: answered([]) })), 3);
  equal(referencedCountText(3), '3 份');
  equal(referencedCountText(0), '0 份');
  equal(referencedCountText(null), '—');
});

// Covers: 点行去对象所在的页并带对象标识作检索词；三本政策册都落在商业规则页、按册定位；段首链接带公司标识。
test('引用的去处带检索词', () => {
  equal(referenceRowHash('supplier-agreement', 'SYN-SUPPLIER-TRUNK-CN-SG-01'), '#/supplier-agreements?q=SYN-SUPPLIER-TRUNK-CN-SG-01');
  equal(
    referenceRowHash('settlement-policy', 'SYN-SETTLEMENT-PREPAID-01'),
    '#/commercial-policies?register=SETTLEMENT_POLICY&q=SYN-SETTLEMENT-PREPAID-01',
  );
  equal(referenceRowHash('credit-policy', 'SYN-CREDIT-01'), '#/commercial-policies?register=CREDIT_POLICY&q=SYN-CREDIT-01');
  equal(referenceRowHash('rule-package', 'SYN-RULEPKG-01'), '#/commercial-policies?q=SYN-RULEPKG-01');
  equal(referenceRegisterHash('supplier-agreement', SELF), '#/supplier-agreements?q=SYN-LE-01');
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
  if (cell.kind === 'status') {
    equal(cell.status.tone, 'warn');
    // 没有生效版本就没有「资料版本」格的数，键也不出：事实行据此显「—」，不拿 0 冒充第 0 版。
    equal('effectiveRevision' in cell, false);
  }
  // 答到了生效版本，事实行「资料版本」格取它的版本号（票 company-subject-ux/08 第 2 条）。
  const resolved: ApiResult<LegalEntityProfileResolutionResponseBody> = {
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'RESOLVED',
      legalEntityId: SELF,
      at: '2026-01-01T00:00:00Z',
      effectiveRevision: { revision: 5 } as unknown as NonNullable<LegalEntityProfileResolutionResponseBody['effectiveRevision']>,
    },
  };
  const resolvedCell = profileStatusCellOf(resolved, SELF);
  equal(resolvedCell.kind === 'status' ? resolvedCell.effectiveRevision : undefined, 5);
});

// Covers: 往来单位册答到了按标识取名；册没读到、读不到、册上没有这个标识、名为空串，一律查不到——行上照显标识，不报错。
test('partyNameOfAnswer 只在册答到且有名时译名', () => {
  const listed: ApiResult<BusinessPartyListResponseBody> = {
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'BUSINESS_PARTIES_LISTED',
      parties: [
        {
          tenantId: 'SYN-TENANT-01',
          partyId: 'SYN-PARTY-CARRIER-LM-01',
          partyName: '合成末端承运方（演示）',
          status: 'EFFECTIVE',
          revision: 1,
          basis: 'SYN-BASIS',
          effectiveFrom: '2026-01-01T00:00:00Z',
          registeredAt: '2026-01-01T00:00:00Z',
        },
        {
          tenantId: 'SYN-TENANT-01',
          partyId: 'SYN-PARTY-BLANK',
          partyName: '',
          status: 'EFFECTIVE',
          revision: 1,
          basis: 'SYN-BASIS',
          effectiveFrom: '2026-01-01T00:00:00Z',
          registeredAt: '2026-01-01T00:00:00Z',
        },
      ],
    },
  };
  const nameOf = partyNameOfAnswer(listed);
  equal(nameOf('SYN-PARTY-CARRIER-LM-01'), '合成末端承运方（演示）');
  equal(nameOf('SYN-PARTY-UNKNOWN'), undefined);
  equal(nameOf('SYN-PARTY-BLANK'), undefined);
  equal(partyNameOfAnswer(null)('SYN-PARTY-CARRIER-LM-01'), undefined);
  equal(partyNameOfAnswer({ kind: 'notGranted' })('SYN-PARTY-CARRIER-LM-01'), undefined);
  equal(partyNameOfAnswer({ kind: 'transport', message: 'x' })('SYN-PARTY-CARRIER-LM-01'), undefined);
});

// Covers: `v1` 拆成「第 1 版」（大小写与首尾空白不计较）；不是 v 加数字的版本标识照显原串，不硬编序数。
test('versionOrdinalText 只把 v 加数字拆成第 N 版', () => {
  equal(versionOrdinalText('v1'), '第 1 版');
  equal(versionOrdinalText('V12'), '第 12 版');
  equal(versionOrdinalText(' v3 '), '第 3 版');
  equal(versionOrdinalText('2026-Q1'), '2026-Q1');
  equal(versionOrdinalText('v1a'), 'v1a');
  equal(versionOrdinalText(''), '');
});
