# 01 壳层升级为多标签工作区：`EditorGroup` + `StatusBar`、工作区状态本地持久化、hash ↔ 标签互为镜像、`Ctrl+W` / `Ctrl+Shift+T`

Category: enhancement
Status: resolved——2026-09-22 通道 1 在 `main` 上直接做完（workflow.md「前端切片：一人在 main 上直接做」六步；本地 `3da0f23c` / `c9312bf6` 两笔码 + 本笔票面，**已进 main `539b8764`**（2026-09-23 push，`e0d3f89d..539b8764` 纯 ff，码 SHA 不换））。完成记录见文末；非作者评审 ← 通道 2（2026-09-24）Spec 阻断 1（`loadWorkspaceState` 遇畸形 id 抛而不回默认，整页白屏）已修 `d365851b`，各条非阻断逐条处置见 Comments「评审后修复」（本地 `main`，**未推**——本宿主此刻连不上 GitHub 代理）。此前 in-progress——用户经 IDP 队列令「参考 idpxyz/idp-ui `apps/myshop-web`，理解，然后来调整我们的 ui」，未逐条答判断项；三项按 spec 推荐取值落地——1 多标签**要**（用户指向的参照物就是多标签壳）、2 分栏**不做**（`showSplitButtons={false}`）、3 `ActivityBar` **不装**；用户若要 2 / 3 各是一张追加票，不改本票已落的形。此前 draft——等用户答 spec「判断项」1–3
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

### 评审 ← 通道 2 · 钉 `c9312bf6`（基 `e0d3f89d`，共享树只读，门禁未重跑）· 2026-09-24 12:4x（推送方自任务台 `task-75730420` 代落原文）

门（未重跑）：引通道 1 在 `3a47da8d` 实跑（Node 22.23.3）`tsc -b --noEmit` 0 / `run-tests` 406 pass 0 fail / `vite build` 成功；CI 在 `539b8764` 与 `3a47da8d` 七个 job 全绿。本评审只用 `git show` / `git diff` / 读文件；vendor 行为对读已装的 `@idpxyz/ui-workspace@0.1.25` 源（`EditorGroup.tsx`、`StatusBar.tsx`、`hooks/useEditorGroupTabState.ts`、`hooks/useResize.ts`）。运行期才能证的标「未实证」。范围：`git diff e0d3f89d c9312bf6 -- apps/admin-web`（`Layout.tsx`、`shell/workspace-state.ts` 与其 test、`shell/WorkspaceStatusBar.tsx`、`shell/CommandPaletteHost.tsx`）+ `git show 9f95520e -- apps/admin-web/src/App.tsx apps/admin-web/README.md`；每条发现另核了在 `3a47da8d` 上是否仍在。

**Standards** — 阻断：无。非阻断：
1. `Layout.tsx` 文件头「点标签、关标签、重开都只写 hash，状态经同一条 hashchange 回流」与代码不符。只有点标签（`onTabClick` → `navigate`）是只写 hash；关 / 重开 / 关其它 / 关右侧 / 全关都经 `applyWorkspace`，先 `setWorkspace(next)`（连同新的 `activeTabId`）再写 `hashForTab(next.activeTabId)`，回流时 `openTab` 已开则激活、幂等——同文件 `applyWorkspace` 的文档注释写的才是实情。两路对 `activeTabId` 各答一次，靠的是 `tabIdFromHash(hashForTab(id)) === id`；这个前提对存储读回的不规范 id 不成立（见 Spec 阻断 1）。改头注那一句即可；`3a47da8d` 上仍是原句。
2. 注释里带着变更叙事：`Layout.tsx` 文件头「第一轮裁…的前提已变——真实页面早就有了…参照物也换成了多标签壳」、`App.tsx` 头注「此前沿 loms-web 的单页区形态…真实页面早已有了」、`workspace-state.ts` 的 `SIDEBAR_WIDTH_DEFAULT` 注「与 Layout 此前写死给 useResize 的同值」、`tabIdFromHash` 注「与 Layout 此前对未知 id 的处置一致」。票面第 6 条要求「写成现状」，AGENTS「写代码注释」也不写变更说明。取舍理由（蓝图母版 B / C 假定同时开着多个对象）留下，「此前 / 前提已变」删掉不丢信息。
3. node:test 小缺口：`loadWorkspaceState` 把 `closedTabs` 截到 20 这一步没钉（`closeTab` → `pushClosed` 那条路径钉了）；`closeOthers` / `closeToRight` 遇到不认识的 id 原样返回也没钉。完成判据「每格至少一条」已满足，补不补由作者定；Spec 阻断 1 的用例另算。

无发现（实核）：新注释全中文。跨文件引用都用符号名或文件头（`templates/loading-shape.ts` 文件头、`shell/command-actions.ts` 的 `isOpenCommandPaletteShortcut`、`pages/my-work/recent-objects.ts` 的 `recentObjectTitle`、`top-bar-model` 的 `themeToggleLabel`、`Layout.tsx` 文件头），逐个对读，都存在且所述属实；无行号。「三条互斥」「两处刻意不同」「只喂它真实的两件事」「三值」都是紧接着就地列出的本处条目，不是在数别处。`workspace-state.ts` 零依赖（运行时不 import `@idpxyz/*`、不 import `navigation.ts`），导航词表由调用方注入，与 `command-actions.ts` 同款。键 `parcel-admin-web:workspace` 与 `preferences.ts` / `recent-objects.ts` / `saved-views.ts` 同前缀。storage 读写不包 try/catch，与这三个邻居一致。纯逻辑 node:test 在 `3da0f23c` 26 条、`c9312bf6` 加状态栏 2 条共 28 条，与 run-tests 368 → 396 对得上。

**Spec** — 阻断：
1. **`loadWorkspaceState` 有一类坏值不回默认而是抛——第 1 条「逐字段校验坏值回默认」与完成判据「`load` 坏值回默认」在这一格不成立。** `sanitizeTabs` 对形对的标签调 `moduleIdOfTab(item.id)`，其中 `decodeURIComponent` 遇畸形百分号序列（id 为 `%`、`%E0` 之类）抛 `URIError`；`loadWorkspaceState` 的 try 只包了 `JSON.parse`，异常一路穿出 `Layout` 的 `useState(workspaceFromStorageAndLocation)` 初始化器。`main.tsx` → `AuthGate` → `App` → `Layout` 一路没有错误边界，于是整页白屏；坏值留在 localStorage，**每次刷新都白屏**，操作者只能自己去开发者工具清站点数据。同一处还放过两类不规范的 id：一类是 `exception-cases/`、`shipment-request-inquiry/SR-1/x` 这种尾斜杠或三段的，能活过 load，但点它写出的 hash 经 `tabIdFromHash` 会认成另一个 id，结果激活或另开那张规范 id 的标签、原标签永远激活不了（点了不动，与判断项 1 所避的假动作同类）；另一类是次段编码畸形的，load 放过，点那张标签时 `objectIdOfTab` 才抛、同样白屏。触发只能是外部写坏这个键——本应用自己的写路径在 `tabForHash` 就先抛了，存不进去——但这正是本条要防的情形，而后果是最坏的一种：自家 docstring 说「存坏了半格不该把所有标签都吞掉」，实际是整台吞掉。修法在本票地盘内：`sanitizeTabs` 对 id 只做一道「规范且可解码」校验——`tabIdFromHash(hashForTab(id), isKnownModule) === id`，并在同一个 try 里跑一遍 `objectIdOfTab(id)`，抛即剔除（词表外、工作台伪标签、不规范、编码畸形四类一并收掉）；补一条 load 用例（首段畸形、尾斜杠各一）。`3a47da8d` 上仍在（票 02 的 `9015cdaf` 没动 `sanitizeTabs` / `moduleIdOfTab`）。附带一句：hash 路径上的同类抛（`tabIdFromHash` / `objectIdOfTab`）在基 `e0d3f89d` 上已由 `recentObjectFromHash` 与 `ShipmentRequestListPage` 的 `selectedIdFromHash` 同样存在，不算本票引入，改地址即可恢复；抽一个共用的安全解码可以一并收掉。

非阻断：
1. **委托查阅「列表 → 详情 → 返回列表」往返后检索词与多选集清零、列表重新取数——票 04 的承诺被本票静默打破，完成记录未记。** `ShipmentRequestListPage` 的 `keyword` / `checked` 是页面 state，其注释写着「列表与详情共用一个导航位」与「进详情再回来仍在（本组件不卸载，只是整区切成详情）」。本票按第 2 条把 `#/shipment-request-inquiry` 与 `#/shipment-request-inquiry/<id>` 认成两张标签，再加 `preserveInactiveTabContent={false}` 与 `renderTab` 的 `<Fragment key={tabId}>`：钻取时列表实例卸载、详情是新实例，`onBack` 写回列表 hash 时又挂一个新实例。于是这两句注释在 `c9312bf6` 之后为假，票 04 第 5 条的「换模块（组件卸载）即丢」实际成了「进详情即丢」。判断项 2 的代价句只写了「切走再切回…页面也已卸载重来」，判断项 6 把「页内状态不串」当收益，都没点名这条既有路径的退化。静态对读可定，运行期未实证；`3a47da8d` 上仍在（票 02 改成双击或检查器快捷动作开详情，走的是同一条 hash 路径）。不列阻断，理由：它是第 2、3 条字面设计的直接后果；修法要么动页面（本票「不做」），要么改壳层取舍（同模块钻取不开新标签、列表标签保活、或把检索词与选择集抬进 hash / 按标签的会话存储），归用户定。建议：票面判断项补记这条退化；立追加票，至少把 `ShipmentRequestListPage` 那两句注释改成现状。
2. 标签名存了第二份：`sanitizeTabs` 把存储里的 `name` / `subtitle` 原样留下。导航词表改名后，恢复出来的非活动标签显旧名，要被点一次（`openTab`）才刷新。`name` 可以由 `pageTitleById[moduleIdOfTab(id)]` 现算、`subtitle` 由 `objectIdOfTab(id)` 现算，存储只存 `id` / `pinned`——与本仓「页面标题的唯一来源是 navigation」一致（`ShipmentRequestListPage` 就是这么注的），也让阻断 1 的校验面更小。
3. 判断项 1 用「按了没反应就是假动作」否掉了常驻工作台标签。拿同一把尺子量 vendor 右键菜单：「Reopen Closed Tab」在空栈时照样可点（vendor 没有禁用口），点了之后 `reopenClosed` 返回原引用、`applyWorkspace` 早退，界面无变化；标签全是固定的时候「Close All」也一样。接受——功能已实现、只是此刻没有可作用的对象，属合法空操作，与永远按不动的 × 不同类。建议判断项 1 补一句这个区分，免得后来者拿同一条红线两头拉。
4. 完成记录漏记两处对票面字面的偏离（都接受）。第 3 条要求「右栏在 flex 结构里留条件渲染空位（`rightPane?: ReactNode`）」，`c9312bf6` 没留，头注改说「随票 02 装」（`9015cdaf` 随即装上，无害）。第 4 条要求「`rightSlot` 显密度档与主题」，实现只显主题词、密度交给 vendor `StatusBar` 自带的切换钮（`WorkspaceStatusBar` 文件头写了理由；实核 vendor 无条件渲染读 `useDensity` 的那个钮，理由成立）。该钮文字是英文 compact / comfortable，与判断项 5 里右键菜单的英文同归上游 i18n。
5. 点当前活动标签本身也走 `navigate(hashForTab(id))`，会把 hash 上的 `?view=` 或第三段剥掉，而页面实例不换（key 未变），hash 与屏上所示可能失配。今天 `savedViewIdFromHash` 没有消费者、也没有页用第三段，影响未实证；`onTabClick` 遇到 `activeTabId` 早退即可。
6. 落点表「（随票 02 壳层笔 `9015cdaf`）」那一行 SHA 指错：`git show --stat 9015cdaf` 不含 `App.tsx` / `README.md`，这两处改口实随 `9f95520e` 提交（该笔提交说明也这么写）。改口本身核过：`App.tsx` 头注与 README「技术栈」一句都改成多标签工作区现状，理由指回 `Layout.tsx` 文件头、不复述。

无发现（实核）——完成记录里偏离票面字面的取舍逐条判：
1. **工作台不做常驻标签 → 接受。** vendor `Tab` 没有逐标签关闭开关，× 对每张标签无条件渲染（固定标签也有）；`activeTab: ''` 匹配不到时 vendor 渲 `emptyStateContent`，零张时整条标签栏不渲染（判断项 1 所述代价属实）。`closeTab` 关最后一张、`closeAll` 关掉活动标签，都落 `null` 回工作台，没有按不动的钮。
2. **hash 为权威的三条 → 接受**（头注措辞见 Standards 1）。点标签 `onTabClick` → `navigate(hashForTab(id))`，不 setState，经 hashchange → `applyHash` → `openTab` 已开则激活。`WorkspaceTab` 只有 `id / name / subtitle? / pinned?`，`tabIdFromHash` 剥查询串与第三段起，不存页内状态。关标签经 `applyWorkspace`，活动标签变了才写新活动标签的 hash；关非活动、固定、拖排都不碰 hash，所以带 `?view=` 时不会被冲掉。
3. **三个快捷键合成一个 keydown → 接受。** `CommandPaletteHost` 的监听整段删、`useEffect` import 随删；Layout 一个 `keydown` 先判 `isOpenCommandPaletteShortcut`（`command-actions.ts` 在本范围零改动，判定函数没动），再判 `workspaceShortcutOf`，两个谓词不相交。工作台上 `activeTabId === null` 时在 `preventDefault` 之前 return；空栈时 Ctrl+Shift+T 同样不拦（票面没要求，合理）。
4. **`preserveInactiveTabContent={false}`、`showSplitButtons={false}` → 接受。** vendor 只在 `showSplitButtons &&` 块内调 `onSplitRight` / `onSplitDown`，所以空函数不可达，注释说它们「声明成必填」属实。`beforeNavigation` 没传。`getTabRiskDot={() => null}` 是显式传了恒 null：vendor 默认的 `defaultGetTabRiskDot` 读 `tab.riskLevel`，而 `tabForHash` / `sanitizeTabs` 构出的标签从不带这个字段，结果与「不接」相同，显式传更稳。`onActivate={() => {}}` 单组无可切、也没有可见入口，不算假动作。判断项 5 实核：`isActive` 若给 false，选中标签的强调样式会一起没掉，只能留那圈 ring。
5. **load 逐字段、closedTabs 上限 20 → 除阻断 1 那一格外成立。** `JSON.parse` 包了 try，非对象回初始。`tabs` / `closedTabs` 逐项过 `isWorkspaceTab`、去重、剔除词表外和工作台伪标签；`activeTabId` 不在集里落 `null`；`sidebarWidth` 非数回默认、越界钳进区间；`closedTabs` 去掉已开着的、去掉固定标记、截到 20；`pushClosed` 同样截 20。
6. **判断项 1–3 → 如实写进了票面。** 票面 Status 行写明「未逐条答判断项；三项按 spec 推荐取值落地」「用户若要 2 / 3 各是一张追加票」，spec Status 同口径。

其余：「不做」守住了——范围 diff 只有 `Layout.tsx`、`shell/workspace-state.ts` 与其 test、`shell/WorkspaceStatusBar.tsx`、`shell/CommandPaletteHost.tsx`，没碰 `TopBar.tsx` / `templates/*` / 业务页 / `page-registry.tsx` / `navigation.ts`，没装 `ActivityBar` / `BottomPanel` / `TitleBar`，没接 `onTabMoveBetweenGroups`。红线：`showDemoIndicators={false}`（vendor 那组确是「142 Active / All Carriers Online…」样板字）、`commandFeedback={null}`，状态栏只显位置与主题这两样真实的东西；本票不动模板，不涉向后兼容。`closeTab` 允许直接关固定标签（理由属实：vendor 在固定标签上照样画 ×）。`reorderTabs` 与 vendor `onTabReorder` 同算法，关闭进栈去固定标记也与 vendor 同；关掉后落右邻、没有右邻才落左邻，照票面（vendor 是落最后一张）。`<Fragment key={tabId}>` 让同模块两张标签各一个实例（判断项 6）。`useResize` 每次 mousemove 都 `setSize`，经 effect 写进工作区状态再落 localStorage（判断项 7 属实）。既有 run-tests 用例零改动（范围内唯一的 test 文件是新增的）；`Layout.tsx` 渲页面仍只经 `pageById`（另 import `Workbench` / `UnwiredModule`，与基同）。esbuild 束判据按判断项 1 改了口，探针源不入库，自报 20 ok 未复核；浏览器「未验」如实写了。

**Standards 0 / 3 · Spec 1 / 6**

结论：须修——Spec 阻断 1（`sanitizeTabs` 的 id 校验补「规范且可解码」，加一条 load 用例；改动限 `shell/workspace-state.ts` 与其 test）。修完只需重跑 Spec 轴那一格。Spec 非阻断 1 建议立追加票并在票面补记。

**处置**（通道 1）：本笔只落原文，不改码。用户 2026-09-24 令「全面收掉」：阻断 1 与各条非阻断随后在 `main` 上逐笔修，逐条处置记在下方「评审后修复」。

### 评审后修复（2026-09-24，通道 1，`main` 上直接做）

**落点**

| 笔 | 文件 | 收的是 |
|---|---|---|
| `376b6ea1` | 票面 | 代落通道 2 评审原文 |
| `d365851b` | `shell/workspace-state.ts`、`.test.ts`、`Layout.tsx` | Spec 阻断 1；Spec 非阻断 2；Standards 非阻断 3 |
| `06b1f87e` | `Layout.tsx`、`App.tsx`、`shell/workspace-state.ts` | Standards 非阻断 1 / 2；Spec 非阻断 5 |
| `2056e054` | `pages/shipment-request/ShipmentRequestListPage.tsx` | Spec 非阻断 1 的注释半 |
| 本笔 | 票面、追加票 06、spec 子票表 | Spec 非阻断 1 的追加票；Spec 非阻断 3 / 4 / 6（票面口径）；本段 |

**逐条处置**

- **Spec 阻断 1 → 已修。** `sanitizeTabs` 对每张存储标签按 id 经 `tabForHash(hashForTab(id))` 重造（`tabFromStoredId`），造不出、认回来不是同一个 id、解码抛
  `URIError` 的一律剔除——畸形百分号、尾斜杠、三段、带查询串、次段畸形与原有的词表外、工作台伪标签走同一道门。评审给的修法是在现有校验上再补一道
  「规范且可解码」；这里改为直接用 hash 开标签的那一个构造器：两条来路（hash 开标签 / 存储恢复标签）共用一个构造器，规范性由构造保证，不用两处各写一份
  校验。load 用例一条（五类坏 id + 已关闭栈里的坏 id）。
- Standards 非阻断 1 → 已改：`Layout.tsx` 头注按实际两条路写——点标签只写 hash；关 / 重开 / 批量关先落状态、活动标签变了再写 hash，两次答成同一张靠
  id 规范，存储读回的标签由 `loadWorkspaceState` 按这一条筛过。
- Standards 非阻断 2 → 已删：`Layout.tsx` 头注「第一轮裁…前提已变」、`App.tsx` 头注「此前沿 loms-web…」、`SIDEBAR_WIDTH_DEFAULT` 注「Layout 此前写死」、
  `tabIdFromHash` 注「与 Layout 此前…一致」。取舍理由（母版 B / C 同时开着多个对象）留下；`workspace-state.ts` 文件头与 `Layout.tsx` 头注各一个「仍」顺手删。
- Standards 非阻断 3 → 已补：`load` 把已关闭栈截到上限、`closeOthers` / `closeToRight` 遇未知 id 原样返回，各一条。
- Spec 非阻断 1 → `ShipmentRequestListPage` 两句失真注释改成现状（列表与详情各一张标签、进详情即丢）；行为的出路立追加票
  [06](./06-list-detail-roundtrip-keeps-page-state.md)，形态取舍归用户。判断项补 8（见下）。
- Spec 非阻断 2 → 已改：标签名与副标题由词表与 id 现算，存储里那一份不读；`loadWorkspaceState` 第二参由 `isKnownModule` 改收 `pageTitleById`（与
  `tabForHash` 同形）。存储格式不变：`saveWorkspaceState` 仍整份写，多出的 `name` / `subtitle` 只是不再被读，旧存储照读。
- Spec 非阻断 3 → 判断项 1 补一句（见下）。
- Spec 非阻断 4 → 完成记录补载两处偏离（见下）。
- Spec 非阻断 5 → 已改：`onTabClick` 遇已活动标签不写 hash。
- Spec 非阻断 6 → 落点更正（见下）。

**判断项补记**

- 补判断项 1：「按了没反应就是假动作」量的是**永远**作用不到任何对象的控件。右键菜单「Reopen Closed Tab」在空栈时、「Close All」在全是固定标签时点了
  无变化，是功能在、此刻没有可作用的对象——合法空操作，不按这条红线判。
- 8. **列表 → 详情往返丢检索词与多选集**：票 04 第 5 条「换模块（组件卸载）即丢」在本票之后实际是「进详情即丢」——列表与详情各一张标签、非活动标签卸载、
  内容按标签 id 键住。判断项 2 的代价句与判断项 6 的「页内状态不串」都没点名这条退化；出路归追加票 06。

**完成记录补载的偏离**（都接受）：第 3 条「右栏在 flex 结构里留条件渲染空位（`rightPane?`）」本票没留、头注写「随票 02 装」，`9015cdaf` 随即装上；第 4 条
「`rightSlot` 显密度档与主题」实现只显主题词，密度交给 vendor `StatusBar` 自带的切换钮（`WorkspaceStatusBar` 文件头写了理由），钮字英文 compact /
comfortable，与右键菜单的英文同归上游 i18n。

**落点更正**：完成记录落点表「（随票 02 壳层笔 `9015cdaf`）」那一行应为 `9f95520e`——`App.tsx` / `README.md` 两处改口随它提交。原表不改写，以此为准。

**门**（钉 `2056e054`；WSL 上跑，Node 22.20.0——本机没有 22，临时从 npmmirror 取官方包、sha256 校验过，只放 `/tmp` 不装进系统；本机自带的 Node 20.18.2
其 `node --test` 不认 glob，跑不了 `run-tests`）：`tsc -b --noEmit` 退 0 / `run-tests` 406 → 414 pass 0 fail（票 01 这边 +4）/ `vite build` 成功。
`go test ./internal/architecture/` 本机未跑：WSL 的 go 要先下 go1.26.5 工具链，出不了网；diff 全在 `apps/admin-web/**`，由 CI 兜。

**探针**（`scripts/dom-probe.mjs`，源 `/tmp/idp-probes/wsform-review-fixes-probe.tsx` 不入库，与票 02 共用一份）：修复后 **24 ok / 0 fail**。同一份探针对修复前的
`376b6ea1`（临时工作树，已拆）**17 FAIL**，本票相关的几项：存储里有畸形百分号 id 时 App 首帧抛 `URIError: URI malformed`、整棵树不渲染；点已活动标签把
`#/exception-cases?view=v1` 剥成 `#/exception-cases`。修复后：坏存储下首帧不抛、只剩规范的那张标签、标签名按词表现算；点已活动标签 hash 不变、点别的
标签照写。

**评审**：修复碰共享面（`shell/*`、`Layout.tsx`），按 workflow 第 5 步要一份 Spec 轴；评审点名「修完只需重跑 Spec 轴那一格」。复核已派回通道 2（任务台，
结果回来代落）；在此之前以**推送方自审**为准，不算非作者评审。自审所得：评审列的四类（词表外、工作台伪标签、不规范、编码畸形）与尾斜杠、三段、带查询串
都在 load 用例或探针里；`loadWorkspaceState` 改签名只有 `Layout` 一处调用；存储格式不变、旧存储照读；`tabForHash` 构出的 id 本就规范，`openTab` 那条路不受影响。

**推送**：未推。本宿主（WSL）仓库级 `http.proxy` 指向 `127.0.0.1:7897`，此刻连接被拒；直连 GitHub 超时。本地 `main` 比 `origin/main`（`3a47da8d`）多出两票的评审代落笔、
修复笔与票面笔，推送后在此补一行进 main 记录。
