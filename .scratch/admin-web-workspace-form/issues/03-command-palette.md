# 03 命令面板 `Ctrl+K`：导航 + 最近对象 + 壳层开关；TopBar 全局搜索位改为面板入口

Category: enhancement
Status: ready-for-agent
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

## Comments

### 评审 ← 通道 4 · 钉 `mcp6-wsform03@71786680`（基 `11e6111a`）· 20:4x（推送方接任后评，只读作者树 `D:\tops\idp-parcel-mcp6-wsform03`；票面 Status 在分支上已改 in-progress，本 Comments 落在 main 的副本上）

分支三笔码：`40873c9c` 第 1 条 `shell/command-actions.ts` + node:test 8 条；`ca56b925` 第 2 条 `shell/CommandPaletteHost.tsx`；`71786680` 第 3 条 `TopBar.tsx` + `top-bar-model.ts` 两常量改名改义、`top-bar-model.test.ts` 跟改逐条写理由。

**Spec** — **阻断 1**：第 2 条「挂在 `Layout` 根下，与 `TopBar` 并列」未做——`Layout.tsx` 在分支上零改动，`CommandPaletteHost` 无人渲染、`TopBar` 的 `onOpenCommandPalette` 无人传；面板不可达，搜索位按代码设计退回「命令面板未接线」`aria-disabled` 留位。完成判据「TopBar 搜索位不再有 `aria-disabled`」「`Ctrl+K` 派发后 DOM 出现 `CommandPalette` 列表且含『打开 工作台』」在此 tip 上不成立（缺完成判据点名的东西 → 阻断）。回作者同一分支补：`Layout` 持 `open` state，渲 `<CommandPaletteHost open onOpenChange readRecent={() => listRecentObjects(window.localStorage)} recentObjectHash={recentObjectHash} />`，`TopBar` 传 `onOpenCommandPalette`；ux-alignment/02 已进 main，`fetch` + `rebase origin/main` 后 `Layout.tsx` 上 02 的钩子已在。非阻断：无。
无发现（实核）：第 1 条三组 navigation / recent（前 `RECENT_ACTIONS_LIMIT` = 10）/ actions（只主题、密度两条，标签复用 `themeToggleLabel` / `densityToggleLabel` 与顶栏同句），无「新建 / 导出 / 刷新」假动作；`run` 只写 hash（`moduleHash` / 注入的 `recentObjectHash`），一条不发请求；关键词小写去重（vendor 过滤只小写查询词）；node:test 8 条含「空存储 recent 0 条」「id 全集无重复」「快捷键只认 Ctrl/⌘+K」；第 3 条搜索位接线态无 `aria-disabled`、`aria-keyshortcuts="Control+K Meta+K"`、键帽 `aria-hidden`，未接线态诚实退回留位（spec 红线：禁用态 + 说明，无假动作）；第 4 条头注写明零请求与「只搜到本机打开过的对象」边界；「不做」：无 `searchSource`、无最近命令、不动 `Sidebar` 与词表。

**Standards** — 阻断：无。非阻断：无。无发现（实核）：注释全中文，引 `Layout.tsx` 文件头 / `templates/loading-shape.ts` 文件头 / idp-ui@6751fb2 用符号名与 SHA，无行号无计数；`command-actions.ts` 只 `import type` `@idpxyz/ui-workspace`，运行时零 `@idpxyz/*`，node:test 可 require；`RecentEntry` 与 02 的 `RecentObject` 结构兼容、不 import 那边类型（存储归 02），`recentObjectHash` 由调用方注入不复制写法；`top-bar-model.test.ts` 原「全局搜索位的名与禁用说明」一条改写，理由写在用例注释里（事实变了不是放松）；`TopBar` `onOpenCommandPalette` 可选是为接线前能编译，注释写明。

**Spec 1 / 0 · Standards 0 / 0** → 有阻断，未重放；补完 `Layout` 接线后只需重跑 Spec 轴。
