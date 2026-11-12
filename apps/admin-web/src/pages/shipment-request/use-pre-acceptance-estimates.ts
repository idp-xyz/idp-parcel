import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { getPreAcceptanceEstimates, type PreAcceptanceEstimatesResponseBody } from '../settlement/api';

// 运单详情向结算要这一票的受理前估价。各问各的、各自的加载与出错：结算那一口读不到，不让运单详情跟着空掉。不缓存：估价随
// 受理推进会多出新版本，每次打开详情都重读。

export function usePreAcceptanceEstimates(shipmentRequestId: string): {
  answer: ApiResult<PreAcceptanceEstimatesResponseBody> | null;
  retry: () => void;
} {
  const [answer, setAnswer] = useState<ApiResult<PreAcceptanceEstimatesResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void getPreAcceptanceEstimates(shipmentRequestId).then(
      (result) => {
        if (!cancelled) setAnswer(result);
      },
      (error: unknown) => {
        if (!cancelled) setAnswer({ kind: 'transport', message: error instanceof Error ? error.message : String(error) });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [shipmentRequestId, reloadToken]);
  return { answer, retry: () => setReloadToken((token) => token + 1) };
}
