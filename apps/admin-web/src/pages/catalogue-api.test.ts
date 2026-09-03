import { afterEach, test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import { configureMasterDataApi, exchangeMasterData, postMasterData } from './catalogue-api';

// 五格结果代数的判读是所有主数据页共用的一道门，期望值取 ADR-0022（HTTP 状态只说答案有没有形成）
// 与 ADR-0055（接入渠道未配置 = 403 + ACCESS_CHANNEL_NOT_CONFIGURED）的原句，不从实现推。

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

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

afterEach(() => {
  globalThis.fetch = realFetch;
  configureMasterDataApi({ basePrefix: '' });
});

test('2xx JSON 是形成了的业务答案，原样交回 outcome', async () => {
  stubFetch(() => jsonResponse(200, { outcome: 'PRICE_CARDS_LISTED', cards: [] }));

  const result = await exchangeMasterData<{ outcome: string; cards: unknown[] }>('/pricing-price-cards');

  ok(result.kind === 'outcome');
  equal(result.status, 200);
  deepEqual(result.body, { outcome: 'PRICE_CARDS_LISTED', cards: [] });
});

test('403 + ACCESS_CHANNEL_NOT_CONFIGURED 判为未配置，不是错误也不是空目录', async () => {
  stubFetch(() => jsonResponse(403, { error: { code: 'ACCESS_CHANNEL_NOT_CONFIGURED' } }));

  const result = await exchangeMasterData('/network-catalog?family=node');

  deepEqual(result, { kind: 'unconfigured' });
});

test('403 带别的问题码不是未配置，按调用方问题交回状态与码', async () => {
  stubFetch(() => jsonResponse(403, { error: { code: 'INTAKE_FAILED' } }));

  const result = await exchangeMasterData('/network-catalog?family=node');

  deepEqual(result, { kind: 'callerProblem', status: 403, code: 'INTAKE_FAILED' });
});

test('4xx 是调用方式问题，带回服务端问题码', async () => {
  stubFetch(() => jsonResponse(400, { error: { code: 'MALFORMED_REQUEST' } }));

  const result = await exchangeMasterData('/commercial-policies');

  deepEqual(result, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
});

test('5xx 是没形成答案，可重试，与业务答案分格', async () => {
  stubFetch(() => jsonResponse(500, { error: { code: 'NO_ANSWER_FORMED' } }));

  const result = await exchangeMasterData('/settlement-charges?registry=customer-charge');

  deepEqual(result, { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
});

test('非 2xx 且问题体缺 code 时以 UNKNOWN 占码，不吞掉状态', async () => {
  stubFetch(() => jsonResponse(502, {}));

  const result = await exchangeMasterData('/collection-subledgers');

  deepEqual(result, { kind: 'noAnswer', status: 502, code: 'UNKNOWN' });
});

test('响应不是 JSON 时判为传输层问题并带上状态，提示请求可能没到 parcel-api', async () => {
  stubFetch(() => new Response('<html>proxy error</html>', { status: 502 }));

  const result = await exchangeMasterData('/governance-registers?register=suspension');

  ok(result.kind === 'transport');
  match(result.message, /502/);
  match(result.message, /parcel-api/);
});

test('fetch 抛错（网络不通）判为传输层问题并带原始信息', async () => {
  stubFetch(() => {
    throw new TypeError('Failed to fetch');
  });

  const result = await exchangeMasterData('/label-transactions');

  deepEqual(result, { kind: 'transport', message: 'Failed to fetch' });
});

test('查阅请求带 basePrefix、GET 方法与 Accept JSON', async () => {
  configureMasterDataApi({ basePrefix: '/api/' });
  const captured = stubFetch(() => jsonResponse(200, { outcome: 'X' }));

  await exchangeMasterData('/pricing-price-cards');

  equal(captured.length, 1);
  equal(captured[0].url, '/api/pricing-price-cards');
  equal(captured[0].init?.method, 'GET');
  deepEqual(captured[0].init?.headers, { Accept: 'application/json' });
});

test('登记写面以 POST 送出 JSON 快照，并与查阅共用同一套结果代数', async () => {
  configureMasterDataApi({ basePrefix: '/api' });
  const captured = stubFetch(() => jsonResponse(200, { outcome: 'REGISTERED' }));

  const result = await postMasterData<{ outcome: string }>('/pricing-price-card-registrations', {
    tenantId: 'SYN-TENANT',
  });

  equal(captured[0].url, '/api/pricing-price-card-registrations');
  equal(captured[0].init?.method, 'POST');
  equal(captured[0].init?.body, '{"tenantId":"SYN-TENANT"}');
  ok(result.kind === 'outcome');
  equal(result.body.outcome, 'REGISTERED');
});

test('登记写面撞上未配置同样判为 unconfigured', async () => {
  stubFetch(() => jsonResponse(403, { error: { code: 'ACCESS_CHANNEL_NOT_CONFIGURED' } }));

  const result = await postMasterData('/commercial-publications', {});

  deepEqual(result, { kind: 'unconfigured' });
});
