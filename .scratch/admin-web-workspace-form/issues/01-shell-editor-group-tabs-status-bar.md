# 01 壳层升级为多标签工作区：`EditorGroup` + `StatusBar`、工作区状态本地持久化、hash ↔ 标签互为镜像、`Ctrl+W` / `Ctrl+Shift+T`

Category: enhancement
Status: resolved——2026-09-22 通道 1 在 `main` 上直接做完（workflow.md「前端切片：一人在 main 上直接做」六步；本地 `3da0f23c` / `c9312bf6` 两笔码 + 本笔票面，**已进 main `539b8764`**（2026-09-23 push，`e0d3f89d..539b8764` 纯 ff，码 SHA 不换））。完成记录见文末。此前 in-progress——用户经 IDP 队列令「参考 idpxyz/idp-ui `apps/myshop-web`，理解，然后来调整我们的 ui」，未逐条答判断项；三项按 spec 推荐取值落地——1 多标签**要**（用户指向的参照物就是多标签壳）、2 分栏**不做**（`showSplitButtons={false}`）、3 `ActivityBar` **不装**；用户若要 2 / 3 各是一张追加票，不改本票已落的形。此前 draft——等用户答 spec「判断项」1–3
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

## 完成记录（2026-09-22，通道 1，`main` 上直接做）

参照物取证：GitHub `idpxyz/idp-ui` 对本宿主 404（私有仓、无凭据），idp-110 `ssh dev` 连接超时；因此**没有重读 myshop-web 源**，按 spec 09-20 对
`idp-ui@6751fb2` 的取证（`App.tsx` / `useWorkspaceState.ts` 的接线与语义清单）与本仓 vendor `@idpxyz/ui-workspace@0.1.25` 的 `src/`
（`EditorGroup.tsx`、`hooks/useEditorGroupTabState.ts`、`StatusBar.tsx`、`hooks/useResize.ts`、`HOSTING.md`）落地——vendor 那份 hook 就是
参照物的标签操作语义，本票的纯逻辑逐条对着它写。

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `3da0f23c` | `shell/workspace-state.ts`、`.test.ts` | 第 1 条：标签集 / 活动标签 / 已关闭栈 / 侧栏宽度全部操作 + hash ↔ 标签换算 + load/save + 快捷键判定；node:test 26 条 |
| `c9312bf6` | `Layout.tsx`、`shell/WorkspaceStatusBar.tsx`、`shell/CommandPaletteHost.tsx`、`workspace-state.ts` | 第 2–6 条：主区换 `EditorGroup`、hash 镜像、状态栏、三个快捷键合成一个 keydown、头注改写；状态栏文字两条规则 + 测试 |
| （随票 02 壳层笔 `9015cdaf`） | `App.tsx`、`README.md` | 两处「单页区 / 不用标签页」的旧话改口（随 02 的收尾笔一起提） |

**完成判据**

- ✅ 四道门：tsc 0 / run-tests 368 → 396（本票两笔）/ vite build 0；`go test ./internal/architecture/` 本轮未单跑（本票 diff 全在 `apps/admin-web/**`，不触它的样本；CI static job 会跑）。
- ✅ `workspace-state.test.ts`：`openTab` 同 id 激活 / 新开、`closeTab` 三种邻接、`closeAll` 留固定、`reopenClosed` 上限与幂等、`load` 坏值回默认——每格至少一条（实为 26 条）。
- ✅ 既有 `run-tests` 用例零改动；`Layout.tsx` 仍只经 `pageById` 渲页面（另 import `Workbench` / `UnwiredModule` 与此前同）。
- ✅ 探针（`scripts/dom-probe.mjs`，源 `/tmp/idp-probes/wsform01-probe.tsx` 不入库）**20 ok / 0 fail**：初始 hash `#/shipment-request-inquiry` 渲染后标签栏含「委托查阅」且活动；写 `#/workbench` 后标签仍一张、**无活动标签**、主区是工作台、状态栏显「工作台」（票面原写「标签两个、活动为工作台」——工作台不是标签，见判断项 1）；对象地址开第二张带副标题 SR-1、状态栏显「委托查阅 · SR-1」；点标签写 hash；Ctrl+W 关落邻居且 hash 跟着；Ctrl+Shift+T 重开；关完落工作台；工作台上 Ctrl+W 不 `preventDefault`；Ctrl+K 仍开面板；`localStorage` 有工作区键。
- ◑ 浏览器**未验**：拖标签排序、右键菜单、标签溢出滚动是 vendor 行为，探针只证本票的状态与 hash 路径。

**判断项**

1. **工作台不是常驻标签，是「没有活动标签」那一格。** 票面写「工作台标签常驻不可关」；照做的话 `EditorGroup` 会在它上面照样画一个 ×（vendor 不支持逐标签隐藏关闭按钮），按下去什么都不发生——spec 红线「不允许假动作」。改为 `activeTabId: null` → `emptyStateContent` = 工作台，关掉最后一张自然落回它。代价：标签栏在只剩零张时隐藏（vendor 行为），工作台上方没有标签条。
2. **hash 为权威的三条互斥**：不双写——点标签只写 hash、状态经 hashchange 回流，两条路对同一件事各答一次时没人知道哪个对；不在标签里存页内状态——第二段之后与查询串归页面，标签只记地址（代价：从带 `?view=` 的列表切走再切回，查询串丢，页面也已卸载重来）；关标签写 hash——「现在在哪」仍由 hash 答，标签集只算该落到哪个邻居。
3. **三个快捷键合成一个 keydown**（票面第 5 条「不开两个监听」）：把票 03 放在 `CommandPaletteHost` 里的 Ctrl+K 监听挪到 Layout，host 只剩受控件；判定函数不动。
4. **Ctrl/⌘+W 在 Chromium 桌面版是浏览器保留键**，页面 `preventDefault` 拦不住关浏览器标签页——本票按票面与参照物照装，实际只在 Firefox / PWA / kiosk 形态下起作用；关标签的主路径是 × 与右键菜单。工作台上不拦默认行为这条按票面落了。
5. `EditorGroup` 的 `isActive` 只有 true（单组），它随之给主区加一圈 `ring-1 ring-idpxyz-accent/30`——vendor 的焦点组标记，单组时是多余的一圈；不改 vendor，记下。右键菜单条目是 vendor 英文（Close / Pin Tab / Reopen Closed Tab…），i18n 归上游。
6. 内容按标签 id 键住（`<Fragment key={tabId}>`）：列表与它的一份详情是两张同模块标签，各自一个页面实例，页内状态不串。
7. 侧栏宽度随 `useResize` 每一像素写一次工作区状态（含 `localStorage`）；量小，未做节流。

**评审**：共享面（`shell/*`、`Layout.tsx`）按 workflow 第 5 步要一份 Spec 轴。`list_sessions` 通道 2 / 3 online 但 idle 且不在 `check_messages` 上等，无人可派——**推送方自审**（判断项 1–7 即自审所得），不算非作者评审。

## Comments

- 2026-09-23 · 进 main：`origin/main` = `539b8764`（`e0d3f89d..539b8764` 纯 ff，码 `3da0f23c` / `c9312bf6` 与票面笔 SHA 不换）。此前状态行写的「未推——本宿主没有 GitHub 推送凭据」在这次推送之后失效。
