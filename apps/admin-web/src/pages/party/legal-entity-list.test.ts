import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { GroupLegalEntityRecord, RegistrationNumberTypeRecord } from './api';
import {
  defaultLegalEntitySort,
  legalEntityListConditions,
  legalEntityNeedsAttention,
  legalEntityNoMatchNote,
  legalEntityStatusFilterOptions,
  registrationCountryOptions,
} from './legal-entity-list';
import { catalogueConditionsQuery } from '../catalogue-query';

// 本文件钉法人册的列表纯逻辑：检索、状态、注册地与表头排序编成读口的条件（ADR-0144，可排维与筛选维照后端 GroupLegalEntityPages
// 的声明）；「待处理」由身份层与名称转写推出来，只筛到手的这一页；「筛出为空」与「登记册为空」是两个事实，由调用页按裁决 1 另显。

function record(over: Partial<GroupLegalEntityRecord>): GroupLegalEntityRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'SYN-PARTY-OPERATOR-01',
    partyName: '合成运营方一号（演示）',
    partyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-LE-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    identityLayerRegistered: true,
    registrationCountry: 'CN',
    lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91310000SYN00001' }],
    ...over,
  };
}

const rows: GroupLegalEntityRecord[] = [
  record({
    legalEntityId: 'SYN-LE-02',
    status: 'REGISTERED',
    effectiveFrom: '2030-01-01T00:00:00Z',
    registeredAt: '2026-09-16T05:00:00Z',
    registrationCountry: 'US',
    lifetimeRegistrationNumbers: [{ typeCode: 'EIN', number: '91310000SYN00002' }],
  }),
  record({ legalEntityId: 'SYN-LE-01' }),
  record({
    legalEntityId: 'SYN-LE-03',
    status: 'DEACTIVATED',
    partyId: 'SYN-PARTY-RETIRED-01',
    partyName: '合成停用示例（演示）',
    deactivatedAt: '2026-06-01T00:00:00Z',
    deactivationBasis: 'SYN-DEACT-01',
    effectiveFrom: '2025-12-01T00:00:00Z',
    registeredAt: '2026-09-16T03:00:00Z',
    identityLayerRegistered: false,
    registrationCountry: undefined,
    lifetimeRegistrationNumbers: undefined,
  }),
  record({ legalEntityId: 'SYN-LE-04', partyName: undefined, partyNameKnown: false, registeredAt: '2026-09-16T04:00:00Z' }),
];

const ids = (list: GroupLegalEntityRecord[]) => list.map((row) => row.legalEntityId);

// Covers: 检索词、状态、注册地与排序编成一问——「全部」即那一维缺席，登记时间新→旧是读口的缺省序、不写进查询串；生效时间两向
// 都下推；公司主体（名称）列与认不得的列不在读口的可排维里，按缺省序问。
test('legalEntityListConditions 按读口声明的维下推', () => {
  equal(catalogueConditionsQuery(legalEntityListConditions('', 'ALL', 'ALL', defaultLegalEntitySort)), '');
  equal(
    catalogueConditionsQuery(legalEntityListConditions(' SYN-LE ', 'EFFECTIVE', 'CN', { key: 'effective-from', dir: -1 })),
    'sort=-effectiveFrom&registrationCountry=CN&status=EFFECTIVE&q=SYN-LE',
  );
  equal(catalogueConditionsQuery(legalEntityListConditions('', 'ALL', 'ALL', { key: 'registered-at', dir: 1 })), 'sort=registeredAt');
  equal(catalogueConditionsQuery(legalEntityListConditions('', 'ALL', 'US', { key: 'party-name', dir: 1 })), 'registrationCountry=US');
  deepEqual(defaultLegalEntitySort, { key: 'registered-at', dir: -1 });
});

function numberType(over: Partial<RegistrationNumberTypeRecord>): RegistrationNumberTypeRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    countryCode: 'CN',
    typeCode: 'USCC',
    revision: 1,
    typeName: '统一社会信用代码',
    layer: 'IDENTITY',
    formatPattern: '^[0-9A-Z]{18}$',
    basis: 'SYN-TYPE-BASIS-01',
    status: 'EFFECTIVE',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 注册地选项是目录里身份层类型的国家，并上这一页行上的与此刻选中的那一个（目录只取一页、类型后来撤了都不丢）；资料层
// 类型的国家不算；去重、按码排，「全部」不进选项。
test('registrationCountryOptions 并起目录、本页与选中的国家', () => {
  const types = [numberType({}), numberType({ countryCode: 'SG', typeCode: 'UEN' }), numberType({ countryCode: 'JP', layer: 'PROFILE' })];
  deepEqual(registrationCountryOptions(types, rows, 'ALL'), ['CN', 'SG', 'US']);
  deepEqual(registrationCountryOptions([], rows, 'HK'), ['CN', 'HK', 'US']);
  deepEqual(registrationCountryOptions([], [], 'ALL'), []);
});

// Covers: 待补 = 身份层两格没有，或参与方册查无此身份；停用本身不算待补。
test('待补判据', () => {
  deepEqual(rows.map(legalEntityNeedsAttention), [false, false, true, true]);
  equal(legalEntityNoMatchNote, '当前筛选条件下没有匹配的公司主体');
});

// Covers: 胶囊选项与封闭词一一对应、「全部」在首位——页面直接渲染这张表，不另抄一份。
test('状态胶囊选项表', () => {
  deepEqual(
    legalEntityStatusFilterOptions.map((option) => option.value),
    ['ALL', 'REGISTERED', 'EFFECTIVE', 'DEACTIVATED'],
  );
  // 词从 identityStatusLabels 派生：三格显示词（待生效 / 已生效 / 已停用）不在本文件另抄。
  deepEqual(
    legalEntityStatusFilterOptions.map((option) => option.label),
    ['全部', '待生效', '已生效', '已停用'],
  );
});
