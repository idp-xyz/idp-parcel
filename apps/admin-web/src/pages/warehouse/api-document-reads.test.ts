import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { getStockImportDetail, getUnclaimedArrivalDetail } from './api';

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function answerWith(status: number, body: unknown): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  return urls;
}

// 单据页按号取一张的两个读口。建账导入的读口答导入本身而不是带 outcome 的外壳，查无此号答 404；页面不认问题码，
// 所以 api 层把它收成与别的按号读口同一个形状。

test('建账导入按导入号取一张，号里的特殊字符照编码，答的导入本身包成 found', async () => {
  const record = { importId: 'IMP/1 一', status: 'PENDING', goodsLines: [], cartonLines: [] };
  const urls = answerWith(200, record);
  const answer = await getStockImportDetail('IMP/1 一');
  deepEqual(urls, ['/warehouse-fulfillment-stock-import?importId=IMP%2F1+%E4%B8%80']);
  deepEqual(answer, { kind: 'outcome', status: 200, body: { found: true, stockImport: record } });
});

test('建账导入查无此号（404 STOCK_IMPORT_NOT_FOUND）收成没有这张单，不是读取失败', async () => {
  answerWith(404, { error: { code: 'STOCK_IMPORT_NOT_FOUND' } });
  deepEqual(await getStockImportDetail('IMP-9'), { kind: 'outcome', status: 404, body: { found: false } });
});

test('建账导入别的非业务答复原样交回：别的 404 码、没授予、服务端没形成答案都不冒充没有这张单', async () => {
  answerWith(404, { error: { code: 'SOMETHING_ELSE' } });
  const other = await getStockImportDetail('IMP-9');
  equal(other.kind, 'callerProblem');

  answerWith(403, { error: { code: 'OPERATOR_NOT_GRANTED' } });
  equal((await getStockImportDetail('IMP-9')).kind, 'notGranted');

  answerWith(500, { error: { code: 'NO_ANSWER_FORMED' } });
  equal((await getStockImportDetail('IMP-9')).kind, 'noAnswer');
});

test('待认领到货按到货号取一张，答复照原样交回（found 由读口自己说）', async () => {
  const body = { outcome: 'UNCLAIMED_ARRIVAL_READ', arrivalId: 'UCA-1', found: false };
  const urls = answerWith(200, body);
  const answer = await getUnclaimedArrivalDetail('UCA-1');
  deepEqual(urls, ['/warehouse-fulfillment-unclaimed-arrival?arrivalId=UCA-1']);
  deepEqual(answer, { kind: 'outcome', status: 200, body });
});
