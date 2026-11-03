import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { contactCardOf, socialAccountsText } from './legal-entity-contact-cards';

// 本文件钉联系人卡的判读（票 company-subject-ux/11 第 1 条）：没有的项整行缺席、用途胶囊带「主要」、社交账号串的形状。

test('齐全的联系人：部门与职务并成一行，用途按资料顺序、主要的标出来', () => {
  const card = contactCardOf({
    name: '陆星河（模拟）',
    department: '商务发展部',
    position: '商务经理',
    phone: '+86 21 0000 0101（模拟）',
    email: 'business.operator-01@idp-parcel.example.invalid',
    purposes: ['BUSINESS', 'GENERAL'],
    primaryFor: ['BUSINESS'],
    socialAccounts: [{ platform: '微信', account: 'SYN-1-SYN-LE-01-01' }, { platform: '企业微信', account: 'SYN-2-SYN-LE-01-01' }],
  });
  equal(card.name, '陆星河（模拟）');
  equal(card.affiliation, '商务发展部 · 商务经理');
  deepEqual(card.purposes, [
    { code: 'BUSINESS', label: '商务', primary: true },
    { code: 'GENERAL', label: '日常联系', primary: false },
  ]);
  equal(card.phone, '+86 21 0000 0101（模拟）');
  equal(card.email, 'business.operator-01@idp-parcel.example.invalid');
  equal(socialAccountsText(card.socialAccounts), '微信 SYN-1-SYN-LE-01-01 · 企业微信 SYN-2-SYN-LE-01-01');
});

test('没有的项缺席而不是空串：只有职务时那一行只显职务，空白当没有', () => {
  const bare = contactCardOf({ name: 'SYN-A' });
  equal(bare.affiliation, undefined);
  equal(bare.phone, undefined);
  equal(bare.email, undefined);
  deepEqual(bare.purposes, []);
  deepEqual(bare.socialAccounts, []);
  const onlyPosition = contactCardOf({ name: 'SYN-B', department: '  ', position: '运营主管', phone: ' ', email: '' });
  equal(onlyPosition.affiliation, '运营主管');
  equal(onlyPosition.phone, undefined);
  equal(onlyPosition.email, undefined);
});

test('主要联系人不在用途列表里时补成一颗胶囊，不丢；未知用途码原样显', () => {
  const card = contactCardOf({ name: 'SYN-C', purposes: ['FINANCE'], primaryFor: ['LEGAL'] });
  deepEqual(card.purposes, [
    { code: 'FINANCE', label: '财务', primary: false },
    { code: 'LEGAL', label: '法务', primary: true },
  ]);
  const unknown = contactCardOf({ name: 'SYN-D', purposes: ['CUSTOMS' as never] });
  deepEqual(unknown.purposes, [{ code: 'CUSTOMS', label: 'CUSTOMS', primary: false }]);
});

test('社交账号：账号空的条目剔掉，平台空的只显账号，首尾空白修掉', () => {
  const card = contactCardOf({
    name: 'SYN-E',
    socialAccounts: [{ platform: ' 微信 ', account: ' SYN-WX ' }, { platform: 'Telegram', account: '  ' }, { platform: '', account: 'SYN-BARE' }],
  });
  deepEqual(card.socialAccounts, [{ platform: '微信', account: 'SYN-WX' }, { platform: '', account: 'SYN-BARE' }]);
  equal(socialAccountsText(card.socialAccounts), '微信 SYN-WX · SYN-BARE');
  equal(socialAccountsText([]), '');
});
