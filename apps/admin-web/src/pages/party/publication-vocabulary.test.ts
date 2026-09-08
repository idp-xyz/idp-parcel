import { afterEach, test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { configureMasterDataApi } from '../catalogue-api';
import {
  fetchPublicationVocabulary,
  publicationVocabularyEndpoint,
  vocabularyOptions,
} from './publication-draft-api';

// 本文件钉词表读口的前端公共半边（票 admin-write-faces/20「要做的」admin-web 一节）：
//   1. fetchPublicationVocabulary 发向端点表上那条路径、kind 进查询串，答复按主数据五格结果代数交回——
//      403 ACCESS_CHANNEL_NOT_CONFIGURED 是可辨的「未配置」不是抛错，表单票拿它写占位；
//   2. vocabularyOptions 是码 × 本页中文词表：顺序照服务端（领域枚举顺序），词表没收录的码原样示出、不猜格，
//      不给默认选中也不补「请选择」占位项（「表单不给默认、不预选」归表单票）；
//   3. 这里没有任何一份内置的码：服务端答什么就是什么，前端不备一份回退。

const realFetch = globalThis.fetch;

function stubFetch(status: number, body: unknown): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

afterEach(() => {
  globalThis.fetch = realFetch;
  configureMasterDataApi({ basePrefix: '' });
});

test('fetchPublicationVocabulary 发向 /commercial-publication-vocabularies 并把 kind 放进查询串', async () => {
  const urls = stubFetch(200, {
    outcome: 'PUBLICATION_VOCABULARY_LISTED',
    kind: 'SETTLEMENT_POLICY',
    sets: [{ name: 'method', codes: ['PREPAID', 'TERMS'] }],
  });

  const result = await fetchPublicationVocabulary('SETTLEMENT_POLICY');

  deepEqual(urls, [`${publicationVocabularyEndpoint}?kind=SETTLEMENT_POLICY`]);
  equal(publicationVocabularyEndpoint, '/commercial-publication-vocabularies');
  ok(result.kind === 'outcome');
  equal(result.body.kind, 'SETTLEMENT_POLICY');
  deepEqual(result.body.sets, [{ name: 'method', codes: ['PREPAID', 'TERMS'] }]);
});

test('kind 合法而没词时 sets 是空数组，仍是形成了的答案', async () => {
  stubFetch(200, { outcome: 'PUBLICATION_VOCABULARY_LISTED', kind: 'SERVICE_PRODUCT', sets: [] });

  const result = await fetchPublicationVocabulary('SERVICE_PRODUCT');

  ok(result.kind === 'outcome');
  deepEqual(result.body.sets, []);
});

test('403 ACCESS_CHANNEL_NOT_CONFIGURED 交回可辨的「未配置」，不抛错、不备内置码回退', async () => {
  stubFetch(403, { error: { code: 'ACCESS_CHANNEL_NOT_CONFIGURED' } });

  const result = await fetchPublicationVocabulary('ACCEPTANCE_RULE_PACKAGE');

  deepEqual(result, { kind: 'unconfigured' });
});

test('坏 kind 的 400 按调用方问题交回状态与码', async () => {
  stubFetch(400, {
    error: { code: 'MALFORMED_REQUEST', problems: [{ field: 'kind', problem: '集合外的商业对象类别 "X"' }] },
  });

  const result = await fetchPublicationVocabulary('PRICE_RULE');

  deepEqual(result, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
});

test('vocabularyOptions 照服务端顺序给选项，码作 value、本页词表作 label', () => {
  const options = vocabularyOptions(['PREPAID', 'TERMS'], { PREPAID: '预付', TERMS: '账期' });

  deepEqual(options, [
    { value: 'PREPAID', label: '预付' },
    { value: 'TERMS', label: '账期' },
  ]);
});

test('词表没收录的码原样示出，不猜格、不丢掉', () => {
  const options = vocabularyOptions(['ALLOWED', 'DISALLOWED', 'NEW_WORD'], { ALLOWED: '允许' });

  deepEqual(options, [
    { value: 'ALLOWED', label: '允许' },
    { value: 'DISALLOWED', label: 'DISALLOWED' },
    { value: 'NEW_WORD', label: 'NEW_WORD' },
  ]);
});

test('空码列表给空选项：不补占位项，也没有任何一项带默认选中', () => {
  const options = vocabularyOptions([], { PREPAID: '预付' });

  deepEqual(options, []);
  ok(vocabularyOptions(['PREPAID'], {}).every((option) => Object.keys(option).join(',') === 'value,label'));
});
