import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

// 字号只用角色类（docs/design/admin-web-typography-roles.md）：页面写任意值（text-[12px]、text-[1rem]、text-[length:…]）或 Tailwind 自带档
// （text-xs、text-sm……）就会各页各取一个值、慢慢漂开——公司主体右栏比别的右栏大一档就是这样来的。`text-<角色名>` 也拦：组件库的
// tailwind-merge 会把它当颜色吃掉（配置里有原因）。这里扫源码，凡在非注释行里出现就红，红的那行点名。
// 测试在 run-tests 的产物目录里跑，cwd 是 apps/admin-web（scripts/run-tests.mjs），所以从 src 起扫。

const forbidden = /(?<![\w-])text-(\[(length:)?[\d.]+(px|rem|em)\]|\[length:[^\]]+\]|\d*xs|sm|base|lg|\d*xl|caption|body|value|section|title|display)(?![\w-])/i;

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.tsx?$/.test(entry.name) && !entry.name.endsWith('.test.ts') ? [path] : [];
  });
}

test('管理台源码不写裸像素字号，也不用 Tailwind 自带的字号档', () => {
  const offenders: string[] = [];
  for (const file of sourceFiles('src')) {
    readFileSync(file, 'utf8')
      .split('\n')
      .forEach((line, index) => {
        const trimmed = line.trim();
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('* ') || trimmed === '*') return;
        if (forbidden.test(line)) offenders.push(`${file}:${index + 1}: ${trimmed.slice(0, 120)}`);
      });
  }
  deepEqual(offenders, []);
});

// 角色类只认配置里那几档：拼错的 type-* 不生成任何样式，字号悄悄回落到继承值。名单与 tailwind.config.js 的插件同一份——加一档两处一起改。
const roles = new Set(['caption', 'body', 'value', 'section', 'title', 'display']);
// 类名边界：前后不是字母数字、连字符或冒号；排除 `type="button"`、`type: 'x'` 这类属性与字段（后面紧跟 = 或 :）。
const roleClass = /(?<![\w:-])type-([a-z][\w-]*)(?![\w=:-])/g;

test('type-* 只用角色表里的名字', () => {
  const unknown: string[] = [];
  for (const file of sourceFiles('src')) {
    readFileSync(file, 'utf8')
      .split('\n')
      .forEach((line, index) => {
        const trimmed = line.trim();
        if (trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('* ') || trimmed === '*') return;
        for (const match of line.matchAll(roleClass)) {
          if (!roles.has(match[1])) unknown.push(`${file}:${index + 1}: type-${match[1]}`);
        }
      });
  }
  deepEqual(unknown, []);
});
