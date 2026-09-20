# 01 壳层：Top Bar 四件（归属信息 / 全局搜索位 / 作用域位 / 用户菜单位）、Light 默认 + 主题切换、密度两档

Category: enhancement
Status: resolved · 已进 main（码 `83da6021`，推送方注释笔 `ed7ef224`；评审 ← 通道 2 两轴 0 阻断）——2026-09-20 17:5x 通道 4 交活（task-88934341 接续；分支 `mcp4-ux01` 基 main `7d29af39`，代码五笔 tip `ea8b63b8`、票面两笔在其上；按推送方裁决补一笔 `5269239e` 删 `SessionBadge`；均已推 origin）。此前 in-progress（15:57 通道 4 认领 task-2fa9c998，该会话 16:05 后无响应、三件未提交现场由接续会话 17:2x 原样入库 `2b8847cb`）；更早 ready-for-agent
Blocked by: 无
地盘：`apps/admin-web/src/App.tsx`、`Layout.tsx`、`index.css`、新 `apps/admin-web/src/shell/`（TopBar 及其子件）。不动 `navigation.ts`（票 02 的地盘）、不动 `pages/`。
`auth/AuthGate.tsx` 只删 `SessionBadge`（与用户菜单重复，推送方 18:0x 裁）——两个退出入口是本票引入的重复，由本票收；会话 / 登出逻辑不动。
出处：spec「缺口」表第一档；手册「顶栏规范」「Top Bar 归属信息规范」「主题策略」「栅格与密度」；黄金标准「Top Bar 黄金标准」「Light Theme 黄金标准」；
参照 idp-ui@53df1666 `apps/loms-web/src/console/Layout.tsx`（品牌头那 8 行）与 `packages/ui-theme-runtime`（`useTheme().toggleTheme`、`DensityProvider` / `useDensity().toggleDensity`）。

## 为什么

品牌头今天只有「IDP Parcel / 租户管理台 … 页名」。手册把 Top Bar 定为**全局能力**的固定位置（搜索、作用域、告警、用户），黄金标准说「全局搜索永远在
稳定位置」——位置不立，后面每张页都会在自己头部长出搜索框。主题默认 dark 与手册「企业级工作台默认 Light」相反，而 `index.css` 的 Light 令牌早就写好、
页面里零硬编码色，切默认几乎零成本。密度两档 `useDensity` 现成，36 张列表页经模板一处接入（票 03 消费它，本票只提供开关与 Provider）。

## 要做的

1. **归属信息**：品牌头左侧改为手册格式 `Parcel / IDP · {模块名称}`——「Parcel / IDP」弱化色，模块名称主色，中间 `·`；模块名称 = `pageTitleById[active]`。
   产品图标照旧 `Package`。`ThemeProvider product` **仍不传**（`ui-tokens` `ProductKey` 未登记 parcel，归用户在 idp-ui 上游登记），头注写明。
2. **Top Bar 右侧全局工具区**，四个位从左到右：全局搜索（按钮形外壳——`Button outline` + `aria-disabled` + `Tooltip`「尚无跨对象搜索读口」——留位不留假动作；
   **原文「`Command` 系原语做外壳」经推送方 18:0x 裁改口**：那是手段不是目的，cmdk 的 Input 渲成 `role=combobox aria-expanded` 而背后无列表，对读屏正是一个假动作；
   上游 ui-workspace `TitleBar` 的搜索位本身也是按钮形外壳，`Command` 原语留给点开之后的 `CommandPalette`）；
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
| `5269239e` | 裁决落地（判断项 6） | `auth/AuthGate.tsx` 删悬浮 `SessionBadge`（JSX 引用、函数定义、它独用的 `displayName` / `logout` import）；两态、续期看守、`LoginScreen` 不动。同笔票面：地盘句加注、第 2 条括注改口、判断项 2 / 6 落为裁决 |

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
2. **裁决（推送方 18:0x，接受）——第 2 条搜索位用 `Button variant="outline"` 做外壳，不用 `Command` + `CommandInput`**。理由一句：cmdk 的 Input 渲成 `role="combobox" aria-expanded="true" aria-controls=<列表 id>`，禁用的搜索位背后没有列表，读屏念出「组合框，已展开」是 spec 红线「留位不留假动作」正面撞上的那种假动作；上游 ui-workspace `TitleBar` 的搜索位本身也是按钮形外壳（点开才弹 `CommandPalette`），与之同形。另一件顺带的：`CommandInput` 的包装 div 带死的 `border-b`，塞进顶栏一个带边框的盒子里会多一道线。接真搜索时这一位点开 `CommandPalette`。票面第 2 条括注已按此改口。
3. **第 2 条搜索位用 `aria-disabled` 不用原生 `disabled`**：原生 `disabled` 的按钮不发指针与焦点事件，Tooltip 永远弹不出来，「为什么不能用」就没人看得到。
4. **第 5 条「所有图标按钮 aria-label + Tooltip」——用户菜单触发按钮不是纯图标**：带可见主体名（图标 + 名 + 箭头，上游 `TitleBar` 用户按钮的形）。原因是 ui-primitives 的 `Tooltip` 与 `DropdownMenu` 各自把 Radix 的 Root + Trigger 包成一个件、两个 `asChild` Trigger 套不到同一个 `button` 上（外层 Trigger 的 props 会被内层包装件吞掉，菜单打不开）；本仓没有直接依赖 `@radix-ui/*`，不能绕过包装自己拼。主体名未到的那一拍（读口异步，首帧）触发按钮只有图标、无 Tooltip、`aria-label="用户菜单"`——瞬态，写明不藏。
5. **作用域 chip 恒显兜底句**：`id_token` 没有任何登记过的租户声明，ADR-0100 把操作者—租户绑定放在服务端操作者册；`TopBar` 留了 `tenantName` prop 与 `scopeChipLabel(tenantName)`，Layout 今天不传。有「本会话绑哪个租户」的读口时接上，位与文案不动。
6. **裁决（推送方 18:0x）——`AuthGate` 的悬浮 `SessionBadge`（右下角主体名 + 「退出」）另起一笔在同分支删**，条件是顶栏用户菜单在 `AuthGate` 放行后的所有渲染路径上都在场。
   核过：放行后 `AuthGate` 只渲染 `children`，`children` 在 `main.tsx` 里就是 `<App>`；`App` → 三个 Provider → `Layout`，`Layout` 顶部无条件渲染 `<TopBar>`，`renderActive()` 只换主区；
   `src/` 下没有任何 ErrorBoundary（`git grep` 零命中），不存在把 `Layout` 换掉的路径——条件成立，删。改动只在 `SessionBadge` 那一块：JSX 引用、函数定义、它独用的 `displayName` / `logout` 两个 import；
   `checking` / `login` 两态、续期看守、`LoginScreen` 一字未动。它自己那句头注「退出入口以悬浮徽章承载而不改 Layout：品牌头属页面形态地盘，本轮只做门」写的就是「等顶栏接手」——现在接了。
7. **`shell/session.ts` 经 `ensureSession` 读会话**而不改 `AuthGate` 往子树传 context：同上，`auth/` 不在地盘。代价是首帧无名、下一拍补上；`ensureSession` 临近到期会顺手续期，与门自己的续期由 `oidc.ts` 在途单例合成一次。
8. **第 5 条与第 2 条同笔**（`ea8b63b8`）：图标按钮本就按「`aria-label` 与 `Tooltip` 同句」造（`IconButton`），顶栏高度沿 `h-12`，没有单独可提的改动；派单「每条一笔」在这一条上没有内容可分。

**评审 / 推送方要看的**：`git diff 7d29af39..5269239e -- apps/admin-web`（11 文件：上面 10 件 + `auth/AuthGate.tsx`）；只跑 `apps/admin-web` 三道门 + `./internal/architecture/`，不需 DSN。

## Comments

### 评审 ← 通道 2 · 钉 `113d048f`（码 `5269239e`，基线 `7d29af39`）· 18:0x（推送方自任务台 `task-34a89410` 代落原文）

只读，隔离树已拆，未跑门禁、未占 55432、未碰作者树。

**Standards** — 阻断：无。非阻断：
1. `shell/TopBar.tsx` 文件头注「手册『顶栏规范』列的第三样『Alerts / Notifications』」——序数即计数（AGENTS「改文档」条），手册那张表增删一项就无声变错；名字已在场，删「第三样」即可。
2. `Layout.tsx` `<main data-density>` 处注释称「模板层（票 03）按它选行高与间距，不必各自再读 useDensity」，但 `templates/ListPageTemplate.tsx` 实为直接 `useDensity()`，该属性今日无读者。
   属性本身是票面第 4 条要的，留；只改注释把消费者写实（此块来自 `2b8847cb` 原样入库的前任现场，写在 03 落地前）。
3. `auth/AuthGate.tsx` 删 `SessionBadge` 后文件末尾无换行（仓内无 prettier / editorconfig 兜底，属手工纠）。
无发现（实核）：注释全中文；跨文件引用皆用符号名 / 小节标题，无行号；`shell/preferences.ts` 头注对上游的断言实核 dist 成立——`ThemeProvider` `useState` 只读 `idpxyz-theme`、缺省 dark，
`DensityProvider` `useState('compact')` 无初值口；`shell/preference-sync.tsx` `DensityPreferenceAlignment` 三段相位在 StrictMode 双跑下成立（ref 跨模拟卸载保留，第二跑见 `'toggled'` 不再切；
写回只在 `'aligned'` 之后，而 aligned 要求 density === 存储值，故无把上游 compact 写进本产品键的窗口）；`useSeedUpstreamTheme` 在 App 函数体内先于 `ThemeProvider` 渲染，`useState` 懒初始化幂等；
`readEnum` 坏值回默认，测试覆盖大小写 / 空串 / 串键；`shell/session.ts` `useSessionPrincipal` 只经 `ensureSession` 读同一份 sessionStorage，续期合入 `inFlightRenewal` 单例，无新鉴权路径；
`AuthGate` 只删徽章块与其独用的 `displayName` / `logout` import，`OidcSession` 仍被 `GateState` 用；Fowler：`TopBar` 的 `tenantName` prop 无调用方属可能 Speculative Generality，但票面第 2 条明写「有租户名就显」，seam 合理，不计。

**Spec** — 阻断：无。非阻断：
1. `index.css` `:root` 挂 Light 后，存储为 dark 的操作者首帧是 Light，`.dark` 要等 `ThemeProvider` 的 `useEffect` 挂载后才加到 `<html>`——反向闪帧。票面「未验」只列了「Light 首屏是否有 dark 闪帧」，
   这一向也该写进未验；修法是 `index.html` 内联脚本读 `parcel-admin-web:theme` 预挂 class，越地盘，不在本票。
2. `shell/TopBar.tsx` `ScopeChip` 的 Tooltip 挂在 `Tag`（非焦点元素）上，「为什么没有切换」这句键盘 / 读屏到不了；票面只要求「悬停说明」，可不改，记一笔供 03 / 06 同型位参考。
无发现：第 1 条 `PRODUCT_ATTRIBUTION`='Parcel / IDP' 弱化色、`·` aria-hidden、模块名主色，`App.tsx` 仍不传 product 且头注写明；第 2 条四位顺序搜索→作用域→主题/密度→用户，搜索 `Button variant=outline` + `aria-disabled` +
Tooltip、无 onClick、无原生 disabled，作用域 `Tag` 只读无切换器，用户菜单 `MenuItem destructive` 退出经 `Layout` `onSignOut={logout}` 走 oidc.ts 既有登出，无铃铛；第 3 条 dark / light 两块实比 41 / 41 键、值零差，
`.dark` 写在 `:root` 后同特异性压过，键名 / 默认值由 `preferences.test.ts` 钉住，`DEFAULT_DENSITY === 'comfortable'` 与 03 裁决 3 对上；第 4 条 `DensityProvider` 同级、`<main data-density>` 在；第 5 条 `h-12` 不动、
`IconButton` aria-label 与 Tooltip 同句。完成判据：比法（gh api 取 53df1666 版 + 按块逐键逐值）写清；探针 17 / 17 标「一次性实测、组件层未钉」；浏览器「未验」如实。判断项 7 对 `inFlightRenewal` 的说法与 oidc.ts 相符，
判断项 1 与 03 的 comfortable 默认一致；未动 `navigation.ts` / `pages/`；`SessionBadge` 删除为推送方裁决，不计越权。

**Standards 0 / 3 · Spec 0 / 2** → 无阻断，可重放。

### 处置（推送方 · 通道 1 · 18:0x）

- Standards 1 / 2 / 3 → 推送方代落 `ed7ef224`（只改注释与空白：去「第三样」；`data-density` 注释写实为「给只能从 DOM 读档的消费者用，`ListPageTemplate` 走 `useDensity` 不读它」；`AuthGate.tsx` 补末行换行）。
- Spec 1 → 记入未验：**存储为 dark 时首帧 Light → `.dark` 挂上的反向闪帧**与「Light 首屏是否有 dark 闪帧」同为未验；修法（`index.html` 内联脚本预挂 class）越地盘，随首个浏览器验收的票一并看，不另立票。
- Spec 2 → 记；`ScopeChip` 的说明改挂可聚焦元素或加 `aria-describedby`，留给 02 动 `Layout` / 导航那一轮顺手，或首个浏览器验收时一并定。

### 进 main 记录（推送方 · 通道 1）

- 重放：`idp-land-ux01` 上 cherry-pick `7d29af39..113d048f` 九笔到 main `94bd39fd` 零冲突（本票 12 件与 main 其后各笔零重叠，`git merge-tree` 干跑先核过），SHA 对照
  `949706e6→9b0e30d4` / `a8fe62a4→aed4b860` / `2b8847cb→dc400b78` / `c3c2555a→ee3c2cf0` / `ea8b63b8→35244703` / `170c3527→9e8f1c83` / `0ec5dfa5→13db355a` / `5269239e→83da6021` / `113d048f→dd57ede1`；
  12 件与作者 tip 逐字节同。推送 tip `ed7ef224`（含 Standards 1 / 2 / 3 注释笔）。
- 门禁在 `ed7ef224` 上实跑：`tsc -b --noEmit` 0 / `run-tests` **324**（main 313 + 本票 11）/ `vite build` 0（产物含「Parcel / IDP」1 处）/ `gofmt -l` 空 / `go build` 0 / `go vet` 0 / 清点重生成零差 /
  带 DSN 全量 `-p 1 -count=1` 18:06:16→18:08:15 **115 ok / 0 FAIL / 16 无测试 / 0 cached**，DSN 判别单跑为 PASS。
- 18:08:27 `ls-remote` 核 `94bd39fd` 未动 → `push ed7ef224:main` 成，**远端 main = `ed7ef224`**；共享树 ff 同 SHA。评审到之后才推。
- 浏览器未验沿作者所报，另加评审 Spec 1 那一向。02 的 Blocked by 随本票进 main 解除。
