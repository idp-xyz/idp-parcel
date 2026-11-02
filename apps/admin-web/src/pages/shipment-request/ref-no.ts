// 运单号与包裹号的屏上写法(纯函数,无 React;refs.tsx 里的签组件用它)。运营找货认的是号本身,
// `DPCL-` / `SHR-` 这类形状前缀是线格式的事,上屏只添噪音;这里只管显,存储与线格式不动,整串随 title
// 与「复制」一起交回。短写法不是另一套标识:复制下来的永远是整串,查日志、对协议都用整串。
//
// 运单号是委托的单据号(ADR-0355 决定十一),不是委托标识:有单据号就显它;没有单据号的委托(上线前建的,或建单时
// 单据编号时区未配置)照旧显去前缀的委托标识,技术详情按记下的情形注明为什么没有。

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

/** 一张运单在屏上的运单号:有单据号显单据号(原样,不裁前缀);没有就显去前缀的委托标识。 */
export function shipmentDisplayNoOf(shipment: { shipmentRequestId: string; documentNumber?: string }): string {
  return shipment.documentNumber ?? shipmentNoOf(shipment.shipmentRequestId);
}

/**
 * 没有单据号时技术详情里那一句;有号时交回 undefined。读口没带情形(旧版后端)时按上线前开立说——那正是两列皆空的意思。
 * 两种情形分开说:「时区未配置」要租户去规则册登时区,「上线前开立」什么也不用做,并成一句会让人去做没用的事。
 */
export function documentNumberAbsenceNoteOf(shipment: { documentNumber?: string; documentNumberAbsence?: string }): string | undefined {
  if (shipment.documentNumber !== undefined) return undefined;
  return shipment.documentNumberAbsence === 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
    ? '开立时单据编号时区未配置，无单据号'
    : '上线前开立，无单据号';
}
