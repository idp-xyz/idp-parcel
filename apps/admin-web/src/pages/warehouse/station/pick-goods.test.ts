import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  createGoodsIdentifier,
  goodsLookupOf,
  liveGoodsIdentifier,
  ownerLookupOf,
  type GoodsLookup,
  type OwnerLookup,
} from './pick-goods';

const cell = { requestId: 'REQ-1', goodsCode: 'SYN-SKU-CHARGER-02' };

function identifier(options: { owner?: OwnerLookup | ((requestId: string) => OwnerLookup); goods?: (code: string) => GoodsLookup } = {}) {
  const calls = { owners: [] as string[], goods: [] as string[] };
  const instance = createGoodsIdentifier({
    ownerOf: async (requestId) => {
      calls.owners.push(requestId);
      const owner = options.owner;
      if (typeof owner === 'function') return owner(requestId);
      return owner ?? { kind: 'owner', ownerAccount: 'SYN-OWNER-01' };
    },
    findGoods: async (ownerAccount, code) => {
      calls.goods.push(`${ownerAccount}|${code}`);
      return options.goods ? options.goods(code) : { kind: 'notFound' };
    },
  });
  return { instance, calls };
}

test('扫到的码就是任务的商品编码：当场算对，不取货主也不问后端', async () => {
  const { instance, calls } = identifier();
  deepEqual(await instance.identify(cell, 'SYN-SKU-CHARGER-02'), { kind: 'same', via: 'code' });
  deepEqual(calls, { owners: [], goods: [] });
});

test('码不等于商品编码时按这位货主认一次：认成同一个商品算对（条码）', async () => {
  const { instance, calls } = identifier({ goods: () => ({ kind: 'found', goodsCode: 'SYN-SKU-CHARGER-02' }) });
  deepEqual(await instance.identify(cell, '6900001000028'), { kind: 'same', via: 'alias' });
  deepEqual(calls, { owners: ['REQ-1'], goods: ['SYN-OWNER-01|6900001000028'] });
});

test('认成这位货主的另一个商品算错，并说是哪一个', async () => {
  const { instance } = identifier({ goods: () => ({ kind: 'found', goodsCode: 'SYN-SKU-EARBUD-01' }) });
  deepEqual(await instance.identify(cell, '6900001000011'), { kind: 'other', goodsCode: 'SYN-SKU-EARBUD-01' });
});

test('认不出算错：商品编码里没有，也不是有效的条码或平台商品码', async () => {
  const { instance } = identifier();
  deepEqual(await instance.identify(cell, '6900009999999'), { kind: 'unknown' });
});

test('同一张出库单的货主只取一次，别的格同一张单也复用', async () => {
  const { instance, calls } = identifier();
  await instance.identify(cell, 'X-1');
  await instance.identify(cell, 'X-2');
  await instance.identify({ requestId: 'REQ-1', goodsCode: 'SYN-SKU-OTHER' }, 'X-3');
  deepEqual(calls.owners, ['REQ-1']);
});

test('不同出库单各取各的货主；同一位货主的两张单，同一个码只问一次', async () => {
  const { instance, calls } = identifier();
  await instance.identify(cell, 'X-1');
  await instance.identify({ requestId: 'REQ-2', goodsCode: 'SYN-SKU-CHARGER-02' }, 'X-1');
  deepEqual(calls.owners, ['REQ-1', 'REQ-2']);
  deepEqual(calls.goods, ['SYN-OWNER-01|X-1']);
});

test('两位货主的两张单：同一个码按各自的货主各认一次，商品册是按货主分的', async () => {
  const { instance, calls } = identifier({
    owner: (requestId) => ({ kind: 'owner', ownerAccount: requestId === 'REQ-1' ? 'SYN-OWNER-01' : 'SYN-OWNER-02' }),
  });
  await instance.identify(cell, 'X-1');
  await instance.identify({ requestId: 'REQ-2', goodsCode: 'SYN-SKU-CHARGER-02' }, 'X-1');
  deepEqual(calls.goods, ['SYN-OWNER-01|X-1', 'SYN-OWNER-02|X-1']);
});

test('同一个（货主，码）在这一趟里只问一次：认成了的与认不出的都记下', async () => {
  const { instance, calls } = identifier({ goods: (code) => (code === 'ALIAS' ? { kind: 'found', goodsCode: 'SYN-SKU-CHARGER-02' } : { kind: 'notFound' }) });
  deepEqual(await instance.identify(cell, 'ALIAS'), { kind: 'same', via: 'alias' });
  deepEqual(await instance.identify(cell, 'ALIAS'), { kind: 'same', via: 'alias' });
  deepEqual(await instance.identify(cell, 'NOPE'), { kind: 'unknown' });
  deepEqual(await instance.identify(cell, 'NOPE'), { kind: 'unknown' });
  deepEqual(calls.goods, ['SYN-OWNER-01|ALIAS', 'SYN-OWNER-01|NOPE']);
});

test('缓存按这一格要的商品之外的东西记：同一个码换一格还是同一个认法，只是对错按这一格判', async () => {
  const { instance, calls } = identifier({ goods: () => ({ kind: 'found', goodsCode: 'SYN-SKU-EARBUD-01' }) });
  deepEqual(await instance.identify(cell, '6900001000011'), { kind: 'other', goodsCode: 'SYN-SKU-EARBUD-01' });
  deepEqual(await instance.identify({ requestId: 'REQ-1', goodsCode: 'SYN-SKU-EARBUD-01' }, '6900001000011'), { kind: 'same', via: 'alias' });
  equal(calls.goods.length, 1);
});

test('没问成的不记：下一次扫同一个码还问', async () => {
  let answer: GoodsLookup = { kind: 'failed', reason: '网络没通（断了）' };
  const calls: string[] = [];
  const instance = createGoodsIdentifier({
    ownerOf: async () => ({ kind: 'owner', ownerAccount: 'SYN-OWNER-01' }),
    findGoods: async (_owner, code) => {
      calls.push(code);
      return answer;
    },
  });
  const failed = await instance.identify(cell, '6900001000028');
  equal(failed.kind, 'failed');
  if (failed.kind === 'failed') equal(failed.reason, '读商品资料没成：网络没通（断了）');
  answer = { kind: 'found', goodsCode: 'SYN-SKU-CHARGER-02' };
  deepEqual(await instance.identify(cell, '6900001000028'), { kind: 'same', via: 'alias' });
  deepEqual(calls, ['6900001000028', '6900001000028']);
});

test('取不到货主不说成认不出：查无这张出库单，或读口没问成，都说没能认', async () => {
  const missing = identifier({ owner: { kind: 'missing' } });
  const first = await missing.instance.identify(cell, '6900001000028');
  equal(first.kind, 'failed');
  deepEqual(missing.calls.goods, []);

  const failing = identifier({ owner: { kind: 'failed', reason: '系统暂时没答出来' } });
  const second = await failing.instance.identify(cell, '6900001000028');
  equal(second.kind, 'failed');
  if (second.kind === 'failed') equal(second.reason.includes('系统暂时没答出来'), true);
});

test('货主没取到的也不记，下一次再取', async () => {
  let owner: OwnerLookup = { kind: 'failed', reason: '断了' };
  let asked = 0;
  const instance = createGoodsIdentifier({
    ownerOf: async () => {
      asked += 1;
      return owner;
    },
    findGoods: async () => ({ kind: 'notFound' }),
  });
  equal((await instance.identify(cell, 'X')).kind, 'failed');
  owner = { kind: 'owner', ownerAccount: 'SYN-OWNER-01' };
  deepEqual(await instance.identify(cell, 'X'), { kind: 'unknown' });
  equal(asked, 2);
});

// —— 读口答复的收法 ——

test('出库单明细答里有这张单就取它的货主；没有是查无此单；没问成交失败原因', () => {
  deepEqual(
    ownerLookupOf({ kind: 'outcome', status: 200, body: { outcome: 'READ', request: { ownerAccount: 'SYN-OWNER-01' } } as never }),
    { kind: 'owner', ownerAccount: 'SYN-OWNER-01' },
  );
  deepEqual(ownerLookupOf({ kind: 'outcome', status: 200, body: { outcome: 'NOT_FOUND' } }), { kind: 'missing' });
  const failed = ownerLookupOf({ kind: 'transport', message: '断了' });
  equal(failed.kind, 'failed');
});

test('认码读口：found 为真交商品编码，found 为假是没有，别的是没问成', () => {
  deepEqual(
    goodsLookupOf({ kind: 'outcome', status: 200, body: { outcome: 'GOODS_FOUND', ownerAccount: 'O', code: 'C', found: true, goods: { goodsCode: 'G-1' } as never } }),
    { kind: 'found', goodsCode: 'G-1' },
  );
  deepEqual(goodsLookupOf({ kind: 'outcome', status: 200, body: { outcome: 'GOODS_FOUND', ownerAccount: 'O', code: 'C', found: false } }), {
    kind: 'notFound',
  });
  equal(goodsLookupOf({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }).kind, 'failed');
  equal(goodsLookupOf({ kind: 'notGranted' }).kind, 'failed');
});

// —— 接真读口：请求的路径与键 ——

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function answerByPath(routes: Record<string, unknown>): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    urls.push(url);
    const path = url.split('?')[0];
    return new Response(JSON.stringify(routes[path]), { status: 200, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  return urls;
}

test('接真读口：先按号取出库单明细拿货主，再用（货主，码）认商品', async () => {
  const urls = answerByPath({
    '/warehouse-fulfillment-outbound-request': { outcome: 'READ', request: { requestId: 'REQ-1', ownerAccount: 'SYN-OWNER-01' } },
    '/warehouse-fulfillment-goods-by-code': {
      outcome: 'GOODS_FOUND',
      ownerAccount: 'SYN-OWNER-01',
      code: '6900001000028',
      found: true,
      goods: { goodsCode: 'SYN-SKU-CHARGER-02' },
    },
  });
  const result = await liveGoodsIdentifier().identify(cell, '6900001000028');
  deepEqual(result, { kind: 'same', via: 'alias' });
  deepEqual(urls, [
    '/warehouse-fulfillment-outbound-request?requestId=REQ-1',
    '/warehouse-fulfillment-goods-by-code?ownerAccount=SYN-OWNER-01&code=6900001000028',
  ]);
});
