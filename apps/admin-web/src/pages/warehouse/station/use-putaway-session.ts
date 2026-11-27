import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiResult } from '../../catalogue-api';
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
        void loadLocationStock(effect.owner, input.warehouse, effect.source).then((loaded) => {
          if (loaded.kind === 'failed') {
            apply(stockLoaded(stateRef.current, [], [], describeReadFailure(loaded.failure)));
            return;
          }
          apply(stockLoaded(stateRef.current, loaded.cells, loaded.cartons, undefined, loaded.missed));
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
    void (async () => {
      const loaded = await loadPutawayOwners();
      if (cancelled) return;
      if (loaded.kind === 'failed') {
        setOwnerNote(`货主没读成：${describeReadFailure(loaded.failure)}`);
        return;
      }
      setOwners(loaded.owners);
      setOwnerNote(loaded.truncated ? '货主名册没翻完。' : '');
      const saved = storedPutawayOwner(input.warehouse);
      if (saved !== null && loaded.owners.includes(saved)) act(chooseOwner(stateRef.current, saved));
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    state,
    owners,
    ownerNote,
    chooseOwner: (owner: string) => {
      writePutawayOwner(input.warehouse, owner);
      act(chooseOwner(stateRef.current, owner));
    },
    scan: (code: string) => act(scanCode(stateRef.current, code)),
    setQuantity: (quantity: number) => act(setQuantity(stateRef.current, quantity)),
    submit: () => act(submitMove(stateRef.current)),
    resend: () => act(resend(stateRef.current)),
  };
}

const maxStationPages = 5;
const stockPageSize = 200;

function putawayOwnerKey(warehouse: string): string {
  return `idp-parcel.station.putaway.owner.${warehouse}`;
}

function storedPutawayOwner(warehouse: string): string | null {
  try {
    const value = sessionStorage.getItem(putawayOwnerKey(warehouse));
    return value !== null && value !== '' ? value : null;
  } catch {
    return null;
  }
}

function writePutawayOwner(warehouse: string, owner: string) {
  try {
    sessionStorage.setItem(putawayOwnerKey(warehouse), owner);
  } catch {
    // 这一屏记不住，下回再选。
  }
}

async function loadPutawayOwners(): Promise<
  { kind: 'ok'; owners: string[]; truncated: boolean } | { kind: 'failed'; failure: Exclude<ApiResult<unknown>, { kind: 'outcome' }> }
> {
  const owners: string[] = [];
  let after: string | null = null;
  let pages = 0;
  let next: string | null = null;
  do {
    const answer = await listCustomerAccounts({ pageSize: stockPageSize }, after);
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    for (const account of answer.body.accounts ?? []) owners.push(account.accountId);
    next = answer.body.page?.next ?? null;
    after = next;
    pages += 1;
  } while (next !== null && pages < maxStationPages);
  return { kind: 'ok', owners, truncated: next !== null };
}

async function loadLocationStock(
  owner: string,
  warehouse: string,
  location: string,
): Promise<
  { kind: 'ok'; cells: StockCell[]; cartons: StockCarton[]; missed: boolean } | { kind: 'failed'; failure: Exclude<ApiResult<unknown>, { kind: 'outcome' }> }
> {
  const cells: StockCell[] = [];
  const cartons: StockCarton[] = [];
  let missed = false;
  for (const view of ['cells', 'cartons'] as const) {
    let page = 1;
    let seen = 0;
    let total: number | null = null;
    while (page <= maxStationPages) {
      const answer = await queryOwnerStock(owner, warehouse, view, null, stockPageSize, page);
      if (answer.kind !== 'outcome') {
        if (view === 'cells') return { kind: 'failed', failure: answer };
        break;
      }
      total = answer.body.book?.total ?? null;
      if (view === 'cells') {
        const rows = answer.body.cells ?? [];
        seen += rows.length;
        for (const item of rows) {
          if (item.location === location && item.quantity > 0) {
            cells.push({
              goodsCode: item.goodsCode,
              unit: item.unit,
              stockState: item.stockState,
              quantity: item.quantity,
              batchNumber: item.batchNumber,
              expiryDate: item.expiryDate,
              inboundBatchId: item.inboundBatchId,
            });
          }
        }
        if (rows.length === 0 || (total !== null && seen >= total)) break;
      } else {
        const rows = answer.body.cartons ?? [];
        seen += rows.length;
        for (const item of rows) {
          if (item.location === location) {
            cartons.push({ cartonNumber: item.cartonNumber, summary: (item.bill ?? []).map((line) => line.goodsCode).join('，') });
          }
        }
        if (rows.length === 0 || (total !== null && seen >= total)) break;
      }
      page += 1;
    }
    if (total !== null && seen < total) missed = true;
  }
  return { kind: 'ok', cells, cartons, missed };
}
