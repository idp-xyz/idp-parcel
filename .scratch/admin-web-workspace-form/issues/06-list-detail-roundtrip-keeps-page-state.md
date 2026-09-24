# 06 列表 → 详情往返保住检索词与多选集：多标签壳层下「进详情再回来即清零」的出路

Category: enhancement
Status: draft——2026-09-24 通道 1 按票 01 评审 ← 通道 2 的 Spec 非阻断 1 立。形态取舍归用户（见「判断项」），答之前不动码
Blocked by: 无
地盘：视判断项的答案而定——A 动 `apps/admin-web/src/shell/workspace-state.ts` 与 `Layout.tsx`；B 动 `Layout.tsx`；C 动 `pages/shipment-request/ShipmentRequestListPage.tsx`
（及想要同样行为的列表页）。三者都只在 `apps/admin-web/**`，走 workflow.md「前端切片」。
出处：票 01 评审 ← 通道 2 Spec 非阻断 1；票 04 第 5 条「换模块（组件卸载）即丢」。

## 为什么

票 04 给委托查阅的多选集写的承诺是「翻页 / 改检索词都不清；进详情再回来仍在」——当时列表与详情共用一个导航位，页面组件不卸载。票 01 把
`#/shipment-request-inquiry` 与 `#/shipment-request-inquiry/<id>` 认成两张标签，非活动标签卸载（`preserveInactiveTabContent={false}`），内容按标签 id 键住
（`<Fragment key={tabId}>`）：钻取时列表实例卸载，详情的「返回列表」写回列表 hash 时再挂一个新实例——检索词与多选集清零、列表重新取数。票 04 的
「换模块即丢」在票 01 之后实际是「进详情即丢」。票 01 已把 `ShipmentRequestListPage` 里两句失真的注释改成现状，行为留给本票。

## 判断项（归用户）

- **A. 同模块钻取不开新标签**：对象地址落在它的模块标签里（标签 id 只取模块段），详情在标签内切换。回到票 01 之前的单页区语义；代价是「同时开着
  一张队列和几个对象在比对」这件多标签的本意对同一模块失效。
- **B. 列表标签保活**：非活动的列表标签不卸载（按标签种类保活，或整组打开 `preserveInactiveTabContent`）。检索词、多选、滚动位置都在；代价是留着的页
  继续持有请求与内存——票 01 关掉 `preserveInactiveTabContent` 的理由正是这个。
- **C. 把页内状态抬出实例**：检索词进 hash 查询串（与保存视图的 `?view=` 同层）、多选集进按标签的 `sessionStorage`。标签照旧卸载，回来时读回；代价是每张
  想要这条行为的列表页各自接。

推荐 C 的检索词半（hash 本就是位置权威，检索词进地址还能收藏转发），多选集改口为「进详情即丢」并写进票 04——多选是一次批量动作的暂存，跨标签保留它的
场景尚未见到。这是形态取舍，等用户答。

## 不做

- 答之前不改任何行为。

## 完成判据

- 按所选方案：委托查阅「检索 → 勾选 → 双击进详情 → 返回列表」后，方案承诺保住的状态仍在（`scripts/dom-probe.mjs` 实测，结论写票面）；没承诺保住的，
  页面注释与票 04 如实写。
