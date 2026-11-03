import { test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  COPIED_NOTE_MS,
  COPY_FAILED_NOTE,
  COPY_UNSUPPORTED_NOTE,
  CopyAside,
  RefText,
  copyFeedback,
  copyNoteTimer,
} from './copy-affordance';
import { COPIED_NOTE } from '../../components/CodeRef';

// 本文件钉复制规则（票 company-subject-ux/14）：号码类整个值是按钮、带读屏名与「点击复制」提示、等宽、悬停图标；
// 文本类的文字在按钮外、图标按钮只叫「复制<名>」；三种结局各对哪句反馈；反馈两秒后自清、连着复制重新计时。

test('号码类：整个值是按钮，读屏名「复制<名> <值>」，悬停说明带「点击复制」，等宽，值后跟复制图标与常驻的 status 格', () => {
  const html = renderToStaticMarkup(createElement(RefText, { value: 'SYN-LE-CN-SH-01', name: '编码' }));
  match(html, /^<button type="button" aria-label="复制编码 SYN-LE-CN-SH-01" title="编码：SYN-LE-CN-SH-01（点击复制）"/);
  match(html, /font-mono/);
  match(html, /<span class="\[overflow-wrap:anywhere\]">SYN-LE-CN-SH-01<\/span><svg[^>]*class="[^"]*lucide-copy[^"]*h-3\.5 w-3\.5[^"]*group-hover\/ref:opacity-100/);
  match(html, /<span role="status" class="sr-only"><\/span><\/button>$/);
});

test('文本类：文字在按钮外照常可选，末尾的图标按钮只叫「复制<名>」，整段只在 value 里', () => {
  const html = renderToStaticMarkup(
    createElement(CopyAside, {
      value: 'SYN 一行\nSYN 二行',
      name: '注册地址',
      children: ['SYN 一行', createElement('br', { key: 'br' }), 'SYN 二行'],
    }),
  );
  match(html, /^<span class="group\/copy inline-block max-w-full align-top">SYN 一行<br\/>SYN 二行<button type="button" aria-label="复制注册地址" title="复制注册地址"/);
  match(html, /group-hover\/copy:opacity-100[^>]*><svg[^>]*lucide-copy/);
  equal(html.includes('SYN 一行\nSYN 二行'), false);
  match(html, /<span role="status" class="sr-only"><\/span><\/span>$/);
});

test('三种结局各有一句：写成功「已复制」、被拒「复制失败」、没有剪贴板「无法复制」', async () => {
  equal(await copyFeedback(() => Promise.resolve()), COPIED_NOTE);
  equal(await copyFeedback(() => Promise.reject(new Error('NotAllowedError'))), COPY_FAILED_NOTE);
  equal(await copyFeedback(undefined), COPY_UNSUPPORTED_NOTE);
});

test('反馈时序：显出来、两秒后清掉；连着复制时上一回的计时作废；卸载时取消', () => {
  const shown: (string | null)[] = [];
  const pending: { run: () => void; ms: number; cancelled: boolean }[] = [];
  const timer = copyNoteTimer(
    (note) => shown.push(note),
    (run, ms) => {
      const entry = { run, ms, cancelled: false };
      pending.push(entry);
      return () => {
        entry.cancelled = true;
      };
    },
  );
  equal(COPIED_NOTE_MS, 2000);

  timer.show(COPIED_NOTE);
  deepEqual(shown, [COPIED_NOTE]);
  equal(pending.length, 1);
  equal(pending[0].ms, COPIED_NOTE_MS);
  pending[0].run();
  deepEqual(shown, [COPIED_NOTE, null]);

  timer.show(COPIED_NOTE);
  timer.show(COPY_FAILED_NOTE);
  equal(pending[1].cancelled, true);
  equal(pending[2].cancelled, false);
  deepEqual(shown, [COPIED_NOTE, null, COPIED_NOTE, COPY_FAILED_NOTE]);

  timer.cancel();
  equal(pending[2].cancelled, true);
  pending[1].run();
  deepEqual(shown.slice(-1), [null]);
});
