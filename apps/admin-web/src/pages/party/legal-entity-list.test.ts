import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { GroupLegalEntityRecord } from './api';
import {
  countLegalEntities,
  defaultLegalEntitySort,
  filterLegalEntities,
  legalEntityCountSummary,
  legalEntityNeedsAttention,
  legalEntityNoMatchNote,
  legalEntityStatusFilterOptions,
  sortLegalEntities,
  type LegalEntityFilter,
} from './legal-entity-list';

// 本文件钉的是筛选、排序与计数都只在已取回的行上做（README 列表页上列通则：不下推成查询参数），
// 且「筛出为空」与「登记册为空」是两个事实——前者由调用页按裁决 1 另显，这里只保证筛选
// 不把行吞掉也不多算。

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
    ...over,
  };
}

const rows: GroupLegalEntityRecord[] = [
  record({ legalEntityId: 'SYN-LE-02', status: 'REGISTERED', effectiveFrom: '2030-01-01T00:00:00Z', registeredAt: '2026-09-16T05:00:00Z' }),
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
  }),
  record({ legalEntityId: 'SYN-LE-04', partyName: undefined, partyNameKnown: false, registeredAt: '2026-09-16T04:00:00Z' }),
];

const ids = (list: GroupLegalEntityRecord[]) => list.map((row) => row.legalEntityId);
const filter = (over: Partial<LegalEntityFilter>): LegalEntityFilter => ({
  search: '',
  status: 'ALL',
  attentionOnly: false,
  ...over,
});

// Covers: 搜索是包含匹配、不分大小写，命中法人标识 / 参与方身份 / 名称任一格；名称未知的行按空串参与、不抛。
test('搜索按包含匹配四格之一', () => {
  deepEqual(ids(filterLegalEntities(rows, filter({ search: 'le-0' }))), ['SYN-LE-02', 'SYN-LE-01', 'SYN-LE-03', 'SYN-LE-04']);
  deepEqual(ids(filterLegalEntities(rows, filter({ search: '停用示例' }))), ['SYN-LE-03']);
  deepEqual(ids(filterLegalEntities(rows, filter({ search: 'retired' }))), ['SYN-LE-03']);
  deepEqual(ids(filterLegalEntities(rows, filter({ search: '  ' }))), ids(rows));
});

// Covers: 状态筛选精确到封闭词；ALL 不筛；筛出为空交回空数组而不是原行。
test('状态筛选精确匹配封闭词', () => {
  deepEqual(ids(filterLegalEntities(rows, filter({ status: 'DEACTIVATED' }))), ['SYN-LE-03']);
  deepEqual(ids(filterLegalEntities(rows, filter({ status: 'REGISTERED' }))), ['SYN-LE-02']);
  deepEqual(ids(filterLegalEntities(rows, filter({ search: 'LE-01', status: 'DEACTIVATED' }))), []);
});

// Covers: 待补 = 身份层两格没有，或参与方册查无此身份；停用本身不算待补。只看待补与状态筛选叠加。
test('待补判据与只看待补', () => {
  deepEqual(rows.map(legalEntityNeedsAttention), [false, false, true, true]);
  deepEqual(ids(filterLegalEntities(rows, filter({ attentionOnly: true }))), ['SYN-LE-03', 'SYN-LE-04']);
  deepEqual(ids(filterLegalEntities(rows, filter({ attentionOnly: true, status: 'EFFECTIVE' }))), ['SYN-LE-04']);
});

// Covers: 计数按状态分格、待补单独计，数的是传进来的这一页；词表之外的状态码不进任何格也不抛。
test('countLegalEntities 按状态与待补计数', () => {
  deepEqual(countLegalEntities(rows), {
    total: 4,
    byStatus: { REGISTERED: 1, EFFECTIVE: 2, DEACTIVATED: 1 },
    attention: 2,
  });
  deepEqual(countLegalEntities([record({ status: 'SOMETHING_NEW' })]).byStatus, { REGISTERED: 0, EFFECTIVE: 0, DEACTIVATED: 0 });
});

// Covers: 默认按登记时间新→旧；四列各按其键，方向随 dir；排序不改原数组；认不得的键原样交回。
test('表头排序各按其键与方向，且不改原数组', () => {
  const before = ids(rows);
  deepEqual(defaultLegalEntitySort, { key: 'registered-at', dir: -1 });
  deepEqual(ids(sortLegalEntities(rows, defaultLegalEntitySort)), ['SYN-LE-02', 'SYN-LE-01', 'SYN-LE-04', 'SYN-LE-03']);
  deepEqual(ids(sortLegalEntities(rows, { key: 'legal-entity', dir: 1 })), ['SYN-LE-01', 'SYN-LE-02', 'SYN-LE-03', 'SYN-LE-04']);
  deepEqual(ids(sortLegalEntities(rows, { key: 'legal-entity', dir: -1 })), ['SYN-LE-04', 'SYN-LE-03', 'SYN-LE-02', 'SYN-LE-01']);
  deepEqual(ids(sortLegalEntities(rows, { key: 'effective-from', dir: 1 })), ['SYN-LE-03', 'SYN-LE-01', 'SYN-LE-04', 'SYN-LE-02']);
  // 名称未知按空串排最前；同名两行保持原序。
  deepEqual(ids(sortLegalEntities(rows, { key: 'party-name', dir: 1 })), ['SYN-LE-04', 'SYN-LE-03', 'SYN-LE-02', 'SYN-LE-01']);
  deepEqual(ids(sortLegalEntities(rows, { key: 'no-such-column', dir: 1 })), before);
  deepEqual(ids(rows), before);
});

// Covers: 时刻比按解析值不按字典序——端点用 RFC3339Nano 剪尾零，同一秒内小数位数不定，字典序会把
// `55Z` 排到 `55.939Z` 之后、`55.9Z` 排到 `55.93Z` 之后（评审 ← 通道 3 · 钉 71c7d5a3 · Standards 1）。
// 解析不了的值退回字符串比，不抛、不丢行。
test('同一秒内小数位数不定的时刻按真实先后排', () => {
  const sameSecond = [
    record({ legalEntityId: 'A', registeredAt: '2026-09-16T04:27:55.9Z' }),
    record({ legalEntityId: 'B', registeredAt: '2026-09-16T04:27:55Z' }),
    record({ legalEntityId: 'C', registeredAt: '2026-09-16T04:27:55.939Z' }),
    record({ legalEntityId: 'D', registeredAt: '2026-09-16T04:27:55.93Z' }),
  ];
  deepEqual(ids(sortLegalEntities(sameSecond, defaultLegalEntitySort)), ['C', 'D', 'A', 'B']);
  const unparsable = [record({ legalEntityId: 'X', registeredAt: 'not-a-time' }), record({ legalEntityId: 'Y' })];
  deepEqual(ids(sortLegalEntities(unparsable, defaultLegalEntitySort)), ['X', 'Y']);
});

// Covers: 裁决 1——筛出为空不是空态。计数摘要总数与当前显示数分开报，筛空时照显「共 N 个，当前显示 0 个」，
// 表格区那一行说的是「当前条件下无匹配」而不是「登记册为空」（两者续办不同：前者改条件，后者去登记）。
test('计数摘要分报总数与当前显示数，筛空提示不冒充空态', () => {
  equal(legalEntityCountSummary(4, 4), '共 4 个责任法人，当前显示 4 个');
  equal(legalEntityCountSummary(4, 0), '共 4 个责任法人，当前显示 0 个');
  equal(legalEntityNoMatchNote, '当前筛选条件下没有匹配的法人');
});

// Covers: 胶囊选项与封闭词一一对应、「全部」在首位——页面直接渲染这张表，不另抄一份。
test('状态胶囊选项表', () => {
  deepEqual(
    legalEntityStatusFilterOptions.map((option) => option.value),
    ['ALL', 'REGISTERED', 'EFFECTIVE', 'DEACTIVATED'],
  );
  // 词从 identityStatusLabels 派生：三格 CONTEXT 原词（已登记 / 已生效 / 已停用）不在本文件另抄。
  deepEqual(
    legalEntityStatusFilterOptions.map((option) => option.label),
    ['全部', '已登记', '已生效', '已停用'],
  );
});
