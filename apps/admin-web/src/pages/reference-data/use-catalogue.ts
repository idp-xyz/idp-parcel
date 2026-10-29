import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';

/** 取一份读口的答案；挂载时取，刷新（retry）重取。读口函数是模块级恒等函数，只随刷新钮重取。 */
export function useCatalogue<Body>(load: () => Promise<ApiResult<Body>>) {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void load().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadToken]);
  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  return { answer, retry, refreshing };
}
