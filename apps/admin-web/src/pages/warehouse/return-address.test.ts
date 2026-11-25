import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { objectIdOfTab, tabIdFromHash } from '../../shell/workspace-state';
import { documentHash, documentKeyParts } from './document-address';
import { returnAdviceKind, returnReceiptKind, returnsModule, returnsTarget } from './return-address';

test('两种种类词各认成自己那一种，第二段是标识', () => {
  deepEqual(returnsTarget(['退货预报', 'RT-20261120-00001']), { kind: 'advice', id: 'RT-20261120-00001' });
  deepEqual(returnsTarget(['退货到仓', 'SYN-FACT-SEED-RET-01']), { kind: 'receipt', id: 'SYN-FACT-SEED-RET-01' });
});

test('标识缺段时 id 是空串，种类词还认得：由单据页说缺什么', () => {
  deepEqual(returnsTarget(['退货预报']), { kind: 'advice', id: '' });
  deepEqual(returnsTarget(['退货到仓', '']), { kind: 'receipt', id: '' });
});

test('不认识的种类词不猜，原样交回；别的页的种类词在这里也不认', () => {
  deepEqual(returnsTarget(['advice', 'RT-1']), { kind: 'unknown', word: 'advice' });
  deepEqual(returnsTarget(['RT-1']), { kind: 'unknown', word: 'RT-1' });
  deepEqual(returnsTarget(['盘点', 'CNT-1']), { kind: 'unknown', word: '盘点' });
  deepEqual(returnsTarget([]), { kind: 'unknown', word: '' });
});

test('两种单据各是壳层里一张标签，标签上的键拆回来就是种类词加标识；标识里自带斜杠也不串段', () => {
  for (const [word, id] of [
    [returnAdviceKind, 'RT-20261120-00001'],
    [returnReceiptKind, 'FACT/2 一'],
  ]) {
    const tabId = tabIdFromHash(documentHash(returnsModule, [word, id]), () => true);
    equal(tabId?.startsWith(`${returnsModule}/`), true);
    equal(tabId?.split('/').length, 2);
    const parts = documentKeyParts(objectIdOfTab(tabId!)!);
    deepEqual(parts, [word, id]);
    equal(returnsTarget(parts).kind !== 'unknown', true);
  }
});

test('地址形状：模块 id 一段，单据键整体编码成第二段', () => {
  equal(
    documentHash(returnsModule, [returnAdviceKind, 'RT-20261120-00001']),
    `#/warehouse-returns/${encodeURIComponent('退货预报/RT-20261120-00001')}`,
  );
  equal(
    documentHash(returnsModule, [returnReceiptKind, 'SYN-FACT-SEED-RET-01']),
    `#/warehouse-returns/${encodeURIComponent('退货到仓/SYN-FACT-SEED-RET-01')}`,
  );
});
