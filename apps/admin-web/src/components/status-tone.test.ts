import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { statusToneBanner, statusToneSurface, statusToneText, type StatusTone } from './status-tone';

// 本文件钉状态语气的类名（票 group-legal-entities-uiux/08）：每种语气取主题运行时写进根元素的 `--ds-status-<语气>-*` 变量，
// 浅深两套各有一份配过对比度的值；任意值一律带 `color:` 类型提示——`text-[var(--x)]` 不带提示时 Tailwind 会把它读成字号。

const tones: StatusTone[] = ['success', 'info', 'warning', 'danger'];

// Covers: 字色、横幅、圆点底三份都取对应语气的主题变量，且带类型提示；横幅正文用正文色——语气字色叠在淡底上
// 不足 4.5:1（票 08 评审），语气只留在描边与底色上。
test('每种语气取主题的 --ds-status 变量并带类型提示，横幅正文用正文色', () => {
  for (const tone of tones) {
    equal(statusToneText[tone], `text-[color:var(--ds-status-${tone}-fg)]`);
    match(statusToneBanner[tone], new RegExp(`border-\\[color:var\\(--ds-status-${tone}-border\\)\\]`));
    match(statusToneBanner[tone], new RegExp(`bg-\\[color:var\\(--ds-status-${tone}-bg-subtle\\)\\]`));
    match(statusToneBanner[tone], /(^| )text-idpxyz-text( |$)/);
    equal(/--ds-status-[a-z]+-fg/.test(statusToneBanner[tone]), false);
    match(statusToneSurface[tone], new RegExp(`--ds-status-${tone}-`));
  }
});

// Covers: 不出现没有类型提示的 var() 任意值。
test('任意值都带类型提示', () => {
  const all = [...Object.values(statusToneText), ...Object.values(statusToneBanner), ...Object.values(statusToneSurface)].join(' ');
  equal(/-\[var\(/.test(all), false);
});
