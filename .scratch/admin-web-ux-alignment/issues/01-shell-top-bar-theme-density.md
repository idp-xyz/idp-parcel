# 01 壳层：Top Bar 四件（归属信息 / 全局搜索位 / 作用域位 / 用户菜单位）、Light 默认 + 主题切换、密度两档

Category: enhancement
Status: resolved——2026-09-20 17:5x 通道 4 交活（task-88934341 接续；分支 `mcp4-ux01` tip `ea8b63b8` 基 main `7d29af39`，五笔已推 origin；待非作者评审与推送方重放）。此前 in-progress（15:57 通道 4 认领 task-2fa9c998，该会话 16:05 后无响应、三件未提交现场由接续会话 17:2x 原样入库 `2b8847cb`）；更早 ready-for-agent
Blocked by: 无
地盘：`apps/admin-web/src/App.tsx`、`Layout.tsx`、`index.css`、新 `apps/admin-web/src/shell/`（TopBar 及其子件）。不动 `navigation.ts`（票 02 的地盘）、不动 `pages/`。
出处：spec「缺口」表第一档；手册「顶栏规范」「Top Bar 归属信息规范」「主题策略」「栅格与密度」；黄金标准「Top Bar 黄金标准」「Light Theme 黄金标准」；
参照 idp-ui@53df1666 `apps/loms-web/src/console/Layout.tsx`（品牌头那 8 行）与 `packages/ui-theme-runtime`（`useTheme().toggleTheme`、`DensityProvider` / `useDensity().toggleDensity`）。

## 为什么

品牌头今天只有「IDP Parcel / 租户管理台 … 页名」。手册把 Top Bar 定为**全局能力**的固定位置（搜索、作用域、告警、用户），黄金标准说「全局搜索永远在
稳定位置」——位置不立，后面每张页都会在自己头部长出搜索框。主题默认 dark 与手册「企业级工作台默认 Light」相反，而 `index.css` 的 Light 令牌早就写好、
页面里零硬编码色，切默认几乎零成本。密度两档 `useDensity` 现成，36 张列表页经模板一处接入（票 03 消费它，本票只提供开关与 Provider）。

## 要做的

1. **归属信息**：品牌头左侧改为手册格式 `Parcel / IDP · {模块名称}`——「Parcel / IDP」弱化色，模块名称主色，中间 `·`；模块名称 = `pageTitleById[active]`。
   产品图标照旧 `Package`。`ThemeProvider product` **仍不传**（`ui-tokens` `ProductKey` 未登记 parcel，归用户在 idp-ui 上游登记），头注写明。
2. **Top Bar 右侧全局工具区**，四个位从左到右：全局搜索（`Command` 系原语做外壳，**禁用态** + `Tooltip`「尚无跨对象搜索读口」——留位不留假动作）；
   作用域（显示当前授权作用域——`AuthGate` 会话里若有租户 / 主体名就显，没有就显「作用域：由服务端按会话判定」的只读 chip，**不做切换器**：本产品一会话一租户）；
   主题切换（`useTheme().toggleTheme`，图标按钮带 accessible label）；密度切换（`useDensity().toggleDensity`）；用户菜单（`DropdownMenu`：显示会话主体 +
   「退出」走 `auth/oidc.ts` 既有登出）。告警 / 通知位**不留**——没有面向 UI 的通知读口，留一个永远为 0 的铃铛是假位。
3. **Light 默认**：`index.css` 的 `:root` 默认令牌改为 Light 那组、`.dark` 保留 dark 组（今天 `:root, .dark` 合写为 dark）；`ThemeProvider` 初始
   模式与持久化：本地 `localStorage` key 带产品名（`parcel-admin-web:theme` / `:density`，手册「状态持久化」那条的 loms 做法），无存储时 Light / comfortable。
4. **`DensityProvider`** 包在 `App.tsx`（与 `ThemeProvider` / `ToastProvider` 同级），并在 `Layout.tsx` 主区加 `data-density` 属性供模板读。
5. Top Bar 高度固定 48px 不变；所有图标按钮有 `aria-label` + `Tooltip`（手册「可访问性规范」）。

## 不做

- 不做真搜索、不做作用域切换、不做通知；不动导航（02）、不动模板（03 / 06）。
- 不改任何页面文件。

## 完成判据

- 三道门绿；`node:test` 钉：主题 / 密度持久化键名与默认值（纯逻辑抽成 `shell/preferences.ts`）。
- `index.css` 只改令牌归组，两组令牌值逐一与 idp-ui@53df1666 `apps/loms-web/src/index.css` 相同（`git diff --no-index` 或逐行对，票面写明比法）。
- 无浏览器时：`vite build` 产物里 `Parcel / IDP` 字样存在；`Layout` 渲染树用 `react-dom/server` `renderToStaticMarkup` 在 node:test 里断言四个位与 `aria-label`
  在场（AuthGate 之外单测 `TopBar` 组件即可）。
- 浏览器验收做不到如实写「未验」。

## 完成记录（通道 4，2026-09-20 17:5x；分支 `mcp4-ux01`，tip `ea8b63b8`，基 main `7d29af39`）

五笔，每笔三道门（`tsc -b --noEmit` / `run-tests` / `vite build`）+ `go test ./internal/architecture/ -count=1` 绿后推 origin：

| 笔 | 条 | 落点 |
|---|---|---|
| `949706e6` | 第 3 条（键名 / 默认值） | 新 `shell/preferences.ts`：`parcel-admin-web:theme` / `:density`、无存储时 `light` / `comfortable`、坏值当没存、播种上游 `idpxyz-theme` 键；`preferences.test.ts` 六条钉 |
| `a8fe62a4` | 第 3 条（令牌归组） | `index.css`：`:root, .light` 挂 Light 组、`.dark` 只在 class 下生效；两组令牌值不动，只改归组（比法见下） |
| `2b8847cb` | 第 3 条后半 + 第 4 条 | `App.tsx` `DensityProvider` 与 `ThemeProvider` / `ToastProvider` 同级，挂 `ThemePreferenceMirror` / `DensityPreferenceAlignment`、`useSeedUpstreamTheme`；新 `shell/preference-sync.tsx`（三件不渲染 DOM 的同步件：主题在 Provider 首渲前播种、之后镜像写回；密度挂载后按存储值对齐一次、StrictMode 双跑用三段相位挡住）；`Layout.tsx` `<main data-density>`。**该笔是前任会话 16:04–16:05 留在树上的未提交现场，接续会话按 parallel-sessions「未提交现场」原样入库、一字未改**，提交信写的是证据（mtime、无响应起止）不是结论 |
| `c3c2555a` | 第 1 条 | 新 `shell/top-bar-model.ts`（`PRODUCT_ATTRIBUTION = 'Parcel / IDP'`、分隔符 `·`、`attributionText`）+ `top-bar-model.test.ts`；新 `shell/TopBar.tsx` 左侧三段（归属两段弱化色、模块名称主色，模块名称 = `pageTitleById[active]`）；`Layout.tsx` 品牌头换成 `<TopBar>`。`ThemeProvider product` 仍不传，`App.tsx` 头注原句成立 |
| `ea8b63b8` | 第 2 条 + 第 5 条 | `TopBar.tsx` 右侧四位：全局搜索（`Button outline`，`aria-disabled` + `Tooltip`「尚无跨对象搜索读口」，无 onClick）、作用域（`Tag outline` 只读 chip「作用域：由服务端按会话判定」+ 悬停说明为什么没有切换）、主题 / 密度切换（`IconButton`：`aria-label` 与 `Tooltip` 同一句，读 `useTheme` / `useDensity`）、用户菜单（`DropdownMenu`：`MenuLabel` 主体名 + `MenuItem destructive`「退出」→ `auth/oidc.ts` `logout`）；**不留铃铛**。新 `shell/session.ts` `useSessionPrincipal` 经 `ensureSession` + `displayName` 读主体名。`h-12` 不动 |

**完成判据逐条**

- ✅ 三道门绿（五笔各自）：末笔 `tsc` 0 / `run-tests` **309 pass 0 fail**（`2b8847cb` 时 304——含 `949706e6` 的 preferences 六条；`c3c2555a` +1 → 305；`ea8b63b8` +4 → 309）/ `vite build` 绿；`go test ./internal/architecture/ -count=1` 绿。
- ✅ `node:test` 钉键名与默认值：`preferences.test.ts`（键名带产品名、无存储 Light / comfortable、坏值当没存、写后可读回、播种以本产品键为准）。
- ✅ `index.css` 两组令牌与 idp-ui@53df1666 `apps/loms-web/src/index.css` 逐一相同。**比法**：`gh api -H "Accept: application/vnd.github.raw" repos/idpxyz/idp-ui/contents/apps/loms-web/src/index.css?ref=53df1666` 取文件（经 `cmd /c … >` 落盘保 UTF-8；`git hash-object` 得 `8e5981ef`，与 GitHub 报的 blob sha 一致），一次性 node 脚本解析上游 `:root, .dark {…}` / `.light {…}` 与本仓 `:root, .light {…}` / `.dark {…}` 四个块里的 `--idpxyz-*` 与 `color-scheme` 声明，按块逐键逐值比：**light 41 条 / dark 41 条，键并集各 41，值不同 0**。`git diff --no-index` 全文比则有差——差在注释（本仓中文头注）、`body` 的 `font-family`（本仓加 `Microsoft YaHei`，`041adc37` 之前就有）与归组，不在令牌值。脚本源不入库。
- ✅ `vite build` 产物 `dist/assets/index-*.js` 含 `Parcel / IDP`（`Select-String -SimpleMatch` 1 处命中）。产物里仍有一处「IDP Parcel 租户管理台」——那是 `pages/Workbench.tsx` 的页内 h1，与 `index.html` 的 `<title>`，都不是顶栏、不在本票地盘。
- ◑ 四个位与 `aria-label` 的 `renderToStaticMarkup` 断言：**一次性实测、组件层未钉**（照 05 / 06 结论：`.test.ts` 引不到 `import` 了 `@idpxyz/*` 的模块）。用一次性 esbuild 束
  （`node node_modules/.pnpm/esbuild@0.25.12/node_modules/esbuild/bin/esbuild .tmp-test/probe.tsx --bundle --platform=node --format=cjs --jsx=automatic --loader:.css=empty --outfile=$env:TEMP\ux01-probe.cjs`）
  把 `<ThemeProvider><DensityProvider><TopBar moduleTitle="工作台" principal="ops@example.test" onSignOut/></DensityProvider></ThemeProvider>` 渲成静态 HTML。
  **绕法**：`ThemeProvider` 的 `useState` 初始化直接读 `localStorage`，SSR 下没有这个全局会抛——探针在 `globalThis` 垫一个内存版并预放 `idpxyz-theme=light`（模拟 `preference-sync` 的播种）。
  断言 **17/17**：`<header class="h-12 …">`；`Parcel / IDP` 在场、`·` 带 `aria-hidden`、模块名称在场；位 1 `aria-label="全局搜索"` + `aria-disabled="true"` 且**无原生 `disabled`**；位 2 文案「作用域：由服务端按会话判定」；
  位 3 `aria-label="切换到深色主题"`（播种 light）与 `aria-label="切换到舒适密度"`（`DensityProvider` 起手 compact，对齐在 layout effect、SSR 不跑——生产里挂载后即对齐到 comfortable）；位 4 `aria-label="用户菜单：ops@example.test"` + `aria-haspopup="menu"` + 主体名可见；
  无 bell / 通知 / 告警字样；`aria-label="切换到…"` 的 `<button>` 恰两个；触发器 `data-state="closed"` ≥ 5 处（搜索、chip、主题、密度、用户）；不传 `principal` 时 `aria-label` 退回「用户菜单」、位仍在。
  Tooltip 与菜单内容走 Radix Portal，SSR 不渲染，「尚无跨对象搜索读口」这句因此**不在**静态树里（探针把这一点也断成 PASS，免得下一个人以为漏了）。源与产物不入库；`.tmp-test/` 本就在 `.gitignore`。
- ❌ 浏览器**未验**（AuthGate 要 gk.idp.xyz 会话）。未验的具体有：Tooltip 悬停 / 聚焦是否弹、`DropdownMenu` 打开与「退出」跳转、主题切换后 `<html>` class 与令牌内联、密度切换后列表行距、Light 首屏是否有 dark 闪帧。

**判断项**

1. **密度默认档 = comfortable**（推送方 17:2x 裁定）：`preferences.ts` `DEFAULT_DENSITY = 'comfortable'`；03 的 `ListPageTemplate` 无 Provider 时随库回退 compact（`py-1`），挂上本票的 Provider 后列表行距变 `py-2.5`——预期内的一次性变化，未为迁就它改默认。
2. **第 2 条「`Command` 系原语做外壳」未照字面**：全局搜索位用 `Button variant="outline"` 做外壳而不是 `Command` + `CommandInput`。理由：(a) cmdk 的 Input 渲成 `role="combobox" aria-expanded="true" aria-controls=<列表 id>`，一个禁用的搜索位背后没有列表，读屏会念出「组合框，已展开」——比没有名字更误导；(b) 上游 ui-workspace `TitleBar` 的搜索位本身也是按钮形外壳，`Command` 原语是点开之后的 `CommandPalette` 用的；(c) `CommandInput` 的包装 div 带死的 `border-b`，塞进顶栏一个带边框的盒子里会多一道线。接真搜索时这一位点开 `CommandPalette`，形与上游同。评审若认为该照字面，改回去是局部改动。
3. **第 2 条搜索位用 `aria-disabled` 不用原生 `disabled`**：原生 `disabled` 的按钮不发指针与焦点事件，Tooltip 永远弹不出来，「为什么不能用」就没人看得到。
4. **第 5 条「所有图标按钮 aria-label + Tooltip」——用户菜单触发按钮不是纯图标**：带可见主体名（图标 + 名 + 箭头，上游 `TitleBar` 用户按钮的形）。原因是 ui-primitives 的 `Tooltip` 与 `DropdownMenu` 各自把 Radix 的 Root + Trigger 包成一个件、两个 `asChild` Trigger 套不到同一个 `button` 上（外层 Trigger 的 props 会被内层包装件吞掉，菜单打不开）；本仓没有直接依赖 `@radix-ui/*`，不能绕过包装自己拼。主体名未到的那一拍（读口异步，首帧）触发按钮只有图标、无 Tooltip、`aria-label="用户菜单"`——瞬态，写明不藏。
5. **作用域 chip 恒显兜底句**：`id_token` 没有任何登记过的租户声明，ADR-0100 把操作者—租户绑定放在服务端操作者册；`TopBar` 留了 `tenantName` prop 与 `scopeChipLabel(tenantName)`，Layout 今天不传。有「本会话绑哪个租户」的读口时接上，位与文案不动。
6. **`AuthGate` 的悬浮 `SessionBadge`（右下角「退出」）与新用户菜单重复**。`auth/AuthGate.tsx` 不在本票地盘、未动；它自己的注释写着「退出入口以悬浮徽章承载而不改 Layout：品牌头属页面形态地盘，本轮只做门」——顶栏接手之后那个徽章就该退场。建议另立一笔删 `SessionBadge`（顺带删它的 `displayName` 引用），归推送方派。
7. **`shell/session.ts` 经 `ensureSession` 读会话**而不改 `AuthGate` 往子树传 context：同上，`auth/` 不在地盘。代价是首帧无名、下一拍补上；`ensureSession` 临近到期会顺手续期，与门自己的续期由 `oidc.ts` 在途单例合成一次。
8. **第 5 条与第 2 条同笔**（`ea8b63b8`）：图标按钮本就按「`aria-label` 与 `Tooltip` 同句」造（`IconButton`），顶栏高度沿 `h-12`，没有单独可提的改动；派单「每条一笔」在这一条上没有内容可分。

**评审 / 推送方要看的**：`git diff 7d29af39..ea8b63b8 -- apps/admin-web`（10 文件，+602 / −65）；只跑 `apps/admin-web` 三道门 + `./internal/architecture/`，不需 DSN。

## Comments
