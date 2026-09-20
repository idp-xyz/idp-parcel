# 04 `ListPageTemplate` 多选 + 批量动作栏：选择模型、表头全选、「已选 N 项」栏、默认动作「导出所选 CSV」、页面级动作槽

Category: enhancement
Status: resolved——码两笔 tip `59c51d25`（分支 `mcp3-wsform04`，基 `11e6111a`；通道 3 20:1x 推完并报过半后会话 crash，完成记录由推送方按 git 与作者进度报代落，见下）；进 main 记录见 Comments。此前 in-progress（20:0x 通道 3 认领 task-b3f07954）、ready-for-agent
Blocked by: 无
地盘：`apps/admin-web/src/templates/ListPageTemplate.tsx`（加可选 `selection` / `bulkActions` 两 prop）、`templates/list-page-structure.ts`（选择态纯逻辑）+ 新 `templates/list-selection.ts`
（选择集 / 全选本页 / CSV 生成纯逻辑 + node:test）、`templates/index.ts`（只追加导出）、首用页两张：`pages/shipment-request/ShipmentRequestListPage.tsx`、
`pages/visibility/ExceptionCasesPage.tsx`（只传 prop，不改页内逻辑）。
出处：spec「缺口」表第四档；蓝图 10.5「Multi-select rows: reveal Bulk Action Bar」、10.7 批量动作；参照 `idp-ui@6751fb2` `apps/myshop-web/src/pages/OrdersList.tsx`
（`selectedIds` Set + 表头复选全选本页 + 「已选 N 项」栏出现在 Filter Bar 之下、表之上；行内复选 `stopPropagation` 不触发行点）。

## 为什么

蓝图母版 B 的列表是「处理队列」不是「看表」：多选后一条动作栏让人对一批对象做一件事。我们的列表页今天一行一动作。批量动作本身大多要命令端点（暂挂 / 分配 /
打标签——本仓没有这些语义），但**选择模型与动作栏是形态，端点是内容**：形态先立，默认只放本机能诚实完成的动作（把选中行导出为 CSV），页面级动作由调用方按端点有无传入。

## 要做的

1. **`list-selection.ts` 纯逻辑**：`toggleSelected(set, key)`、`toggleAllOnPage(set, pageKeys)`（本页全选 / 全不选，跨页选中保留）、`selectedOnPage(set, pageKeys)`
   （表头复选三态：无 / 部分 / 全）、`clearSelection()`；`rowsToCsv(rows, columns, cellText)`——列取 `ListColumn.header` 的文本、格取调用方给的 `cellText(row, column)`
   （`render` 出的是 ReactNode，CSV 不能从节点抠字，所以要调用方给一个取字函数；没给的列跳过），RFC 4180 转义（引号 / 逗号 / 换行）、UTF-8 BOM 让 Excel 认中文。node:test 钉边界。
2. **`ListPageTemplate` 两个可选 prop**：`selection?: { selected: ReadonlySet<string>; onChange: (next: Set<string>) => void }`——有它才出复选列（表头 + 每行），
   行内复选点击 `stopPropagation`（蓝图 10.5：选择不等于预览，不触发 `onRowClick` / `onRowOpen`）；`bulkActions?: { csv?: { fileName: string; cellText: (row, column) => string };
   extra?: ReactNode }`——`selected.size > 0` 时在 Filter Bar 之下、表之上出现动作栏：「已选 N 项」+「导出所选（CSV）」（有 `csv` 才出）+ `extra` + 「取消选择」；
   `selected.size === 0` 时不渲染（不留空条）。密度两档下栏高随 `densityRowPadding`。
3. **`list-page-structure.ts`**：`rowInteraction` 加一格「复选列点击不算行交互」的判读（如果它在那里判），或在 `list-selection.ts` 里另写并注明为何不合进去。
4. **首用两页**只加 `selection` state + `bulkActions.csv`（`cellText` 取行里已有的字段字面量），不加 `extra`——本仓今天没有能对一批委托 / 案件做的命令端点，
   `extra` 留给第一个有端点的页；票面写明。
5. 头注写清：选择集按 `rowKey` 记、翻页保留、筛选变更**不**自动清（用户可能在跨筛选攒一批），换模块（组件卸载）即丢——它是 UI 瞬态，不进 saved-views。

## 不做

- 不做「全选所有页」（隔离读口 `isolatedReadLimit = 200` 且分页在服务端，全量选中是假的）；不做行内快捷动作（蓝图 10.5 hover quick actions，本仓无端点）。
- 不改其余列表页——两张首用之外的页不传 prop，行为零变化。
- 不碰 `DetailPageTemplate` / `ReviewFlowTemplate` / `state-slot.tsx`。

## 完成判据

- 四道门绿；`list-selection.test.ts`：切换 / 本页全选三态 / 跨页保留 / CSV 转义（含引号、逗号、换行、中文 BOM）各至少一条正向一条边界。
- `ListPageTemplate` 不传 `selection` 时 DOM 无复选列（一次性 esbuild 束断言 `input[type=checkbox]` 为 0）；传了且选中 2 行时动作栏文案含「已选 2 项」。
- 两张首用页 `tsc` 过、`vite build` 产物含「导出所选」字面量；浏览器验收做不到如实写「未验」。
- 票面判断项写清：为什么 CSV 取字要调用方给函数、为什么筛选变更不清选择集。

## 完成记录（推送方代落，2026-09-20 20:3x；作者通道 3，码 tip `59c51d25`，基 `11e6111a`）

作者两笔码推完、四道门绿、一次性探针 21 / 21 跑完后会话 crash（用户 20:2x 报，附终端截图：探针 `ok 21 / fail 0`、`probe exit=0`），票面未写。下表按 `git diff 11e6111a..59c51d25`
与作者 20:1x 过半报代落，不取记忆。

| 笔 | 条 | 落点 |
|---|---|---|
| `8b2d15e7` | 第 1 条 | 新 `templates/list-selection.ts`：`toggleSelected` / `toggleAllOnPage`（本页全选 / 退掉本页，别页保留）/ `selectedOnPage`（`PageSelectionState` 三态只看本页）/ `clearSelection` / `retainSelectedRows`（键 → 行的记忆：可见的刷新、翻走的保留、取消的删、没见过的不造）/ `selectedRows` / `rowsToCsv`（RFC 4180：CRLF 分行、含逗号 / 双引号 / 换行的字段加引号且双引号成对、UTF-8 BOM 开头、整列无文本的列跳过、零行只出表头、非文本表头退回列 id）；`CsvColumn` / `CsvCellText` 不引 `ListColumn` 本体，纯逻辑文件不反向依赖 `.tsx`。头注写明选择集生命周期（按 rowKey、翻页保留、筛选变更不清、换模块即丢、不进 saved-views）与「复选列点击不算行交互」为何不并进 `rowInteraction`。`list-selection.test.ts` node:test 9 条 |
| `59c51d25` | 第 2 / 4 / 5 条 | `ListPageTemplate.tsx` 加可选 `selection: ListSelectionProps` / `bulkActions: ListBulkActionsProps<Row>`（`csv?: ListCsvExportProps<Row>` + `extra?`）：有 `selection` 才出复选列——表头原生 `input` 三态（`indeterminate` 在提交后对节点设、`aria-label="选择本页全部"`）、每行 `aria-label="选择 <key>"`，复选格 `stopRowEvent` 拦 click / dblclick 不触发 `onRowClick` / `onRowOpen`；`selected.size > 0` 时在 Filter Bar 之下、表之上出 `role="toolbar" aria-label="批量动作"` 栏：「已选 N 项」+「导出所选（CSV）」（有 `csv` 才出；`Blob` → 对象 URL → 隐藏 `<a download>`，URL 延后释放）+ `extra` + 「取消选择」，栏高随 `densityRowPadding`；选中行记忆放 `ref`（只在导出那一刻读）；栏不随 `viewState` 收起。`templates/index.ts` 只追加导出。首用 `ShipmentRequestListPage`（`csvCellText` 按列 id 取行上已有字面量，状态取模型状态码不取徽章词）与 `ExceptionCasesPage`（`row.values[column.id] ?? ''`），两页只加 `checked` state + 两个 prop，**不传 `extra`**（本仓今天没有能对一批委托 / 案件做的命令端点） |

第 3 条：未改 `list-page-structure.ts`——「复选列点击不算行交互」落在复选格的 `stopPropagation`，没有可供纯函数计算的输入，`list-selection.ts` 头注写了理由（票面第 3 条的「或另写并注明」那一支）。

**完成判据逐条**

- ✅ 四道门：作者在 `59c51d25` 上 tsc 0 / run-tests **333**（324 + 9）/ vite 0 产物含「导出所选」/ `go test ./internal/architecture/` ok；推送方重放到 `d2f11618`（含 02 的 343）后 tsc 0 / run-tests **352** / vite 0 / architecture ok。
- ✅ `list-selection.test.ts` 9 条：切换（不改入参）/ 本页全选三态与别页保留 / 三态只按本页判 / 清空 / 记忆四情形 / 按选中先后取行 / CSV BOM + 表头 + 整列跳过 + CRLF / 三种字段加引号 / 部分行空、零行只表头、非文本表头退列 id——各有正向与边界。
- ◑ 组件层：**一次性实测、组件层未钉**（作者 esbuild 探针 21 ok / 0 fail，含「不传 `selection` 时 checkbox = 0」「选 2 行含『已选 2 项』」；源与产物未入库，推送方只见截图，未重跑）。
- ✅ 两页 `tsc` 过、`vite build` 产物含「导出所选」字面量（推送方在重放 tip 上 grep 复现）。
- ❌ 浏览器**未验**：复选点击不触发行点 / 双击、动作栏出现与消失、CSV 下载与 Excel 打开中文、密度两档栏高、表头 `indeterminate` 视觉。

**判断项**（推送方按代码与头注代写）

1. **CSV 取字要调用方给函数**：`ListColumn.render` 出的是 ReactNode，从节点里抠字既不可靠（徽章、`<time>`、图标）也会把屏幕上的留白符号（`—`）当成数据；调用方按列 id 给字面量，状态列给模型状态码而不是徽章词——码给机器读，词给人读，两者是同一事实的两种呈现。
2. **筛选变更不清选择集**：用户可能跨筛选攒一批再一起导出；选中集是对一批具体对象的临时圈定，与保存视图存的「筛选态」保质期差几个量级，所以不进 saved-views、换模块即丢。
3. **不做「全选所有页」**：隔离读口 `isolatedReadLimit` 有上限且分页在服务端，「全部」是假的。
4. **原生 `input` 而不是 ui-primitives `Checkbox`**：表格一列几十个复选，原生控件自带 `indeterminate` 三态且不多一层 Radix button；`indeterminate` 是 DOM 属性不是 HTML 特性，React 声明不了，提交后对节点设。
5. **选中行记忆放 `ref` 不放 state**：只在导出那一刻被读，不驱动渲染；翻走的行模板手里已没有，可见时记下来，「已选 N 项」的 N 仍按选中集报——两个数不同时以选中集为准，不补假行。

## Comments
