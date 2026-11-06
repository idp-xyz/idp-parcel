import { afterEach, test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { findChannelSelectionDecision, listTiedChannelSelectionDecisions, tiedChannelSelectionConditions } from './api';

// 本文件钉渠道择优并列清单在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：view、scope、mapping 是册子选择器，与检索词、游标同进一个查询串；按决定号取单份的请求一字不变。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: null, total: 0 };

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ outcome: 'TIED_CHANNEL_SELECTION_DECISIONS_LISTED', decisions: [], page }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

// Covers: 不收窄时只剩 view=tied（第一页、缺省序）；按对象收窄时 scope 与 mapping 一起带上；检索词与游标随后。
test('并列清单把选择器、检索词与游标一起带上', async () => {
  const urls = captureRequests();
  await listTiedChannelSelectionDecisions();
  await listTiedChannelSelectionDecisions(tiedChannelSelectionConditions({ scope: 'scope-a', mapping: 'mapping-1' }), 'c1');
  await listTiedChannelSelectionDecisions(tiedChannelSelectionConditions(undefined, { q: ' CSDN ' }));
  deepEqual(urls, [
    '/channel-selection-decisions?view=tied',
    '/channel-selection-decisions?mapping=mapping-1&scope=scope-a&view=tied&after=c1',
    '/channel-selection-decisions?view=tied&q=CSDN',
  ]);
});

// Covers: 按决定号取单份是有界查找，不翻页。
test('按决定号取单份的请求一字不变', async () => {
  const urls = captureRequests();
  await findChannelSelectionDecision('CSDN-1');
  deepEqual(urls, ['/channel-selection-decisions?decisionId=CSDN-1']);
});

// Covers: 答复的 page 原样交出。
test('并列清单答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listTiedChannelSelectionDecisions();
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});
