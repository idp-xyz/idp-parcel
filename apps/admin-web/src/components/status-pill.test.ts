import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { StatusPill } from './StatusPill';

// 本文件钉状态胶囊：码经词典译词、色调落在 data-tone 与主题变量类上；只给词时按词反查色调；任意值带类型提示。

test('码译成中文词，色调随词典', () => {
  const html = renderToStaticMarkup(createElement(StatusPill, { code: 'DELIVERED' }));
  match(html, /已签收/);
  equal(html.includes('DELIVERED'), false);
  match(html, /data-tone="good"/);
  match(html, /--ds-status-success-border/);
});

test('只给词时按词反查色调；指定 tone 覆盖词典', () => {
  const byWord = renderToStaticMarkup(createElement(StatusPill, { label: '待受理' }));
  match(byWord, /data-tone="warn"/);
  const forced = renderToStaticMarkup(createElement(StatusPill, { code: 'DELIVERED', tone: 'muted' }));
  match(forced, /data-tone="muted"/);
  match(forced, /已签收/);
});

test('分册覆盖经 scope 生效', () => {
  const html = renderToStaticMarkup(createElement(StatusPill, { code: 'REGISTERED', scope: 'work-device' }));
  match(html, /在用/);
  match(html, /data-tone="good"/);
});

test('任意值都带 color: 类型提示', () => {
  const html = renderToStaticMarkup(createElement(StatusPill, { code: 'REJECTED' }));
  equal(/-\[var\(/.test(html), false);
});
