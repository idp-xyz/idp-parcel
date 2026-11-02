// 别的上下文的页面上出现的委托怎么显运单号(纯逻辑,无 React,node:test 钉着;取数的 hook 在 use-shipment-numbers.ts)。
//
// 委托与它的单据号归 parcel-shipment,引用委托的上下文(例如 network-routing 的路线规划)读模型里只有委托标识,不复制单据号
// (CONTEXT-MAP 的所有权)。所以页面把页上出现的那几份委托标识经委托查阅读口一次取回(`?shipmentRequestIds=`),再照
// ADR-0355 决定十一显:有号显运单号;没有号的照旧显去前缀的委托标识,技术详情按读口记下的情形注明为什么没有;读口没答的,
// 只让号这一格降级——显委托标识并注明读不到,不拖垮引用它的那一页;看不见的(作用域外或查无)照旧显委托标识,不替它说什么。

import { documentTypeWords } from '../../vocabulary/document-numbering';
import { SHIPMENT_IDS_PER_LOOKUP, type ApiResult, type ShipmentRequestSummary, type ViewsListResponseBody } from './api';
import { documentNumberAbsenceNoteOf, shipmentNoOf } from './ref-no';

/** 取回的运单概要按委托标识索引;null 是读口没答。 */
export type ShipmentNumberIndex = ReadonlyMap<string, ShipmentRequestSummary> | null;

/** 引到委托处的一格:屏上的号、有号时的运单号本身(检索用)、技术详情里关于这个号的那一句(没有可说的为空串)。 */
export interface ShipmentNumberCell {
  text: string;
  documentNumber?: string;
  note: string;
}

/** 去重后按读口一次收的上限分批;保留首次出现的次序,空标识不问。 */
export function shipmentIdBatches(ids: Iterable<string>, size: number = SHIPMENT_IDS_PER_LOOKUP): string[][] {
  const unique = [...new Set([...ids].filter((id) => id.trim() !== ''))];
  const batches: string[][] = [];
  for (let start = 0; start < unique.length; start += size) batches.push(unique.slice(start, start + size));
  return batches;
}

/**
 * 各批答复合成一份索引。任一批没形成答案就整份当读不到:缺一批的索引会把那一批的委托说成看不见,而它们只是没问到。
 */
export function shipmentNumberIndexOf(answers: readonly ApiResult<ViewsListResponseBody>[]): ShipmentNumberIndex {
  const index = new Map<string, ShipmentRequestSummary>();
  for (const answer of answers) {
    if (answer.kind !== 'outcome') return null;
    for (const row of answer.body.requests) index.set(row.shipmentRequestId, row);
  }
  return index;
}

const numberName = documentTypeWords.SHIPMENT.numberName;
const unreadableNote = `${numberName}读不到(运单查阅这次没有答复),暂显委托标识`;

/** 按索引给出一份委托在屏上的那一格。index 为 undefined 是还在读:先显委托标识,不注明。 */
export function shipmentNumberReader(index: ShipmentNumberIndex | undefined): (shipmentRequestId: string) => ShipmentNumberCell {
  return (shipmentRequestId) => {
    const fallback = shipmentNoOf(shipmentRequestId);
    if (index === undefined) return { text: fallback, note: '' };
    if (index === null) return { text: fallback, note: unreadableNote };
    const summary = index.get(shipmentRequestId);
    if (summary === undefined) return { text: fallback, note: '' };
    if (summary.documentNumber !== undefined) {
      return { text: summary.documentNumber, documentNumber: summary.documentNumber, note: '' };
    }
    return { text: fallback, note: documentNumberAbsenceNoteOf(summary) ?? '' };
  };
}
