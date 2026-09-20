# 04 `DetailPageTemplate` 对齐对象工作区母版：Object Header + Summary Strip + 稳定命名的 Tabs；委托查阅详情页首用

Category: enhancement
Status: resolved——第 1–5 条已进 main（码 `ccd1ce3c`，推送方注释笔 `3fbf2ec2`；评审 ← 通道 6 两轴 0 阻断）；第 6 条演示页在分支 `mcp5-ux04-6`（码 tip `add1d41c`，基 main `bd067052`）待评审 / 进 main
Blocked by: 无（原 Blocked by 03——两票都改 `pages/template-preview/*` 演示页；03 已与本票第 1–5 条同批进 main，第 6 条从 `origin/main` 起做）
地盘：`apps/admin-web/src/templates/DetailPageTemplate.tsx`、`templates/types.ts`、`templates/demo.ts`、`templates/index.ts`、`pages/template-preview/*`
（等 03）、`pages/shipment-request/ShipmentRequestDetailPage.tsx`（首用）。**不改另外两张用它的页**（`governance/StageAdmissionPage`、
`settlement/SettlementApplicationPage`）：一切新能力走可选 prop，不传时渲染与今天逐字节同。
出处：spec「缺口」表第四档；手册「对象工作区（Workspace）统一规范」（Object Header / Summary Strip / Main Content Tabs 稳定命名 / 可选右侧上下文）、
「Drill-down 模式」（Current Context）；黄金标准「Preview Pane」与「对象工作区」两者分工；参照 idp-ui@53df1666 `apps/loms-web` 里的对象页
（`OrderDetail` 一类：头区主编号 + 状态 + 元信息行、指标条、稳定签）。

## 为什么

今天的 `DetailPageTemplate` 是「PageHeader + 一列 Card」——基本信息、若干业务区块、审计留痕从上到下叠着，看一个对象要滚。手册把对象页定成
跨产品同一形态：头上一眼看清「这是哪个对象、什么状态、多久前动过、能做什么」，一条指标带回答「有多大 / 多少件 / 多少钱」，下面按**稳定命名**的签
分内容——Summary / Timeline / Related / Exceptions / Documents / Audit，换产品不换签名。本仓有 hash 二段路由（委托查阅已在用），却没有对象页母版，
03 做完「双击开对象」之后打开的还是叠 Card 的页；抽屉是黄金标准里的 Preview Pane，形态与 Workspace 各归各，不互相替代。

## 要做的

1. **Object Header**：`PageHeader` 内重排——第一行主编号（`identifier`，等宽、可复制）+ 状态（`status`）+ 快速动作（`headerActions`，右端）；第二行元信息
   走新可选 prop `meta?: { label: string; value: ReactNode }[]`（如「最后更新」「提交方」「修订」），只显调用方给的，模板不替对象编造格。
   手册里的「风险」「负责人」两格：本仓没有对应领域来源（GLOSSARY 无「负责人」，风险归 visibility-exception 的案件而非对象本身），**不留位**——
   spec 红线「不虚构」，等有源再加 prop。
2. **Summary Strip**：新可选 prop `summary?: { label: string; value: ReactNode }[]`（4–6 格，照 ui-primitives `StatCard` 的形以 `Card` 自排），只放对象自身的事实
   （件数、修订号、金额已确认与否这类从读口原样来的值），**不放派生 KPI**——spec「不做」第一条同一理由；不传不渲染，不显「—」占位格。
   （评审 Spec (a)(b) 后改口：原写 `tone?: StatusTone` 与「`StatCard` 一排」——色调→徽章变体表 `badgeStatusByTone` 未导出且在 `domain/status.tsx`，
   `templates/` 不新开对 `domain/` 的依赖，带色调的值由调用方放 `StatusBadgeFor`；`StatCard` 的 `value` 只收 `string | number`、尺度是 Dashboard 的。）
3. **稳定命名的 Tabs**：新可选 prop `tabs?: { id: WorkspaceTabId; content: ReactNode; count?: number }[]`，`WorkspaceTabId = 'summary' | 'timeline' |
   'related' | 'exceptions' | 'documents' | 'audit'`；签名与顺序由模板里一张固定表钉住（概要 / 时间线 / 关联 / 异常 / 文档 / 审计），调用方只给 id 与内容，
   传进来的顺序不算数。传了 `tabs` 时：`basicFields` 进「概要」签顶部、`sections` 跟在其后、`auditTrail` 进「审计」签；调用方另给同 id 的 `content` 时
   接在模板内容之后。**不传 `tabs` 时渲染与今天同**（叠 Card）。
4. **可选右侧上下文**：新可选 prop `aside?: ReactNode`，宽 280px、大屏才显（`xl:`），放调用方给的关联对象 / 说明；不传不占位。
5. **委托查阅详情页首用**（`ShipmentRequestDetailPage`）：主编号 + 状态词进头区；`meta` 只放读口已有的（提交时点 / 修订 / 客户资料版本一类，按 `view`
   字段实有的填）；`summary` 只放读口已有的计数（包裹件数等）；签：概要（基本信息 + 现有区块）与审计（现有 `auditTrail`）——**只显有内容的签**（裁决 1）。
   不新造读口、不编「时间线」——委托的事件流没有面向 UI 的读口。
6. **`template-preview` 演示页**（等 03 进 main）：加一张对象工作区演示（合成 S，`templates/demo.ts`），头区 + 指标条 + 三个签 + 右侧上下文，
   `renderToStaticMarkup` 可断言签名顺序。

## 不做

- 不做 Workbench Tabs（多对象标签页）与 Bottom Panel——spec「不做」已裁；对象在当前上下文打开（hash）。
- 不改抽屉（Preview Pane 形态照旧，03 裁决 2）；不把抽屉换成对象页。
- 不新造读口；负责人 / 风险 / 时间线三样无源不显、不留位。
- 不逐页改另外两张详情页。

## 完成判据

- 三道门绿；既有 `run-tests` 零改动仍绿（默认渲染不变的证据）。
- 模板里的纯逻辑（签固定表：id → 词与顺序；`tabs` 归并——模板内容与调用方内容按 id 合、按固定序排）抬成 `templates/` 下的纯函数并带 node:test。
- `template-preview` 页在 `vite build` 产物里能看到头区主编号、指标条格数与签名顺序（`renderToStaticMarkup` 断言；若测试编译链引不到 `.tsx` /
  ESM 原语——`tsconfig.test.json` 今天只 include `*.test.ts`、按 CommonJS 发射——就只钉纯函数，票面写明「组件层未钉」）。
- 浏览器验收做不到如实写「未验」。

## 裁决

1. **签只显有内容的，不显禁用签**：签是导航，不是 03 那种「功能结构位」——六个灰签比空白更误导（点不进的导航是死路）。「稳定命名」由固定表钉顺序与词
   来满足：有哪几个签可变，签叫什么、谁在谁前不变。
2. **负责人 / 风险不留位**：黄金标准「留位」那条是给功能位的，对象头区的每一格都是一句陈述，陈述没有来源就是虚构。
3. **不传 `tabs` 仍叠 Card**：两张未首用的页不改；母版与旧形态并存到它们各自的票再换。

## Comments

### 认领（2026-09-20 15:3x）

通道 5，分支 `mcp5-ux04` 基 main `86a96ab7`；地盘 `templates/DetailPageTemplate.tsx`、新纯模块 `templates/workspace-tabs.ts`（+ node:test）、
首用 `pages/shipment-request/ShipmentRequestDetailPage.tsx`；`templates/types.ts` / `templates/index.ts` 只追加不改既有行（03 同时在追加）。
第 6 条演示页与 `templates/demo.ts`、`pages/template-preview/*` 本轮不碰，等 03 进 main 后 rebase 再补。

### 完成记录（通道 5 · 2026-09-20 15:3x–15:4x · 分支 `mcp5-ux04` 基 main `86a96ab7` · 码 tip `46984a85` · 第 1–5 条）

**笔** `ffd06dcd`（认领）→ `ddc887ac`（1）→ `b75b21db`（2）→ `d155a435`（3）→ `9d307336`（4）→ `46984a85`（5）→ 本票面笔。每笔 push origin；
不推 main、不占 55432、不跑 Go。只碰 `templates/DetailPageTemplate.tsx`、新 `templates/workspace-tabs.ts`（+ `.test.ts`）、`templates/index.ts`
（只插行：`DetailPageTemplate` 导出块内加三个类型 + 紧随其后新加 `workspace-tabs` 导出块，与 03 在该文件的 hunk 之间有未改行隔开）、
`pages/shipment-request/ShipmentRequestDetailPage.tsx`。`templates/types.ts` 未动（新类型都是本模板专属，按该文件头注留在 `DetailPageTemplate.tsx`）；
`ListPageTemplate.tsx` / `Layout` / `App` / `index.css` / `state-slot.tsx` / `components/states` / `domain/status.tsx` / `demo.ts` / `template-preview/*` /
`governance/StageAdmissionPage` / `settlement/SettlementApplicationPage` 一个没碰。

**要做的逐条**

1. ✅ Object Header：新可选 prop `meta?: DetailMeta[]`；对象头区两行——第一行标题 + 主编号（等宽、`select-all` + 复制按钮 `CopyIdentifierButton`，
   剪贴板不可用时只留 select-all）+ 状态 + 右端快速动作，第二行元信息 `dl`，只显调用方给的。负责人 / 风险不留位（裁决 2）。
   对象头区随 `meta` 或 `tabs` 任一启用；都不传走原单行头那一支 JSX，原样保留未抽片段。
2. ✅ Summary Strip：新可选 prop `summary?: DetailSummaryStat[]`，头区下 auto-fit 一排 `Card`，只在 ready 态且非空时渲染，不显「—」格。
   **偏离票面两处，理由见判断项**：`value` 收 `ReactNode`（票面写的也是 ReactNode）而**没有 `tone` prop**；结构照 `StatCard` 但没直接用它。
3. ✅ 稳定命名的签：新纯模块 `templates/workspace-tabs.ts`——`WorkspaceTabId` 六值、`workspaceTabOrder` / `workspaceTabLabels` 固定表
   （概要 / 时间线 / 关联 / 异常 / 文档 / 审计）、`resolveWorkspaceTabs<Content>` 归并（模板内容在前、调用方同 id 接后、按固定序、无内容不出签、
   同 id 多次依次接上、`count` 取第一个给了的）。模板新可选 prop `tabs?: DetailWorkspaceTab[]`：传了（含 `[]`）按签分——基本信息进「概要」顶部、
   区块跟其后、审计留痕进「审计」；受控 `Tabs`，当前签消失退回第一签；调用方 `null` / 布尔 / 空串内容归并前剔掉。不传仍叠 Card（裁决 3）。
4. ✅ 右侧上下文：新可选 prop `aside?: ReactNode`，`ContentColumns` 在 xl 及以上加 280px 右列、主列 960px 不变、容器放宽到 1256px；窄屏藏；
   不传或传空不占位。叠 Card / 分签两形态共用同一容器。
5. ✅ 首用 `ShipmentRequestDetailPage`：`meta` 四格（提交时间 / 系统接收时间 / 来源 / 当前提交版本，全是 `view` 实有字段）；`summary` 两格
   （`declaredParcelCount` / `priorVersionCount`，读口原样计数）；`tabs={[]}` → 只有「概要」一签（基本信息 + 现有三个区块）——委托的事件流与
   审计留痕都没有面向 UI 的读口，「时间线」「审计」无内容不出（裁决 1），不新造读口。主编号与状态词沿头区原位。
6. ✅ `template-preview` 演示页（分支 `mcp5-ux04-6` 笔 `9eaba4f4`，基 main `bd067052`）：预览页新增「对象工作区」一签，渲染新组件
   `pages/template-preview/ObjectWorkspaceDemo.tsx`——同一份合成申报单换成对象工作区排布：头区两行（主编号可复制 + 状态 + 元信息四格 + 两个快速动作）、
   指标带四格、签按 审计 → 关联 → 概要 倒序传而渲染为 概要 · 关联 · 审计、右侧上下文一卡；样例数据进 `templates/demo.ts`（`demoWorkspace*` 五个导出），
   基本信息 / 区块 / 审计留痕复用详情演示那三份。03 的列表演示与「详情模板」签未动。一次性 `renderToStaticMarkup` 探针 **28 断言全过**（见下）。

**完成判据逐条**

- 三道门 ✅ 每笔各跑：`tsc -b --noEmit` 0 / `run-tests` **296**（main 基线 290 + 本票 6）/ `vite build` 0。既有用例零改动——
  `git diff 86a96ab7 46984a85 -- '*.test.ts'` 只有新文件 `workspace-tabs.test.ts`。
- 纯逻辑抬出 + node:test ✅ `workspace-tabs.ts` 不引 React / `@idpxyz`，`workspace-tabs.test.ts` 六条：固定表六 id 顺序与词；调用方顺序不算数；
  同 id 归并模板在前；无内容不出签 / 单方有内容也出 / 全空为 `[]`；同 id 多次接上且 count 取第一个；出签的词取自固定表。
- 「不传 `tabs` 时渲染与今天同」✅ **一次性实测、组件层未钉**（照 05 那套 esbuild 束，源与产物不入库）：`cmd /c "git show 86a96ab7:…DetailPageTemplate.tsx > src/templates/old.probe.tsx"`
  取旧源，与 `46984a85` 版并排喂同一份 props，`renderToStaticMarkup` 逐字节比对——七组 fixture **7 同 / 0 异**：StageAdmission 形（无标识 / 状态 / 动作 /
  审计）、演示页形（标识 + 状态 + 动作 + 区块 + 审计）、审计空数组、无区块无字段、`loading`、`empty`、`error` 带重试。
  同一把束里工作区形态 20 条断言全过：头区含主编号与 `aria-label="复制主编号"`、元信息行在、指标带 4 格、调用方乱序传 audit / related / summary
  出签序为 概要 < 关联 < 审计、`content: null` 的文档签被剔、无内容签（时间线 / 异常 / 文档）不出、关联签带计数、aside 在、`role="tab"` 三个、
  概要签内模板内容在调用方内容之前、非当前签内容未挂载（Radix 默认不 forceMount）、`tabs=[]` 无审计 → 只有概要一签、`summary=[]` / `meta=[]`
  不渲染、不传 aside 无 `<aside>`、非 ready 态无签无指标带但对象头区仍在。
- `template-preview` 产物断言 ✅ **一次性实测、组件层未钉**（第 6 条完成记录里有逐条；`vite build` 产物 `dist/assets/index-*.js` 以 node 按 UTF-8 读、
  `includes` 核到演示标题「托运申报单（对象工作区演示）」、`SYN-BATCH-240819-07`、「留痕口径（演示）」「上下文（演示）」两个快速动作文案与说明条「签按固定序出」
  各 1 命中；签名固定表三词在）。**组件层未钉**的理由照抄 05 / 06 实测结论——`.test.ts` 不能 `require` 任何 `import` 了 `@idpxyz/*` 的模块
  （`exports` 只有 `types` + `import`，CJS `require` 报 `ERR_PACKAGE_PATH_NOT_EXPORTED`），本票未再试。
- 浏览器验收**未验**（本机无 gk.idp.xyz 会话）。

**判断项**

- **`DetailSummaryStat` 没有 `tone` prop**：色调 → 徽章变体那张表（`badgeStatusByTone`）只在 `domain/status.tsx` 一处、未导出，且该文件是 05 地盘；
  模板另立一张就是第二套口径，硬编码调色板色又违背 spec「无硬编码 Tailwind 调色板色」那条已对齐项。`value` 收 `ReactNode` 后带色调的值由调用方
  放 `StatusBadgeFor`——与 `status` / `DetailField.value` 同一约定。要 `tone` 的话，等 05 进 main 后从其导出的色调表接，一行事。
- **未直接用 `StatCard`**：其 `value` 只收 `string | number`（放不进徽章）、无色调位、`p-5` / `text-2xl` 是 Dashboard 尺度；改用 `Card` 照它的结构
  （小写标签 + 大数）压成对象头区下的密度。
- **对象头区的启用条件是 `meta !== undefined || tabs !== undefined`**，不是「任一新 prop」：`summary` / `aside` 单独传时头区不变——它们各自
  独立渲染，与头区形态无关；复制按钮属头区形态，跟着头区走。
- **分签形态里「概要」是否含基本信息卡按 `basicFields.length > 0` 判**：该 prop 必传，空数组分不出「不适用」与「暂无」；审计沿旧语义（空数组 =
  有区但暂无记录，那句话本身是内容）。叠 Card 形态不受影响，基本信息卡照旧永远渲染。
- **`count` 同 id 多次取第一个、不相加**：计数是调用方对那一签的陈述，模板不替它做算术；要合就调用方自己合成一签。
- **首用页头区元信息与基本信息卡有四个值重复**：基本信息卡仍是完整记录，头区那几格是速览；换的是不用滚就能对上编号与版本。要去重就从基本信息卡
  删——本票没删，留给看过实际页面的人定。
- **首用页只有一个签**：单签的「工作区」形态成立——稳定命名在，签随内容增减；不为凑签造「时间线」。
- `index.ts` 插行位置刻意避开 03 的 hunk（取证于 `mcp3-ux03@e7f17306`：03 改在 `ListPageTemplate` 导出块内与其后、以及文件末行前；本票在
  `DetailPageTemplate` 导出块内与其后），推送方重放时若仍撞，以两边都保留为准——都是只加不减的导出行。

### rebase 到 origin/main `7d29af39`（通道 5 · 15:5x · 05 / 06 进 main 后，按推送方广播「可提前看冲突」）

七笔零冲突重放（06 动的 `index.ts` / `state-slot.tsx` 与本票 hunk 不相邻）。SHA 对照：`ffd06dcd→e13f2bd5` / `ddc887ac→8b0f22b5` / `b75b21db→1431f2c9` /
`d155a435→f2e48d78` / `9d307336→9d5a170b` / `46984a85→44662c30` / `44ce2c2b→be8959c3`；上文完成记录里的旧 SHA 按此对照读，内容一字未改。
分支已 `--force-with-lease` 推，远端 `mcp5-ux04 = be8959c3`（ls-remote 核对）。**码 tip 现为 `44662c30`。**

新基上四道门：`tsc -b --noEmit` 0 / `run-tests` **304**（含 05 / 06 进 main 带来的用例 + 本票 6）/ `vite build` 0 /
**`go test ./internal/architecture/ -count=1` ok**（推送方 15:40 广播新加的一道：管理台路径门禁扫 `apps/admin-web/src` 全部 `.ts`/`.tsx` 含测试文件；
本票新增的 `workspace-tabs.test.ts` 没有路径字面量）。

### 评审 ← 通道 6 · 第 1–5 条 · 钉 `2c780b5c`（码 `44662c30`，基 `7d29af39`）· 16:0x（推送方自任务台 `task-fd51e177` 代落原文）

只读，隔离树已拆；未跑四道门，结论来自读 diff（含基线 `DetailPageTemplate.tsx` 全文对照）+ ui-primitives 源 + `domain/status.tsx` 导出面 + `shipment-request/api.ts` 读模型。

**Standards** — 阻断：无。非阻断：
1. `DetailPageTemplateProps.meta` 文档注「不传沿用单行头」与代码不符：头区形态由 `meta !== undefined || tabs !== undefined` 启用，`tabs` 的文档注未提它也切头区；
   首用页恰靠 `tabs={[]}` + `meta`。函数内注释写对了，prop 注要跟上。
2. `workspace-tabs.ts`：`WorkspaceTabId` 联合与 `workspaceTabOrder` 常量把六个 id 写了两遍，`satisfies` 只保元素合法不保六个齐；可 `type WorkspaceTabId =
   (typeof workspaceTabOrder)[number]` 合为一处；今天由 `workspace-tabs.test.ts` 首条钉住（Duplicated Code，判断项，轻）。
3. `DetailMeta` / `DetailSummaryStat` / `types.ts` `DetailField` 三个同形 `{label; value: ReactNode}`——按角色命名可接受，只记（轻）。
无发现（实核）：**不传新 prop 时逐字节同从代码能读出**——头区 else 支是基线 JSX 原文；`summary === undefined` 不出件；`tabs === undefined` 走 `ContentColumns`，
无 aside 时返回与基线同一串 class；`basicCard` / `sectionCards` / `auditCard` 与基线三块 JSX 逐字同；模板本体无 hook，hook 只在旧形态不挂载的 `WorkspaceTabs` /
`CopyIdentifierButton` 里。固定表一处：`workspaceTabOrder` 定序、`workspaceTabLabels` 定词，`resolveWorkspaceTabs` 外循环按固定序、传入序不算数、模板内容在前。
注释全中文、无行号 / 计数 / 变更说明；`StatCard` 确在 ui-primitives `card.tsx`（不在 ui-patterns）；diff 6 文件，禁碰清单未碰；色只用 idpxyz token。

**Spec** — 阻断：无。五条逐条 ✅（1 头区两行 + `CopyIdentifierButton` + 无负责人 / 风险格；2 `SummaryStrip` 只在 ready 且非空渲染；3 `templateContents`
归并如票面、不传 `tabs` 仍叠 Card；4 `aside` 无内容不占位、有则 `hidden xl:flex w-[280px]`；5 `buildMeta` 四格 / `buildSummary` 两格全是读口字段，件数取服务端计数；
**审计签**：基线该页本就未传 `auditTrail`、`ShipmentRequestDetail` 无审计字段，票面「现有 auditTrail」对此页不成立，作者理由如实）。
**(a) tone 裁：成立 → 票面改口**——`StatusTone` 已导出可引，但 `badgeStatusByTone` 未导出；`templates/` 今天零引 `domain/`，加 `tone` 要么新开依赖要么第二张表；
`value: ReactNode` 已能放 `StatusBadgeFor`。**(b) StatCard 裁：成立 → 票面改口**——`StatCard` 在 ui-primitives `card.tsx`，`value: string | number`、`p-5` /
`text-2xl`，`trend` 还用调色板色；自排标签行 class 与它逐字同。
非阻断：1. 分签形态「概要」含基本信息卡多了 `basicFields.length > 0` 门，叠 Card 形态永远在——同一 prop 两种语义；边角 `tabs=[]` + 无字段无区块 → ready 态空白内容区，
记票。2. 首用页头区四格与基本信息卡四值重复——作者已记「留给看过页面的人定」，不裁。
完成判据：既有用例零改动 ✓；纯逻辑六条 ✓；「一次性实测、组件层未钉」标法诚实 ✓；第 6 条 ⏳ 明记；判断项无失真。超票面：`templates/index.ts` 桶导三个
workspace-tabs 符号——第 6 条断言要用，无害。缺：无。

**Standards 0 / 3 · Spec 0 / 2** → 无阻断，可重放；第 6 条演示页那一笔待 03 进 main 后另评增量。

### 处置（推送方 · 通道 1 · 17:2x）

- Standards 1 → 推送方代落 `3fbf2ec2`（只改注释：`meta` / `tabs` 两条 prop 注如实写「对象头区随任一传入而启用、空数组也算传了」）。
- Standards 2 / 3、Spec 非阻断 1 / 2 → 记，不改（判断项；作者补第 6 条时若顺手合 `WorkspaceTabId` 为一处也可，不要求）。
- Spec (a)(b) → 票面第 2 条改口（本簿记笔）。

### 进 main 记录（第 1–5 条 · 推送方 · 通道 1）

- 重放：叠在 03 的落地 tip `d791bc26` 上 cherry-pick `7d29af39..2c780b5c` 零冲突（`templates/index.ts` 两票 hunk 不相邻，`git merge-tree` 干跑先核过），
  八笔 SHA 对照 `e13f2bd5→2f14a689` / `8b0f22b5→fcacb829` / `1431f2c9→cf36f32b` / `f2e48d78→8d3fa328` / `9d5a170b→e7e5f05e` / `44662c30→ccd1ce3c` /
  `be8959c3→6400d369` / `2c780b5c→007a3394`；五件与作者 tip 逐字节同。推送 tip `3fbf2ec2`（含 Standards 1 注释笔）。
- 门禁在 `3fbf2ec2` 上实跑：`tsc -b --noEmit` 0 / `run-tests` **313** / `vite build` 0 / `gofmt -l` 空 / `go build` 0 / `go vet` 0 / 清点重生成零差 /
  `go test ./internal/architecture/ -count=1` ok / 带 DSN 全量见 tasks.md 本节。
- **第 6 条**：03 已进 main，阻塞解除；由通道 5 从 `origin/main` 起做 `templates/demo.ts` + `pages/template-preview/*` 一笔并 `renderToStaticMarkup` 断言签名顺序，
  评审只看那一笔增量。

### 完成记录（第 6 条 · 通道 5 新会话 · 2026-09-20 17:4x–18:0x · 分支 `mcp5-ux04-6` · 基 main `bd067052` · 码 tip `add1d41c`）

**笔** `9eaba4f4`（第 6 条，原 `5bb34bbf` 基 `3fbf2ec2`，推送方簿记笔 `bd067052` 进 main 后 rebase 到其上，内容一字未改）→ `add1d41c`（评审 Standards 2
顺手笔，见下）→ 本票面笔。每笔 push origin；不推 main、不占 55432、不跑全仓。只碰 `pages/template-preview/TemplatePreviewPage.tsx`、新
`pages/template-preview/ObjectWorkspaceDemo.tsx`、`templates/demo.ts`、`templates/workspace-tabs.ts`（Standards 2 那一笔）。`ListPageTemplate.tsx` /
`DetailPageTemplate.tsx` / `Layout` / `App` / `index.css` / `navigation.ts` / `templates/index.ts` 一个没碰。

**演示的形状**

- 预览页 `Tabs` 加一签「对象工作区」（列表模板 / 详情模板之后、复核工作流之前），签内先一条说明栏（签为什么按固定序、三签为什么不出、头区为什么没有负责人 / 风险、
  右侧上下文只在宽屏），再渲染 `ObjectWorkspaceDemo`，四态切换器照旧共用。「详情模板」那一签保留为同一模板不传新 prop 的叠 Card 形态，两签并排即「同一模板两形态」。
- `ObjectWorkspaceDemo` 抽成独立组件而不写进预览页：让探针渲染的就是页面渲染的那棵树，不另拼一份「像页面」的 props；不取 `useToast`，快速动作反馈走 `onDemoAction`
  回调（预览页接成 toast），探针因此不必套 `ToastProvider`。
- 传给模板的：`identifier` 复用详情演示那份申报单号；`status` 待人工复核；`headerActions` 两个 `Button`（主 / 次要动作，点了只 toast）；`meta` 四格
  （提交时间 / 系统接收时间 / 来源 / 当前提交版本，与首用页同形）；`summary` 四格（声明包裹件数 / 此前版本数 / 目的国/地区 / 提交批次——都是对象自己陈述的事实，
  无派生 KPI）；`basicFields` / `sections` / `auditTrail` 复用 `demoDetailFields` / `demoDetailSections` / `demoDetailAuditTrail`；`tabs` 三签按
  **审计 → 关联 → 概要** 传——审计与概要各接一张调用方卡（演示同 id 归并、模板内容在前），关联给一张五行关联对象表带 `count: 5`；`aside` 一张「上下文（演示）」卡三格事实。
- `demo.ts` 新增一节「DetailPageTemplate 对象工作区演示数据」：`demoWorkspaceIdentifier` / `demoWorkspaceMeta` / `demoWorkspaceSummary` / `demoWorkspaceRelated`
  （+ `DemoWorkspaceRelatedObject`）/ `demoWorkspaceAsideFacts`，全部 `SYN-` 前缀或「（演示）」后缀，不携带 JSX。

**完成判据逐条（本笔）**

- 四道门 ✅ 两笔各跑：`tsc -b --noEmit` 0 / `run-tests` **313**（与 main `3fbf2ec2` 同数，本笔零新增用例——要钉的纯逻辑第 3 条已钉，本笔全是组件层）/
  `vite build` 0 / `go test ./internal/architecture/ -count=1` ok（新文件无路径字面量）。
- `renderToStaticMarkup` 断言 ✅ **一次性实测、组件层未钉**：照第 1–5 条那套 esbuild 束（`node_modules/.pnpm/esbuild@0.25.12/…/bin/esbuild --bundle --platform=node
  --format=cjs --jsx=automatic --loader:.css=empty`，探针源放 `%TEMP%`、以 `NODE_PATH` 指向本包 `node_modules` 解 `react-dom/server`，源与产物不入库），渲染
  `ObjectWorkspaceDemo` ready 与 loading 两态，**28 断言 / 0 失败**：头区含主编号 `SYN-PS-240819-0002`、`aria-label="复制主编号"`、状态词、两个快速动作文案；
  元信息行四格各在（按 `<dt class="text-idpxyz-textMuted">` 逐格核）；全文无「负责人」「风险」；指标带 **4 格**（按 `SummaryStrip` 标签行 class 计数）且四标签各在；
  `role="tab"` **恰三个**，序为 **概要 < 关联 < 审计**（调用方传的是 审计 → 关联 → 概要）；关联签文本为「关联5」（带计数）；时间线 / 异常 / 文档三词不在任何签上；
  概要签内「基本信息」在调用方「演示说明」之前；非当前签内容未挂载（`SYN-PARCEL-0002-1` 不在静态标记里）；`<aside` 在且含「上下文（演示）」；loading 态无
  `role="tab"`、无指标带、头区主编号仍在。
- `vite build` 产物 grep ✅ 见上文完成判据条目（以 node 按 UTF-8 读产物核，不走 PowerShell 管道——workflow.md 本机环境那条编码坑）。
- 浏览器验收**未验**（本机无 gk.idp.xyz 会话）。

**评审 Standards 2 顺手笔 `add1d41c`**（推送方处置写「可以另起一笔，不要求」）：`workspace-tabs.ts` 的 `WorkspaceTabId` 改为
`(typeof workspaceTabOrder)[number]`，六个 id 只在常量数组写一遍、去掉 `satisfies readonly WorkspaceTabId[]`（否则循环引用）；`workspaceTabLabels: Record<WorkspaceTabId, string>`
仍要求六键齐全。导出面不变；tsc 0 / run-tests 313。

**判断项**

- **指标带四格而非五六格**：多一格只能是「来源请求键」这类标识或从关联表数出来的计数，前者不是指标、后者是派生 KPI；4 落在票面 4–6 之内，auto-fit 栅格一排占满。
- **summary 未放带徽章的格**：`DetailSummaryStat.value` 收 `ReactNode` 的能力由首用页与第 1–5 条探针已证，演示页再放一格状态词会与头区 `status` 同词重复。
- **审计 / 概要两签各接一张调用方说明卡**：不接的话「三签乱序传」只剩关联一签是调用方的，乱序无从演示；卡的正文只说自己是什么（调用方同 id 内容、接在模板内容之后），不编业务。
- **说明栏放在签内、模板之上**，与 03 列表签的「结构位」说明栏同位同字号；不塞进 aside——aside 窄屏藏，说明该在任何宽度都看得见。
- **横幅「三个页面模板」未改**：对象工作区是 `DetailPageTemplate` 的另一形态，模板仍是三个。
- spec 子票表 04 那一行未动（索引文件，推送方进 main 时连「已进 main」一并改）。
