import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { goodsDraftOf } from './goods-draft';

// 本文件钉下单页把逐包裹填的货物格并成委托级货物明细行的写法(ADR-0373):货物的值只写在委托的货物明细行上一处,
// 页面暂不报装箱分配(整票清单),多件委托逐箱怎么分留给 FBA 货件下单页。

const blank = { goodsDescription: '', quantity: '', declaredValue: '', currency: '', originCountry: '' };

test('每个填了货物的包裹并成一行,没填的包裹不出行', () => {
  const goods = goodsDraftOf([
    { goodsDescription: ' 男式棉质T恤 ', quantity: '5', declaredValue: '16.00', currency: 'USD', originCountry: 'CN' },
    blank,
    { ...blank, goodsDescription: 'Wrist watch' },
  ]);
  deepEqual(goods, {
    lines: [
      {
        descriptions: [{ lang: 'und', text: '男式棉质T恤' }],
        quantity: '5',
        lineValue: '16.00',
        currency: 'USD',
        originCountry: 'CN',
      },
      { descriptions: [{ lang: 'und', text: 'Wrist watch' }] },
    ],
  });
});

test('一个包裹都没填货物时整段缺席,不送空的货物明细', () => {
  equal(goodsDraftOf([blank, blank]), undefined);
});

test('只填了数量没填品名的包裹照样成行:页面不发明必填,必填由接单规则包说', () => {
  deepEqual(goodsDraftOf([{ ...blank, quantity: '2' }]), { lines: [{ quantity: '2' }] });
});
