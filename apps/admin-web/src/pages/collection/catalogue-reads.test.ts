import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { listCodSubledgers } from './api';

// 本文件钉代收分户账读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：不带条件即迁移前的裸路径，检索词与游标一并下推。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: 'c2', total: 3 };

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ outcome: 'COD_SUBLEDGERS_LISTED', subledgers: [], page }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

// Covers: 不带参数即裸路径；检索词去首尾空白下推成 q，游标殿后；读口声明的唯一可排维照名下推。
test('不带参数即裸路径，检索词、排序与游标一并下推', async () => {
  const urls = captureRequests();
  await listCodSubledgers();
  await listCodSubledgers({ q: ' SYN-ACCOUNT-01 ' }, 'c1');
  await listCodSubledgers({ q: '   ', sort: '-customer' });
  deepEqual(urls, ['/collection-subledgers', '/collection-subledgers?q=SYN-ACCOUNT-01&after=c1', '/collection-subledgers?sort=-customer']);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listCodSubledgers();
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——callerProblem 原样带上 detail；服务端没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '检索词 q 超过 100 个字符' } }), {
      status: 400,
    })) as typeof fetch;
  deepEqual(await listCodSubledgers({ q: '甲'.repeat(101) }), {
    kind: 'callerProblem',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: '检索词 q 超过 100 个字符',
  });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listCodSubledgers({}, 'stale');
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
