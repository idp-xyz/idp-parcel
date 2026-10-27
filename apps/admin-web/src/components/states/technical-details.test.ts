import { test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { TechnicalDetails, httpRows } from './technical-details';

// 本文件钉的是「技术详情」折叠件（票 party-commercial-uiux/18）：默认收起（没有 open 属性）、字仍在标记里（按码检索与测试
// 按字断言不受影响）、没有可显的行就不出这一格。与 section-error.test.ts 同理，件只引 react，node:test 里钉得到。

test('有行就出一格默认收起的折叠，标签与值都在标记里', () => {
  const html = renderToStaticMarkup(createElement(TechnicalDetails, { rows: [{ label: '错误码', value: 'NO_ANSWER_FORMED' }] }));
  match(html, /<details/);
  equal(/<details[^>]*\sopen/.test(html), false);
  match(html, /技术详情/);
  match(html, /错误码/);
  match(html, /NO_ANSWER_FORMED/);
});

test('值为空的行不显；一行都没有就不出折叠', () => {
  const some = renderToStaticMarkup(
    createElement(TechnicalDetails, {
      rows: [
        { label: '有', value: 'x' },
        { label: '空', value: '  ' },
      ],
    }),
  );
  equal(some.includes('空'), false);
  equal(renderToStaticMarkup(createElement(TechnicalDetails, { rows: [{ label: '空', value: '' }] })), '');
  equal(renderToStaticMarkup(createElement(TechnicalDetails, { rows: [] })), '');
});

test('一次 HTTP 答复交两行：状态码与问题码', () => {
  deepEqual(httpRows(502, 'NO_ANSWER_FORMED'), [
    { label: 'HTTP 状态', value: '502' },
    { label: '问题码', value: 'NO_ANSWER_FORMED' },
  ]);
});
