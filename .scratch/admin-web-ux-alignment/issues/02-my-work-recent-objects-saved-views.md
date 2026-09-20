# 02 左导航「我的工作」：最近对象（hash 历史）+ 保存视图（本地筛选态）两页与记录钩子；Watchlists / My Queues 归 owner

Category: enhancement
Status: resolved · **已进 main `5b032504`**（2026-09-20 20:13 通道 1 推；码在 main 上为 `1001d3b1` / `1bf4d305` / `7d974131`，推送方文案注释笔 `5b032504`；评审 ← 通道 2 两轴 0 阻断，见 Comments）。此前 19:5x 通道 4 交活（task-002c3bd6 接续；分支 `mcp4-ux02` 基 `origin/main` `1c04c773`，隔离树 `D:\tops\idp-parcel-mcp4-ux02`；码三笔 tip `1deb5d38`、票面笔在其上；均已推 origin）。此前 in-progress（18:2x 通道 4 认领 task-fe0d5bc2，该会话 18:4x 后无响应、六件未提交现场由接续会话 19:4x 原样入库 `89065856`）；更早 ready-for-agent
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

## 完成记录（通道 4，2026-09-20 19:5x；分支 `mcp4-ux02`，码 tip `1deb5d38`，基 `origin/main` `1c04c773`）

三笔，每笔三道门（`tsc -b --noEmit` / `run-tests` / `vite build`）+ `go test ./internal/architecture/ -count=1` 绿后推 origin（`89065856` 例外：先入库再跑门，见表）：

| 笔 | 条 | 落点 |
|---|---|---|
| `a543a289` | 第 2 / 3 / 5 条纯逻辑 | 新 `pages/my-work/recent-objects.ts`（`recordRecentObject` 去重置顶、`RECENT_OBJECTS_LIMIT` 50、`clearRecentObjects` 唯一删除入口、`recentObjectFromHash` 只认两段并剥 `?`、`recentObjectTitle` 只拼字、`recentObjectHash`）与 `saved-views.ts`（`state` 不透明、`setDefaultSavedView` 同模块唯一、`listSavedViewsFor`、`removeSavedView` 唯一删除入口、`savedViewHash` = `#/<moduleId>?view=<id>`、`savedViewIdFromHash` 供 03 消费）；键 `parcel-admin-web:recent-objects` / `:saved-views`（与 `shell/preferences.ts` 同前缀）；存储、时钟、id 生成经接口注入；第 5 条隐私边界写进两份头注。`recent-objects.test.ts` / `saved-views.test.ts` node:test 19 条。前任会话 18:32 提交 |
| `89065856` | 第 1 条 + 第 2 / 3 条页面 | `navigation.ts`「我的工作」分区在「总览」之前、`recent-objects`（History）/ `saved-views`（Star）两条目、`moduleInfoById` owner「管理台自身（apps/admin-web）」+ source 指 spec「缺口」表与黄金标准小节；`page-registry.tsx` `pageById` 登记两页；新 `pages/my-work/RecentObjectsPage.tsx`（头 + 「清空历史」`ConfirmDialog`、搜索 + 模块 chip、对象 / 模块 / 上次打开 `InstantCell`、行点跳 hash，走 `ListPageTemplate`）、`SavedViewsPage.tsx`（名称 / 模块 / 保存时刻 / 默认 `Tag`、设为默认 + 删除二次确认、行点跳 `savedViewHash`）、`shared.tsx`（`moduleTitleOf` / `ModuleChips` / `InstantCell`）、`index.ts`。**该笔是前任通道 4 会话 18:2x–18:4x 留在树上的未提交现场（mtime 18:33–18:46，截至 19:37 未响应），接续会话按 parallel-sessions「未提交现场」原样入库、一字未改**，提交信写的是证据不是结论；入库后四道门绿，无需补修笔 |
| `1deb5d38` | 第 2 条钩子 + 第 4 条 + 完成判据「计入已接线」 | `Layout.tsx` `moduleIdFromHash` 取第一段前先 `split('?')[0]`；`recordRecentObjectFromHash` 挂在已有的 hashchange effect 里、首帧也记一次，moduleId 在 `pageTitleById` 内才记，标题 `recentObjectTitle(模块名, moduleId, objectId)`，不发请求取名；`page-registry.tsx` `liveIds` 追加两页（只追加、不动既有行）+ 头注改口；`pages/Workbench.tsx` `readinessMeta.live.explain` 同口径改口，就绪度四档之上加「最近对象 / 保存视图」两卡（各前 8、`本机 N 条`、「查看全部」→ `onNavigate`、行点写 `recentObjectHash` / `savedViewHash`、空态一句实话不放样例），复用 `shared.tsx` 的 `moduleTitleOf` / `InstantCell`，头注写明本机浏览器事实、与就绪度总览同一口径、不是业务统计 |

**完成判据逐条**

- ✅ 三道门 + 架构测试绿（三笔各自）：末笔 `tsc` 0 / `run-tests` **343 pass 0 fail**（main `1c04c773` 时 324；`a543a289` +19 → 343；`89065856` / `1deb5d38` 未改纯逻辑，用例零增减）/ `vite build` 0 / `go test ./internal/architecture/ -count=1` ok。
- ✅ `recent-objects.test.ts` / `saved-views.test.ts`：去重置顶（含同标识不同模块不算同一对象）/ 上限从尾部截 / 清空唯一入口 / 默认同模块唯一且不越模块 / 按模块列出，各有正向与边界；另钉键名带产品名、坏存储当空、`recentObjectFromHash` 对一段 / 两段 / 带 `?view=` / 编码段 / 空的判读、地址与解析互逆、`savedViewIdFromHash`、页面过滤。存储用 `Map` 顶替 `RecentObjectsStorage` / `SavedViewsStorage`，时钟与 id 注入定值，不碰真 `localStorage`。
- ✅ 导航新增分区在「总览」之前：`navigationSections` 首项即「我的工作」；探针（下）断言工作台模块总览里「我的工作」分区排在首个业务分区之前。
- ✅ 就绪度四档计数不失真：两页登记进 `liveIds`，`readinessOf('recent-objects')` / `readinessOf('saved-views')` 皆 `live`；探针断言「我的工作」分区「已接线 2/2」、四档卡上的已接线总数与按 `navigationSections` × `readinessOf` 派生的值相等（在 `1deb5d38` 上为 42）。为什么计入「已接线」而不是别档，见判断项 1。
- ◑ 组件层 `renderToStaticMarkup` 断言：**一次性实测、组件层未钉**（照 01 结论：`.test.ts` 引不到 `import` 了 `@idpxyz/*` 的模块）。照 01 那套一次性 esbuild 束（`.tmp-test/ux02-probe.tsx` → `$env:TEMP\ux02-probe.cjs`，同一条 esbuild 命令），`globalThis` 垫 `window`（`location.hash` / 内存 `localStorage` / 空 `addEventListener`）。断言 **21/21**：
  有数时两卡标题在场、「查看全部」恰两处、「本机 2 条」恰两处、最近对象行标题与 `<time dateTime title>` 原串在场、保存视图行名称在场且「默认」`Tag` 恰一处、两卡排在「规划占位」卡之前、已接线释义已改口且旧句不在、「我的工作」分区 2/2、分区序、已接线总数 = 派生值、`readinessOf` 两页 = live；
  空存储时两句空态在场、无 `<time>`、「本机 0 条」两处、「查看全部」仍两处；
  外壳裹 `ThemeProvider` / `DensityProvider` / `ToastProvider` 渲 `Layout`：`#/saved-views?view=abc` 渲保存视图页（不落工作台、无「未接线」）、`#/recent-objects?view=abc` 渲最近对象页、`#/no-such-module?view=abc` 落回工作台、`#/saved-views` 无查询串照旧。源与产物不入库。
- ❌ 浏览器**未验**（AuthGate 要 gk.idp.xyz 会话；静态渲染下 effect 不跑）。未验的具体有：记录钩子实跑——hash 变到对象地址 / 首帧刷新时 `localStorage` 真被写入、StrictMode 双跑只留一条；两页行点跳转与 `ConfirmDialog` 二次确认；工作台卡行点与「查看全部」；`History` / `Star` 图标在 `Sidebar` 上渲出；`?view=` 落到模块页后模块页自身的行为（本票不读，03 后续）。

**判断项**

1. **「已接线」口径改口的理由**。原句「页面对 parcel-api 真实端点发请求」写在所有登记页要么接 parcel-api、要么是骨架 / 演示的时候。「我的工作」两页的数据是本机浏览器的 localStorage——是真实来源、不是未配置、不是合成 S，另外三档都套不上：`skeleton` 的释义是「数据区如实呈现未配置态」而这两页没有「未配置」可呈现；`demo` 是合成 S 而这里是操作者自己的行为记录；`planned` 是页面待建。不登记进 `liveIds`，`readinessOf` 会把它们判成骨架，向使用者说「未配置」——与事实反向的失真。于是释义改成「页面已接真实数据来源：parcel-api 端点，或『我的工作』两页读的本机浏览器事实」，`liveIds` 头注同口径并点明两条例外的理由；集合只追加、既有行不动。**代价**：「已接线」总数从此含两条非 parcel-api 条目，拿总数判「多少页接了 parcel-api」的人要减二——两处注释都点明了，且工作台按分区列出，「我的工作 2/2」单独可见。
2. **`moduleIdFromHash` 剥 `?` 的影响面**。改前：`#/<moduleId>?view=<id>` 的第一段是 `<moduleId>?view=<id>` 整串，查 `pageTitleById` 不中、落回工作台——`savedViewHash` 生成的地址在本票之前是死链。改后：先剥 `?` 再取段，模块页正常渲出，查询串原样留在 hash 里归模块页读（`savedViewIdFromHash`）；今天没有任何页读它，`?view=` 惰性但不再破坏路由。未知 id 带 `?` 仍落回工作台（探针）。剥的位置在 `decodeURIComponent` 之前，模块 id 都是 ASCII slug，编码过的 `%3F` 不会被误剥。**边界**：若有人手写 `#/<moduleId>/<objectId>?x=y`，外壳认模块没问题，`recentObjectFromHash` 也剥 `?` 得到干净的 objectId，但各模块页自己读第二段的那段代码（如委托查阅的详情钻取）本票未动、未核它们是否剥 `?`——本仓今天没有任何地方生成这种形状的地址（`savedViewHash` 只有一段），记在这里供 03 / 04 接 `?view=` 时一并看。
3. **记录钩子挂在外壳而不是各页**：外壳站在每次 hash 变化的必经之路上，一处记、不用各列表页与详情页各自记；放进已有的 hashchange effect 里，不另起一个 hash 来源。首帧也记一次——刷新回到详情页、从收藏直接打开，都是「打开过」；同一对象去重置顶，StrictMode 双跑与重复触发都只留一条。只记导航词表内的 moduleId：未知 id 的地址已落回工作台，历史里若留下它就成了一条查无出处的模块。标题只拼字（票面第 2 条），对象名称属业务数据、不发请求取。
4. **工作台两卡的跳转路径**：「查看全部」走 `onNavigate`（外壳注入的 `setActive`，写 `#/<id>`），行点直接写 `recentObjectHash` / `savedViewHash`——与两张页面的行点同一条路，都经 hashchange 回流到外壳，没有第二套跳转。`onNavigate` 在 props 里是可选的（既有签名），缺席时「查看全部」禁用；`Layout` 总是传。
5. **两卡取「上」**：黄金标准把 My Work 放在业务导航之上，工作台里同样放在就绪度四档之前；派单给了上下两选项，取上。工作台页头描述句从「模块就绪度总览」改成「『我的工作』与模块就绪度总览」并点明「这台浏览器的记录」，h1「IDP Parcel 租户管理台」不动。
6. **「我的工作」分区出现在工作台的模块总览里**（「已接线 2/2」，owner 显「管理台自身（apps/admin-web）」）：`Workbench` 的分区列表是「其余分区按导航原序呈现，不另造第二套分组」，不为它开例外；owner 那一栏写的是实话，不会被读成某个限界上下文。
7. **前任现场原样入库而不是「修好再提」**：六件看着已写完但没过门没提交，按 parallel-sessions「未提交现场」原样入库、一字不改、提交信只写证据（mtime、无响应起止）；入库后四道门绿，不需要补修笔。前任写的页面形态已对齐 `ListPageTemplate`（黄金标准 Rule 2），未重写。
8. **笔 B 未加 node:test**：未改纯逻辑；`moduleIdFromHash` 留在 `Layout.tsx`、没有抬成 `.ts` 纯函数——抬出去是一条新缝、超出派单，三种地址的落点由探针一次性覆盖（判据 ◑）。`recordRecentObjectFromHash` 的三个零件（`recentObjectFromHash` / `recentObjectTitle` / `recordRecentObject`）由 `a543a289` 的 node:test 钉住，组合本身未钉。
9. **01 评审 Spec 2 留给 02 的那件（`ScopeChip` 的 Tooltip 挂在非焦点元素上）本票未做**：`shell/TopBar.tsx` 不在本票地盘，派单「不碰」清单之外也没有把它派进来；仍留给首个浏览器验收的票。
10. **不做的照票面**：Watchlists / My Queues 不占位（归 owner）；不改 `templates/*`（03 地盘）、不改任何业务页、不碰共享树；`?view=` 怎么读归 03 后续。

**评审 / 推送方要看的**：`git diff 1c04c773..1deb5d38 -- apps/admin-web`（12 文件：`Layout.tsx`、`navigation.ts`、`page-registry.tsx`、`pages/Workbench.tsx`、`pages/my-work/` 下八件）；只跑 `apps/admin-web` 三道门 + `./internal/architecture/`，不需 DSN。未动 `templates/*`。

**推送方代补（评审 Spec ①）**：第 2 条票面写的页头「图标 + 标题 + 一句说明 + 清空历史」，`RecentObjectsPage` 落地时**没有图标位**——`ListPageTemplate` 的 `title` 收 `string`，加图标位要改模板、属 03 地盘且本票「不做」明写不改模板；图标只在左导航条目上（`sidebarIconMap` 的 History），页头不重复。这是有意缺省，不是漏做。

## Comments

**评审 ← 通道 2 · 钉 `86d718d8`（码 tip `1deb5d38`，基 `1c04c773`）· 20:10**（原文代落，推送方处置见后）

门（隔离树 `$TEMP\idp-review-ux02`，看完已拆）：tsc 0 / run-tests 343 pass 0 fail / vite build ok（chunk 警告为既有）/ go test ./internal/architecture/ ok。

*Standards* — **阻断**：无。**非阻断**：
① `pages/Workbench.tsx` 保存视图卡 `emptyNote` 与 `SavedViewsPage.tsx` 空态 `description` / 页描述，指引用户「在列表页过滤条上按『保存当前视图』」「行点回到列表页并带上视图」；但 `templates/list-page-structure.ts` 里 saved-view 位在所有生产页 `enabled: false`（'本页尚未接入保存视图'），`?view=` 今天也无任何页读取（票面判断项 2 自认）——说明词描述了一条今天不存在的路径。spec.md 红线「留位只允许禁用态 + 说明」要求说明本身为真。建议改口「保存视图位接通后（03 消费）…」。
② `navigation.ts` 「我的工作」分区注释「四项里的 Watchlists / My Queues」数的是上游黄金标准 Rule 6 条目（AGENTS.md「不用计数」）；spec.md 已钉 idp-ui@53df1666，低风险，可写「Rule 6 列的」。
③ 烟味（判断）：`RecentObjectsStorage`/`SavedViewsStorage` 同形接口、`filterRecentObjects`/`filterSavedViews`、`recentModuleIds`/`savedViewModuleIds` 及两份 test 的 `storageOf` 逐一重复（Duplicated Code，头注以「零依赖」自辩但同目录一份零依赖公共件即可）；`MyWorkCard` `icon: typeof History` 用具体图标给类型，`LucideIcon`/`ElementType`（`sidebarIconMap` 用法）更直说；`<h2>` 内嵌「查看全部」Button，标题被读成动作。
**无发现**：注释全中文、无行号引用，「第 N 条 / Rule N」写法与全仓同例；隐私边界两头成立——`removeItem` 仅 `clearRecentObjects`/`removeSavedView` 两处调用，`Layout` 只经 `recordRecentObject` 写，`Workbench` 只读，键前缀同 `shell/preferences.ts`；`liveIds` 头注与 `readinessMeta.live.explain` 同口径改口，票面第 1 条本就规定归「已接线」，另三档确实套不上，理由站得住（代价「已接线」总数含两条非 parcel-api，判断项 1 已写）。

*Spec* — **阻断**：无。**非阻断**：
① 第 2 条「头（图标 + 标题 + 一句说明 + 清空历史）」：`RecentObjectsPage` 页头无图标，页内注释以「模板 title 收 string、改模板属 03」自辩，与「不做」一致，但完成记录未点明此缺省——请推送方代补一句。
② 首帧 / hashchange 记录：`Layout.tsx` `recordRecentObjectFromHash` 以 `pageTitleById` 守门（该表由 `moduleInfoById` 派生 + workbench/shipment-request/template-preview 三条手写），不在词表的 moduleId **不会**被记 ✓；但守门放过所有导航 id，手改 `#/workbench/x`、`#/recent-objects/x`、`#/template-preview/x` 会记成「工作台 · x」等非对象条目。树内唯一对象地址写方是 `ShipmentRequestListPage`（`#/shipment-request-inquiry/<id>`），仅手改地址可触发；可收紧为 `pageById` 内且排除本目录两页（票面无此要求，记为判断）。
**无发现**：五条逐条——「我的工作」为 `navigationSections` 首项、在「总览」前；`recordRecentObject` 去重置顶 / `RECENT_OBJECTS_LIMIT` 50 / `recentObjectTitle` 只拼字；`SavedView.state: unknown` 不解释、`setDefaultSavedView` 同模块唯一不越模块、`savedViewHash` = `#/<moduleId>?view=<id>`；两卡 `MY_WORK_PREVIEW_LIMIT` 8 + 「查看全部」经 `onNavigate`；`sections`/`totals` 由 `navigationSections × readinessOf` 派生，四档不失真。完成判据：两份 test 用 `Map` 顶替 Storage、时钟与 id 注入，去重/上限/清空/默认唯一/按模块各有正向 + 边界。`moduleIdFromHash` 在 `decodeURIComponent` 前剥 `?`：`#/saved-views?view=abc` 落保存视图页、`#/no-such?view=abc` 落工作台、`#/m/obj?x=y` 认 m 且 `recentObjectFromHash` 得净 objectId；各模块页自读第二段是否剥 `?` 未动（判断项 2 已记）。「不做」守住：diff 12 文件无 `templates/*`、无业务页。票面 86d718d8 与码对得上：三 SHA 与 log 一致、19 = 10 + 9 条 test、343 pass 复现、`liveIds` 仅追加、h1 不动、两卡在四档之前。

**结论**：Standards 阻断 0 / 非阻断 3；Spec 阻断 0 / 非阻断 2。两轴最重皆为①（说明词 / 记录缺省），均可合入后补。

**处置（通道 1，20:1x）**：Standards ① / ② 推送方代落一笔 `5b032504`（`Workbench.tsx` `emptyNote`、`SavedViewsPage.tsx` 页描述与空态 `description` 三句改成「『保存当前视图』位今天还未接通；接通后…」；`navigation.ts` 头注「四项里的」→「Rule 6 列的」；只改文案与注释，`git diff -U0` 四行）。Standards ③ 三条记不改：两份纯逻辑的同形接口与过滤 / 模块列举是**有意各写一份**（头注已辩），抽公共件等第三个同形消费者出现再做；`icon: typeof History` 与 `<h2>` 内嵌按钮两处归下一张碰 `Workbench.tsx` 的票顺手。Spec ① 已代补一句（上方「推送方代补」）。Spec ② 记为判断项 11：守门收紧到 `pageById` 内且排除「我的工作」两页——今天只有手改地址才触发，等第二个对象地址写方出现时一并收；不在本票改。

**进 main 记录**：隔离树 `%TEMP%\idp-land-ux02` 在 `11e6111a` 上 cherry-pick `1c04c773..86d718d8` 五笔零冲突（`85beb1aa→01a09eec` / `a543a289→1001d3b1` / `89065856→1bf4d305` / `1deb5d38→7d974131` / `86d718d8→2ddaea5d`），`git diff origin/mcp4-ux02 2ddaea5d -- apps/admin-web .scratch/admin-web-ux-alignment` 为空；推送方代落 `5b032504`。在 `5b032504` 上实跑：tsc 0 / run-tests 343 / vite 0（dist 含新文案）/ `gofmt -l` 空 / build 0 / vet 0 / 清点重生成零差 / `go test ./internal/architecture/` ok / 带 DSN 全量 `-p 1 -count=1` 20:10:31→20:12:50 **115 ok / 0 FAIL / 16 无测试 / 0 cached**，探针 `TestFreezeScopesAreInvisibleToEachOther -v` PASS。20:13:02 `ls-remote` 核 `11e6111a` 未动 → `push 5b032504:main` 成，远端 main = `5b032504`（六笔：02 五 + 推送方一；其下无他人提交）；共享树 `merge --ff-only` 同 SHA。
