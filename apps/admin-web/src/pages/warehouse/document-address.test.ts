import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { objectIdOfTab, tabIdFromHash } from '../../shell/workspace-state';
import { documentHash, documentKey, documentKeyParts } from './document-address';

test('寻常标识连成的键原样可读，中文的种类词不变成百分号', () => {
  equal(documentKey(['SYN-OWNER-01', 'SYN-IPA-01']), 'SYN-OWNER-01/SYN-IPA-01');
  equal(documentKey(['盘点', 'SYN-COUNT-01']), '盘点/SYN-COUNT-01');
});

test('段里自带斜杠与百分号时拆回来仍是原来那几段', () => {
  for (const parts of [
    ['MANUAL/SRC-1', 'A%2FB'],
    ['a%', '%25', '%2F'],
    ['货主/一', '单 号'],
    ['only'],
  ]) {
    deepEqual(documentKeyParts(documentKey(parts)), parts);
  }
});

test('单据地址被壳层认成一张标签，标签上的对象标识就是单据键', () => {
  const parts = ['SYN-AUTHORITY/customs-cn', 'IPA 01'];
  const tabId = tabIdFromHash(documentHash('warehouse-pre-advices', parts), () => true);
  equal(tabId?.startsWith('warehouse-pre-advices/'), true);
  // 键里的斜杠编码过，不会被壳层当成第三段切掉。
  equal(tabId?.split('/').length, 2);
  deepEqual(documentKeyParts(objectIdOfTab(tabId!)!), parts);
});
