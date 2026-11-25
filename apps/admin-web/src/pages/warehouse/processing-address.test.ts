import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { objectIdOfTab, tabIdFromHash } from '../../shell/workspace-state';
import { documentHash, documentKeyParts } from './document-address';
import { freezeKind, instructionKind, internalJobKind, processingModule, processingTarget } from './processing-address';

test('三种种类词各认成自己那一种，第二段是标识', () => {
  deepEqual(processingTarget(['指令', 'INS-1']), { kind: 'instruction', id: 'INS-1' });
  deepEqual(processingTarget(['冻结', 'FRZ-1']), { kind: 'freeze', id: 'FRZ-1' });
  deepEqual(processingTarget(['库内作业', 'FACT-1']), { kind: 'internalJob', id: 'FACT-1' });
});

test('标识缺段时 id 是空串，种类词还认得：由单据页说缺什么', () => {
  deepEqual(processingTarget(['指令']), { kind: 'instruction', id: '' });
  deepEqual(processingTarget(['冻结', '']), { kind: 'freeze', id: '' });
  deepEqual(processingTarget(['库内作业']), { kind: 'internalJob', id: '' });
});

test('不认识的种类词不猜，原样交回；盘点与调整那一页的种类词在这里也不认', () => {
  deepEqual(processingTarget(['instruction', 'INS-1']), { kind: 'unknown', word: 'instruction' });
  deepEqual(processingTarget(['INS-1']), { kind: 'unknown', word: 'INS-1' });
  deepEqual(processingTarget(['盘点', 'CNT-1']), { kind: 'unknown', word: '盘点' });
  deepEqual(processingTarget([]), { kind: 'unknown', word: '' });
});

test('三种单据各是壳层里一张标签，标签上的键拆回来就是种类词加标识；标识里自带斜杠也不串段', () => {
  for (const [word, id] of [
    [instructionKind, 'INS-1'],
    [freezeKind, 'FRZ/2'],
    [internalJobKind, 'FACT 3'],
  ]) {
    const tabId = tabIdFromHash(documentHash(processingModule, [word, id]), () => true);
    equal(tabId?.startsWith(`${processingModule}/`), true);
    equal(tabId?.split('/').length, 2);
    const parts = documentKeyParts(objectIdOfTab(tabId!)!);
    deepEqual(parts, [word, id]);
    equal(processingTarget(parts).kind !== 'unknown', true);
  }
});

test('作业差异页指过来的库内作业地址：事实号照原样回到库内作业那一种', () => {
  const hash = documentHash(processingModule, [internalJobKind, 'SYN-FACT-01']);
  const parts = documentKeyParts(objectIdOfTab(tabIdFromHash(hash, () => true)!)!);
  deepEqual(processingTarget(parts), { kind: 'internalJob', id: 'SYN-FACT-01' });
});
