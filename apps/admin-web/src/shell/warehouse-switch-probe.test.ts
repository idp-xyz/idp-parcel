import { afterEach, test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { configureMasterDataApi } from '../pages/catalogue-api';
import { catalogueReadPaths } from '../pages/warehouse/rows';
import { probeWarehouseSwitch } from './warehouse-switch-probe';
import type { WarehouseSwitch } from './warehouse-switch';

// 本文件钉的是探针发出去的请求，与真实的服务端答复经 exchangeMasterData 落到三态：判读逐格的用例在 warehouse-switch.test.ts，
// 这里证它不只在手搭的答复上成立。

interface CapturedRequest {
  url: string;
  init: RequestInit | undefined;
}

const realFetch = globalThis.fetch;

function stubFetch(respond: (request: CapturedRequest) => Response | Promise<Response>) {
  const captured: CapturedRequest[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = { url: String(input), init };
    captured.push(request);
    return respond(request);
  }) as typeof fetch;
  return captured;
}

afterEach(() => {
  globalThis.fetch = realFetch;
  configureMasterDataApi({ basePrefix: '' });
});

// 服务端关着时写的就是这一份：RFC 9457 问题详情，`code` 在顶层，媒体类型 application/problem+json（httpapi.WriteProblem）。
function problemResponse(status: number, code: string): Response {
  const body = {
    type: `tag:idp.xyz,2026:problem:${code}`,
    title: 'Problem',
    status,
    code,
    requestId: 'req-1',
  };
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/problem+json' } });
}

// Covers: 探针发的是一个不带条件、不带请求体的 GET——关着时它在认证之前就被固定处理器截下，查阅条件与令牌都不影响答复；
// 路径与仓库册读口是同一条（探针不能引 rows，由这里把两处钉在一起）。
test('探针是一个不带查询串的 GET，打的就是仓库册的读口', async () => {
  configureMasterDataApi({ basePrefix: '/api' });
  const captured = stubFetch(() => problemResponse(403, 'WAREHOUSE_FULFILLMENT_NOT_ENABLED'));

  const answer = await probeWarehouseSwitch();

  equal(captured.length, 1);
  equal(captured[0].url, `/api${catalogueReadPaths.warehouse}`);
  ok(!captured[0].url.includes('?'), '不带查询串');
  equal(captured[0].init?.method, 'GET');
  equal(captured[0].init?.body, undefined);
  equal(answer, 'disabled');
});

// Covers: 服务端真实答出的各格都经 exchangeMasterData 落到三态里：开着的部署（有授予的业务结果、令牌被拒、没有授予、渠道未配置、
// 别的 403）都是开着，依赖故障与网关错误页是不知道。
test('开着的部署对探针的各种答复都判成开着，没答出来判成不知道', async () => {
  const replies: ReadonlyArray<readonly [string, () => Response, WarehouseSwitch]> = [
    ['有授予：业务结果', () => new Response(JSON.stringify({ outcome: 'WAREHOUSES_LISTED', warehouses: [] }), { status: 200 }), 'enabled'],
    ['令牌被拒', () => problemResponse(401, 'OPERATOR_CREDENTIAL_REJECTED'), 'enabled'],
    ['没有授予', () => problemResponse(403, 'OPERATOR_NOT_GRANTED'), 'enabled'],
    ['接入渠道未配置', () => problemResponse(403, 'ACCESS_CHANNEL_NOT_CONFIGURED'), 'enabled'],
    ['别的 403 问题码', () => problemResponse(403, 'SOMETHING_ELSE'), 'enabled'],
    ['依赖故障', () => problemResponse(500, 'NO_ANSWER_FORMED'), 'unknown'],
    ['网关错误页', () => new Response('<html>bad gateway</html>', { status: 502 }), 'unknown'],
  ];
  for (const [name, reply, want] of replies) {
    stubFetch(() => reply());
    equal(await probeWarehouseSwitch(), want, name);
  }
});

// Covers: 传输失败（fetch 抛错）也交回三态而不是拒绝——调用方不必再接一层。
test('传输失败交回不知道，不拒绝', async () => {
  stubFetch(() => {
    throw new TypeError('Failed to fetch');
  });
  equal(await probeWarehouseSwitch(), 'unknown');
});
