# 02 右侧检查器：壳层右栏位 + `InspectorContent` 契约（五节）+ `ListPageTemplate` 单击进检查器 + 两张首用页渲染器

Category: enhancement
Status: draft——分两段：**模板段**（契约 + 首用渲染器 + `ListPageTemplate` 接口）不等任何票，可先派；**壳层段**（右栏装进 `Layout.tsx`）Blocked by 01
Blocked by: 01（仅壳层段）
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

## Comments
