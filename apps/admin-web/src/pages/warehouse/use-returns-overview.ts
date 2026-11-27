import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { listReturnsPage, type ReturnsListQuery, type ReturnsPageBody } from './api';

export interface ReturnsPageState {
  /** 这一问还没答出、也没有上一份可垫时是 null。 */
  answer: ApiResult<ReturnsPageBody> | null;
  reload: () => void;
  /** 换页、换筛选或点了刷新，这一问还在途。第一次读不算。 */
  reloading: boolean;
}

/**
 * 退货预报或到仓记录的一页。换条件时留着上一页，新的答到再换，表不先闪成空白。
 */
export function useReturnsPage(query: ReturnsListQuery): ReturnsPageState {
  const key = JSON.stringify(query);
  const [slot, setSlot] = useState<{ key: string; answer: ApiResult<ReturnsPageBody> } | null>(null);
  const [reloads, setReloads] = useState(0);
  const [reloading, setReloading] = useState(false);
  const reload = () => setReloads((count) => count + 1);

  useEffect(() => {
    let cancelled = false;
    if (slot !== null) setReloading(true);
    void listReturnsPage(query).then((answer) => {
      if (cancelled) return;
      setSlot({ key, answer });
      setReloading(false);
    });
    return () => {
      cancelled = true;
    };
    // query 每次渲染是新对象，认的是它的签名。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, reloads]);

  if (slot?.key === key) return { answer: slot.answer, reload, reloading };
  if (slot?.answer.kind === 'outcome') return { answer: slot.answer, reload, reloading: true };
  return { answer: null, reload, reloading };
}
