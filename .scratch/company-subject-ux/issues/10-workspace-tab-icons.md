# 10 对象工作区的签统一配小图标（全站，不只公司主体）

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
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

## 完成记录（2026-11-03，分支 `claude-csux-10`）

### 落点

- 第 1 条：新 `apps/admin-web/src/templates/workspace-tab-icons.tsx`——`workspaceTabIcons: Record<WorkspaceTabId, LucideIcon>`，九个图标照票面所列，一个没换（summary `FileText`、contacts `Contact`、profile-versions `Layers`、identity-revisions `ClipboardList`、timeline `History`、related `Link2`、exceptions `AlertTriangle`、documents `Paperclip`、audit `ScrollText`）；`WorkspaceTabLabel({ id, children? })` 渲 14px `text-idpxyz-breadcrumb` 图标（`aria-hidden`）后接词，词默认取 `workspaceTabLabels`，`children` 是词的覆盖口（给签词另有口径的详情栏用），计数徽章由调用方接在它后面。`templates/index.ts` 导出两者。`workspace-tabs.ts` 一字未动。另加 `workspace-tab-icons.test.ts` 钉「一签一图、互不重复」——`Record` 只替 tsc 守「每签有图」，不重复类型说不出。提交 `eaee993cd`。
- 第 2 条：`templates/DetailPageTemplate.tsx` 渲签处 `{tab.label}` → `<WorkspaceTabLabel id={tab.id}>{tab.label}</WorkspaceTabLabel>`，词仍取归并结果；计数照旧。提交 `b1fca5795`。
- 第 3 条：`pages/party/LegalEntityDetailPanel.tsx` 概要 / 联系人 / 资料版本 / 登记记录 / 关联 直接按表取词与图标（Radix value 仍是栏自己的 `PanelTab`，它存在视图状态里）；`BusinessPartyDetailPanel.tsx`「全景」借 `summary`、「变更记录」借 `timeline`（对象页 `BusinessPartyDetailPage` 把同一条修订链放在时间线签），词经 `children` 保留，原先各自挑的 `Network` / `IdCard` 退掉；`VersionDetailDrawer.tsx`「详情」借 `summary`、「版本历史」借 `profile-versions`（同是对象的版本链）。页级列表签未碰。提交 `51531f6e5`。
- 第 4 条：选中态不另定强调色，图标 `group-data-[state=active]:text-current` 跟随词色——`TabsTrigger` 原语自带 `group`，`data-state` 挂在它身上；模板签选中是 textBright、公司主体栏是 accent，图标各随其词。

### 判据逐条

- ✅ 三道门退 0：每笔提交前各跑一遍，`tsc -b --noEmit` 0、`run-tests` 1164 通过 0 失败（含新测试）、`vite build` 成功。
- ✅ 对象页与两处详情栏的签都是「小图标 + 词 (+ 计数)」：playwright 对每个 `role=tab` 取证，`svg` 皆在、`width=14`，词与计数未变。截图（1440×900）：`legal-entity-object.png`、`legal-entity-object-contacts.png`、`legal-entity-list-panel.png`、`business-party-object.png`、`business-party-list-panel.png`。
- ✅ 选中态图标变色：选中签 `iconColor === textColor`（对象页 `rgb(15,45,64)`、公司主体栏 `rgb(10,110,209)`），非选中 `rgb(83,107,130)`；切到「联系人」后随之换签。
- ✅ 运单对象页的签带图标：运单列表第一行 → 详情栏「打开详情」→ `#/shipment-request-inquiry/EBAIYXVMVZQJ76X4P5AY6AKQ3E`，概要签带图标（`shipment-object.png`）。
- ✅ 版本抽屉：客户合同第一行开抽屉，详情 / 版本历史带图标（`version-drawer.png`）。
- ◑ 暗色主题：`legal-entity-object-dark.png`、`legal-entity-object-contacts-dark.png`、`legal-entity-list-panel-dark.png`——选中态仍与词同色（对象页 `rgb(204,204,204)`、栏 `rgb(0,122,204)`），非选中图标 breadcrumb `rgb(169,169,169)` 比模板签词的 textMuted `rgb(133,133,133)` 亮一档（栏上两者同为 169）；肉眼看齐整、可辨，照票面的 breadcrumb 不改，要图标与词完全同色就把非选中也改成 `text-current`，留给评审裁。
- ✅ 核验全程零写请求（脚本断言）。
- ⏳ 第 6 条共享面评审：待协调方派。

截图与脚本在会话 scratchpad `csux10/`（`shots.js`，登录与 OIDC 回调改写抄 rd-ops）。

### 未验

- 异常 / 文档 / 审计三签眼下没有带数据的业务页渲它们，只经图标表与测试覆盖；`template-preview` 的工作区演示页走同一模板，未截。
- 核验跑动中浏览器控制台出现两条 `Failed to load resource: 404`（只读 GET，图标是内联 SVG 不走网络，与本票代码路径无关），未追。

## 评审与进 main 记录（2026-11-03，协调方）

- 共享面 Spec 轴隔离评审钉 `faafb578d`：阻断为零。非阻断三条：概要用 `FileText` 与同页主动作「更新公司资料」及导航「供应商协议」同图——协调方改成 `AlignLeft`；暗色下非选中图标 breadcrumb 比词 textMuted 亮一档是色板漂移——改成 `text-current` 与词同色；`VersionDetailDrawer` 借 `profile-versions` 只借图标、词未漂——接受。
- main `8bb21e3e7` 上 `git merge --no-ff origin/claude-csux-10`（tip `faafb578d`），无冲突，协调方再加一笔上述两处改正。三道门在 main 上退 0（tsc、run-tests 1164、vite build）。
