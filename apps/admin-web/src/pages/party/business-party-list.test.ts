import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord, GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import {
  partiesPinnedByLegalEntities,
  businessPartyListConditions,
  businessPartyPanelTabsOf,
  businessPartyRegistrationCountry,
  businessPartyTypeFilterOptions,
  partyRegistrationCountriesOf,
  shownBusinessPartyPanelTab,
  businessPartyNoMatchNote,
  businessPartyStatusFilterOptions,
  businessPartyTypeLabel,
  defaultBusinessPartySort,
  filterBusinessPartiesByType,
  indexPartyRoles,
  ourCompanyPartyIds,
  rolesOfParty,
} from './business-party-list';
import { catalogueConditionsQuery } from '../catalogue-query';

// 本文件钉的是身份本体册的列表纯逻辑：检索、状态与表头排序编成读口的条件（ADR-0144，可排维与筛选维照后端 BusinessPartyPages
// 的声明），类型由别的册推出来、只筛到手的这一页；「筛出为空」与「登记册为空」是两个事实，由调用页按票 01 裁决 1 另显。

function record(over: Partial<BusinessPartyRecord>): BusinessPartyRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    partyId: 'SYN-PARTY-OPERATOR-01',
    partyName: '合成运营方一号（演示）',
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    ...over,
  };
}

const rows: BusinessPartyRecord[] = [
  record({
    partyId: 'SYN-PARTY-CARRIER-02',
    partyName: '合成承运商二号（演示）',
    status: 'REGISTERED',
    effectiveFrom: '2030-01-01T00:00:00Z',
    registeredAt: '2026-09-16T05:00:00Z',
  }),
  record({ partyId: 'SYN-PARTY-OPERATOR-01' }),
  record({
    partyId: 'SYN-PARTY-RETIRED-01',
    partyName: '合成停用示例（演示）',
    status: 'DEACTIVATED',
    deactivatedAt: '2026-06-01T00:00:00Z',
    deactivationBasis: 'SYN-DEACT-01',
    effectiveFrom: '2025-12-01T00:00:00Z',
    registeredAt: '2026-09-16T03:00:00Z',
  }),
  record({ partyId: 'SYN-PARTY-AGENT-04', partyName: '合成代理四号（演示）', registeredAt: '2026-09-16T04:00:00Z' }),
];

const ids = (list: BusinessPartyRecord[]) => list.map((row) => row.partyId);

// Covers: 检索词、状态与排序编成一问——「全部」即 status 一维缺席，登记时间新→旧是读口的缺省序、不写进查询串（不带条件的第一页
// 是裸路径）；生效时间两向都下推；名称列与认不得的列不在读口的可排维里，按缺省序问，不带 sort。
test('businessPartyListConditions 按读口声明的维下推', () => {
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', defaultBusinessPartySort)), '');
  equal(
    catalogueConditionsQuery(businessPartyListConditions(' 承运 ', 'DEACTIVATED', { key: 'effective-from', dir: 1 })),
    'sort=effectiveFrom&status=DEACTIVATED&q=%E6%89%BF%E8%BF%90',
  );
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', { key: 'effective-from', dir: -1 })), 'sort=-effectiveFrom');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', { key: 'registered-at', dir: 1 })), 'sort=registeredAt');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', { key: 'party-name', dir: 1 })), '');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'REGISTERED', { key: 'no-such-column', dir: 1 })), 'status=REGISTERED');
  deepEqual(defaultBusinessPartySort, { key: 'registered-at', dir: -1 });
  equal(businessPartyNoMatchNote, '当前筛选条件下没有匹配的单位');
});

// Covers: 胶囊选项与封闭词一一对应、「全部」在首位；词从 identityStatusLabels 派生（待生效 / 已生效 / 已停用），本文件不另抄。
test('状态胶囊选项表', () => {
  deepEqual(
    businessPartyStatusFilterOptions.map((option) => option.value),
    ['ALL', 'REGISTERED', 'EFFECTIVE', 'DEACTIVATED'],
  );
  deepEqual(
    businessPartyStatusFilterOptions.map((option) => option.label),
    ['全部', '待生效', '已生效', '已停用'],
  );
});

function relationship(over: Partial<PartyRelationshipRecord>): PartyRelationshipRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    relationshipId: 'SYN-REL-01',
    revision: 1,
    holderId: 'SYN-PARTY-OPERATOR-01',
    holderName: '合成运营方一号（演示）',
    holderNameKnown: true,
    counterpartyId: 'SYN-PARTY-LE-01',
    counterpartyName: '合成物流（上海）',
    counterpartyNameKnown: true,
    role: 'CUSTOMER',
    scope: 'SYN-SCOPE-01',
    basis: 'SYN-REL-BASIS-01',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
    ...over,
  };
}

// Covers: 类型由关系册上持有的角色算——只数持有方那一侧、只数在用（待确认 / 已生效）的关系、去重并按词表顺序；
// 词表之外的角色码不进索引；关系册没取到时索引为空。
test('indexPartyRoles 按持有方汇总在用关系的角色', () => {
  const index = indexPartyRoles([
    relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-OPERATOR-01', role: 'SUPPLIER' }),
    relationship({ relationshipId: 'R2', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER', status: 'CANDIDATE' }),
    relationship({ relationshipId: 'R3', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER' }),
    relationship({ relationshipId: 'R4', holderId: 'SYN-PARTY-AGENT-04', role: 'CARRIER_AGENT', status: 'EXPIRED' }),
    relationship({ relationshipId: 'R5', holderId: 'SYN-PARTY-AGENT-04', role: 'SOMETHING_NEW' }),
  ]);
  deepEqual(rolesOfParty(index, 'SYN-PARTY-OPERATOR-01'), ['CUSTOMER', 'SUPPLIER']);
  deepEqual(rolesOfParty(index, 'SYN-PARTY-AGENT-04'), []);
  // 相对方不因被持有而得到角色。
  deepEqual(rolesOfParty(index, 'SYN-PARTY-LE-01'), []);
  equal(indexPartyRoles(null).size, 0);
});

// Covers: ADR-0367 决定四——未停用客户账户的所属客户持有客户角色，不要求先登客户关系；已停用的账户不算；与关系上的客户
// 角色合并不重复；关系册没取到时账户册照样贡献。
test('indexPartyRoles 把未停用客户账户的所属客户算作客户', () => {
  const account = (accountId: string, customerPartyId: string, status: string) => ({
    tenantId: 'SYN-TENANT-01', accountId, customerPartyId, customerPartyNameKnown: true, status,
    revision: 1, basis: 'SYN-BASIS', effectiveFrom: '2026-01-01T00:00:00Z', registeredAt: '2026-01-01T00:00:00Z',
  });
  const index = indexPartyRoles(
    [relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-SHIPPER-01', role: 'CUSTOMER' })],
    [
      account('A1', 'SYN-PARTY-SHIPPER-01', 'EFFECTIVE'),
      account('A2', 'SYN-PARTY-SHIPPER-02', 'REGISTERED'),
      account('A3', 'SYN-PARTY-SHIPPER-03', 'DEACTIVATED'),
    ],
  );
  deepEqual(rolesOfParty(index, 'SYN-PARTY-SHIPPER-01'), ['CUSTOMER']);
  deepEqual(rolesOfParty(index, 'SYN-PARTY-SHIPPER-02'), ['CUSTOMER']);
  deepEqual(rolesOfParty(index, 'SYN-PARTY-SHIPPER-03'), []);
  deepEqual(rolesOfParty(indexPartyRoles(null, [account('A4', 'SYN-PARTY-SHIPPER-04', 'EFFECTIVE')]), 'SYN-PARTY-SHIPPER-04'), ['CUSTOMER']);
});

// Covers: 类型筛选只筛传进来的这一页、精确到角色；NONE 只留没有角色的；「全部」交回原行的副本，不改输入。
test('类型筛选按持有的角色', () => {
  const index = indexPartyRoles([
    relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER' }),
    relationship({ relationshipId: 'R2', holderId: 'SYN-PARTY-AGENT-04', role: 'CARRIER_AGENT' }),
  ]);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'CUSTOMER', index)), ['SYN-PARTY-OPERATOR-01']);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'NONE', index)), ['SYN-PARTY-CARRIER-02', 'SYN-PARTY-RETIRED-01']);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'SUPPLIER', index)), []);
  const all = filterBusinessPartiesByType(rows, 'ALL', index);
  deepEqual(ids(all), ids(rows));
  equal(all === rows, false);
});

// Covers: 类型选项——「全部类型」在首位，我方公司在角色之前，「暂无合作关系」在最后，与类型列里的次序一样。
test('类型筛选选项表', () => {
  deepEqual(
    businessPartyTypeFilterOptions.map((option) => option.value),
    ['ALL', 'OURS', 'CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT', 'RESELLER', 'ACCOUNT_HOLDER', 'AUTHORIZED_DELIVERING_PARTY', 'NONE'],
  );
  equal(businessPartyTypeFilterOptions[0].label, '全部类型');
  equal(businessPartyTypeFilterOptions[1].label, '我方公司');
  equal(businessPartyTypeFilterOptions[businessPartyTypeFilterOptions.length - 1].label, '暂无合作关系');
});

function legalEntity(over: Partial<GroupLegalEntityRecord>): GroupLegalEntityRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'SYN-PARTY-OPERATOR-01',
    partyNameKnown: true,
    partyName: '合成运营方一号（演示）',
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-LE-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-01-02T00:00:00Z',
    identityLayerRegistered: false,
    ...over,
  };
}

// Covers: 「我方公司」按法人册算（票 company-subject-ux/02）——法人钉着的参与方就是；只数在用（待生效 / 已生效）的法人，
// 已停用的不算；名称转写不到的照算（钉的是标识）；法人册没取到即空集。
test('ourCompanyPartyIds 按法人册钉着的参与方算，只数在用的法人', () => {
  const ours = ourCompanyPartyIds([
    legalEntity({}),
    legalEntity({ legalEntityId: 'SYN-LE-02', partyId: 'SYN-PARTY-AGENT-04', status: 'REGISTERED' }),
    legalEntity({ legalEntityId: 'SYN-LE-03', partyId: 'SYN-PARTY-RETIRED-01', status: 'DEACTIVATED' }),
    legalEntity({ legalEntityId: 'SYN-LE-04', partyId: 'SYN-PARTY-DANGLING-01', partyNameKnown: false, partyName: undefined }),
  ]);
  deepEqual([...ours].sort(), ['SYN-PARTY-AGENT-04', 'SYN-PARTY-DANGLING-01', 'SYN-PARTY-OPERATOR-01']);
  equal(ourCompanyPartyIds(null).size, 0);
});

// Covers: 类型列的文字——我方公司排在角色前面；不是我方公司就照角色，没角色叫「暂无合作关系」。
test('businessPartyTypeLabel 把我方公司排在角色前面', () => {
  equal(businessPartyTypeLabel(true, []), '我方公司');
  equal(businessPartyTypeLabel(true, ['CUSTOMER']), '我方公司 / 客户');
  equal(businessPartyTypeLabel(false, ['CUSTOMER', 'SUPPLIER']), '客户 / 供应商');
  equal(businessPartyTypeLabel(false, []), '暂无合作关系');
});

// Covers: 类型筛选 OURS 只留我方公司；NONE 不再把我方公司算成「往来单位」；法人册没取到时谁都不是我方公司。
test('我方公司进类型筛选', () => {
  const index = indexPartyRoles([relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER' })]);
  const ours = ourCompanyPartyIds([legalEntity({}), legalEntity({ legalEntityId: 'SYN-LE-02', partyId: 'SYN-PARTY-AGENT-04' })]);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'OURS', index, ours)), ['SYN-PARTY-OPERATOR-01', 'SYN-PARTY-AGENT-04']);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'NONE', index, ours)), ['SYN-PARTY-CARRIER-02', 'SYN-PARTY-RETIRED-01']);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'CUSTOMER', index, ours)), ['SYN-PARTY-OPERATOR-01']);
  deepEqual(ids(filterBusinessPartiesByType(rows, 'OURS', index)), []);
});

// Covers: 对象页收起写入口用的口径——已停用的公司主体钉着的参与方也算；法人册没取到为空集。
test('partiesPinnedByLegalEntities 认已停用的公司主体', () => {
  const entity = (partyId: string, status: string) =>
    ({ tenantId: 'T', legalEntityId: `LE-${partyId}`, kind: 'RESPONSIBLE_LEGAL_ENTITY', partyId, partyNameKnown: true, status, revision: 1, basis: 'B', effectiveFrom: '2026-01-01T00:00:00Z', registeredAt: '2026-01-01T00:00:00Z', identityLayerRegistered: false }) as never;
  const pinned = partiesPinnedByLegalEntities([entity('P1', 'EFFECTIVE'), entity('P2', 'DEACTIVATED')]);
  equal(pinned.has('P1'), true);
  equal(pinned.has('P2'), true);
  equal(partiesPinnedByLegalEntities(null).size, 0);
});

// Covers: 注册地先取往来单位自己登的；我方公司取钉着它的公司主体的（已停用的也算）；公司主体没登身份层、法人册没取到都不补。
test('businessPartyRegistrationCountry 我方公司取公司主体的注册地', () => {
  const entity = (partyId: string, status: string, registrationCountry?: string) =>
    ({ tenantId: 'T', legalEntityId: `LE-${partyId}`, kind: 'RESPONSIBLE_LEGAL_ENTITY', partyId, partyNameKnown: true, status, revision: 1, basis: 'B', effectiveFrom: '2026-01-01T00:00:00Z', registeredAt: '2026-01-01T00:00:00Z', identityLayerRegistered: registrationCountry !== undefined, registrationCountry }) as never;
  const countries = partyRegistrationCountriesOf([entity('OURS', 'EFFECTIVE', 'CN'), entity('OLD', 'DEACTIVATED', 'HK'), entity('BARE', 'EFFECTIVE')]);
  const party = (partyId: string, registrationCountry?: string) => ({ partyId, identityLayerRegistered: registrationCountry !== undefined, registrationCountry });
  equal(businessPartyRegistrationCountry(party('SHIPPER', 'SG'), countries), 'SG');
  equal(businessPartyRegistrationCountry(party('OURS'), countries), 'CN');
  equal(businessPartyRegistrationCountry(party('OLD'), countries), 'HK');
  equal(businessPartyRegistrationCountry(party('BARE'), countries), null);
  equal(businessPartyRegistrationCountry(party('OURS'), partyRegistrationCountriesOf(null)), null);
});

// Covers: 详情栏与对象页同一道门——我方公司不摆资料两签；法人册未答出时签条照常，不先多后少。
test('businessPartyPanelTabsOf 我方公司不摆联系人与资料版本', () => {
  const all = ['summary', 'contacts', 'profile', 'identity', 'overview'];
  deepEqual(businessPartyPanelTabsOf(false), all);
  deepEqual(businessPartyPanelTabsOf(null), all);
  deepEqual(businessPartyPanelTabsOf(true), ['summary', 'identity', 'overview']);
});

// Covers: 逐行翻到我方公司时签退回概要，但记下的那一签不被改掉。
test('shownBusinessPartyPanelTab 记下的签这一行没有就显概要', () => {
  equal(shownBusinessPartyPanelTab('contacts', businessPartyPanelTabsOf(true)), 'summary');
  equal(shownBusinessPartyPanelTab('overview', businessPartyPanelTabsOf(true)), 'overview');
  equal(shownBusinessPartyPanelTab('contacts', businessPartyPanelTabsOf(false)), 'contacts');
});
