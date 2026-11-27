import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { OutboundRequestListBody, OutboundRequestRecord } from '../api';
import { dispatchQueueNote, loadDispatchQueue } from './dispatch-queue';

function row(requestId: string): OutboundRequestRecord {
  return {
    requestId,
    ownerAccount: 'SYN-OWNER-01',
    warehouseCode: 'SYN-WH-01',
    origin: 'ON_BEHALF',
    requestKey: requestId,
    dispatch: 'PRODUCT_NETWORK',
    recipient: '收件人',
    address: '地址',
    status: 'PACKED',
    platformStocking: false,
    receivedAt: '2026-11-20T06:00:00Z',
    lines: [],
    reservedQuantity: 0,
    wantedQuantity: 1,
  };
}

function reader(pages: Array<{ rows: OutboundRequestRecord[]; next: string | null; total: number }>) {
  const seen: Array<{ conditions: CatalogueConditions; after: string | null }> = [];
  const read = async (conditions: CatalogueConditions, after: string | null): Promise<ApiResult<OutboundRequestListBody>> => {
    seen.push({ conditions, after });
    const index = after === null ? 0 : Number(after);
    const page = pages[index];
    return {
      kind: 'outcome',
      status: 200,
      body: {
        outcome: 'OUTBOUND_REQUESTS_LISTED',
        requests: page.rows,
        page: { size: 200, next: page.next, total: page.total },
      },
    };
  };
  return { read, seen };
}

test('发货交接队列翻到没有下一页，张数用总数', async () => {
  const source = reader([
    { rows: [row('A')], next: '1', total: 2 },
    { rows: [row('B')], next: null, total: 2 },
  ]);
  const answer = await loadDispatchQueue({ warehouse: 'SYN-WH-01', read: source.read });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  deepEqual(answer.queue.rows.map((item) => item.requestId), ['A', 'B']);
  equal(answer.queue.truncated, false);
  equal(dispatchQueueNote(answer.queue), '已打包、等交出的出库单 2 张。');
  equal(source.seen.length, 2);
  deepEqual(source.seen[0].conditions.filters, { warehouseCode: ['SYN-WH-01'], status: ['PACKED'] });
});

test('发货交接队列翻到上限还有下一页，说只列了前多少张', async () => {
  const source = reader([
    { rows: [row('A'), row('B')], next: '1', total: 5 },
    { rows: [row('C')], next: '2', total: 5 },
  ]);
  const answer = await loadDispatchQueue({ warehouse: 'SYN-WH-01', read: source.read, maxPages: 1 });
  ok(answer.kind === 'queue');
  if (answer.kind !== 'queue') return;
  equal(answer.queue.rows.length, 2);
  equal(answer.queue.truncated, true);
  equal(dispatchQueueNote(answer.queue), '已打包、等交出的出库单 5 张。列表只列了前 2 张。');
});
