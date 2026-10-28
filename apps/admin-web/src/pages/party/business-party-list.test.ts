import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord } from './api';
import {
  businessPartyCountSummary,
  businessPartyNoMatchNote,
  businessPartyStatusFilterOptions,
  countBusinessParties,
  defaultBusinessPartySort,
  filterBusinessParties,
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
  deepEqual(countBusinessParties(rows), { total: 4, byStatus: { REGISTERED: 1, EFFECTIVE: 2, DEACTIVATED: 1 } });
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
  equal(businessPartyCountSummary(4, 4), '共 4 个参与方身份，当前显示 4 个');
  equal(businessPartyCountSummary(4, 0), '共 4 个参与方身份，当前显示 0 个');
  equal(businessPartyNoMatchNote, '当前筛选条件下没有匹配的参与方身份');
});

// Covers: 胶囊选项与封闭词一一对应、「全部」在首位；词从 identityStatusLabels 派生（已登记 / 已生效 / 已停用），本文件不另抄。
test('状态胶囊选项表', () => {
  deepEqual(
    businessPartyStatusFilterOptions.map((option) => option.value),
    ['ALL', 'REGISTERED', 'EFFECTIVE', 'DEACTIVATED'],
  );
  deepEqual(
    businessPartyStatusFilterOptions.map((option) => option.label),
    ['全部', '已登记', '已生效', '已停用'],
  );
});
