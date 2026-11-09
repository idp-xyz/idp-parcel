import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { listOperators } from './api';
import { listWorkDevices } from '../work-devices/api';

// 本文件钉 access-identity 两册（操作者、作业设备）在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；
// 契约 ADR-0144 决定一、四、五）：不带条件即裸路径；操作者册只有 q，作业设备册另有 status 一维；游标殿后。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 200, next: 'c2', total: 412 };

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

// Covers: 操作者册——不带条件即裸路径；检索词去首尾空白下推成 q，游标跟在后面。
test('操作者册不带条件即裸路径，检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listOperators();
  await listOperators({ q: ' ChZTWU4 ' }, 'c1');
  deepEqual(urls, ['/operators', '/operators?q=ChZTWU4&after=c1']);
});

// Covers: 作业设备册——status 一维逐值去重升序下推（维内为或），检索词与游标跟在后面。
test('作业设备册不带条件即裸路径，status、检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listWorkDevices();
  await listWorkDevices({ q: 'SYN-NODE', filters: { status: ['REGISTERED', 'DEACTIVATED', 'REGISTERED'] } }, 'c1');
  deepEqual(urls, ['/work-devices', '/work-devices?status=DEACTIVATED&status=REGISTERED&q=SYN-NODE&after=c1']);
});

// Covers: 两册答复的 page 原样交出（翻页条上的「共 N 条」就是它的 total）。
test('两册答复的 page 原样交出', async () => {
  captureRequests();
  for (const result of [await listOperators(), await listWorkDevices()]) {
    if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
    deepEqual(result.body.page, page);
  }
});

// Covers: 读口答 400 带理由散文（如词表外的 status、检索词过长），api 层原样放进 detail。
test('400 的理由散文进 detail', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '检索词 q 超过 100 个字符' } }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    })) as typeof fetch;
  const result = await listOperators({ q: 'X'.repeat(101) });
  equal(result.kind, 'callerProblem');
  if (result.kind === 'callerProblem') equal(result.detail, '检索词 q 超过 100 个字符');
});
