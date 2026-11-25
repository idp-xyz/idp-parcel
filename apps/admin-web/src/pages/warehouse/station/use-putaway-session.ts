import { useCallback, useEffect, useRef, useState } from 'react';
import { listCustomerAccounts } from '../../party/api';
import { queryOwnerStock } from '../api';
import { workFactSpecs } from '../work-fact';
import { browserStorage } from './device-binding';
import { livePlaceVerifier } from './putaway-place';
import {
  chooseOwner,
  moveVerdict,
  placeVerdict,
  resend,
  scanCode,
  setQuantity,
  startPutaway,
  stockLoaded,
  submitMove,
  type PutawayEffect,
  type PutawayState,
  type PutawayStep,
  type StockCarton,
  type StockCell,
} from './putaway-flow';
import type { ScanCue } from './scan-sound';
import { describeReadFailure, judgeWorkFactAnswer, mintWorkFact, sendWorkFact, type WorkFactAttempt } from './work-fact-submit';

const moveLabels = {
  REGISTERED: '这一笔已经移过去了',
  SHORTAGE_REGISTERED: '够的部分已移走，不够的记成了待批准的作业差异',
};

export function usePutawaySession(input: { deviceId: string; warehouse: string; cue: (cue: ScanCue) => void }) {
  const [state, setState] = useState<PutawayState>(() => startPutaway().state);
  const stateRef = useRef(state);
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const [owners, setOwners] = useState<string[]>([]);
  const [ownerNote, setOwnerNote] = useState('在读货主。');
  const verifier = livePlaceVerifier();

  const apply = useCallback((step: PutawayStep) => {
    stateRef.current = step.state;
    setState(step.state);
    if (step.state.cue) input.cue(step.state.cue);
  }, [input]);

  const run = useCallback((effects: PutawayEffect[]) => {
    for (const effect of effects) {
      if (effect.kind === 'verify') {
        void verifier.verify(input.warehouse, effect.code, effect.role).then((verdict) => {
          const step = placeVerdict(stateRef.current, effect.code, verdict);
          apply(step);
          run(step.effects);
        });
      }
      if (effect.kind === 'load') {
        void Promise.all([
          queryOwnerStock(effect.owner, input.warehouse, 'cells'),
          queryOwnerStock(effect.owner, input.warehouse, 'cartons'),
        ]).then(([cellsAnswer, cartonsAnswer]) => {
          if (cellsAnswer.kind !== 'outcome') {
            apply(stockLoaded(stateRef.current, [], [], describeReadFailure(cellsAnswer)));
            return;
          }
          const cells: StockCell[] = (cellsAnswer.body.cells ?? [])
            .filter((item) => item.location === effect.source && item.quantity > 0)
            .map((item) => ({
              goodsCode: item.goodsCode,
              unit: item.unit,
              stockState: item.stockState,
              quantity: item.quantity,
              batchNumber: item.batchNumber,
              expiryDate: item.expiryDate,
              inboundBatchId: item.inboundBatchId,
            }));
          const cartons: StockCarton[] = cartonsAnswer.kind === 'outcome'
            ? (cartonsAnswer.body.cartons ?? [])
              .filter((item) => item.location === effect.source)
              .map((item) => ({ cartonNumber: item.cartonNumber, summary: (item.bill ?? []).map((line) => line.goodsCode).join('，') }))
            : [];
          apply(stockLoaded(stateRef.current, cells, cartons));
        });
      }
      if (effect.kind === 'submit') {
        const spec = workFactSpecs.find((item) => item.id === 'internal-move');
        if (!spec) return;
        const minted = mintWorkFact(browserStorage(), { spec, deviceId: input.deviceId, warehouse: input.warehouse, values: effect.values });
        if (!minted.ok) {
          apply(moveVerdict(stateRef.current, { verdict: 'refused', headline: minted.reason }));
          continue;
        }
        attemptRef.current = minted.attempt;
        void sendWorkFact(minted.attempt).then((answer) => {
          apply(moveVerdict(stateRef.current, judgeWorkFactAnswer(answer, { ...moveLabels })));
        });
      }
      if (effect.kind === 'resend' && attemptRef.current) {
        void sendWorkFact(attemptRef.current).then((answer) => {
          apply(moveVerdict(stateRef.current, judgeWorkFactAnswer(answer, { ...moveLabels })));
        });
      }
    }
  }, [apply, input.deviceId, input.warehouse, verifier]);

  const act = useCallback((step: PutawayStep) => {
    apply(step);
    run(step.effects);
  }, [apply, run]);

  useEffect(() => {
    let cancelled = false;
    void listCustomerAccounts().then((answer) => {
      if (cancelled) return;
      if (answer.kind !== 'outcome') {
        setOwnerNote(`货主没读成：${describeReadFailure(answer)}`);
        return;
      }
      setOwners((answer.body.accounts ?? []).map((account) => account.accountId));
      setOwnerNote(answer.body.page?.next ? '货主只列了第一页。' : '');
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    state,
    owners,
    ownerNote,
    chooseOwner: (owner: string) => act(chooseOwner(stateRef.current, owner)),
    scan: (code: string) => act(scanCode(stateRef.current, code)),
    setQuantity: (quantity: number) => act(setQuantity(stateRef.current, quantity)),
    submit: () => act(submitMove(stateRef.current)),
    resend: () => act(resend(stateRef.current)),
  };
}
