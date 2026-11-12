import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { emptyGoodsLine, goodsDraftOfLines, validateGoodsLines } from './goods-form';

// 本文件钉下单页货物明细行的组装(ADR-0373,票 declared-goods-lines/05):一行一种货,值原样交给服务端;一格都没填的行当没加过;
// 页面只拦服务端必拒的形状(数量与金额不是数),不发明必填。

test('填了的行原样成行,中英文品名各一条,编码分出口与进口;空行不出', () => {
  const draft = goodsDraftOfLines(
    [
      {
        ...emptyGoodsLine,
        descriptionZh: '男式棉质T恤',
        descriptionEn: "Men's cotton T-shirt",
        quantity: '48',
        unit: 'PCS',
        unitValue: '3.20',
        currency: 'USD',
        exportCode: '6109100000',
        originCountry: 'CN',
        sku: 'SKU-TEE-1',
        orderNumber: '1001',
      },
      { ...emptyGoodsLine },
    ],
    'SALE_OF_GOODS',
    '',
  );
  deepEqual(draft, {
    contentCategory: 'SALE_OF_GOODS',
    lines: [
      {
        descriptions: [
          { lang: 'zh', text: '男式棉质T恤' },
          { lang: 'en', text: "Men's cotton T-shirt" },
        ],
        quantity: '48',
        unit: 'PCS',
        unitValue: '3.20',
        currency: 'USD',
        commodityCodes: [{ side: 'EXPORT', code: '6109100000' }],
        originCountry: 'CN',
        sku: 'SKU-TEE-1',
        salesOrder: { orderNumber: '1001' },
      },
    ],
  });
});

test('一行都没填、内件性质也没选时整段缺席;只选了「文件」照样交', () => {
  equal(goodsDraftOfLines([{ ...emptyGoodsLine }], '', ''), undefined);
  deepEqual(goodsDraftOfLines([], 'DOCUMENTS', ''), { contentCategory: 'DOCUMENTS', lines: [] });
});

test('「其他」的说明随内件性质交,别的格不带说明', () => {
  deepEqual(goodsDraftOfLines([{ ...emptyGoodsLine, quantity: '1' }], 'OTHER', '维修配件')?.contentCategoryNote, '维修配件');
  equal(goodsDraftOfLines([{ ...emptyGoodsLine, quantity: '1' }], 'GIFT', '生日')?.contentCategoryNote, undefined);
});

test('数量与金额不是数时在那一格标出,其余不拦', () => {
  const errors = validateGoodsLines([{ ...emptyGoodsLine, quantity: '48 件', lineValue: '1,200', descriptionEn: 'Tee' }], '');
  deepEqual(Object.keys(errors).sort(), ['g0.lineValue', 'g0.quantity']);
  deepEqual(validateGoodsLines([{ ...emptyGoodsLine, quantity: '48', unitValue: '3.20' }], ''), {});
});

test('选了内件性质就至少要一行货,只有「文件」可以不填;什么都没选、没填就整段不报', () => {
  equal(validateGoodsLines([{ ...emptyGoodsLine }], 'GIFT').contentCategory, '选了内件性质就至少填一行货;只有「文件」可以不填');
  deepEqual(validateGoodsLines([{ ...emptyGoodsLine }], 'DOCUMENTS'), {});
  deepEqual(validateGoodsLines([{ ...emptyGoodsLine }], ''), {});
});
