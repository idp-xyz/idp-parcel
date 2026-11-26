import { deepEqual, equal } from 'node:assert/strict';
import { test } from 'node:test';
import type { ApiResult } from '../../catalogue-api';
import type { OutboundRequestRecord, ToteListBody, ToteRecord } from '../api';
import type { OrderFinder, OrderLookup } from './pack-order';
import { withToteLookup } from './pack-tote';
import type { ToteReader } from './pick-totes';

const order = { requestId: 'OBR-A', documentNumber: 'OB-1', warehouseCode: 'SYN-WH-01', status: 'PICKED' } as OutboundRequestRecord;

function orders(known: Record<string, OrderLookup>, asked: string[]): OrderFinder {
  return {
    find: async (_warehouse, code) => {
      asked.push(code);
      return known[code] ?? { kind: 'absent' };
    },
  };
}

const tote = (code: string, state: string, requestId?: string): ToteRecord => ({
  warehouseCode: 'SYN-WH-01',
  toteCode: code,
  state,
  ...(requestId === undefined ? {} : { requestId }),
  registeredBy: 'op',
  registeredAt: '2026-11-26T00:00:00Z',
});

function totes(rows: ToteRecord[] | 'fail', asked: string[] = []): ToteReader {
  return async (conditions) => {
    asked.push(`${conditions.filters?.warehouseCode?.[0]}/${conditions.q}`);
    if (rows === 'fail') return { kind: 'unreachable' } as unknown as ApiResult<ToteListBody>;
    return { kind: 'outcome', status: 200, body: { outcome: 'TOTES_LISTED', totes: rows, page: { size: 50, next: null, total: rows.length } } };
  };
}

// Covers: 出库单号优先；当单号找得到就不去问周转箱名册。
test('扫到的是出库单号时照原样找单，不问箱', async () => {
  const askedOrders: string[] = [];
  const askedTotes: string[] = [];
  const finder = withToteLookup(orders({ 'OB-1': { kind: 'found', row: order } }, askedOrders), totes([], askedTotes));
  deepEqual(await finder.find('SYN-WH-01', 'OB-1'), { kind: 'found', row: order });
  deepEqual(askedTotes, []);
});

// Covers: 当单号找不到、本仓有一个箱号完全相同的在用的箱：按它绑着的出库请求号再找一次单。箱号只是包含这个码的不算。
test('扫到在用的周转箱，找到它绑着的那张单', async () => {
  const askedOrders: string[] = [];
  const askedTotes: string[] = [];
  const finder = withToteLookup(
    orders({ 'OBR-A': { kind: 'found', row: order } }, askedOrders),
    totes([tote('TOTE-10', 'IN_USE', 'OBR-Z'), tote('TOTE-1', 'IN_USE', 'OBR-A')], askedTotes),
  );
  deepEqual(await finder.find('SYN-WH-01', 'TOTE-1'), { kind: 'found', row: order });
  deepEqual(askedOrders, ['TOTE-1', 'OBR-A']);
  deepEqual(askedTotes, ['SYN-WH-01/TOTE-1']);
});

// Covers: 箱空着或已停用，如实说这是个没绑单的箱；名册里也没有这个号，仍是没有这张单；名册没读成说没核成，不说没有。
test('空着的箱、没有的码、没读成的名册各说各的', async () => {
  const none = orders({}, []);
  deepEqual(await withToteLookup(none, totes([tote('TOTE-1', 'FREE')])).find('SYN-WH-01', 'TOTE-1'), { kind: 'emptyTote', state: 'FREE' });
  deepEqual(await withToteLookup(none, totes([tote('TOTE-1', 'RETIRED')])).find('SYN-WH-01', 'TOTE-1'), { kind: 'emptyTote', state: 'RETIRED' });
  deepEqual(await withToteLookup(none, totes([tote('TOTE-10', 'IN_USE', 'OBR-A')])).find('SYN-WH-01', 'TOTE-1'), { kind: 'absent' });
  const unread = await withToteLookup(none, totes('fail')).find('SYN-WH-01', 'TOTE-1');
  equal(unread.kind, 'unverified');
});

// Covers: 当单号找的结果不是「没有」（在别的仓、没核成）时原样交回，不拿箱去盖它。
test('单在别的仓或没核成时不再问箱', async () => {
  const askedTotes: string[] = [];
  const elsewhere: OrderLookup = { kind: 'elsewhere', warehouse: 'SYN-WH-02', number: 'OB-9' };
  deepEqual(await withToteLookup(orders({ X: elsewhere }, []), totes([], askedTotes)).find('SYN-WH-01', 'X'), elsewhere);
  const unverified: OrderLookup = { kind: 'unverified', reason: '读口没通' };
  deepEqual(await withToteLookup(orders({ X: unverified }, []), totes([], askedTotes)).find('SYN-WH-01', 'X'), unverified);
  deepEqual(askedTotes, []);
});
