import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  countPendingSeriesEvaluations,
  listPriceCardDrafts,
  listPriceCards,
  listPricingEvaluations,
  listReferenceSeries,
  listReferenceSeriesCoverage,
  readPriceCardContent,
} from './api';
import type { CatalogueConditions } from '../catalogue-query';

// 本文件钉定价各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：不带参数即裸路径，带条件时经 catalogueRequestPath 拼成同一个查询串；挂起评价计数与价卡内容查阅不是目录读，
// 请求一字不变。

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

// Covers: 五个列表读口各自裸路径照旧（价格政策与供应商协议的选单、价卡编辑器的序列候选、试算页都是无参调用），带检索词与
// 游标时一并下推。
test('五个列表读口不带参数即裸路径，带检索词与游标时一并下推', async () => {
  const reads: Array<[string, (conditions?: CatalogueConditions, after?: string | null) => Promise<unknown>]> = [
    ['/pricing-evaluations', listPricingEvaluations],
    ['/pricing-price-cards', listPriceCards],
    ['/pricing-reference-series', listReferenceSeries],
    ['/pricing-reference-series-coverage', listReferenceSeriesCoverage],
    ['/pricing-price-card-draft-views', listPriceCardDrafts],
  ];
  for (const [path, read] of reads) {
    const urls = captureRequests();
    await read();
    await read({ q: ' SYN-PLAN ' }, 'c1');
    deepEqual(urls, [path, `${path}?q=SYN-PLAN&after=c1`], path);
  }
});

// Covers: 评价的结果胶囊落到 status 一维，与检索词、排序、游标一起下推，键按字典序、游标殿后。
test('评价按结果、检索词、排序与游标一并下推', async () => {
  const urls = captureRequests();
  await listPricingEvaluations({ q: 'SYN-EVAL', filters: { status: ['PENDING'] }, sort: '-recordedAt' }, 'c1');
  deepEqual(urls, ['/pricing-evaluations?sort=-recordedAt&status=PENDING&q=SYN-EVAL&after=c1']);
});

// Covers: 「只看这条序列」落到 seriesId 与 kind 两个精确维；草稿的 status 一维可重复给，多个值去重、升序、逐个带上（维内为或）。
test('精确筛选维与可重复的筛选维按参数名下推', async () => {
  const urls = captureRequests();
  await listReferenceSeries({ q: 'ops', filters: { seriesId: ['SYN-PRC-FUEL'], kind: ['FUEL_RATE'] } });
  await listPriceCardDrafts({ filters: { status: ['VALIDATED', 'DRAFT', 'VALIDATED'] } });
  deepEqual(urls, [
    '/pricing-reference-series?kind=FUEL_RATE&seriesId=SYN-PRC-FUEL&q=ops',
    '/pricing-price-card-draft-views?status=DRAFT&status=VALIDATED',
  ]);
});

// Covers: 挂起评价计数（按种类分组的计数）与价卡内容查阅不翻页，请求与迁移前一字不变。
test('不是目录读的两个读口请求一字不变', async () => {
  const urls = captureRequests();
  await countPendingSeriesEvaluations();
  await readPriceCardContent('SYN-PLAN-A', 'v1');
  deepEqual(urls, ['/pricing-pending-series-evaluations', '/pricing-price-cards/content?id=SYN-PLAN-A&version=v1']);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listReferenceSeriesCoverage();
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——callerProblem 原样带上 detail；服务端没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }),
      { status: 400 },
    )) as typeof fetch;
  deepEqual(await listPricingEvaluations({}, 'stale'), {
    kind: 'callerProblem',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: '游标与本次的排序或筛选不符，请从第一页重取',
  });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listPriceCards();
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
