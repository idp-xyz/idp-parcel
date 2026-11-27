// 退货预报与退货到仓：从概览里认出一张、把预报与到仓记录对上、判状态与下一步、算进度（纯 TypeScript，node:test 直接测）。
//
// 退货只有一个概览读口（`NewQueryReturnsOverviewEndpoint` 与 postgres 里的 `ReturnsOverview`）：整份预报（连商品行）与整份退件
// 到仓记录（连回库处置）一次答回，不分页、不接筛选维。确认过它没有条数上限：预报与到仓记录两条查询都只按租户取、按时刻排，
// 没有给行数设上限（到仓记录那条里的 LIMIT 1 只是每笔取最近一次处置），HTTP 口也不读 limit，答回来的就是这个租户的全册。
// 所以单据页按号在概览里认不出，就是这个租户没有这一张；列表上各状态的数量也是全数。读口日后加了上限，这两处都要改口：
// 认不出改说「概览里没有」，数量不再写。
// 序：预报按受理时刻新到旧，到仓记录按到仓时刻新到旧，同刻按标识；列表页自己再排。
//
// 一笔到仓记录就是一个商品：同一件退件里有几种商品，就是同一个退件单号下的几笔到仓记录，各有各的作业事实号。
//
// 预报自己没有状态字段，读口也不答：到仓与处置都记在到仓记录上，所以预报的状态是拿到仓记录对出来的。读口答不出的环节不编——
// 预报之后只有「到仓」与「处置」两件事看得出来；认不出的退件转成待认领到货，它之后怎么了要去待认领页看，这里不替它说。

import type { DocumentTrack, TrackStep } from './document-track';

export interface ReturnAdviceFacts {
  adviceId: string;
  documentNumber?: string;
  ownerAccount: string;
  warehouseCode: string;
  trackingNumber: string;
  receivedAt: string;
  lines?: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
}

export interface ReturnReceiptFacts {
  factId: string;
  warehouseCode: string;
  trackingNumber: string;
  /** 认不出货主的转待认领，这一格是空。 */
  ownerAccount?: string;
  adviceId?: string;
  arrivalId?: string;
  goodsCode: string;
  unit: string;
  quantity: number;
  outcome: string;
  occurredAt: string;
  /** 有它就是已经记过去向。 */
  disposition?: { destination: string };
}

/** 预报号：有预报号取预报号；仓的业务时区没登记时不给号，退回预报标识——读口答的是空串，不能只防 null。 */
export function returnAdviceNumber(advice: Pick<ReturnAdviceFacts, 'adviceId' | 'documentNumber'>): string {
  return advice.documentNumber || advice.adviceId;
}

/** 按单据键认出预报：键是预报号，没有预报号的预报键就是它的标识。 */
export function findReturnAdvice<A extends Pick<ReturnAdviceFacts, 'adviceId' | 'documentNumber'>>(
  advices: readonly A[],
  key: string,
): A | undefined {
  if (key === '') return undefined;
  return advices.find((advice) => advice.documentNumber === key) ?? advices.find((advice) => advice.adviceId === key);
}

export function findReturnReceipt<R extends Pick<ReturnReceiptFacts, 'factId'>>(receipts: readonly R[], factId: string): R | undefined {
  if (factId === '') return undefined;
  return receipts.find((receipt) => receipt.factId === factId);
}

/**
 * 一笔到仓记录算不算这张预报的。系统认出来的记着预报标识，以它为准；没记的按系统自己认预报的口径对：同一个仓、同一个退件
 * 单号，货主若已认出也得是同一家。认领待认领到货之后另起的那笔入库就是这种——它不带预报标识，退件单号却对得上。
 */
export function receiptMatchesAdvice(
  receipt: Pick<ReturnReceiptFacts, 'adviceId' | 'warehouseCode' | 'trackingNumber' | 'ownerAccount'>,
  advice: Pick<ReturnAdviceFacts, 'adviceId' | 'warehouseCode' | 'trackingNumber' | 'ownerAccount'>,
): boolean {
  if (receipt.adviceId) return receipt.adviceId === advice.adviceId;
  if (receipt.warehouseCode !== advice.warehouseCode || receipt.trackingNumber !== advice.trackingNumber) return false;
  return !receipt.ownerAccount || receipt.ownerAccount === advice.ownerAccount;
}

/** 对上这张预报的到仓记录，保持概览里的先后（新到旧）。 */
export function receiptsOfAdvice<R extends ReturnReceiptFacts>(advice: ReturnAdviceFacts, receipts: readonly R[]): R[] {
  return receipts.filter((receipt) => receiptMatchesAdvice(receipt, advice));
}

/** 这笔到仓记录对上的预报：记着预报标识的就是那一张，没记的按退件单号对。 */
export function advicesOfReceipt<A extends ReturnAdviceFacts>(receipt: ReturnReceiptFacts, advices: readonly A[]): A[] {
  return advices.filter((advice) => receiptMatchesAdvice(receipt, advice));
}

/** 同一件退件里别的商品：同仓同退件单号的其余到仓记录。 */
export function siblingReceipts<R extends ReturnReceiptFacts>(receipt: ReturnReceiptFacts, receipts: readonly R[]): R[] {
  return receipts.filter(
    (other) => other.factId !== receipt.factId && other.warehouseCode === receipt.warehouseCode && other.trackingNumber === receipt.trackingNumber,
  );
}

/**
 * 整份预报一次对上各自的到仓记录，列表用。结果与逐张调 receiptsOfAdvice 一样（测试钉着），只是不做预报数乘到仓数次比较：
 * 按预报标识与「仓 + 退件单号」各建一份索引，每张预报只取这两处。
 */
export function receiptsByAdvice<A extends ReturnAdviceFacts, R extends ReturnReceiptFacts>(
  advices: readonly A[],
  receipts: readonly R[],
): Map<string, R[]> {
  const order = new Map<string, number>();
  const byAdvice = new Map<string, R[]>();
  const byParcel = new Map<string, R[]>();
  const add = (index: Map<string, R[]>, key: string, receipt: R) => {
    const bucket = index.get(key);
    if (bucket) bucket.push(receipt);
    else index.set(key, [receipt]);
  };
  receipts.forEach((receipt, position) => {
    order.set(receipt.factId, position);
    if (receipt.adviceId) add(byAdvice, receipt.adviceId, receipt);
    else add(byParcel, `${receipt.warehouseCode}\n${receipt.trackingNumber}`, receipt);
  });
  const matched = new Map<string, R[]>();
  for (const advice of advices) {
    const sameParcel = (byParcel.get(`${advice.warehouseCode}\n${advice.trackingNumber}`) ?? []).filter(
      (receipt) => !receipt.ownerAccount || receipt.ownerAccount === advice.ownerAccount,
    );
    const own = byAdvice.get(advice.adviceId) ?? [];
    matched.set(
      advice.adviceId,
      [...own, ...sameParcel].sort((a, b) => (order.get(a.factId) ?? 0) - (order.get(b.factId) ?? 0)),
    );
  }
  return matched;
}

// —— 预报的状态 ——

export type ReturnAdviceStatus = 'AWAITING_ARRIVAL' | 'ARRIVED' | 'DISPOSED';

/**
 * 没有到仓记录是待到仓；有到仓记录、其中入了暂存的都记过去向是已处置；其余是已到仓。转了待认领的那几笔不入库、不能处置，
 * 不拦已处置——它们的下文在待认领页。预报的商品行没到齐不影响状态：退货没有「收齐」这一说，预报也没有关闭，到了几行就是几行，
 * 差多少在预报页的商品行上写。
 */
export function returnAdviceStatus(receipts: ReadonlyArray<Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>>): ReturnAdviceStatus {
  if (receipts.length === 0) return 'AWAITING_ARRIVAL';
  const counted = receipts.filter((receipt) => receipt.outcome === 'COUNTED');
  return counted.length > 0 && counted.every((receipt) => receipt.disposition) ? 'DISPOSED' : 'ARRIVED';
}

export interface AdviceRow<A, R> {
  advice: A;
  receipts: readonly R[];
  status: ReturnAdviceStatus;
}

export function adviceRows<A extends ReturnAdviceFacts, R extends ReturnReceiptFacts>(
  advices: readonly A[],
  receipts: readonly R[],
): Array<AdviceRow<A, R>> {
  const matched = receiptsByAdvice(advices, receipts);
  return advices.map((advice) => {
    const own = matched.get(advice.adviceId) ?? [];
    return { advice, receipts: own, status: returnAdviceStatus(own) };
  });
}

// —— 到仓记录的状态 ——

export type ReturnReceiptStatus = 'AWAITING_DISPOSITION' | 'DISPOSED' | 'DIVERTED' | 'OTHER';

/**
 * 转待认领的那一笔自己不入账，认领后另起一笔退货入库，处置落在那一笔上，所以它既不是待处置也不是已处置。入了暂存的看有没有
 * 记过去向。读口以后多答一种到仓结果时落在「其他」，页面照它的原词写，不替它猜。
 */
export function returnReceiptStatus(receipt: Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>): ReturnReceiptStatus {
  if (receipt.outcome === 'UNCLAIMED') return 'DIVERTED';
  if (receipt.disposition) return 'DISPOSED';
  return receipt.outcome === 'COUNTED' ? 'AWAITING_DISPOSITION' : 'OTHER';
}

export function awaitsDisposition(receipt: Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>): boolean {
  return returnReceiptStatus(receipt) === 'AWAITING_DISPOSITION';
}

export interface ReceiptTally {
  total: number;
  awaiting: number;
  disposed: number;
  diverted: number;
}

export function receiptTally(receipts: ReadonlyArray<Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>>): ReceiptTally {
  const tally: ReceiptTally = { total: receipts.length, awaiting: 0, disposed: 0, diverted: 0 };
  for (const receipt of receipts) {
    const status = returnReceiptStatus(receipt);
    if (status === 'AWAITING_DISPOSITION') tally.awaiting += 1;
    else if (status === 'DISPOSED') tally.disposed += 1;
    else if (status === 'DIVERTED') tally.diverted += 1;
  }
  return tally;
}

// —— 进度条 ——

export const returnAdviceSteps: readonly TrackStep[] = [
  { id: 'ADVISED', label: '已预报' },
  { id: 'ARRIVED', label: '已到仓' },
  { id: 'DISPOSED', label: '已处置' },
];

/**
 * 待到仓与已到仓而还有货等处置的，都卡在自己这一步、等人动手，用待办色标出；已处置走到末一步。到了仓却一笔也没入暂存
 * （都转了待认领）的，货在待认领那边，这里只说一句，不画成走完。
 */
export function returnAdviceTrack(receipts: ReadonlyArray<Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>>): DocumentTrack {
  const status = returnAdviceStatus(receipts);
  if (status === 'AWAITING_ARRIVAL') return { steps: returnAdviceSteps, currentId: 'ADVISED', currentLabel: '已预报，待到仓' };
  if (status === 'DISPOSED') return { steps: returnAdviceSteps, currentId: 'DISPOSED' };
  const note = returnAdviceArrivalNote(receiptTally(receipts));
  return {
    steps: returnAdviceSteps,
    currentId: 'ARRIVED',
    currentLabel: note ? `已到仓，${note}` : undefined,
  };
}

/** 已到仓这一词盖住的那一句：还有几笔没记去向，或者货转去了待认领、库存里没有。待处置优先。 */
export function returnAdviceArrivalNote(tally: Pick<ReceiptTally, 'awaiting' | 'diverted'>): string | null {
  if (tally.awaiting > 0) return `${tally.awaiting} 笔待处置`;
  if (tally.diverted > 0) return '转了待认领';
  return null;
}

export const returnReceiptSteps: readonly TrackStep[] = [
  { id: 'ARRIVED', label: '已到仓' },
  { id: 'DISPOSED', label: '已处置' },
];

/** 认不出的转待认领，之后的认领、入库与处置都不在这笔记录上：换成「已到仓 → 转待认领」两步，停在第二步。 */
export const returnDivertedSteps: readonly TrackStep[] = [
  { id: 'ARRIVED', label: '已到仓' },
  { id: 'DIVERTED', label: '转待认领' },
];

export function returnReceiptTrack(receipt: Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>): DocumentTrack {
  switch (returnReceiptStatus(receipt)) {
    case 'AWAITING_DISPOSITION':
      return { steps: returnReceiptSteps, currentId: 'ARRIVED', currentLabel: '已到仓，待处置' };
    case 'DISPOSED':
      return { steps: returnReceiptSteps, currentId: 'DISPOSED' };
    case 'DIVERTED':
      return { steps: returnDivertedSteps, currentId: 'DIVERTED' };
    default:
      return { steps: returnReceiptSteps, currentId: 'ARRIVED', currentLabel: receipt.outcome };
  }
}

// —— 下一步 ——

export interface ReturnAdviceNextStep {
  label: string;
  /** 收退件在作业台做，页面只指路。 */
  moduleId: string;
}

/** 只有还没到仓的预报有下一步：去作业台收。到了仓的，下一步落在到仓记录上（可能不止一笔），预报自己不指。 */
export function returnAdviceNextStep(status: ReturnAdviceStatus): ReturnAdviceNextStep | null {
  return status === 'AWAITING_ARRIVAL' ? { label: '去作业台收退件', moduleId: 'warehouse-workstation' } : null;
}

export interface ReturnReceiptNextStep {
  label: string;
  /** 去向已记之后去上架与移库。处置当时不改库存，货还在退货暂存。 */
  station?: 'putaway';
}

/**
 * 入了暂存、还没记去向：就地处置。去向已记：去上架与移库把货移出退货暂存。
 * 转待认领的下一步在待认领页，读口没答它此刻在哪一步，这里不指。
 */
export function returnReceiptNextStep(status: ReturnReceiptStatus): ReturnReceiptNextStep | null {
  switch (status) {
    case 'AWAITING_DISPOSITION':
      return { label: '处置' };
    case 'DISPOSED':
      return { label: '移出退货暂存', station: 'putaway' };
    default:
      return null;
  }
}

/** 待到仓的预报在等货，从受理算起；别的状态不在等谁，答 undefined。 */
export function returnAdviceWaitingSince(status: ReturnAdviceStatus, advice: Pick<ReturnAdviceFacts, 'receivedAt'>): string | undefined {
  return status === 'AWAITING_ARRIVAL' ? advice.receivedAt : undefined;
}

/** 待处置的到仓记录在等人记去向，从到仓算起。 */
export function returnReceiptWaitingSince(receipt: Pick<ReturnReceiptFacts, 'outcome' | 'disposition' | 'occurredAt'>): string | undefined {
  return awaitsDisposition(receipt) ? receipt.occurredAt : undefined;
}

// —— 预报的商品行对到仓 ——

export interface AdviceLineRow {
  goodsCode: string;
  unit: string;
  /** 预报里写的，同一个商品同一个单位出现几行就加起来。 */
  expected: number;
  /** 入了暂存的到仓数量；转待认领的没入库，不算。 */
  arrived: number;
}

/**
 * 预报每个商品（同商品同单位合并）预计多少、到仓入暂存多少。预报里没写的商品到了也列出来，预计是 0——退回的货与预报不符是常有的，
 * 不能只列预报里写了的。
 */
export function adviceLineRows(
  advice: Pick<ReturnAdviceFacts, 'lines'>,
  receipts: ReadonlyArray<Pick<ReturnReceiptFacts, 'goodsCode' | 'unit' | 'quantity' | 'outcome'>>,
): AdviceLineRow[] {
  const rows = new Map<string, AdviceLineRow>();
  const rowOf = (goodsCode: string, unit: string): AdviceLineRow => {
    const key = `${goodsCode}\n${unit}`;
    let row = rows.get(key);
    if (!row) {
      row = { goodsCode, unit, expected: 0, arrived: 0 };
      rows.set(key, row);
    }
    return row;
  };
  for (const line of advice.lines ?? []) rowOf(line.goodsCode, line.unit).expected += line.quantity;
  for (const receipt of receipts) {
    if (receipt.outcome === 'COUNTED') rowOf(receipt.goodsCode, receipt.unit).arrived += receipt.quantity;
  }
  return [...rows.values()];
}

export interface LineNote {
  label: string;
  tone: 'success' | 'warning' | 'neutral';
}

/** 一行商品到了没有：预报里没有的、还没到的、还差几个、到齐、多到几个。 */
export function adviceLineNote(row: Pick<AdviceLineRow, 'expected' | 'arrived'>): LineNote {
  if (row.expected === 0) return { label: '预报里没有', tone: 'warning' };
  if (row.arrived === 0) return { label: '还没到', tone: 'neutral' };
  if (row.arrived < row.expected) return { label: `还差 ${row.expected - row.arrived}`, tone: 'warning' };
  if (row.arrived > row.expected) return { label: `多到 ${row.arrived - row.expected}`, tone: 'warning' };
  return { label: '到齐', tone: 'success' };
}

// —— 列表：筛选、数量、排序 ——

export interface ReturnFilter {
  keyword: string;
  /** 空串即不按这一维筛。 */
  owner: string;
  warehouse: string;
}

function includesKeyword(value: string | undefined, keyword: string): boolean {
  return (value ?? '').toLowerCase().includes(keyword);
}

function keywordOf(filter: ReturnFilter): string {
  return filter.keyword.trim().toLowerCase();
}

/** 预报是否落在检索词、货主与仓里。检索词不分大小写，按包含匹配。 */
export function adviceInFilter(advice: ReturnAdviceFacts, filter: ReturnFilter): boolean {
  if (filter.owner !== '' && advice.ownerAccount !== filter.owner) return false;
  if (filter.warehouse !== '' && advice.warehouseCode !== filter.warehouse) return false;
  const keyword = keywordOf(filter);
  if (keyword === '') return true;
  return [advice.adviceId, advice.documentNumber, advice.trackingNumber, advice.ownerAccount, advice.warehouseCode].some((value) =>
    includesKeyword(value, keyword),
  );
}

export function receiptInFilter(receipt: ReturnReceiptFacts, filter: ReturnFilter): boolean {
  if (filter.owner !== '' && receipt.ownerAccount !== filter.owner) return false;
  if (filter.warehouse !== '' && receipt.warehouseCode !== filter.warehouse) return false;
  const keyword = keywordOf(filter);
  if (keyword === '') return true;
  return [receipt.factId, receipt.trackingNumber, receipt.ownerAccount, receipt.goodsCode, receipt.warehouseCode].some((value) =>
    includesKeyword(value, keyword),
  );
}

/** 各状态各有几条。没出现的状态不在结果里，读的时候按 0。 */
export function countByStatus<T>(rows: readonly T[], statusOf: (row: T) => string): Readonly<Record<string, number>> {
  const counts: Record<string, number> = {};
  for (const row of rows) {
    const status = statusOf(row);
    counts[status] = (counts[status] ?? 0) + 1;
  }
  return counts;
}

/** newest 是新到旧，oldest 是等得最久的在前。 */
export type ReturnOrder = 'newest' | 'oldest';

export function isReturnOrder(value: string | null): value is ReturnOrder {
  return value === 'newest' || value === 'oldest';
}

// 时刻认不出的排在最老的一端：不知道就别把它排到最前面。
function instantMs(value: string): number {
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? 0 : ms;
}

export function sortAdviceRows<T extends { advice: Pick<ReturnAdviceFacts, 'adviceId' | 'receivedAt'> }>(rows: readonly T[], order: ReturnOrder): T[] {
  const sign = order === 'oldest' ? 1 : -1;
  return [...rows].sort(
    (a, b) => sign * (instantMs(a.advice.receivedAt) - instantMs(b.advice.receivedAt)) || a.advice.adviceId.localeCompare(b.advice.adviceId),
  );
}

/** 还没处置的排前，各组里再按到仓时刻。「没处置」只算待处置的：转待认领的不能在这条记录上处置，不该顶在最上面占位置。 */
export function sortReceipts<R extends Pick<ReturnReceiptFacts, 'factId' | 'occurredAt' | 'outcome' | 'disposition'>>(rows: readonly R[], order: ReturnOrder): R[] {
  const sign = order === 'oldest' ? 1 : -1;
  const rank = (row: R) => (awaitsDisposition(row) ? 0 : 1);
  return [...rows].sort(
    (a, b) => rank(a) - rank(b) || sign * (instantMs(a.occurredAt) - instantMs(b.occurredAt)) || a.factId.localeCompare(b.factId),
  );
}

// —— 字 ——

/** 回库处置的去向，词取自后端的四个去向词；「转次品」不叫通用词表里库存状态那个「次品」。陌生的原样交回。 */
export const returnDestinationLabels: Readonly<Record<string, string>> = {
  GOOD_PUTAWAY: '良品上架',
  DEFECTIVE: '转次品',
  DESTROY: '销毁',
  RETURN_TO_OWNER: '退还货主',
};

export function returnDestinationLabel(destination: string): string {
  return returnDestinationLabels[destination] ?? destination;
}

/** 没有预报号时说为什么：读口答的是原因词，不把它原样摆在界面上。 */
export function numberingAbsenceNote(absence: string | undefined): string {
  return absence === 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' ? '仓的业务时区没登记，没有给预报号' : '没有给预报号';
}

/** 登记人：货主自己提交的预报没有运营登记人，说是货主提交的，不留一个看不出缘由的空格。 */
export function adviceRegistrant(advice: { origin: string; operator?: string }): string {
  if (advice.operator) return advice.operator;
  return advice.origin === 'CUSTOMER' ? '货主自己提交' : '—';
}

/** 这笔到仓记录的处置情况，一句话：去向、待处置，或转待认领后另起一笔入库再处置。 */
export function returnDispositionText(receipt: Pick<ReturnReceiptFacts, 'outcome' | 'disposition'>): string {
  if (receipt.disposition) return returnDestinationLabel(receipt.disposition.destination);
  switch (returnReceiptStatus(receipt)) {
    case 'AWAITING_DISPOSITION':
      return '待处置';
    case 'DIVERTED':
      return '认领后另起一笔入库再处置';
    default:
      return '—';
  }
}
