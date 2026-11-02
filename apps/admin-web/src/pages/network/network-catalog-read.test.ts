import { afterEach, test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { listNetworkCatalog, networkCatalogConditions } from './api';

// 本文件钉网络目录读口在 api 层带上的查询参数（票 catalogue-read-pagination/05 第 1 条；契约 ADR-0144 决定一、四）：
// 族是册子选择器，与检索词、游标同进一个查询串。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(
      JSON.stringify({ outcome: 'NODE_VERSIONS_LISTED', versions: [], page: { size: 2, next: null, total: 0 } }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    );
  }) as typeof fetch;
  return urls;
}

// Covers: 不带检索与游标时只剩 family（第一页、缺省序）；带检索词（去首尾空白）与游标时三样一起下推。
test('网络目录读口把族、检索词与游标一起带上', async () => {
  const urls = captureRequests();
  await listNetworkCatalog(networkCatalogConditions('node'));
  await listNetworkCatalog(networkCatalogConditions('connection', { q: ' SIN ' }), 'c1');
  deepEqual(urls, ['/network-catalog?family=node', '/network-catalog?family=connection&q=SIN&after=c1']);
});

// Covers: ADR-0353 决定二的读口契约——网点版本行登了名称带 displayName，没登的行上没有这个键；api 层原样交出，不补空串、
// 不拿 code 顶（页面怎么称呼由页面定，不在 api 层）。
test('网点版本行的显示名称原样交出，没登的行不带这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        outcome: 'NODE_VERSIONS_LISTED',
        versions: [
          { code: 'SYN-NODE-SHA-HUB', version: 1, businessTimezone: 'Asia/Shanghai', effectiveFrom: '2026-01-01T00:00:00Z' },
          {
            code: 'SYN-NODE-SHA-HUB',
            version: 2,
            displayName: '上海枢纽',
            businessTimezone: 'Asia/Shanghai',
            effectiveFrom: '2026-06-01T00:00:00Z',
          },
        ],
        page: { size: 2, next: null, total: 2 },
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )) as typeof fetch;
  const result = await listNetworkCatalog(networkCatalogConditions('node'));
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(
    result.body.versions.map((version) => ['displayName' in version, version.displayName]),
    [
      [false, undefined],
      [true, '上海枢纽'],
    ],
  );
});
