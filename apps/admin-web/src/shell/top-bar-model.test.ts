import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  ATTRIBUTION_SEPARATOR,
  COMMAND_PALETTE_ARIA_KEYSHORTCUTS,
  COMMAND_PALETTE_HINT,
  COMMAND_PALETTE_SHORTCUT_KEYS,
  COMMAND_PALETTE_TRIGGER_LABEL,
  COMMAND_PALETTE_UNWIRED_REASON,
  PRODUCT_ATTRIBUTION,
  SCOPE_JUDGED_BY_SERVER,
  SIGN_OUT_LABEL,
  USER_MENU_LABEL,
  attributionText,
  densityToggleLabel,
  scopeChipLabel,
  themeToggleLabel,
  userMenuLabel,
} from './top-bar-model';

// 本文件钉的是顶栏的字面事实（票 admin-web-ux-alignment/01）：手册「Top Bar 归属信息规范」的格式落到本产品是什么字、
// 四个位上控件的可访问名与说明各说什么。组件那半（三段怎么着色、四个位怎么摆、Tooltip 挂没挂）在 node:test 里钉不到，
// 理由见 templates/loading-shape.ts 文件头；本票用一次性 esbuild 束实测，结论写票面。

// Covers: 手册格式 `{产品} / IDP · {模块名称}`——产品名 Parcel、品牌 IDP、分隔符是间隔号 `·`（不是 `-` 或 `/`）。
test('归属信息按手册格式拼出 Parcel / IDP · 模块名称', () => {
  equal(PRODUCT_ATTRIBUTION, 'Parcel / IDP');
  equal(ATTRIBUTION_SEPARATOR, '·');
  equal(attributionText('工作台'), 'Parcel / IDP · 工作台');
  equal(attributionText('提交与撤回'), 'Parcel / IDP · 提交与撤回');
});

// Covers: 搜索位是命令面板的入口（票 admin-web-workspace-form/03 第 3 条）——可见文案「搜索或跳转…」、键帽 Ctrl K、
// 悬停说明两半都写（能搜什么 / 仍缺什么），且不写「即将上线」这类许诺。
// 改动理由：本用例原钉 GLOBAL_SEARCH_LABEL「全局搜索」与 GLOBAL_SEARCH_UNAVAILABLE_REASON「尚无跨对象搜索读口」，
// 那是第一轮 01 留位时的字面事实；位有了真动作（开面板），两常量改名改义，原断言随事实作废——不是放松，是事实变了。
// 「跨对象搜索等读口」这半句把原禁用说明的内容保留了下来：缺读口这件事今天仍然成立。
test('搜索位作为命令面板入口的文案、键帽与说明', () => {
  equal(COMMAND_PALETTE_TRIGGER_LABEL, '搜索或跳转…');
  deepEqual([...COMMAND_PALETTE_SHORTCUT_KEYS], ['Ctrl', 'K']);
  equal(COMMAND_PALETTE_ARIA_KEYSHORTCUTS, 'Control+K Meta+K');
  equal(COMMAND_PALETTE_HINT, '搜索导航与最近对象；跨对象搜索等读口');
  ok(!/即将|敬请期待|coming soon/i.test(COMMAND_PALETTE_HINT));
});

// Covers: Layout 还没接开面板动作时的留位说明——写的是「未接线」这个事实，不是假装可用。
test('未接线时的留位说明', () => {
  equal(COMMAND_PALETTE_UNWIRED_REASON, '命令面板未接线');
});

// Covers: 作用域 chip 拿不到租户名时如实写「由服务端按会话判定」；拿到了才显名字，且前缀一致。
test('作用域 chip 有租户名显名字、没有显服务端判定', () => {
  equal(scopeChipLabel(undefined), SCOPE_JUDGED_BY_SERVER);
  equal(scopeChipLabel(''), SCOPE_JUDGED_BY_SERVER);
  equal(scopeChipLabel('SYN-租户甲'), '作用域：SYN-租户甲');
  match(SCOPE_JUDGED_BY_SERVER, /^作用域：/);
});

// Covers: 切换按钮的名说的是「切过去会变成什么」，与当前态相反。
test('主题与密度切换的可访问名指向目标态', () => {
  equal(themeToggleLabel('light'), '切换到深色主题');
  equal(themeToggleLabel('dark'), '切换到浅色主题');
  equal(densityToggleLabel('comfortable'), '切换到紧凑密度');
  equal(densityToggleLabel('compact'), '切换到舒适密度');
});

// Covers: 用户菜单触发按钮的可访问名包含可见的主体名（WCAG「标签在名称中」）；主体名未到时只剩「用户菜单」。
test('用户菜单可访问名含主体名', () => {
  equal(USER_MENU_LABEL, '用户菜单');
  equal(userMenuLabel(undefined), '用户菜单');
  equal(userMenuLabel('ops@example.test'), '用户菜单：ops@example.test');
  equal(SIGN_OUT_LABEL, '退出');
});
