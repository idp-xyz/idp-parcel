# 15 集团与法人页照 idp-prism 采购订单工作台重做：命令头指标、胶囊、表头排序、可拖主从、键盘、窄栏详情

Category: enhancement
Status: in-progress
Blocked by: 无
地盘：`apps/admin-web/src/templates/`（新增 `WorkbenchPageTemplate.tsx`、`workbench.ts` + test；`ListPageTemplate.tsx` 撤回
`442523f0` 加的 masterDetail）、`apps/admin-web/src/pages/party/`（集团与法人页、新详情栏、生命周期判读、列表判读、`register-list.ts`）、
`apps/admin-web/scripts/dom-probe.mjs`。
出处：2026-09-30 用户令（经 idp-mcp-3 队列）：「我们需要完全参考采购订单的 ui/ux，包括你前面发现的没实现和做的不好的部分」。
参照物：192.168.8.110 上 `/workspace/idp/idp-prism/apps/prism-web/src/pages/purchase-orders/` 与 `src/shared/workbench/`，
钉 idp-prism `cedbde95`（2026-08-31）。

## 为什么

`442523f0` 只把参照页的主从骨架搬了过来。通道 1 同日对照列出的缺口：命令头没有可点指标、刷新与主动作；状态是下拉不是带计数的
胶囊；排序是下拉不是表头；分隔条只有样式不能拖；没有 ↑/↓ / Esc / `/`；没有关注行、选中行强调条、「清除筛选」；详情是整页模板
塞进半栏；单击时壳层检查器与内嵌详情同时显示同一法人。

本票取代 [spec](../spec.md)「不做」里「不改两签结构」那一条——那条说二十余张册页同用两签、一页独改只添不一致；用户令明确要本页
照参照页做，登记因此从第二个签改为页头主动作进登记视图。其余册页不动。

## 做什么

1. **模板**：新增 `WorkbenchPageTemplate`，形态照参照页 `DocumentWorkbench` + `WorkbenchTable`：命令头（标题 + 可点指标 + 刷新
   + 主动作）· 工具条（`/` 聚焦检索 + 带计数的状态胶囊 + 右端计数）· `useSplitResize` 可拖主从 · 表头点排序（`aria-sort`）·
   关注行橙底 · 选中行左侧强调条 · ↑/↓ 换行、Esc 收栏 · 筛空「清除筛选」· 刷新收尾 toast。四态照旧走 StateSlot。
   `ListPageTemplate` 撤回 masterDetail（只有本页用过），其余 37 页零改动。
2. **页面**：指标四格（待补 / 已登记未生效 / 已生效 / 生效占比）与胶囊都数已取回的这一页、只在业务答案下显示；表头可排四列；
   单击进窄栏详情、双击或对象地址开整页对象标签；去掉检查器，不再双显。
3. **详情栏**：照参照页 `PODetailPanel`——头部（标识 + 种类 / 状态 / 修订徽章、副题、引用签）、主动作「登记资料修订」+「更多」
   菜单 + 关闭、生命周期条（已登记 → 已生效 → 已停用，停用早于生效时那段画「未经过」）、停用与待补横幅、关键事实条、三签
  （概要 / 法人资料 / 修订历史）。

## 不做（参照页有、本册没有对应事实或端点）

- 分页：读口一页答完，下推归票 04 / ADR-0144 的实施票。
- 批量勾选与批量栏：没有批量命令端点，摆出来是假动作。
- 收货、证据、协作三签；参照页 i18n。
- 其余册页不换形态。

## 完成判据

- 三道门退 0；新纯逻辑有 node:test。
- dom-probe 证事件路径：表头排序两向、指标与胶囊筛选、筛空与清除、单击开栏收列、分隔条在、↑/↓ 换行、页签条内方向键不换行、
  Esc 收栏、`/` 聚焦、刷新重取与 toast、主动作进登记视图且回来状态还在、双击开对象地址。
- 共享面评审一份（Spec 轴）。
