import { useCallback, useEffect, useState } from 'react';
import { accessRefusalTitle, type AccessRefusalKind } from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';

const refusalKinds = new Set<string>(['unconfigured', 'loginRequired', 'notGranted']);

/** 把读口的非业务答复收成抽屉里的一句话。业务 outcome 不在这里解释。 */
export function readFailureMessage<T>(answer: ApiResult<T> | null): string | null {
  if (!answer || answer.kind === 'outcome') return null;
  if (answer.kind === 'transport') return answer.message;
  if (answer.kind === 'callerProblem') return answer.detail ?? answer.code;
  if (answer.kind === 'noAnswer') return answer.code;
  if (refusalKinds.has(answer.kind)) return accessRefusalTitle(answer.kind as AccessRefusalKind);
  return '读取失败';
}

/**
 * 按一把键取一份详情。键为空时不发请求。换键后先视为加载中，避免把上一张单据的答复画到这一张上。
 * `load` 必须是稳定引用。
 *
 * `reload` 用同一把键重读：读到之前留着上一份答复，不退回加载中——在单据页上做完一个动作后重读，正文不该先闪成空白。
 */
export function useKeyedLoad<T>(
  activeKey: string | null,
  load: (key: string) => Promise<ApiResult<T>>,
): { answer: ApiResult<T> | null; loading: boolean; reload: () => void } {
  const [slot, setSlot] = useState<{ key: string; answer: ApiResult<T> | null } | null>(null);
  const [reloads, setReloads] = useState(0);
  const reload = useCallback(() => setReloads((count) => count + 1), []);

  useEffect(() => {
    if (!activeKey) return;
    let cancelled = false;
    setSlot((current) => (current?.key === activeKey ? current : { key: activeKey, answer: null }));
    void load(activeKey).then((answer) => {
      if (!cancelled) setSlot({ key: activeKey, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [activeKey, load, reloads]);

  if (!activeKey) return { answer: null, loading: false, reload };
  if (!slot || slot.key !== activeKey || slot.answer === null) return { answer: null, loading: true, reload };
  return { answer: slot.answer, loading: false, reload };
}
