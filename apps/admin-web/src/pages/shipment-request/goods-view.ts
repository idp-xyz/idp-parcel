import type { GoodsCellView, GoodsLineView, GoodsView, PackingDraft } from './api';

// 委托详情里货物明细的屏上写法(ADR-0373,票 declared-goods-lines/04)。每格原样显示客户给的串,不换算、不补算;来源不是本单申报的
// 格单独点名——本单申报是客户渠道的常态,逐格都标反倒看不出哪格是仓配从箱单或商品申报资料里取的。

const contentCategoryLabels: Record<string, string> = {
  GIFT: '礼品',
  DOCUMENTS: '文件',
  COMMERCIAL_SAMPLE: '商业样品',
  RETURNED_GOODS: '退回货物',
  SALE_OF_GOODS: '销售货物',
  OTHER: '其他',
};

const sourceMarkLabels: Record<string, string> = {
  BOX_LIST: '箱单',
  COMMODITY_DECLARATION: '商品申报资料',
};

const commodityCodeSideLabels: Record<string, string> = { EXPORT: '出口', IMPORT: '进口' };

const absent = '—';

/** 内件性质的中文;没报为空串,词表外的码原样显示。 */
export function contentCategoryLabel(code: string | undefined): string {
  if (!code) return '';
  return contentCategoryLabels[code] ?? code;
}

export interface GoodsLineRow {
  line: number;
  description: string;
  quantity: string;
  unitValue: string;
  lineValue: string;
  currency: string;
  netWeight: string;
  commodityCodes: string;
  originCountry: string;
  sku: string;
  salesOrder: string;
  /** 来源不是本单申报的格,逐格写成「品名取自箱单」这样一句。 */
  sourceNotes: string[];
}

function valueOf(cell: GoodsCellView | undefined): string {
  return cell?.value ?? '';
}

function withUnit(amount: GoodsCellView | undefined, unit: GoodsCellView | undefined): string {
  if (!amount) return absent;
  return unit ? `${amount.value} ${unit.value}` : amount.value;
}

function descriptionOf(line: GoodsLineView): string {
  const descriptions = line.descriptions ?? [];
  if (descriptions.length === 0) return absent;
  // und 是「语言未定」:只有这一条时只显文字,挂个 und 前缀读的人看不懂。
  if (descriptions.length === 1 && descriptions[0].lang === 'und') return descriptions[0].text.value;
  return descriptions.map((description) => `${description.lang}: ${description.text.value}`).join(' / ');
}

function salesOrderOf(line: GoodsLineView): string {
  const order = line.salesOrder;
  if (!order) return absent;
  const head = [valueOf(order.platform), valueOf(order.orderNumber)].filter(Boolean).join(' ');
  const tail = order.orderLine ? ` #${order.orderLine.value}` : '';
  return `${head}${tail}`.trim() || absent;
}

function sourceNotesOf(line: GoodsLineView): string[] {
  const notes: string[] = [];
  const note = (field: string, cells: (GoodsCellView | undefined)[]) => {
    const marks = new Set(cells.map((cell) => (cell ? sourceMarkLabels[cell.mark] : undefined)).filter(Boolean));
    for (const mark of marks) notes.push(`${field}取自${mark}`);
  };
  note('品名', (line.descriptions ?? []).map((description) => description.text));
  note('数量', [line.quantity]);
  note('单位', [line.unit]);
  note('单价', [line.unitValue]);
  note('行总价', [line.lineValue]);
  note('币种', [line.currency]);
  note('净重', [line.netWeight, line.netWeightUnit]);
  note('编码', (line.commodityCodes ?? []).map((code) => code.code));
  note('原产国', [line.originCountry]);
  note('SKU', [line.sku]);
  note('销售订单', [line.salesOrder?.platform, line.salesOrder?.orderNumber, line.salesOrder?.orderLine]);
  return notes;
}

/** 把货物明细的每一行摊成屏上的几格,行的次序就是行号的次序。 */
export function goodsLineRowsOf(goods: GoodsView): GoodsLineRow[] {
  return goods.lines.map((line) => ({
    line: line.line,
    description: descriptionOf(line),
    quantity: withUnit(line.quantity, line.unit),
    unitValue: valueOf(line.unitValue) || absent,
    lineValue: valueOf(line.lineValue) || absent,
    currency: valueOf(line.currency) || absent,
    netWeight: withUnit(line.netWeight, line.netWeightUnit),
    commodityCodes:
      (line.commodityCodes ?? [])
        .map((code) => `${commodityCodeSideLabels[code.side] ?? code.side} ${code.code.value}`)
        .join(' / ') || absent,
    originCountry: valueOf(line.originCountry) || absent,
    sku: valueOf(line.sku) || absent,
    salesOrder: salesOrderOf(line),
    sourceNotes: sourceNotesOf(line),
  }));
}

/** 一箱的装箱分配摊成「第 1 行 ×24；第 2 行 ×2」;整票清单(没报)为空串。 */
export function packingSummaryOf(packing: PackingDraft[] | undefined): string {
  if (!packing || packing.length === 0) return '';
  return packing.map((allocation) => `第 ${allocation.line} 行 ×${allocation.quantity}`).join('；');
}
