# 01 壳层：Top Bar 四件（归属信息 / 全局搜索位 / 作用域位 / 用户菜单位）、Light 默认 + 主题切换、密度两档

Category: enhancement
Status: ready-for-agent
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

## Comments
