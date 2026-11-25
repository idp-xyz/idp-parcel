import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { cartonMarkSheet, countSheet, inboundSheet, packListSheet, pickListSheet } from './print-forms';
import { escapePrint, printHtml } from './print-sheet';

test('打印格子里的尖括号与和号被转义', () => {
  equal(escapePrint(`a<b>&"c`), 'a&lt;b&gt;&amp;&quot;c');
  const html = printHtml(pickListSheet('W<1', [{ locationCode: 'L', goodsCode: 'G&1', unit: '件', quantity: 2, picked: 0, requestId: 'R' }]));
  ok(html.includes('W&lt;1'));
  ok(html.includes('G&amp;1'));
  ok(!html.includes('G&1'));
});

test('拣货单按库位再按商品排', () => {
  const sheet = pickListSheet('W-1', [
    { locationCode: 'B', goodsCode: 'G2', unit: '件', quantity: 1, picked: 0, requestId: 'R' },
    { locationCode: 'A', goodsCode: 'G2', unit: '件', quantity: 1, picked: 1, requestId: 'R' },
    { locationCode: 'A', goodsCode: 'G1', unit: '件', quantity: 3, picked: 0, requestId: 'R' },
  ]);
  equal(sheet.sections[0].rows.map((row) => row[0] + row[1]).join(','), 'AG1,AG2,BG2');
});

test('装箱单与箱唛一只包裹一段；没记重量写没记', () => {
  const parcels = [{ sequence: 1, goods: [{ goodsCode: 'G', unit: '件', quantity: 2 }] }];
  equal(packListSheet('OB-1', parcels).sections[0].lines[1].value, '没记');
  equal(cartonMarkSheet('OB-1', parcels).sections.length, 1);
  equal(cartonMarkSheet('OB-1', parcels).title.startsWith('箱唛'), true);
});

test('入库单带已收；盘点单没录入的库位不编数量', () => {
  const inbound = inboundSheet('IB-1', 'WH', [{ goodsCode: 'G', unit: '件', advised: 10, received: 4 }]);
  equal(inbound.sections[0].rows[0][3], '4');
  const count = countSheet('CNT', 'WH', ['L1', 'L2'], [{ locationCode: 'L1', goodsCode: 'G', quantity: 3, stockState: 'GOOD' }]);
  equal(count.sections[0].rows[1][1], '还没录入');
  equal(count.sections[1].rows.length, 1);
  equal(count.sections[1].rows[0][2], '良品');
});
