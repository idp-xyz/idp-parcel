# 06 Loading 按内容形状出骨架、错误按页 / 区块 / 动作分层、空态换 ui-primitives 三件

Category: enhancement
Status: resolved
Blocked by: 无（03 / 04 改 `templates/ListPageTemplate.tsx` / `DetailPageTemplate.tsx`，本票**不碰这两个文件**；形状经 `TemplateViewState` 传进 `StateSlot`）
地盘：`apps/admin-web/src/templates/state-slot.tsx`、`components/states/index.tsx`（+ 新 test）、`pages/catalogue-view.ts`（+ 既有 test）。**不逐页改 36 张列表页**：
形状默认值由 `catalogueViewState` 给，页面零改动。
出处：spec「缺口」表第六档；手册「Loading」（列表出 skeleton 不出转圈；骨架按内容形状）、「Error State」（whole page / widget / section / action /
background refresh 五层，局部隔离、不整页红）、「状态与反馈规范」空状态五分（已对齐的那半不动）；ui-patterns `Skeleton` / `SkeletonRow` /
`SkeletonTable` / `SkeletonEventList`、ui-primitives `FilteredEmptyState` / `TrulyEmptyState` / `UnavailableState` / `SectionErrorState`（loms-web 在用）。

## 为什么

`LoadingState` 今天是一块通用的四行脉冲，列表页、详情页、抽屉里都是它——手册要的是「骨架长得像即将出现的内容」：表格出表格骨架（列数对得上），
时间线出事件列表骨架。错误今天只有一态：`error` 一来整个内容区换成 `ErrorState`，抽屉里的一段历史读失败、一个动作没送出去，也没有比「整页红」更细的形。
这两处都不是内容问题，是形态没分层；模板与状态槽改一处，全站跟着变。

## 要做的

1. **Loading 带形状**：`TemplateViewState` 的 `loading` 态加可选 `shape?: 'table' | 'list' | 'detail' | 'block'`（与可选 `cols?: number`、`rows?: number`）；
   `StateSlot` 按形状渲染——`table` → `SkeletonTable`、`list` → `SkeletonEventList`、`detail` → 头区一行 + 两块 `SkeletonCard`、`block` / 不传 → 今天的
   `LoadingState`（默认不变）。`catalogueViewState`（36 张目录页共用的判读）在 `loading` 态默认给 `shape: 'table'`——页面零改动即换骨架；`cols` 它不知道，
   用 `SkeletonTable` 默认列数（03 若想传真实列数，在它自己的模板文件里补，不在本票）。
2. **错误分层**：
   - **页级**（whole page）：`ErrorState` 照旧，仍由 `StateSlot` 的 `error` 态渲染——这是「详情半份比没有更误导」那条红线的形态，不动。
   - **区块级**（section / widget）：`components/states` 新出口 `SectionError({ title, description, onRetry })`——包 ui-primitives `SectionErrorState`，
     文案由调用方给（本仓文案规则），一段区读失败只红那一段；供抽屉历史区、表单内册读失败这类地方用。**本票只出件 + 一处首用**：`pages/party/
     RevisionHistorySection.tsx` 的 `noAnswer` / `transport` 两态换成它（文案逐字保住、重试按钮照旧）——这是唯一跨出地盘的一处，与 03 / 04 文件不交，票面写明。
   - **动作级**（action）：登记 / 发布口的答案区（`RegistrationPanel` 的 `answered` 态）今天已是动作级反馈，不整页红——只在票面记「已对齐」，不动。
   - **后台刷新**（background refresh）：`useRegisterList` 重取时保留旧答案（票 admin-web-group-legal-entities/13 第 6 条）已是这一层——记「已对齐」，不动。
3. **空态换三件**：`components/states` 的 `EmptyState` 内部改包 ui-primitives `TrulyEmptyState`（`title` / `description` / `onCreate` 由本仓的 `action` 映射），
   `ErrorState` 的 `transport` 一类「服务不可达」改包 `UnavailableState`（若 `ErrorState` 的调用方今天分不出传输失败与服务端未形成答案，就只包一层、不分）；
   `FilteredEmptyState` 只收 `onReset`、文案不可定制——**不换**（列表筛空那一行 `emptyRowsNote` 的措辞是票 admin-web-group-legal-entities/01 裁的，文案归本仓），
   理由写进票面。原语渲染出的文字若含英文默认句（`TrulyEmptyState` 无 title 时），一律传本仓文案盖住，评审核 build 产物无英文缺省句漏出。
4. **`components/states` 与 `state-slot` 的纯逻辑带 test**：`shape` → 骨架件的映射、`catalogueViewState` 的 `loading` 默认形状（既有 `catalogue-view.test.ts`
   加一条）。

## 不做

- 不碰 `ListPageTemplate.tsx` / `DetailPageTemplate.tsx`（03 / 04 的地盘）；不改 36 张页。
- 不做「骨架与真实列宽对齐」（要列定义，归 03 若它想做）。
- 不做全局错误边界（React ErrorBoundary）——那是崩溃处理，不是错误状态分层；另议。
- 不改任何文案与状态词。

## 完成判据

- 三道门绿；既有 `run-tests` 零改动仍绿（`catalogue-view.test.ts` 只加不改）。
- 新 test：`shape` → 骨架件映射全覆盖；`catalogueViewState` loading 默认 `shape: 'table'`；`SectionError` 渲染出重试按钮（`renderToStaticMarkup`——若测试编译链
  引不到 `.tsx` / ESM 原语，改为钉纯逻辑并写明「组件层未钉」）。
- `vite build` 产物 grep 无 ui-primitives 英文缺省句（如 `No results` / `Nothing here` 一类，按原语源码实有的查）。
- 浏览器验收做不到如实写「未验」。

## 裁决

1. **形状经 `TemplateViewState` 走而不是改模板**：三票同时改两个模板文件必撞；状态槽是三张模板共用的一处，形状是状态的属性，放这里正合它的职责。
2. **`FilteredEmptyState` 不换**：文案是内容，内容规范赢形态规范（spec 红线第一条）。
3. **区块级错误只出件 + 一处首用**：全站换法归各页自己的票；本票证明件能用、形对，不逐页铺。

## Comments

### 认领（2026-09-20 12:5x）

通道 5，分支 `mcp5-ux06` 基 main `86a96ab7`；地盘 `templates/state-slot.tsx`、`components/states/`、`pages/catalogue-view.ts`（+ test）、首用 `pages/party/RevisionHistorySection.tsx`。

### 完成记录（通道 5 · 2026-09-20 12:5x–13:2x · 分支 `mcp5-ux06` 基 main `86a96ab7` · tip `e655c020`）

**笔** `8b175662`（认领）→ `547e1072`（1）→ `e0d5e642`（2）→ `e655c020`（3）→ 本票面笔。每笔 push origin；不推 main、不占 55432、不跑 Go。
未碰 `ListPageTemplate.tsx` / `DetailPageTemplate.tsx` / `Layout` / `App` / `index.css` / `domain/status.tsx`，36 张页零改动。

**要做的逐条**

1. ✅ `TemplateViewState.loading` 加 `shape?: LoadingShape` / `cols?` / `rows?`；`StateSlot` 交 `LoadingSkeleton` 按形状摆——`table` → `SkeletonTable`
   （它自身是 tbody，套在 `table` 里）、`list` → `SkeletonEventList`、`detail` → 头区一行 + 两块 `SkeletonCard`、`block` / 不传 → 今天的 `LoadingState`。
   形状 → 件的映射是纯 `templates/loading-shape.ts` 的 `skeletonOf`（理由见第 4 条）。`catalogueViewState` 的 `loading` 默认 `shape: 'table'`，`cols` 不传用
   骨架件默认。`LoadingShape` 从 `templates` 桶导出。`547e1072`。
2. ✅ 页级 `ErrorState` 不动。区块级：`components/states/section-error.tsx` 的 `SectionError({ title, description?, onRetry? })`，`role="alert"` 窄区块、危险色
   左侧色条 + 标题、说明一行、给 `onRetry` 才出「重试」；从 `components/states` 桶导出。**不包 `SectionErrorState`**：其标题「Section failed to load」与按钮
   「Retry」写死、只开放 `message`，包一层即漏英文缺省句——文案归本仓（spec 红线；与裁决 2 同一条理由），照其形自绘，原语开放文案后可换成包它。
   首用 `RevisionHistorySection` 的 `noAnswer` / `transport` 两态：字逐字保住——「服务端未形成答案（HTTP n）」/「无法连接主数据读取服务」作标题、
   `problemNote` / `message` 作说明，原句中间那个「：」变成标题 / 说明两行的结构；重试照旧；与 `catalogueViewState` 的错误口径一致。动作级
   （`RegistrationPanel` `answered` 态）与后台刷新（`useRegisterList` 保留旧答案）**已对齐**，只在 `components/states/index.tsx` 头注记层次。`e0d5e642`。
3. ✅ `EmptyState` 形取 `TrulyEmptyState`，标题 / 说明总是传（本仓缺省），`action` 不映 `onCreate`（那颗按钮的字「Create First Record」写死），仍收 ReactNode 摆在
   卡片下。`ErrorState` **判不换** `UnavailableState`：标题 / 说明 / 按钮全写死英文、只收 `onRetry`，且调用方今天把传输失败与服务端未形成答案都送到这一态只靠
   `title` 区分，原语没有传这句话的口；形保持今天的。`FilteredEmptyState` 不换（裁决 2）。`e655c020`。
4. ✅ 新 test 三份：`templates/loading-shape.test.ts`（四形状全覆盖、不传 = block）、`pages/catalogue-view.test.ts`（loading 默认表；**该文件此前不存在**，票面
   「既有」有误，新建）、`components/states/section-error.test.ts`（`renderToStaticMarkup`：有 `onRetry` 才出一个重试按钮、标题说明逐字在、`role=alert`）。

**组件层断言实测（spec「验收口径」要首个做到的票记，后面的票照抄）**

- `.test.ts` 引 `.tsx`：tsc 按 `tsconfig.test.json` 连带发成 CJS **没问题**；`react` / `react-dom/server` / `lucide-react` 都有 CJS 入口，`renderToStaticMarkup`
  能跑——`section-error.test.ts` 就是这样钉到的。
- 引 `@idpxyz/ui-patterns` / `ui-primitives` 的组件**钉不到**：两包 `exports` 只有 `types` + `import` 条件、无 `require` / `default`，CJS 一 `require` 即
  `ERR_PACKAGE_PATH_NOT_EXPORTED`；Node 22 的 `require(esm)` 过不了 `exports` 这一关。出路二选一，都不在本票：上游包补 `default` 条件（归用户 / idp-ui），或
  `run-tests.mjs` 改发 ESM（要解决无扩展名 import 的解析）。本票按票面兜底把形状映射抬进纯 `.ts` 钉；`StateSlot` 真渲染出 table / 事件列表这一层**未钉**；
  `SectionError` 单独成文件正为绕开 `index.tsx` 对 ui-primitives 的引用。

**完成判据**

- 三道门 ✅ 每笔提交信带数字；tip `e655c020`：`tsc -b --noEmit` 0 / `run-tests` **295**（main `86a96ab7` 290 + 5）/ `vite build` 0。既有用例零改动：
  `git diff 86a96ab7..e655c020 -- '*.test.ts'` 三文件全为新增。
- 新 test ✅ 映射全覆盖 / 默认 `table` / `SectionError` 重试按钮，见第 4 条。
- build 产物 grep ⚠️ **判断项**（钉 `e655c020`，`dist/**/*.js` 按原语源码实有的十句查）：`No matching results` / `Permission required` /
  `Data temporarily unavailable` / `Retry in a moment` / `Section failed to load` / `Loading workspace` / `Clear Filters` 七句 **0**（未引的原语全被 tree-shake 掉；
  基线 `86a96ab7` 十句皆 0）；`No records yet` / `There is no data available` / `Create First Record` **各 1**——是 `TrulyEmptyState` 函数体内的兜底分支，本仓总传
  `title` / `description`、从不传 `onCreate`，三句**不可达**。字面判据「产物无英文缺省句」不成立、语义判据「无英文缺省句漏出页面」成立；要连字节都不进产物只能
  不引原语、照形自绘 `EmptyState`——那样第 3 条就一件不换了，交评审裁。
- 浏览器验收**未验**（本机无 AuthGate 会话路径）。

**判断项 / 交评审**

- 三件原语里两件（`SectionErrorState` / `UnavailableState`）文案写死不可盖，票面「文案由调用方给」「传本仓文案盖住」对它们做不到；本票按红线自绘 / 不换。
  若评审判「形态优先、英文可接受」，把 `SectionError` 改成包 `SectionErrorState`、`ErrorState` 包 `UnavailableState` 各是一笔小改。
- `EmptyState` 换成 `TrulyEmptyState` 后是带边框与 `bg-idpxyz-editor` 的卡片，`ErrorState` / `UnconfiguredState` 仍是居中文字块——三态形不再同一族；要齐要么
  后两者也照 `StateShell` 的形自绘，要么等原语开放文案。归后续票。
