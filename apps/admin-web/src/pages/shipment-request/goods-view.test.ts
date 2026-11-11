import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { contentCategoryLabel, goodsLineRowsOf, packingSummaryOf } from './goods-view';

// 本文件钉委托详情里货物明细的屏上写法(ADR-0373,票 declared-goods-lines/04):每格原样显示客户给的串,不换算;来源不是本单申报
// 的格(箱单、商品申报资料)单独点名,本单申报是客户渠道的常态不另标;没报的格显「—」。

const cell = (value: string, mark = 'THIS_DECLARATION') => ({ value, mark });

test('内件性质按 CN23 六格显中文,没报为空', () => {
  equal(contentCategoryLabel('SALE_OF_GOODS'), '销售货物');
  equal(contentCategoryLabel('RETURNED_GOODS'), '退回货物');
  equal(contentCategoryLabel(undefined), '');
});

test('一行货物摊成屏上的几格:品名按语言列出,数量带单位,编码带口径,来源不是本单申报的格点名', () => {
  const rows = goodsLineRowsOf({
    contentCategory: 'SALE_OF_GOODS',
    lines: [
      {
        line: 1,
        descriptions: [
          { lang: 'en', text: cell("Men's cotton T-shirt", 'BOX_LIST') },
          { lang: 'zh', text: cell('男式棉质T恤', 'BOX_LIST') },
        ],
        quantity: cell('48'),
        unit: cell('PCS'),
        unitValue: cell('3.20', 'COMMODITY_DECLARATION'),
        currency: cell('USD', 'COMMODITY_DECLARATION'),
        commodityCodes: [{ side: 'EXPORT', code: cell('6109100000') }],
        salesOrder: { platform: cell('SHOPIFY'), orderNumber: cell('1001'), orderLine: cell('2') },
      },
    ],
  });
  deepEqual(rows, [
    {
      line: 1,
      description: "en: Men's cotton T-shirt / zh: 男式棉质T恤",
      quantity: '48 PCS',
      unitValue: '3.20',
      lineValue: '—',
      currency: 'USD',
      netWeight: '—',
      commodityCodes: '出口 6109100000',
      originCountry: '—',
      sku: '—',
      salesOrder: 'SHOPIFY 1001 #2',
      sourceNotes: ['品名取自箱单', '单价取自商品申报资料', '币种取自商品申报资料'],
    },
  ]);
});

test('语言未定(und)的单一品名只显文字,不挂语言前缀', () => {
  const [row] = goodsLineRowsOf({ lines: [{ line: 1, descriptions: [{ lang: 'und', text: cell('手表') }] }] });
  equal(row.description, '手表');
});

test('一箱的装箱分配摊成「第几行 ×数量」,没报为空', () => {
  equal(packingSummaryOf([{ line: 1, quantity: '24' }, { line: 2, quantity: '2' }]), '第 1 行 ×24；第 2 行 ×2');
  equal(packingSummaryOf(undefined), '');
});
