import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { SectionError, type SectionErrorProps } from './section-error';

// 本文件钉的是区块级错误件（票 admin-web-ux-alignment/06 第 2 条）：标题与说明照给的出、给了 onRetry 才有重试按钮、
// 是 role="alert" 的一块而不是整页。组件层能钉是因为 section-error.tsx 只引 react 与 lucide-react（两者有 CJS 入口）；
// 引 @idpxyz/* 的组件在这条链上钉不到，见 templates/loading-shape.ts 文件头。

const markupOf = (props: SectionErrorProps): string => renderToStaticMarkup(createElement(SectionError, props));

// Covers: 给了 onRetry → 有一个「重试」按钮；标题与说明逐字在。
test('区块级错误渲染标题、说明与重试按钮', () => {
  const html = markupOf({ title: '服务端未形成答案（HTTP 500）', description: '目录读取依赖未能形成答案，可稍后重试。', onRetry: () => {} });
  match(html, /role="alert"/);
  match(html, /服务端未形成答案（HTTP 500）/);
  match(html, /目录读取依赖未能形成答案，可稍后重试。/);
  equal((html.match(/<button/g) ?? []).length, 1);
  match(html, /重试/);
});

// Covers: 不给 onRetry → 没有按钮（重试不会改变结果的场合不给人一个假出口）；说明可省。
test('不给 onRetry 就没有重试按钮', () => {
  const html = markupOf({ title: '只有标题' });
  equal(/<button/.test(html), false);
  equal(/重试/.test(html), false);
  match(html, /只有标题/);
});
