import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PartyRelationshipRecord } from './api';
import {
  countPartyRelationships,
  defaultPartyRelationshipSort,
  partyRelationshipListConditions,
  partyRelationshipNeedsAttention,
  partyRelationshipNoMatchNote,
  partyRelationshipRoleFilterOf,
  partyRelationshipRoleFilterOptions,
  partyRelationshipStatusFilterOptions,
} from './party-relationship-list';
import { labelOf, partyRoleLabels } from './presentation';
import { catalogueConditionsQuery } from '../catalogue-query';

// 本文件钉关系册的列表纯逻辑：检索、角色、状态与表头排序编成读口的条件（ADR-0144，可排维与筛选维照后端 PartyRelationshipPages
// 的声明）；「待查」由名称转写推出来，只筛到手的这一页；「筛出为空」与「登记册为空」是两个事实——票 09 完成判据点名的那一格：
// 关系签筛「候选关系」得筛空文案而不是「0 段」。

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

// Covers: 检索词、角色、状态与排序编成一问——「全部」即那一维缺席；页面缺省的生效起新→旧与读口缺省序（登记时间）不同，总写进
// 查询串；双方两列与认不得的列不在读口的可排维里，按读口缺省序问、不带 sort。
test('partyRelationshipListConditions 按读口声明的维下推', () => {
  equal(
    catalogueConditionsQuery(partyRelationshipListConditions('', 'ALL', 'ALL', defaultPartyRelationshipSort)),
    'sort=-effectiveStartsAt',
  );
  equal(
    catalogueConditionsQuery(partyRelationshipListConditions(' 承运商 ', 'CARRIER_AGENT', 'CANDIDATE', { key: 'effective-start', dir: 1 })),
    'sort=effectiveStartsAt&role=CARRIER_AGENT&status=CANDIDATE&q=%E6%89%BF%E8%BF%90%E5%95%86',
  );
  equal(catalogueConditionsQuery(partyRelationshipListConditions('', 'ALL', 'ALL', { key: 'holder', dir: 1 })), '');
  equal(catalogueConditionsQuery(partyRelationshipListConditions('', 'AUTHORIZED_DELIVERING_PARTY', 'ALL', { key: 'counterparty', dir: -1 })), 'role=AUTHORIZED_DELIVERING_PARTY');
  deepEqual(defaultPartyRelationshipSort, { key: 'effective-start', dir: -1 });
});

// Covers: 待查 = 持有方或相对方名称转写不到；撤销、替代这些终局不算待查。
test('待查判据', () => {
  deepEqual(rows.map(partyRelationshipNeedsAttention), [false, false, true, false]);
  equal(partyRelationshipNoMatchNote, '当前筛选条件下没有匹配的合作关系');
});

// Covers: 票 16——授权交付方在关系册上显中文显示名而不是原码。
test('授权交付方显中文显示名', () => {
  equal(labelOf(partyRoleLabels, 'AUTHORIZED_DELIVERING_PARTY'), '授权交付方');
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

// Covers: 别处带着角色跳过来时角色码怎么落到筛选上——词表里的原样；词表之外的下拉里没有这一项，交「全部」。
test('partyRelationshipRoleFilterOf 只认词表里的角色', () => {
  equal(partyRelationshipRoleFilterOf('CUSTOMER'), 'CUSTOMER');
  equal(partyRelationshipRoleFilterOf('AUTHORIZED_DELIVERING_PARTY'), 'AUTHORIZED_DELIVERING_PARTY');
  equal(partyRelationshipRoleFilterOf('SOMETHING_NEW'), 'ALL');
  // 「全部」不是角色码：交回去的仍是「全部」，不会被当成一个角色下推。
  equal(partyRelationshipRoleFilterOf('ALL'), 'ALL');
});
