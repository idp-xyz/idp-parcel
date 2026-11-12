import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord } from './api';
import { similarPartiesOf, similarPartiesShown, similarPartyKeywords } from './business-party-similar';

// 本文件钉「名录里相近的单位」什么时候问、问什么，以及几份答复怎么并成一栏。

function party(partyId: string): BusinessPartyRecord {
  return { partyId, partyName: `单位 ${partyId}` } as BusinessPartyRecord;
}

test('名称或编码输到两个字才问，名称在前', () => {
  deepEqual(similarPartyKeywords('', ''), []);
  deepEqual(similarPartyKeywords('深', 'S'), []);
  deepEqual(similarPartyKeywords('深圳', 'S'), ['深圳']);
  deepEqual(similarPartyKeywords('深', 'SH'), ['SH']);
  deepEqual(similarPartyKeywords('深圳', 'SH'), ['深圳', 'SH']);
});

test('字数按字符数、去掉首尾空白之后算', () => {
  // 两个空格夹一个字，仍是一个字。
  deepEqual(similarPartyKeywords(' 深 ', ''), []);
  deepEqual(similarPartyKeywords('  深圳  ', ''), ['深圳']);
  // 一个增补平面的字是两个 UTF-16 码元，仍只算一个字。
  equal('𠮷'.length, 2);
  deepEqual(similarPartyKeywords('𠮷', ''), []);
  deepEqual(similarPartyKeywords('𠮷野', ''), ['𠮷野']);
});

test('两格填的一样只问一次，不分大小写', () => {
  deepEqual(similarPartyKeywords('acme', 'ACME'), ['acme']);
  deepEqual(similarPartyKeywords('Acme', 'Acme-01'), ['Acme', 'Acme-01']);
});

test('长过读口检索词上限的那一格不问，另一格照问', () => {
  deepEqual(similarPartyKeywords('长'.repeat(101), 'SH-01'), ['SH-01']);
  deepEqual(similarPartyKeywords('长'.repeat(100), ''), ['长'.repeat(100)]);
});

test('几份答复并成一栏：名称查到的在前，同一家只列一次', () => {
  const merged = similarPartiesOf([
    { parties: [party('A'), party('B')], hasNext: false },
    { parties: [party('B'), party('C')], hasNext: false },
  ]);
  deepEqual(
    merged.shown.map((row) => row.partyId),
    ['A', 'B', 'C'],
  );
  equal(merged.more, false);
  equal(merged.unread, false);
});

test('查到的多过列得下的，只列前几家并说还有', () => {
  const many = Array.from({ length: similarPartiesShown + 1 }, (_, index) => party(`P${index}`));
  const merged = similarPartiesOf([{ parties: many, hasNext: false }]);
  equal(merged.shown.length, similarPartiesShown);
  equal(merged.more, true);
  // 刚好列得下、但读口说后面还有一页，同样说还有。
  equal(similarPartiesOf([{ parties: many.slice(0, 2), hasNext: true }]).more, true);
  equal(similarPartiesOf([{ parties: many.slice(0, similarPartiesShown), hasNext: false }]).more, false);
});

test('有检索词没读到答复：读到的照列，并标出没读全', () => {
  const partial = similarPartiesOf([null, { parties: [party('A')], hasNext: false }]);
  deepEqual(
    partial.shown.map((row) => row.partyId),
    ['A'],
  );
  equal(partial.unread, true);
  const none = similarPartiesOf([null]);
  deepEqual(none.shown, []);
  equal(none.unread, true);
});
