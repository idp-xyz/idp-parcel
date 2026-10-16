import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { groupLegalEntitiesViewOf, registerViewHash } from './group-legal-entities-address';

// 本文件钉集团与法人页「地址 → 视图」的判读（票 group-legal-entities-uiux/04）：登记视图住在查询串上，不占路径第二段——
// 第二段是法人标识，法人标识是自由文本，拿保留词占它会让同名法人的对象页打不开。

// Covers: 列表、登记视图、对象页三种地址各判各的；登记视图与检索词等别的查询键并存。
test('地址判出列表、登记视图或对象页', () => {
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities'), { kind: 'list' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities?register=1'), { kind: 'register' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities?q=SYN&register=1'), { kind: 'register' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities?register=0'), { kind: 'list' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities/SYN-LE-01'), { kind: 'object', legalEntityId: 'SYN-LE-01' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities/LE%2F01'), { kind: 'object', legalEntityId: 'LE/01' });
});

// Covers: 叫 register 的法人照样是对象页；对象地址上带了登记键也还是对象页——对象地址先认。
test('与登记键同名的法人标识不被认成登记视图', () => {
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities/register'), { kind: 'object', legalEntityId: 'register' });
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities/register?register=1'), {
    kind: 'object',
    legalEntityId: 'register',
  });
});

// Covers: 畸形百分号序列不抛，退回列表；别的模块的地址不归本页管，也答列表（本页不会被挂在那里）。
test('畸形地址退回列表', () => {
  deepEqual(groupLegalEntitiesViewOf('#/group-legal-entities/%E0'), { kind: 'list' });
  deepEqual(groupLegalEntitiesViewOf(''), { kind: 'list' });
});

// Covers: 进出登记视图只动登记键，路径与检索词原样。
test('进出登记视图只改登记键', () => {
  equal(registerViewHash('#/group-legal-entities', true), '#/group-legal-entities?register=1');
  equal(registerViewHash('#/group-legal-entities?q=SYN', true), '#/group-legal-entities?q=SYN&register=1');
  equal(registerViewHash('#/group-legal-entities?q=SYN&register=1', false), '#/group-legal-entities?q=SYN');
  equal(registerViewHash('#/group-legal-entities?register=1', false), '#/group-legal-entities');
});
