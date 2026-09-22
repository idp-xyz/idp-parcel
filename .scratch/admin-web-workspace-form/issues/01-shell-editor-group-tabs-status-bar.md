# 01 壳层升级为多标签工作区：`EditorGroup` + `StatusBar`、工作区状态本地持久化、hash ↔ 标签互为镜像、`Ctrl+W` / `Ctrl+Shift+T`

Category: enhancement
Status: in-progress——2026-09-22 通道 1 在 `main` 上直接做（workflow.md「前端切片：一人在 main 上直接做」六步）。用户经 IDP 队列令「参考 idpxyz/idp-ui `apps/myshop-web`，理解，然后来调整我们的 ui」，未逐条答判断项；三项按 spec 推荐取值落地——1 多标签**要**（用户指向的参照物就是多标签壳）、2 分栏**不做**（`showSplitButtons={false}`）、3 `ActivityBar` **不装**；用户若要 2 / 3 各是一张追加票，不改本票已落的形。此前 draft——等用户答 spec「判断项」1–3
Blocked by: 无（admin-web-ux-alignment/02 已进 main `5b032504`）
地盘：`apps/admin-web/src/Layout.tsx`（主区从单页换成 `EditorGroup`；右栏 / 底栏两个**空位**只留结构不装内容，02 装）、新 `shell/workspace-state.ts`（标签集 / 活动标签 /
已关闭栈 / 侧栏宽度的纯逻辑 + `localStorage` 持久化 + node:test）、新 `shell/WorkspaceStatusBar.tsx`（包 `StatusBar`）、`shell/preferences.ts`（若持久化键前缀要复用它的约定，只追加）。
**不碰** `shell/TopBar.tsx`（不换 `TitleBar`）、`templates/*`、任何业务页、`page-registry.tsx`、`navigation.ts`。
出处：spec「缺口」表第一档；蓝图 7 节三张母版的前提（同时开着多个对象）；参照 `idp-ui@6751fb2` `apps/myshop-web/src/App.tsx`（`EditorGroup` 接线：`onTabClick` / `onTabClose` /
`onTabPin` / `onCloseOthers` / `onCloseToRight` / `onCloseAll` / `onReopenClosed` / `onTabReorder`、`getTabRiskDot`、`emptyStateContent`；`keydown`：`Ctrl+W` 关活动标签、
`Ctrl+Shift+T` 重开）与 `hooks/useWorkspaceState.ts`（`PersistedWorkspaceState` 整份存 `localStorage`，`loadWorkspaceState` 逐字段校验坏值回默认；`handleOpenTab` 先在所有组里找同 id
再新开；`closedTabs` 上限 20）。

## 为什么

第一轮 spec 把 Workbench Tabs 列为「不做」，理由是参照物 loms-web 的 console 形态没有它、且 `Layout.tsx` 头注裁「等首个真实页面出现后再决定」。两个前提都变了：参照物换成了
myshop-web（有），真实页面早就有了（委托查阅的 hash 二段详情、04 的对象工作区母版）。蓝图母版 B / C 都假定人同时开着一张队列和几个对象在比对——单页区做不到，
每次回列表都丢对象。**但这是形态取舍不是缺陷修复**，所以判断项归用户，票面先写好等答。

## 要做的

1. **`workspace-state.ts` 纯逻辑**（不依赖 React）：`WorkspaceState = { tabs: Tab[]; activeTabId: string; closedTabs: Tab[]; sidebarWidth: number }`；
   `openTab(state, tab)`（同 id 已开 → 只激活；否则追加并激活；`closedTabs` 里同 id 的移除）、`closeTab(state, id)`（活动标签被关 → 激活右邻，没有则左邻；关最后一个 → 活动落工作台且工作台标签常驻不可关）、
   `pinTab` / `closeOthers` / `closeToRight` / `closeAll`（固定的不关）、`reopenClosed`（栈顶，上限 20）、`reorder`；`load(storage)` / `save(storage, state)` 逐字段校验坏值回默认
   （照 myshop-web `loadWorkspaceState` 的写法，不 `JSON.parse` 完直接信）。键 `parcel-admin-web:workspace`（与 01 / 02 的前缀约定同）。node:test 钉每个操作的正向 + 边界。
2. **标签 = 地址**：标签 `id` 就是 hash 路径 `<moduleId>` 或 `<moduleId>/<objectId>`（含 `?view=` 时剥掉再当 id，查询串留在 hash 上归页面）；`name` 取 `pageTitleById[moduleId]`，
   对象标签 `subtitle` = 02 的 `recentObjectTitle` 那一套（只拼字不取名）。**hash 仍是位置权威**：`hashchange` → `openTab`（新地址开新标签 / 已开则激活）；点标签 → 写 hash；
   关标签 → 写新活动标签的 hash。不双写、不在标签里另存页内状态。
3. **`Layout.tsx`**：主区换 `EditorGroup`，`renderContent(tabId)` 按 tabId 第一段查 `pageById`（同今天的 `renderActive`）；`preserveInactiveTabContent` **关**（非活动标签卸载——
   每张页各自 fetch，留着会攒请求；myshop-web 演示数据无此顾虑，我们有）；`showSplitButtons={false}`（判断项 2 推荐不做）；`emptyStateContent` = 工作台（活动落工作台时不算空）；
   `getTabRiskDot` **不接**（点的颜色要从对象状态层来，等 02 的检查器把「选中对象的状态簇」立起来再说，本票不给标签编颜色）；`beforeNavigation` 不接（没有未保存表单守卫的场景，有再加）。
   右栏与底栏：只在 flex 结构里留两个条件渲染的空位（`rightPane?: ReactNode` / 无底栏），不装内容、不加按钮——02 来装。
4. **`WorkspaceStatusBar.tsx`**：底部 `StatusBar`，`leftSlot` 显「<模块名> · <对象 id>」（活动标签地址的人话）、`rightSlot` 显密度档与主题（读两个 Provider，和 TopBar 同源）；
   `commandFeedback` 位留给 03 的面板动作反馈（03 落了再接，本票传 `null`）；`showDemoIndicators={false}`。
5. **快捷键**：`Ctrl/⌘+W` 关活动标签（工作台不可关时不拦默认行为——别把浏览器关标签页的快捷键吞了却什么都不做）、`Ctrl/⌘+Shift+T` 重开；监听挂在 Layout，与 03 的 `Ctrl+K` 同一个
   `keydown` 处置点（谁先落谁建，后者往里加 case，不开两个监听）。
6. 头注改写：原「不引入标签页与底部 / 右侧面板，等首个真实页面出现后再按实际交互决定」那句删掉，写成现状（多标签、hash 为权威、右栏底栏位空着等 02、不装 `ActivityBar` 的理由）。

## 不做

- 不装 `ActivityBar`（判断项 3，推荐不装：只有一种侧栏内容）；不做分栏（判断项 2）；不做拖标签跨组（`onTabMoveBetweenGroups`）；不做 `getTabRiskDot`。
- 不换 `TopBar` 为 `TitleBar`；不做 `BottomPanel`（spec「不做」）。
- 不改任何页面：页面看到的仍是「我被渲染在主区」，不知道自己在标签里。

## 完成判据

- 四道门绿；`workspace-state.test.ts` 覆盖 `openTab` 同 id 激活 / 新开、`closeTab` 三种邻接、`closeAll` 留固定、`reopenClosed` 上限、`load` 坏值回默认（每格至少一条）。
- 既有 `run-tests` 用例零改动；`Layout.tsx` 不再 import `Workbench` 之外的页面组件（仍只经 `pageById`）。
- 一次性 esbuild 束：初始 hash `#/shipment-request-inquiry` 渲染后标签栏含「委托查阅」；写 hash `#/workbench` 后标签两个、活动为工作台。
- 浏览器验收做不到如实写「未验」；票面判断项写清「hash 为权威」的三条互斥（不双写 / 不存页内状态 / 关标签写 hash）各自为何。

## Comments
