# 管理台工作区形态：以 idp-ui `apps/myshop-web` 为参照（UX 对齐第二轮）

Category: enhancement
Status: resolved——五票全落（03 / 04 / 05 于 09-20 至 09-21 进 main；01 / 02 于 2026-09-23 随 `539b8764` 进 main，码 SHA 不换）。判断项 1–3 未经用户逐条答，按 spec 推荐取值落地并写进票 01；用户若要分栏 / `ActivityBar` 各立追加票。2026-09-24 票 01 / 02 补齐非作者评审（通道 2 / 3），01 的 Spec 阻断与两票各条非阻断已在 `main` 上逐笔处置（本地，未推）；另立追加票 06（draft，形态取舍归用户）
出处：用户 2026-09-20 19:4x 经 IDP 队列「`/workspace/idp/idp-ui/apps/myshop-web`，ssh idp-110-dev 参考这个 ui 对我们的项目进行 ux 优化」。
参照物钉在 `idp-ui@6751fb2`（idp-110-dev `/workspace/idp/idp-ui`，2026-08-31 `feat(ui-workspace): CommandPalette async searchSource, badges, emptyState`；`scp` 取证于 19:4x，
源在 `%TEMP%\myshop-web-ref`，不入库）：`apps/myshop-web`（MyShop 订单管理工作台演示，数据全为 `data/oms.ts` 硬编码样例）与一份规范
`docs/oms_ui_ux_blueprint_v_1.md`（OMS UI/UX Blueprint v1，下称**蓝图**）。蓝图是 OMS 领域的 UX 蓝图，能搬到本仓的是它**领域无关的那半**：
第 7 节三张母版（A Dashboard / B List → Preview → Inspector / C Header + Context Bar + Tabs + Inspector）、第 13 节检查器约束、第 14.6 节「一主态多副态」、
第 21 节全局 UX 规则；订单 / 支付 / 履约那些词一个都不搬。第一轮（`.scratch/admin-web-ux-alignment/`，参照 loms-web + 手册 + 黄金标准）已落的六票是本轮的地基，
本轮只做第一轮没覆盖或当时明确「不做」而参照物现在给出了新证据的部分。

## 评估结论（钉 main `5a272da6`，通道 1，2026-09-20 19:4x–20:0x；代码对读，浏览器未起）

**已经对齐的**（不立票）：

- 顶栏四位、Light 默认、密度两档（第一轮 01）；列表母版面包屑 / Filter Bar 完整位 / 单击预览 + 双击打开 / surface 容器（03）；对象工作区 Object Header +
  Summary Strip + 稳定命名 Tabs（04，= 蓝图母版 C 去掉右侧检查器）；状态五层分家（05，= 蓝图 14.6「一主态多副态」）；skeleton / 错误分层 / 空态三件（06）；
  「我的工作」两页 + 记录钩子（02，在途 `mcp4-ux02`）。
- 依赖面：本仓 vendor 的 `@idpxyz/ui-workspace@0.1.25`（`apps/admin-web/vendor/idpxyz-ui/`）已导出 myshop-web 壳层用到的全部件——`TitleBar` / `ActivityBar` /
  `Sidebar` / `EditorGroup` / `StatusBar` / `CommandPalette` / `useResize` / `useSplitResize` / `useEditorGroupTabState` / `WorkspaceNavigationGuard`——从
  `node_modules/@idpxyz/ui-workspace/dist/index.d.ts` 对读；`6751fb2` 新加的 `CommandPalette` 异步 `searchSource` 不在 0.1.25 里，本轮不需要它（没有跨对象搜索读口）。
  不必升级 vendor 包。
- 异常处置面：`pages/visibility/ExceptionTriagePage` 已是蓝图第 15 节「队列 + 右侧处置面板」的形（`ReviewFlowTemplate`），四项分诊决定诚实禁用——缺的是命令端点，
  不是 UI 形态，不立票。

**缺口**，按蓝图逐节对照：

| 档 | 缺口（蓝图出处） | myshop-web 落点 | 归属 |
|---|---|---|---|
| 壳层：多标签工作区 | 蓝图 7 节三张母版都假定「同时开着多个对象」；myshop-web 壳是 `TitleBar` + `ActivityBar` + `Sidebar` + `EditorGroup`（多标签：关闭 / 固定 / 关闭其他 / 关闭右侧 / 重开已关 / 拖排 / 分栏）+ `BottomPanel` + `RightSidebar` + `StatusBar`，工作区状态整份持久化到 `localStorage`。我们是单页区 + hash 路由（`Layout.tsx` 头注当年裁「等首个真实页面出现后再按实际交互决定是否升级形态」——真实页面早已有了：委托查阅的 hash 二段详情、对象工作区母版）。第一轮 spec 把 Workbench Tabs 列为「不做」，理由是 loms-web 的 console 形态没有；myshop-web 有，且参照物换了 | `App.tsx`、`hooks/useWorkspaceState.ts` | 票 01（**形态取舍归用户**，见「判断项」） |
| 右侧检查器 | 蓝图母版 B「List → Detail Preview → Inspector」与 13 节：单击行 → 右侧检查器（Summary / 状态簇 / 快速动作 / 关联对象 / 审计元数据），「不成为第二张页、长表单不进、宽度稳定」。我们 03 的单击预览走各页自己的抽屉（`DetailRow` 十几格），没有壳层级的检查器位 | `shell/RightSidebar.tsx`（`OrderInspector` / `ExceptionInspector` 等按对象种类分派，`Section` 折叠节） | 票 02 |
| 命令面板 | 蓝图 23.1 `CommandPalette`；黄金标准「Command Palette」最近访问切换。myshop-web `Ctrl+K` 起面板，动作 = 导航（打开 X）+ 最近对象前 10 + 布局切换 + 快捷动作。我们第一轮 01 只留了禁用的全局搜索位 | `commandActions.ts` `buildDefaultCommandActions`、`App.tsx` 键盘监听 | 票 03 |
| 列表多选 + 批量动作栏 | 蓝图 10.5「多选行 → 露出 Bulk Action Bar」、10.7 批量动作。我们 `ListPageTemplate` 无选择模型 | `pages/OrdersList.tsx`（`selectedIds` + 表头全选 + 「已选 N 项」栏） | 票 04 |
| 动作反馈与高风险确认 | 蓝图 21.1「每个动作必须有反馈：成功 / 失败 / 进行中 / 排队 / 部分成功」、21.2「高风险动作需确认」。我们 `useToast` 只在 `pages/party/detail-primitives` 与演示页出现，其余写面（`postMasterData` 的调用点）反馈各自为政 | `RightSidebar.tsx` 快捷操作 → `addToast`、`components/ActionModal.tsx` | 票 05 |

**不做 / 不照搬的**（写明理由）：

- **`BottomPanel`**（事件 / 日志 / 异常 / 任务 / 审计 / 备注六页签）：myshop-web 里全是硬编码样例；本仓没有面向 UI 的事件流、日志或备注读口，审计已在对象工作区
  Audit 页签与 `RevisionHistorySection`。留位也不留——一个永远空的底栏不是「禁用态 + 说明」能交代的。等读口。
- **`ActivityBar`**：myshop-web 只用了 `explorer` 一项（点它只是折叠侧栏），是 IDE 形不是能力；我们只有一种侧栏内容。票 01 不装，作为判断项写明，用户要就加。
- **Dashboard / 控制塔**（蓝图 9 节）：与第一轮同一理由——红线「不放伪造统计」，没有计数读口；`Workbench` 维持就绪度总览 + 02 的两块卡。蓝图 9.5「点指标开对应筛选队列」
  这条形态规则留给将来有计数读口的那票。
- **角色化落地页**（蓝图 20 节）：会话主体没有角色声明（ADR-0100 把绑定放服务端操作者册），归 owner。
- **Watchlists / My Queues**、**`ThemeProvider product`**：同第一轮，归 owner / 归用户上游。
- **POS 专注模式、订单 / 支付 / 履约 / 促销 / 优惠券 / 会员**：OMS 专属，与本仓限界上下文无交集。
- **`TitleBar` 换掉 `shell/TopBar`**：01 的 TopBar 按手册「顶栏规范」做的、评审过；`TitleBar` 的 `recentObjects` 下拉与 `onOpenCommandPalette` 两个位由票 03 在 TopBar 上补，不换件。

**判断项（归用户，票 01 派前要答）**：

1. 多标签工作区要不要——蓝图与 myshop-web 都要；代价是 hash 路由与标签集要互为镜像（hash 仍是位置权威，见票 01），每张已打开的对象页都变成一个可关闭的标签。
   不要的话票 01 作废，02 / 03 / 04 / 05 不受影响。
2. 分栏（`onSplitRight` / `onSplitDown`）要不要——myshop-web 有，两栏各自一组标签。推荐**第一版不做**（`showSplitButtons={false}`），先把单组标签做稳。
3. `ActivityBar` 要不要——推荐不要（理由见上）。

阻塞边：01 与第一轮 02 同动 `Layout.tsx`，01 Blocked by ux-alignment/02 进 main；02 的壳层右栏位落在 01 重写后的 `Layout.tsx`，02 Blocked by 01（模板层的检查器契约与
首用页渲染器不等 01，可先做，票面分两段）；03 读第一轮 02 的最近对象存储，03 Blocked by ux-alignment/02 进 main；04 只动 `templates/ListPageTemplate.tsx` +
`list-page-structure.ts` + 一两张首用页，05 只动各页写动作调用点与 `components/`，两票互不相交、也不与 01–03 相交，可立即派。

## 范围

- **做**：五张票，各在自己的隔离 worktree 上做，全 TS；模板改动一律**向后兼容**（新 prop 可选、默认行为不变）；不改服务端 / 端点；不新造读口；不动领域文档。
- **验收口径**：沿第一轮——每票四道门 `tsc -b --noEmit` / `run-tests` / `vite build` / `go test ./internal/architecture/ -count=1`；浏览器验收做不到如实写「未验」；
  组件层断言照第一轮 01 的一次性 esbuild 束（源与产物不入库）；纯逻辑抬进 `.ts` 用 node:test 钉。

## 红线

- 蓝图是**形态**规范，本仓 AGENTS 与 CONTEXT 是**内容**规范：撞车时后者赢。检查器的「快速动作」只放已有端点的动作，其余禁用 + 说明；批量动作栏默认只有本机能完成的动作。
- 「即使功能未完整也要留位」只允许**禁用态 + 说明**，不允许假动作、假数据、假计数（myshop-web 的 `addToast('操作成功')` 假反馈一条都不搬）。
- 状态词只取 CONTEXT 原词；检查器状态簇复用 05 的 `LayeredStatusBadge`，不另画。
- 注释中文、引符号名、不计数不引行号；引蓝图用节号 + 仓 SHA。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-shell-editor-group-tabs-status-bar.md) | 壳层升级为多标签工作区：`EditorGroup` + `StatusBar`、工作区状态本地持久化、hash ↔ 标签互为镜像、`Ctrl+W` / `Ctrl+Shift+T` | resolved · 已进 main `539b8764`（2026-09-23 push，`e0d3f89d..539b8764` 纯 ff，码 `3da0f23c` / `c9312bf6` SHA 不换；判断项按推荐取值——多标签要 / 分栏不做 / `ActivityBar` 不装，工作台改为「无活动标签」那一格而非常驻标签；探针 20 ok；推送方自审）· 2026-09-24 非作者评审 ← 通道 2：Spec 阻断 1（坏存储整页白屏）已修 `d365851b`，非阻断逐条处置见票面「评审后修复」（本地，未推） |
| [02](./issues/02-right-inspector-list-preview.md) | 右侧检查器：壳层右栏位 + `InspectorContent` 契约（五节）+ `ListPageTemplate` 单击进检查器 + 两张首用页渲染器 | resolved · 已进 main `539b8764`（2026-09-23 push，`e0d3f89d..539b8764` 纯 ff，码 `92cbcf1a` 模板段 / `9015cdaf` 壳层段 SHA 不换；面板用 ui-primitives 自带的 Inspector* 一族；委托查阅单击进检查器、双击开详情；探针 25 ok；推送方自审）· 2026-09-24 非作者评审 ← 通道 3：两轴 0 阻断，非阻断逐条处置见票面「评审后修复」（本地，未推） |
| [03](./issues/03-command-palette.md) | 命令面板 `Ctrl+K`：导航 + 最近对象 + 壳层开关；TopBar 全局搜索位改为面板入口 | resolved · 已进 main `6739ab54`（码 `e785a28e` / `67982672` / `18ba6620` + `938a2a54`——通道 6 09-20 21:00 写完 `Layout.tsx` 接线未提交、会话无响应，推送方 09-21 原样入库；完成记录推送方代落 `6739ab54`；评审 ← 通道 4 Spec 阻断 1 → 接线笔后推送方自跑 Spec 轴 0 阻断，不算非作者评审） |
| [04](./issues/04-list-selection-bulk-action-bar.md) | `ListPageTemplate` 多选 + 批量动作栏：选择模型、表头全选、「已选 N 项」栏、默认动作「导出所选 CSV」、页面级动作槽 | resolved · 已进 main `a5a5527d`（码 `8fe22c59` / `3b97134b`，推送方代落完成记录 `5deab441`；通道 3 码推完后会话 crash；评审 ← 通道 4 两轴 0 阻断，非阻断六条记票面） |
| [05](./issues/05-action-feedback-confirmation-sweep.md) | 写动作反馈与高风险确认一致性：先审计全部写面调用点成对照表，再补 `useToast` / `ConfirmDialog` / 进行中态 | resolved · 已进 main `a5a5527d`（码 `2f3a8d3a` / `cdaa27b4` / `f8d37cf6` + 推送方注释笔 `844bc6a7`，完成记录由推送方代落；通道 5 末笔后会话未响应；评审 ← 通道 4 两轴 0 阻断） |
| [06](./issues/06-list-detail-roundtrip-keeps-page-state.md) | 列表 → 详情往返保住检索词与多选集：多标签壳层下「进详情再回来即清零」的出路 | draft · 形态取舍等用户答（票 01 评审 ← 通道 2 Spec 非阻断 1） |
