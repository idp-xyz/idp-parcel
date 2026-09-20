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
