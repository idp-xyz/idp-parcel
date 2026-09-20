import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import {
  ATTRIBUTION_SEPARATOR,
  GLOBAL_SEARCH_LABEL,
  GLOBAL_SEARCH_UNAVAILABLE_REASON,
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

// Covers: 全局搜索留位不留假动作——可访问名是「全局搜索」，禁用说明写的是缺什么（读口），不是许诺。
test('全局搜索位的名与禁用说明', () => {
  equal(GLOBAL_SEARCH_LABEL, '全局搜索');
  equal(GLOBAL_SEARCH_UNAVAILABLE_REASON, '尚无跨对象搜索读口');
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
