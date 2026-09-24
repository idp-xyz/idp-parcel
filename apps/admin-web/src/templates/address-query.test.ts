import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { ADDRESS_KEYWORD_PARAM, decodeHashSegment, hashQueryValue, hashWithQueryValue } from './address-query';

// 本文件钉的是 hash 查询串的读写（票 admin-web-workspace-form/06）：检索词进地址、与 `?view=` 同层、清空即删键。
// 钩子那半（replaceState 不压后退记录、回程读回）在 node:test 里钉不到，用 scripts/dom-probe.mjs 一次性实测，结论写票面。

test('decodeHashSegment：解得开的照解，畸形百分号答 null 不抛', () => {
  equal(decodeHashSegment('SR%2F1'), 'SR/1');
  equal(decodeHashSegment('%E5%BC%82'), '异');
  equal(decodeHashSegment('plain'), 'plain');
  for (const bad of ['%', '%E0', '%E0%A4%A', 'a%zz']) equal(decodeHashSegment(bad), null, bad);
});

test('检索词的键名是 q', () => {
  equal(ADDRESS_KEYWORD_PARAM, 'q');
});

test('hashQueryValue：读查询串里的键；没有查询串、没有这个键都是空串', () => {
  equal(hashQueryValue('#/exception-cases?view=v1&q=SR-1', 'q'), 'SR-1');
  equal(hashQueryValue('#/exception-cases?view=v1', 'q'), '');
  equal(hashQueryValue('#/exception-cases', 'q'), '');
  equal(hashQueryValue('', 'q'), '');
});

test('hashWithQueryValue：设值不动路径与别的键；空串删键，删空了连 ? 一起去掉', () => {
  equal(hashWithQueryValue('#/exception-cases', 'q', 'SR-1'), '#/exception-cases?q=SR-1');
  equal(hashWithQueryValue('#/exception-cases?view=v1', 'q', 'SR-1'), '#/exception-cases?view=v1&q=SR-1');
  equal(hashWithQueryValue('#/exception-cases?view=v1&q=SR-1', 'q', 'SR-2'), '#/exception-cases?view=v1&q=SR-2');
  equal(hashWithQueryValue('#/exception-cases?view=v1&q=SR-1', 'q', ''), '#/exception-cases?view=v1');
  equal(hashWithQueryValue('#/exception-cases?q=SR-1', 'q', ''), '#/exception-cases');
  equal(hashWithQueryValue('#/shipment-request-inquiry/SR-1/timeline', 'q', 'x'), '#/shipment-request-inquiry/SR-1/timeline?q=x');
});

test('中文、空格、& 与 ? 往返不走样', () => {
  for (const keyword of ['异常 案件', 'a&b=c', '什么?', ' 前后空格 ', '100%']) {
    equal(hashQueryValue(hashWithQueryValue('#/exception-cases?view=v1', 'q', keyword), 'q'), keyword, keyword);
  }
});
