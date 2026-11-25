import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { simulatedTracking, simulatedUspsLabelHtml } from './simulated-usps-label';

test('模拟号以 SIM 起头，同一单同一包裹每次一样，不是纯数字邮资号', () => {
  equal(simulatedTracking('ob-1/a', 1), simulatedTracking('ob-1/a', 1));
  equal(simulatedTracking('ob-1/a', 1).startsWith('SIM01'), true);
  ok(!/^\d{20,}$/.test(simulatedTracking('ob-1/a', 1)));
  ok(simulatedTracking('ob-1/a', 1) !== simulatedTracking('ob-1/a', 2));
});

test('模拟面单一只包裹一页，收件人里的尖括号被转义，并写明不能当邮资', () => {
  const html = simulatedUspsLabelHtml({
    documentNumber: 'OB<1',
    warehouseCode: 'WH',
    recipient: 'Ann <Lee>',
    address: '1 Main',
    parcels: [
      { sequence: 1, weight: '1.2', weightUnit: 'kg' },
      { sequence: 2 },
    ],
  });
  equal(html.split('class="sheet"').length - 1, 2);
  ok(html.includes('不能当邮资'));
  ok(html.includes('Ann &lt;Lee&gt;'));
  ok(html.includes('OB&lt;1'));
  ok(html.includes('重量没记'));
  ok(html.includes('USPS'));
  ok(!html.includes('Ann <Lee>'));
});
