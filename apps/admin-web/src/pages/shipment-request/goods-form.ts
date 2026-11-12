import type { GoodsDraft, GoodsLineDraft } from './api';
import { decimalShape } from './form-shapes';

// 下单页货物明细行的表单状态与组装(ADR-0373,票 declared-goods-lines/05)。小包下单页与平台货件页共用。
//
// 一行一种货;值原样交给服务端(两端空白去掉,这是页面输入框的常态,不是规范化客户的值)。页面只拦服务端必拒的形状——数量
// 与金额要是十进制数——不发明必填:哪些格必填由接单规则包说。

export interface GoodsLineForm {
  descriptionZh: string;
  descriptionEn: string;
  quantity: string;
  unit: string;
  unitValue: string;
  lineValue: string;
  currency: string;
  netWeight: string;
  netWeightUnit: string;
  exportCode: string;
  importCode: string;
  originCountry: string;
  sku: string;
  orderPlatform: string;
  orderNumber: string;
  orderLine: string;
}

export const emptyGoodsLine: GoodsLineForm = {
  descriptionZh: '',
  descriptionEn: '',
  quantity: '',
  unit: '',
  unitValue: '',
  lineValue: '',
  currency: '',
  netWeight: '',
  netWeightUnit: '',
  exportCode: '',
  importCode: '',
  originCountry: '',
  sku: '',
  orderPlatform: '',
  orderNumber: '',
  orderLine: '',
};

/** 内件性质的六格(UPU CN23),值是服务端的码。 */
export const contentCategoryOptions: { value: string; label: string }[] = [
  { value: 'SALE_OF_GOODS', label: '销售货物' },
  { value: 'GIFT', label: '礼品' },
  { value: 'COMMERCIAL_SAMPLE', label: '商业样品' },
  { value: 'DOCUMENTS', label: '文件' },
  { value: 'RETURNED_GOODS', label: '退回货物' },
  { value: 'OTHER', label: '其他' },
];

function trimmed(value: string): string | undefined {
  const text = value.trim();
  return text === '' ? undefined : text;
}

/** 一行是不是一格都没填:这样的行当没加过,不交给服务端(服务端对空行答形状错)。 */
export function isBlankGoodsLine(line: GoodsLineForm): boolean {
  return Object.values(line).every((value) => value.trim() === '');
}

function lineDraftOf(line: GoodsLineForm): GoodsLineDraft {
  const draft: GoodsLineDraft = {};
  const descriptions = [
    { lang: 'zh', text: trimmed(line.descriptionZh) },
    { lang: 'en', text: trimmed(line.descriptionEn) },
  ].filter((description): description is { lang: string; text: string } => description.text !== undefined);
  if (descriptions.length > 0) draft.descriptions = descriptions;
  const cells: [keyof GoodsLineDraft, string][] = [
    ['quantity', line.quantity],
    ['unit', line.unit],
    ['unitValue', line.unitValue],
    ['lineValue', line.lineValue],
    ['currency', line.currency],
    ['netWeight', line.netWeight],
    ['netWeightUnit', line.netWeightUnit],
  ];
  for (const [key, value] of cells) {
    const text = trimmed(value);
    if (text !== undefined) (draft as Record<string, unknown>)[key] = text;
  }
  const codes = [
    { side: 'EXPORT', code: trimmed(line.exportCode) },
    { side: 'IMPORT', code: trimmed(line.importCode) },
  ].filter((code): code is { side: string; code: string } => code.code !== undefined);
  if (codes.length > 0) draft.commodityCodes = codes;
  const origin = trimmed(line.originCountry);
  if (origin !== undefined) draft.originCountry = origin;
  const sku = trimmed(line.sku);
  if (sku !== undefined) draft.sku = sku;
  const order = {
    platform: trimmed(line.orderPlatform),
    orderNumber: trimmed(line.orderNumber),
    orderLine: trimmed(line.orderLine),
  };
  if (order.platform || order.orderNumber || order.orderLine) {
    draft.salesOrder = Object.fromEntries(Object.entries(order).filter(([, value]) => value !== undefined));
  }
  return draft;
}

/**
 * 组装委托级货物明细。空行不出;一行都没填、内件性质也没选,整段缺席。只有「其他」带说明。
 * 行的次序就是服务端的行号次序——装箱分配按它指行,所以调用方要用同一份去空行之后的次序(nonBlankGoodsLineIndexes)。
 */
export function goodsDraftOfLines(lines: GoodsLineForm[], contentCategory: string, contentCategoryNote: string): GoodsDraft | undefined {
  const drafts = lines.filter((line) => !isBlankGoodsLine(line)).map(lineDraftOf);
  if (drafts.length === 0 && !contentCategory) return undefined;
  const goods: GoodsDraft = { lines: drafts };
  if (contentCategory) goods.contentCategory = contentCategory;
  const note = trimmed(contentCategoryNote);
  if (contentCategory === 'OTHER' && note !== undefined) goods.contentCategoryNote = note;
  return goods;
}

/** 非空行在表单里的下标,按次序;第 k 个非空行就是服务端的第 k+1 行。货物行与装箱分配两半都按它编号,才说的是同一行。 */
export function nonBlankGoodsLineIndexes(lines: GoodsLineForm[]): number[] {
  return lines.flatMap((line, index) => (isBlankGoodsLine(line) ? [] : [index]));
}

/**
 * 只拦服务端必拒的形状:数量与金额、净重要是十进制数(键 `g{行下标}.{格}`);选了内件性质而一行货都没填,只有「文件」可以
 * (键 `contentCategory`)。
 */
export function validateGoodsLines(lines: GoodsLineForm[], contentCategory: string): Record<string, string> {
  const errors: Record<string, string> = {};
  lines.forEach((line, index) => {
    for (const field of ['quantity', 'unitValue', 'lineValue', 'netWeight'] as const) {
      const value = line[field].trim();
      if (value && !decimalShape(value)) errors[`g${index}.${field}`] = '请填数字,如 48 或 3.20,不带单位与千分位';
    }
  });
  if (contentCategory && contentCategory !== 'DOCUMENTS' && nonBlankGoodsLineIndexes(lines).length === 0) {
    errors.contentCategory = '选了内件性质就至少填一行货;只有「文件」可以不填';
  }
  return errors;
}
