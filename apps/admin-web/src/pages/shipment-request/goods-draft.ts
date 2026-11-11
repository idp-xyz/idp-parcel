import type { GoodsDraft, GoodsLineDraft } from './api';

/** 下单页逐包裹填的那几格货物(品名、数量、申报价值、币种、原产地)。 */
export interface ParcelGoodsFields {
  goodsDescription: string;
  quantity: string;
  declaredValue: string;
  currency: string;
  originCountry: string;
}

/**
 * 把逐包裹填的货物格并成委托级的货物明细行(ADR-0373 决定二):每个填了货物的包裹出一行,一格都没填的不出行;
 * 一个包裹都没填就整段缺席。不报装箱分配——多件委托就是整票清单(决定三),逐箱怎么分留给 FBA 货件下单页。
 *
 * 品名的语言标签记 `und`(BCP 47「未确定」):这一页只有一个品名框,不知道客户填的是哪种语言,不替他猜。申报价值记作
 * 行总价:这一格从来说的是这个包裹里货物的价值,不是单价。
 */
export function goodsDraftOf(parcels: ParcelGoodsFields[]): GoodsDraft | undefined {
  const lines: GoodsLineDraft[] = [];
  for (const parcel of parcels) {
    const line: GoodsLineDraft = {};
    const description = parcel.goodsDescription.trim();
    if (description) line.descriptions = [{ lang: 'und', text: description }];
    if (parcel.quantity.trim()) line.quantity = parcel.quantity.trim();
    if (parcel.declaredValue.trim()) line.lineValue = parcel.declaredValue.trim();
    if (parcel.currency.trim()) line.currency = parcel.currency.trim();
    if (parcel.originCountry.trim()) line.originCountry = parcel.originCountry.trim();
    if (Object.keys(line).length > 0) lines.push(line);
  }
  return lines.length > 0 ? { lines } : undefined;
}
