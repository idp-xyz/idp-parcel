import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord } from './api';
import {
  facetCountsOf,
  facetOptionShown,
  facetTotal,
  formatCount,
  partyColumnLayoutOf,
  partyFilteredFooterCountText,
  partyFooterCountText,
  partyFooterPageText,
  partyLiveRelationshipsOf,
  partyRelationshipCellOf,
  partyStatusOptionsOf,
  partyTileKindOf,
  partyTileKindOfRow,
  partyTypeAllCount,
  partyTypeChipsFit,
  partyTypeChipsOf,
  statusCountsOf,
} from './business-parties-workbench';

// 本文件钉的是往来单位页新版式（票 business-party-redesign/02）的列表纯逻辑：筛选项上的数取读口的分面（票 09）、「合作关系」
// 一格与类型列同一口径、图标块取哪一种、类型芯片摆不摆得下、各列怎么排、表底两句话。

/** 一行上与类型有关的两格：读口答的类别，与两侧的关系段数（在用的、其中待确认的）。 */
function row(categories: string[], held: [number, number] = [0, 0], faced: [number, number] = [0, 0]): Pick<BusinessPartyRecord, 'categories' | 'relationshipCounts'> {
  return { categories, relationshipCounts: { asHolder: { live: held[0], candidate: held[1] }, asCounterparty: { live: faced[0], candidate: faced[1] } } };
}

const counts = (entries: Record<string, number>) => new Map(Object.entries(entries));

// Covers: 从答复的分面里取一维——答了的交各值的数（零也在）；这一维没答、整格分面没有、答复还没到，都交 null，不交空表。
test('facetCountsOf 取一维，没答交 null', () => {
  const facets = { status: [{ value: 'EFFECTIVE', count: 22 }, { value: 'DEACTIVATED', count: 0 }] };
  deepEqual([...(facetCountsOf(facets, 'status') ?? [])], [['EFFECTIVE', 22], ['DEACTIVATED', 0]]);
  equal(facetCountsOf(facets, 'categories'), null);
  equal(facetCountsOf(undefined, 'status'), null);
  equal(facetCountsOf(null, 'status'), null);
});

// Covers: 状态这样的单值维，「全部」是各值之和；各值照分面给；分面没答一律不给数，不拿 0 占位。
test('statusCountsOf：「全部」是各状态之和，没答不给数', () => {
  const countOf = statusCountsOf(counts({ REGISTERED: 1, EFFECTIVE: 22, DEACTIVATED: 0 }));
  equal(countOf('ALL'), 23);
  equal(countOf('EFFECTIVE'), 22);
  equal(countOf('DEACTIVATED'), 0);
  equal(countOf('NO_SUCH_STATUS'), null);
  equal(facetTotal(counts({})), 0);
  equal(facetTotal(null), null);
  const unanswered = statusCountsOf(null);
  equal(unanswered('ALL'), null);
  equal(unanswered('EFFECTIVE'), null);
});

// Covers: 筛选项摆不摆——整册里一条都没有的值不摆；选中的那一项总在；没有分面可看时都摆。
test('facetOptionShown 看整册的分面', () => {
  const whole = counts({ CANDIDATE: 3, EFFECTIVE: 23, EXPIRED: 0 });
  equal(facetOptionShown(whole, 'CANDIDATE', 'ALL'), true);
  equal(facetOptionShown(whole, 'EXPIRED', 'ALL'), false);
  equal(facetOptionShown(whole, 'REVOKED', 'ALL'), false);
  equal(facetOptionShown(whole, 'EXPIRED', 'EXPIRED'), true);
  equal(facetOptionShown(null, 'EXPIRED', 'ALL'), true);
});

// Covers: 「合作关系」一格取行上答的段数，两侧都算——只当相对方的单位照样有数（它名下确有在用的关系）；待确认另数；
// 一段都没有（只凭客户账户是客户的、刚登记的）交 null。
test('合作关系一格取行上的段数，两侧都算', () => {
  deepEqual(partyRelationshipCellOf(row(['CUSTOMER'], [2, 1])), { count: 2, candidates: 1 });
  deepEqual(partyRelationshipCellOf(row(['NO_ROLE'], [0, 0], [3, 1])), { count: 3, candidates: 1 });
  deepEqual(partyRelationshipCellOf(row(['CUSTOMER'], [3, 0], [1, 1])), { count: 4, candidates: 1 });
  deepEqual(partyRelationshipCellOf(row(['OUR_COMPANY', 'ACCOUNT_HOLDER'], [1, 0], [3, 1])), { count: 4, candidates: 1 });
  equal(partyRelationshipCellOf(row(['CUSTOMER'])), null);
  equal(partyRelationshipCellOf(row(['NO_ROLE'])), null);
});

// Covers: 一家单位名下在用的段数两侧都算——右栏的「还没有合作关系」与停用对话框说的是「名下还有没有」，不分它站哪一侧。
test('partyLiveRelationshipsOf 两侧都算', () => {
  deepEqual(partyLiveRelationshipsOf(row(['NO_ROLE'], [0, 0], [3, 1])), { live: 3, candidates: 1 });
  deepEqual(partyLiveRelationshipsOf(row(['CUSTOMER'], [2, 1], [1, 0])), { live: 3, candidates: 1 });
  deepEqual(partyLiveRelationshipsOf(row(['NO_ROLE'])), { live: 0, candidates: 0 });
});

// Covers: 图标块——我方公司优先；否则取持有的角色里词表序最前的；没有角色或码不在词表里的是 NONE。
test('partyTileKindOf 取图标块的种类', () => {
  equal(partyTileKindOf(true, ['CUSTOMER']), 'OURS');
  equal(partyTileKindOf(false, ['SUPPLIER', 'CUSTOMER']), 'CUSTOMER');
  equal(partyTileKindOf(false, ['CARRIER_AGENT']), 'CARRIER_AGENT');
  equal(partyTileKindOf(false, []), 'NONE');
  equal(partyTileKindOf(false, ['FUTURE_ROLE']), 'NONE');
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

// Covers: 各列怎么排——放得下各按定宽、余量归行尾；放不下时首列拿剩下的，首列还够定宽别的列不动，不够了别的列按比例收、
// 只收到补上缺口；都收到窄宽后首列再让；表的最小宽度是各窄宽加首列下限，再窄横向滚。
test('partyColumnLayoutOf 放不下时别的列按需收，首列留下限', () => {
  const columns = [{ width: 380 }, { width: 240, narrowWidth: 180 }, { width: 120 }];
  const layout = (listWidth: number | null) => partyColumnLayoutOf(listWidth, columns, 48, 200);
  deepEqual(layout(null), { fits: true, widths: [380, 240, 120], minWidth: null });
  deepEqual(layout(788), { fits: true, widths: [380, 240, 120], minWidth: null });
  // 只差行尾的箭头：别的列不动，首列拿剩下的（还比定宽宽）
  deepEqual(layout(787), { fits: false, widths: [null, 240, 120], minWidth: 500 });
  deepEqual(layout(740), { fits: false, widths: [null, 240, 120], minWidth: 500 });
  // 缺 30：能收的那一列收 30，首列正好还是 380
  deepEqual(layout(710).widths, [null, 210, 120]);
  // 缺口比能收的多：收到窄宽为止，余下的首列让（600 − 180 − 120 = 300）
  deepEqual(layout(600).widths, [null, 180, 120]);
  deepEqual(layout(420).widths, [null, 180, 120]);
  // 没有一列给窄宽：谁都不收，只有首列让
  deepEqual(partyColumnLayoutOf(500, [{ width: 380 }, { width: 240 }], 48, 200), { fits: false, widths: [null, 240], minWidth: 440 });
  // 几列按各自能收的比例分摊：缺 60，能收 100 与 20
  deepEqual(partyColumnLayoutOf(640, [{ width: 280 }, { width: 280, narrowWidth: 180 }, { width: 140, narrowWidth: 120 }], 48, 200).widths, [
    null,
    230,
    130,
  ]);
});

// Covers: 一行的图标块照读口答的类别——我方公司优先，其次角色里词表序最前的，一类都不属于是 NONE。
test('partyTileKindOfRow 照行上的类别', () => {
  equal(partyTileKindOfRow(row(['OUR_COMPANY', 'CUSTOMER'])), 'OURS');
  equal(partyTileKindOfRow(row(['SUPPLIER', 'CUSTOMER'])), 'CUSTOMER');
  equal(partyTileKindOfRow(row(['NO_ROLE'])), 'NONE');
});

// 整册：客户 9、供应商 2、承运商代理 3、我方公司 5、无角色 3；转售一家都没有。
const wholeCategories = counts({ CUSTOMER: 9, SUPPLIER: 2, CARRIER_AGENT: 3, RESELLER: 0, ACCOUNT_HOLDER: 0, AUTHORIZED_DELIVERING_PARTY: 0, OUR_COMPANY: 5, NO_ROLE: 3 });

// Covers: 类型芯片——摆哪几枚看整册的分面（一家都没有的不摆，选中的总在），次序是全部、各角色按词表序、我方公司、未担任角色；
// 数看当前条件的分面，那一问没答出不摆数；当前条件下为零又没选中的点不动；「全部」上的数照调用方给的。
test('partyTypeChipsOf 摆哪几枚看整册的分面，数看当前条件的分面', () => {
  const conditioned = counts({ CUSTOMER: 4, SUPPLIER: 0, CARRIER_AGENT: 1, RESELLER: 0, OUR_COMPANY: 2, NO_ROLE: 0 });
  const chips = partyTypeChipsOf({ whole: wholeCategories, wholeTotal: 24, conditioned, allCount: 7, selected: 'CUSTOMER' });
  deepEqual(
    chips.map((chip) => [chip.type, chip.label, chip.count, chip.registerCount, chip.active, chip.disabled, chip.group]),
    [
      ['ALL', '全部', 7, 24, false, false, 'all'],
      ['CUSTOMER', '客户', 4, 9, true, false, 'role'],
      ['SUPPLIER', '供应商', 0, 2, false, true, 'role'],
      ['CARRIER_AGENT', '承运商代理', 1, 3, false, false, 'role'],
      ['OURS', '我方公司', 2, 5, false, false, 'other'],
      ['NONE', '未担任角色', 0, 3, false, true, 'other'],
    ],
  );
  // 选中的那一枚整册里是零也摆，而且点得动（要能点回去）。
  const selectedEmpty = partyTypeChipsOf({ whole: wholeCategories, wholeTotal: 24, conditioned, allCount: null, selected: 'RESELLER' });
  deepEqual(
    selectedEmpty.filter((chip) => chip.type === 'RESELLER').map((chip) => [chip.count, chip.active, chip.disabled]),
    [[0, true, false]],
  );
  equal(selectedEmpty[0].count, null);
  // 当前条件那一问还没答出：芯片照摆，数都不给。
  const pending = partyTypeChipsOf({ whole: wholeCategories, wholeTotal: 24, conditioned: null, allCount: null, selected: 'ALL' });
  deepEqual(pending.map((chip) => chip.count), [null, null, null, null, null, null]);
  equal(pending.some((chip) => chip.disabled), false);
  // 不带条件那一问没给分面：各类都摆，不替它收。
  const noWhole = partyTypeChipsOf({ whole: null, wholeTotal: null, conditioned: null, allCount: null, selected: 'ALL' });
  equal(noWhole.length, 9);
  equal(noWhole[0].registerCount, 0);
});

// Covers: 类型「全部」上的数从哪凑——类型没筛取当前这一问的总数；筛着类型、没有检索词时取整册的总数或整册里那个状态的家数；
// 筛着类型又有检索词凑不出，交 null。
test('partyTypeAllCount 凑「全部」上的数', () => {
  const wholeStatus = counts({ REGISTERED: 1, EFFECTIVE: 22, DEACTIVATED: 1 });
  const of = (over: Partial<Parameters<typeof partyTypeAllCount>[0]>) =>
    partyTypeAllCount({ type: 'ALL', keyword: '', status: 'ALL', conditionedTotal: 7, wholeTotal: 24, wholeStatus, ...over });
  equal(of({}), 7);
  equal(of({ keyword: 'SYN', status: 'EFFECTIVE' }), 7);
  equal(of({ conditionedTotal: null }), null);
  equal(of({ type: 'CUSTOMER' }), 24);
  equal(of({ type: 'CUSTOMER', status: 'EFFECTIVE' }), 22);
  equal(of({ type: 'CUSTOMER', status: 'EFFECTIVE', wholeStatus: null }), null);
  equal(of({ type: 'CUSTOMER', keyword: 'SYN' }), null);
  equal(of({ type: 'CUSTOMER', keyword: '   ' }), 24);
});

// Covers: 名录状态下拉——各项按词表序，数取当前条件的状态分面，「全部」是各状态之和；那一问没答出只写词。
test('partyStatusOptionsOf 的数取状态分面', () => {
  deepEqual(
    partyStatusOptionsOf(counts({ REGISTERED: 1, EFFECTIVE: 8, DEACTIVATED: 0 })).map((option) => [option.value, option.label, option.count]),
    [
      ['ALL', '全部', 9],
      ['REGISTERED', '待生效', 1],
      ['EFFECTIVE', '已生效', 8],
      ['DEACTIVATED', '已停用', 0],
    ],
  );
  deepEqual(partyStatusOptionsOf(null).map((option) => option.count), [null, null, null, null]);
});

// Covers: 芯片摆不摆得下只看工具条宽度与整册的数——还没量到宽度先不摆；够宽摆芯片，窄了收成下拉。
test('partyTypeChipsFit 按工具条宽度判', () => {
  const chips = partyTypeChipsOf({ whole: wholeCategories, wholeTotal: 24, conditioned: null, allCount: null, selected: 'ALL' });
  equal(partyTypeChipsFit(null, chips), false);
  equal(partyTypeChipsFit(4000, chips), true);
  equal(partyTypeChipsFit(700, chips), false);
});
