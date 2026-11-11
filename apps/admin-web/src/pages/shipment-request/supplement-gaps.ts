import type { SupplementGapRecord } from './api';

// 受理停在等客户补充时缺的各项(ADR-0376 决定一,票 declared-goods-lines/09)的屏上写法。原因码取服务端的原词;认得的译成
// 「缺什么」,认不得的照原码显示——页面落后于服务端时宁可露出原码,也不把它并进某个认识的说法。

const missingWords: Record<string, string> = {
  PLATFORM_SHIPMENT_DECLARED: '平台货件(销售平台与平台货件号)',
  EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL: '平台箱标号',
  PACKING_DECLARED: '装箱分配',
};

export interface SupplementGapLine {
  /** 包裹号;委托级的缺口为空串。 */
  parcelId: string;
  missing: string;
}

/** 把缺口摊成屏上的行:委托级的排在前面,逐件的按包裹号排,同一件的几项并成一行。 */
export function supplementGapLinesOf(gaps: SupplementGapRecord[] | undefined): SupplementGapLine[] {
  const byParcel = new Map<string, string[]>();
  for (const gap of gaps ?? []) {
    const parcel = gap.parcelId ?? '';
    const words = byParcel.get(parcel) ?? [];
    words.push(missingWords[gap.reason] ?? gap.reason);
    byParcel.set(parcel, words);
  }
  return [...byParcel.entries()]
    .sort(([left], [right]) => (left === '' ? -1 : right === '' ? 1 : left.localeCompare(right)))
    .map(([parcelId, words]) => ({ parcelId, missing: words.join('、') }));
}
