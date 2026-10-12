import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { configureBearerSource } from './bearer';
import { exchangeMasterData, postMasterData, postMasterDataForm } from '../pages/catalogue-api';
import { listShipmentRequestViews, withdrawShipmentRequest } from '../pages/shipment-request/api';
import { listTrackingProjections } from '../pages/visibility/api';

// 票 operator-channel/07「请求带 Bearer」：各 fetch 出口的每一种请求都要带上，且令牌在发请求那一刻才取
// （续期换来的新令牌要用在下一个请求上）。期望取 ADR-0100 决定二第一条「SPA 取令牌、parcel-api 自己校验」。
// 新增出口时把它的读与写各加一次进 hitEveryExit。

const realFetch = globalThis.fetch;

function captureHeaders(): Array<Record<string, string> | undefined> {
  const captured: Array<Record<string, string> | undefined> = [];
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    captured.push(init?.headers as Record<string, string> | undefined);
    return new Response(JSON.stringify({ outcome: 'X' }), { status: 200 });
  }) as typeof fetch;
  return captured;
}

async function hitEveryExit(): Promise<void> {
  await exchangeMasterData('/pricing-price-cards');
  await postMasterData('/pricing-price-card-registrations', {});
  const form = new FormData();
  form.append('file', new Blob(['bytes']), 'card.xlsx');
  await postMasterDataForm('/pricing-price-card-previews', form);
  await listShipmentRequestViews();
  await withdrawShipmentRequest({} as Parameters<typeof withdrawShipmentRequest>[0]);
  await listTrackingProjections();
}

afterEach(() => {
  globalThis.fetch = realFetch;
  configureBearerSource(() => Promise.resolve(null));
});

test('各出口的每种请求都带 Bearer，原有请求头不丢', async () => {
  configureBearerSource(() => Promise.resolve('SYN-ACCESS-TOKEN'));
  const captured = captureHeaders();

  await hitEveryExit();

  equal(captured.length, 6);
  for (const headers of captured) {
    equal(headers?.Authorization, 'Bearer SYN-ACCESS-TOKEN');
  }
  deepEqual(captured[0], { Accept: 'application/json', Authorization: 'Bearer SYN-ACCESS-TOKEN' });
  deepEqual(captured[1], {
    Accept: 'application/json',
    'Content-Type': 'application/json',
    Authorization: 'Bearer SYN-ACCESS-TOKEN',
  });
  // 上传那一口仍不自写 Content-Type：multipart 边界要由浏览器带上。
  deepEqual(captured[2], { Accept: 'application/json', Authorization: 'Bearer SYN-ACCESS-TOKEN' });
  deepEqual(captured[4], { 'Content-Type': 'application/json', Authorization: 'Bearer SYN-ACCESS-TOKEN' });
});

test('取不到令牌时请求照发、不带 Authorization，也不加一格空值', async () => {
  configureBearerSource(() => Promise.resolve(null));
  const captured = captureHeaders();

  await hitEveryExit();

  equal(captured.length, 6);
  for (const headers of captured) {
    equal(headers !== undefined && 'Authorization' in headers, false);
  }
  deepEqual(captured[0], { Accept: 'application/json' });
});

test('令牌在发请求那一刻才取：续期换来的新令牌用在下一个请求上', async () => {
  let issued = 0;
  configureBearerSource(() => Promise.resolve(`SYN-ACCESS-TOKEN-${++issued}`));
  const captured = captureHeaders();

  await exchangeMasterData('/pricing-price-cards');
  await exchangeMasterData('/pricing-price-cards');

  equal(captured[0]?.Authorization, 'Bearer SYN-ACCESS-TOKEN-1');
  equal(captured[1]?.Authorization, 'Bearer SYN-ACCESS-TOKEN-2');
});
