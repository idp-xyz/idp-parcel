import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { objectIdOfTab, tabIdFromHash } from '../../shell/workspace-state';
import { documentHash, documentKeyParts } from './document-address';
import {
  adjustmentKind,
  countKind,
  countsAdjustmentsModule,
  countsAdjustmentsTarget,
  discrepancyKind,
} from './counts-adjustments-address';

test('三种种类词各认成自己那一种，第二段是标识', () => {
  deepEqual(countsAdjustmentsTarget(['盘点', 'CNT-1']), { kind: 'count', id: 'CNT-1' });
  deepEqual(countsAdjustmentsTarget(['调整', 'ADJ-1']), { kind: 'adjustment', id: 'ADJ-1' });
  deepEqual(countsAdjustmentsTarget(['作业差异', 'SDP-1']), { kind: 'discrepancy', id: 'SDP-1' });
});

test('标识缺段时 id 是空串，种类词还认得：由单据页说缺什么', () => {
  deepEqual(countsAdjustmentsTarget(['盘点']), { kind: 'count', id: '' });
  deepEqual(countsAdjustmentsTarget(['调整', '']), { kind: 'adjustment', id: '' });
});

test('不认识的种类词不猜，原样交回', () => {
  deepEqual(countsAdjustmentsTarget(['count', 'CNT-1']), { kind: 'unknown', word: 'count' });
  deepEqual(countsAdjustmentsTarget(['CNT-1']), { kind: 'unknown', word: 'CNT-1' });
  deepEqual(countsAdjustmentsTarget([]), { kind: 'unknown', word: '' });
});

test('三种单据各是壳层里一张标签，标签上的键拆回来就是种类词加标识；标识里自带斜杠也不串段', () => {
  for (const [word, id] of [
    [countKind, 'CNT-1'],
    [adjustmentKind, 'ADJ/2'],
    [discrepancyKind, 'SDP 3'],
  ]) {
    const tabId = tabIdFromHash(documentHash(countsAdjustmentsModule, [word, id]), () => true);
    equal(tabId?.startsWith(`${countsAdjustmentsModule}/`), true);
    equal(tabId?.split('/').length, 2);
    const parts = documentKeyParts(objectIdOfTab(tabId!)!);
    deepEqual(parts, [word, id]);
    equal(countsAdjustmentsTarget(parts).kind !== 'unknown', true);
  }
});
