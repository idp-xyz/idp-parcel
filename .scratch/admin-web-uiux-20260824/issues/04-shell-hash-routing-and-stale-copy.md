# 04 外壳 hash 路由与两处过期未配置文案（MCP-3，补记）

Category: enhancement
Status: resolved

补记票：工作先于票面——用户经通道 3 对 UI/UX 完整性评估批「继续」后当场实现，
本票按 ../spec.md 完成纪律补簿记（票面 resolved + 提交 SHA + 验证结果）。

## 范围与落点

1. **外壳 hash 路由**（`src/Layout.tsx`）：导航位置唯一权威改为地址栏 hash
   （`#/<模块id>[/<页内子路径>]`）——刷新不回工作台、浏览器前进后退可用、模块页可
   收藏转发；未知 id 落回工作台。点导航写 hash、状态经 hashchange 回流，单一来源。
2. **委托详情深链**（`src/pages/shipment-request/ShipmentRequestListPage.tsx`）：
   详情钻取选中承载在 hash 第二段（`#/shipment-request-inquiry/<委托标识>`），
   外壳只认第一段、第二段归页面所有。
3. **两处过期文案订正**：
   - `LabelTransactionsPage.tsx`：删「后端现仅提供…两个动作端点」的端点计数
     （九端点时代已失真；计数类断言按 AGENTS 计数戒条不再写）；
   - `TrackingProjectionPage.tsx`：原「端点尚未放行」已失真——客户追踪视图读口
     （GET /customer-tracking-view）已接真；新文案如实记「本页运营查阅作用域是否
     复用该读口尚未裁决，裁决前不接线」。该裁决问题与票 03「三、追踪视图接真」的
     取证闸是同一件事，留给票 03 走。

## 地盘说明

Layout.tsx 属 7 号地盘、两处文案属 9 号地盘（见 ../spec.md 地盘表）。本工作发生在
16:57–17:00，经通道占号/释号广播各一次，无异议；两票（01/03）届时按检查单过页时
以库内版本为准。`pages/shipment-request/**` 属 MCP-3 预留地盘，无涉他人。

## 未含（显式排除，见评估报告）

- 提交/撤回页内 tab 入 hash（影响小，后补）；
- vitest 测试基建（动 package.json 与锁文件，等单独下令）；
- 取消页接线与 liveIds 登记（候第十端点真挂载，MCP-3 取消切片既定纪律）。

## 验证

- `tsc -b && vite build` 绿（2026-08-24 17:00，2685 模块）；编辑文件零 lint。
- 提交 SHA：`54ade2e`（同轮另有取消切片传输层 `391f30a`，属 MCP-3 预留地盘，非本票范围）。

## Comments

- 2026-08-25（MCP-1）：第三节第二处文案所记裁决闸已解——裁决落 ADR-0076，追踪页经 ve-operations-tracking-read 票 03 接真（对 GET /tracking-projections 取数），「裁决前不接线」文案随接线一并撤下。
