# 02 左导航「我的工作」：最近对象（hash 历史）+ 保存视图（本地筛选态）两页与记录钩子；Watchlists / My Queues 归 owner

Category: enhancement
Status: ready-for-agent
Blocked by: 无（原 01——同动 `Layout.tsx`，记录钩子挂在 01 定下的壳层结构上；01 已于 2026-09-20 18:08 进 main `ed7ef224`，从 `origin/main` 起做即可）
地盘：`apps/admin-web/src/navigation.ts`（加「我的工作」分区两条目）、`Layout.tsx`（挂记录钩子；01 之后）、`page-registry.tsx`（登记两页）、
新 `apps/admin-web/src/pages/my-work/`（`RecentObjectsPage.tsx`、`SavedViewsPage.tsx`、`recent-objects.ts` / `saved-views.ts` 纯逻辑 + test）、
`pages/Workbench.tsx`（加「最近对象 / 保存视图」两块）。
出处：spec「缺口」表第二档；黄金标准「Left Navigation 黄金标准」（My Work 四项）与 Rule 6；手册「左侧导航规范」（Saved Views / Watchlists 可选）、
「Command Palette」（最近访问切换）；参照 idp-ui@53df1666 `apps/loms-web/src/loms/pages/RecentObjectsPage.tsx` / `SavedViews.tsx`（页面形态）与
`hooks/useWorkspaceState.ts`（`recentObjects` 的 localStorage 持久化，key 带产品名）。

## 为什么

黄金标准把左导航钉成「我的工作入口 + 业务入口」两层，我们只有后者。四项里两项今天就能**诚实**成立：最近对象 = 用户在本机打开过的对象
（hash 二段路由已经是对象地址，如 `#/shipment-request-inquiry/<id>`），保存视图 = 某张列表页的筛选 / 排序态存本地——两者都是 UI 自身的事实，
不需要新读口。另两项 Watchlists（关注对象并接通知）与 My Queues（按指派 / 负责人分派的工作队列）需要领域里有「关注」「指派」——
CONTEXT 与 ADR 里都没有，本仓红线不虚构，归 owner 裁要不要进领域，本票不占位。

## 要做的

1. **`navigation.ts`** 顶部加分区「我的工作」，两条目 `recent-objects`（图标 History）/ `saved-views`（图标 Star）；`moduleInfoById` 给它们 owner
   写「管理台自身（apps/admin-web）」、source 写本 spec 与黄金标准小节名——它们不是限界上下文页，工作台就绪度总览里归「已接线」（数据是本机事实）。
2. **最近对象**：`recent-objects.ts` 纯逻辑——`record({ moduleId, objectId, title, at })`、去重置顶、上限 50、`list()`、`clear()`；存 `localStorage`
   key `parcel-admin-web:recent-objects`。`Layout.tsx` 在 hash 变化且第二段在场时调 `record`（title 从 `pageTitleById[moduleId]` + objectId 拼，
   不发请求取名——对象名称属业务数据，列表页打开时自己知道、进详情页再显）。页面照 loms-web 形态：头（图标 + 标题 + 一句说明 + 「清空历史」
   `ConfirmDialog` 确认）、筛选条（搜索 + 按模块的 chip 组）、表（对象 / 模块 / 上次打开时刻 `Instant` 悬停原串）、行点即跳 hash。
3. **保存视图**：`saved-views.ts` 纯逻辑——`save({ moduleId, name, state })`、`update`、`remove`、`setDefault`、`listFor(moduleId)`；`state` 是
   **不透明的 JSON**（各列表页自己决定存什么，本票不解释）；key `parcel-admin-web:saved-views`。页面列全部视图（模块 / 名称 / 保存时刻 / 默认标记），
   行点跳 `#/<moduleId>?view=<id>`；列表页那头怎么读 `?view=` 归票 03 的 Saved View 控件（本票只提供存储与页面，03 消费）。
4. **`Workbench.tsx`** 加两块卡：「最近对象」前 8 条、「保存视图」前 8 条，各带「查看全部」跳对应页；数据来自本机存储，与就绪度总览同一口径——
   都是 UI 自身的事实，不是业务统计。
5. 隐私边界写进两份纯逻辑的头注：存的是本机浏览器；换机不带；`clear()` 是唯一删除入口。

## 不做

- 不做 Watchlists / My Queues（归 owner）；不做跨设备同步；不做服务端保存。
- 不改 `ListPageTemplate`（03）；不改任何业务页。

## 完成判据

- 三道门绿；`recent-objects.test.ts` / `saved-views.test.ts`：去重置顶 / 上限 / 清空 / 默认唯一 / 按模块列出，各至少一条正向一条边界；
  存储用可注入的 `Storage` 替身，不碰真 `localStorage`。
- 导航新增分区在「总览」之前（黄金标准：My Work 在业务导航之上）；`Workbench` 就绪度四档计数不因两条新目录而失真（它们计入「已接线」，
  票面写明理由）。
- 浏览器验收做不到如实写「未验」。

## Comments
