import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord, GroupLegalEntityRecord } from './api';
import {
  businessPartyCategoryOf,
  businessPartyListConditions,
  businessPartyNoMatchNote,
  businessPartyPanelTabsOf,
  businessPartyRegistrationCountry,
  businessPartyStatusFilterOptions,
  businessPartyTypeLabel,
  defaultBusinessPartySort,
  partiesPinnedByLegalEntities,
  partyIsOurCompany,
  partyRegistrationCountriesOf,
  partyRolesOf,
  shownBusinessPartyPanelTab,
} from './business-party-list';
import { catalogueConditionsQuery } from '../catalogue-query';

// 本文件钉的是身份本体册的列表纯逻辑：检索、状态、类型与表头排序编成读口的条件（ADR-0144，可排维与筛选维照后端
// BusinessPartyPages 的声明）；类型由读口在行上答（ADR-0378 的 categories），这里只钉怎么照着读——哪些是角色、是不是我方公司、
// 类型筛选的值落到读口的哪个词。「筛出为空」与「登记册为空」是两个事实，由调用页按票 01 裁决 1 另显。

function record(categories: string[]): Pick<BusinessPartyRecord, 'categories'> {
  return { categories };
}

// Covers: 检索词、状态、类型与排序编成一问——「全部」即那一维缺席，登记时间新→旧是读口的缺省序、不写进查询串（不带条件的第一页
// 是裸路径）；生效时间两向都下推；名称列与认不得的列不在读口的可排维里，按缺省序问，不带 sort。
test('businessPartyListConditions 按读口声明的维下推', () => {
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'ALL', defaultBusinessPartySort)), '');
  equal(
    catalogueConditionsQuery(businessPartyListConditions(' 承运 ', 'DEACTIVATED', 'ALL', { key: 'effective-from', dir: 1 })),
    'sort=effectiveFrom&status=DEACTIVATED&q=%E6%89%BF%E8%BF%90',
  );
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'ALL', { key: 'effective-from', dir: -1 })), 'sort=-effectiveFrom');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'ALL', { key: 'registered-at', dir: 1 })), 'sort=registeredAt');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'ALL', { key: 'party-name', dir: 1 })), '');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'REGISTERED', 'ALL', { key: 'no-such-column', dir: 1 })), 'status=REGISTERED');
  deepEqual(defaultBusinessPartySort, { key: 'registered-at', dir: -1 });
  equal(businessPartyNoMatchNote, '当前筛选条件下没有匹配的单位');
});

// Covers: 类型下推成读口的 categories 维——角色原样，我方公司与「暂无合作关系」各换成读口的词；与状态、检索词可以一起带。
test('类型筛选下推成 categories 维', () => {
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'SUPPLIER', defaultBusinessPartySort)), 'categories=SUPPLIER');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'OURS', defaultBusinessPartySort)), 'categories=OUR_COMPANY');
  equal(catalogueConditionsQuery(businessPartyListConditions('', 'ALL', 'NONE', defaultBusinessPartySort)), 'categories=NO_ROLE');
  equal(
    catalogueConditionsQuery(businessPartyListConditions('SYN', 'EFFECTIVE', 'CUSTOMER', defaultBusinessPartySort)),
    'categories=CUSTOMER&status=EFFECTIVE&q=SYN',
  );
  equal(businessPartyCategoryOf('CARRIER_AGENT'), 'CARRIER_AGENT');
  equal(businessPartyCategoryOf('OURS'), 'OUR_COMPANY');
  equal(businessPartyCategoryOf('NONE'), 'NO_ROLE');
});

// Covers: 照读口在行上答的类别读——角色是去掉「我方公司」「无角色」两个词之后的那些，按词表序排，不随读口给的次序；词表之外的
// 词排在后面原样留着；我方公司看有没有那个词；一类都不属于的一个角色也没有、也不是我方公司。
test('partyRolesOf 与 partyIsOurCompany 照行上的类别读', () => {
  deepEqual(partyRolesOf(record(['CUSTOMER'])), ['CUSTOMER']);
  deepEqual(partyRolesOf(record(['CARRIER_AGENT', 'CUSTOMER', 'SUPPLIER'])), ['CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT']);
  deepEqual(partyRolesOf(record(['OUR_COMPANY', 'ACCOUNT_HOLDER'])), ['ACCOUNT_HOLDER']);
  deepEqual(partyRolesOf(record(['FUTURE_ROLE', 'CUSTOMER'])), ['CUSTOMER', 'FUTURE_ROLE']);
  deepEqual(partyRolesOf(record(['NO_ROLE'])), []);
  deepEqual(partyRolesOf(record(['OUR_COMPANY'])), []);
  equal(partyIsOurCompany(record(['OUR_COMPANY', 'ACCOUNT_HOLDER'])), true);
  equal(partyIsOurCompany(record(['CUSTOMER'])), false);
  equal(partyIsOurCompany(record(['NO_ROLE'])), false);
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

// Covers: 类型列的文字——我方公司排在角色前面；不是我方公司就照角色，没角色叫「暂无合作关系」。
test('businessPartyTypeLabel 把我方公司排在角色前面', () => {
  equal(businessPartyTypeLabel(true, []), '我方公司');
  equal(businessPartyTypeLabel(true, ['CUSTOMER']), '我方公司 / 客户');
  equal(businessPartyTypeLabel(false, ['CUSTOMER', 'SUPPLIER']), '客户 / 供应商');
  equal(businessPartyTypeLabel(false, []), '暂无合作关系');
});

// Covers: 对象页收起写入口用的口径——已停用的公司主体钉着的参与方也算（与类别里的「我方公司」不同，那个只认未停用的）；
// 法人册没取到为空集。
test('partiesPinnedByLegalEntities 认已停用的公司主体', () => {
  const pinned = partiesPinnedByLegalEntities([
    legalEntity({ legalEntityId: 'LE-P1', partyId: 'P1' }),
    legalEntity({ legalEntityId: 'LE-P2', partyId: 'P2', status: 'DEACTIVATED' }),
  ]);
  equal(pinned.has('P1'), true);
  equal(pinned.has('P2'), true);
  equal(pinned.has('P3'), false);
  equal(partiesPinnedByLegalEntities(null).size, 0);
});

// Covers: 注册地先取往来单位自己登的；我方公司取钉着它的公司主体的（已停用的也算）；公司主体没登身份层、法人册没取到都不补。
test('businessPartyRegistrationCountry 我方公司取公司主体的注册地', () => {
  const entity = (partyId: string, status: string, registrationCountry?: string) =>
    legalEntity({ legalEntityId: `LE-${partyId}`, partyId, status, identityLayerRegistered: registrationCountry !== undefined, registrationCountry });
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
