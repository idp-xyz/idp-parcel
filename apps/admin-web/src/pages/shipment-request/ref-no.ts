// 运单号与包裹号的屏上写法(纯函数,无 React;refs.tsx 里的签组件用它)。运营找货认的是号本身,
// `DPCL-` / `SHR-` 这类形状前缀是线格式的事,上屏只添噪音;这里只管显,存储与线格式不动,整串随 title
// 与「复制」一起交回。短写法不是另一套标识:复制下来的永远是整串,查日志、对协议都用整串。

const parcelPrefix = 'DPCL-';
const shipmentPrefix = 'SHR-';

/** 包裹号去掉 `DPCL-` 前缀后的屏上写法;没有这个前缀原样交回,不替它猜结构。 */
export function parcelNoOf(parcelId: string): string {
  return parcelId.startsWith(parcelPrefix) ? parcelId.slice(parcelPrefix.length) : parcelId;
}

/** 运单号(委托标识)去掉 `SHR-` 前缀后的屏上写法。 */
export function shipmentNoOf(shipmentRequestId: string): string {
  return shipmentRequestId.startsWith(shipmentPrefix) ? shipmentRequestId.slice(shipmentPrefix.length) : shipmentRequestId;
}
