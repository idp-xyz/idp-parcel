// 复核打包工位的会话状态机（纯 TypeScript，node:test 直接测）：现在等扫什么、扫到一个码之后变成什么、该提交什么。
// 判断一律在这里；组件只管摆与接线，不写「这个码对不对」。
//
// 一张出库单在这个工位上走两步：复核（逐件扫，扫齐自动提交）→ 打包（选包材、录重量、分包裹，一笔提交整张单）。已复核的单直接进打包。
//
// 到 Go 端核过的规则（application.WaveHandler.Check 与 Pack；载荷由 registrationjson.CheckCommand 与 PackCommand 译；
// 口在 warehousehttp.NewRecordCheckEndpoint 与 NewRecordPackEndpoint；打包之后的委托提交在装配里的 transactionalWave.Pack）。
// 屏上的话照这些说，不照猜的说：
//   - 复核只收已拣货（PICKED）的单；实点总数要等于所有拣货任务已拣数量的合计、且大于 0，才答 CHECKED 并让这张单进入已复核
//     （CHECKED）。数量对不上（多了、少了、一件没点到）答 CHECK_MISMATCH：这一次复核**照样记下**（outbound_check 落一行，
//     matched 为假），出库单的状态不动，仍是已拣货，所以不能打包；之后可以用新的作业事实号再复核。同一个事实号同样数量重放，
//     对上过的答已有结果（EXISTING_RESULT），没对上的再答一次 CHECK_MISMATCH；同一个事实号不同数量答内容冲突。
//   - 复核的别的答复：单找不到 WAVE_NOT_FOUND；状态不是已拣货、或还有任务没拣完 STATUS_MISMATCH；仓与设备绑的仓对不上
//     CONTENT_INVALID；这个租户一个仓都没登记 NOT_CONFIGURED。
//   - 打包只收已复核的单。一笔提交整张单：parcels 数组一次说完，一项一个包裹、只带一种货，序号不重复，件数合计等于已拣合计。
//     成了答 PACKED；本产品网络的单随后在另一笔事务里提交委托，打包的答复被委托的答复换掉（commission.go 的 AfterCommission）：
//     SUBMITTED（已提交）、EXISTING_RESULT（已有），或者没提交成的 OWNERSHIP_UNRESOLVED（生产归属未定）、ADMISSION_PAUSED（准入暂停）、
//     OTHER_PRODUCTION_AUTHORITY（别的权威承接）、INPUT_NOT_ACCEPTED——这四个词只有提交委托那一步会答，所以答到它们就是
//     **打包已经记下、只是委托没提交成**（装配里的注释：「打包已经落定；这一笔没提交成，重放打包口会接着提交」），要给「再送一次」，
//     用同一个事实号。换了新的事实号再打包，单已经是已打包，只会答状态不符，所以这一笔的 attempt 丢了就接不上了。
//   - 有四个词打包口自己也会答，换成委托的答复后又分不出是哪一步答的：NOT_CONFIGURED（没登记仓，或代办授予没登记）、
//     CONTENT_CONFLICT（事实号撞了不同内容，或同一来源已有不同内容的委托）、STATUS_MISMATCH 与 WAVE_NOT_FOUND（单不是已复核了，
//     或打包记下后单被取消）。屏上对它们只说答复能证明的话，有没有记下让人重新扫这张单看状态；NOT_CONFIGURED 给「再送一次」，
//     因为重送对两种情形都无害。
//     打包自己没记成的答复：WAVE_NOT_FOUND、STATUS_MISMATCH、CONTENT_INVALID（合计对不上、某一格空、仓对不上）。
//     同一个事实号重放答已有结果，不比内容。
//
// 屏上文字与扫码顺序照屏幕示意里复核打包那一项：扫出库单号 → 逐件扫单里的货 → 扫齐自动提交复核（数量相符才进打包）→
// 选包材（扫包材码或点选）、录重量，尺寸先取包材的外廓 → 提交打包，一张单装成几个包裹时点「再加一个包裹」。

import type { PackagingLookup } from './pack-packaging';
import {
  goodsKey,
  judgeOpened,
  judgeOrder,
  waitingCounter,
  type Goto,
  type OrderLookup,
  type OrderOpened,
  type PackGoods,
  type PackOrder,
  type PackQueue,
} from './pack-order';
import {
  emptyParcel,
  firstUnpacked,
  judgePack,
  packBalance,
  withMaterial,
  type PackagingMaterial,
  type ParcelDraft,
  type ParcelField,
} from './pack-parcels';
import type { GoodsIdentification } from './pick-goods';
import { normalizeScan } from './scan-input';
import type { ScanCue } from './scan-sound';
import type { WorkFactVerdict } from './work-fact-submit';

export type PackPhase =
  /** 等扫出库单号。 */
  | 'waiting'
  /** 正在认扫到的这个码是哪张单（读口在途）。 */
  | 'finding'
  /** 正在读这张单的明细（读口在途）。 */
  | 'opening'
  /** 复核：逐件扫单里的货。 */
  | 'checking'
  /** 正在认一个商品码（读口在途）。 */
  | 'resolving'
  /** 复核正在提交，不收扫码。 */
  | 'check-submitting'
  /** 复核没送到：等「再送一次」。 */
  | 'check-unsent'
  /** 复核业务上没记成：说明原因，等换一张单。 */
  | 'check-refused'
  /** 复核记下了，数量对不上：说明这张单停在哪，等换一张单。 */
  | 'mismatch'
  /** 打包：选包材、录重量、分包裹。 */
  | 'packing'
  /** 正在核一个包材码（读口在途）。 */
  | 'identifying'
  /** 打包正在提交，不收扫码。 */
  | 'pack-submitting'
  /** 打包没送到：等「再送一次」。 */
  | 'pack-unsent'
  /** 打包已记下，但提交委托没成：等「再送一次」接着提交，或换一张单。 */
  | 'pack-pending'
  /** 打包业务上没记成：说明原因，回去改，或换一张单。 */
  | 'pack-refused';

export interface PackFeedback {
  /** 等扫 / 扫对 / 扫错三种样子；unsent 是结果还不确定（没送到、没核成、委托没提交）。 */
  kind: 'waiting' | 'ok' | 'wrong' | 'unsent';
  text: string;
  /** 刚扫到的码。 */
  scanned?: string;
  /** 说完了该去哪：屏上给一个链接。 */
  goto?: Goto;
  /** 等扫出库单号时扫错了，最大的字写它错在哪（没有这张单、在别的仓）；不给就用这一阶段通用的那句。 */
  headline?: string;
}

export type PackSubmission =
  | { step: 'check'; order: PackOrder; observed: number }
  | { step: 'pack'; order: PackOrder; parcels: readonly ParcelDraft[] };

export interface PackState {
  phase: PackPhase;
  /** 设备绑的仓。 */
  warehouse: string;
  /** 在做的这张单；等扫出库单号时为 null。 */
  order: PackOrder | null;
  /** 复核：各种货已扫几件，键是 goodsKey。 */
  counted: Readonly<Record<string, number>>;
  parcels: readonly ParcelDraft[];
  nextKey: number;
  /** 这一趟打包好了几张。 */
  done: number;
  /** 在途的那个码（找单、认商品、认包材）或那张单的请求号（读明细）；没有为 null。 */
  pending: string | null;
  feedback: PackFeedback;
  /** 在途或没成的那一笔提交。 */
  submission: PackSubmission | null;
  /** 复核不符的那一笔：该有几件、点到几件。 */
  mismatch: { expected: number; observed: number } | null;
  /** 委托没提交成时，后端的答复能不能证明打包已经记下（只有提交委托那一步才会答的词）；证明不了时说话要留余地。 */
  recorded: boolean;
}

export type PackEffect =
  | { kind: 'find-order'; code: string }
  | { kind: 'open-order'; requestId: string }
  | { kind: 'identify'; order: PackOrder; code: string }
  | { kind: 'identify-packaging'; code: string }
  | { kind: 'submit-check'; order: PackOrder; observed: number }
  | { kind: 'submit-pack'; order: PackOrder; parcels: readonly ParcelDraft[] }
  | { kind: 'resend' }
  /** 队列已经不是现在的样子（做完、记下了、换了单）：重读。 */
  | { kind: 'refresh-queue' };

export interface PackStep {
  state: PackState;
  effects: readonly PackEffect[];
  /** 这一步该响哪种声音；没有就不响。 */
  cue?: ScanCue;
}

const waitingFeedback: PackFeedback = { kind: 'waiting', text: '等你扫码。' };

function step(state: PackState, cue?: ScanCue, effects: readonly PackEffect[] = []): PackStep {
  return cue === undefined ? { state, effects } : { state, effects, cue };
}

function wrong(state: PackState, text: string, scanned?: string, goto?: Goto, headline?: string): PackStep {
  return step(
    {
      ...state,
      feedback: {
        kind: 'wrong',
        text,
        ...(scanned === undefined ? {} : { scanned }),
        ...(goto === undefined ? {} : { goto }),
        ...(headline === undefined ? {} : { headline }),
      },
    },
    'wrong',
  );
}

const nothing = (state: PackState): PackStep => ({ state, effects: [] });

// —— 开工 ——

function blank(warehouse: string): PackState {
  return {
    phase: 'waiting',
    warehouse,
    order: null,
    counted: {},
    parcels: [],
    nextKey: 1,
    done: 0,
    pending: null,
    feedback: waitingFeedback,
    submission: null,
    mismatch: null,
    recorded: false,
  };
}

/** 开一趟：地址里带了出库请求号就直接去读那张单，没带就等人扫。 */
export function startPack(warehouse: string, request: string | null): PackStep {
  const state = blank(warehouse);
  if (request === null) return nothing(state);
  return step({ ...state, phase: 'opening', pending: request, feedback: { kind: 'waiting', text: `正在读出库单 ${request}…` } }, undefined, [
    { kind: 'open-order', requestId: request },
  ]);
}

// —— 数目 ——

export const expectedTotal = (order: PackOrder): number => order.goods.reduce((sum, goods) => sum + goods.quantity, 0);

export const countedTotal = (state: PackState): number => Object.values(state.counted).reduce((sum, count) => sum + count, 0);

const countOf = (state: PackState, goods: PackGoods): number => state.counted[goodsKey(goods)] ?? 0;

// —— 找单 ——

/** 回到等扫出库单号：这张单的一切草稿都丢掉（没提交的数与表单不会记下），这一趟做完几张留着。 */
function toWaiting(state: PackState, feedback: PackFeedback): PackState {
  return { ...blank(state.warehouse), done: state.done, nextKey: state.nextKey, feedback };
}

/** 点队列里的一张单（只在等扫出库单号时）：去读它的明细。 */
export function openOrder(state: PackState, requestId: string): PackStep {
  if (state.phase !== 'waiting') return nothing(state);
  return step(
    { ...state, phase: 'opening', pending: requestId, feedback: { kind: 'waiting', text: `正在读出库单 ${requestId}…` } },
    undefined,
    [{ kind: 'open-order', requestId }],
  );
}

/** 扫到的码找出来了。认的是哪个码要对得上，对不上说明这一步已经过期，不动。 */
export function orderFound(state: PackState, code: string, lookup: OrderLookup): PackStep {
  if (state.phase !== 'finding' || state.pending !== code) return nothing(state);
  const back = (feedback: PackFeedback): PackState => ({ ...state, phase: 'waiting', pending: null, feedback });
  switch (lookup.kind) {
    case 'found': {
      const row = lookup.row;
      const number = row.documentNumber ? row.documentNumber : row.requestId;
      const work = judgeOrder({ requestId: row.requestId, number, warehouse: row.warehouseCode, status: row.status }, state.warehouse);
      if (work.kind === 'not-here') return wrong(back(waitingFeedback), `${work.text}没有记任何东西。`, code, work.goto);
      return step(
        { ...state, phase: 'opening', pending: row.requestId, feedback: { kind: 'waiting', text: `正在读 ${number}…`, scanned: code } },
        undefined,
        [{ kind: 'open-order', requestId: row.requestId }],
      );
    }
    case 'elsewhere':
      return wrong(
        back(waitingFeedback),
        `${lookup.number} 属于仓 ${lookup.warehouse}，这台设备绑的是仓 ${state.warehouse}。没有记任何东西。`,
        code,
        undefined,
        '这张单在别的仓',
      );
    case 'absent':
      return wrong(
        back(waitingFeedback),
        `没有找到出库单 ${code}：这个仓和别的仓里都问过了，没有这个号；本仓没有这个号的周转箱，也没有等复核的单品单件出库单要这种货。没有记任何东西。`,
        code,
        undefined,
        '没有这张单',
      );
    case 'emptyTote':
      return wrong(
        back(waitingFeedback),
        lookup.state === 'RETIRED'
          ? `${code} 是一个已停用的周转箱，上面没有出库单。没有记任何东西。`
          : `${code} 是一个空着的周转箱，上面没有绑着出库单：拣货时没有扫这个箱，或它的单已经打完包。扫出库单号试试。没有记任何东西。`,
        code,
        undefined,
        '这个箱上没有单',
      );
    case 'unverified':
      return step(
        {
          ...back(waitingFeedback),
          feedback: { kind: 'unsent', text: `没能核对出库单 ${code}：${withoutStop(lookup.reason)}。没有记任何东西，再扫一次。`, scanned: code },
        },
        'wrong',
      );
  }
}

/** 这张单的明细读到了：能复核就开始逐件扫，已复核就直接打包，别的说清为什么不能在这里做。 */
export function orderOpened(state: PackState, requestId: string, opened: OrderOpened): PackStep {
  if (state.phase !== 'opening' || state.pending !== requestId) return nothing(state);
  const back = (feedback: PackFeedback): PackState => toWaiting(state, feedback);
  switch (opened.kind) {
    case 'absent':
      return wrong(back(waitingFeedback), `没有找到出库单 ${requestId}：读口答了，没有这个号。没有记任何东西。`, requestId, undefined, '没有这张单');
    case 'failed':
      return step(
        { ...back(waitingFeedback), feedback: { kind: 'unsent', text: `没能读出库单 ${requestId}：${withoutStop(opened.reason)}。没有记任何东西，再试一次。` } },
        'wrong',
      );
    case 'order': {
      const work = judgeOpened(opened, state.warehouse);
      const order = opened.order;
      if (work.kind === 'not-here') return wrong(back(waitingFeedback), `${work.text}没有记任何东西。`, undefined, work.goto);
      if (work.kind === 'pack') return beginPacking({ ...state, order, pending: null }, `${order.number} 已经复核过了，直接打包。`);
      const earlier =
        order.lastMismatch === null
          ? ''
          : `这张单以前复核过 ${order.lastMismatch.times} 次，数量都没对上（最近一次点了 ${order.lastMismatch.observed} 件）。`;
      return step({
        ...state,
        phase: 'checking',
        order,
        pending: null,
        counted: {},
        feedback: { kind: 'waiting', text: `${earlier}逐件扫单里的货，扫齐了自动提交复核。` },
      });
    }
  }
}

// —— 复核：逐件扫 ——

function goodsNamed(order: PackOrder, goodsCode: string): readonly PackGoods[] {
  return order.goods.filter((goods) => goods.goodsCode === goodsCode);
}

/** 这一格又扫对了一件：够数（合计等于已拣合计）就自动提交复核。 */
function counting(state: PackState, goods: PackGoods, scanned?: string): PackStep {
  const key = goodsKey(goods);
  const counted = { ...state.counted, [key]: countOf(state, goods) + 1 };
  const text = `对。${goods.goodsCode} 已扫 ${counted[key]} / ${goods.quantity}。`;
  return afterCounts({ ...state, counted }, text, scanned);
}

function afterCounts(state: PackState, text: string, scanned?: string): PackStep {
  const order = state.order as PackOrder;
  const total = expectedTotal(order);
  const sum = countedTotal(state);
  const feedback: PackFeedback = { kind: 'ok', text, ...(scanned === undefined ? {} : { scanned }) };
  if (sum !== total) return step({ ...state, phase: 'checking', pending: null, feedback }, 'ok');
  return step(
    {
      ...state,
      phase: 'check-submitting',
      pending: null,
      mismatch: null,
      submission: { step: 'check', order, observed: sum },
      feedback: { ...feedback, text: `${text}扫齐了，正在提交复核。` },
    },
    'ok',
    [{ kind: 'submit-check', order, observed: sum }],
  );
}

function scanGoods(state: PackState, code: string): PackStep {
  const order = state.order as PackOrder;
  const named = goodsNamed(order, code);
  if (named.length > 0) {
    const open = named.find((goods) => countOf(state, goods) < goods.quantity);
    if (open === undefined) return wrong(state, overScan(code, named), code);
    return counting(state, open, code);
  }
  return step({ ...state, phase: 'resolving', pending: code, feedback: { kind: 'waiting', text: `正在认 ${code}…`, scanned: code } }, undefined, [
    { kind: 'identify', order, code },
  ]);
}

function overScan(goodsCode: string, named: readonly PackGoods[]): string {
  const total = named.reduce((sum, goods) => sum + goods.quantity, 0);
  return `${goodsCode} 已经扫够了（这张单该有 ${total} 件）。没有记数，拿开再扫。`;
}

/**
 * 认商品码要拿一种货当参照去问（pick-goods 的认法：认成参照就答「同一个」，认成别的商品就答那个商品的编码）。
 * 这张单里随便取一种都行，因为答回「另一种」时 `goodsIdentified` 会按商品编码到单里找；统一取第一种，接线的一方与这里用同一个。
 */
export const referenceGoods = (order: PackOrder): PackGoods => order.goods[0];

/**
 * 商品码认出来了：认的是单里的货且还没扫够就加一，别的都红、不记数。认的是哪个码要对得上，对不上说明这一步已经过期，不动。
 * 认码是拿这张单里的一种货当参照问的（pick-goods 的认法）：认成参照货答「同一个」，认成别的商品答商品编码，
 * 所以答回的商品在不在这张单里要在这里再判一遍。
 */
export function goodsIdentified(state: PackState, code: string, identification: GoodsIdentification): PackStep {
  if (state.phase !== 'resolving' || state.pending !== code || state.order === null) return nothing(state);
  const order = state.order;
  const back: PackState = { ...state, phase: 'checking', pending: null };
  switch (identification.kind) {
    case 'same': {
      const goods = referenceGoods(order);
      // 同一个商品编码可能在单里占好几格（单位不同）：先装还没扫够的那一格，全扫够了才算多扫
      const named = goodsNamed(order, goods.goodsCode);
      const open = named.find((candidate) => countOf(back, candidate) < candidate.quantity);
      if (open === undefined) return wrong(back, overScan(goods.goodsCode, named), code);
      return counting(back, open, code);
    }
    case 'other': {
      const named = goodsNamed(order, identification.goodsCode);
      if (named.length === 0) {
        // 扫到的就是商品编码本身时不说「认成」它自己
        const what = code === identification.goodsCode ? `扫到的是商品 ${code}` : `扫到的是 ${code}，认成商品 ${identification.goodsCode}`;
        return wrong(back, `${what}，它不是这张单的货。没有记数，拿开再扫。`, code);
      }
      const open = named.find((goods) => countOf(back, goods) < goods.quantity);
      if (open === undefined) return wrong(back, overScan(identification.goodsCode, named), code);
      return counting(back, open, code);
    }
    case 'unknown':
      return wrong(
        back,
        `扫到的是 ${code}，认不出这是什么商品（只认商品编码，和这位货主登记过的条码、平台商品码）。没有记数，拿开再扫。`,
        code,
      );
    case 'failed':
      return wrong(back, `没能认出 ${code}：${withoutStop(identification.reason)}。没有记数，再扫一次。`, code);
  }
}

// —— 数量 ——

export type CountsJudgement = { ok: true; counts: Record<string, number> } | { ok: false; reason: string };

/** 一种货已扫的件数最多填到几：只是挡手滑，多出来的件数照实点。 */
const maxCount = 999999;

/** 「改数量」填的各种货已扫的件数：0 或正整数。多出来（超过该有的）也收，那是点到的真数，照实提交时由后端判不符。 */
export function judgeCounts(order: PackOrder, texts: Readonly<Record<string, string>>): CountsJudgement {
  const counts: Record<string, number> = {};
  for (const goods of order.goods) {
    const key = goodsKey(goods);
    const trimmed = (texts[key] ?? '').trim();
    if (!/^\d+$/.test(trimmed)) return { ok: false, reason: `${goods.goodsCode}：已扫的件数要填 0 或正整数。` };
    const value = Number.parseInt(trimmed, 10);
    if (!Number.isSafeInteger(value) || value > maxCount) return { ok: false, reason: `${goods.goodsCode}：最多填到 ${maxCount} 件。` };
    counts[key] = value;
  }
  return { ok: true, counts };
}

/** 直接把各种货已扫的件数改成指定的数；改到合计等于该有的，像扫齐一样自动提交。 */
export function changeCounts(state: PackState, texts: Readonly<Record<string, string>>): { ok: true; step: PackStep } | { ok: false; reason: string } {
  if (state.phase !== 'checking' || state.order === null) return { ok: false, reason: '现在不在复核逐件扫的这一步。' };
  const judged = judgeCounts(state.order, texts);
  if (!judged.ok) return judged;
  const next: PackState = { ...state, counted: judged.counts };
  return { ok: true, step: afterCounts(next, `数量改过了：已扫 ${countedTotal(next)} / ${expectedTotal(state.order)}。`) };
}

// —— 数量不符，照实提交 ——

export interface MismatchPlan {
  expected: number;
  observed: number;
  /** 差几件（取正数）。 */
  difference: number;
  /** 点到的比该有的多。 */
  over: boolean;
}

/** 数量不符、要照实提交时的情形；对得上（自动提交了）或不在逐件扫的这一步答 null。一件都没点到也算：照实记 0。 */
export function mismatchPlan(state: PackState): MismatchPlan | null {
  if (state.phase !== 'checking' || state.order === null) return null;
  const expected = expectedTotal(state.order);
  const observed = countedTotal(state);
  if (observed === expected) return null;
  return { expected, observed, difference: Math.abs(expected - observed), over: observed > expected };
}

/** 确认框里的话照核到的规则：记下这一次复核、这张单仍停在已拣、不能打包、可以再复核。 */
export function mismatchLines(plan: MismatchPlan): string[] {
  const how = plan.observed === 0 ? '一件都没点到' : `点到的是 ${plan.observed} 件，${plan.over ? '多' : '少'}了 ${plan.difference} 件`;
  return [
    `这张单已拣 ${plan.expected} 件，你${how}。`,
    '照实提交之后，后端会把这一次复核记下来，但数量对不上：这张单仍停在「已拣货」，不能打包。',
    '之后点准了数，可以再复核一次。',
  ];
}

/** 确认之后提交：按已点的总数。 */
export function submitMismatch(state: PackState): PackStep {
  const plan = mismatchPlan(state);
  if (plan === null || state.order === null) return nothing(state);
  const order = state.order;
  return step(
    {
      ...state,
      phase: 'check-submitting',
      mismatch: { expected: plan.expected, observed: plan.observed },
      submission: { step: 'check', order, observed: plan.observed },
      feedback: { kind: 'waiting', text: `按实点的 ${plan.observed} 件提交复核（该有 ${plan.expected} 件）…` },
    },
    undefined,
    [{ kind: 'submit-check', order, observed: plan.observed }],
  );
}

// —— 打包：包裹 ——

/** 开始打包：先备好第一个包裹，装第一种还没装满的货。 */
function beginPacking(state: PackState, text: string): PackStep {
  const order = state.order as PackOrder;
  const first = firstUnpacked(order, []);
  const parcel = first === null ? emptyParcel(state.nextKey, order.goods[0], 0) : emptyParcel(state.nextKey, first.goods, first.remaining);
  return step({
    ...state,
    phase: 'packing',
    pending: null,
    submission: null,
    parcels: [parcel],
    nextKey: state.nextKey + 1,
    feedback: { kind: 'ok', text },
  });
}

/** 「再加一个包裹」：新包裹先装第一种还没装满的货；都装满了就留空件数，让人从别的包裹里匀。 */
export function addParcel(state: PackState): PackStep {
  if (state.phase !== 'packing' || state.order === null) return nothing(state);
  const order = state.order;
  const unpacked = firstUnpacked(order, state.parcels);
  const parcel = unpacked === null ? emptyParcel(state.nextKey, order.goods[0], 0) : emptyParcel(state.nextKey, unpacked.goods, unpacked.remaining);
  return step({
    ...state,
    parcels: [...state.parcels, parcel],
    nextKey: state.nextKey + 1,
    feedback: { kind: 'waiting', text: `加了包裹 ${state.parcels.length + 1}。` },
  });
}

export function removeParcel(state: PackState, key: number): PackStep {
  if (state.phase !== 'packing' || !state.parcels.some((parcel) => parcel.key === key)) return nothing(state);
  return step({ ...state, parcels: state.parcels.filter((parcel) => parcel.key !== key), feedback: waitingFeedback });
}

/** 人在表单里能改的格。 */
export type ParcelPatch = Partial<
  Pick<ParcelDraft, 'goodsCode' | 'unit' | 'quantity' | 'weight' | 'weightUnit' | 'length' | 'width' | 'height' | 'dimensionUnit'>
>;

/** 改一个包裹上的格。改了尺寸的任一格就记下「人改过尺寸」，之后换包材不再覆盖。 */
export function editParcel(state: PackState, key: number, patch: ParcelPatch): PackStep {
  if (state.phase !== 'packing') return nothing(state);
  const touchesDimensions = 'length' in patch || 'width' in patch || 'height' in patch || 'dimensionUnit' in patch;
  return step({
    ...state,
    parcels: state.parcels.map((parcel) => (parcel.key === key ? { ...parcel, ...patch, ...(touchesDimensions ? { dimsEdited: true } : {}) } : parcel)),
  });
}

/** 点选了一个包材给某个包裹（扫来的走 packagingIdentified）。 */
export function chooseMaterial(state: PackState, key: number, material: PackagingMaterial): PackStep {
  if (state.phase !== 'packing' || material.deactivated) return nothing(state);
  const position = state.parcels.findIndex((parcel) => parcel.key === key);
  if (position < 0) return nothing(state);
  const parcels = state.parcels.map((parcel) => (parcel.key === key ? withMaterial(parcel, material) : parcel));
  return step({ ...state, parcels, feedback: { kind: 'ok', text: materialText(material, parcels[position], position + 1) } });
}

function materialText(material: PackagingMaterial, parcel: ParcelDraft, position: number): string {
  const named = material.name ? `${material.code}（${material.name}）` : material.code;
  // 包材没登记外廓时包裹上的尺寸是人填的或空着，不能说成是取了包材的；两边恰好都空更不算
  const fromMaterial =
    material.length !== '' &&
    parcel.length === material.length &&
    parcel.width === material.width &&
    parcel.height === material.height &&
    parcel.dimensionUnit === material.dimensionUnit;
  return fromMaterial ? `包裹 ${position} 用包材 ${named}，尺寸取了它的外廓，可以改。` : `包裹 ${position} 用包材 ${named}。`;
}

/** 扫来的包材码要落在哪个包裹上：第一个还没选包材的；都选好了答 null（要换的点那个包裹的「换包材」）。 */
export function scanTarget(state: PackState): number | null {
  const parcel = state.parcels.find((candidate) => candidate.packaging.trim() === '');
  return parcel === undefined ? null : parcel.key;
}

function scanPackaging(state: PackState, code: string): PackStep {
  const order = state.order as PackOrder;
  if (order.goods.some((goods) => goods.goodsCode === code)) {
    return wrong(state, `扫到的是商品 ${code}。这一步要扫包材码，货不用再扫了。没有记任何东西。`, code);
  }
  if (code === order.requestId || code === order.number) {
    return wrong(state, `扫到的是这张出库单的单号。这一步要扫包材码；要换一张单，先点「换一张单」。没有记任何东西。`, code);
  }
  if (scanTarget(state) === null) {
    return wrong(state, '每个包裹都选好包材了。要换哪个包裹的包材，点那个包裹的「换包材」。没有记任何东西。', code);
  }
  return step({ ...state, phase: 'identifying', pending: code, feedback: { kind: 'waiting', text: `正在核对包材 ${code}…`, scanned: code } }, undefined, [
    { kind: 'identify-packaging', code },
  ]);
}

/** 包材码核出结果了：在用的落到第一个没选包材的包裹上；停用、没有、没核成各说各的，什么都不记。过期的答复不动。 */
export function packagingIdentified(state: PackState, code: string, lookup: PackagingLookup): PackStep {
  if (state.phase !== 'identifying' || state.pending !== code) return nothing(state);
  const back: PackState = { ...state, phase: 'packing', pending: null };
  switch (lookup.kind) {
    case 'found': {
      if (lookup.material.deactivated) return wrong(back, `包材 ${code} 已停用，不能用。没有记下，换一个再扫。`, code);
      const key = scanTarget(back);
      if (key === null) return wrong(back, '每个包裹都选好包材了。要换哪个包裹的包材，点那个包裹的「换包材」。没有记任何东西。', code);
      const position = back.parcels.findIndex((parcel) => parcel.key === key);
      const parcels = back.parcels.map((parcel) => (parcel.key === key ? withMaterial(parcel, lookup.material) : parcel));
      return step({ ...back, parcels, feedback: { kind: 'ok', text: `对。${materialText(lookup.material, parcels[position], position + 1)}`, scanned: code } }, 'ok');
    }
    case 'absent':
      return wrong(back, `登记册里没有包材 ${code}。没有记下，换一个再扫。`, code);
    case 'unverified':
      return step(
        { ...back, feedback: { kind: 'unsent', text: `没能核对包材 ${code}：${withoutStop(lookup.reason)}。没有记下，再扫一次。`, scanned: code } },
        'wrong',
      );
  }
}

// —— 打包：提交 ——

/** 打包提交前能不能点；能就按草稿交出整张单。 */
export function submitPack(state: PackState): PackStep {
  if (state.phase !== 'packing' || state.order === null) return nothing(state);
  if (!judgePack(state.order, state.parcels).ok) return nothing(state);
  const order = state.order;
  const parcels = state.parcels;
  return step(
    {
      ...state,
      phase: 'pack-submitting',
      submission: { step: 'pack', order, parcels },
      feedback: { kind: 'waiting', text: `正在提交 ${order.number} 的打包（${parcels.length} 个包裹）…` },
    },
    undefined,
    [{ kind: 'submit-pack', order, parcels }],
  );
}

// —— 扫码 ——

/**
 * 扫到一个码。码是扫描框规范化过的；这里再规范一次，空的不算。各阶段的判定：
 * 等扫出库单号 → 去认是哪张单（effect: find-order）；复核逐件扫 → 等于单里某种货的商品编码当场算对，扫够了再扫算错，别的要去认
 * （effect: identify）；打包 → 当包材码去核（effect: identify-packaging）。读口在途与提交的阶段不收扫码。
 */
export function scanCode(state: PackState, raw: string): PackStep {
  const code = normalizeScan(raw);
  if (code === '') return nothing(state);
  switch (state.phase) {
    case 'waiting':
      return step({ ...state, phase: 'finding', pending: code, feedback: { kind: 'waiting', text: `正在找 ${code} 是哪张单…`, scanned: code } }, undefined, [
        { kind: 'find-order', code },
      ]);
    case 'checking':
      return scanGoods(state, code);
    case 'packing':
      return scanPackaging(state, code);
    default:
      return nothing(state);
  }
}

/** Esc：清掉扫错与没核成，回到等扫。这两样只是屏上的一句话，什么都没记，所以清它们不改别的。 */
export function dismissWrong(state: PackState): PackStep {
  const scanning = state.phase === 'waiting' || state.phase === 'checking' || state.phase === 'packing';
  if (!scanning || (state.feedback.kind !== 'wrong' && state.feedback.kind !== 'unsent')) return nothing(state);
  return step({ ...state, feedback: waitingFeedback });
}

// —— 换一张单 ——

const leavable: readonly PackPhase[] = ['checking', 'check-unsent', 'check-refused', 'mismatch', 'packing', 'pack-unsent', 'pack-pending', 'pack-refused'];

/** 离开时屏上说一句：哪些东西没有记下、这张单现在是什么状况。不能让人以为扫到一半的数或填到一半的包裹已经存下了。 */
function leaveNote(state: PackState): string {
  switch (state.phase) {
    case 'checking':
      return countedTotal(state) > 0 ? '换了一张单。扫到一半的数没有记下。' : '换了一张单。';
    case 'check-unsent':
      return '换了一张单。刚才那笔复核没有收到后端的确认，不知道记下了没有；要弄清，重新扫那张单，屏上会写它现在的状态。';
    case 'check-refused':
      return '换了一张单。刚才那笔复核没有记成。';
    case 'mismatch':
      return '换了一张单。那张单的复核不符已经记下，它仍停在已拣货。';
    case 'packing':
      return '换了一张单。填到一半的包裹没有记下。';
    case 'pack-unsent':
      return '换了一张单。刚才那笔打包没有收到后端的确认，不知道记下了没有；要弄清，重新扫那张单，屏上会写它现在的状态。';
    case 'pack-pending':
      return state.recorded
        ? '换了一张单。那张单的打包已经记下，但委托没有提交，这个工位不能再用同一个事实号接着送。'
        : '换了一张单。刚才那笔打包没有走完，不知道记下了没有；要弄清，重新扫那张单，屏上会写它现在的状态。这个工位不能再送那一笔了。';
    case 'pack-refused':
      return '换了一张单。刚才那笔打包没有记成，填的包裹没有记下。';
    default:
      return '换了一张单。';
  }
}

/**
 * 「换一张单」：回到等扫出库单号，这张单扫到一半的数与填到一半的包裹丢掉——它们本来就没记下，屏上要说清这一点。
 * 在途（读口、提交）时不让换：一笔请求发出去了，结果要等它回来。
 */
export function leaveOrder(state: PackState): PackStep {
  if (!leavable.includes(state.phase) || state.order === null) return nothing(state);
  return step(toWaiting(state, { kind: 'waiting', text: leaveNote(state) }), undefined, [{ kind: 'refresh-queue' }]);
}

// —— 提交的结果 ——

function withoutStop(reason: string): string {
  return reason.replace(/[。.]+$/, '');
}

/**
 * 工位对个别答复的说法：复核与打包各一张表，同一个词在两个口上意思不同（EXISTING_RESULT、STATUS_MISMATCH、CONTENT_INVALID……）。
 * 哪些答复算成、算没成不在这里，在 judgeWorkFactAnswer。
 */
export const checkOutcomeLabels: Readonly<Record<string, string>> = {
  CHECKED: '复核通过：实点数与已拣数量一致，这张单可以打包了。',
  CHECK_MISMATCH: '复核数量与已拣不符：这一次复核已经记下，这张单仍停在已拣，不能打包。',
  EXISTING_RESULT: '这一笔复核之前已经记过且对上了（同一个作业事实号），这次没有重复记。',
  CONTENT_CONFLICT: '这个作业事实号已经记过一笔数量不同的复核，这一笔没有记。',
  CONTENT_INVALID: '后端不认这一笔的内容：这张单不属于这台设备绑的仓，没有记。',
  WAVE_NOT_FOUND: '找不到这张出库单，它可能已经被撤掉。这一笔没有记。',
  STATUS_MISMATCH: '这张单现在不能复核：它已经不是「已拣货」了（可能别的工位已经复核过），或者还有拣货任务没拣完。这一笔没有记。',
  NOT_CONFIGURED: '未配置：本租户还没有登记仓，这一笔复核没有记。',
};

export const packOutcomeLabels: Readonly<Record<string, string>> = {
  PACKED: '已打包。这张单不走本产品网络，没有委托要提交。',
  SUBMITTED: '已打包，并且已提交本产品网络委托。',
  EXISTING_RESULT: '这一笔打包之前已经记过了（同一个作业事实号），这次没有重复记。',
  CONTENT_INVALID:
    '后端不认这一笔的内容，没有记。最常见的是各包裹装的件数合计对不上这张单已拣的件数，或者某一格是空的；改一改再提交。',
  // 这两个词打包口自己答，提交委托那一步在打包记下之后也会答（单被取消了、单找不到了），所以只说答复能证明的话
  WAVE_NOT_FOUND: '后端答找不到这张出库单，它可能已经被撤掉。这一笔有没有记下，重新扫这张单，屏上会写它现在的状态。',
  STATUS_MISMATCH:
    '后端答这张单现在的状态不对：它已经不是「已复核」了（可能别人已经打包，或已经被取消）。这一笔有没有记下，重新扫这张单，屏上会写它现在的状态。',
  NOT_CONFIGURED:
    '后端答未配置：打包所需的仓，或本产品网络的代办授予还没有登记。这一笔有没有记下，重新扫这张单看它的状态：「已打包」就是打包记下了、只是委托没提交，登记好之后用「再送一次」接着提交。',
  CONTENT_CONFLICT:
    '后端答内容冲突：同一个作业事实号，或同一来源的委托，已经有一份内容不同的记录，没有覆盖。这一笔有没有记下，重新扫这张单，屏上会写它现在的状态。',
  OWNERSHIP_UNRESOLVED: '打包已经记下，但本产品网络委托没有提交：这位货主的生产归属还没有确定。归属定下之后，用「再送一次」（同一个作业事实号）接着提交。',
  ADMISSION_PAUSED: '打包已经记下，但本产品网络委托没有提交：这位货主所在范围的准入现在暂停着。恢复之后，用「再送一次」接着提交。',
  OTHER_PRODUCTION_AUTHORITY: '打包已经记下，但本产品网络委托没有提交：这位货主的生产由别的权威方承接，本产品不为它建委托。',
  INPUT_NOT_ACCEPTED: '打包已经记下，但本产品网络委托没有提交：委托的标识或内容没有被受理。',
};

/** 只有提交委托那一步才会答的四个词：答到它们，就证明打包已经记下，只是委托没提交成。 */
export const packCommissionWords: ReadonlySet<string> = new Set([
  'OWNERSHIP_UNRESOLVED',
  'ADMISSION_PAUSED',
  'OTHER_PRODUCTION_AUTHORITY',
  'INPUT_NOT_ACCEPTED',
]);

/**
 * 打包答复里给「再送一次」的几种，用同一个作业事实号，后端重放打包口会接着提交委托：`packCommissionWords` 里的四个词，加 NOT_CONFIGURED——
 * 它既可能是代办授予没登记（打包已记下），也可能是一个仓都没登记（没记下），答复里分不出，而重送对两种都无害，
 * 没记下的重送还是同一个答复。
 */
export const packResendable: ReadonlySet<string> = new Set([...packCommissionWords, 'NOT_CONFIGURED']);

export function mismatchText(plan: { expected: number; observed: number }): string {
  if (plan.expected === plan.observed) {
    return '后端核出来的已拣数量与屏上的不一样（可能刚有人改过拣货）。这一次复核已经记下，这张单仍停在「已拣货」，不能打包。重新扫这张单，按新的数再复核。';
  }
  const how = plan.observed === 0 ? '一件都没点到' : `点到 ${plan.observed} 件`;
  return `这张单复核不符：该有 ${plan.expected} 件，${how}。这一次复核已经记下，但数量对不上，这张单仍停在「已拣货」，不能打包。点准了数，可以再复核一次。`;
}

function packedText(order: PackOrder, parcels: number, verdict: WorkFactVerdict): string {
  if (verdict.outcome === 'EXISTING_RESULT') return `${order.number}：${verdict.headline}`;
  return `${order.number} 打包好了：${parcels} 个包裹。${verdict.headline}`;
}

/** 后端答了：复核对上了进打包，不符照实说，打包成了回到等扫；没送到等「再送一次」；没成说明原因。 */
export function applyVerdict(state: PackState, verdict: WorkFactVerdict): PackStep {
  const submission = state.submission;
  if (submission === null) return nothing(state);
  if (state.phase === 'check-submitting' && submission.step === 'check') return applyCheckVerdict(state, submission, verdict);
  if (state.phase === 'pack-submitting' && submission.step === 'pack') return applyPackVerdict(state, submission, verdict);
  return nothing(state);
}

function applyCheckVerdict(state: PackState, submission: Extract<PackSubmission, { step: 'check' }>, verdict: WorkFactVerdict): PackStep {
  switch (verdict.verdict) {
    case 'done': {
      if (verdict.outcome === 'CHECK_MISMATCH') {
        const plan = state.mismatch ?? { expected: expectedTotal(submission.order), observed: submission.observed };
        return step(
          { ...state, phase: 'mismatch', mismatch: plan, feedback: { kind: 'wrong', text: mismatchText(plan) } },
          'wrong',
          [{ kind: 'refresh-queue' }],
        );
      }
      const text =
        verdict.outcome === 'EXISTING_RESULT'
          ? `${verdict.headline}接着打包。`
          : `复核通过：${submission.observed} 件都对上了。接着打包。`;
      return {
        ...beginPacking({ ...state, mismatch: null }, text),
        effects: [{ kind: 'refresh-queue' }],
        cue: 'ok',
      };
    }
    case 'retry':
      return step({ ...state, phase: 'check-unsent', feedback: { kind: 'unsent', text: verdict.headline } }, 'wrong');
    case 'refused':
      return step({ ...state, phase: 'check-refused', feedback: { kind: 'wrong', text: verdict.headline } }, 'wrong');
  }
}

function applyPackVerdict(state: PackState, submission: Extract<PackSubmission, { step: 'pack' }>, verdict: WorkFactVerdict): PackStep {
  switch (verdict.verdict) {
    case 'done':
      return step(
        toWaiting({ ...state, done: state.done + 1 }, { kind: 'ok', text: packedText(submission.order, submission.parcels.length, verdict) }),
        'ok',
        [{ kind: 'refresh-queue' }],
      );
    case 'retry':
      return step({ ...state, phase: 'pack-unsent', feedback: { kind: 'unsent', text: verdict.headline } }, 'wrong');
    case 'refused':
      if (verdict.outcome !== undefined && packResendable.has(verdict.outcome)) {
        return step(
          { ...state, phase: 'pack-pending', recorded: packCommissionWords.has(verdict.outcome), feedback: { kind: 'unsent', text: verdict.headline } },
          'wrong',
          [{ kind: 'refresh-queue' }],
        );
      }
      return step({ ...state, phase: 'pack-refused', feedback: { kind: 'wrong', text: verdict.headline } }, 'wrong');
  }
}

/** 「再送一次」：用同一个事实号（由接线的一方保留那一笔 attempt）。 */
export function resend(state: PackState): PackStep {
  if (state.phase === 'check-unsent') {
    return step({ ...state, phase: 'check-submitting', feedback: { kind: 'waiting', text: '正在再送一次…' } }, undefined, [{ kind: 'resend' }]);
  }
  if (state.phase === 'pack-unsent' || state.phase === 'pack-pending') {
    return step({ ...state, phase: 'pack-submitting', feedback: { kind: 'waiting', text: '正在再送一次…' } }, undefined, [{ kind: 'resend' }]);
  }
  return nothing(state);
}

/** 打包没记成（业务上不收）之后回到表单：包裹草稿都还在，改一改再提交。 */
export function backToForm(state: PackState): PackStep {
  if (state.phase !== 'pack-refused') return nothing(state);
  return step({ ...state, phase: 'packing', submission: null, feedback: waitingFeedback });
}

// —— 屏上的字 ——

export interface PackScreen {
  /** 标题上方的小字：「下一步」，扫错时是「扫错了」。 */
  label: string;
  /** 最大的字：下一步该做什么。 */
  headline: string;
  /** 一行小字。 */
  counter: string;
}

const wrongHeadlines: Partial<Record<PackPhase, string>> = {
  waiting: '这张单现在不能在这里做',
  checking: '这件不能记数',
  packing: '这个码不能用',
};

/** 打包这一步下一步该做什么：指到第一处没填好的格；都填好了说可以提交。 */
export function packHeadline(state: PackState): string {
  if (state.order === null) return '';
  if (state.parcels.length === 0) return '点「再加一个包裹」';
  const judged = judgePack(state.order, state.parcels);
  if (judged.ok) return '都填好了，提交打包';
  if (judged.at === undefined) {
    const line = packBalance(state.order, state.parcels).find((item) => item.assigned !== item.expected);
    if (line === undefined) return '再加一个包裹';
    return line.assigned < line.expected ? `${line.goodsCode} 还有 ${line.expected - line.assigned} 件没装进包裹` : `${line.goodsCode} 装多了 ${line.assigned - line.expected} 件`;
  }
  const { position, field } = judged.at;
  const byField: Record<ParcelField, string> = {
    goods: `给包裹 ${position} 选货`,
    quantity: `给包裹 ${position} 填件数`,
    packaging: `给包裹 ${position} 扫包材码（或点选）`,
    weight: `给包裹 ${position} 录重量`,
    weightUnit: `给包裹 ${position} 填重量单位`,
    dimensions: `给包裹 ${position} 填长宽高`,
    dimensionUnit: `给包裹 ${position} 填尺寸单位`,
  };
  return byField[field];
}

export function packPacked(state: PackState): { assigned: number; expected: number } {
  if (state.order === null) return { assigned: 0, expected: 0 };
  const balance = packBalance(state.order, state.parcels);
  return { assigned: balance.reduce((sum, line) => sum + line.assigned, 0), expected: balance.reduce((sum, line) => sum + line.expected, 0) };
}

export function packScreen(state: PackState, queue: PackQueue | null): PackScreen {
  const wrongNow = state.feedback.kind === 'wrong' && wrongHeadlines[state.phase] !== undefined;
  const label = wrongNow ? '扫错了' : '下一步';
  const order = state.order;
  switch (state.phase) {
    case 'waiting':
      return {
        label,
        headline: wrongNow ? (state.feedback.headline ?? (wrongHeadlines.waiting as string)) : '扫出库单号',
        counter: state.done > 0 ? `${waitingCounter(queue)}。这一趟打包好了 ${state.done} 张。` : waitingCounter(queue),
      };
    case 'finding':
      return { label: '请稍等', headline: '正在找这张单…', counter: `扫到的是 ${state.pending ?? ''}，到出库单里核对它在哪个仓、现在什么状态。` };
    case 'opening':
      return { label: '请稍等', headline: '正在读这张单…', counter: `出库请求号 ${state.pending ?? ''}。` };
    case 'checking':
    case 'resolving': {
      const total = order === null ? 0 : expectedTotal(order);
      const sum = countedTotal(state);
      return {
        label,
        headline: wrongNow ? (wrongHeadlines.checking as string) : sum > total ? `已经扫了 ${sum} 件，比该有的多` : `逐件扫，还差 ${total - sum} 件`,
        counter: `这张单已扫 ${sum} / ${total}`,
      };
    }
    case 'check-submitting':
      return { label: '请稍等', headline: '正在提交复核…', counter: `实点 ${state.submission?.step === 'check' ? state.submission.observed : countedTotal(state)} 件` };
    case 'check-unsent':
      return { label: '没送到', headline: '这一笔复核没有送到', counter: `实点 ${state.submission?.step === 'check' ? state.submission.observed : countedTotal(state)} 件，用同一个作业事实号再送。` };
    case 'check-refused':
      return { label: '没记成', headline: '这一笔复核没有记成', counter: order === null ? '' : `${order.number}` };
    case 'mismatch':
      return {
        label: '复核不符',
        headline: '数量对不上，不能打包',
        counter: state.mismatch === null ? '' : `该有 ${state.mismatch.expected} 件，点到 ${state.mismatch.observed} 件`,
      };
    case 'packing':
    case 'identifying': {
      const { assigned, expected } = packPacked(state);
      return { label, headline: wrongNow ? (wrongHeadlines.packing as string) : packHeadline(state), counter: `已装进包裹 ${assigned} / ${expected} 件，${state.parcels.length} 个包裹` };
    }
    case 'pack-submitting':
      return { label: '请稍等', headline: '正在提交打包…', counter: order === null ? '' : `${order.number}，${state.parcels.length} 个包裹` };
    case 'pack-unsent':
      return { label: '没送到', headline: '这一笔打包没有送到', counter: '用同一个作业事实号再送，不会记两笔。' };
    case 'pack-pending':
      return state.recorded
        ? {
            label: '委托没提交',
            headline: '打包记下了，委托还没提交',
            counter: '打包已经记下。条件满足后点「再送一次」接着提交委托；换了单，这个工位就不能再送这一笔。',
          }
        : {
            label: '没走完',
            headline: '这一笔打包还没有走完',
            counter: '有没有记下，以上面后端答的话为准。条件满足后点「再送一次」，用同一个作业事实号，不会记两笔；换了单，这个工位就不能再送这一笔。',
          };
    case 'pack-refused':
      return { label: '没记成', headline: '这一笔打包没有记成', counter: '包裹都还在，可以改一改再提交。' };
  }
}

/** 屏上「进度」那一格：复核是第 1 步，打包是第 2 步。 */
export function packProgress(state: PackState): string {
  switch (state.phase) {
    case 'checking':
    case 'resolving':
    case 'check-submitting':
    case 'check-unsent':
    case 'check-refused':
    case 'mismatch':
      return '第 1 步，共 2 步：复核';
    case 'packing':
    case 'identifying':
    case 'pack-submitting':
    case 'pack-unsent':
    case 'pack-pending':
    case 'pack-refused':
      return '第 2 步，共 2 步：打包';
    default:
      return state.done > 0 ? `这一趟打包了 ${state.done} 张` : '';
  }
}

// —— 按钮 ——

export interface PackActions {
  /** 复核：直接改各种货已扫的数。 */
  changeCount: boolean;
  /** 复核：数量不符，照实提交。 */
  mismatch: boolean;
  addParcel: boolean;
  submitPack: boolean;
  /** 打包：回去改表单。 */
  backToForm: boolean;
  leave: boolean;
  resend: boolean;
  refresh: boolean;
}

/** 这一刻哪些按钮能点。不能点的在界面上禁用，不藏起来：人要看得出它存在，只是现在不是时候。 */
export function packActions(state: PackState): PackActions {
  return {
    changeCount: state.phase === 'checking',
    mismatch: mismatchPlan(state) !== null,
    addParcel: state.phase === 'packing',
    submitPack: state.phase === 'packing' && state.order !== null && judgePack(state.order, state.parcels).ok,
    backToForm: state.phase === 'pack-refused',
    leave: leavable.includes(state.phase) && state.order !== null,
    resend: state.phase === 'check-unsent' || state.phase === 'pack-unsent' || state.phase === 'pack-pending',
    refresh: state.phase === 'waiting',
  };
}

/** 扫描框收不收码：只有等扫出库单号、逐件扫、选包材这三步收。 */
export function acceptingScans(state: PackState): boolean {
  return state.phase === 'waiting' || state.phase === 'checking' || state.phase === 'packing';
}

/** 不收码时扫描框里写的字，让人知道为什么现在不理扫码。 */
export function scanPlaceholder(state: PackState): string {
  switch (state.phase) {
    case 'finding':
    case 'opening':
    case 'resolving':
    case 'identifying':
    case 'check-submitting':
    case 'pack-submitting':
      return '正在处理，先不收扫码';
    case 'check-unsent':
    case 'pack-unsent':
    case 'pack-pending':
      return '先点「再送一次」';
    case 'check-refused':
    case 'mismatch':
      return '先点「换一张单」';
    case 'pack-refused':
      return '先点「回去改」';
    case 'packing':
      return '扫包材码，或敲进去按回车';
    case 'checking':
      return '扫商品码，或敲进去按回车';
    default:
      return '扫出库单号，或敲进去按回车';
  }
}
