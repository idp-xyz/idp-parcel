# 管理台 UI/UX 对齐 IDP 产品族规范：以 idp-ui `apps/loms-web` 为参照

Category: enhancement
Status: resolved——六票 01–06 全部 resolved · 已进 main（末票 02 于 2026-09-20 20:13 `5b032504`）；第二轮见 `.scratch/admin-web-workspace-form/spec.md`
出处：用户 2026-09-16 23:5x 经 IDP 队列「https://github.com/idpxyz/idp-ui/tree/master/apps/loms-web, 参考这个调整我们的 ui/ux」。
参照物钉在 `idpxyz/idp-ui@53df1666`（tarball 取证于 23:5x，私有仓，`gh` 可读）：`apps/loms-web`（console 形态 + 二十余张演示页，数据全为硬编码样例）
与三份规范文档——`docs/idp_product_ux_ui_design_system_handbook_v_1.md`（产品族 UX/UI 手册，下称**手册**）、
`docs/idp_monitor_page_golden_standard_v_1.md`（Monitor 页黄金标准，下称**黄金标准**）、`docs/loms_console_page_wireframe_blueprint_v_1.md`
（LOMS 四张页的线框蓝图）。loms-web 的页面代码注释逐条引这三份文档的小节号，所以「参考 loms-web」实际要参考的是规范本身，页面只是范例。
入库：spec 与票 01–03 由 09-17 00:0x 的通道 1 会话写就、留在共享树未提交；09-20 通道 1 新会话按用户「自决」补写 04–06 后一并入库（代码对读钉的仍是
`041adc37`，其后 main 只多了 admin-web-group-legal-entities 13 / 14 的 `pages/party` 收口，与本 spec 六票地盘只在 `RevisionHistorySection.tsx` 一处相接，见 06）。

## 评估结论（钉 main `041adc37`，通道 1，2026-09-16 23:5x–00:1x；代码对读，浏览器未起）

**已经对齐的**（不立票）：

- 壳层形态：`App.tsx` / `Layout.tsx` 本来就照 loms-web `console/Layout` 做——`ThemeProvider` + `ToastProvider` + 品牌头 + `Sidebar`（`useResize`）+ 单页区；
  `index.css` 的 `--idpxyz-*` 令牌与 loms-web 同一份。
- 组件来源：36 张列表页经 `ListPageTemplate` 用 `@idpxyz/ui-patterns` 的 `PageHeader` / `FilterBar` 与 `@idpxyz/ui-primitives` 的 `Table` / `Pagination`——
  比 loms-web 自己手写 `<table>` 更贴设计系统；`Tabs` 24 张页在用。
- 空态分层（手册「状态与反馈规范」空状态五分）：`TemplateViewState` 已分 `loading / ready / empty / error / unconfigured`，筛空另走
  `emptyRowsNote`（票 admin-web-group-legal-entities/01 裁决 1）；「未配置」与「暂无数据」严格两态是本仓红线，不是缺口。
- 状态一词一色：`domain/status.tsx` `domainStatusTones` + `StatusBadgeFor`，词取 CONTEXT 原词——手册「色彩系统规范」状态语义色统一那条已守住。
- 导航按业务价值链分区、条目锚定限界上下文、`moduleInfoById` 记主责与出处；未接线条目落 `UnwiredModule` 诚实占位；工作台只陈述就绪度、不放伪造统计。
- 页面里无硬编码 Tailwind 调色板色（`git grep` 零命中），换主题不用扫页。

**缺口**，按手册 / 黄金标准逐节对照，五档：

| 档 | 缺口（规范出处） | 归属 |
|---|---|---|
| 壳层 Top Bar | 手册「顶栏规范」要求 Global Search / Scope Switcher / Alerts / User Menu，我们只有品牌 + 当前页名；手册「Top Bar 归属信息规范」要求 `{产品} / IDP · {模块名称}`，我们是「IDP Parcel / 租户管理台 … 页名」；手册「主题策略」企业工作台**默认 Light**、黄金标准第 13 节整章讲 Light 四层 surface，我们默认 dark 且无切换；手册「栅格与密度」要求 Comfortable / Compact 两档，`useDensity` 在 ui-theme-runtime 里现成，我们没接 | 票 01 |
| 左导航「我的工作」 | 黄金标准「Left Navigation 黄金标准」Rule 6：左导航固定两层「My Work（Saved Views / Recent Objects / Watchlists / My Queues）+ 业务导航」；我们只有业务导航。Recent Objects 与 Saved Views 纯客户端即可诚实成立（hash 历史 + 本地保存的筛选态）；Watchlists / My Queues 需要「关注」「指派」这类领域概念，CONTEXT 里没有——**不虚构**，归 owner 裁要不要进领域 | 票 02（前两项）；后两项归 owner |
| Monitor 页母版 | 黄金标准「Filter Bar 黄金标准」P0 最低可见结构 Search / Status / Sort / View Mode / More Filters + Saved View，且「即使暂时只有 Search + Status，视觉上也必须按完整模板来做」；「Page Header / Breadcrumb」要 `区 › 页` 面包屑；「Table 黄金标准」单击预览 / 双击开对象工作区、密度两档、表格放进 surface container、页脚稳定；我们的 `ListPageTemplate` 是 PageHeader + FilterBar（只搜索 + 调用方自带下拉）+ Table + Pagination，无面包屑、无排序 / 视图 / 保存视图 / 更多筛选的位、无密度、行只有单击 | 票 03 |
| 对象工作区母版 | 手册「对象工作区（Workspace）统一规范」：Object Header（主编号 / 状态 / 风险 / 负责人 / 最后更新 / 快速动作）+ Summary Strip（4–6 指标）+ Main Content Tabs（Summary / Timeline / Related / Exceptions / Documents / Audit 稳定命名）+ 可选右侧上下文；我们的 `DetailPageTemplate` 是分节列表（3 张页在用），行详情靠抽屉（`DetailRow` 十几格 + 修订历史时间线）——抽屉是黄金标准里的 Preview Pane，不是 Workspace；有 hash 二段路由（委托查阅）却没有对象页母版 | 票 04 |
| 状态 badge 分层 | 黄金标准「状态语义黄金标准」：Lifecycle / SLA / Risk / Severity / Flags 五层必须在设计系统层预先分开；`domainStatusTones` 今天是一张扁平的词→色表，生命周期词与异常严重度词同表同形；追踪 ETA / 异常案件 / 关务限制页将来会同时出现三层 | 票 05 |
| Loading / 错误分层 | 手册「Loading」要求列表 skeleton 而不是转圈、骨架按内容形状；「Error State」要求 whole page / widget / section / action / background refresh 分层并局部隔离；我们 `LoadingState` 是一块通用四行脉冲（不是转圈，但不按内容形状——表格与时间线长一样），`error` 一态不分层；ui-patterns 已有 `SkeletonTable` / `SkeletonEventList`，ui-primitives 已有 `FilteredEmptyState` / `TrulyEmptyState` / `UnavailableState` / `SectionErrorState`（loms-web 在用） | 票 06 |

**不做 / 不照搬的**（写明理由，免得下一个人再评一遍）：

- **Dashboard 的 KPI Strip / 热力图 / 趋势**（手册「Dashboard / Overview Page」、蓝图页 01）：loms-web 那页全是硬编码样例数字。本仓红线「不放任何伪造统计」；
  目录读口一次拉全量且 `isolatedReadLimit = 200`，从截断列表数出来的 KPI 是假的。工作台维持「就绪度总览」+ 02 落地后加「最近对象 / 保存视图」两块——
  都是 UI 自身的事实。业务 KPI 等计数读口有了再立票（归票 admin-web-group-legal-entities/04 那条契约决策的同族）。
- **Workbench Tabs（多对象标签页）与 Bottom Panel**（黄金标准第 7 / 12 节）：loms-web 自己的 console 形态也**没有**这两样（`console/Layout` 注释明写
  「minus the IDE-workspace tabs / activity-bar / panels」），IDE 工作区形态在它 `src/App.tsx` 里另存一套。本仓 `Layout.tsx` 头注已裁「等首个真实页面出现后
  再按实际交互决定是否升级形态」。Bottom Panel 要承载 Events / Logs / Audit——本仓没有面向 UI 的事件流读口。两样都不在本轮；03 的双击开对象走 hash 路由、
  当前上下文打开（手册「Drill-down 模式」的 Current Context），不开标签。
- **Watchlists / My Queues**：见上表，领域概念缺位，归 owner。
- **Command Palette / 全局搜索的真实现**：没有跨对象搜索读口；01 只留位（黄金标准「即使当前功能暂不完整，也应保留视觉占位和结构位置」）并
  用禁用态 + 悬停说明诚实交代，不接假搜索。
- **产品 accent**：`ThemeProvider product` 今天不传（`ui-tokens` 的 `ProductKey` 没登记 parcel）——登记在 idp-ui 上游仓，**归用户**；登记后 01 的
  归属信息处一并传入。

阻塞边：01 与 02 都动 `Layout.tsx`，02 Blocked by 01；03 与 04 都动 `pages/template-preview/*` 演示页，04 的演示页那一笔 Blocked by 03（模板本体与首用页
不等）；03 / 04 / 05 / 06 的主体分别落 `templates/ListPageTemplate.tsx`、`templates/DetailPageTemplate.tsx` + `pages/shipment-request/*`、`domain/status.tsx`
（用 `StatusBadgeFor` 的页只被动受影响、不逐页改）、`templates/state-slot.tsx` + `components/states` + `pages/catalogue-view.ts`（+ 一处首用
`pages/party/RevisionHistorySection.tsx`），地盘互不相交，可并行；03 / 04 / 06 都改模板层但不同文件——06 的形状经 `TemplateViewState` 传，不碰两个模板文件。

## 范围

- **做**：六张票，各在自己的隔离 worktree 上做，全 TS；模板改动一律**向后兼容**（新 prop 可选、默认行为不变），36 张列表页与 24 张带 Tabs 的页不逐页改。
- **不做**：不改服务端 / 端点；不新造读口；不动领域文档。
- **验收口径**：每票三道门 `tsc -b --noEmit` / `run-tests` / `vite build`；浏览器验收本机做不到（AuthGate 要 gk.idp.xyz 会话）就如实写「未验」，并附
  `vite build` 产物里的关键 DOM 结构断言或 node:test 钉纯逻辑。**组件层断言的前提**：`tsconfig.test.json` 今天只 include `src/**/*.test.ts` 并按 CommonJS
  发射到 `.tmp-test/`，`.test.ts` 引 `.tsx` 会被 `tsc` 跟进编译，但运行期 `require` 到 ESM 的 `@idpxyz/*` 原语可能失败——首个做到的票在票面写下实测
  结论（成 / 不成、怎么成），后面的票照抄，不各自再试；不成就把要钉的逻辑抬进 `.ts` 纯函数钉，票面写「组件层未钉」。

## 红线

- 手册与黄金标准是**形态**规范，本仓 AGENTS 与 CONTEXT 是**内容**规范：形态服从前者，任何文字、状态词、数字服从后者。撞车时后者赢——例如 KPI 位宁可空着。
- 「即使功能未完整也要留位」只允许**禁用态 + 说明**，不允许假动作、假数据、假计数。
- 状态词只取 CONTEXT 原词（沿票 admin-web-group-legal-entities 的红线），分层只改色调族与形状，不改词。
- 注释中文、引符号名、不计数不引行号；引上游文档用小节标题名 + 仓 SHA，不引行号。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-shell-top-bar-theme-density.md) | 壳层：Top Bar 四件（归属信息 / 全局搜索位 / 作用域位 / 用户菜单位）、Light 默认 + 主题切换、密度两档 | resolved · 已进 main `ed7ef224`（通道 4 两任；评审 ← 通道 2 两轴 0 阻断；搜索位 Button 外壳与删 `SessionBadge` 两条为推送方裁决，见票面判断项 2 / 6） |
| [02](./issues/02-my-work-recent-objects-saved-views.md) | 左导航「我的工作」：最近对象（hash 历史）+ 保存视图（本地筛选态）两页与记录钩子；Watchlists / My Queues 归 owner | resolved · 已进 main `5b032504`（通道 4 两任——前任现场原样封存后接续；评审 ← 通道 2 两轴 0 阻断；保存视图空态文案改口为推送方代落，见票面处置） |
| [03](./issues/03-list-template-monitor-golden.md) | `ListPageTemplate` 对齐 Monitor 黄金母版：面包屑、Filter Bar 完整结构位、密度、单击预览 / 双击开对象、surface 容器 | resolved · 已进 main `3fbf2ec2`（通道 3；评审 ← 通道 2 两轴 0 阻断；默认形态按 Rule 2 改了哪几样与密度默认档归 01，见票面裁决 3） |
| [04](./issues/04-object-workspace-template.md) | `DetailPageTemplate` 对齐对象工作区母版：Object Header + Summary Strip + 稳定命名的 Tabs；委托查阅详情页首用 | resolved · 已进 main（第 1–5 条 `3fbf2ec2`、第 6 条 `4a19ed33` 纯 ff；通道 5；评审 ← 通道 6 两轮皆 0 阻断） |
| [05](./issues/05-status-badge-layering.md) | 状态 badge 五层分家：`domainStatusTones` 拆生命周期 / SLA / 风险 / 严重度 / 标记，`StatusBadgeFor` 按层取形 | resolved · 已进 main `f6f843c8`（通道 1 自接；评审 ← 通道 2 两轴 0 阻断；sla / flag 两形无色与 `TagVariant` 派生留给首批词进表那票） |
| [06](./issues/06-loading-skeleton-error-layering.md) | Loading 用 skeleton、错误按页 / 区块 / 动作分层、空态换 ui-primitives 三件 | resolved · 已进 main `f6f843c8`（通道 5；评审 ← 通道 6 两轴 0 阻断；`SectionErrorState` / `UnavailableState` 文案写死故自绘 / 不换，票面改口；骨架 `self-start` 等三处小改归模板层收口） |
