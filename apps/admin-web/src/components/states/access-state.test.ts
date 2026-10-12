import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { OperatorRefusalNote, OperatorRefusalState, UnconfiguredState } from './access-state';

// 本文件钉的是接入那一格三态各自的字、码与出路（票 operator-channel/07）：只有「需要登录」有一颗按钮（重新登录），
// 未配置与没有授予重试、重新登录都不会好，不给假出口。组件层能钉的理由同 section-error.test.ts。

const buttons = (html: string): number => (html.match(/<button/g) ?? []).length;

// Covers: 未配置那张卡的标题与码名照旧，没有按钮。
test('未配置：标题与码名照旧，没有按钮', () => {
  const html = renderToStaticMarkup(createElement(UnconfiguredState, {}));
  match(html, /接入渠道未配置/);
  match(html, /ACCESS_CHANNEL_NOT_CONFIGURED/);
  equal(buttons(html), 0);
});

// Covers: 需要登录的标题、码名、缺省续办与事实区；带一颗重新登录。
test('需要登录：标题、码名、续办与事实，带一颗重新登录', () => {
  const html = renderToStaticMarkup(
    createElement(OperatorRefusalState, { kind: 'loginRequired', facts: { owner: '计价（parcel-pricing）' } }),
  );
  match(html, /需要登录/);
  match(html, /OPERATOR_CREDENTIAL_REJECTED/);
  match(html, /重新登录后再试/);
  match(html, /主责上下文/);
  match(html, /计价（parcel-pricing）/);
  equal(buttons(html), 1);
  match(html, /<button[^>]*>.*重新登录<\/button>/);
});

// Covers: 没有授予的续办指向管授予的人；不给按钮。
test('没有授予：标题、码名与续办，没有按钮', () => {
  const html = renderToStaticMarkup(createElement(OperatorRefusalState, { kind: 'notGranted' }));
  match(html, /没有授予/);
  match(html, /OPERATOR_NOT_GRANTED/);
  match(html, /要管授予的人去登记授予/);
  equal(buttons(html), 0);
});

// Covers: 行内一句带被拒的那一个动作；需要登录句尾接重新登录，没有授予不接。
test('行内一句：需要登录句尾接重新登录，没有授予不接', () => {
  const login = renderToStaticMarkup(createElement(OperatorRefusalNote, { kind: 'loginRequired', what: '预览' }));
  match(login, /预览：需要登录（401 OPERATOR_CREDENTIAL_REJECTED）。/);
  equal(buttons(login), 1);

  const grant = renderToStaticMarkup(createElement(OperatorRefusalNote, { kind: 'notGranted', what: '预览' }));
  match(grant, /预览：没有授予（403 OPERATOR_NOT_GRANTED）。/);
  equal(buttons(grant), 0);
});
