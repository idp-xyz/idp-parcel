import { useCallback, useEffect, useRef, useState } from 'react';
import { findGoodsByCode, getCountDetail, listCounts, type StockCountRecord } from '../api';
import { workFactSpecs } from '../work-fact';
import { browserStorage } from './device-binding';
import {
  chooseStockState,
  countOpened,
  countVerdict,
  goodsFound,
  markUnknown,
  resend,
  scanCode,
  setQuantity,
  startCount,
  submitLocation,
  type CountEffect,
  type CountSheet,
  type CountState,
  type CountStep,
} from './count-flow';
import type { ScanCue } from './scan-sound';
import { describeReadFailure, judgeWorkFactAnswer, mintWorkFact, sendWorkFact, type WorkFactAttempt } from './work-fact-submit';

function sheetOf(count: StockCountRecord, entries: ReadonlyArray<{ locationCode: string }> | undefined): CountSheet {
  return {
    countId: count.countId,
    warehouse: count.warehouseCode,
    status: count.status,
    owner: count.ownerAccount && count.ownerAccount.trim() !== '' ? count.ownerAccount : null,
    locations: [...count.locations],
    counted: [...new Set((entries ?? []).map((entry) => entry.locationCode))],
  };
}

export function useCountSession(input: { deviceId: string; warehouse: string; countId: string | null; cue: (cue: ScanCue) => void }) {
  const [state, setState] = useState<CountState>(() => startCount(input.countId).state);
  const stateRef = useRef(state);
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const [queue, setQueue] = useState<StockCountRecord[]>([]);
  const [queueNote, setQueueNote] = useState('在读盘点中的盘点单。');

  const apply = useCallback((step: CountStep) => {
    stateRef.current = step.state;
    setState(step.state);
    if (step.state.cue) input.cue(step.state.cue);
    return step;
  }, [input]);

  const run = useCallback((effects: CountEffect[]) => {
    for (const effect of effects) {
      if (effect.kind === 'open') {
        void getCountDetail(effect.countId).then((answer) => {
          if (answer.kind !== 'outcome' || !answer.body.count) {
            const failed = answer.kind === 'outcome' ? undefined : describeReadFailure(answer);
            apply(countOpened(stateRef.current, answer.kind === 'outcome' ? null : null, input.warehouse, failed));
            return;
          }
          const step = countOpened(stateRef.current, sheetOf(answer.body.count, answer.body.entries), input.warehouse);
          apply(step);
        });
      }
      if (effect.kind === 'identify') {
        void findGoodsByCode(effect.owner, effect.code).then((answer) => {
          const goods = answer.kind === 'outcome' && answer.body.found ? answer.body.goods : undefined;
          const step = goodsFound(stateRef.current, effect.code, goods ? { goodsCode: goods.goodsCode, unit: goods.baseUnit } : null);
          apply(step);
        });
      }
      if (effect.kind === 'submit') {
        const spec = workFactSpecs.find((item) => item.id === 'count-entry');
        if (!spec) return;
        const minted = mintWorkFact(browserStorage(), { spec, deviceId: input.deviceId, warehouse: input.warehouse, values: effect.values });
        if (!minted.ok) {
          apply(countVerdict(stateRef.current, { verdict: 'refused', headline: minted.reason }));
          continue;
        }
        attemptRef.current = minted.attempt;
        void sendWorkFact(minted.attempt).then((answer) => apply(countVerdict(stateRef.current, judgeWorkFactAnswer(answer))));
      }
      if (effect.kind === 'resend' && attemptRef.current) {
        void sendWorkFact(attemptRef.current).then((answer) => apply(countVerdict(stateRef.current, judgeWorkFactAnswer(answer))));
      }
    }
  }, [apply, input.deviceId, input.warehouse]);

  const act = useCallback((step: CountStep) => {
    apply(step);
    run(step.effects);
  }, [apply, run]);

  useEffect(() => {
    const started = startCount(input.countId);
    stateRef.current = started.state;
    setState(started.state);
    run(started.effects);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [input.deviceId, input.countId, input.warehouse]);

  useEffect(() => {
    let cancelled = false;
    void listCounts({ filters: { warehouseCode: [input.warehouse], status: ['COUNTING'] } }, null).then((answer) => {
      if (cancelled) return;
      if (answer.kind !== 'outcome') {
        setQueueNote(`盘点单没读成：${describeReadFailure(answer)}`);
        return;
      }
      setQueue(answer.body.counts ?? []);
      setQueueNote(`盘点中的盘点单 ${answer.body.page?.total ?? (answer.body.counts ?? []).length} 张。`);
    });
    return () => {
      cancelled = true;
    };
  }, [input.warehouse]);

  return {
    state,
    queue,
    queueNote,
    scan: (code: string) => act(scanCode(stateRef.current, code)),
    chooseState: (stockState: string) => act(chooseStockState(stateRef.current, stockState)),
    unknown: () => act(markUnknown(stateRef.current)),
    setQuantity: (index: number, quantity: number) => act(setQuantity(stateRef.current, index, quantity)),
    submit: () => act(submitLocation(stateRef.current, false)),
    empty: () => act(submitLocation(stateRef.current, true)),
    resend: () => act(resend(stateRef.current)),
    openCount: (countId: string) => act(scanCode({ ...stateRef.current, phase: 'waiting', sheet: null }, countId)),
  };
}
