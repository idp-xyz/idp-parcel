import { useCallback, useEffect, useRef, useState } from 'react';
import { getOutboundRequestDetail, listOutboundRequests, type OutboundRequestRecord } from '../api';
import { workFactSpecs, type WorkFactSpec } from '../work-fact';
import { browserStorage } from './device-binding';
import {
  confirmOffline,
  confirmParcel,
  handoffVerdict,
  leaveOrder,
  orderFound,
  orderOpened,
  resend,
  scanCode,
  setPickup,
  startDispatch,
  type DispatchEffect,
  type DispatchState,
  type DispatchStep,
} from './dispatch-flow';
import { sheetOf } from './dispatch-sheet';
import { liveOrderFinder } from './pack-order';
import type { ScanCue } from './scan-sound';
import { describeReadFailure, judgeWorkFactAnswer, mintWorkFact, sendWorkFact, type WorkFactAttempt } from './work-fact-submit';

function specOf(id: string): WorkFactSpec {
  const spec = workFactSpecs.find((candidate) => candidate.id === id);
  if (spec === undefined) throw new Error(`作业事实定义里没有 ${id}`);
  return spec;
}

export interface DispatchSessionInput {
  deviceId: string;
  warehouse: string;
  requestId: string | null;
  cue: (cue: ScanCue) => void;
}

export function useDispatchSession(input: DispatchSessionInput) {
  const [state, setState] = useState<DispatchState>(() => startDispatch(input.requestId).state);
  const stateRef = useRef(state);
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const [queue, setQueue] = useState<OutboundRequestRecord[]>([]);
  const [queueNote, setQueueNote] = useState('在读已打包、等交出的出库单。');
  const finder = liveOrderFinder();

  const apply = useCallback((step: DispatchStep) => {
    stateRef.current = step.state;
    setState(step.state);
    if (step.state.cue) input.cue(step.state.cue);
    return step;
  }, [input]);

  const send = useCallback((attempt: WorkFactAttempt) => {
    void sendWorkFact(attempt).then((answer) => {
      apply(handoffVerdict(stateRef.current, judgeWorkFactAnswer(answer)));
    });
  }, [apply]);

  const run = useCallback((effects: DispatchEffect[]) => {
    for (const effect of effects) {
      if (effect.kind === 'find') {
        void finder.find(input.warehouse, effect.code).then((lookup) => {
          const step = orderFound(stateRef.current, lookup);
          apply(step);
          run(step.effects);
        });
      }
      if (effect.kind === 'open') {
        void getOutboundRequestDetail(effect.requestId).then((answer) => {
          if (answer.kind !== 'outcome') {
            apply(orderOpened(stateRef.current, null, input.warehouse));
            stateRef.current = {
              ...stateRef.current,
              feedback: { tone: 'unsent', headline: '这一次没读成', detail: describeReadFailure(answer) },
            };
            setState(stateRef.current);
            return;
          }
          apply(orderOpened(stateRef.current, sheetOf(answer.body), input.warehouse));
        });
      }
      if (effect.kind === 'submit') {
        const minted = mintWorkFact(browserStorage(), {
          spec: specOf(effect.spec),
          deviceId: input.deviceId,
          warehouse: input.warehouse,
          values: effect.values,
        });
        if (!minted.ok) {
          apply(handoffVerdict(stateRef.current, { verdict: 'refused', headline: minted.reason }));
          continue;
        }
        attemptRef.current = minted.attempt;
        send(minted.attempt);
      }
      if (effect.kind === 'resend' && attemptRef.current !== null) send(attemptRef.current);
    }
  }, [apply, finder, input.deviceId, input.warehouse, send]);

  const act = useCallback((step: DispatchStep) => {
    apply(step);
    run(step.effects);
  }, [apply, run]);

  useEffect(() => {
    const started = startDispatch(input.requestId);
    stateRef.current = started.state;
    setState(started.state);
    run(started.effects);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [input.deviceId, input.requestId, input.warehouse]);

  useEffect(() => {
    let cancelled = false;
    void listOutboundRequests({ filters: { status: ['PACKED'], warehouseCode: [input.warehouse] } }).then((answer) => {
      if (cancelled) return;
      if (answer.kind !== 'outcome') {
        setQueueNote(`等交出的出库单没读成：${describeReadFailure(answer)}`);
        return;
      }
      const rows = answer.body.requests ?? [];
      setQueue(rows);
      const more = answer.body.page?.next ? '只列了第一页。' : '';
      setQueueNote(`已打包、等交出的出库单 ${answer.body.page?.total ?? rows.length} 张。${more}`);
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
    confirm: () => act(confirmParcel(stateRef.current)),
    offline: () => act(confirmOffline(stateRef.current)),
    setPickup: (party: string, credential: string) => act(setPickup(stateRef.current, party, credential)),
    resend: () => act(resend(stateRef.current)),
    leave: () => act(leaveOrder(stateRef.current)),
    openRequest: (requestId: string) => act(scanCode({ ...stateRef.current, phase: 'waiting', sheet: null }, requestId)),
  };
}
