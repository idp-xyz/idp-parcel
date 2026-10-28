import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PartyRelationshipRecord } from './api';
import {
  countPartyRelationships,
  defaultPartyRelationshipSort,
  filterPartyRelationships,
  partyRelationshipCountSummary,
  partyRelationshipNeedsAttention,
  partyRelationshipNoMatchNote,
  partyRelationshipRoleFilterOptions,
  partyRelationshipStatusFilterOptions,
  sortPartyRelationships,
} from './party-relationship-list';
import { labelOf, partyRoleLabels } from './presentation';

// 本文件钉的是关系册的筛选、排序与计数都只在已取回的行上做（README 列表页上列通则：不下推成查询参数），
// 且「筛出为空」与「登记册为空」是两个事实——票 09 完成判据点名的那一格：关系签筛「候选关系」得筛空文案而不是「0 段」。

function record(over: Partial<PartyRelationshipRecord>): PartyRelationshipRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    relationshipId: 'SYN-REL-01',
    revision: 1,
    holderId: 'SYN-PARTY-OPERATOR-01',
    holderName: '合成运营方一号（演示）',
    holderNameKnown: true,
    counterpartyId: 'SYN-PARTY-CARRIER-02',
    counterpartyName: '合成承运商二号（演示）',
    counterpartyNameKnown: true,
    role: 'SUPPLIER',
    scope: 'CN-SG',
    basis: 'SYN-REL-BASIS-01',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    ...over,
  };
}

const rows: PartyRelationshipRecord[] = [
  record({ relationshipId: 'SYN-REL-01' }),
  record({
    relationshipId: 'SYN-REL-02',
    role: 'CARRIER_AGENT',
    status: 'REVOKED',
    holderId: 'SYN-PARTY-AGENT-04',
    holderName: '合成代理四号（演示）',
    effectiveStartsAt: '2025-06-01T00:00:00Z',
    effectiveEndsAt: '2026-03-01T00:00:00Z',
    endedAt: '2026-03-01T00:00:00Z',
    endBasis: 'SYN-REVOKE-01',
    registeredAt: '2026-09-16T03:00:00Z',
  }),
  record({
    relationshipId: 'SYN-REL-03',
    role: 'ACCOUNT_HOLDER',
    status: 'SUPERSEDED',
    successorId: 'SYN-REL-04',
    effectiveStartsAt: '2026-05-01T00:00:00Z',
    counterpartyId: 'SYN-PARTY-GONE-09',
    counterpartyName: undefined,
    counterpartyNameKnown: false,
    registeredAt: '2026-09-16T05:00:00Z',
  }),
  record({
    relationshipId: 'SYN-REL-04',
    role: 'ACCOUNT_HOLDER',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-08-01T00:00:00Z',
    registeredAt: '2026-09-16T04:00:00Z',
  }),
];

const ids = (list: PartyRelationshipRecord[]) => list.map((row) => row.relationshipId);
const all = { search: '', role: 'ALL', status: 'ALL', attentionOnly: false } as const;

// Covers: 搜索是包含匹配、不分大小写，命中关系标识 / 持有方标识与名称 / 相对方标识与名称 / 角色 / 状态原名任一格；
// 名称未知的行按空串参与、不抛。
test('搜索按包含匹配多格之一', () => {
  deepEqual(ids(filterPartyRelationships(rows, { ...all, search: 'rel-0' })), ids(rows));
  deepEqual(ids(filterPartyRelationships(rows, { ...all, search: '承运商二号' })), ['SYN-REL-01', 'SYN-REL-02', 'SYN-REL-04']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, search: 'account_holder' })), ['SYN-REL-03', 'SYN-REL-04']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, search: 'revoked' })), ['SYN-REL-02']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, search: '  ' })), ids(rows));
});

// Covers: 角色与状态各精确到封闭词、可叠加；ALL 不筛；筛「候选关系」在没有候选行时交回空数组而不是原行。
test('角色与状态筛选精确匹配封闭词且可叠加', () => {
  deepEqual(ids(filterPartyRelationships(rows, { ...all, role: 'ACCOUNT_HOLDER' })), ['SYN-REL-03', 'SYN-REL-04']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, status: 'REVOKED' })), ['SYN-REL-02']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, role: 'ACCOUNT_HOLDER', status: 'EFFECTIVE' })), ['SYN-REL-04']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, status: 'CANDIDATE' })), []);
});

// Covers: 待查 = 持有方或相对方名称转写不到；撤销、替代这些终局不算待查。只看待查与状态筛选叠加。
test('待查判据与只看待查', () => {
  deepEqual(rows.map(partyRelationshipNeedsAttention), [false, false, true, false]);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, attentionOnly: true })), ['SYN-REL-03']);
  deepEqual(ids(filterPartyRelationships(rows, { ...all, attentionOnly: true, status: 'EFFECTIVE' })), []);
});

// Covers: 计数按状态分格、待查单独计，数的是传进来的这一页；词表之外的状态码不进任何格也不抛。
test('countPartyRelationships 按状态与待查计数', () => {
  deepEqual(countPartyRelationships(rows), {
    total: 4,
    byStatus: { CANDIDATE: 0, EFFECTIVE: 2, EXPIRED: 0, REVOKED: 1, SUPERSEDED: 1 },
    attention: 1,
  });
  deepEqual(countPartyRelationships([record({ status: 'SOMETHING_NEW' })]).byStatus, {
    CANDIDATE: 0,
    EFFECTIVE: 0,
    EXPIRED: 0,
    REVOKED: 0,
    SUPERSEDED: 0,
  });
});

// Covers: 票 16——授权交付方在关系册上显中文显示名而不是原码，角色筛选筛得出它的行。
test('授权交付方显中文显示名、按它筛得出行', () => {
  equal(labelOf(partyRoleLabels, 'AUTHORIZED_DELIVERING_PARTY'), '授权交付方');
  const delivery = record({
    relationshipId: 'SYN-REL-05',
    role: 'AUTHORIZED_DELIVERING_PARTY',
    scope: 'customerAccount=SYN-ACCOUNT-01&node=SYN-NODE-SHA-HUB',
  });
  deepEqual(ids(filterPartyRelationships([...rows, delivery], { ...all, role: 'AUTHORIZED_DELIVERING_PARTY' })), ['SYN-REL-05']);
});

// Covers: 默认按生效起新→旧（票 09 裁决 1 同判据）；四列各按其键，方向随 dir；双方按标识排；排序不改原数组；认不得的键（含不上列的
// 登记时间）原样交回。
test('表头排序各按其键与方向，且不改原数组', () => {
  const before = ids(rows);
  deepEqual(defaultPartyRelationshipSort, { key: 'effective-start', dir: -1 });
  deepEqual(ids(sortPartyRelationships(rows, defaultPartyRelationshipSort)), ['SYN-REL-04', 'SYN-REL-03', 'SYN-REL-01', 'SYN-REL-02']);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'relationship', dir: 1 })), ['SYN-REL-01', 'SYN-REL-02', 'SYN-REL-03', 'SYN-REL-04']);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'relationship', dir: -1 })), ['SYN-REL-04', 'SYN-REL-03', 'SYN-REL-02', 'SYN-REL-01']);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'holder', dir: 1 })), ['SYN-REL-02', 'SYN-REL-01', 'SYN-REL-03', 'SYN-REL-04']);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'counterparty', dir: -1 })), ['SYN-REL-03', 'SYN-REL-01', 'SYN-REL-02', 'SYN-REL-04']);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'registered-at', dir: -1 })), before);
  deepEqual(ids(sortPartyRelationships(rows, { key: 'no-such-column', dir: 1 })), before);
  deepEqual(ids(rows), before);
});

// Covers: 时刻比按解析值不按字典序（RFC3339Nano 剪尾零，同一秒内小数位数不定）；解析不了的值退回字符串比，不抛、不丢行。
test('同一秒内小数位数不定的时刻按真实先后排', () => {
  const sameSecond = [
    record({ relationshipId: 'A', effectiveStartsAt: '2026-09-16T04:27:55.9Z' }),
    record({ relationshipId: 'B', effectiveStartsAt: '2026-09-16T04:27:55Z' }),
    record({ relationshipId: 'C', effectiveStartsAt: '2026-09-16T04:27:55.939Z' }),
    record({ relationshipId: 'D', effectiveStartsAt: '2026-09-16T04:27:55.93Z' }),
  ];
  deepEqual(ids(sortPartyRelationships(sameSecond, defaultPartyRelationshipSort)), ['C', 'D', 'A', 'B']);
  const unparsable = [record({ relationshipId: 'X', effectiveStartsAt: 'not-a-time' }), record({ relationshipId: 'Y' })];
  deepEqual(ids(sortPartyRelationships(unparsable, defaultPartyRelationshipSort)), ['X', 'Y']);
});

// Covers: 票 01 裁决 1——筛出为空不是空态。计数摘要总数与当前显示数分开报（单位沿用读签原词「段」），筛空时照显
// 「共 N 段，当前显示 0 段」，表格区那一行说的是「当前条件下无匹配」而不是「登记册为空」。
test('计数摘要分报总数与当前显示数，筛空提示不冒充空态', () => {
  equal(partyRelationshipCountSummary(4, 4), '共 4 段合作关系，当前显示 4 段');
  equal(partyRelationshipCountSummary(4, 0), '共 4 段合作关系，当前显示 0 段');
  equal(partyRelationshipNoMatchNote, '当前筛选条件下没有匹配的合作关系');
});

// Covers: 选项表与封闭词一一对应、默认项在首位；角色词从 partyRoleLabels、状态词从 relationshipStatusLabels 派生
// （CANDIDATE 显示为「待确认」），本文件不另抄。
test('筛选选项表', () => {
  deepEqual(
    partyRelationshipRoleFilterOptions.map((option) => option.value),
    ['ALL', 'CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT', 'RESELLER', 'ACCOUNT_HOLDER', 'AUTHORIZED_DELIVERING_PARTY'],
  );
  deepEqual(
    partyRelationshipRoleFilterOptions.map((option) => option.label),
    ['全部角色', '客户', '供应商', '承运商代理', '转售', '渠道账号持有', '授权交付方'],
  );
  deepEqual(
    partyRelationshipStatusFilterOptions.map((option) => option.value),
    ['ALL', 'CANDIDATE', 'EFFECTIVE', 'EXPIRED', 'REVOKED', 'SUPERSEDED'],
  );
  deepEqual(
    partyRelationshipStatusFilterOptions.map((option) => option.label),
    ['全部', '待确认', '已生效', '已到期', '已撤销', '已替代'],
  );
});
