import { afterEach, test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { listCustomerAccounts } from './api';

// 本文件钉客户账户读口在 api 层带上的查询参数（票 catalogue-read-pagination/05 第 1 条；契约 ADR-0144 决定一、四）。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(
      JSON.stringify({ outcome: 'CUSTOMER_ACCOUNTS_LISTED', accounts: [], page: { size: 2, next: null, total: 0 } }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    );
  }) as typeof fetch;
  return urls;
}

// Covers: 不带参数即裸路径——不翻页的调用方照旧拿第一页；带检索词与游标时一并下推，中文检索词按查询串编码。
test('客户账户读口不带参数即裸路径，带检索词与游标时一并下推', async () => {
  const urls = captureRequests();
  await listCustomerAccounts();
  await listCustomerAccounts({ q: '合成' }, 'c1');
  deepEqual(urls, [
    '/commercial-customer-accounts',
    '/commercial-customer-accounts?q=%E5%90%88%E6%88%90&after=c1',
  ]);
});
