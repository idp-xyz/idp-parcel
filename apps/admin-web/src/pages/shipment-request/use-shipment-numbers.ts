import { useEffect, useMemo, useState } from 'react';
import { findShipmentRequestViewsByIds } from './api';
import {
  shipmentIdBatches,
  shipmentNumberIndexOf,
  shipmentNumberReader,
  type ShipmentNumberCell,
  type ShipmentNumberIndex,
} from './shipment-numbers';

// 别的上下文的页面按页上出现的委托标识取回运单号(规则在 shipment-numbers.ts)。标识集合变了或页面刷新了才再问——集合按排序
// 去重后的 JSON 判变,行数组每次渲染换新引用也不重问;刷新令牌是页面自己的重取计数,点刷新时一起重问,上一次读不到留下的
// 「读不到」注明不会一直挂着。再问期间沿用上一份索引:单据号开立后不变,旧索引里有的那几份照旧对,新出现的先显委托标识,
// 答复到了再换上。

/** 委托标识 → 屏上那一格。读回之前显委托标识。refreshToken 变了就重问,传页面的刷新计数。 */
export function useShipmentNumbers(
  shipmentRequestIds: readonly string[],
  refreshToken: number,
): (shipmentRequestId: string) => ShipmentNumberCell {
  const key = JSON.stringify([...new Set(shipmentRequestIds)].sort());
  const [index, setIndex] = useState<ShipmentNumberIndex | undefined>(undefined);

  useEffect(() => {
    const batches = shipmentIdBatches(JSON.parse(key) as string[]);
    if (batches.length === 0) return;
    let cancelled = false;
    void Promise.all(batches.map((batch) => findShipmentRequestViewsByIds(batch))).then((answers) => {
      if (!cancelled) setIndex(shipmentNumberIndexOf(answers));
    });
    return () => {
      cancelled = true;
    };
  }, [key, refreshToken]);

  return useMemo(() => shipmentNumberReader(index), [index]);
}
