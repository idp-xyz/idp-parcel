// 顶栏（Top Bar）的纯逻辑：归属信息的三段与四个位上控件的字面事实（票 admin-web-ux-alignment/01 第 1、2、5 条）。
//
// 为什么单独一个 .ts：这些是手册「Top Bar 归属信息规范」「顶栏规范」「可访问性规范」落到本产品的字面事实，与 React 无关；
// 组件层在 node:test 里钉不到（理由见 preferences.ts 文件头），把字面事实抬到这里钉，TopBar.tsx 只剩摆。
// 与 preferences.ts 同款：不 import 任何 @idpxyz/* 的东西，测试编成 CommonJS 后能直接 require。

import type { DensityPreference, ThemePreference } from './preferences';

/**
 * 手册「Top Bar 归属信息规范」的 `{产品} / IDP` 那一半：产品名 Parcel。
 * 它是品牌归属的文字，与 ThemeProvider 的 product（ui-tokens 里的 accent 色键，parcel 尚未登记）是两回事——
 * 文字现在就能对齐手册，色键要等上游登记。
 */
export const PRODUCT_ATTRIBUTION = 'Parcel / IDP';

/** 归属信息中间的分隔符，手册原文用 `·`。 */
export const ATTRIBUTION_SEPARATOR = '·';

/** 归属信息整句 `Parcel / IDP · {模块名称}`。组件分三段着色渲染，整句给断言与日志用。 */
export function attributionText(moduleTitle: string): string {
  return `${PRODUCT_ATTRIBUTION} ${ATTRIBUTION_SEPARATOR} ${moduleTitle}`;
}

/** 全局搜索位的可访问名；禁用态的说明写明缺的是什么，不写「即将上线」这类许诺。 */
export const GLOBAL_SEARCH_LABEL = '全局搜索';
export const GLOBAL_SEARCH_UNAVAILABLE_REASON = '尚无跨对象搜索读口';

/**
 * 作用域 chip：本产品一会话一租户（ADR-0100——操作者主体绑定唯一租户，绑定登在服务端的操作者册，不在 id_token 里），
 * 所以它只读、不做切换器；前端拿不到租户名时如实写「由服务端按会话判定」，不编一个。
 */
export const SCOPE_JUDGED_BY_SERVER = '作用域：由服务端按会话判定';
export const SCOPE_CHIP_HINT = '本产品一会话一租户，作用域随登录会话由服务端判定，不提供切换';

export function scopeChipLabel(tenantName: string | undefined): string {
  return tenantName ? `作用域：${tenantName}` : SCOPE_JUDGED_BY_SERVER;
}

/** 切换按钮的可访问名说「切过去会变成什么」而不是「现在是什么」——现在是什么图标已经在表示，念两遍是噪音。 */
export function themeToggleLabel(theme: ThemePreference): string {
  return theme === 'dark' ? '切换到浅色主题' : '切换到深色主题';
}

export function densityToggleLabel(density: DensityPreference): string {
  return density === 'compact' ? '切换到舒适密度' : '切换到紧凑密度';
}

/** 用户菜单触发按钮的可访问名要把可见的主体名包进去，读屏念出的名字才与看到的对得上。 */
export const USER_MENU_LABEL = '用户菜单';

export function userMenuLabel(principal: string | undefined): string {
  return principal ? `${USER_MENU_LABEL}：${principal}` : USER_MENU_LABEL;
}

export const SIGN_OUT_LABEL = '退出';
