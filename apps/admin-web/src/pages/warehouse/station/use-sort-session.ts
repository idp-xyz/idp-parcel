import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { findGoodsByCode, getSortPlan } from '../api';
import { browserStorage } from './device-binding';
import { goodsLookupOf } from './pick-goods';
import type { ScanCue } from './scan-sound';
import {
  addTote,
  applyVerdict,
  dismissWrong,
  goodsIdentified,
  putAll,
  reloadPlan,
  resend,
  scanCode,
  setAside,
  sortFactValues,
  sortOutcomeLabels,
  startSort,
  type SortEffect,
  type SortState,
  type SortStep,
} from './sort-flow';
import { sortFactSpec, sortPlanOf } from './sort-plan';
import { judgeWorkFactAnswer, mintWorkFact, sendWorkFact, type WorkFactAttempt, type WorkFactVerdict } from './work-fact-submit';

// 分拣工位的接线：把状态机（sort-flow.ts）接上读口、提交与本机存储。这里不判断扫到的码对不对——每一步都是「调状态机的
// 纯函数拿到新状态与要做的事，把新状态放上屏，再去做那些事」。状态同时放在 ref 与 state 里，理由同拣货工位。

export type SortLoad = { phase: 'loading' } | { phase: 'refused'; message: string } | { phase: 'error'; message: string } | { phase: 'ready' };

export interface SortSessionInput {
  deviceId: string;
  warehouse: string;
  wave: string;
  cue: (cue: ScanCue) => void;
}

export interface SortSession {
  load: SortLoad;
  state: SortState | null;
  verdict: WorkFactVerdict | null;
  refreshing: boolean;
  /** 刷新没读成的原因；屏上的分拣情况还是刷新之前的。 */
  refreshError: string | null;
  scan: (raw: string) => void;
  setAside: () => void;
  addTote: () => void;
  putAll: () => void;
  resend: () => void;
  dismiss: () => void;
  refresh: () => void;
}

export function useSortSession({ deviceId, warehouse, wave, cue }: SortSessionInput): SortSession {
  const [load, setLoad] = useState<SortLoad>({ phase: 'loading' });
  const [state, setState] = useState<SortState | null>(null);
  const [verdict, setVerdict] = useState<WorkFactVerdict | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  const stateRef = useRef<SortState | null>(null);
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const floorRef = useRef(0);
  const cueRef = useRef(cue);
  cueRef.current = cue;
  // 同一位货主名下同一个码这一趟只问一次，「认不出」也记下；没问成的不记，下一次还问。
  const codesRef = useRef(new Map<string, string | null>());
  const applyRef = useRef<(step: SortStep) => void>(() => undefined);

  const finish = useCallback((answered: WorkFactVerdict) => {
    setVerdict(answered);
    const current = stateRef.current;
    if (current !== null) applyRef.current(applyVerdict(current, answered));
  }, []);

  const send = useCallback(
    (attempt: WorkFactAttempt) => {
      void sendWorkFact(attempt).then((answer) => finish(judgeWorkFactAnswer(answer, sortOutcomeLabels)));
    },
    [finish],
  );

  const identify = useCallback(async (code: string, owners: readonly string[]) => {
    const resolved: Record<string, string | null> = {};
    for (const owner of owners) {
      const key = `${owner}\u0000${code}`;
      let goods = codesRef.current.get(key);
      if (goods === undefined) {
        const found = goodsLookupOf(await findGoodsByCode(owner, code));
        if (found.kind === 'failed') {
          const current = stateRef.current;
          if (current !== null) applyRef.current(goodsIdentified(current, code, null, `读商品资料没成：${found.reason}`));
          return;
        }
        goods = found.kind === 'found' ? found.goodsCode : null;
        codesRef.current.set(key, goods);
      }
      resolved[owner] = goods;
    }
    const current = stateRef.current;
    if (current !== null) applyRef.current(goodsIdentified(current, code, resolved));
  }, []);

  const runEffect = useCallback(
    (effect: SortEffect) => {
      switch (effect.kind) {
        case 'identify':
          void identify(effect.code, effect.owners);
          return;
        case 'submit': {
          const minted = mintWorkFact(
            browserStorage(),
            { spec: sortFactSpec, deviceId, warehouse, values: sortFactValues(effect.submission, wave) },
            floorRef.current,
          );
          if (!minted.ok) {
            finish({ verdict: 'refused', headline: `这一笔装不出来：${minted.reason}。` });
            return;
          }
          attemptRef.current = minted.attempt;
          floorRef.current = minted.attempt.sequence;
          send(minted.attempt);
          return;
        }
        case 'resend':
          if (attemptRef.current !== null) send(attemptRef.current);
          return;
        case 'reload':
          setTick((value) => value + 1);
          return;
      }
    },
    [deviceId, warehouse, wave, finish, identify, send],
  );

  const apply = useCallback(
    (step: SortStep) => {
      stateRef.current = step.state;
      setState(step.state);
      if (step.cue !== undefined) cueRef.current(step.cue);
      for (const effect of step.effects) runEffect(effect);
    },
    [runEffect],
  );
  applyRef.current = apply;

  // 读分拣情况：第一次读到就开工；以后（刷新、后端说不合）读到的交给状态机换上去。
  useEffect(() => {
    let cancelled = false;
    setRefreshing(true);
    setRefreshError(null);
    void getSortPlan(wave).then((answer) => {
      if (cancelled) return;
      setRefreshing(false);
      const plan = sortPlanOf(answer, wave, warehouse);
      if (plan.kind !== 'plan') {
        // 已经开工之后刷新没读成：屏上的留着，只说这一次没读成
        if (stateRef.current !== null) {
          setRefreshError(plan.reason);
          return;
        }
        setLoad({ phase: plan.kind === 'refused' ? 'refused' : 'error', message: plan.reason });
        return;
      }
      setLoad({ phase: 'ready' });
      const current = stateRef.current;
      apply(current === null ? { state: startSort(wave, plan.orders), effects: [] } : reloadPlan(current, plan.orders));
    });
    return () => {
      cancelled = true;
    };
  }, [wave, warehouse, tick, apply]);

  const scan = useCallback(
    (raw: string) => {
      const current = stateRef.current;
      if (current !== null) apply(scanCode(current, raw));
    },
    [apply],
  );

  const act = useCallback(
    (action: (current: SortState) => SortStep) => () => {
      const current = stateRef.current;
      if (current !== null) apply(action(current));
    },
    [apply],
  );
  const aside = useMemo(() => act(setAside), [act]);
  const another = useMemo(() => act(addTote), [act]);
  const all = useMemo(() => act(putAll), [act]);
  const resendNow = useMemo(() => act(resend), [act]);
  const dismiss = useMemo(() => act(dismissWrong), [act]);
  const refresh = useCallback(() => setTick((value) => value + 1), []);

  return {
    load,
    state,
    verdict,
    refreshing,
    refreshError,
    scan,
    setAside: aside,
    addTote: another,
    putAll: all,
    resend: resendNow,
    dismiss,
    refresh,
  };
}
