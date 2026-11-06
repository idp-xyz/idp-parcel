import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord, GroupLegalEntityRecord, PartyRelationshipRecord } from './api';
import {
  partiesPinnedByLegalEntities,
  businessPartyPanelTabsOf,
  businessPartyRegistrationCountry,
  partyRegistrationCountriesOf,
  shownBusinessPartyPanelTab,
  businessPartyCountSummary,
  businessPartyNoMatchNote,
  businessPartyStatusFilterOptions,
  businessPartyTypeFilterOptions,
  businessPartyTypeLabel,
  countBusinessParties,
  defaultBusinessPartySort,
  filterBusinessParties,
  indexPartyRoles,
  ourCompanyPartyIds,
  rolesOfParty,
  sortBusinessParties,
} from './business-party-list';
import { effectiveShareOfInUse } from './legal-entity-list';

// 本文件钉的是身份本体册的筛选、排序与计数都只在已取回的行上做（README 列表页上列通则：不下推成查询参数），
// 且「筛出为空」与「登记册为空」是两个事实——前者由调用页按票 01 裁决 1 另显，这里只保证筛选不把行吞掉也不多算。

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

// Covers: 搜索是包含匹配、不分大小写，命中参与方标识 / 名称 / 状态原名任一格；空白搜索不筛。
test('搜索按包含匹配三格之一', () => {
  deepEqual(ids(filterBusinessParties(rows, { search: 'party-', status: 'ALL' })), ids(rows));
  deepEqual(ids(filterBusinessParties(rows, { search: '停用示例', status: 'ALL' })), ['SYN-PARTY-RETIRED-01']);
  deepEqual(ids(filterBusinessParties(rows, { search: 'registered', status: 'ALL' })), ['SYN-PARTY-CARRIER-02']);
  deepEqual(ids(filterBusinessParties(rows, { search: '  ', status: 'ALL' })), ids(rows));
});

// Covers: 状态筛选精确到封闭词；ALL 不筛；筛出为空交回空数组而不是原行。
test('状态筛选精确匹配封闭词', () => {
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'DEACTIVATED' })), ['SYN-PARTY-RETIRED-01']);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'REGISTERED' })), ['SYN-PARTY-CARRIER-02']);
  deepEqual(ids(filterBusinessParties(rows, { search: 'OPERATOR', status: 'DEACTIVATED' })), []);
});

// Covers: 计数按状态分格，数的是传进来的这一页；词表之外的状态码不进任何格也不抛。占比公式借法人册那一份：分母只数在用两格。
test('countBusinessParties 按状态计数，占比只按在用身份算', () => {
  const counts = countBusinessParties(rows);
  equal(counts.total, 4);
  deepEqual(counts.byStatus, { REGISTERED: 1, EFFECTIVE: 2, DEACTIVATED: 1 });
  // 没有关系册时谁都没有角色：四家都是「往来单位」。
  equal(counts.withoutRole, 4);
  deepEqual(countBusinessParties([record({ status: 'SOMETHING_NEW' })]).byStatus, { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 });
  equal(effectiveShareOfInUse(countBusinessParties(rows)), 67);
  equal(effectiveShareOfInUse(countBusinessParties([record({ status: 'DEACTIVATED' })])), null);
});

// Covers: 默认按登记时间新→旧（裁决 1）；四列各按其键，方向随 dir；排序不改原数组；认不得的键原样交回。
test('表头排序各按其键与方向，且不改原数组', () => {
  const before = ids(rows);
  deepEqual(defaultBusinessPartySort, { key: 'registered-at', dir: -1 });
  deepEqual(ids(sortBusinessParties(rows, defaultBusinessPartySort)), [
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-AGENT-04',
    'SYN-PARTY-RETIRED-01',
  ]);
  deepEqual(ids(sortBusinessParties(rows, { key: 'party', dir: 1 })), [
    'SYN-PARTY-AGENT-04',
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-RETIRED-01',
  ]);
  deepEqual(ids(sortBusinessParties(rows, { key: 'party', dir: -1 })), [
    'SYN-PARTY-RETIRED-01',
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-AGENT-04',
  ]);
  deepEqual(ids(sortBusinessParties(rows, { key: 'effective-from', dir: 1 })), [
    'SYN-PARTY-RETIRED-01',
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-AGENT-04',
    'SYN-PARTY-CARRIER-02',
  ]);
  // 名称按码位字典序（byString 不按 locale）：代 < 停 < 承 < 运。
  deepEqual(ids(sortBusinessParties(rows, { key: 'party-name', dir: 1 })), [
    'SYN-PARTY-AGENT-04',
    'SYN-PARTY-RETIRED-01',
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-OPERATOR-01',
  ]);
  deepEqual(ids(sortBusinessParties(rows, { key: 'no-such-column', dir: 1 })), before);
  deepEqual(ids(rows), before);
});

// Covers: 时刻比按解析值不按字典序——端点用 RFC3339Nano 剪尾零，同一秒内小数位数不定，字典序会把 `55Z` 排到
// `55.939Z` 之后（票 01 评审 Standards 1 的教训）。解析不了的值退回字符串比，不抛、不丢行。
test('同一秒内小数位数不定的时刻按真实先后排', () => {
  const sameSecond = [
    record({ partyId: 'A', registeredAt: '2026-09-16T04:27:55.9Z' }),
    record({ partyId: 'B', registeredAt: '2026-09-16T04:27:55Z' }),
    record({ partyId: 'C', registeredAt: '2026-09-16T04:27:55.939Z' }),
    record({ partyId: 'D', registeredAt: '2026-09-16T04:27:55.93Z' }),
  ];
  deepEqual(ids(sortBusinessParties(sameSecond, defaultBusinessPartySort)), ['C', 'D', 'A', 'B']);
  const unparsable = [record({ partyId: 'X', registeredAt: 'not-a-time' }), record({ partyId: 'Y' })];
  deepEqual(ids(sortBusinessParties(unparsable, defaultBusinessPartySort)), ['X', 'Y']);
});

// Covers: 票 01 裁决 1——筛出为空不是空态。计数摘要总数与当前显示数分开报，筛空时照显「共 N 个，当前显示 0 个」，
// 表格区那一行说的是「当前条件下无匹配」而不是「登记册为空」。
test('计数摘要分报总数与当前显示数，筛空提示不冒充空态', () => {
  equal(businessPartyCountSummary(4, 4), '共 4 家单位，当前显示 4 家');
  equal(businessPartyCountSummary(4, 0), '共 4 家单位，当前显示 0 家');
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

// Covers: 类型筛选精确到角色；NONE 只留没有角色的；类型中文也能被搜索命中；计数按角色与「无角色」分格。
test('类型筛选与计数按持有的角色', () => {
  const index = indexPartyRoles([
    relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER' }),
    relationship({ relationshipId: 'R2', holderId: 'SYN-PARTY-AGENT-04', role: 'CARRIER_AGENT' }),
  ]);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'CUSTOMER' }, index)), ['SYN-PARTY-OPERATOR-01']);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'NONE' }, index)), [
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-RETIRED-01',
  ]);
  deepEqual(ids(filterBusinessParties(rows, { search: '承运商代理', status: 'ALL' }, index)), ['SYN-PARTY-AGENT-04']);
  deepEqual(ids(filterBusinessParties(rows, { search: '暂无合作关系', status: 'ALL' }, index)), ['SYN-PARTY-CARRIER-02', 'SYN-PARTY-RETIRED-01']);
  const counts = countBusinessParties(rows, index);
  equal(counts.byRole.CUSTOMER, 1);
  equal(counts.byRole.CARRIER_AGENT, 1);
  equal(counts.withoutRole, 2);
  equal(businessPartyTypeFilterOptions[0].value, 'ALL');
  equal(businessPartyTypeFilterOptions[businessPartyTypeFilterOptions.length - 1].value, 'NONE');
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

// Covers: 筛选 OURS 只留我方公司；NONE 不再把我方公司算成「往来单位」；搜索能按「我方公司」命中；计数分出 ours、
// withoutRole 不含我方公司；下拉里「我方公司」在「全部类型」之后、角色之前。
test('我方公司进筛选、搜索与计数', () => {
  const index = indexPartyRoles([relationship({ relationshipId: 'R1', holderId: 'SYN-PARTY-OPERATOR-01', role: 'CUSTOMER' })]);
  const ours = ourCompanyPartyIds([legalEntity({}), legalEntity({ legalEntityId: 'SYN-LE-02', partyId: 'SYN-PARTY-AGENT-04' })]);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'OURS' }, index, ours)), [
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-AGENT-04',
  ]);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'NONE' }, index, ours)), [
    'SYN-PARTY-CARRIER-02',
    'SYN-PARTY-RETIRED-01',
  ]);
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'CUSTOMER' }, index, ours)), ['SYN-PARTY-OPERATOR-01']);
  deepEqual(ids(filterBusinessParties(rows, { search: '我方公司', status: 'ALL' }, index, ours)), [
    'SYN-PARTY-OPERATOR-01',
    'SYN-PARTY-AGENT-04',
  ]);
  // 法人册没取到：谁都不是我方公司，与此前的口径一样。
  deepEqual(ids(filterBusinessParties(rows, { search: '', status: 'ALL', type: 'OURS' }, index)), []);

  const counts = countBusinessParties(rows, index, ours);
  equal(counts.ours, 2);
  equal(counts.byRole.CUSTOMER, 1);
  equal(counts.withoutRole, 2);
  equal(countBusinessParties(rows, index).withoutRole, 3);

  deepEqual(businessPartyTypeFilterOptions.slice(0, 2).map((option) => option.value), ['ALL', 'OURS']);
  equal(businessPartyTypeFilterOptions[1].label, '我方公司');
  equal(businessPartyTypeFilterOptions[businessPartyTypeFilterOptions.length - 1].value, 'NONE');
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
