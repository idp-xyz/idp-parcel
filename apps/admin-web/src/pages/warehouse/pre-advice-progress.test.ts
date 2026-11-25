import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { preAdviceProgress } from './pre-advice-progress';

test('同一商品分几行预报时按商品合计，与累计实收对', () => {
  const progress = preAdviceProgress({
    inboundMode: 'BY_GOODS',
    goodsLines: [
      { goodsCode: 'G1', unit: '件', quantity: 6 },
      { goodsCode: 'G1', unit: '件', quantity: 4 },
      { goodsCode: 'G2', unit: '箱', quantity: 3 },
    ],
    cumulativeReceived: [{ goodsCode: 'G1', quantity: 7 }],
  });
  deepEqual(progress.rows, [
    { key: 'G1', detail: '件', advised: 10, received: 7 },
    { key: 'G2', detail: '箱', advised: 3, received: 0 },
  ]);
  equal(progress.advised, 13);
  equal(progress.received, 7);
  equal(progress.outstanding, 6);
});

test('多收不抵少收，预报里没有的货照样列出', () => {
  const progress = preAdviceProgress({
    inboundMode: 'BY_GOODS',
    goodsLines: [
      { goodsCode: 'G1', unit: '件', quantity: 5 },
      { goodsCode: 'G2', unit: '件', quantity: 5 },
    ],
    cumulativeReceived: [
      { goodsCode: 'G1', quantity: 8 },
      { goodsCode: 'G9', quantity: 2 },
    ],
  });
  equal(progress.outstanding, 5);
  deepEqual(progress.rows.find((row) => row.key === 'G9'), { key: 'G9', detail: '', advised: 0, received: 2 });
});

test('按整箱入库的按箱号对，一箱算一件', () => {
  const progress = preAdviceProgress({
    inboundMode: 'BY_CARTON',
    cartons: [
      { cartonNumber: 'C1', lines: [{ goodsCode: 'G1', quantity: 12 }] },
      { cartonNumber: 'C2', lines: [{ goodsCode: 'G1', quantity: 12 }, { goodsCode: 'G2', quantity: 1 }] },
    ],
    cumulativeReceived: [{ cartonNumber: 'C1', quantity: 1 }],
  });
  equal(progress.byCarton, true);
  equal(progress.advised, 2);
  equal(progress.received, 1);
  equal(progress.rows[1].detail, 'G1×12、G2×1');
});
