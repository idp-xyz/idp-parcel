// 运单号与包裹号的屏上写法(纯函数,无 React;refs.tsx 里的签组件用它)。运营找货认的是号本身,
// `DPCL-` / `SHR-` 这类形状前缀是线格式的事,上屏只添噪音;这里只管显,存储与线格式不动,整串随 title
// 与「复制」一起交回。短写法不是另一套标识:复制下来的永远是整串,查日志、对协议都用整串。
//
// 运单号是委托的单据号(ADR-0355 决定十一),不是委托标识:有单据号就显它;没有单据号的委托(上线前建的,或建单时
// 单据编号时区未配置)照旧显去前缀的委托标识,技术详情按记下的情形注明为什么没有。

import { documentNumberAbsenceNote } from '../../vocabulary/document-numbering';

const parcelPrefix = 'DPCL-';
const shipmentPrefix = 'SHR-';

/** 包裹号去掉 `DPCL-` 前缀后的屏上写法;没有这个前缀原样交回,不替它猜结构。 */
export function parcelNoOf(parcelId: string): string {
  return parcelId.startsWith(parcelPrefix) ? parcelId.slice(parcelPrefix.length) : parcelId;
}

/** 委托标识去掉 `SHR-` 前缀后的屏上写法;只给没有单据号的委托顶「运单号」那一格用,有号的走 shipmentDisplayNoOf。 */
export function shipmentNoOf(shipmentRequestId: string): string {
  return shipmentRequestId.startsWith(shipmentPrefix) ? shipmentRequestId.slice(shipmentPrefix.length) : shipmentRequestId;
}

/**
 * 屏上一个号可能对应的委托标识:原样那一个,以及它若是去了前缀的短写法,补回前缀的那一个。shipmentNoOf 不替没有前缀的
 * 标识猜结构、原样交回,所以原样那一个也要问——补前缀只是多问一处,不是断定它就是短写法。
 */
export function shipmentRequestIdsOfDisplay(display: string): string[] {
  return display.startsWith(shipmentPrefix) ? [display] : [display, shipmentPrefix + display];
}

/** 一张运单在屏上的运单号:有单据号显单据号(原样,不裁前缀);没有就显去前缀的委托标识。 */
export function shipmentDisplayNoOf(shipment: { shipmentRequestId: string; documentNumber?: string }): string {
  return shipment.documentNumber ?? shipmentNoOf(shipment.shipmentRequestId);
}

/**
 * 没有单据号时技术详情里那一句,说法取单据编号词典(各类单据共用一处);有号时交回 undefined。读口没带无号情形(例如
 * 释放未受理册上读不到的那份委托)时也交回 undefined——不替读口猜是哪一种。
 */
export function documentNumberAbsenceNoteOf(shipment: { documentNumber?: string; documentNumberAbsence?: string }): string | undefined {
  if (shipment.documentNumber !== undefined || shipment.documentNumberAbsence === undefined) return undefined;
  return documentNumberAbsenceNote(shipment.documentNumberAbsence);
}
