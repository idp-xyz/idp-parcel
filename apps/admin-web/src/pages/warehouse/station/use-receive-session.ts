import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ApiResult } from '../../catalogue-api';
import { listCustomerAccounts } from '../../party/api';
import { findGoodsByCode, getPreAdviceByNumber, listOwnerPreAdvices, type PreAdviceRecord } from '../api';
import { workFactSpecs, type WorkFactSpec } from '../work-fact';
import { browserStorage } from './device-binding';
import { liveReceivingVerifier, readReceiveLocation, writeReceiveLocation } from './receive-location';
import {
  acceptStray,
  adviceFound,
  cancelAsk,
  chooseStockState,
  confirmAsk,
  goodsIdentified,
  leaveAdvice,
  locationVerdict,
  receiptVerdict,
  registerStray,
  resend,
  scanCode,
  rememberedStockState,
  startReceive,
  submitReceipt,
  toggleDamage,
  type IdentifiedGoods,
  type FoundAdvice,
  type ReceiveEffect,
  type ReceiveSpecId,
  type ReceiveState,
  type ReceiveStep,
} from './receive-flow';
import { sheetOf, type AdviceSheet } from './receive-sheet';
import type { ScanCue } from './scan-sound';
import {
  describeReadFailure,
  judgeWorkFactAnswer,
  mintWorkFact,
  sendWorkFact,
  type WorkFactAttempt,
} from './work-fact-submit';

// 收货工位的接线：状态机出效果，这里去读、去提交，答复再交回状态机。扫到的码对不对不在这里判。

function specOf(id: ReceiveSpecId): WorkFactSpec {
  const spec = workFactSpecs.find((candidate) => candidate.id === id);
  if (spec === undefined) throw new Error(`作业事实定义里没有 ${id}`);
  return spec;
}

export interface ReceiveQueueItem {
  preAdviceId: string;
  documentNumber: string | null;
  ownerAccount: string;
  status: string;
  inboundMode: string;
  late: string;
}

export interface ReceiveSessionInput {
  deviceId: string;
  warehouse: string;
  /** 地址里带的预报标识；没带是 null。 */
  adviceId: string | null;
  cue: (cue: ScanCue) => void;
}

async function lookupAdvice(code: string): Promise<FoundAdvice> {
  const byNumber = await getPreAdviceByNumber(code);
  if (byNumber.kind !== 'outcome') return { kind: 'unread', reason: `认这张预报没成：${describeReadFailure(byNumber)}` };
  if (byNumber.body.found && byNumber.body.preAdvice) {
    const made = sheetOf(byNumber.body.preAdvice, new Date());
    return 'preAdviceId' in made ? { kind: 'sheet', sheet: made } : made;
  }
  return { kind: 'absent' };
}

async function identifyGoods(owner: string, code: string): Promise<IdentifiedGoods> {
  const answer = await findGoodsByCode(owner, code);
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: `读商品资料没成：${describeReadFailure(answer)}` };
  const goods = answer.body.goods;
  if (!answer.body.found || goods === undefined) return { kind: 'unknown' };
  return { kind: 'found', goodsCode: goods.goodsCode, unit: goods.baseUnit, trackBatch: goods.trackBatch, trackExpiry: goods.trackExpiry };
}

export function useReceiveSession(input: ReceiveSessionInput) {
  const remembered = readReceiveLocation(browserStorage(), input.deviceId);
  const [state, setState] = useState<ReceiveState>(() => startReceive(remembered, input.adviceId).state);
  const stateRef = useRef(state);
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const specRef = useRef<ReceiveSpecId>('receipt-goods');
  const identifiedRef = useRef<IdentifiedGoods>({ kind: 'unknown' });
  const [queue, setQueue] = useState<ReceiveQueueItem[]>([]);
  const [queueNote, setQueueNote] = useState('在读还没收完的预报。');
  const verifier = useMemo(() => liveReceivingVerifier(), []);

  const apply = useCallback((step: ReceiveStep) => {
    stateRef.current = step.state;
    setState(step.state);
    if (step.state.cue) input.cue(step.state.cue);
    return step;
  }, [input]);

  const send = useCallback((attempt: WorkFactAttempt, spec: ReceiveSpecId) => {
    void sendWorkFact(attempt).then((answer) => {
      apply(receiptVerdict(stateRef.current, judgeWorkFactAnswer(answer), spec));
    });
  }, [apply]);

  const run = useCallback((effects: ReceiveEffect[]) => {
    for (const effect of effects) {
      if (effect.kind === 'remember-location') writeReceiveLocation(browserStorage(), input.deviceId, effect.code);
      if (effect.kind === 'verify-location') {
        void verifier.verify(input.warehouse, effect.code).then((verdict) => {
          const step = locationVerdict(stateRef.current, effect.code, verdict);
          apply(step);
          run(step.effects);
        });
      }
      if (effect.kind === 'find') {
        void lookupAdvice(effect.code).then((found) => {
          apply(adviceFound(stateRef.current, found, input.warehouse));
        });
      }
      if (effect.kind === 'identify') {
        void identifyGoods(effect.owner, effect.code).then((identified) => {
          identifiedRef.current = identified;
          apply(goodsIdentified(stateRef.current, effect.code, identified));
        });
      }
      if (effect.kind === 'submit') {
        specRef.current = effect.spec;
        const minted = mintWorkFact(browserStorage(), {
          spec: specOf(effect.spec),
          deviceId: input.deviceId,
          warehouse: input.warehouse,
          values: effect.values,
        });
        if (!minted.ok) {
          apply(receiptVerdict(stateRef.current, { verdict: 'refused', headline: minted.reason }, effect.spec));
          continue;
        }
        attemptRef.current = minted.attempt;
        send(minted.attempt, effect.spec);
      }
      if (effect.kind === 'resend' && attemptRef.current !== null) send(attemptRef.current, specRef.current);
    }
  }, [apply, input.deviceId, input.warehouse, send, verifier]);

  const act = useCallback((step: ReceiveStep) => {
    apply(step);
    run(step.effects);
  }, [apply, run]);

  useEffect(() => {
    const started = startReceive(remembered, input.adviceId, storedStockState(input.warehouse));
    stateRef.current = started.state;
    setState(started.state);
    run(started.effects);
    // 设备或地址里的预报一换就是新的一趟
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [input.deviceId, input.adviceId, input.warehouse]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const loaded = await loadOwnerAccounts();
      if (cancelled) return;
      if (loaded.kind === 'failed') {
        setQueueNote(`还没收完的预报没读成：${describeReadFailure(loaded.failure)}`);
        return;
      }
      const accounts = loaded.accounts;
      const ownersCut = loaded.truncated;
      const items: ReceiveQueueItem[] = [];
      let adviceCut = false;
      for (const account of accounts) {
        for (const status of ['ADVISED', 'RECEIVING'] as const) {
          let after: string | null = null;
          let pages = 0;
          do {
            const listed = await listOwnerPreAdvices(account.accountId, '', after, 200, status, input.warehouse);
            if (cancelled) return;
            if (listed.kind !== 'outcome') {
              setQueueNote(`货主 ${account.accountId} 的预报没读成：${describeReadFailure(listed)}`);
              after = null;
              break;
            }
            for (const row of listed.body.preAdvices ?? []) {
              const made = sheetOf(row, new Date());
              if (!('preAdviceId' in made)) continue;
              items.push(queueItem(made));
            }
            after = listed.body.page?.next ?? null;
            pages += 1;
          } while (after !== null && pages < maxStationPages);
          if (after !== null) adviceCut = true;
        }
      }
      if (cancelled) return;
      setQueue(items);
      const late = items.filter((item) => item.late !== '').length;
      const tail = [ownersCut ? '货主名册没翻完。' : '', adviceCut ? '有的货主预报没翻完。' : ''].filter((part) => part !== '').join('');
      setQueueNote(`还没收完的预报 ${items.length} 张${late > 0 ? `，其中已迟 ${late} 张` : ''}。${tail}`);
    })();
    return () => {
      cancelled = true;
    };
  }, [input.warehouse]);

  return {
    state,
    queue,
    queueNote,
    scan: (code: string) => act(scanCode(stateRef.current, code)),
    chooseState: (stockState: string) => {
      if (rememberedStockState(stockState) !== null) writeStockState(input.warehouse, stockState);
      act(chooseStockState(stateRef.current, stockState));
    },
    confirmAsk: (batch: string, expiry: string, unit: string) => act(confirmAsk(stateRef.current, batch, expiry, unit)),
    cancelAsk: () => act(cancelAsk(stateRef.current)),
    acceptStray: () => act(acceptStray(stateRef.current, identifiedRef.current)),
    registerStray: () => act(registerStray(stateRef.current)),
    toggleDamage: () => act(toggleDamage(stateRef.current)),
    submit: () => act(submitReceipt(stateRef.current)),
    resend: () => act(resend(stateRef.current)),
    leave: () => act(leaveAdvice(stateRef.current)),
    openAdvice: (id: string) => act(scanCode({ ...stateRef.current, phase: 'waiting', advice: null }, id)),
  };
}

const maxStationPages = 5;

function stockStateKey(warehouse: string): string {
  return `idp-parcel.station.receive.stock-state.${warehouse}`;
}

function storedStockState(warehouse: string): string | null {
  try {
    return rememberedStockState(sessionStorage.getItem(stockStateKey(warehouse)));
  } catch {
    return null;
  }
}

function writeStockState(warehouse: string, stockState: string) {
  try {
    sessionStorage.setItem(stockStateKey(warehouse), stockState);
  } catch {
    // 这一屏记不住，下回再选。
  }
}

async function loadOwnerAccounts(): Promise<
  { kind: 'ok'; accounts: { accountId: string }[]; truncated: boolean } | { kind: 'failed'; failure: Exclude<ApiResult<unknown>, { kind: 'outcome' }> }
> {
  const accounts: { accountId: string }[] = [];
  let after: string | null = null;
  let pages = 0;
  let next: string | null = null;
  do {
    const answer = await listCustomerAccounts({ pageSize: 200 }, after);
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    for (const account of answer.body.accounts ?? []) accounts.push({ accountId: account.accountId });
    next = answer.body.page?.next ?? null;
    after = next;
    pages += 1;
  } while (next !== null && pages < maxStationPages);
  return { kind: 'ok', accounts, truncated: next !== null };
}

function queueItem(sheet: AdviceSheet): ReceiveQueueItem {
  return {
    preAdviceId: sheet.preAdviceId,
    documentNumber: sheet.documentNumber,
    ownerAccount: sheet.ownerAccount,
    status: sheet.status,
    inboundMode: sheet.inboundMode,
    late: sheet.late,
  };
}

export type { PreAdviceRecord };
