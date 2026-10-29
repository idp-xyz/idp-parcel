import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { contentKeyOf, createContentLoader, type ContentAnswer } from './price-card-content-loader';

// 价卡内容取数：同一版只发一个请求、读到了就记住、并发有上限；不是「查到了」的答案一律不记。

const found = (planId: string, planVersion: string): ContentAnswer => ({
  kind: 'outcome',
  status: 200,
  body: {
    outcome: 'PRICE_CARD_CONTENT_FOUND',
    planId,
    planVersion,
    canonicalization: 'PPC-5',
    contentDigest: 'd',
    publicationApprover: 'SYN-GOV',
    registeredAt: '2026-01-01T00:00:00Z',
    document: {},
  },
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

test('同一版同时被问几次只发一个请求，读到后记住', async () => {
  let calls = 0;
  const loader = createContentLoader(async (id, version) => {
    calls += 1;
    return found(id, version);
  }, 4);
  const key = contentKeyOf({ planId: 'P', planVersion: 'v1', contentDigest: 'd' });
  const [first, second] = await Promise.all([loader.load(key, 'P', 'v1'), loader.load(key, 'P', 'v1')]);
  equal(calls, 1);
  equal(first, second);
  equal(loader.peek(key), first);
  await loader.load(key, 'P', 'v1');
  equal(calls, 1, '记住之后不再发请求');
});

test('摘要变了就是另一把键，不拿旧内容顶', async () => {
  let calls = 0;
  const loader = createContentLoader(async (id, version) => {
    calls += 1;
    return found(id, version);
  }, 4);
  await loader.load(contentKeyOf({ planId: 'P', planVersion: 'v1', contentDigest: 'a' }), 'P', 'v1');
  await loader.load(contentKeyOf({ planId: 'P', planVersion: 'v1', contentDigest: 'b' }), 'P', 'v1');
  equal(calls, 2);
});

test('不是「查到了」的答案不记：查无、未配置、没有授予、传输失败都会因配置、授予、登记而变', async () => {
  const answers: ContentAnswer[] = [
    { kind: 'outcome', status: 200, body: { outcome: 'NOT_FOUND', planId: 'P', planVersion: 'v1' } },
    { kind: 'unconfigured' },
    { kind: 'notGranted' },
    { kind: 'transport', message: 'down' },
  ];
  let calls = 0;
  const loader = createContentLoader(async () => answers[calls++], 4);
  for (let index = 0; index < answers.length; index += 1) {
    await loader.load('k', 'P', 'v1');
    equal(loader.peek('k'), null);
  }
  equal(calls, answers.length, '每次都重新问');
});

test('并发有上限，排队的在前面的完成后依次放行', async () => {
  const gates = [deferred<ContentAnswer>(), deferred<ContentAnswer>(), deferred<ContentAnswer>()];
  let running = 0;
  let peak = 0;
  const started: string[] = [];
  const loader = createContentLoader(async (id) => {
    started.push(id);
    running += 1;
    peak = Math.max(peak, running);
    const answer = await gates[Number(id)].promise;
    running -= 1;
    return answer;
  }, 2);
  const results = [0, 1, 2].map((index) => loader.load(`k${index}`, String(index), 'v1'));
  await Promise.resolve();
  await Promise.resolve();
  deepEqual(started, ['0', '1'], '第三个排队');
  gates[0].resolve(found('0', 'v1'));
  await results[0];
  await Promise.resolve();
  deepEqual(started, ['0', '1', '2']);
  gates[1].resolve(found('1', 'v1'));
  gates[2].resolve(found('2', 'v1'));
  await Promise.all(results);
  equal(peak, 2);
});
