# 06 Loading 按内容形状出骨架、错误按页 / 区块 / 动作分层、空态换 ui-primitives 三件

Category: enhancement
Status: in-progress
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
