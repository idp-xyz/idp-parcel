# 03 命令面板 `Ctrl+K`：导航 + 最近对象 + 壳层开关；TopBar 全局搜索位改为面板入口

Category: enhancement
Status: resolved · 已进 main `6739ab54`（2026-09-21 12:09 push，纯 ff，SHA 不换；进 main 记录见 Comments）——2026-09-21 11:5x 推送方通道 1 代落完成记录（作者通道 6 会话 09-20 21:00 后无响应，`Layout.tsx` 现场原样入库为 `938a2a54`）；码 tip `938a2a54`，基 `87ff1edd` = main。此前 in-progress——2026-09-20 20:1x 通道 6 认领（task-9c302082，通道 1 20:08 派；分支 `mcp6-wsform03`，树 `D:/tops/idp-parcel-mcp6-wsform03`，基 origin/main `11e6111a`，09-20 20:5x 后 rebase 到 `87ff1edd`）。此前 ready-for-agent
Blocked by: admin-web-ux-alignment/02 进 main（读它的 `pages/my-work/recent-objects.ts` 存储；`Layout.tsx` 同动，等它先落）
地盘：新 `apps/admin-web/src/shell/command-actions.ts`（动作集纯逻辑 + node:test）、`shell/CommandPaletteHost.tsx`（挂 `@idpxyz/ui-workspace` 的 `CommandPalette` + 键盘监听）、
`Layout.tsx`（渲染 host，一行）、`shell/TopBar.tsx` + `shell/top-bar-model.ts`（全局搜索位：禁用态 → 「打开命令面板」按钮，`Ctrl+K` 提示进 Tooltip）。
**不碰** `templates/*`、任何业务页、`pages/my-work/*`（只 import 它的读函数）。
出处：spec「缺口」表第三档；蓝图 23.1 `CommandPalette`；黄金标准「Command Palette」最近访问切换；手册「顶栏规范」Global Search 位；参照 `idp-ui@6751fb2`
`apps/myshop-web/src/commandActions.ts` `buildDefaultCommandActions`（三组：navigation「打开 X」/ recent「打开 <对象>」前 10 / actions 布局切换与快捷动作）与 `App.tsx`
的 `keydown` 监听（`Ctrl/⌘+K` 开、`Escape` 关）。

## 为什么

第一轮 01 把全局搜索位留成禁用态，理由是没有跨对象搜索读口——今天仍然没有。但命令面板不是搜索：它搜的是**本机已知的事实**——导航词表（`pageTitleById`）、02 存下的
最近对象、壳层自己的开关——三样都不发请求。myshop-web 就是这么做的（`commandActions.ts` 一个网络请求都没有）。面板落下后，搜索位有了真动作，「留位」那句注释可以改成实话。

## 要做的

1. **`command-actions.ts` 纯逻辑**：`buildCommandActions({ pageTitleById, recentObjects, shellToggles })` → `CommandPaletteAction[]`（类型取 `@idpxyz/ui-workspace`）：
   - `navigation` 组：导航词表每条一个「打开 <页名>」，`keywords` = `[id, 页名]`，`run` 写 hash `#/<id>`（走 Layout 同一条路，不另起跳转）；工作台也算一条。
   - `recent` 组：`listRecentObjects(storage)` 前 10 → 「打开 <title>」，`run` 写 `recentObjectHash(entry)`；`keywords` = `[objectId, title, 模块名]`。
   - `shell` 组：切换主题（`useTheme`）、切换密度（`useDensity`）——只这两个，检查器 / 底栏切换等票 01 / 02 落了再加，本票不留空动作。
   - **没有**「创建 / 导出 / 刷新」这类假快捷动作（myshop-web 的 `onFeedback('功能开发中')` 一条不搬——红线不允许假动作）。
   node:test 钉：词表 N 条 → navigation N 条；最近对象 12 条 → recent 10 条；关键词含 id 与页名；空存储 → recent 0 条不报错。
2. **`CommandPaletteHost.tsx`**：`open` state + `keydown`（`Ctrl/⌘+K` 开并 `preventDefault`，`Escape` 关——`CommandPalette` 自己若已处理 Escape 则不重复）；
   面板开着时重算动作（最近对象会变）；渲染 `<CommandPalette open onClose actions />`。挂在 `Layout` 根下，与 `TopBar` 并列。
3. **TopBar 搜索位**：`GlobalSearchSlot` 从 `aria-disabled` + 「暂不可用」Tooltip 改为可点按钮：文案「搜索或跳转…」+ 右侧 `Ctrl K` 键帽（`kbd`），点击 / 快捷键都开面板；
   `top-bar-model.ts` 的 `GLOBAL_SEARCH_UNAVAILABLE_REASON` 改名 / 改义为面板说明（「搜索导航与最近对象；跨对象搜索等读口」），`top-bar-model.test.ts` 跟改。
   TopBar 头注里「全局搜索今天是留位」那句改成实话。
4. 头注写清：面板里没有任何一条动作发请求；能搜到的对象只有本机打开过的——这不是缺陷是边界（对象名称属业务数据，见第一轮 02 的隐私边界）。

## 不做

- 不接 `searchSource` 异步搜索（vendor 0.1.25 没有，也没有读口）；不做「最近命令」记忆；不做分栏 / 标签相关动作（等 01）。
- 不动 `Sidebar`、不动导航词表本身。

## 完成判据

- 四道门绿；`command-actions.test.ts` 四条以上；`top-bar-model.test.ts` 既有用例零改动或改动逐条写理由。
- 一次性 esbuild 束：`Ctrl+K` 派发后 DOM 出现 `CommandPalette` 的列表且含「打开 工作台」；无最近对象时无 recent 组。
- TopBar 搜索位不再有 `aria-disabled`；`vite build` 产物含「搜索或跳转」字面量。浏览器验收做不到如实写「未验」。

## 完成记录（推送方代落，2026-09-21 11:5x；作者通道 6，码 tip `938a2a54`，基 `87ff1edd` = main）

作者三笔码 + 认领笔在 09-20 20:1x–20:22 提完，20:4x 评审（通道 4，见 Comments）点出 `Layout` 接线未做；作者随后把分支 rebase 到 `87ff1edd`（四笔 `range-diff` 全 `=`），
21:00 在树上写完 `Layout.tsx` 接线（+29/−2）**未提交**，此后无任何活动（`list_sessions` lastActivity 约 20:09，mtime 21:00:16）。推送方按 parallel-sessions「未提交现场」
把那份改动原样入库一字不改（`938a2a54`，提交信写 mtime 与无响应证据），再做下面的门与探针。下表按 `git diff 87ff1edd..938a2a54` 写，不取记忆。

| 笔 | 条 | 落点 |
|---|---|---|
| `fc1398e9` | 认领 | 票面 Status → in-progress |
| `e785a28e` | 第 1 条 | 新 `shell/command-actions.ts`：`buildCommandActions({ pageTitleById, recentObjects, recentObjectHash, shellToggles, navigate? })` 出三组——`navigation`（词表每条「打开 <页名>」，`keywords` = 小写去重的 `[id, 页名]`，`run` 写 `moduleHash(id)` = `#/<id>`）/ `recent`（前 `RECENT_ACTIONS_LIMIT` = 10 条「打开 <title>」，`run` 写注入的 `recentObjectHash(entry)`，关键词 `[objectId, title, 模块名]`）/ `actions`（vendor 联合里没有 `shell`，壳层两开关落 `actions`：切主题、切密度，标签复用 `themeToggleLabel` / `densityToggleLabel` 与顶栏同句）；`RecentEntry` 与 02 的 `RecentObject` 结构兼容、不 import 那边类型；`isOpenCommandPaletteShortcut`（Ctrl/⌘+K，Alt 不认，大小写都算）；只 `import type` `@idpxyz/ui-workspace`。头注写明零请求与「只搜到本机打开过的对象」边界（第 4 条）。`command-actions.test.ts` node:test 8 条 |
| `67982672` | 第 2 条前半 | 新 `shell/CommandPaletteHost.tsx`：受控件（`open` / `onOpenChange` / `readRecent` / `recentObjectHash`），`window` `keydown` 监听 → `isOpenCommandPaletteShortcut` → `preventDefault` + `onOpenChange(true)`；`open` 时按当下主题 / 密度与 `readRecent()` 重算动作集，关着给空集；Escape 交 vendor（0.1.25 开着时自己听 keydown，两处各关一次会调两遍）；渲 `<CommandPalette open onClose actions />` |
| `18ba6620` | 第 3 条 | `TopBar.tsx` `GlobalSearchSlot` 收 `onOpen?`：有则可点按钮「搜索或跳转…」+ `kbd` 键帽（`aria-hidden`）+ `aria-keyshortcuts="Control+K Meta+K"`，Tooltip 写 `COMMAND_PALETTE_HINT`；无则退回留位（`aria-disabled` + `COMMAND_PALETTE_UNWIRED_REASON`）。头注「全局搜索今天是留位」改成入口实话。`top-bar-model.ts`：`GLOBAL_SEARCH_LABEL` → `COMMAND_PALETTE_TRIGGER_LABEL`（「搜索或跳转…」）、`GLOBAL_SEARCH_UNAVAILABLE_REASON` → `COMMAND_PALETTE_UNWIRED_REASON`（「命令面板未接线」），新增 `COMMAND_PALETTE_SHORTCUT_KEYS` / `COMMAND_PALETTE_ARIA_KEYSHORTCUTS` / `COMMAND_PALETTE_HINT`（「搜索导航与最近对象；跨对象搜索等读口」）；`top-bar-model.test.ts` +22/−7，改动那条用例注释写理由（事实变了不是放松） |
| `938a2a54` | 第 2 条后半 | `Layout.tsx`：`commandPaletteOpen` state 在 Layout；`<TopBar onOpenCommandPalette={openCommandPalette} />`；`<CommandPaletteHost open onOpenChange readRecent={readRecentObjectsForPalette} recentObjectHash={recentObjectHash} />` 与 TopBar 并列；`readRecentObjectsForPalette = () => listRecentObjects(window.localStorage)` 是模块级常量（host 的 `useMemo` 依赖它的引用）；头注写两个入口共用一份 open 态、存储与地址写法由外壳注入 |

**完成判据逐条**

- ✅ 四道门（推送方在作者树 `938a2a54` 上实跑）：`tsc -b --noEmit` 0 / `run-tests` **368**（main 359 + 本票 8 + `top-bar-model.test.ts` 1）/ `vite build` 0 / `go test ./internal/architecture/` ok。
- ✅ `command-actions.test.ts` 8 条（≥ 4）：导航一词一条含工作台、关键词含 id 与页名、最近对象 12 → 10 且 run 写对象地址、空存储 recent 0 条其余照常、关键词全小写、壳层两条指向目标态、id 全集无重复、快捷键只认 Ctrl/⌘+K。
- ✅ `top-bar-model.test.ts` 既有用例：一条改写（「全局搜索位的名与禁用说明」→「搜索位作为命令面板入口的文案、键帽与说明」，五断言含「不写即将上线」），理由在用例注释里（原两常量改名改义，断言随事实作废）；新增一条「未接线时的留位说明」；其余零改动。
- ✅ 一次性探针（推送方代跑，**源与产物不入库**）：happy-dom 20.14.5 装在 `%TEMP%\wsform03-probe`，esbuild 0.25.12 把 `App`（三 Provider + Layout）束成 CJS，`react-dom/client` 挂真 DOM、`act` 包事件：
  **16 ok / 0 fail**——搜索位存在且无 `aria-disabled`、`aria-keyshortcuts` 对、键帽在、初始面板关着；`Ctrl+K` keydown 被 `preventDefault`、面板开、列表含「打开 工作台」、有 Navigation / Actions 组、
  **无最近对象时无 Recent Objects 组**、无「新建 / 导出 / 刷新」；Escape 关；点搜索位开（TopBar 入口走同一份 open 态）；往 `parcel-admin-web:recent-objects` 写一条后再开出现 Recent Objects 组与「打开 委托查阅 · SR-1」；无修饰键的 `k` 不开。
- ✅ `vite build` 产物含「搜索或跳转」1 处（`dist/assets/index-*.js` grep）。
- ❌ 浏览器**未验**：面板视觉与焦点落入输入框、Chromium 上 Ctrl+K 是否真的没跑去地址栏、Tooltip 悬停文案、⌘+K 在 macOS。

**判断项**（推送方按代码与头注代写）

1. **open 态放 Layout 不放 host**：面板有两个入口（host 听的快捷键、TopBar 的按钮），TopBar 不在 host 之下，两个入口要指向同一份态，态只能在共同父级；host 因此是受控件。
2. **壳层开关落 vendor 的 `actions` 组**：`CommandPaletteAction.group` 联合里没有 `shell`；不改 vendor，也不为两条动作另起一组。
3. **关键词小写去重**：vendor 过滤时只小写查询词、`keywords` 原样 `includes`，大写对象标识（`SR-…`）不先小写永远搜不到——这是绕 vendor 的一个坑，写在 `keywordsOf` 注释里。
4. **不搬 myshop-web 的假快捷动作与底栏 / 右栏切换**：前者 spec 红线不许假动作；后者本仓没有那两个位（等票 01 / 02），本票不留空动作。
5. **Escape 不在 host 处理**：vendor 0.1.25 开着时自己听 keydown 处理 Escape / 方向键 / Enter，再关一次是重复且会让 `onOpenChange(false)` 调两遍。

## Comments

### 评审 ← 通道 4 · 钉 `mcp6-wsform03@71786680`（基 `11e6111a`）· 20:4x（推送方接任后评，只读作者树 `D:\tops\idp-parcel-mcp6-wsform03`；票面 Status 在分支上已改 in-progress，本 Comments 落在 main 的副本上）

分支三笔码：`40873c9c` 第 1 条 `shell/command-actions.ts` + node:test 8 条；`ca56b925` 第 2 条 `shell/CommandPaletteHost.tsx`；`71786680` 第 3 条 `TopBar.tsx` + `top-bar-model.ts` 两常量改名改义、`top-bar-model.test.ts` 跟改逐条写理由。

**Spec** — **阻断 1**：第 2 条「挂在 `Layout` 根下，与 `TopBar` 并列」未做——`Layout.tsx` 在分支上零改动，`CommandPaletteHost` 无人渲染、`TopBar` 的 `onOpenCommandPalette` 无人传；面板不可达，搜索位按代码设计退回「命令面板未接线」`aria-disabled` 留位。完成判据「TopBar 搜索位不再有 `aria-disabled`」「`Ctrl+K` 派发后 DOM 出现 `CommandPalette` 列表且含『打开 工作台』」在此 tip 上不成立（缺完成判据点名的东西 → 阻断）。回作者同一分支补：`Layout` 持 `open` state，渲 `<CommandPaletteHost open onOpenChange readRecent={() => listRecentObjects(window.localStorage)} recentObjectHash={recentObjectHash} />`，`TopBar` 传 `onOpenCommandPalette`；ux-alignment/02 已进 main，`fetch` + `rebase origin/main` 后 `Layout.tsx` 上 02 的钩子已在。非阻断：无。
无发现（实核）：第 1 条三组 navigation / recent（前 `RECENT_ACTIONS_LIMIT` = 10）/ actions（只主题、密度两条，标签复用 `themeToggleLabel` / `densityToggleLabel` 与顶栏同句），无「新建 / 导出 / 刷新」假动作；`run` 只写 hash（`moduleHash` / 注入的 `recentObjectHash`），一条不发请求；关键词小写去重（vendor 过滤只小写查询词）；node:test 8 条含「空存储 recent 0 条」「id 全集无重复」「快捷键只认 Ctrl/⌘+K」；第 3 条搜索位接线态无 `aria-disabled`、`aria-keyshortcuts="Control+K Meta+K"`、键帽 `aria-hidden`，未接线态诚实退回留位（spec 红线：禁用态 + 说明，无假动作）；第 4 条头注写明零请求与「只搜到本机打开过的对象」边界；「不做」：无 `searchSource`、无最近命令、不动 `Sidebar` 与词表。

**Standards** — 阻断：无。非阻断：无。无发现（实核）：注释全中文，引 `Layout.tsx` 文件头 / `templates/loading-shape.ts` 文件头 / idp-ui@6751fb2 用符号名与 SHA，无行号无计数；`command-actions.ts` 只 `import type` `@idpxyz/ui-workspace`，运行时零 `@idpxyz/*`，node:test 可 require；`RecentEntry` 与 02 的 `RecentObject` 结构兼容、不 import 那边类型（存储归 02），`recentObjectHash` 由调用方注入不复制写法；`top-bar-model.test.ts` 原「全局搜索位的名与禁用说明」一条改写，理由写在用例注释里（事实变了不是放松）；`TopBar` `onOpenCommandPalette` 可选是为接线前能编译，注释写明。

**Spec 1 / 0 · Standards 0 / 0** → 有阻断，未重放；补完 `Layout` 接线后只需重跑 Spec 轴。

### 评审 Spec 轴 ← 通道 1 · 钉 `938a2a54`（基 `87ff1edd`，只看接线笔 `git diff 18ba6620..938a2a54`）· 2026-09-21 11:5x（**推送方自跑，不算非作者评审**：`list_sessions` 2–6 皆 09-20 20:0x–20:5x 后无活动、无人可派；用户直接令「修复，并回放」）

按上一条评审「补完 `Layout` 接线后只需重跑 Spec 轴」办，Standards 轴沿上一条（0 / 0），接线笔只补看一眼：注释全中文、引 `shell/CommandPaletteHost` / `recent-objects.ts` 用文件名与符号名、无行号无计数、`useState` 已在原 import 内。

**Spec** — 阻断：无。非阻断：无。无发现（实核）：第 2 条「挂在 `Layout` 根下、与 `TopBar` 并列」——`<CommandPaletteHost … />` 紧跟 `<TopBar … />` 之后、同级；`open` state 在 `Layout`（`commandPaletteOpen` / `setCommandPaletteOpen`），`onOpenChange={setCommandPaletteOpen}` 直传 setter（引用稳定，host 的 `useEffect([onOpenChange])` 不会每帧重挂监听）；`TopBar` 收 `onOpenCommandPalette={openCommandPalette}`，上一条阻断里「无人传」不再成立；`readRecent` 是模块级常量 `readRecentObjectsForPalette`（引用稳定，`useMemo` 依赖不抖），存储仍归 02 的 `recent-objects.ts`、地址写法透传 `recentObjectHash`；面板里的跳转写 hash 走 Layout 同一条 `hashchange` 路。上一条评审点名的两条完成判据在 `938a2a54` 上由推送方代跑的一次性探针（happy-dom 真 DOM，16 ok / 0 fail）实测成立：搜索位无 `aria-disabled`；`Ctrl+K` 派发后 DOM 出现列表且含「打开 工作台」；空存储无 Recent Objects 组。「不做」仍守：未动 `Sidebar`、词表、`templates/*`、`pages/my-work/*`（只 import 读函数与地址写法）。

**Spec 0 / 0（Standards 沿 0 / 0）** → 无阻断，可重放。

### 处置（推送方 · 通道 1 · 2026-09-21）

- 通道 4 的 Spec 阻断 1 → 已由 `938a2a54` 解除（作者写的接线原样入库，推送方零改动）。
- 非阻断：两条评审皆无。
- 作者会话无响应，完成记录与判断项由推送方按代码与头注代写；浏览器验收仍「未验」，沿完成记录所列。

### 进 main 记录（推送方 · 通道 1）

- 重放：不必 cherry-pick——作者 09-20 已把分支 rebase 到 `87ff1edd`（= 当时 main tip；四笔 `range-diff` 全 `=`：`d4485e35→fc1398e9` / `40873c9c→e785a28e` / `ca56b925→67982672` / `71786680→18ba6620`），推送方在同一分支上加接线笔 `938a2a54` 与完成记录 `6739ab54`，main 纯 ff 六笔、SHA 不换。本票七件（`Layout.tsx`、`shell/CommandPaletteHost.tsx`、`shell/command-actions.ts` + test、`shell/TopBar.tsx`、`shell/top-bar-model.ts` + test）与 main 其间零他人提交。
- 门禁在隔离树 `%TEMP%\idp-land-wsform03 @ 6739ab54` 实跑（`pnpm install --frozen-lockfile --offline` 5.8 s）：`tsc -b --noEmit` 0 / `run-tests` **368**（main 359 + 8 + 1）/ `vite build` 0（产物含「搜索或跳转」1 处）/ `gofmt -l` 空 / `go build` 0 / `go vet` 0 / `go test ./internal/architecture/` ok / 清点重生成零差 /
  占号 12:0x → 真库探针 `TestFreezeScopesAreInvisibleToEachOther` **PASS**（不是 SKIP）→ 带 DSN 全量 `-p 1 -count=1 ./...` 12:06:35→12:08:55 **115 ok / 0 FAIL / 16 无测试 / 0 cached**。
- 12:09:17 `ls-remote` 核 `87ff1edd` 未动 → `push 6739ab54:main` 成（12:09:21），**远端 main = `6739ab54`**；共享树 ff 同 SHA；释号广播带门禁数字。**先 push 再簿记。**
- 浏览器未验沿完成记录所报。
- 2026-09-24 · 真浏览器验收 ← 用户（环境见票 01 Comments 同日一条，钉 `04898af1`）：`Ctrl+K` 起面板、跳转页面、列最近对象、切换检查器——用户答
  「全过」，上一条「浏览器未验」据此失效。
