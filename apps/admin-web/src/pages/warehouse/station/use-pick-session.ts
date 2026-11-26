import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { getSettingsOverview, getWave, listTotes, listWaves } from '../api';
import { workFactSpecs } from '../work-fact';
import { browserStorage } from './device-binding';
import { liveGoodsIdentifier, type GoodsIdentifier } from './pick-goods';
import {
  applyVerdict,
  beginPick,
  changeCount,
  changeStaging,
  dismissWrong,
  goodsIdentified,
  nextCell,
  pickFactValues,
  pickOutcomeLabels,
  reloadQueue,
  resend,
  scanCode,
  setToteMode,
  skipCell,
  skipTote,
  stagingVerified,
  submitShortPick,
  type PickEffect,
  type PickState,
  type PickStep,
} from './pick-flow';
import { loadPickQueue, type PickQueue } from './pick-queue';
import { readPickStaging, writePickStaging } from './pick-staging';
import { warehouseToteMode } from './pick-totes';
import { liveStagingVerifier, type StagingVerifier } from './pick-staging-check';
import type { ScanCue } from './scan-sound';
import {
  describeReadFailure,
  judgeWorkFactAnswer,
  mintWorkFact,
  sendWorkFact,
  type WorkFactAttempt,
  type WorkFactVerdict,
} from './work-fact-submit';

// 拣货工位的接线：把状态机（pick-flow.ts）接上读口、提交与本机存储。这里不判断扫到的码对不对——每一步都是「调状态机的纯函数
// 拿到新状态与要做的事，把新状态放上屏，再去做那些事」；做完事得到的答复再交回状态机。
//
// 状态同时放在 ref 与 state 里：事件处理函数读 ref 拿到的永远是最新的一份（枪一秒能连着来两次），state 只负责触发重渲。

const pickSpec = (() => {
  const spec = workFactSpecs.find((candidate) => candidate.id === 'pick');
  if (spec === undefined) throw new Error('作业事实定义里没有拣货');
  return spec;
})();

export type PickLoad = { phase: 'loading' } | { phase: 'error'; message: string } | { phase: 'ready'; queue: PickQueue };

export interface PickSessionInput {
  deviceId: string;
  /** 设备绑的仓。 */
  warehouse: string;
  /** 只做这个波次；null 是本仓全部波次。 */
  wave: string | null;
  cue: (cue: ScanCue) => void;
}

export interface PickSession {
  load: PickLoad;
  /** 队列读出来之前是 null。 */
  state: PickState | null;
  /** 刚收到的后端答复；没送到与没成时屏上要配它的技术详情与重新登录。 */
  verdict: WorkFactVerdict | null;
  refreshing: boolean;
  /** 刷新没读成时的原因（第一次读不成走 load 的 error）；队列还是刷新之前的样子，屏上要说清它可能不是最新的。 */
  refreshError: string | null;
  scan: (raw: string) => void;
  changeQuantity: (text: string) => { ok: true } | { ok: false; reason: string };
  confirmShortPick: () => void;
  skip: () => void;
  changeStagingLocation: () => void;
  resend: () => void;
  next: () => void;
  dismiss: () => void;
  refresh: () => void;
  /** 「不用箱」。 */
  skipTote: () => void;
}

export function usePickSession({ deviceId, warehouse, wave, cue }: PickSessionInput): PickSession {
  const [load, setLoad] = useState<PickLoad>({ phase: 'loading' });
  const [state, setState] = useState<PickState | null>(null);
  const [verdict, setVerdict] = useState<WorkFactVerdict | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  const stateRef = useRef<PickState | null>(null);
  // 没送到时「再送一次」用的就是它：事实号在铸这一笔时定下，之后送几次都是它。
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const floorRef = useRef(0);
  const cueRef = useRef(cue);
  cueRef.current = cue;
  const identifierRef = useRef<GoodsIdentifier | null>(null);
  if (identifierRef.current === null) identifierRef.current = liveGoodsIdentifier();
  const verifierRef = useRef<StagingVerifier | null>(null);
  if (verifierRef.current === null) verifierRef.current = liveStagingVerifier();

  const applyRef = useRef<(step: PickStep) => void>(() => undefined);

  const finish = useCallback((answered: WorkFactVerdict) => {
    setVerdict(answered);
    const current = stateRef.current;
    const submitted = current?.submission ?? null;
    if (current !== null) applyRef.current(applyVerdict(current, answered));
    // 少拣成立之后，后端会在这个波次里换别的库位补这几件；添出来的格在后端，重读队列才看得到。
    if (answered.verdict === 'done' && answered.outcome === 'PICKED' && submitted !== null && submitted.quantity < submitted.cell.quantity) {
      setTick((value) => value + 1);
    }
  }, []);

  const send = useCallback(
    (attempt: WorkFactAttempt) => {
      void sendWorkFact(attempt).then((answer) => finish(judgeWorkFactAnswer(answer, pickOutcomeLabels)));
    },
    [finish],
  );

  const runEffect = useCallback(
    (effect: PickEffect, from: PickState) => {
      switch (effect.kind) {
        case 'remember-staging':
          writePickStaging(browserStorage(), deviceId, effect.code);
          return;
        case 'identify':
          void identifierRef.current?.identify(effect.cell, effect.code).then((result) => {
            const current = stateRef.current;
            if (current !== null) applyRef.current(goodsIdentified(current, effect.code, result));
          });
          return;
        case 'verify-staging':
          void verifierRef.current?.verify(warehouse, effect.code).then((verdict) => {
            const current = stateRef.current;
            if (current !== null) applyRef.current(stagingVerified(current, effect.code, verdict));
          });
          return;
        case 'submit': {
          const minted = mintWorkFact(
            browserStorage(),
            {
              spec: pickSpec,
              deviceId,
              warehouse,
              values: pickFactValues(
                { cell: effect.cell, quantity: effect.quantity, ...(effect.tote === undefined ? {} : { tote: effect.tote }) },
                from.staging ?? '',
              ),
            },
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
      }
    },
    [deviceId, warehouse, finish, send],
  );

  const apply = useCallback(
    (step: PickStep) => {
      stateRef.current = step.state;
      setState(step.state);
      if (step.cue !== undefined) cueRef.current(step.cue);
      for (const effect of step.effects) runEffect(effect, step.state);
    },
    [runEffect],
  );
  applyRef.current = apply;

  // 读队列：第一次读到之后开工；以后（刷新）读到的交给状态机换队列。
  useEffect(() => {
    let cancelled = false;
    setRefreshing(true);
    setRefreshError(null);
    void loadPickQueue({ warehouse, wave, list: listWaves, one: getWave }).then((answer) => {
      if (cancelled) return;
      setRefreshing(false);
      if (answer.kind === 'failed') {
        const message = describeReadFailure(answer.failure);
        // 已经读成过的队列留着：刷新没读成不把屏上的格抹掉，只说这一次没读成
        setLoad((previous) => (previous.phase === 'ready' ? previous : { phase: 'error', message }));
        setRefreshError(message);
        return;
      }
      setLoad({ phase: 'ready', queue: answer.queue });
      const current = stateRef.current;
      if (current !== null) apply(reloadQueue(current, answer.queue.cells));
    });
    return () => {
      cancelled = true;
    };
  }, [warehouse, wave, tick, apply]);

  // 队列读出来了就开工，只开一次：本机记着的放货位由状态机先去核，核完才收扫码。刷新读到新队列时 load 换了一个对象，
  // 这个 effect 会再跑一遍，stateRef 拦住它不让重新开工；换队列是 reloadQueue 的事。
  useEffect(() => {
    if (stateRef.current !== null || load.phase !== 'ready') return;
    apply(beginPick(load.queue.cells, readPickStaging(browserStorage(), deviceId)));
  }, [load, deviceId, apply]);

  // 这个仓登记了周转箱没有、租户要不要求用箱：开工后读一次，读回来告诉状态机。没读成就当没有箱、不要求——
  // 读成与否都不拦着拣；真要求的话后端会拒不带箱的那一笔，状态机从那一笔起改成必须用箱。
  useEffect(() => {
    if (load.phase !== 'ready') return;
    let cancelled = false;
    void warehouseToteMode(warehouse, listTotes, getSettingsOverview).then((mode) => {
      const current = stateRef.current;
      if (!cancelled && current !== null) apply(setToteMode(current, mode.on, mode.required));
    });
    return () => {
      cancelled = true;
    };
    // 只在队列第一次读成、或换了仓时读；刷新队列不必重读
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load.phase, warehouse, apply]);

  const scan = useCallback(
    (raw: string) => {
      const current = stateRef.current;
      if (current !== null) apply(scanCode(current, raw));
    },
    [apply],
  );

  const changeQuantity = useCallback(
    (text: string) => {
      const current = stateRef.current;
      if (current === null) return { ok: false as const, reason: '还没开工。' };
      const result = changeCount(current, text);
      if (!result.ok) return result;
      apply(result.step);
      return { ok: true as const };
    },
    [apply],
  );

  // 其余几个动作都是「拿最新的状态调一个纯函数、把结果放上屏」。
  const act = useCallback(
    (action: (current: PickState) => PickStep) => () => {
      const current = stateRef.current;
      if (current !== null) apply(action(current));
    },
    [apply],
  );
  const confirmShortPick = useMemo(() => act(submitShortPick), [act]);
  const skip = useMemo(() => act(skipCell), [act]);
  const changeStagingLocation = useMemo(() => act(changeStaging), [act]);
  const resendNow = useMemo(() => act(resend), [act]);
  const next = useMemo(() => act(nextCell), [act]);
  const dismiss = useMemo(() => act(dismissWrong), [act]);
  const withoutTote = useMemo(() => act(skipTote), [act]);
  const refresh = useCallback(() => setTick((value) => value + 1), []);

  return {
    load,
    state,
    verdict,
    refreshing,
    refreshError,
    scan,
    changeQuantity,
    confirmShortPick,
    skip,
    changeStagingLocation,
    resend: resendNow,
    next,
    dismiss,
    refresh,
    skipTote: withoutTote,
  };
}
