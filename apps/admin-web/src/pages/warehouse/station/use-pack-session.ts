import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { findGoodsByCode, getOutboundRequestDetail, listOutboundRequests, listTotes, matchSingleUnitOrder } from '../api';
import { workFactSpecs, type WorkFactSpec } from '../work-fact';
import type { GuidedFormValues } from '../guided-forms';
import { browserStorage } from './device-binding';
import {
  addParcel,
  applyVerdict,
  backToForm,
  changeCounts,
  checkOutcomeLabels,
  chooseMaterial,
  dismissWrong,
  editParcel,
  goodsIdentified,
  leaveOrder,
  rematchAfterRefusal,
  openOrder,
  orderFound,
  orderOpened,
  packagingIdentified,
  packOutcomeLabels,
  referenceGoods,
  removeParcel,
  resend,
  scanCode,
  startPack,
  submitMismatch,
  submitPack,
  type PackEffect,
  type PackState,
  type PackStep,
  type ParcelPatch,
} from './pack-flow';
import { liveOrderFinder, loadPackQueue, openedOf, type OrderFinder, type PackQueue } from './pack-order';
import { liveLoadPackagingList, livePackagingFinder, type PackagingFinder, type PackagingList } from './pack-packaging';
import { checkFactValues, packFactValues, type PackagingMaterial } from './pack-parcels';
import { scanAfterMatch, withSingleUnitMatch, withToteLookup } from './pack-tote';
import { createGoodsIdentifier, goodsLookupOf, type GoodsIdentifier } from './pick-goods';
import type { ScanCue } from './scan-sound';
import {
  describeReadFailure,
  judgeWorkFactAnswer,
  mintWorkFact,
  sendWorkFact,
  type WorkFactAttempt,
  type WorkFactVerdict,
} from './work-fact-submit';

// 复核打包工位的接线：把状态机（pack-flow.ts）接上读口、提交与本机存储。这里不判断扫到的码对不对——每一步都是「调状态机的纯函数
// 拿到新状态与要做的事，把新状态放上屏，再去做那些事」；做完事得到的答复再交回状态机。
//
// 状态同时放在 ref 与 state 里：事件处理函数读 ref 拿到的永远是最新的一份（枪一秒能连着来两次），state 只负责触发重渲。
// 两个写口（复核、打包）的提交都经 work-fact-submit：事实号在铸这一笔时定下一次，之后不管送几次都是它。

function specOf(id: string): WorkFactSpec {
  const spec = workFactSpecs.find((candidate) => candidate.id === id);
  if (spec === undefined) throw new Error(`作业事实定义里没有 ${id}`);
  return spec;
}

const checkSpec = specOf('outbound-check');
const packSpec = specOf('pack');

export type PackagingChoices =
  | { phase: 'idle' }
  | { phase: 'loading' }
  | { phase: 'ready'; list: PackagingList }
  | { phase: 'error'; message: string };

export interface PackSessionInput {
  deviceId: string;
  /** 设备绑的仓。 */
  warehouse: string;
  /** 地址里带的出库请求号；没带是 null。 */
  request: string | null;
  cue: (cue: ScanCue) => void;
}

export interface PackSession {
  state: PackState;
  /** 刚收到的后端答复；没送到与没成时屏上要配它的技术详情与重新登录。 */
  verdict: WorkFactVerdict | null;
  /** 本仓等复核与等打包的单；第一次读出来之前是 null。 */
  queue: PackQueue | null;
  queueLoading: boolean;
  /** 队列没读成的原因；队列还是读成过的那一份（没有就是 null），屏上要说清它可能不是最新。 */
  queueError: string | null;
  /** 点选包材用的列表，开选包材的框时才读。 */
  packagingChoices: PackagingChoices;
  scan: (raw: string) => void;
  open: (requestId: string) => void;
  changeCounts: (texts: Readonly<Record<string, string>>) => { ok: true } | { ok: false; reason: string };
  confirmMismatch: () => void;
  addParcel: () => void;
  removeParcel: (key: number) => void;
  editParcel: (key: number, patch: ParcelPatch) => void;
  chooseMaterial: (key: number, material: PackagingMaterial) => void;
  submitPack: () => void;
  backToForm: () => void;
  leave: () => void;
  resend: () => void;
  dismiss: () => void;
  refreshQueue: () => void;
  loadPackagingChoices: () => void;
}

export function usePackSession({ deviceId, warehouse, request, cue }: PackSessionInput): PackSession {
  // 开工的那一步只算一次：地址里带了请求号就直接去读那张单，要做的事在挂上之后执行
  const startRef = useRef<PackStep | null>(null);
  if (startRef.current === null) startRef.current = startPack(warehouse, request);
  const [state, setState] = useState<PackState>(startRef.current.state);
  const [verdict, setVerdict] = useState<WorkFactVerdict | null>(null);
  const [queue, setQueue] = useState<PackQueue | null>(null);
  const [queueLoading, setQueueLoading] = useState(true);
  const [queueError, setQueueError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const [packagingChoices, setPackagingChoices] = useState<PackagingChoices>({ phase: 'idle' });

  const stateRef = useRef<PackState>(startRef.current.state);
  // 没送到时「再送一次」用的就是它：事实号在铸这一笔时定下，之后送几次都是它；说法表跟着这一笔走（复核与打包同一个词意思不同）
  const attemptRef = useRef<WorkFactAttempt | null>(null);
  const labelsRef = useRef<Readonly<Record<string, string>>>(checkOutcomeLabels);
  const floorRef = useRef(0);
  const cueRef = useRef(cue);
  cueRef.current = cue;
  const startedRef = useRef(false);
  const choicesRef = useRef(0);

  // 认商品码要货主：取自读到的这张单，不再另读
  const ownersRef = useRef(new Map<string, string>());
  const identifierRef = useRef<GoodsIdentifier | null>(null);
  if (identifierRef.current === null) {
    identifierRef.current = createGoodsIdentifier({
      ownerOf: async (requestId) => {
        const owner = ownersRef.current.get(requestId);
        return owner === undefined ? { kind: 'missing' } : { kind: 'owner', ownerAccount: owner };
      },
      findGoods: async (ownerAccount, code) => goodsLookupOf(await findGoodsByCode(ownerAccount, code)),
    });
  }
  const finderRef = useRef<OrderFinder | null>(null);
  // 扫到的码当出库单号找不到时，再当周转箱号找（货是装在箱里送到复核台的）；还找不到，当一件货给单品单件的出库单配单
  if (finderRef.current === null) {
    finderRef.current = withSingleUnitMatch(withToteLookup(liveOrderFinder(), listTotes), matchSingleUnitOrder);
  }
  // 按货配出来的那张单与扫的那个码：单读开之后把这一件算进复核的数里，人不必再扫一次
  const matchedRef = useRef<{ requestId: string; code: string } | null>(null);
  // 同一份，但单读开之后还留着：这张单复核时被别的工位抢了先，要用这个码另配一张。下一次找单时换掉
  const byGoodsRef = useRef<{ requestId: string; code: string } | null>(null);
  const packagingRef = useRef<PackagingFinder | null>(null);
  if (packagingRef.current === null) packagingRef.current = livePackagingFinder();

  const applyRef = useRef<(step: PackStep) => void>(() => undefined);

  const finish = useCallback((answered: WorkFactVerdict) => {
    setVerdict(answered);
    applyRef.current(applyVerdict(stateRef.current, answered));
    // 按货配的单被别的工位先复核了：用同一个码另配一张，人不必再扫
    const again = rematchAfterRefusal(stateRef.current, answered, byGoodsRef.current);
    if (again !== null) applyRef.current(again);
  }, []);

  const send = useCallback(
    (attempt: WorkFactAttempt) => {
      void sendWorkFact(attempt).then((answer) => finish(judgeWorkFactAnswer(answer, labelsRef.current)));
    },
    [finish],
  );

  const mint = useCallback(
    (spec: WorkFactSpec, values: GuidedFormValues, labels: Readonly<Record<string, string>>) => {
      const minted = mintWorkFact(browserStorage(), { spec, deviceId, warehouse, values }, floorRef.current);
      if (!minted.ok) {
        finish({ verdict: 'refused', headline: `这一笔装不出来：${minted.reason}。` });
        return;
      }
      attemptRef.current = minted.attempt;
      labelsRef.current = labels;
      floorRef.current = minted.attempt.sequence;
      send(minted.attempt);
    },
    [deviceId, warehouse, finish, send],
  );

  const runEffect = useCallback(
    (effect: PackEffect) => {
      switch (effect.kind) {
        case 'find-order':
          void finderRef.current?.find(warehouse, effect.code).then((lookup) => {
            matchedRef.current = lookup.kind === 'found' && lookup.byGoods ? { requestId: lookup.row.requestId, code: effect.code } : null;
            byGoodsRef.current = matchedRef.current;
            applyRef.current(orderFound(stateRef.current, effect.code, lookup));
          });
          return;
        case 'open-order':
          void getOutboundRequestDetail(effect.requestId).then((answer) => {
            const opened = openedOf(answer);
            if (opened.kind === 'order') ownersRef.current.set(opened.order.requestId, opened.order.ownerAccount);
            applyRef.current(orderOpened(stateRef.current, effect.requestId, opened));
            const again = scanAfterMatch(matchedRef.current, effect.requestId, stateRef.current.phase);
            matchedRef.current = null;
            if (again !== null) applyRef.current(scanCode(stateRef.current, again));
          });
          return;
        case 'identify':
          ownersRef.current.set(effect.order.requestId, effect.order.ownerAccount);
          void identifierRef.current
            ?.identify({ requestId: effect.order.requestId, goodsCode: referenceGoods(effect.order).goodsCode }, effect.code)
            .then((result) => applyRef.current(goodsIdentified(stateRef.current, effect.code, result)));
          return;
        case 'identify-packaging':
          void packagingRef.current?.find(effect.code).then((lookup) => applyRef.current(packagingIdentified(stateRef.current, effect.code, lookup)));
          return;
        case 'submit-check':
          mint(checkSpec, checkFactValues(effect.order, effect.observed), checkOutcomeLabels);
          return;
        case 'submit-pack':
          mint(packSpec, packFactValues(effect.order, effect.parcels), packOutcomeLabels);
          return;
        case 'resend':
          if (attemptRef.current !== null) send(attemptRef.current);
          return;
        case 'refresh-queue':
          setTick((value) => value + 1);
          return;
      }
    },
    [warehouse, mint, send],
  );

  const apply = useCallback(
    (step: PackStep) => {
      stateRef.current = step.state;
      setState(step.state);
      if (step.cue !== undefined) cueRef.current(step.cue);
      for (const effect of step.effects) runEffect(effect);
    },
    [runEffect],
  );
  applyRef.current = apply;

  // 地址里带了请求号时，开工要做的事（读那张单）在挂上之后做一次。严格模式会把挂上的 effect 跑两遍，ref 拦住第二遍。
  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    for (const effect of startRef.current?.effects ?? []) runEffect(effect);
  }, [runEffect]);

  // 读队列：挂上时读一次，之后每逢做完、记下了、换了单（refresh-queue）或点刷新再读。读不成时留着读成过的那一份。
  useEffect(() => {
    let cancelled = false;
    setQueueLoading(true);
    void loadPackQueue({ warehouse, read: listOutboundRequests }).then((answer) => {
      if (cancelled) return;
      setQueueLoading(false);
      if (answer.kind === 'failed') {
        setQueueError(describeReadFailure(answer.failure));
        return;
      }
      setQueue(answer.queue);
      setQueueError(null);
    });
    return () => {
      cancelled = true;
    };
  }, [warehouse, tick]);

  const scan = useCallback((raw: string) => apply(scanCode(stateRef.current, raw)), [apply]);
  const open = useCallback((requestId: string) => apply(openOrder(stateRef.current, requestId)), [apply]);

  const changeCountsNow = useCallback(
    (texts: Readonly<Record<string, string>>) => {
      const result = changeCounts(stateRef.current, texts);
      if (!result.ok) return result;
      apply(result.step);
      return { ok: true as const };
    },
    [apply],
  );

  const removeParcelNow = useCallback((key: number) => apply(removeParcel(stateRef.current, key)), [apply]);
  const editParcelNow = useCallback((key: number, patch: ParcelPatch) => apply(editParcel(stateRef.current, key, patch)), [apply]);
  const chooseMaterialNow = useCallback((key: number, material: PackagingMaterial) => apply(chooseMaterial(stateRef.current, key, material)), [apply]);

  // 其余几个动作都是「拿最新的状态调一个纯函数、把结果放上屏」。
  const act = useCallback((action: (current: PackState) => PackStep) => () => apply(action(stateRef.current)), [apply]);
  const confirmMismatch = useMemo(() => act(submitMismatch), [act]);
  const addParcelNow = useMemo(() => act(addParcel), [act]);
  const submitPackNow = useMemo(() => act(submitPack), [act]);
  const backToFormNow = useMemo(() => act(backToForm), [act]);
  const leave = useMemo(() => act(leaveOrder), [act]);
  const resendNow = useMemo(() => act(resend), [act]);
  const dismiss = useMemo(() => act(dismissWrong), [act]);
  const refreshQueue = useCallback(() => setTick((value) => value + 1), []);

  // 点选包材的列表：开框时读，每次开都重读一遍，免得拿着久远的名册。后到的答复只认最近这一次。
  const loadPackagingChoices = useCallback(() => {
    choicesRef.current += 1;
    const mine = choicesRef.current;
    setPackagingChoices({ phase: 'loading' });
    void liveLoadPackagingList().then((answer) => {
      if (mine !== choicesRef.current) return;
      setPackagingChoices(answer.kind === 'list' ? { phase: 'ready', list: answer.list } : { phase: 'error', message: answer.reason });
    });
  }, []);

  return {
    state,
    verdict,
    queue,
    queueLoading,
    queueError,
    packagingChoices,
    scan,
    open,
    changeCounts: changeCountsNow,
    confirmMismatch,
    addParcel: addParcelNow,
    removeParcel: removeParcelNow,
    editParcel: editParcelNow,
    chooseMaterial: chooseMaterialNow,
    submitPack: submitPackNow,
    backToForm: backToFormNow,
    leave,
    resend: resendNow,
    dismiss,
    refreshQueue,
    loadPackagingChoices,
  };
}
