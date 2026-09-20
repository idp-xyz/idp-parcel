# 04 `ListPageTemplate` 多选 + 批量动作栏：选择模型、表头全选、「已选 N 项」栏、默认动作「导出所选 CSV」、页面级动作槽

Category: enhancement
Status: ready-for-agent
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

## Comments
