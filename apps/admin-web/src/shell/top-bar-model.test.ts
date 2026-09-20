import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { ATTRIBUTION_SEPARATOR, PRODUCT_ATTRIBUTION, attributionText } from './top-bar-model';

// 本文件钉的是顶栏的字面事实（票 admin-web-ux-alignment/01）：手册「Top Bar 归属信息规范」的格式落到本产品是什么字。
// 组件那半（三段怎么着色、四个位怎么摆）在 node:test 里钉不到，理由见 templates/loading-shape.ts 文件头；本票用一次性 esbuild 束实测，结论写票面。

// Covers: 手册格式 `{产品} / IDP · {模块名称}`——产品名 Parcel、品牌 IDP、分隔符是间隔号 `·`（不是 `-` 或 `/`）。
test('归属信息按手册格式拼出 Parcel / IDP · 模块名称', () => {
  equal(PRODUCT_ATTRIBUTION, 'Parcel / IDP');
  equal(ATTRIBUTION_SEPARATOR, '·');
  equal(attributionText('工作台'), 'Parcel / IDP · 工作台');
  equal(attributionText('提交与撤回'), 'Parcel / IDP · 提交与撤回');
});
