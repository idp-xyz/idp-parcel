import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { CodeRef, codeRefTitle } from './CodeRef';

// 本文件钉编码引用格：值等宽、悬停说明带全文与「点击复制」、标签可选、折行与截断二选一。

test('值等宽显示，悬停说明带全文与复制提示', () => {
  const html = renderToStaticMarkup(createElement(CodeRef, { value: 'SHR-ABC', label: '委托', title: '委托标识' }));
  match(html, /font-mono/);
  match(html, /SHR-ABC/);
  match(html, /title="委托标识：SHR-ABC（点击复制）"/);
  match(html, />委托</);
  match(html, /truncate/);
});

test('没有标签时只显值；wrap 折行不截断', () => {
  const html = renderToStaticMarkup(createElement(CodeRef, { value: 'X-1', wrap: true }));
  equal(html.includes('truncate'), false);
  match(html, /break-all/);
  equal(codeRefTitle(undefined, 'X-1'), 'X-1（点击复制）');
});
