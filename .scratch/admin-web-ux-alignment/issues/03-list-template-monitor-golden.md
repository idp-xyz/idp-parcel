# 03 `ListPageTemplate` 对齐 Monitor 黄金母版：面包屑、Filter Bar 完整结构位、密度、单击预览 / 双击开对象、surface 容器

Category: enhancement
Status: resolved · 已进 main（码 `082fafe0`，推送方注释笔 `d791bc26`，推送 tip `3fbf2ec2`；评审 ← 通道 2 两轴 0 阻断）
Blocked by: 无（密度开关由 01 提供，但 `useDensity` 无 Provider 时应有默认——本票自己兜底，不等 01）
地盘：`apps/admin-web/src/templates/ListPageTemplate.tsx`、`templates/types.ts`、`templates/demo.ts`、`pages/template-preview/*`（演示页跟着模板长）、
`pages/catalogue-view.ts` 如需加共用判读。**不逐页改列表页**：行为一律走可选 prop、不传则与原先同；形态按黄金标准 Rule 2 无条件长出（裁决 3，
评审 Spec 2 后改口——原句「默认行为与今天逐字节同」对形态不成立）。
出处：spec「缺口」表第三档；黄金标准「Page Header / Breadcrumb 黄金标准」「Filter Bar 黄金标准」（P0 最低可见结构、Rule 2）「Main Content 黄金标准」
（surface container、吃满空间、页脚稳定）「Table 黄金标准」（单击 / 双击、密度）「留白与空间分配」；手册「Table 规范」「Drill-down 模式」；
参照 idp-ui@53df1666 `apps/loms-web/src/loms/pages/OrderList.tsx`（22px 面包屑条、toolbar、`useDensity` 行高）与 `ShipmentMonitor.tsx`
（Filter Bar 的 Golden Template 注释：Search / Primary / View-Sort / Saved View / More Filters；Preview Pane；Esc 关预览）。

## 为什么

36 张页共用这一个模板，是本仓最划算的一处对齐：模板长一格，全站长一格。黄金标准最硬的一条是 Rule 2——「Filter Bar 必须标准化，即使功能未完整，
也要按完整模板布局」；我们今天只有搜索 + 调用方自带的下拉，排序 / 视图 / 保存视图 / 更多筛选没有位，各页于是各长各的（09 把排序做成了一个普通下拉）。
双击开对象是手册跨产品一致的交互（Monitor 单击预览、双击开工作区），我们只有单击。

## 要做的

1. **面包屑条**（22px，`Breadcrumb` 原语）：`区 › 页`——区名从 `navigationSections` 反查条目所在分区，页名 `pageTitleById`；模板新增可选 prop
   `breadcrumb?: { section: string; page: string }`，不传则模板自己按 `moduleId` 反查（新增可选 prop `moduleId`），都没有就不渲染这一条。
2. **Filter Bar 完整结构位**（黄金标准 9.2 顺序）：搜索区（已有）→ 主筛选（已有 `filters`）→ **排序**（新可选 prop `sort?: { options, value, onChange }`，
   不传则显禁用态「排序」按钮 + 悬停「本页尚未提供排序」）→ **视图模式**（表格固定，禁用态，悬停「本页只有表格视图」）→ **保存视图**
   （新可选 prop `savedViews?: { current, list, onSave, onSelect }`；不传则禁用态；02 落地后各页按需接）→ **更多筛选**（新可选 prop
   `moreFilters?: ReactNode`，不传则禁用态）→ 右端计数摘要（已有）。禁用位一律 `Tooltip` 说明 + `aria-disabled`——「留位不留假动作」（spec 红线）。
3. **密度**：`useDensity()` 读 `density`，行内边距 compact / comfortable 两档（照 loms-web `cellPy`）；无 Provider 时 `useDensity` 若抛错就
   `try / catch` 兜底 comfortable，头注写明这是等 01 的过渡。
4. **表格容器**：主表放进 `surface` 层容器（边框 + 底色取 `--idpxyz-sidebar` 一档，Light 下形成黄金标准四层里的 Layer 2），表格吃满剩余高度，
   分页固定底部（今天已是），空表体不留大片空白——`emptyRowsNote` 那一行居中且高度收紧。
5. **单击 / 双击**：新可选 prop `onRowOpen?: (row) => void`；有它时单击仍走 `onRowClick`（预览 / 选中），**双击**走 `onRowOpen`（开对象——
   调用方写 hash 二段路由），`Enter` 键等价双击、行有 `tabIndex` 与焦点态（手册「可访问性」表格键盘导航）；无 `onRowOpen` 时行为与今天同。
6. **`template-preview` 演示页**跟着模板补齐五个位与双击（合成 S，`templates/demo.ts` 的样例数据照旧不承载业务语义）。

## 不做

- 不做 Board / Timeline 视图；不做批量选择与批量动作栏（本仓没有批量命令）；不做右键菜单；不做侧边 Preview Pane（抽屉已是预览，形态归 04 一并裁）。
- 不改任何业务页的调用（新 prop 全可选）；不动 `DetailPageTemplate` / `ReviewFlowTemplate`。

## 完成判据

- 三道门绿；既有 `run-tests` 零改动仍绿（模板默认行为不变的证据）。
- `templates/` 下新纯逻辑（面包屑反查、密度兜底、`?view=` 解析若做）带 node:test。
- `template-preview` 页在 `vite build` 产物里能看到五个 Filter Bar 位（`renderToStaticMarkup` 断言按钮文案与 `aria-disabled`）。
- 浏览器验收做不到如实写「未验」。

## 裁决

1. 禁用位用**按钮 + 悬停说明**而不是灰色占位方块：黄金标准要的是「结构位置」，可访问的说明比装饰方块诚实。
2. 双击开对象在**当前上下文**打开（hash），不开标签页：本仓 console 形态无 Workbench Tabs（spec「不做」）。
3. **默认形态按 Rule 2 改了，改了哪几样在此列清**（推送方进 main 时补，据评审 Spec 2 / Spec 3）：不传任何新 prop 的列表页，(a) Filter Bar 多出
   排序 / 视图 / 保存视图 / 更多筛选四个 `aria-disabled` 位；(b) 主表进 surface 容器、吸顶表头底色盖成容器同色；(c) 行内边距随密度档，无 Provider
   时随库回退 compact（`py-1`，原语默认 `py-2`）；(d) 过滤条由换行改横向滚（`overflow-x-auto` + 搜索框 `min-w-[160px]` + 计数 `shrink-0`）。
   行为（回调、Tab 序、键盘）不传则同原先。**密度默认档归 01**：全站默认由 01 挂的 `DensityProvider` 初值定（01 票面第 3 条 = comfortable），
   本票的 compact 回退只在 Provider 缺席时生效，是过渡；01 进 main 后列表行距从 `py-1` 变 `py-2.5` 是预期内的一次性变化，两票都不为迁就它改默认。

## Comments

### 认领（2026-09-20 12:4x）

通道 3，分支 `mcp3-ux03` 基 main `3245faed`，地盘 `templates/ListPageTemplate.tsx` / `templates/types.ts` / `templates/demo.ts` /
`pages/template-preview/*`；`pages/catalogue-view.ts` 只加不改（06 同时在动它的 loading 态）。不碰 `Layout.tsx` / `App.tsx` / `index.css`（01）、
`DetailPageTemplate.tsx`（04）、`state-slot.tsx` / `components/states`（06）、`domain/status.tsx`（05）；36 张列表页零改动。

### 完成记录（通道 3，分支 `mcp3-ux03` tip `50b1a8c6`，基 main `3245faed`；前四笔 12:5x–13:0x 由上一个会话做，后三笔 15:3x–15:4x 由 15:15 重启后的会话接做）

**六条落点**（每条一笔，提交信带三道门数字）

1. ✅ 面包屑条 `c6b4777d`：`ListPageTemplate` 新可选 prop `breadcrumb` / `moduleId`，22px 条 + `Breadcrumb` 原语，`templates/breadcrumb.ts` 的
   `resolveBreadcrumb` 按 `navigationSections` 反查分区、页名取 `pageTitleById`（缺则退条目 label），查无出处返回 `null` 不渲染。
2. ✅ Filter Bar 完整结构位 `fb147357`：黄金标准 9.2 顺序 搜索 → 主筛选 → 排序 → 视图 → 保存视图 → 更多筛选 → 右端计数；新可选 prop `sort` /
   `savedViews` / `moreFilters`；`templates/list-page-structure.ts` 的 `filterBarSlots` 算四个位的启用与说明，不传的位渲染 `aria-disabled` 按钮 +
   `Tooltip` 说明（裁决 1）；视图模式位永远禁用（只有表格视图）。用 `aria-disabled` 而非 `disabled`：`Button` 原语的 `disabled` 带
   `pointer-events-none`，悬停说明出不来。
3. ✅ 密度 `e7f17306`：`useDensity()` 读档，`densityRowPadding` 映射 compact `py-1` / comfortable `py-2.5`（照 loms-web `cellPy`）。**票面「若抛错
   try / catch 兜底 comfortable」的前提实测不成立**：`@idpxyz/ui-theme-runtime` 0.1.23 的 `useDensity` 无 Provider 时不抛错、回 `createContext`
   默认值 compact，于是不加 try / catch；默认档 compact 与 01 挂 `DensityProvider` 后的初值一致，也是黄金标准 11.5 对 Monitor 页的推荐。
4. ✅ 表格容器 `b83efcc2`：主表进 surface 层容器——外圈 `p-2` + `rounded-md border border-idpxyz-border bg-idpxyz-sidebar`（照 loms-web
   `ShipmentMonitor` 的表格容器；Light 下页底 editor 为 Layer 1、容器为 Layer 2）；容器吃满剩余高度、滚动在容器内，吸顶表头原语自带的
   `bg-idpxyz-editor` 经 `cn` 同族让位盖成 `bg-idpxyz-sidebar`；分页条留容器外页底不随表体滚（今天已是）；`emptyRowsNote` 行
   `hover:bg-transparent` + `py-1.5` 居中收紧。纯布局，无新纯逻辑。
5. ✅ 单击 / 双击 `7d73a532`：新可选 prop `onRowOpen`。有它时单击仍走 `onRowClick`、双击走 `onRowOpen`、行 `tabIndex=0` + `focus-visible`
   焦点态（outline 而非 ring：ring 是 box-shadow，浏览器对 `<tr>` 不保证画）、行聚焦后 Enter 等价双击——只认 `event.target === event.currentTarget`
   的 Enter，单元格里按钮 / 链接自己吃的 Enter 不算开行。无 `onRowOpen` 时 `className` / `tabIndex` / 事件与今天同。
   `list-page-structure.ts` 的 `rowInteraction`（tabIndex 与可点性）与 `rowKeyOpens`（只认 Enter；Space 是滚动容器的翻页键、方向键留给浏览器）
   纯模型带 node:test。
6. ✅ `template-preview` 演示页 `50b1a8c6`：`moduleId` 面包屑、主筛选（状态 `FilterChip`）、排序 / 保存视图 / 更多筛选三位**全有真动作**（真排序、
   真筛选、保存视图为本页内存里的筛选态快照，切一个套回来、存一个快照进去）、结构位「已接入 / 未接入（禁用态）」两档切换验收裁决 1 的两种长相、
   `emptyRowsNote`、单击预览 + 双击 / Enter 开对象（toast，文案写明真实页面在此写 hash 二段路由——裁决 2）。`demo.ts` 加 `demoListSortOptions` /
   `demoListSavedViews`（合成 S，排序键与视图快照只对演示行起作用，不对应任何读口参数）。

**完成判据逐条**

- 三道门 ✅ tip `50b1a8c6`：`tsc -b --noEmit` 0 / `run-tests` **299**（main `3245faed` 290 + 9）/ `vite build` 0。既有用例零改动：
  `git diff --numstat 3245faed 50b1a8c6 -- '*.test.ts'` 只有 `templates/breadcrumb.test.ts`（+42/−0，新）与 `templates/list-page-structure.test.ts`
  （+77/−0，新）。36 张列表页零改动：`git diff --stat 3245faed 50b1a8c6 -- apps/admin-web/src/pages ':!apps/admin-web/src/pages/template-preview'` 为空。
  地盘纪律：`templates/types.ts` 未动；`templates/index.ts` +15/−0 只追加。
- `templates/` 新纯逻辑带 node:test ✅ `resolveBreadcrumb`（三条：平铺 / 嵌套命中、查无出处 `null`、真实导航表逐条一致）、`filterBarSlots`（两条：
  四位顺序与全禁用各带说明、接了的启用 / 视图永远禁用 / 启用与禁用同词）、`densityRowPadding`（一条：两档不同且都是 `py-` 族）、`rowInteraction`
  （两条：无 `onRowOpen` 不可聚焦、有则 `tabIndex` 0 且可点）、`rowKeyOpens`（一条：只认 Enter）。`?view=` 解析未做——视图模式位永远禁用，没有可解析的东西。
- `template-preview` 五位 `renderToStaticMarkup` 断言 ⚠️ **一次性实测、组件层未钉**（照 05 / 06 的结论：`.test.ts` 引不到 `import` 了 `@idpxyz/*` 的模块）：
  一次性 esbuild 束（`node_modules/.pnpm/esbuild@0.25.12/…/bin/esbuild <probe.tsx> --bundle --platform=node --format=cjs --jsx=automatic --loader:.css=empty`，
  `NODE_PATH` 指到本包 `node_modules`；`ThemeProvider` 在 SSR 下碰 `localStorage` 抛错，探针只包 `ToastProvider`；源与产物不入库）在 tip `50b1a8c6` 上
  **16 ok / 0 fail**：演示页默认档 12 条——搜索 placeholder、`<select aria-label="排序">` 三项文案、按钮「视图：表格」`aria-disabled="true"`、
  `<select aria-label="保存视图">` 两个预置视图 + 按钮「保存当前视图」、按钮「更多筛选」`aria-expanded="false"`、主筛选 chip「全部状态」+ 演示状态词、
  22px 面包屑条内「演示」+「模板预览（合成 S）」、容器类 `rounded-md border border-idpxyz-border bg-idpxyz-sidebar`、`<th>` 含 `bg-idpxyz-sidebar`、
  5 个数据行 `tabindex="0"`、每个可聚焦行带 `focus-visible:outline` 与 `cursor-pointer`、计数「共 5 条（合成）」；裸模板 3 条（不传 sort / savedViews /
  moreFilters / onRowOpen）——恰四个 `aria-disabled` 按钮「排序 / 视图：表格 / 保存视图 / 更多筛选」、行零 `tabindex` 且无焦点态类但 `cursor-pointer` 照旧、
  无面包屑条；`emptyRowsNote` 1 条——`<tr … hover:bg-transparent><td … py-1.5 text-center …>`。
  `vite build` 产物（`dist/assets/*.js`）grep：四句禁用说明、「视图：表格」、「保存当前视图」、「全部申报（演示）」、「双击开对象（演示）」、
  `focus-visible:outline-idpxyz-accent` 各 1 处，容器类串 3 处。
- 浏览器验收 **未验**（本机无 gk.idp.xyz 会话）。行焦点态在真实浏览器里 outline 是否落在 `<tr>` 上、Radix `Tooltip` 悬停说明是否出、双击时两次 click 先于 dblclick
  触发 `onRowClick` 两次（与 loms-web 同形）——三件都只在浏览器里看得到，交评审 / 首个真实接线页验。

**判断项**（不在票面六条里、做的时候拿的主意，评审若不认可各自一行改回）

- 第 3 条不加 try / catch（见落点 3）；默认档 compact 而非票面写的 comfortable——两处都是实测压过票面假设。
- 第 4 条分页条留在容器**外**（照 loms-web `ShipmentMonitor`，票面「分页固定底部（今天已是）」于是一字不改）；吸顶表头盖成容器同色是本票加的，票面没写——
  不盖会在 Light 下出一条白色异色带。
- 第 5 条键盘只认 Enter、不给 Space 等价单击：票面只要求 Enter 等价双击；Space 在滚动容器里是翻页键，行吞它会坏滚动。
- 第 6 条演示页三位**全接真动作**而不是只摆位：接了的位若无动作就是演示页自己违反「留位不留假动作」；「未接入」档另给，两种长相都能验。
- `spec.md` 子票表 03 那一行仍是 `ready-for-agent`，**没改**——它是六票共用的索引文件，留给推送方在进 main 簿记时翻，免得与 04 / 05 / 06 的同类改动撞行。

### 评审 ← 通道 2 · 钉 `c4537d01`（码 `50b1a8c6`，基线 `3245faed`）· 16:0x（推送方自任务台 `task-18fac5ae` 代落原文）

只读，隔离树已拆，未碰作者树，未跑 pnpm / tsc / Go。

**Standards** — 阻断：无。非阻断：
1. `ListPageTemplate.tsx` 头注「一切新位都走可选 prop，不传时的默认行为就是各页今天的行为」与代码相反：不传任何新 prop 时四个 `aria-disabled` 位
   （`filterBarSlots` 无条件渲染）、surface 容器、`<th>` 的 `bg-idpxyz-sidebar`、每格 `py-1`（原语默认 `py-2`）、`FilterBar` 的 `overflow-x-auto` /
   `min-w-[160px]` / `shrink-0` 全是无条件的。这是黄金标准 Rule 2 压过票面「逐字节同」的正当裁法，但注释要说真话（AGENTS「写代码注释」）。
2. 跨文件计数「36 张」：`ListPageTemplate.tsx` 头注与 `list-page-structure.ts` `RowInteraction.tabIndex` 注释；同处「与今天逐字节同」是相对时点的变更说明。
3. `templates/index.ts` 新导出 `densityRowPadding` / `filterBarSlots` / `rowInteraction` / `rowKeyOpens` / `resolveBreadcrumb` 及类型：全树零外部消费者
   （Speculative Generality，判断项）。
4. `TemplatePreviewPage.tsx` `demoSelectClass` 与 `ListPageTemplate.tsx` `filterControlClass` 字串几乎逐字相同；作者注释已写明不共享的取舍，记为已裁的
   Duplicated Code，不要求改。
无发现（实核）：注释全中文、引上游用小节号 + `idp-ui@53df1666`；`@idpxyz/ui-theme-runtime` 0.1.23 dist 里 `useDensity` 无 Provider 不抛、
`DensityContext` 默认 compact、`DensityProvider` 初值 `useState("compact")`——作者两句实测都对；`densityRowPadding` 与 `<th>` 盖色经 `cn` / twMerge 让位成立；
`Tooltip` 原语自带 Provider；`TableRow` 透传 `tabIndex` / `onDoubleClick` / `onKeyDown`；无 `onRowOpen` 时行属性与 `3245faed` 同；第 5 条只认 Enter 有判据。

**Spec** — 阻断：无。非阻断：
1. 第 3 条票面写兜底 comfortable，代码默认 compact：全站列表行距从原语 `py-2` 无条件收紧到 `py-1`；档位选 compact 靠的是黄金标准 11.5 而非实测；
   01 若按手册把无存储默认定成 comfortable，落地时全站会再翻一次——要推送方 / 01 作者定一处。
2. 完成判据「既有 run-tests 零改动仍绿（模板默认行为不变的证据）」后半句证据为空——run-tests 引不到模板，绿只证明纯函数没坏；默认形态实际变了，
   票面没有一处写成「默认形态按 Rule 2 改了、改了哪几样」的裁决。建议补裁决 3 并改地盘句。
3. 票面没要的：`FilterBar className="overflow-x-auto"` + 搜索框 `min-w-[160px]` + 计数 `shrink-0`——过滤条由换行改横向滚，判断项没提。
无发现：第 1 / 2 / 4 / 5 / 6 条逐条对上（禁用位 `aria-disabled` + `Tooltip`、四句说明逐字同、`DisabledSlot` 无假动作；容器类与 `emptyRowsNote` 收紧；
单击 / 双击 / Enter 判定；演示页「真动作」不越界、`demo.ts` 全标合成 S）；既有用例零改动；九条 node:test 齐；renderToStaticMarkup 如实标「一次性实测、
组件层未钉」；判断项五条无失真。

**Standards 0 / 4 · Spec 0 / 3** → 无阻断，可重放。

### 处置（推送方 · 通道 1 · 16:0x–17:3x）

- Standards 1 / 2 → 推送方代落 `d791bc26`（只改注释：头注分开「形态按 Rule 2 无条件长、行为按 prop 才有」；去「36 张」计数与「与今天逐字节同」时点句）。
- Standards 3 / 4 → 记，不改（判断项）。
- Spec 1 → 裁决 3：密度默认档归 01（comfortable），本票 compact 回退是过渡；已随 01 接续派单写给通道 4。
- Spec 2 / 3 → 裁决 3 列清四样默认形态变化（含过滤条横滚）+ 地盘句改口（本簿记笔）。

### 进 main 记录（推送方 · 通道 1）

- 重放：`idp-land-ux03` 上 cherry-pick 到 main `7d29af39`，八笔 SHA 对照 `b9cd0a24→fc550091` / `c6b4777d→fefd44cd` / `fb147357→1e80c3b1` / `e7f17306→6a71e3b8` /
  `b83efcc2→201ab817` / `7d73a532→da0f666a` / `50b1a8c6→082fafe0` / `c4537d01→760402c6`；八件与作者 tip 逐字节同，`templates/index.ts` 只差 06 的 `LoadingShape` 一行。
  上一任通道 1 于 16:04 做完重放与 `d791bc26`，未推、未簿记即止；本任 17:1x 接手，按任务台里的评审记录续做。
- 04 第 1–5 条叠在其上（见票 04），推送 tip `3fbf2ec2`；门禁在该 tip 上实跑：`tsc -b --noEmit` 0 / `run-tests` **313**（298 + 本票 9 + 04 六）/ `vite build` 0 /
  `gofmt -l` 空 / `go build` 0 / `go vet` 0 / 清点重生成零差 / `go test ./internal/architecture/ -count=1` ok / 带 DSN 全量见 tasks.md 本节。
- 浏览器未验沿作者所报；行焦点态 outline、Radix Tooltip 悬停、双击前两次 click 三件留首个真实接线页验。
