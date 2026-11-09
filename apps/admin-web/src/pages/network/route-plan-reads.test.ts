import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { listInitialRouteTies, listInitialRoutes, listRerouteSuggestions, listRouteReassessments } from './api';

// 本文件钉路线规划各册读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定一、四、五）：
// 两册 /route-plans 的 register 是选择器、由读口函数自己带上，排在最前；改路建议与成本并列不带条件即裸路径。排序、筛选维、检索词、
// 游标经 catalogueRequestPath 拼成同一个查询串。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: 'c2', total: 3 };

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

// Covers: register 由读口函数带上，页面不给；不带条件即该册第一页；conclusion、排序、检索词（去首尾空白）与游标一并下推。
test('路由判断两册以 register 选册，结论、排序、检索词与游标跟在后面', async () => {
  const urls = captureRequests();
  await listInitialRoutes();
  await listInitialRoutes({ q: ' DPCL ', sort: 'recordedAt', filters: { conclusion: ['NO_CURRENT_ROUTE'] } }, 'c1');
  await listRouteReassessments();
  await listRouteReassessments({ filters: { conclusion: ['REROUTED', 'PLAN_LAPSED'] } }, 'c1');
  deepEqual(urls, [
    '/route-plans?register=initial-route',
    '/route-plans?register=initial-route&sort=recordedAt&conclusion=NO_CURRENT_ROUTE&q=DPCL&after=c1',
    '/route-plans?register=reassessment',
    '/route-plans?register=reassessment&conclusion=PLAN_LAPSED&conclusion=REROUTED&after=c1',
  ]);
});

// Covers: 两口没有选择器——不带条件即裸路径；state 一维、排序、检索词与游标照同一规则拼。
test('改路建议与成本并列不带条件即裸路径，state、排序、检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listRerouteSuggestions();
  await listRerouteSuggestions({ q: 'SYN-RPV', filters: { state: ['PENDING'] } }, 'c1');
  await listInitialRouteTies();
  await listInitialRouteTies({ sort: 'settledAt', filters: { state: ['DECIDED'] } });
  deepEqual(urls, [
    '/route-reroute-suggestions',
    '/route-reroute-suggestions?state=PENDING&q=SYN-RPV&after=c1',
    '/route-initial-route-ties',
    '/route-initial-route-ties?sort=settledAt&state=DECIDED',
  ]);
});

// Covers: 答复的 page 原样交出（size、total 是数，next 可为 null）。
test('各册答复的 page 原样交出', async () => {
  captureRequests();
  for (const result of [await listInitialRoutes(), await listRouteReassessments(), await listRerouteSuggestions(), await listInitialRouteTies()]) {
    if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
    deepEqual(result.body.page, page);
  }
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ outcome: 'INITIAL_ROUTE_TIES_LISTED', ties: [], page: { size: 200, next: null, total: 0 } }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })) as typeof fetch;
  const last = await listInitialRouteTies();
  if (last.kind !== 'outcome') throw new Error(`读口答 ${last.kind}`);
  deepEqual(last.body.page, { size: 200, next: null, total: 0 });
});

// Covers: 查询参数不成立时读口答 400 带理由散文，api 层原样放进 detail，页面经 catalogueViewState 接在问题码说明之后。
test('400 的理由散文进 detail', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }),
      { status: 400, headers: { 'Content-Type': 'application/json' } },
    )) as typeof fetch;
  const result = await listRerouteSuggestions({ q: 'X' }, 'stale');
  equal(result.kind, 'callerProblem');
  if (result.kind === 'callerProblem') {
    equal(result.code, 'MALFORMED_REQUEST');
    equal(result.detail, '游标与本次的排序或筛选不符，请从第一页重取');
  }
});
