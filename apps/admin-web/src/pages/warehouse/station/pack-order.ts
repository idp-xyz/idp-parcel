// 复核打包工位认一张出库单（纯 TypeScript，node:test 直接测，读口作参数传入）：扫到的码是哪张单、这张单现在能不能在这个工位上做、
// 该复核与该装箱的货是多少、队列上有哪几张在等。
//
// 到 Go 端核过的规则（复核与打包是 application.WaveHandler.Check 与 Pack）：
//   - 复核只收已拣货（PICKED）的单：别的状态答 STATUS_MISMATCH；任务里还有没拣的（OPEN）也答它。复核的实点数是一个总数，
//     要等于这张单所有拣货任务已拣数量的合计（短拣的按已拣的算，不按应拣的算），且大于 0，才算对上；多了少了都是「不符」。
//   - 打包只收已复核（CHECKED）的单，同样按已拣合计对账。
// 所以屏上「该有几件」是已拣的件数，不是出库单上要的件数。
//
// 出库单号：一张单有系统签发的出库请求号（读口按它取单，写口的载荷也认它），也可能有单据号（单据页的标题、屏上写的就是它）。
// 扫到哪一个都认，写进载荷的永远是请求号。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import { documentHash } from '../document-address';
import {
  listOutboundRequests,
  type OutboundRequestDetailBody,
  type OutboundRequestListBody,
  type OutboundRequestRecord,
} from '../api';
import { seek, type SeekWords } from './catalogue-seek';
import { dispatchStationHash, legacyWorkstationHash, pickStationHash } from './stations';
import { describeReadFailure } from './work-fact-submit';

/** 这个工位要认的两种状态：已拣货等复核，已复核等打包。 */
export const checkableStatus = 'PICKED';
export const packableStatus = 'CHECKED';

export type OrderReader = (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<OutboundRequestListBody>>;

// —— 这张单该有的货 ——

export interface PackGoods {
  goodsCode: string;
  unit: string;
  /** 已拣的件数合计：复核点的、打包装的都以它为准。 */
  quantity: number;
}

/** 在工位上做的这一张单。 */
export interface PackOrder {
  requestId: string;
  /** 屏上写的单号：有单据号写单据号，没有写出库请求号。 */
  number: string;
  ownerAccount: string;
  warehouse: string;
  dispatch: string;
  status: string;
  goods: readonly PackGoods[];
  /** 这张单以前复核过几次不符、最近一次点的是几件；没有为 null。 */
  lastMismatch: { times: number; observed: number } | null;
}

export const goodsKey = (goods: { goodsCode: string; unit: string }): string => `${goods.goodsCode}\u0000${goods.unit}`;

export type OrderOpened =
  | { kind: 'order'; order: PackOrder; openTasks: boolean }
  /** 问到了，没有这张单。 */
  | { kind: 'absent' }
  /** 没问成。 */
  | { kind: 'failed'; reason: string };

/** 按拣货任务把已拣的货按（商品，单位）合计；一件都没拣到的任务不算。 */
export function goodsOfTasks(tasks: ReadonlyArray<{ goodsCode: string; unit: string; picked: number }>): PackGoods[] {
  const sums = new Map<string, PackGoods>();
  for (const task of tasks) {
    if (!(task.picked > 0)) continue;
    const key = goodsKey(task);
    const known = sums.get(key);
    if (known === undefined) sums.set(key, { goodsCode: task.goodsCode, unit: task.unit, quantity: task.picked });
    else known.quantity += task.picked;
  }
  return [...sums.values()].sort((left, right) => (left.goodsCode === right.goodsCode ? compareText(left.unit, right.unit) : compareText(left.goodsCode, right.goodsCode)));
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

/** 出库单明细读口的答复收成工位要的样子；答复里没有这张单是问到了没有，读不成是没问成。 */
export function openedOf(answer: ApiResult<OutboundRequestDetailBody>): OrderOpened {
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
  const request = answer.body.request;
  if (request === undefined || request === null) return { kind: 'absent' };
  const tasks = Array.isArray(answer.body.pickTasks) ? answer.body.pickTasks : [];
  const checks = (Array.isArray(answer.body.checks) ? answer.body.checks : []).filter((check) => !check.matched);
  const last = checks.length === 0 ? null : [...checks].sort((left, right) => compareText(left.checkedAt, right.checkedAt))[checks.length - 1];
  return {
    kind: 'order',
    openTasks: tasks.some((task) => task.status === 'OPEN'),
    order: {
      requestId: request.requestId,
      number: request.documentNumber ? request.documentNumber : request.requestId,
      ownerAccount: request.ownerAccount,
      warehouse: request.warehouseCode,
      dispatch: request.dispatch,
      status: request.status,
      goods: goodsOfTasks(tasks),
      lastMismatch: last === null ? null : { times: checks.length, observed: last.observed },
    },
  };
}

// —— 这张单现在能不能做 ——

export interface Goto {
  label: string;
  hash: string;
}

export type OrderWork =
  /** 已拣货：先复核。 */
  | { kind: 'check' }
  /** 已复核：直接打包。 */
  | { kind: 'pack' }
  /** 这个工位不做：说为什么，能说去哪就说去哪。 */
  | { kind: 'not-here'; text: string; goto?: Goto };

const outboundDocument = (requestId: string): Goto => ({ label: '看这张出库单', hash: documentHash('warehouse-outbound', [requestId]) });

/** 一张单在别的状态上该说什么：一格一句，说清它现在在哪、下一步是什么。 */
function notHere(number: string, status: string, requestId: string): { text: string; goto?: Goto } {
  switch (status) {
    case 'RESERVED':
      return { text: `${number} 还没拣货：它已预占，还没放进波次。`, goto: outboundDocument(requestId) };
    case 'PARTIALLY_RESERVED':
      return { text: `${number} 还没拣货：库存不够，要等补货或拆单。`, goto: outboundDocument(requestId) };
    case 'PENDING_POLICY':
      return { text: `${number} 还没拣货：在等登记缺货处置。`, goto: outboundDocument(requestId) };
    case 'ALLOCATED':
      return { text: `${number} 还没拣完：它还有拣货任务没做。`, goto: { label: '去拣货', hash: pickStationHash() } };
    case 'PACKED':
      return { text: `${number} 已经打包了，不用再复核打包。下一步是出库确认。`, goto: { label: '去发货', hash: dispatchStationHash(requestId) } };
    case 'SHIPPED':
      return { text: `${number} 已经出库了。`, goto: outboundDocument(requestId) };
    case 'CANCELLED':
      return { text: `${number} 已经取消了。`, goto: outboundDocument(requestId) };
    case 'PARTIALLY_CANCELLED':
      return { text: `${number} 有包裹已经截单取消，不在复核打包里做。`, goto: outboundDocument(requestId) };
    case 'AWAITING_RETURN':
      return { text: `${number} 已经截单，拣出来的货在等回库。`, goto: { label: '去回库', hash: legacyWorkstationHash } };
    case 'REJECTED':
      return { text: `${number} 已经被拒单。`, goto: outboundDocument(requestId) };
    case 'SPLIT':
      return { text: `${number} 已经拆成了新单，货在拆出来的新单上。`, goto: outboundDocument(requestId) };
    case 'HANDED_OFF':
    case 'FORWARDED':
      return { text: `${number} 在代理仓，不在自营仓里复核打包。`, goto: { label: '去代理仓履约', hash: '#/warehouse-agents' } };
    default:
      return { text: `${number} 现在的状态既不是等复核，也不是等打包。`, goto: outboundDocument(requestId) };
  }
}

export interface OrderFacts {
  requestId: string;
  number: string;
  warehouse: string;
  status: string;
}

/** 单据号、请求号、仓、状态：只凭这几样判这张单在这个工位上该做什么。 */
export function judgeOrder(facts: OrderFacts, deviceWarehouse: string): OrderWork {
  if (facts.warehouse !== deviceWarehouse) {
    return { kind: 'not-here', text: `${facts.number} 属于仓 ${facts.warehouse}，这台设备绑的是仓 ${deviceWarehouse}。` };
  }
  if (facts.status === checkableStatus) return { kind: 'check' };
  if (facts.status === packableStatus) return { kind: 'pack' };
  return { kind: 'not-here', ...notHere(facts.number, facts.status, facts.requestId) };
}

/** 读到明细之后再判一遍：明细的状态与任务比列表行新；已拣货却还有没拣的任务、一件都没拣到的单，也不能复核。 */
export function judgeOpened(opened: Extract<OrderOpened, { kind: 'order' }>, deviceWarehouse: string): OrderWork {
  const { order } = opened;
  const base = judgeOrder(order, deviceWarehouse);
  if (base.kind === 'not-here') return base;
  if (opened.openTasks) {
    return {
      kind: 'not-here',
      text: `${order.number} 还有拣货任务没拣完，现在不能复核打包。`,
      goto: { label: '去拣货', hash: pickStationHash() },
    };
  }
  if (order.goods.length === 0) {
    return { kind: 'not-here', text: `${order.number} 没有拣出任何货，没有东西可复核打包。`, goto: outboundDocument(order.requestId) };
  }
  return base;
}

// —— 按扫到的码找单 ——

export type OrderLookup =
  | { kind: 'found'; row: OutboundRequestRecord }
  /** 登记在别的仓。 */
  | { kind: 'elsewhere'; warehouse: string; number: string }
  /** 这个仓里问完没有，别的仓里也问完没有。 */
  | { kind: 'absent' }
  /** 没问成，说不出有没有。 */
  | { kind: 'unverified'; reason: string };

export interface OrderFinder {
  find(warehouse: string, code: string): Promise<OrderLookup>;
}

/** 一次定向的读最多翻几页（每页读口至多 200 行）。 */
export const MAX_ORDER_PAGES = 10;

// 读口对检索词的长度上限（后端 cataloguepage 的 MaxKeywordRunes），按字符计；超过它整个请求答 400，这里就不问了。
const keywordLimit = 100;

const orderWords: SeekWords = {
  failed: (cause) => `读出库单没成：${cause}`,
  shape: '读出库单得到的答复不是出库单列表的样子',
  tooMany: (maxPages) => `出库单里含这个码的行太多，翻了 ${maxPages} 页还没认出它`,
};

const numberOf = (row: { requestId: string; documentNumber?: string }): string => (row.documentNumber ? row.documentNumber : row.requestId);

/**
 * 扫到的码是出库请求号或单据号。读口按仓筛（筛选维 warehouseCode）、检索词 q 填这个码：q 在读面上是对请求号、单据号、货主、仓、
 * 收件人与来源键的字面包含、不分大小写，所以要在行里认出请求号或单据号与码完全相同的那一行，这一页没有就沿游标翻。本仓里没有，再不带
 * 仓筛问一次，认「登记在别的仓」与「哪里都没有」；只有两处都问完了才说没有。
 */
export function createOrderFinder(read: OrderReader, maxPages: number = MAX_ORDER_PAGES): OrderFinder {
  return {
    async find(warehouse, code) {
      const length = [...code].length;
      if (length > keywordLimit) {
        return {
          kind: 'unverified',
          reason: `这个码有 ${length} 个字符，比读口收的检索词（最多 ${keywordLimit} 个字符）还长，没法去出库单里找它`,
        };
      }
      const matches = (row: OutboundRequestRecord) => row.requestId === code || row.documentNumber === code;
      const here = await seek<OutboundRequestListBody, OutboundRequestRecord>({
        read: (after) => read({ filters: { warehouseCode: [warehouse] }, q: code }, after),
        rowsOf: (body) => body.requests,
        isIt: (row) => row.warehouseCode === warehouse && matches(row),
        words: orderWords,
        maxPages,
      });
      if (here.kind === 'unread') return { kind: 'unverified', reason: here.reason };
      if (here.kind === 'found') return { kind: 'found', row: here.row };
      const elsewhere = await seek<OutboundRequestListBody, OutboundRequestRecord>({
        read: (after) => read({ q: code }, after),
        rowsOf: (body) => body.requests,
        isIt: (row) => row.warehouseCode !== warehouse && matches(row),
        words: orderWords,
        maxPages,
      });
      if (elsewhere.kind === 'unread') return { kind: 'unverified', reason: elsewhere.reason };
      if (elsewhere.kind === 'found') return { kind: 'elsewhere', warehouse: elsewhere.row.warehouseCode, number: numberOf(elsewhere.row) };
      return { kind: 'absent' };
    },
  };
}

/** 接真读口的那一份。 */
export function liveOrderFinder(): OrderFinder {
  return createOrderFinder(listOutboundRequests);
}

// —— 队列：这个仓里等复核与等打包的单 ——

export interface QueueList {
  rows: readonly OutboundRequestRecord[];
  /** 读口答的总数；它不给时是 null，不去猜。 */
  total: number | null;
}

export interface PackQueue {
  checkable: QueueList;
  packable: QueueList;
}

export type PackQueueAnswer =
  | { kind: 'queue'; queue: PackQueue }
  | { kind: 'failed'; failure: Exclude<ApiResult<OutboundRequestListBody>, { kind: 'outcome' }> };

function listOf(body: OutboundRequestListBody): QueueList {
  const total = body.page?.total;
  return { rows: Array.isArray(body.requests) ? body.requests : [], total: typeof total === 'number' ? total : null };
}

/**
 * 读两份队列：本仓已拣货的、已复核的，各取第一页，早受理的在前。读口声明的筛选维是 warehouseCode 与 status，排序维是 receivedAt。
 * 总数取读口答的 page.total，所以队列比一页长时屏上的张数仍是真的。
 */
export async function loadPackQueue(input: { warehouse: string; read: OrderReader }): Promise<PackQueueAnswer> {
  const ask = (status: string) => input.read({ filters: { warehouseCode: [input.warehouse], status: [status] }, sort: 'receivedAt' }, null);
  const [checkable, packable] = await Promise.all([ask(checkableStatus), ask(packableStatus)]);
  if (checkable.kind !== 'outcome') return { kind: 'failed', failure: checkable };
  if (packable.kind !== 'outcome') return { kind: 'failed', failure: packable };
  return { kind: 'queue', queue: { checkable: listOf(checkable.body), packable: listOf(packable.body) } };
}

/** 屏上说有几张：读口给了总数就用总数，没给用读到的行数；只读到一页而还有更多时，行数不冒充总数。 */
export function queueCount(list: QueueList): number {
  return list.total ?? list.rows.length;
}

/** 等扫出库单号时那一行小字：「已拣完、等复核的出库单 4 张，已复核、等打包的 2 张」。 */
export function waitingCounter(queue: PackQueue | null): string {
  if (queue === null) return '正在读本仓等复核与等打包的出库单…';
  return `已拣完、等复核的出库单 ${queueCount(queue.checkable)} 张；已复核、等打包的 ${queueCount(queue.packable)} 张`;
}
