# 10 对象工作区的签统一配小图标（全站，不只公司主体）

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 问「tab 需要添加小的对应图标吗」；协调方答合适但要全站统一，用户裁「做吧」。参照：WMS「ASN 管理」详情栏的签（`scratchpad/ux-le2/wms-asn-reference.png`）。

## 要做的

1. **图标表一处定义**：新 `templates/workspace-tab-icons.tsx`，`workspaceTabIcons: Record<WorkspaceTabId, LucideIcon>`——`Record` 让每个签 id 必有图标，少一个 tsc 就报。取 lucide 里不会误读的：summary → `FileText`，contacts → `Contact`，profile-versions → `Layers`，identity-revisions → `ClipboardList`，timeline → `History`，related → `Link2`，exceptions → `AlertTriangle`，documents → `Paperclip`，audit → `ScrollText`（有更贴切的可换，票面写明）。另导出一个 `WorkspaceTabLabel({ id, children? })`：图标 14px、`text-idpxyz-breadcrumb` 弱色、`aria-hidden`，后接词（词仍取 `workspaceTabLabels`），计数徽章由调用方照旧接在后面。`workspace-tabs.ts` 不动（它要保持不引 React 让 run-tests 的 CommonJS 发射能加载）。
2. **对象页**：`templates/DetailPageTemplate.tsx` 渲染签的地方改用 `WorkspaceTabLabel`，全站对象页一起生效。
3. **页内详情栏**：签 id 与工作区签同名的详情栏也用同一张表——至少 `pages/party/LegalEntityDetailPanel.tsx`（概要 / 联系人 / 资料版本 / 登记记录 / 关联）、`BusinessPartyDetailPanel.tsx`（概要 / 时间线 …）、`VersionDetailDrawer.tsx`（若它的签对得上 id）。其它页顶的列表签（往来单位 / 合作关系那种页级切换）不是对象工作区的签，不配图标。
4. 选中态图标跟随词的强调色（`data-[state=active]`），非选中弱色；暗色主题下核一眼对比。
5. 三道门；浏览器核：公司主体对象页与详情栏、往来单位对象页与详情栏、一张运单对象页（看模板全站生效）各截一张，1440 宽。
6. 共享面（`templates/*`）改动要一份 Spec 轴非作者评审——协调方派。

## 不做

- 不改签的词与顺序；不给页级列表签加图标；不动 `components/*`、`shell/*`。

## 完成判据

- 三道门退 0；对象页与两处详情栏的签都是「小图标 + 词 (+ 计数)」，选中态图标变色；运单对象页的签同样带图标。
