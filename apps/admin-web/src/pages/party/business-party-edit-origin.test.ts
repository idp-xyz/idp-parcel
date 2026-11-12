import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  closesTabOnExit,
  editSectionDomId,
  markEditFromRoster,
  peekEditOrigin,
  profileEditSections,
  takeEditOrigin,
} from './business-party-edit-origin';

// 本文件钉编辑态的两样（票 business-party-redesign/11）：表单分段的次序，以及从名录右栏进编辑态时记的那一笔怎么取；
// 再加退出时对象页那张标签关不关（票 14）。

// Covers: 这次更新在最前，其余与只读时概要的卡片同名同序；各段的 DOM id 不重。
test('profileEditSections 的次序：这次更新在前，其余同概要的卡片', () => {
  deepEqual(
    profileEditSections.map((section) => section.label),
    ['这次更新', '名称与地址', '税务与开票', '收款账户', '联系人'],
  );
  const ids = profileEditSections.map((section) => editSectionDomId(section.id));
  equal(new Set(ids).size, ids.length);
  equal(editSectionDomId('contacts'), 'party-edit-contacts');
});

// Covers: 记下的那一笔对得上这一家才交出、交出即清（第二次取不到）；记的是别家时不交也不清，留给那一家；后一次覆盖前一次；
// 没记过取不到。
test('takeEditOrigin 一次进入只认一次，记的是别家不动它', () => {
  equal(takeEditOrigin('SYN-PARTY-A'), null);

  markEditFromRoster('SYN-PARTY-A', 'contacts', false);
  equal(takeEditOrigin('SYN-PARTY-B'), null);
  // 看一眼不取走：看几次都在，别家看不到。
  deepEqual(peekEditOrigin('SYN-PARTY-A'), { partyId: 'SYN-PARTY-A', section: 'contacts', tabWasOpen: false });
  deepEqual(peekEditOrigin('SYN-PARTY-A'), { partyId: 'SYN-PARTY-A', section: 'contacts', tabWasOpen: false });
  equal(peekEditOrigin('SYN-PARTY-B'), null);
  deepEqual(takeEditOrigin('SYN-PARTY-A'), { partyId: 'SYN-PARTY-A', section: 'contacts', tabWasOpen: false });
  equal(takeEditOrigin('SYN-PARTY-A'), null);
  equal(peekEditOrigin('SYN-PARTY-A'), null);

  markEditFromRoster('SYN-PARTY-A', null, false);
  markEditFromRoster('SYN-PARTY-B', 'invoicing', true);
  equal(takeEditOrigin('SYN-PARTY-A'), null);
  deepEqual(takeEditOrigin('SYN-PARTY-B'), { partyId: 'SYN-PARTY-B', section: 'invoicing', tabWasOpen: true });

  markEditFromRoster('SYN-PARTY-C', null, false);
  deepEqual(takeEditOrigin('SYN-PARTY-C'), { partyId: 'SYN-PARTY-C', section: null, tabWasOpen: false });
});

// Covers: 退出回名录时对象页那张标签关不关——从名录进来、且那张标签是为这一次编辑才开的，关；进来之前人自己开着的，不关；
// 没有来处（在对象页自己点的、刷新或深链进来的）不关，那时退出后本来就留在对象页。
test('closesTabOnExit 只关为这一次编辑开的那一张', () => {
  equal(closesTabOnExit({ partyId: 'SYN-PARTY-A', section: null, tabWasOpen: false }), true);
  equal(closesTabOnExit({ partyId: 'SYN-PARTY-A', section: 'contacts', tabWasOpen: true }), false);
  equal(closesTabOnExit(null), false);
});
