import { useCallback, useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { getReturnsOverview, type ReturnsOverviewBody } from './api';

export type ReturnsOverviewAnswer = ApiResult<ReturnsOverviewBody>;

export interface ReturnsOverviewState {
  /** 读到之前是 null。重读时留着上一份，读到了才换：做完一个动作之后表不该先闪成空白。 */
  answer: ReturnsOverviewAnswer | null;
  reload: () => void;
  /** 点了刷新、这一问还在途。第一次读不算，免得一进页面就报「已刷新」。 */
  reloading: boolean;
}

/** 列表的详情栏与列表共读这一份概览：换一行不用再读一遍，整份已经在手里。 */
export interface ReturnsPanel {
  onClose: () => void;
  onChanged: () => void;
  overview: ReturnsOverviewAnswer | null;
  refreshing: boolean;
}

/**
 * 退货概览：两张列表共读这一份，持有它的是页签外面的那一层——切页签、筛选都不重取，点刷新与做完动作才重取。
 * 概览是整份答回的，列表上的筛选、状态与数量都在这份已取回的数据上算，不再发请求。
 */
export function useReturnsOverview(): ReturnsOverviewState {
  const [answer, setAnswer] = useState<ReturnsOverviewAnswer | null>(null);
  const [reloads, setReloads] = useState(0);
  const [reloading, setReloading] = useState(false);
  const reload = useCallback(() => setReloads((count) => count + 1), []);

  useEffect(() => {
    let cancelled = false;
    if (reloads > 0) setReloading(true);
    void getReturnsOverview().then((next) => {
      if (cancelled) return;
      setAnswer(next);
      setReloading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloads]);

  return { answer, reload, reloading };
}

/**
 * 单据页读概览：键只用来区分是哪一张单据（换一张就重取、不把上一张的答复画到这一张上），读口本身不按号取。
 * `useKeyedLoad` 要的是稳定引用，所以放在模块里。
 */
export function loadReturnsOverview(_key: string): Promise<ReturnsOverviewAnswer> {
  return getReturnsOverview();
}
