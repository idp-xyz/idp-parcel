# 04 `DetailPageTemplate` 对齐对象工作区母版：Object Header + Summary Strip + 稳定命名的 Tabs；委托查阅详情页首用

Category: enhancement
Status: in-progress
Blocked by: 03（两票都改 `pages/template-preview/*` 演示页；03 进 main 后再动它。模板本体与首用页不等 03，可先做，演示页那一笔最后补）
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
2. **Summary Strip**：新可选 prop `summary?: { label: string; value: ReactNode; tone?: StatusTone }[]`（4–6 格，`StatCard` 一排），只放对象自身的事实
   （件数、修订号、金额已确认与否这类从读口原样来的值），**不放派生 KPI**——spec「不做」第一条同一理由；不传不渲染，不显「—」占位格。
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
6. ⏳ `template-preview` 演示页：**等 03 进 main**。推送方广播后 rebase 到 origin/main，在 `templates/demo.ts` 与 `pages/template-preview/*` 补一笔，
   `renderToStaticMarkup` 断言签名顺序。

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
- `template-preview` 产物断言 ⏳ 随第 6 条（等 03 进 main）。**组件层未钉**：照抄 05 / 06 实测结论——`.test.ts` 不能 `require` 任何 `import` 了
  `@idpxyz/*` 的模块（`exports` 只有 `types` + `import`，CJS `require` 报 `ERR_PACKAGE_PATH_NOT_EXPORTED`），本票未再试。
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
