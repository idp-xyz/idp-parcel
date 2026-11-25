import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { OutboundRequestDetailBody, OutboundRequestListBody, OutboundRequestRecord } from '../api';
import {
  checkableStatus,
  createOrderFinder,
  goodsOfTasks,
  judgeOpened,
  judgeOrder,
  loadPackQueue,
  MAX_ORDER_PAGES,
  openedOf,
  packableStatus,
  queueCount,
  waitingCounter,
  type OrderLookup,
  type OrderOpened,
  type PackOrder,
} from './pack-order';
import { legacyWorkstationHash, pickStationHash } from './stations';

const WAREHOUSE = 'SYN-WH-01';

function outbound(requestId: string, warehouseCode: string, status: string, documentNumber?: string): OutboundRequestRecord {
  return {
    requestId,
    ...(documentNumber === undefined ? {} : { documentNumber }),
    ownerAccount: 'SYN-OWNER-01',
    warehouseCode,
    origin: 'OWNER_API',
    requestKey: `KEY-${requestId}`,
    dispatch: 'PRODUCT_NETWORK',
    recipient: '收件人',
    address: '某地',
    status,
    platformStocking: false,
    receivedAt: '2026-11-25T08:00:00Z',
    lines: [],
    reservedQuantity: 0,
    wantedQuantity: 0,
  };
}

// —— 已拣的货 ——

test('已拣的货按（商品，单位）合计，短拣的按已拣的算，一件没拣到的任务不算，次序固定', () => {
  deepEqual(
    goodsOfTasks([
      { goodsCode: 'G-B', unit: '件', picked: 2 },
      { goodsCode: 'G-A', unit: '件', picked: 3 },
      { goodsCode: 'G-A', unit: '件', picked: 1 },
      { goodsCode: 'G-A', unit: '箱', picked: 4 },
      { goodsCode: 'G-C', unit: '件', picked: 0 },
    ]),
    [
      { goodsCode: 'G-A', unit: '件', quantity: 4 },
      { goodsCode: 'G-A', unit: '箱', quantity: 4 },
      { goodsCode: 'G-B', unit: '件', quantity: 2 },
    ],
  );
});

// —— 读明细的答复 ——

type Detail = ApiResult<OutboundRequestDetailBody>;

function detail(body: Partial<OutboundRequestDetailBody>): Detail {
  return { kind: 'outcome', status: 200, body: { outcome: 'OUTBOUND_REQUEST_FOUND', ...body } };
}

const task = (taskId: string, goodsCode: string, picked: number, status = 'PICKED') => ({
  taskId,
  waveId: 'W-1',
  locationCode: 'A-01',
  goodsCode,
  unit: '件',
  stockState: 'GOOD',
  inboundBatchId: 'B-1',
  quantity: picked,
  picked,
  status,
});

test('读明细：有单就收成工位要的样子；单据号写进屏上的单号，没有就写请求号', () => {
  const opened = openedOf(
    detail({
      request: outbound('OBR-1', WAREHOUSE, 'PICKED', 'DOC-001'),
      pickTasks: [task('T-1', 'G-A', 3), task('T-2', 'G-B', 2)],
    }),
  );
  equal(opened.kind, 'order');
  if (opened.kind !== 'order') return;
  equal(opened.order.requestId, 'OBR-1');
  equal(opened.order.number, 'DOC-001');
  equal(opened.order.ownerAccount, 'SYN-OWNER-01');
  equal(opened.order.warehouse, WAREHOUSE);
  equal(opened.order.dispatch, 'PRODUCT_NETWORK');
  equal(opened.order.status, 'PICKED');
  deepEqual(opened.order.goods, [
    { goodsCode: 'G-A', unit: '件', quantity: 3 },
    { goodsCode: 'G-B', unit: '件', quantity: 2 },
  ]);
  equal(opened.openTasks, false);
  equal(opened.order.lastMismatch, null);

  const bare = openedOf(detail({ request: outbound('OBR-2', WAREHOUSE, 'PICKED'), pickTasks: [task('T-1', 'G-A', 1)] }));
  ok(bare.kind === 'order' && bare.order.number === 'OBR-2');
});

test('读明细：还有没拣的任务（OPEN）要标出来；以前复核不符的次数与最近一次点的数取自没对上的那几笔', () => {
  const opened = openedOf(
    detail({
      request: outbound('OBR-1', WAREHOUSE, 'PICKED'),
      pickTasks: [task('T-1', 'G-A', 3), task('T-2', 'G-B', 0, 'OPEN')],
      checks: [
        { factId: 'F-1', observed: 4, matched: false, checkedAt: '2026-11-25T09:00:00Z' },
        { factId: 'F-2', observed: 2, matched: false, checkedAt: '2026-11-25T10:00:00Z' },
        { factId: 'F-3', observed: 3, matched: true, checkedAt: '2026-11-25T11:00:00Z' },
      ],
    }),
  );
  ok(opened.kind === 'order');
  if (opened.kind !== 'order') return;
  equal(opened.openTasks, true);
  deepEqual(opened.order.lastMismatch, { times: 2, observed: 2 });
});

test('读明细：答复里没有这张单是「问到了没有」，读不成是「没问成」，两样不混', () => {
  deepEqual(openedOf(detail({})), { kind: 'absent' });
  const failed = openedOf({ kind: 'transport', message: '断了' });
  equal(failed.kind, 'failed');
  ok(failed.kind === 'failed' && failed.reason.includes('网络没通'));
  const refused = openedOf({ kind: 'notGranted' });
  equal(refused.kind, 'failed');
});

// —— 这张单现在能不能做 ——

const facts = (status: string, warehouse = WAREHOUSE) => ({ requestId: 'OBR-1', number: 'DOC-001', warehouse, status });

test('已拣货先复核，已复核直接打包', () => {
  deepEqual(judgeOrder(facts(checkableStatus), WAREHOUSE), { kind: 'check' });
  deepEqual(judgeOrder(facts(packableStatus), WAREHOUSE), { kind: 'pack' });
});

test('别的仓的单：说它属于哪个仓、这台设备绑的是哪个仓', () => {
  const work = judgeOrder(facts('PICKED', 'SYN-WH-02'), WAREHOUSE);
  equal(work.kind, 'not-here');
  if (work.kind !== 'not-here') return;
  ok(work.text.includes('SYN-WH-02'));
  ok(work.text.includes(WAREHOUSE));
});

test('不能在这里做的状态，各说各的话、各指各的去处，没有两句话相同', () => {
  const statuses = [
    'RESERVED',
    'PARTIALLY_RESERVED',
    'PENDING_POLICY',
    'ALLOCATED',
    'PACKED',
    'SHIPPED',
    'CANCELLED',
    'PARTIALLY_CANCELLED',
    'AWAITING_RETURN',
    'REJECTED',
    'SPLIT',
    'HANDED_OFF',
    'FORWARDED',
    'SOMETHING_NEW',
  ];
  const texts = new Set<string>();
  for (const status of statuses) {
    const work = judgeOrder(facts(status), WAREHOUSE);
    equal(work.kind, 'not-here', status);
    if (work.kind !== 'not-here') continue;
    ok(work.text.includes('DOC-001'), `${status} 的话里要有单号`);
    ok(work.goto !== undefined && work.goto.label !== '' && work.goto.hash.startsWith('#/'), `${status} 要指个去处`);
    ok(!/[A-Z]{3,}/.test(work.text.replace('DOC-001', '')), `${status} 的话里不带英文状态码：${work.text}`);
    texts.add(work.text);
  }
  equal(texts.size, statuses.length - 1, '只有代理仓的两个状态共用一句，别的各说各的，不认识的状态也有自己的一句');
});

test('去处：还没拣完的去拣货工位，已打包的去出库发货，已截单等回库的去回库，代理仓的去代理仓履约', () => {
  const gotoOf = (status: string) => {
    const work = judgeOrder(facts(status), WAREHOUSE);
    return work.kind === 'not-here' ? work.goto : undefined;
  };
  equal(gotoOf('ALLOCATED')?.hash, pickStationHash());
  equal(gotoOf('PACKED')?.hash, legacyWorkstationHash);
  equal(gotoOf('AWAITING_RETURN')?.hash, legacyWorkstationHash);
  equal(gotoOf('HANDED_OFF')?.hash, '#/warehouse-agents');
  ok(gotoOf('SHIPPED')?.hash.includes('OBR-1'));
});

function opened(order: Partial<PackOrder> = {}, openTasks = false): Extract<OrderOpened, { kind: 'order' }> {
  return {
    kind: 'order',
    openTasks,
    order: {
      requestId: 'OBR-1',
      number: 'DOC-001',
      ownerAccount: 'SYN-OWNER-01',
      warehouse: WAREHOUSE,
      dispatch: 'PRODUCT_NETWORK',
      status: 'PICKED',
      goods: [{ goodsCode: 'G-A', unit: '件', quantity: 3 }],
      lastMismatch: null,
      ...order,
    },
  };
}

test('读到明细后再判一遍：已拣货却还有没拣的任务、一件都没拣到的单，不能复核打包', () => {
  deepEqual(judgeOpened(opened(), WAREHOUSE), { kind: 'check' });
  deepEqual(judgeOpened(opened({ status: 'CHECKED' }), WAREHOUSE), { kind: 'pack' });

  const open = judgeOpened(opened({}, true), WAREHOUSE);
  equal(open.kind, 'not-here');
  ok(open.kind === 'not-here' && open.text.includes('没拣完') && open.goto?.hash === pickStationHash());

  const empty = judgeOpened(opened({ goods: [] }), WAREHOUSE);
  equal(empty.kind, 'not-here');
  ok(empty.kind === 'not-here' && empty.text.includes('没有拣出任何货'));

  const shipped = judgeOpened(opened({ status: 'SHIPPED' }), WAREHOUSE);
  equal(shipped.kind, 'not-here');
  const elsewhere = judgeOpened(opened({ warehouse: 'SYN-WH-02' }), WAREHOUSE);
  equal(elsewhere.kind, 'not-here');
});

// —— 按扫到的码找单：照 Go 端读口的读法造的假名册 ——

interface Call {
  conditions: CatalogueConditions;
  after: string | null;
}

type ListAnswer = ApiResult<OutboundRequestListBody>;

// 出库单读口声明的筛选维是 warehouseCode 与 status，排序维是 receivedAt；不认的键整个请求答 400。
const declaredFilters = ['warehouseCode', 'status'];
const badRequest: ListAnswer = { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' };

const contains = (keyword: string | undefined, columns: readonly (string | undefined)[]): boolean =>
  keyword === undefined || keyword === '' || columns.some((column) => (column ?? '').toLowerCase().includes(keyword.toLowerCase()));

function fakeOutbound(rows: OutboundRequestRecord[], options: { pageSize?: number; fail?: (nth: number) => ListAnswer | undefined } = {}) {
  const pageSize = options.pageSize ?? 200;
  const calls: Call[] = [];
  const read = async (conditions: CatalogueConditions, after: string | null): Promise<ListAnswer> => {
    calls.push({ conditions, after });
    const failed = options.fail?.(calls.length);
    if (failed !== undefined) return failed;
    if (Object.keys(conditions.filters ?? {}).some((key) => !declaredFilters.includes(key))) return badRequest;
    if (conditions.selectors !== undefined && Object.keys(conditions.selectors).length > 0) return badRequest;
    if (conditions.sort !== undefined && conditions.sort !== 'receivedAt') return badRequest;
    const wanted = (key: string, value: string) => {
      const allowed = conditions.filters?.[key];
      return allowed === undefined || allowed.length === 0 || allowed.includes(value);
    };
    const matching = rows
      .filter(
        (row) =>
          wanted('warehouseCode', row.warehouseCode) &&
          wanted('status', row.status) &&
          contains(conditions.q, [row.requestId, row.documentNumber, row.ownerAccount, row.warehouseCode, row.recipient, row.requestKey]),
      )
      .sort((left, right) => (left.requestId < right.requestId ? -1 : 1));
    const start = after === null ? 0 : Number(after);
    const page = { size: pageSize, next: start + pageSize < matching.length ? String(start + pageSize) : null, total: matching.length };
    return { kind: 'outcome', status: 200, body: { outcome: 'OUTBOUND_REQUESTS_LISTED', requests: matching.slice(start, start + pageSize), page } };
  };
  return { read, calls };
}

async function find(rows: OutboundRequestRecord[], code: string, options: Parameters<typeof fakeOutbound>[1] & { maxPages?: number } = {}) {
  const { read, calls } = fakeOutbound(rows, options);
  const lookup = await createOrderFinder(read, options.maxPages).find(WAREHOUSE, code);
  return { lookup, calls };
}

const orders = [
  outbound('OBR-1', WAREHOUSE, 'PICKED', 'DOC-001'),
  outbound('OBR-10', WAREHOUSE, 'CHECKED', 'DOC-010'),
  outbound('OBR-2', WAREHOUSE, 'PACKED'),
  outbound('OBR-OTHER', 'SYN-WH-02', 'PICKED', 'DOC-OTHER'),
];

test('找单：请求号与单据号都认；只发一次读（本仓有就不必再问别的仓），带仓筛选与这个码', async () => {
  const byRequest = await find(orders, 'OBR-1');
  equal(byRequest.lookup.kind, 'found');
  ok(byRequest.lookup.kind === 'found' && byRequest.lookup.row.requestId === 'OBR-1');
  deepEqual(
    byRequest.calls.map((call) => call.conditions),
    [{ filters: { warehouseCode: [WAREHOUSE] }, q: 'OBR-1' }],
  );
  const byNumber = await find(orders, 'DOC-010');
  ok(byNumber.lookup.kind === 'found' && byNumber.lookup.row.requestId === 'OBR-10');
});

test('找单：读口按字面包含，认的是号完全相同的那一行，OBR-1 不会错认成 OBR-10', async () => {
  const lookup = (await find([orders[1]], 'OBR-1')).lookup;
  equal(lookup.kind, 'absent');
  const lower = (await find(orders, 'obr-1')).lookup;
  equal(lower.kind, 'absent', '大小写不同不是同一个号');
});

test('找单：含这个码的行比一页多，目标在后面的页上也认得出，沿游标往下翻', async () => {
  // 读面按请求号排，目标排在最后：前面五行都含 OBR-9、却都不是它
  const many = [...Array.from({ length: 5 }, (_, index) => outbound(`A${index}OBR-9`, WAREHOUSE, 'PICKED')), outbound('OBR-9', WAREHOUSE, 'PICKED')];
  const { lookup, calls } = await find(many, 'OBR-9', { pageSize: 2 });
  equal(lookup.kind, 'found');
  ok(lookup.kind === 'found' && lookup.row.requestId === 'OBR-9');
  deepEqual(
    calls.map((call) => call.after),
    [null, '2', '4'],
  );
});

test('找单：登记在别的仓的，说它在哪个仓；本仓问完了才去不带仓筛选问第二次', async () => {
  const { lookup, calls } = await find(orders, 'DOC-OTHER');
  deepEqual(lookup, { kind: 'elsewhere', warehouse: 'SYN-WH-02', number: 'DOC-OTHER' });
  deepEqual(
    calls.map((call) => call.conditions),
    [
      { filters: { warehouseCode: [WAREHOUSE] }, q: 'DOC-OTHER' },
      { q: 'DOC-OTHER' },
    ],
  );
});

test('找单：「没有」只在本仓与别的仓都问完才说', async () => {
  const { lookup, calls } = await find(orders, 'OBR-404');
  deepEqual(lookup, { kind: 'absent' });
  equal(calls.length, 2);
});

test('找单：读不成不说成没有，各种没问成的原因都带到话里', async () => {
  const unread = async (answer: ListAnswer, nth = 1): Promise<OrderLookup> =>
    (await find(orders, 'OBR-404', { fail: (call) => (call === nth ? answer : undefined) })).lookup;

  const down = await unread({ kind: 'transport', message: '断了' });
  equal(down.kind, 'unverified');
  ok(down.kind === 'unverified' && down.reason.includes('读出库单没成') && down.reason.includes('网络没通'));

  const noAnswer = await unread({ kind: 'noAnswer', status: 503, code: 'UNAVAILABLE' });
  equal(noAnswer.kind, 'unverified');

  const second = await unread({ kind: 'loginRequired' }, 2);
  equal(second.kind, 'unverified', '本仓问完没有、别的仓那一问失败，也不能说没有');

  const shape = await unread({ kind: 'outcome', status: 200, body: { outcome: 'X' } as unknown as OutboundRequestListBody });
  equal(shape.kind, 'unverified');
  ok(shape.kind === 'unverified' && shape.reason.includes('不是出库单列表的样子'));
});

test('找单：翻到页数上限还没认出就说没问完，不说没有', async () => {
  const many = Array.from({ length: 12 }, (_, index) => outbound(`OBR-7${String(index).padStart(2, '0')}`, WAREHOUSE, 'PICKED'));
  const { lookup, calls } = await find(many, 'OBR-7', { pageSize: 2, maxPages: 3 });
  equal(lookup.kind, 'unverified');
  ok(lookup.kind === 'unverified' && lookup.reason.includes('翻了 3 页'));
  equal(calls.length, 3);
  equal(MAX_ORDER_PAGES, 10);
});

test('找单：码比读口收的检索词（100 个字符）还长，一次读都不发，直说没法找', async () => {
  const { lookup, calls } = await find(orders, 'X'.repeat(101));
  equal(lookup.kind, 'unverified');
  ok(lookup.kind === 'unverified' && lookup.reason.includes('101'));
  equal(calls.length, 0);
  const edge = await find(orders, 'X'.repeat(100));
  equal(edge.lookup.kind, 'absent');
});

// —— 队列 ——

test('队列：本仓已拣货与已复核各读一份，早受理的在前；总数取读口答的总数', async () => {
  const rows = [
    outbound('OBR-A', WAREHOUSE, 'PICKED'),
    outbound('OBR-B', WAREHOUSE, 'PICKED'),
    outbound('OBR-C', WAREHOUSE, 'CHECKED'),
    outbound('OBR-D', 'SYN-WH-02', 'PICKED'),
    outbound('OBR-E', WAREHOUSE, 'PACKED'),
  ];
  const { read, calls } = fakeOutbound(rows);
  const answer = await loadPackQueue({ warehouse: WAREHOUSE, read });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  deepEqual(
    answer.queue.checkable.rows.map((row) => row.requestId),
    ['OBR-A', 'OBR-B'],
  );
  deepEqual(
    answer.queue.packable.rows.map((row) => row.requestId),
    ['OBR-C'],
  );
  equal(queueCount(answer.queue.checkable), 2);
  equal(queueCount(answer.queue.packable), 1);
  deepEqual(
    calls.map((call) => call.conditions),
    [
      { filters: { warehouseCode: [WAREHOUSE], status: ['PICKED'] }, sort: 'receivedAt' },
      { filters: { warehouseCode: [WAREHOUSE], status: ['CHECKED'] }, sort: 'receivedAt' },
    ],
  );
  equal(waitingCounter(answer.queue), '已拣完、等复核的出库单 2 张；已复核、等打包的 1 张');
});

test('队列：比一页长时屏上的张数仍是读口答的总数，不是这一页的行数', async () => {
  const many = Array.from({ length: 5 }, (_, index) => outbound(`OBR-${index}`, WAREHOUSE, 'PICKED'));
  const { read } = fakeOutbound(many, { pageSize: 2 });
  const answer = await loadPackQueue({ warehouse: WAREHOUSE, read });
  ok(answer.kind === 'queue');
  if (answer.kind !== 'queue') return;
  equal(answer.queue.checkable.rows.length, 2);
  equal(queueCount(answer.queue.checkable), 5);
});

test('队列：读口没给总数时用读到的行数，不去猜', () => {
  equal(queueCount({ rows: [outbound('A', WAREHOUSE, 'PICKED')], total: null }), 1);
});

test('队列：任一份读不成就整个答没读成，把那一次的失败带出去；读之前的提示是「正在读」', async () => {
  const { read } = fakeOutbound([], { fail: (nth) => (nth === 2 ? { kind: 'transport', message: '断了' } : undefined) });
  const answer = await loadPackQueue({ warehouse: WAREHOUSE, read });
  equal(answer.kind, 'failed');
  ok(answer.kind === 'failed' && answer.failure.kind === 'transport');
  ok(waitingCounter(null).includes('正在读'));
});
