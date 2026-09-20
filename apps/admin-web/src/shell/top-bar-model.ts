// 顶栏（Top Bar）的纯逻辑：归属信息的三段与四个位上控件的字面事实（票 admin-web-ux-alignment/01 第 1、2、5 条）。
//
// 为什么单独一个 .ts：这些是手册「Top Bar 归属信息规范」「顶栏规范」「可访问性规范」落到本产品的字面事实，与 React 无关；
// 组件层在 node:test 里钉不到（理由见 preferences.ts 文件头），把字面事实抬到这里钉，TopBar.tsx 只剩摆。
// 与 preferences.ts 同款：不 import 任何 @idpxyz/* 的东西，测试编成 CommonJS 后能直接 require。

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
