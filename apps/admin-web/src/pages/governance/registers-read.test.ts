import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { listAuthorityIntervals, listResumptions, listSuspensions, listTakeovers } from './api';

// 本文件钉治理登记册四册在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定一、四、五）：
// register 是册子选择器、由各册的读口函数自己带上；不带条件即该册第一页，检索词与游标经 catalogueRequestPath 跟在后面。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 200, next: 'c2', total: 201 };

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ outcome: 'ANY', page }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

// Covers: 四册各带自己的 register；不带条件即第一页；检索词去首尾空白、排序与游标跟在选择器后面。
test('四册以 register 选册，不带条件即第一页，检索词、排序与游标跟在后面', async () => {
  const urls = captureRequests();
  await listAuthorityIntervals();
  await listSuspensions();
  await listResumptions();
  await listTakeovers();
  await listSuspensions({ q: ' SYN-SUSP ' }, 'c1');
  await listTakeovers({ sort: 'effectiveAt' }, 'c1');
  deepEqual(urls, [
    '/governance-registers?register=authority-interval',
    '/governance-registers?register=suspension',
    '/governance-registers?register=resumption',
    '/governance-registers?register=takeover',
    '/governance-registers?register=suspension&q=SYN-SUSP&after=c1',
    '/governance-registers?register=takeover&sort=effectiveAt&after=c1',
  ]);
});

// Covers: 答复的 page 原样交出，概况里的条数取它的 total（整册），不是本页行数。
test('四册答复的 page 原样交出', async () => {
  captureRequests();
  for (const result of [await listAuthorityIntervals(), await listSuspensions(), await listResumptions(), await listTakeovers()]) {
    if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
    deepEqual(result.body.page, page);
  }
});

// Covers: 换了条件还拿旧游标，读口答 400 带理由散文，api 层原样放进 detail。
test('400 的理由散文进 detail', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    })) as typeof fetch;
  const result = await listResumptions({}, 'stale');
  equal(result.kind, 'callerProblem');
  if (result.kind === 'callerProblem') equal(result.detail, '游标与本次的排序或筛选不符，请从第一页重取');
});
