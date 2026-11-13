// 「这条记录是人录的还是接口来的」的统一读法。
//
// 产品形态规定每个依赖外部系统的环节都先有手工口，且人工录入的事实必须与接口送来的分得开。后端读面的做法是：
// 人工录入的记录带一格留痕（谁、凭哪份原件），接口来的整格缺席。这里只把那一格示成文字，不由别的字段猜来源
// ——标识前缀是写口防撞用的，不是给页面判来源用的。

import { shortenEmbeddedPrincipalRefs } from '../components/principal-ref';

/** 后端读面的 manualEntry：整键缺席即接口送来的。enteredAt 只在该读面记了录入时刻时在场。 */
export interface ManualEntryRecord {
  enteredBy: string;
  original: string;
  enteredAt?: string;
}

export const manualEntryWord = '人工录入';
export const interfaceDeliveredWord = '接口送达';

/** 来源两格之一。 */
export function entrySourceWord(entry: ManualEntryRecord | undefined): string {
  return entry ? manualEntryWord : interfaceDeliveredWord;
}

/** 谁、凭哪份原件。verb 是这一环节的动词（录入、递交、告知）。 */
export function manualTrace(who: string, original: string, verb = '录入'): string {
  return `${shortenEmbeddedPrincipalRefs(who)} ${verb}，原件：${original}`;
}

/** 一格文字：人工录入的带出谁与原件，接口来的只说「接口送达」。 */
export function entrySourceLine(entry: ManualEntryRecord | undefined): string {
  return entry ? `${manualEntryWord}｜${manualTrace(entry.enteredBy, entry.original)}` : interfaceDeliveredWord;
}
