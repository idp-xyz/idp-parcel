import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { supplementGapLinesOf } from './supplement-gaps';

// 本文件钉受理等客户补充时缺口的屏上写法(ADR-0376 决定一):认得的原因译成缺什么,认不得的照原码;委托级在前,逐件按包裹号。

test('缺口摊成行:委托级在前、逐件按包裹号,同一件并成一行,认不得的原因照原码', () => {
  deepEqual(
    supplementGapLinesOf([
      { group: 'REQUIRED_DOCUMENT', parcelId: 'DPCL-2', reason: 'EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL' },
      { group: 'REQUIRED_DOCUMENT', reason: 'PACKING_DECLARED' },
      { group: 'REQUIRED_DOCUMENT', parcelId: 'DPCL-1', reason: 'EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL' },
      { group: 'NETWORK_REACHABILITY', parcelId: 'DPCL-1', reason: 'REACHABILITY_INSUFFICIENT_EVIDENCE' },
    ]),
    [
      { parcelId: '', missing: '装箱分配' },
      { parcelId: 'DPCL-1', missing: '平台箱标号、REACHABILITY_INSUFFICIENT_EVIDENCE' },
      { parcelId: 'DPCL-2', missing: '平台箱标号' },
    ],
  );
  deepEqual(supplementGapLinesOf(undefined), []);
});
