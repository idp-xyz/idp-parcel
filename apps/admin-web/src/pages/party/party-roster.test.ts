import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord } from './api';
import {
  partyRosterPageText,
  partyRosterTypeOf,
  partyRosterTypeUnreadText,
  rememberedPartyExpired,
  rememberedPartyOnReturn,
  selectedPartyRowOf,
} from './party-roster';

// 本文件钉的是往来单位页主从分栏（票 business-party-redesign/03）的纯逻辑：名录栏一行的类型那一句与类型列同一判据、栏底的页码、
// 选中的那家不在当前这一页上时右栏显哪一行、写入落册之后旧的一行什么时候不再接着显。

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

// Covers: 我方公司排在角色前面，用「 · 」连；一个都没有、册读全了是「暂无合作关系」，没读全是「类型没读到」并标出来；
// 推得出类型的行不因册没读全而改口。
test('partyRosterTypeOf 照类型列的判据写名录栏一行的类型', () => {
  deepEqual(partyRosterTypeOf(false, ['CUSTOMER'], true), { text: '客户', unread: false });
  deepEqual(partyRosterTypeOf(true, ['CUSTOMER', 'SUPPLIER'], true), { text: '我方公司 · 客户 · 供应商', unread: false });
  deepEqual(partyRosterTypeOf(true, [], true), { text: '我方公司', unread: false });
  deepEqual(partyRosterTypeOf(false, [], true), { text: '暂无合作关系', unread: false });
  deepEqual(partyRosterTypeOf(false, [], false), { text: partyRosterTypeUnreadText, unread: true });
  deepEqual(partyRosterTypeOf(false, ['CUSTOMER'], false), { text: '客户', unread: false });
});

// Covers: 有总数时「页 / 总页数」，读口不给总数时只有页码。
test('partyRosterPageText 只写数', () => {
  equal(partyRosterPageText(2, 200, 1286), '2 / 7');
  equal(partyRosterPageText(1, 200, 24), '1 / 1');
  equal(partyRosterPageText(3, 200, null), '3');
});

// Covers: 当前这一页上有就用它（最新）；不在这一页上就看不带条件的那一页；两处都没有才用记着的那一行；记着的是别家的不算；
// 没选中交 null。
test('selectedPartyRowOf 先当前页、再不带条件的那一页、最后才是记着的那一行', () => {
  const onPage = party('SYN-P-01', { revision: 3 });
  const known = party('SYN-P-01', { revision: 2 });
  const remembered = { row: party('SYN-P-01'), stale: false };
  equal(selectedPartyRowOf('SYN-P-01', [onPage], [known], remembered), onPage);
  equal(selectedPartyRowOf('SYN-P-01', [party('SYN-P-02')], [known], remembered), known);
  equal(selectedPartyRowOf('SYN-P-01', [], [party('SYN-P-02')], remembered), remembered.row);
  equal(selectedPartyRowOf('SYN-P-01', [], null, remembered), remembered.row);
  equal(selectedPartyRowOf('SYN-P-01', [], null, { row: party('SYN-P-02'), stale: false }), null);
  equal(selectedPartyRowOf('SYN-P-01', [], null, null), null);
  equal(selectedPartyRowOf(null, [onPage], [known], remembered), null);
});

// Covers: 记着的那一行只在写入落册之后（stale）才会过期；当前这一页还在途先留着；答回来了、当前页与不带条件的那一页都没有它
// 才算过期；有一处读得到就不算。
test('rememberedPartyExpired 写入之后两处都读不到才算过期', () => {
  const stale = { row: party('SYN-P-01'), stale: true };
  const fresh = { row: party('SYN-P-01'), stale: false };
  equal(rememberedPartyExpired('SYN-P-01', true, [], [party('SYN-P-02')], stale), true);
  equal(rememberedPartyExpired('SYN-P-01', true, [], null, stale), true);
  equal(rememberedPartyExpired('SYN-P-01', false, [], null, stale), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [party('SYN-P-01')], null, stale), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [], [party('SYN-P-01')], stale), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [], null, fresh), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [], null, null), false);
  equal(rememberedPartyExpired(null, true, [], null, stale), false);
});

// Covers: 页面重挂之后取回来的那一行一律按「可能已经变了」对待（票 business-party-redesign/11）——行本身不动，只标上；
// 没记着就是没有。标上之后的去留照 rememberedPartyExpired：重读回来两处都没有它才算过期，还在读、或读到了都不算。
test('rememberedPartyOnReturn 把取回来的那一行标成可能已经变了', () => {
  const row = party('SYN-P-01', { revision: 2 });
  equal(rememberedPartyOnReturn(null), null);
  deepEqual(rememberedPartyOnReturn({ row, stale: false }), { row, stale: true });
  deepEqual(rememberedPartyOnReturn({ row, stale: true }), { row, stale: true });
  const returned = rememberedPartyOnReturn({ row, stale: false });
  equal(rememberedPartyExpired('SYN-P-01', false, [], null, returned), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [party('SYN-P-01', { revision: 3 })], null, returned), false);
  equal(rememberedPartyExpired('SYN-P-01', true, [], [party('SYN-P-02')], returned), true);
});
