import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import type {
  CustomerAccountRecord,
  GroupLegalEntityRecord,
  PartyRelationshipRecord,
  SettlementPolicyRecord,
  SupplierAgreementRecord,
} from './api';
import {
  countText,
  customerAccountsOfParty,
  effectiveRolesOf,
  legalEntitiesOfParty,
  relationshipsOfParty,
  sectionOf,
  settlementPoliciesOfParty,
  supplierAgreementsOfParty,
} from './party-overview';

// 本文件钉参与方全景（票 operator-workspace-gaps/01）的判读：各段只按参与方标识在已取回的行上关联，不下推查询；
// 方向照关系册的「持有方 → 相对方」字段次序读，不替它解释成谁是谁的客户。

const SELF = 'SYN-PARTY-SHIPPER-01';

function relationship(over: Partial<PartyRelationshipRecord>): PartyRelationshipRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    relationshipId: 'SYN-REL-01',
    revision: 1,
    holderId: SELF,
    holderName: '合成货主一号（演示）',
    holderNameKnown: true,
    counterpartyId: 'SYN-PARTY-OPERATOR-01',
    counterpartyName: '合成运营方一号（演示）',
    counterpartyNameKnown: true,
    role: 'CUSTOMER',
    scope: 'SYN-SCOPE-CN-SG',
    basis: 'SYN-REL-BASIS-01',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
    ...over,
  };
}

// Covers: 只收本方在任一侧的行；逐行标本方所在一侧与另一方；按生效起点新→旧，同刻按关系标识。
test('关系段按本方所在一侧取行并标出另一方', () => {
  const rows = [
    relationship({ relationshipId: 'SYN-REL-01', effectiveStartsAt: '2026-01-01T00:00:00Z' }),
    relationship({
      relationshipId: 'SYN-REL-02',
      holderId: 'SYN-PARTY-AGENT-04',
      holderName: '合成代理四号（演示）',
      counterpartyId: SELF,
      counterpartyName: '合成货主一号（演示）',
      role: 'CARRIER_AGENT',
      effectiveStartsAt: '2026-03-01T00:00:00Z',
    }),
    relationship({
      relationshipId: 'SYN-REL-03',
      holderId: 'SYN-PARTY-AGENT-04',
      counterpartyId: 'SYN-PARTY-OPERATOR-01',
    }),
  ];

  const views = relationshipsOfParty(SELF, rows);

  deepEqual(
    views.map((view) => [view.relationshipId, view.side, view.otherPartyId, view.otherPartyName]),
    [
      ['SYN-REL-02', 'COUNTERPARTY', 'SYN-PARTY-AGENT-04', '合成代理四号（演示）'],
      ['SYN-REL-01', 'HOLDER', 'SYN-PARTY-OPERATOR-01', '合成运营方一号（演示）'],
    ],
  );
});

// Covers: 摘要只看登记为「已生效」的关系（关系状态是登记事实，不按装载时钟推）；按本方所在一侧分组、去重；
// 词表内的码按 CONTEXT 列举顺序，词表外的码排在后面原样保留、按字典序。
test('已生效角色摘要按一侧分组去重', () => {
  const views = relationshipsOfParty(SELF, [
    relationship({ relationshipId: 'SYN-REL-01', role: 'SUPPLIER' }),
    relationship({ relationshipId: 'SYN-REL-02', role: 'CUSTOMER' }),
    relationship({ relationshipId: 'SYN-REL-03', role: 'CUSTOMER' }),
    relationship({ relationshipId: 'SYN-REL-04', role: 'RESELLER', status: 'REVOKED' }),
    relationship({ relationshipId: 'SYN-REL-05', role: 'FUTURE_ROLE' }),
    relationship({
      relationshipId: 'SYN-REL-06',
      holderId: 'SYN-PARTY-OPERATOR-01',
      counterpartyId: SELF,
      role: 'ACCOUNT_HOLDER',
    }),
    relationship({
      relationshipId: 'SYN-REL-07',
      holderId: 'SYN-PARTY-OPERATOR-01',
      counterpartyId: SELF,
      role: 'CARRIER_AGENT',
      status: 'CANDIDATE',
    }),
  ]);

  deepEqual(effectiveRolesOf(views), {
    asHolder: ['CUSTOMER', 'SUPPLIER', 'FUTURE_ROLE'],
    asCounterparty: ['ACCOUNT_HOLDER'],
  });
});

function agreement(over: Partial<SupplierAgreementRecord>): SupplierAgreementRecord {
  return {
    objectId: 'SYN-SA-01',
    version: 'v1',
    scope: 'SYN-SCOPE-CN-SG',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    contentRegistered: true,
    supplier: SELF,
    legalEntity: 'SYN-LE-01',
    purchasePlan: 'SYN-PLAN-BUY-01',
    agreementScope: 'SYN-SCOPE-CN-SG',
    agreementEffectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 只收供应商是本方的协议；正文未登记的版本壳没有 supplier，归不进任何参与方、如实不列；按对象标识再按版本排。
test('供应商协议段只收正文里供应商是本方的版本', () => {
  const rows = [
    agreement({ objectId: 'SYN-SA-02', version: 'v2' }),
    agreement({ objectId: 'SYN-SA-02', version: 'v1' }),
    agreement({ objectId: 'SYN-SA-01' }),
    agreement({ objectId: 'SYN-SA-03', supplier: 'SYN-PARTY-CARRIER-02' }),
    agreement({
      objectId: 'SYN-SA-04',
      contentRegistered: false,
      supplier: undefined,
      legalEntity: undefined,
      purchasePlan: undefined,
      agreementScope: undefined,
      agreementEffectiveStartsAt: undefined,
      registeredAt: undefined,
    }),
  ];

  deepEqual(
    supplierAgreementsOfParty(SELF, rows).map((row) => `${row.objectId}@${row.version}`),
    ['SYN-SA-01@v1', 'SYN-SA-02@v1', 'SYN-SA-02@v2'],
  );
});

function account(over: Partial<CustomerAccountRecord>): CustomerAccountRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    accountId: 'SYN-ACCT-01',
    customerPartyId: SELF,
    customerPartyName: '合成货主一号（演示）',
    customerPartyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-ACCT-BASIS-01',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
    ...over,
  };
}

// Covers: 只收关联到本方客户参与方的账户（ADR-0003 三级边界的第三级必须显式关联客户参与方）；停用的照列，状态由调用页显；按账户标识排。
test('客户账户段只收关联到本方的账户', () => {
  const rows = [
    account({ accountId: 'SYN-ACCT-02', status: 'DEACTIVATED' }),
    account({ accountId: 'SYN-ACCT-03', customerPartyId: 'SYN-PARTY-SHIPPER-02' }),
    account({ accountId: 'SYN-ACCT-01' }),
  ];

  deepEqual(
    customerAccountsOfParty(SELF, rows).map((row) => row.accountId),
    ['SYN-ACCT-01', 'SYN-ACCT-02'],
  );
});

function legalEntity(over: Partial<GroupLegalEntityRecord>): GroupLegalEntityRecord {
  return {
    identityLayerRegistered: true,
    registrationCountry: 'SG',
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'SYN-PARTY-OPERATOR-01',
    partyName: '合成运营方一号（演示）',
    partyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-LE-BASIS-01',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
    ...over,
  };
}

// Covers: 法人册钉着哪个参与方身份就归谁——本方是不是本集团的责任法人；不是时交回空，由调用页如实写「不是」。
test('法人段只收钉着本方身份的责任法人', () => {
  const rows = [
    legalEntity({ legalEntityId: 'SYN-LE-02', partyId: SELF }),
    legalEntity({ legalEntityId: 'SYN-LE-01' }),
  ];

  deepEqual(
    legalEntitiesOfParty(SELF, rows).map((row) => row.legalEntityId),
    ['SYN-LE-02'],
  );
  deepEqual(legalEntitiesOfParty('SYN-PARTY-AGENT-04', rows), []);
});

function settlementPolicy(over: Partial<SettlementPolicyRecord>): SettlementPolicyRecord {
  return {
    objectId: 'SYN-SP-01',
    version: 'v1',
    method: 'TERMS',
    legalEntity: 'SYN-LE-01',
    counterparty: SELF,
    contractLabel: 'SYN-CONTRACT-01@v1',
    chargeScope: 'SYN-CHARGE-FREIGHT',
    currency: 'SGD',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
    ...over,
  };
}

// Covers: 结算政策六维里的客户相对方是业务参与方、不是客户账户（PC CONTEXT 结算政策那条）——按 counterparty 关联；
// 同一货主可以同时适用预付与账期两份不重叠的政策，两份都列；按对象标识再按版本排。
test('结算政策段按客户相对方关联', () => {
  const rows = [
    settlementPolicy({ objectId: 'SYN-SP-02', method: 'PREPAID', chargeScope: 'SYN-CHARGE-DUTY' }),
    settlementPolicy({ objectId: 'SYN-SP-01' }),
    settlementPolicy({ objectId: 'SYN-SP-03', counterparty: 'SYN-PARTY-SHIPPER-02' }),
  ];

  deepEqual(
    settlementPoliciesOfParty(SELF, rows).map((row) => `${row.objectId}:${row.method}`),
    ['SYN-SP-01:TERMS', 'SYN-SP-02:PREPAID'],
  );
});

type Listed = { kind: string; items: string[] };
const itemsOf = (body: Listed) => (body.kind === 'WANTED' ? body.items : null);

// Covers: 段状态逐一对应读口结果——没回来是读取中；业务答案交回关联后的行（零条也是答案）；回显的册种与所问不符时丢弃，
// 不冒充「册上没有」；未配置、调用方问题、未形成答案、没连上各自成格，一段的错不牵连别段。
test('段状态对应读口的每一种结果', () => {
  deepEqual(sectionOf<Listed, string>(null, itemsOf), { kind: 'loading' });
  deepEqual(sectionOf({ kind: 'outcome', status: 200, body: { kind: 'WANTED', items: ['a'] } }, itemsOf), {
    kind: 'answered',
    rows: ['a'],
  });
  deepEqual(sectionOf({ kind: 'outcome', status: 200, body: { kind: 'WANTED', items: [] } }, itemsOf), {
    kind: 'answered',
    rows: [],
  });
  deepEqual(sectionOf({ kind: 'outcome', status: 200, body: { kind: 'OTHER', items: ['x'] } }, itemsOf), {
    kind: 'mismatch',
  });
  const unconfigured: ApiResult<Listed> = { kind: 'unconfigured' };
  deepEqual(sectionOf(unconfigured, itemsOf), { kind: 'unconfigured' });
  const callerProblem: ApiResult<Listed> = { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' };
  deepEqual(sectionOf(callerProblem, itemsOf), { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  const noAnswer: ApiResult<Listed> = { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' };
  deepEqual(sectionOf(noAnswer, itemsOf), { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' });
  const transport: ApiResult<Listed> = { kind: 'transport', message: 'fetch failed' };
  deepEqual(sectionOf(transport, itemsOf), { kind: 'transport', message: 'fetch failed' });
});

// Covers: 计数只在拿到业务答案后报（README 列表页上列通则第六条）——答了零条报 0，其余各态报「—」，不拿 0 冒充「没有」。
test('计数只在业务答案下报数', () => {
  equal(countText({ kind: 'answered', rows: [1, 2] }), '2');
  equal(countText({ kind: 'answered', rows: [] }), '0');
  equal(countText({ kind: 'loading' }), '—');
  equal(countText({ kind: 'unconfigured' }), '—');
  equal(countText({ kind: 'mismatch' }), '—');
  equal(countText({ kind: 'transport', message: 'fetch failed' }), '—');
});
