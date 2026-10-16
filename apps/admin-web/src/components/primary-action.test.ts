import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { primaryActionClass } from './primary-action';

// 本文件钉主动作的那串类名（票 group-legal-entities-uiux/05）。按钮本体是上游 Button，这里只守两件：实底强调色配白字
// 都在场；主题色类上不带 `/NN` 透明度——本仓主题色是裸 CSS 变量，Tailwind 不为它生成透明度变体，写了等于没写
// （票 admin-web-group-legal-entities/15 对构建产物实测），上游 default 变体正是栽在这里才看起来像禁用。

// Covers: 实底、白字、可见的聚焦环三件在场。
test('主动作是强调色实底配白字，聚焦环看得见', () => {
  match(primaryActionClass, /(^| )bg-idpxyz-accent( |$)/);
  match(primaryActionClass, /(^| )text-white( |$)/);
  match(primaryActionClass, /(^| )focus-visible:ring-2( |$)/);
  match(primaryActionClass, /(^| )focus-visible:ring-idpxyz-accent( |$)/);
});

// Covers: 任何一个主题色类都不带透明度后缀。
test('主题色类不带透明度后缀', () => {
  equal(/idpxyz-[A-Za-z]+\/\d+/.test(primaryActionClass), false);
});
