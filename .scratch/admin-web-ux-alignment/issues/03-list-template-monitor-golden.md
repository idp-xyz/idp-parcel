# 03 `ListPageTemplate` 对齐 Monitor 黄金母版：面包屑、Filter Bar 完整结构位、密度、单击预览 / 双击开对象、surface 容器

Category: enhancement
Status: ready-for-agent
Blocked by: 无（密度开关由 01 提供，但 `useDensity` 无 Provider 时应有默认——本票自己兜底，不等 01）
地盘：`apps/admin-web/src/templates/ListPageTemplate.tsx`、`templates/types.ts`、`templates/demo.ts`、`pages/template-preview/*`（演示页跟着模板长）、
`pages/catalogue-view.ts` 如需加共用判读。**不逐页改 36 张列表页**：一切新能力走可选 prop，默认行为与今天逐字节同。
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

## Comments
