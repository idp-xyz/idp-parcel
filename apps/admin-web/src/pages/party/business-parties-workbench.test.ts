import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord, PartyRelationshipRecord } from './api';
import { countBusinessParties, indexPartyRoles, ourCompanyPartyIds, rolesOfParty } from './business-party-list';
import {
  formatCount,
  partyAttentionCount,
  partyFilteredFooterCountText,
  partyFooterCountText,
  partyFooterPageText,
  partyRelationshipCellOf,
  partyStatusOptionsOf,
  partyTileKindOf,
  partyTypeChipsFit,
  partyTypeChipsOf,
  tallyPartyRelationships,
} from './business-parties-workbench';

// 本文件钉的是往来单位页新版式（票 business-party-redesign/02）的列表纯逻辑：筛选项上的数随别的条件走、「合作关系」一格与
// 类型列同一口径、图标块取哪一种、类型芯片摆不摆得下、视图签右端待办的数、表底两句话。

function party(partyId: string, over: Partial<BusinessPartyRecord> = {}): BusinessPartyRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    partyId,
    partyName: `合成单位 ${partyId}`,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    ...over,
  };
}

function relationship(relationshipId: string, holderId: string, role: string, counterpartyId: string, status = 'EFFECTIVE'): PartyRelationshipRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    relationshipId,
    revision: 1,
    holderId,
    holderNameKnown: true,
    counterpartyId,
    counterpartyNameKnown: true,
    role,
    scope: 'SYN-SCOPE-01',
    basis: 'SYN-BASIS-01',
    status,
    effectiveStartsAt: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
  };
}

const relationships = [
  relationship('R1', 'SYN-P-CUSTOMER-01', 'CUSTOMER', 'SYN-P-OURS-01'),
  relationship('R2', 'SYN-P-CUSTOMER-02', 'CUSTOMER', 'SYN-P-OURS-01', 'CANDIDATE'),
  relationship('R3', 'SYN-P-AGENT-01', 'CARRIER_AGENT', 'SYN-P-OURS-01'),
  relationship('R4', 'SYN-P-OURS-01', 'ACCOUNT_HOLDER', 'SYN-P-CARRIER-01'),
  relationship('R5', 'SYN-P-CUSTOMER-01', 'CUSTOMER', 'SYN-P-OURS-01', 'EXPIRED'),
];

// Covers: 每家单位名下在用的关系按两侧分开记，候选另记；到期、撤销的不数。「合作关系」一格只数它担任角色的，
// 只当相对方的单位这一格空着（它的类型也是「暂无合作关系」）；我方公司两侧都数。
test('合作关系一格与类型列同一口径', () => {
  const tallies = tallyPartyRelationships(relationships);
  deepEqual(tallies.get('SYN-P-CUSTOMER-01'), { held: 1, heldCandidates: 0, toward: 0, towardCandidates: 0 });
  deepEqual(tallies.get('SYN-P-OURS-01'), { held: 1, heldCandidates: 0, toward: 3, towardCandidates: 1 });
  deepEqual(partyRelationshipCellOf(tallies, 'SYN-P-CUSTOMER-01', false), { count: 1, candidates: 0 });
  deepEqual(partyRelationshipCellOf(tallies, 'SYN-P-CUSTOMER-02', false), { count: 1, candidates: 1 });
  equal(partyRelationshipCellOf(tallies, 'SYN-P-CARRIER-01', false), null);
  deepEqual(partyRelationshipCellOf(tallies, 'SYN-P-OURS-01', true), { count: 4, candidates: 1 });
  equal(partyRelationshipCellOf(tallies, 'SYN-P-NOBODY-01', false), null);
  equal(tallyPartyRelationships(null).size, 0);
});

// Covers: 图标块——我方公司优先；否则取持有的角色里词表序最前的；没有角色或码不在词表里的是 NONE。
test('partyTileKindOf 取图标块的种类', () => {
  equal(partyTileKindOf(true, ['CUSTOMER']), 'OURS');
  equal(partyTileKindOf(false, ['SUPPLIER', 'CUSTOMER']), 'CUSTOMER');
  equal(partyTileKindOf(false, ['CARRIER_AGENT']), 'CARRIER_AGENT');
  equal(partyTileKindOf(false, []), 'NONE');
  equal(partyTileKindOf(false, ['FUTURE_ROLE']), 'NONE');
});

const register = [
  party('SYN-P-OURS-01'),
  party('SYN-P-CUSTOMER-01'),
  party('SYN-P-CUSTOMER-02', { status: 'REGISTERED' }),
  party('SYN-P-AGENT-01', { status: 'DEACTIVATED' }),
  party('SYN-P-CARRIER-01'),
];
const roles = indexPartyRoles(relationships);
const ours = ourCompanyPartyIds([
  {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    kind: 'RESPONSIBLE_LEGAL_ENTITY',
    partyId: 'SYN-P-OURS-01',
    partyNameKnown: true,
    status: 'EFFECTIVE',
    revision: 1,
    basis: 'SYN-REG-BASIS-LE-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-01-02T00:00:00Z',
    identityLayerRegistered: false,
  },
]);

// Covers: 类型芯片——摆哪几枚看整册（整册里没有的类型不摆），数看当前条件下答回的行；条件下数到零的留在原位、点不动；
// 选中的那一枚总在；次序是全部、各角色、我方公司、暂无合作关系。
test('partyTypeChipsOf 摆哪几枚看整册，数看当前条件', () => {
  const whole = countBusinessParties(register, roles, ours);
  const all = partyTypeChipsOf(whole, whole, 'ALL');
  deepEqual(
    all.map((chip) => [chip.type, chip.label, chip.count, chip.group]),
    [
      ['ALL', '全部', 5, 'all'],
      ['CUSTOMER', '客户', 2, 'role'],
      ['CARRIER_AGENT', '承运商代理', 1, 'role'],
      ['ACCOUNT_HOLDER', '渠道账号持有', 1, 'role'],
      ['OURS', '我方公司', 1, 'other'],
      ['NONE', '暂无合作关系', 1, 'other'],
    ],
  );
  equal(all.every((chip) => !chip.disabled), true);

  // 状态筛到「待生效」：读口只答回一家客户
  const conditioned = countBusinessParties([register[2]], roles, ours);
  const narrowed = partyTypeChipsOf(whole, conditioned, 'CUSTOMER');
  deepEqual(
    narrowed.map((chip) => [chip.type, chip.count, chip.active, chip.disabled]),
    [
      ['ALL', 1, false, false],
      ['CUSTOMER', 1, true, false],
      ['CARRIER_AGENT', 0, false, true],
      ['ACCOUNT_HOLDER', 0, false, true],
      ['OURS', 0, false, true],
      ['NONE', 0, false, true],
    ],
  );
  // 选中的那一枚数到零也点得动（要能点回去），整册里没有的类型选中时也摆出来
  const empty = countBusinessParties([], roles, ours);
  equal(partyTypeChipsOf(whole, empty, 'CUSTOMER').find((chip) => chip.type === 'CUSTOMER')?.disabled, false);
  equal(partyTypeChipsOf(whole, whole, 'SUPPLIER').some((chip) => chip.type === 'SUPPLIER'), true);
  equal(partyTypeChipsOf(whole, whole, 'ALL').some((chip) => chip.type === 'SUPPLIER'), false);
});

// Covers: 状态下拉上的数随检索词与类型走、不随状态自己走。没有检索词按整册数；有检索词且状态是「全部」按读口答回的行数；
// 有检索词又选了状态，别的状态数不出，只给选中的一项；整册没读全一律不给数。
test('partyStatusOptionsOf 的数随别的条件走', () => {
  const anyType = () => true;
  const isCustomer = (row: BusinessPartyRecord) => rolesOfParty(roles, row.partyId).includes('CUSTOMER');
  const counts = (options: ReturnType<typeof partyStatusOptionsOf>) => options.map((option) => [option.value, option.count]);

  deepEqual(counts(partyStatusOptionsOf({ whole: register, conditioned: register, keyword: '', status: 'ALL', ofType: anyType })), [
    ['ALL', 5],
    ['REGISTERED', 1],
    ['EFFECTIVE', 3],
    ['DEACTIVATED', 1],
  ]);
  // 选了状态但没有检索词：仍按整册数，别的状态照样有数
  deepEqual(counts(partyStatusOptionsOf({ whole: register, conditioned: [register[2]], keyword: '', status: 'REGISTERED', ofType: isCustomer })), [
    ['ALL', 2],
    ['REGISTERED', 1],
    ['EFFECTIVE', 1],
    ['DEACTIVATED', 0],
  ]);
  // 有检索词、状态「全部」：按答回的行
  deepEqual(
    counts(partyStatusOptionsOf({ whole: register, conditioned: [register[1], register[2]], keyword: 'CUSTOMER', status: 'ALL', ofType: anyType })),
    [
      ['ALL', 2],
      ['REGISTERED', 1],
      ['EFFECTIVE', 1],
      ['DEACTIVATED', 0],
    ],
  );
  // 有检索词又选了状态：只有选中的那一项数得出
  deepEqual(
    counts(partyStatusOptionsOf({ whole: register, conditioned: [register[2]], keyword: 'CUSTOMER', status: 'REGISTERED', ofType: anyType })),
    [
      ['ALL', null],
      ['REGISTERED', 1],
      ['EFFECTIVE', null],
      ['DEACTIVATED', null],
    ],
  );
  equal(
    partyStatusOptionsOf({ whole: null, conditioned: register, keyword: '', status: 'ALL', ofType: anyType }).every((option) => option.count === null),
    true,
  );
});

// Covers: 表底两句——总数千位分节；读口不给总数时只说本页几家、只说第几页。
test('表底的条数与页码', () => {
  equal(formatCount(1286), '1,286');
  equal(partyFooterCountText(1286, 200, '家'), '共 1,286 家');
  equal(partyFooterCountText(null, 200, '家'), '本页 200 家');
  equal(partyFooterPageText(2, 200, 1286), '第 2 / 7 页');
  equal(partyFooterPageText(1, 200, 0), '第 1 / 1 页');
  equal(partyFooterPageText(3, 200, null), '第 3 页');
});

// Covers: 带着页面上筛的那一项时表底怎么说——读得全时说筛出来的条数；读不全时总数照读口答的说，页面上筛了另说本页筛出几条，
// 没筛就只有总数；读口不给总数时说本页几条。
test('partyFilteredFooterCountText 分读得全与读不全', () => {
  const text = (over: Partial<Parameters<typeof partyFilteredFooterCountText>[0]>) =>
    partyFilteredFooterCountText({ exact: true, total: 12, pageRows: 12, shown: 12, pageFiltered: false, unit: '家', ...over });
  equal(text({}), '共 12 家');
  equal(text({ shown: 3, pageFiltered: true }), '共 3 家');
  equal(text({ exact: false, total: 1286, pageRows: 200, shown: 200 }), '共 1,286 家');
  equal(text({ exact: false, total: 1286, pageRows: 200, shown: 41, pageFiltered: true }), '共 1,286 家，本页筛出 41 家');
  equal(text({ exact: false, total: null, pageRows: 200, shown: 41, pageFiltered: true, unit: '段' }), '本页 200 段，本页筛出 41 段');
});

// Covers: 视图签右端待办的数——册读全了从手上的行数（数到零就是零）；读不全时用带状态条件那一问答的总数；两样都没有是 null，
// 不拿 0 冒充。
test('partyAttentionCount 的两个来源', () => {
  const isRegistered = (row: BusinessPartyRecord) => row.status === 'REGISTERED';
  equal(partyAttentionCount(register, isRegistered, null), 1);
  equal(partyAttentionCount(register, isRegistered, 99), 1);
  equal(partyAttentionCount([register[0]], isRegistered, null), 0);
  equal(partyAttentionCount(null, isRegistered, 37), 37);
  equal(partyAttentionCount(null, isRegistered, null), null);
});

// Covers: 类型芯片摆不摆得下工具条那一行——宽度还没量到不摆（收成下拉）；够宽摆，不够宽收；芯片多了、数的位数多了要的宽度随之变大。
test('partyTypeChipsFit 按工具条宽度判', () => {
  const whole = countBusinessParties(register, roles, ours);
  const chips = partyTypeChipsOf(whole, whole, 'ALL');
  equal(partyTypeChipsFit(null, chips), false);
  equal(partyTypeChipsFit(4000, chips), true);
  equal(partyTypeChipsFit(700, chips), false);
  // 找出刚好放得下的宽度：少一个像素就放不下；芯片少了，同一个宽度下更容易放下。
  let width = 700;
  while (!partyTypeChipsFit(width, chips)) width += 1;
  equal(partyTypeChipsFit(width - 1, chips), false);
  equal(partyTypeChipsFit(width - 1, chips.slice(0, -1)), true);
  const wider = chips.map((chip) => ({ ...chip, count: 12_345 }));
  equal(partyTypeChipsFit(width, wider), false);
});
