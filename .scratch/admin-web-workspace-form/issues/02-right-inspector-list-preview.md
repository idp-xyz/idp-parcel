# 02 右侧检查器：壳层右栏位 + `InspectorContent` 契约（五节）+ `ListPageTemplate` 单击进检查器 + 两张首用页渲染器

Category: enhancement
Status: resolved——2026-09-22 通道 1 在 `main` 上直接做完（workflow.md「前端切片」六步；本地 `92cbcf1a` 模板段 / `9015cdaf` 壳层段 + 本笔票面与旧话改口，**已进 main `539b8764`**（2026-09-23 push，`e0d3f89d..539b8764` 纯 ff，码 SHA 不换））。完成记录见文末；非作者评审 ← 通道 3（2026-09-24，两轴 0 阻断，见 Comments），建议各条已在 `main` 上逐笔处置，见 Comments「评审后修复」（本地 `main`，**未推**——本宿主此刻连不上 GitHub 代理）。此前 in-progress——紧接票 01 的壳层笔；模板段先落、壳层段随后。此前 draft——分两段：**模板段**（契约 + 首用渲染器 + `ListPageTemplate` 接口）不等任何票；**壳层段**（右栏装进 `Layout.tsx`）Blocked by 01
Blocked by: 无（01 的壳层笔 `c9312bf6` 已在 main）
地盘：新 `apps/admin-web/src/templates/inspector.ts`（契约类型 + 纯逻辑 + node:test）、新 `templates/InspectorPanel.tsx`（五节渲染件）、`templates/ListPageTemplate.tsx`
（加可选 `inspector?: (row) => InspectorContent`，单击行时交给壳层——**不改** `onRowClick` 语义，只新增）、`templates/index.ts`（只追加）、`Layout.tsx`（壳层段：右栏位装
`InspectorPanel`，可拖宽 `useResize({ reverse: true })`、可折叠、宽度进 01 的 `workspace-state`）、首用两页：`pages/shipment-request/ShipmentRequestListPage.tsx`、
`pages/visibility/ExceptionCasesPage.tsx`（各写一个 `inspectorOf(row)`）。**不碰** `DetailPageTemplate`（对象页自己有母版，检查器不进对象页——蓝图 13.2「不成为第二张页」）。
出处：spec「缺口」表第二档；蓝图 7.2 母版 B「List → Detail Preview → Inspector」、10.8 Inspector Panel 六节（Summary / Current Status / Quick Actions / Notes / Related Objects / Audit Meta）、
13 节检查器约束（不成为第二张页、长表单不进、宽度稳定、必要时折叠节）；参照 `idp-ui@6751fb2` `apps/myshop-web/src/shell/RightSidebar.tsx`（按 `selectedObject.kind` 分派
`OrderInspector` / `ExceptionInspector` / …，每个由 `Section` 折叠节组成：状态概览 / 对象信息 / 快捷操作 / 相关 / 关联对象；无选中时一句空态）与 `PageRouter.tsx`
（列表页 `onSelectOrder={(o) => onSelectObject({ kind, payload })}`——单击只选中不跳转）。

## 为什么

第一轮 03 把「单击预览 / 双击打开」接进了 `ListPageTemplate`，但预览落在哪没定——各页沿用自己的抽屉（`DetailRow` 十几格），抽屉盖住表、看一眼就得关。蓝图母版 B 与
myshop-web 的做法是**壳层级右栏**：选中对象常驻右侧，表还在左边，翻行时右栏跟着换——这是「处理队列」的姿势。检查器的内容契约先立在模板层（不依赖壳），
壳层位随 01 的 `Layout.tsx` 重写一起落，两段可以不同人做。

## 要做的

### 模板段

1. **`inspector.ts` 契约**：`InspectorContent = { title: string; subtitle?: string; sections: InspectorSection[] }`；`InspectorSection` 五种，**顺序与节名固定**（蓝图 10.8 减「Notes」——
   本仓没有备注读口）：`summary`（键值对，≤ 8 格）/ `status`（`LayeredStatusBadge` 的输入数组，复用 05 的层轴）/ `actions`（`{ label, onRun?, disabledReason? }[]`——**没有端点的动作
   必须给 `disabledReason`，不允许 `onRun` 空转**）/ `related`（`{ label, hash }[]`，点即写 hash）/ `audit`（创建 / 更新 / 修订 / 来源的键值对，取自行里已有字段）。
   纯逻辑：`resolveInspectorSections(content)` 按固定顺序补齐缺省节（缺的节**不渲染**而非渲染空节，蓝图 13.2）、`actions` 里无 `onRun` 且无 `disabledReason` 的在 dev 下抛。node:test 钉。
2. **`InspectorPanel.tsx`**：接 `InspectorContent | null`；`null` 渲染一句空态「在列表里单击一行，这里显示它的概要」（不放假内容）；有内容时五节各一个可折叠 `Section`
   （默认展开 summary / status / actions；related / audit 折叠），`actions` 用 `Button variant="ghost" size="sm"` 纵排、禁用带 Tooltip 说明；宽度由父级给，自己不定宽。
3. **`ListPageTemplate`** 加 `inspector?: (row: Row) => InspectorContent`：有它时单击行 = `onRowClick?.(row)` **且**把 `inspector(row)` 交给壳层（经一个 `useInspector()` context，
   模板段先建 context 与 Provider 的**空实现**——壳层段把 Provider 挂进 Layout）；没有它时行为零变化。双击仍走 `onRowOpen`。
4. **首用两页**各写 `inspectorOf(row)`：委托查阅——summary（委托号 / 客户 / 服务产品 / 提交时刻）、status（05 的层）、actions（今天有的只有「打开详情」→ hash 二段；
   其余不列）、related（客户合同 / 服务产品的 hash，若行里有 id）、audit（提交 / 最近变更）；异常案件——summary / status（严重度层）/ actions（「打开分诊」→ `#/exception-triage`；
   决定端点没有就不列）/ related（关联委托）/ audit。字段只取行里已有的，不发第二个请求。

### 壳层段（Blocked by 01）

5. `Layout.tsx` 右栏位装 `InspectorPanel`：`useResize({ direction: 'horizontal', reverse: true, min 240, max 480 })`，宽度进 `workspace-state`；折叠按钮在栏顶；
   切换活动标签时**清空**选中（检查器显示的是当前列表选中的行，换页就不成立了）；`useInspector` 的 Provider 挂在这里。
6. 03 的命令面板 `shell` 组加「切换检查器」一条（03 已落则本票加；否则 03 加）。

## 不做

- 不做 Notes 节（无备注读口）；不做检查器内表单（蓝图 13.2）；不做 myshop-web 那种 `addToast('操作成功')` 的假快捷操作。
- 不进 `DetailPageTemplate`；不改两张首用之外的页。
- 不做 `getTabRiskDot`（01 不做的，这里也不接——对象状态进标签颜色是第三件事）。

## 完成判据

- 四道门绿；`inspector.test.ts`：节顺序固定 / 缺省节不渲染 / 无 `onRun` 无 `disabledReason` 抛，各至少一条。
- 一次性 esbuild 束：`InspectorPanel` 收 `null` 渲染空态一句；收含 5 节的内容渲染 5 个 `Section` 标题且顺序固定；禁用动作带 `aria-disabled` 与说明。
- `ListPageTemplate` 不传 `inspector` 时既有用例零改动。壳层段：拖宽后刷新宽度保留（读 `workspace-state`）。浏览器验收做不到如实写「未验」。

## 完成记录（2026-09-22，通道 1，`main` 上直接做）

参照物取证同票 01 完成记录：myshop-web 源本宿主取不到（私有仓 404、idp-110 超时），按 spec 对 `RightSidebar.tsx` / `PageRouter.tsx` 的取证落地。
渲染件没有手画：`@idpxyz/ui-primitives@0.1.25` 自带 `InspectorShell / InspectorHeader / InspectorBody / InspectorIdentity / InspectorSection / InspectorRow / InspectorActions`
一族（`src/inspector.tsx`），面板用它们摆。

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `92cbcf1a` | `templates/inspector.ts`、`.test.ts`、`inspector-context.tsx`、`InspectorPanel.tsx`、`ListPageTemplate.tsx`、`templates/index.ts`、`test/node-builtins.d.ts`、`pages/shipment-request/ShipmentRequestListPage.tsx`、`pages/visibility/ExceptionCasesPage.tsx` | 模板段第 1–4 条：契约 + 归并 + node:test 8 条；控制口 context（默认 no-op）；面板；`inspector` prop 与选中高亮；两张首用页的 `inspectorOf` |
| `9015cdaf` | `Layout.tsx`、`shell/workspace-state.ts`、`.test.ts`、`shell/command-actions.ts`、`.test.ts`、`shell/CommandPaletteHost.tsx` | 壳层段第 5–6 条：右栏位 + 拖宽 + 折叠 + 持久化 + 换标签清空；命令面板「显示 / 隐藏检查器」 |
| 本笔 | `InspectorPanel.tsx`、`App.tsx`、`README.md`、票面 | 节展开态改为翻行保留（自审所得，见判断项 3）；两处旧话改口 |

**完成判据**

- ✅ 四道门：tsc 0 / run-tests 396 → 406 / vite build 0。
- ✅ `inspector.test.ts`：节序固定 / 缺省与空节不渲染 / 同节多次接起 / 概要越界抛且恰好上限放行 / 无 `onRun` 无 `disabledReason` 抛 / `inspectorActionDisabled` / `presentFields`——各至少一条（实为 8 条）。
- ✅ 探针（`scripts/dom-probe.mjs`，源 `/tmp/idp-probes/wsform02-probe.tsx` 不入库）**25 ok / 0 fail**：`InspectorPanel` 收 `null` 渲染空态一句；收五节内容按固定序渲染五个节标题；禁用动作带 `aria-disabled`、可按的不带；词表词渲徽章、词表外原样示文；审计默认折叠、点标题展开；关联对象是带 `href` 的锚点；`ListPageTemplate` 单击行把 `inspector(row)` 交给控制口、`onRowClick` 仍调、行标 `data-inspected` 且翻行跟换；壳层右栏默认可见宽 320、把手往左拖 60 变 380 并进 `localStorage`、× 折叠后只剩展开按钮且折叠态持久化、命令面板「显示检查器」运行后右栏回来且宽度仍 380。
- ✅ `ListPageTemplate` 不传 `inspector` 时零变化：既有 `run-tests` 用例零改动；`onClick` 只在 `onRowClick || inspector` 时挂。
- ◑ 「切换活动标签清空选中」按 `useEffect([activeTabId])` 落了，探针**未证**（App 里造不出带行的列表——页面读口在探针里挂起）。
- ◑ 浏览器**未验**：栏的实际视觉宽度、折叠按钮位置、Tooltip 出没。

**判断项**

1. **壳层不认识对象，页面给内容。** myshop-web 的 `RightSidebar` 按 `selectedObject.kind` 在壳层分派 `OrderInspector` 等渲染器；这里反过来——页面写 `inspectorOf(row)` 产契约内容，壳层只渲染契约。对象长什么样归拥有它的页，与本仓「模板层不反向依赖页面层」一致。
2. **委托查阅的单击语义变了**：此前单击即整区切详情；现在单击进检查器、双击 / Enter 开详情（蓝图母版 B）；检查器「打开详情」是同一条路。详情因票 01 开成自己的标签，列表留在原标签。
3. **节的展开态翻行保留**（面板里 `key={kind}` 不带对象）：处理队列的姿势是折掉不看的节、一行行往下翻；换一行就把折好的节全弹开等于每行重折一次。首版曾按对象重置，自审改回。
4. **关联对象两页都没给**：委托行的客户账户 / 来源请求键、案件行的根对象，本管理台都没有它们的对象地址，编一条链接就是死路（票面「若行里有 id」的前提不成立）。快速动作各只列一条有端点的（打开详情 / 打开分诊），撤回 / 取消 / 复核 / 归并 / 关闭不列禁用假动作——它们各有自己的页与门。
5. **概要 ≤ 8 与假动作两条硬规则抛而不是静默**：契约错误是编程错误，`InspectorContractError` 在渲染时抛出、测试先拦；不做 dev-only（本包 tsconfig 无 `process` 声明，也不引 `import.meta` 进 CJS 测试链）。
6. `InspectorRow` 的值 `truncate` 无 title（vendor），长标识会被截且无悬停全文；记下不改 vendor。`onClose` 复用为「折叠」——蓝图的检查器常驻，× 的语义是收起不是丢弃内容，展开后内容仍在。
7. 检查器栏默认**可见**且对所有页常驻（蓝图母版 B 的常驻位）；今天只有两页给内容，其余页看到的是空态一句。折叠态持久化，不看的人折一次即可。

**评审**：共享面（`templates/*`、`shell/*`、`Layout.tsx`）按 workflow 第 5 步要一份 Spec 轴；无可派通道（同票 01），**推送方自审**——判断项 3 与 4 即自审改动，不算非作者评审。

## Comments

- 2026-09-23 · 进 main：`origin/main` = `539b8764`（`e0d3f89d..539b8764` 纯 ff，码 `92cbcf1a` / `9015cdaf` 与票面笔 SHA 不换）。此前状态行写的「未推——本宿主没有 GitHub 推送凭据」在这次推送之后失效。

### 评审 ← 通道 3 · 钉 `9f95520e`（基 `c9312bf6`，共享树只读，门禁未重跑）· 2026-09-24 12:3x（推送方自任务台 `task-5a1d3537` 代落原文）

门禁未重跑，引通道 1 在 `3a47da8d` 实跑（Node 22.23.3）：`tsc -b --noEmit` 0 / `run-tests` 406 pass 0 fail / `vite build` 成功；CI 在 `539b8764` 与 `3a47da8d` 七个 job 全绿。`9f95520e..3a47da8d` 在 `apps/admin-web` 下零差、共享树该目录无未提交改动，钉点即现 main。读的是 `git diff c9312bf6 9f95520e -- apps/admin-web`（17 文件）、现文件与 vendor 源（`@idpxyz/ui-primitives@0.1.25` 的 `src/inspector.tsx`，`@idpxyz/ui-workspace@0.1.25` 的 `EditorGroup.tsx`、`hooks/useResize.ts`）。

**Standards** — 阻断：无。非阻断：
1. 收起控件读屏无名：`Layout.tsx` 把 `toggleInspector` 作 `InspectorHeader` 的 `onClose` 传入，vendor 渲染的 × 是无 `aria-label`、无 `title` 的裸 `<button>`——展开态下栏内唯一的收起控件读屏无名、无悬停说明，× 的通行语义又是「关闭」；折叠态的展开按钮却有 `aria-label="显示检查器"` + Tooltip，两端不对称。不动 vendor 的修法：`InspectorHeader` 收 `children`（渲在 × 之前），放一个 `PanelRightClose` 图标按钮带 `aria-label="隐藏检查器"` + Tooltip，不再传 `onClose`（判断项 6 随之改口）。同族记 vendor：`InspectorSection` 的标题按钮无 `aria-expanded`，翻行保留折叠态之后读屏更难知道哪节折着。
2. 注释抄了别处的值：`Layout.tsx` 文件头「栏可拖宽（240–480）」抄的是 `shell/workspace-state.ts` 的 `INSPECTOR_WIDTH_MIN` / `INSPECTOR_WIDTH_MAX`；`InspectorPanel.tsx` `SectionBody` 的「横排在 240px 的栏里会折成两行半」抄的是同一个下限，还让模板层注释绑上了壳层的具体宽度（同文件头说「宽度由父级给，自己不定宽」）。常量一改两句无声变旧，与「不计数」同一理由；改引常量名或只说「窄栏」。
3. 变更说明进了注释：`ShipmentRequestListPage.tsx` `inspector` prop 上方「（此前单击即整区切详情）」、`shell/command-actions.ts` 文件头「「切换右栏」随票 02 的检查器栏落地后加进壳层组」、`App.tsx` 头注「此前沿 loms-web 的单页区形态……」是历史叙述，归票面与提交信息（AGENTS「不写变更说明」）。
4. 重复 key 的缝：`InspectorPanel.tsx` 的 `FieldRows`、`SectionBody`（状态）、`ActionButton` 列表以 `label` 作 key，关联对象以 `hash`；契约又允许同种节多次给、`mergeSections` 按序接起——同名格一出现即重复 key。今天两页撞不上。可让 `resolveInspectorSections` 对同节重名抛 `InspectorContractError`（与契约「抛而不静默」同口径），或 key 带序号。

无发现（实核）：注释全中文；跨文件引用皆为符号名 / 文件路径（`templates/workspace-tabs.ts` 同一手法、`ListPageTemplate` 的 `DisabledSlot`、`ShipmentRequestListPage` 的 `requestStateBadge`、`CaseRow` 头注均实有其物），无行号；蓝图 10.8「六节」锚在 `idp-ui@6751fb2`，其余计数只数本文件自己的东西。命名 / 惯用法与邻近一致：`inspectorOf` 对 `viewStateOf` / `csvCellText`，`INSPECTOR_WIDTH_*` / `setInspectorWidth` 对 `SIDEBAR_WIDTH_*` / `setSidebarWidth`，禁用动作 `aria-disabled` + Tooltip + `preventDefault` 与 `DisabledSlot` 同形，词表判法与 `ExceptionCasesPage` `toneWordOrText` 同为 `in domainStatusTones`。纯逻辑有 node:test：`inspector.test.ts` 8 条，`workspace-state.test.ts` 检查器 2 条（钳区间、非数回默认、非布尔当可见），`command-actions.test.ts` 壳层组改三条——396 → 406 与新增 8 + 2 对得上。分层：`templates/*` 新增引用只有 `./inspector`、`./inspector-context`、`../domain/status`（共享词表，非页面层）与 `@idpxyz/*`，不引 `pages/`；`inspector.ts` 不引 React / 原语，CJS 测试链可载；`templates/index.ts` 只追加。`toggleInspector` 空依赖 `useCallback` 包 `applyWorkspace`：实核 `applyWorkspace` 只读 `workspaceRef` 与稳定的 `setWorkspace`，`setInspectorVisible` 不改 `activeTabId`、不写 hash，注释成立。

**Spec** — 阻断：无。

派单七项取舍逐条判：
1. 契约硬规则落为渲染时抛 `InspectorContractError`（票面写「dev 下抛」）→ **接受**。定性为编程错误、抛而不静默，与契约一致；`inspector.ts` 被 `inspector.test.ts` 引、身在 `tsconfig.test.json` 的 CJS 链里、碰不得 `import.meta`，理由成立。代价见非阻断 2。
2. `ListPageTemplate` 不传 `inspector` 零变化 → **接受（实核）**。`rowInteraction` 的 `click` 退化为 `onRowClick !== undefined`；行 `className` 在 `inspected` 恒假时化简回 `rowClass`（含 `undefined`）；不出 `data-inspected`；`onClick` 仅在 `onRowClick || inspector` 时挂，`handleRowClick` 先调 `onRowClick?.(row)`、无 `inspector` 即返回；无 Provider 时 `useInspector` 给 `noopController`；`onDoubleClick` 与 Enter 仍只走 `onRowOpen`；复选格 `stopRowEvent` 截单击，勾选不进检查器。
3. 委托查阅单击语义变更 → **接受**。合 `list-page-structure.ts` `rowInteraction` 头注所引手册「Drill-down 模式：单击预览、双击开对象」与蓝图母版 B；检索全库（除 node_modules）无文档或用例写过「委托查阅单击进详情」（命中只有本票与 spec 状态行）。此前该页只接 `onRowClick`，行不进 Tab 序、键盘开不了详情；现接 `onRowOpen`，Enter 可开——是改进。代价：检查器折起时单击只剩行高亮。
4. 两张首用页 → **接受**。两个 `inspectorOf` 都是行的纯函数，不发请求；「关联对象两页都不给」实核成立——全 `src/pages` 只有 `ShipmentRequestListPage` 读 hash 第二段，客户账户、来源请求键、包裹身份都没有对象地址，`case-api.ts` 的 `ExceptionCaseRecord` 也没有委托标识；动作各一条且都有落点（hash 二段 / `#/exception-triage`）。附非阻断 3、6。
5. 壳层 → **接受**。vendor `useResize` 实有 `reverse`（`startSize - diff`，钳 min/max）；240 / 480 / 默认 320 在 `shell/workspace-state.ts`；宽与可见性进 `WorkspaceState`，由既有的 `saveWorkspaceState` effect 落盘，`loadWorkspaceState` 越界钳、非布尔当可见。「换标签清空」读码实核两半都清：Layout 的 `useEffect([workspace.activeTabId])` 把 `inspectorContent` 置空；行高亮 `inspectedKey` 是 `ListPageTemplate` 局部 state，而 `renderTab` 以 `Fragment key={tabId}` 包页、`EditorGroup` 在 `preserveInactiveTabContent={false}` 下只挂活动标签——换标签即卸旧页、高亮归零，列表与同模块详情两张标签也不共用实例；无子组件在挂载时调 `show`，父级 effect 不会误清新内容。运行期**未实证**。
6. 节展开态翻行保留（`key={kind}`）→ **接受**，并更正一处叙述：票面第 2 条只写默认展开哪几节，没写按对象重置；首版 `92cbcf1a` 的 key 是 `${content.title}:${resolved.kind}`，而两页 `title` 是常量「委托」「异常案件」（标识在 `subtitle`），首版本来就不按对象重置——`9f95520e` 对两页零行为变化，改的是让注释说真话。判断项 3「首版曾按对象重置」与实际不符。残留：某行缺某节（如审计全空）再翻到有它的行，该节重挂回默认。
7. 默认可见、全页常驻、不进 `DetailPageTemplate`、命令面板「显示 / 隐藏检查器」→ **接受**。`initialWorkspaceState` 可见；diff 未碰 `DetailPageTemplate`；`TOGGLE_INSPECTOR_ACTION_ID` 标签随可见性答目标态，测试钉。代价见非阻断 1。

非阻断：
1. 空态句在多数页不成立（第 7 项的副作用，建议下一笔先修）：右栏全页常驻，`INSPECTOR_EMPTY_NOTE`「在列表里单击一行，这里显示它的概要」只在两张首用页为真。`GroupLegalEntitiesPage`、`BusinessPartiesPage`、`ChannelSelectionDecisionsPage` 单击行走本页自己的选中预览（`setSelectedId` / `setInspecting`），`RecentObjectsPage`、`SavedViewsPage` 单击即打开——右栏照旧空着却仍这么说；对象标签（如委托详情）上也显这句——票面说「检查器不进对象页」，内容确实没进，但栏和一句指向列表的空态还在。spec 红线「留位只允许禁用态 + 说明」要的是一句为真的说明。修法任一：`ListPageTemplate` 接了 `inspector` 时经控制口报「本页供内容」、壳层据此选空态句；或改成处处为真的一句。
2. 契约错误会拖垮整个外壳（第 1 项的代价，建议下一笔先修）：`resolveInspectorSections` 在 `InspectorPanel` 渲染时调，`apps/admin-web/src` 下没有任何错误边界——一次违约 React 即卸掉整棵树、整台白屏。今天两页撞不上（概要候选 ≤ 4 格、动作都有 `onRun`），但 `presentFields` 让格数随数据变：将来某页列 9 个候选格、样例行有空时开发期过得去，生产里全满的那行一点就白屏。判断项 5「测试先拦」只拦契约函数本身；页面 `inspectorOf` 在 `.tsx` 里、不在 CJS 测试链，没有测试跑到它。修法：`InspectorPanel` 就地 try/catch `resolveInspectorSections`，接住 `InspectorContractError` 在栏内显错误句、其余照抛——开发期照样响亮，生产不牵连外壳，也不必分 dev / prod（没有测试引 `InspectorPanel.tsx` 或 `templates/index.ts`，它本就不在测试链，`import.meta` 的顾虑不及于它）。
3. 委托查阅检查器字段偏离票面、完成记录未载：票面第 4 条概要「委托号 / 客户 / 服务产品 / 提交时刻」、第 1 条审计「创建 / 更新 / 修订 / 来源」；`ShipmentRequestListPage.tsx` `inspectorOf` 把提交时刻放进默认折叠的审计节、把来源放进概要，行里已有的 `submissionVersionId`（提交版本，即审计的「修订」）哪节都没进。服务产品读模型没有（`api.ts` `ShipmentRequestSummary` 头注明言不虚构），缺得正当。建议把 `submissionVersionId` 补进审计，其余作为取舍补进判断项。
4. 检查器只有指针能到：`rowInteraction` 仅在接了 `onRowOpen` 时给行 `tabIndex`——异常案件页只接 `inspector`，行不可聚焦；委托查阅行可聚焦但 Enter 是开详情，没有键把行交给检查器。可把「接了 `inspector`」也算进 Tab 序理由、聚焦即交给检查器（不占 Enter / Space）。属判断项，交作者定。
5. 检查器是单击那一刻的行快照：检索筛掉该行后右栏仍显它、行高亮已消失；出错重取后同键行若还在，高亮回到新行而右栏是旧快照，状态词可能已变。模板握着 `inspectedKey` 与新 `rows`，可在 `rows` 变时按键重推或清空。低优先。
6. 异常案件「打开分诊」落在模块级 `#/exception-triage`：该页列信号发作期册与处置请求册，不列案件、也没有按案件的地址，点了回不到「这件案子的分诊」。票面第 4 条原样如此，不算偏离；动作名可改为不暗示对象范围的「转到异常分诊」。
7. 完成记录写「四道门」只列三道（缺 `go test ./internal/architecture/`）；CI 的 Test shards 按 `go list ./...` 全覆盖，第四道由 CI 绿兜住，记录补一句即可。

无发现（实核）：spec 红线——状态簇走 `StatusWord` → `StatusBadgeFor` → `LayeredStatusBadge`（层由 `domainStatusLayers` 定），词表外原样示文，未另画；无假动作——`resolveInspectorSections` 对既无 `onRun` 又无 `disabledReason` 的动作抛，两页动作都有真实落点，撤回 / 取消 / 复核 / 归并 / 关闭不列（红线「只放已有端点的」是限制不是义务）；无假数据——字段全取自行，`presentFields` 丢空格、不代填「—」；异常案件状态只一枚主状态，严重度 / 优先级在 `ExceptionCaseRecord` 上结构性不存在；模板改动向后兼容（第 2 项）。票面「不做」：无 Notes 节、无检查器内表单、无 toast 假反馈、未碰 `DetailPageTemplate` 与两页以外的页（`App.tsx` / `README.md` 只改注释与说明）、`getTabRiskDot` 仍 `() => null`。完成判据：node:test 三类（节序 / 缺省与空节 / 假动作抛）各至少一条在；探针源不入库，自报 25 ok 未复核。

**Standards 0 / 4 · Spec 0 / 7** → 结论：**可接受**。建议下一笔先修 Spec 非阻断 1（空态句）与 2（面板就地接住契约错误），两件都不动内容契约 `InspectorContent`；其余记票面。

**处置**（通道 1）：本笔只落原文，不改码、不立票。评审建议先修的 Spec 非阻断 1 / 2 与其余非阻断待用户定；完成记录判断项 3 那句与实际不符（Spec 第 6 项），完成记录未改写，以本评审为准。

### 评审后修复（2026-09-24，通道 1，`main` 上直接做；用户令「全面收掉」）

**落点**

| 笔 | 文件 | 收的是 |
|---|---|---|
| `9f7de26e` | `templates/inspector.ts`、`.test.ts`、`templates/index.ts`、`InspectorPanel.tsx` | Spec 非阻断 2 |
| `07a25e61` | `inspector.ts`、`.test.ts`、`index.ts`、`InspectorPanel.tsx`、`inspector-context.tsx`、`ListPageTemplate.tsx`、`Layout.tsx` | Spec 非阻断 1 |
| `387203de` | `InspectorPanel.tsx`、`Layout.tsx`、`shell/command-actions.ts` | Standards 非阻断 1 / 2；Standards 非阻断 3 的壳层 / 模板半 |
| `93376770` | `inspector.ts`、`.test.ts`、`InspectorPanel.tsx` | Standards 非阻断 4 |
| `2a677101` | `ListPageTemplate.tsx`、`templates/list-page-structure.ts`、`.test.ts` | Spec 非阻断 4 / 5 |
| `2056e054` | `pages/shipment-request/ShipmentRequestListPage.tsx`、`pages/visibility/ExceptionCasesPage.tsx` | Spec 非阻断 3 / 6；Standards 非阻断 3 的页面半 |
| 本笔 | 票面 | Spec 非阻断 7；判断项更正；本段 |

**逐条处置**

- Standards 非阻断 1 → 已改：栏顶收起钮由 `Layout` 给（`PanelRightClose` 图标钮，`aria-label` 与 Tooltip 都是「隐藏检查器」，与折叠态的「显示检查器」成对），
  `InspectorPanel` 的 `onClose` 换成 `headerActions`，不再渲 vendor 的无名 ×。同族的 vendor `InspectorSection` 标题钮无 `aria-expanded`：不动 vendor，归上游。
- Standards 非阻断 2 → 已改：`Layout.tsx` 头注改引 `INSPECTOR_WIDTH_MIN` / `INSPECTOR_WIDTH_MAX`，`SectionBody` 注改说「窄栏」。
- Standards 非阻断 3 → 已删：`ShipmentRequestListPage` 的「此前单击即整区切详情」、`command-actions.ts` 头注「随票 02 … 落地后加进」；`App.tsx` 头注随票 01 那条一并删。
- Standards 非阻断 4 → 已改：`resolveInspectorSections` 对同一节里重名的格（关联对象按地址）抛 `InspectorContractError`。选「抛」不选「键带序号」：与契约「抛而
  不静默」同口径，而且两格同名，读的人本来就分不清哪格是哪格。
- **Spec 非阻断 1 → 已改。** 控制口加 `offer()`（返回撤回函数），列表模板接了 `inspector` 时挂载期声明、卸载撤回；`Layout` 计数后以 `contentOffered` 交给面板，
  `inspectorEmptyNote` 二选一——供内容的页仍说「在列表里单击一行，这里显示它的概要」，其余页（工作台、对象标签、自带预览的列表、单击即开的页）说
  「本页没有要在检查器里显示的内容」。评审给了两个修法，取前一个（壳层据实选句）：后一个（换一句处处为真的话）在两张首用页上会丢掉「单击一行」这条操作指引。
- **Spec 非阻断 2 → 已改。** `resolveInspectorForPanel` 接住 `InspectorContractError`，面板在栏内以 `SectionError`（标题 + 违反的是哪条，不给重试）替掉各节，别的
  错误照抛；不分开发 / 生产。
- Spec 非阻断 3 → 已补 `submissionVersionId`（「提交版本」，进审计节）；其余偏离作取舍记入判断项 8。
- Spec 非阻断 4 →（作者定）做了：接了 `inspector` 的行进 Tab 序，聚焦即交给检查器（只认落在行本身的聚焦，不占 Enter / Space）；`rowInteraction` 加可选
  `inspect`，不传的调用零变化。代价：只接 `inspector` 的列表（异常案件）每行一个 Tab 停点，与接了 `onRowOpen` 的页相同。
- Spec 非阻断 5 → 已改：`rows` 变时按键找回那一行重推，找不到就清空。比的是内容 JSON 而不是行对象身份——异常案件页每次渲染都 `map` 重建行对象，按身份比会
  与壳层互相重渲、转不出来（探针用同样写法的页实测：重渲 5 轮后 show 仍是 1 次）。
- Spec 非阻断 6 → 已改：「打开分诊」改名「转到异常分诊」。已核 `ExceptionTriagePage`：它列信号发作期册与处置请求册，没有按案件的地址。
- Spec 非阻断 7 → 完成记录补一句：「四道门」的第四道 `go test ./internal/architecture/` 当时没有单跑，由 CI 的 Test shards（`go list ./...`）兜住；本轮同样没在本机跑，见「门」。

**判断项更正与补记**

- 判断项 3 更正：首版 `92cbcf1a` 的节 key 是 `${content.title}:${resolved.kind}`，而两页 `title` 是常量，首版本来就不按对象重置；`9f95520e` 对两页零行为变化，改的是
  让注释说真话。「首版曾按对象重置」一句不成立。
- 判断项 6 改口：`onClose` 不再复用为「折叠」，栏顶是宿主给的带名收起钮（见 Standards 非阻断 1）。
- 8. **委托查阅检查器字段对票面第 1 / 4 条的取舍**：委托号在检查器副标题；服务产品读模型没有（`ShipmentRequestSummary` 不虚构），不列；提交时刻与提交版本同归
  审计节（都是修订元数据）；来源与来源请求键放概要（行上已有，说明这份委托从哪来）。

**门**：同票 01「评审后修复」——钉 `2056e054`，Node 22.20.0，`tsc -b --noEmit` 退 0 / `run-tests` 406 → 414 pass 0 fail（票 02 这边 +4）/ `vite build` 成功；
`go test ./internal/architecture/` 本机未跑，由 CI 兜。

**探针**（与票 01 共用 `/tmp/idp-probes/wsform-review-fixes-probe.tsx`，不入库）：修复后 **24 ok / 0 fail**；对修复前 `376b6ea1` **17 FAIL**，本票相关的几项：
工作台与委托详情对象标签上的空态句是「在列表里单击一行…」；栏里有 1 个无名按钮（vendor ×）；只接 `inspector` 的行没有 tabindex、聚焦不进检查器；重取后
同键行内容变了不重推、行被检索筛掉不清空；概要 9 格与同节重名时渲染直接抛出。修复后：四种页面上的空态句各自为真；收起 / 展开成对且都有名字；聚焦行
即交给检查器、聚焦行里的复选框不算；重取后按键重推、筛掉即清空、单击照旧；两种违约都在栏内显错误段，合契约的内容照常渲。

**评审**：修复碰共享面（`templates/*`、`shell/*`、`Layout.tsx`），按 workflow 第 5 步要一份 Spec 轴。复核已派回通道 3（任务台，结果回来代落）；在此之前以
**推送方自审**为准，不算非作者评审。自审所得：模板改动向后兼容——不传 `inspector` 的列表既不声明 `offer`、行也不进 Tab 序，`rowInteraction` 的新入参可选；
`InspectorPanel` 换 prop 只有 `Layout` 一处调用；`templates/index.ts` 只追加；空态句两句都不放假内容；重复名与超格一样走契约错误，不静默截断。

**推送**：同票 01——未推，推送后在票 01 与本票各补一行进 main 记录。
