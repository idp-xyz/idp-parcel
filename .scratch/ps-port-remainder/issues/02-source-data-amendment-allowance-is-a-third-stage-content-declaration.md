# `SourceDataRuleDeclaration`：允许矩阵是接单规则包版本下的第三族阶段内容声明；PS 要先长出「资料修订阶段」

Category: enhancement
Status: in-progress——四问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments）；**PC 半边（第三族阶段内容声明表族 + 读口）等 pc-gaps 批（MCP-3）**；**PS 半边拆两段**：不依赖 PC 的那段（词条「资料修订阶段」进 CONTEXT、领域 `AmendmentStage`、`SourceDataAmendmentQuery.Stage`、编排问矩阵前先判阶段、CC/NO 消费侧读口 + ADR-0118）由通道 2 按 task-b77525c9 ③ 在分支 `mcp2-ps-ports` 实施中；依赖 PC 的那段（消费适配器读声明）Blocked by PC 半边
Blocked by: 消费适配器那段 Blocked by PC 半边；其余不阻

## 端口今天说什么

`ports.SourceDataRuleDeclaration.DeclareSourceDataAmendment(ctx, SourceDataAmendmentQuery{Identity, Scope, Intent, Reason}) (SourceDataAmendmentAllowance, error)`，三值 `NotDeclared`（零值，最保守格）/ `Allowed` / `Disallowed`。头注：「字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例半边；本上下文只消费登记结论，绝不自带一份矩阵。」消费方 `AmendCustomerSourceDataHandler`：只有 `Allowed` 才形成版本；`Disallowed` → 业务拒绝；`NotDeclared` → `AmendmentAwaitingReview`（「还没人说这能不能改」）。仓内无生产实现，只有 `sourceDataRuleDouble`；编排本身在 `cmd/` 无调用方。

**查询里没有「阶段」。** 而 UC-PS-002「资料范围与阶段边界」那张表按业务阶段分六行，`BD-PS-010` 的建议是「版本化阶段矩阵；未登记的字段和阶段不默认允许」，PS CONTEXT 硬句是「显式清空必须由版本化**字段和阶段**规则允许」。矩阵没有阶段这一维就登记不了 UC 写的任何一行——这是端口形状上的缺口，不只是实现缺席。

## 语言从哪里来

- UC-PS-002 六个阶段（原词）：已接受、尚未收寄 / 已收寄或已测量 / 已制签或已装袋 / 关务资料形成中、尚未提交 / 已提交关务 / 案件已关闭或服务已完成。它们跨三个上下文的事实：接受、收寄采用、面单交易、包裹终局归 PS；装袋归 NO；关务三格归 CC。
- 矩阵的另两维在 PS 领域已有：`SourceDataScope`（委托 + 可空包裹 + `SourceDataGroupReference` 资料组）与 `AmendmentIntent`（补充 / 更正 / 显式清空，`AT-PS-020` 要求意图进矩阵）。
- PC CONTEXT Rules：「接单规则包必须……区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及跨字段条件」——规则包已经在按**资料组**说话；「规则包只装配各权威上下文的规则引用」——线路（NR）、关务（CC）那部分规则由规则包引用，不由规则包拥有正文，这与 `PAR-COM-13` 说的「与真实合同、产品、线路和关务规则登记」对得上。
- ADR-0058：阶段内容声明（收寄资格、终局规则、取消授权）按拥有规则对象归属，产品与合同只采用；无父行 = 未配置；消费侧不得从身份发明采用版本。0013 已有两族挂在接单规则包版本下，PS 消费侧 `DeclaredStageContent` 已把它们接进翻译适配器。
- UC-PS-002 元数据：「业务决定所有者：`parcel-shipment`；字段与阶段适用性依据来自已登记规则」——决定归 PS，规则归登记方。

## 裁决（通道 2，2026-09-07；owner 授权自决口径；取证锚远端 main `ffa6bd0e`）

**先答归属：矩阵正文归 party-commercial，作接单规则包版本下的第三族阶段内容声明；「当前处于哪个阶段」这一判断归 parcel-shipment。** 理由三条：`PAR-COM-13` 本就是商业参数行，登记责任写的是「客户接入、托运业务、商业、关务、节点运营、结算」共同出证，规则包正是「装配各权威上下文的规则引用」的那个对象；ADR-0058 已经把同类的「某阶段允许什么」（收寄资格、终局规则）判给规则包版本，第三族不另起归属；PS 若自建矩阵，就与端口头注「绝不自带一份矩阵」、与 UC「依据来自已登记规则」两处相违。反过来，**阶段是 PS 对自己对象生命周期位置的判断**，规则包只声明「在某阶段允许什么」，不判断「此刻在哪个阶段」——判断权在消费方，与收寄资格「PC 声明硬资格、PS 取证判断」的分工同形（ADR-0063）。

**① 机制半边现在能立什么。** 两半，先后有序：

- **PC 半边（先）**：第三族阶段内容声明「**资料修订允许声明**」，拥有对象 = 接单规则包版本（`ACCEPTANCE_RULE_PACKAGE`，形照 0013 的 `intake_qualification_*` / `final_rule_*`：父行一条 + 子行若干，主键 = 租户 + 拥有版本四元组 + 声明格）。子行的格：（资料组引用 × 修订阶段 × 修订意图）→ 允许 / 不允许。无父行 = 未配置（消费方译 `NotDeclared`）；父行在而子行坏 = error。读口 `pcports.SourceDataAmendmentAllowanceView`（名字随 PC 惯例定），发布随 `PublishCommercialAuthorityHandler` 加一条通道。**缺格语义**见问题 3。
- **PS 半边（后，三件）**：(a) CONTEXT 新词条「**资料修订阶段**」（提议，见下）+ 领域封闭集 `AmendmentStage`，取 UC-PS-002 六格原词；(b) `SourceDataAmendmentQuery` 加 `Stage`，由编排在问矩阵前判出：PS 自有事实（接受基线在、收寄采用有无、面单交易结果有无、包裹终局有无）本上下文直接读，装袋（NO）与关务三格（CC）是跨上下文输入——**输入缝的形状本票不裁**（随信封携带照 ADR-0075，还是消费侧读口照 ADR-0025，见问题 4）；判不出阶段时编排停未决，不猜一个阶段去问矩阵；(c) 消费适配器照 `DeclaredStageContent`：`AdoptedStageOwner.AcceptanceRulePackageFor` 回指接受时固定的规则包版本 → 读声明 → 查（资料组, 阶段, 意图）那一格 → 译三值。

**② 实例半边留什么、在哪一格拒默认。** 矩阵里每一格的取值、资料组的划分、哪些阶段对哪些资料组开放，全是 `PAR-COM-13` / `BD-PS-010` 待提供；拒默认落在 `NotDeclared` 零值（已有）与「判不出阶段则未决」（新增，不得默认成「已接受、尚未收寄」）。

**③ 形状照哪个先例。** ADR-0058（归属、分表分读、无父行=未配置）、0013 两族表、`DeclaredStageContent` + `AdoptedStageOwner`（消费侧）、ADR-0063（声明与取证分工）。

**④ 要不要 ADR。** 归属不要——ADR-0058 决定一的表加一行即可（它自己写「三件」，第四件按同一条纪律归规则包版本，PC owner 落地时在 ADR-0058 Consequences 或新 ADR 记一句由其自裁）。**阶段判断的跨上下文输入缝可能要一篇**：CC 申报阶段进 PS 是新的一条 CC → PS 缝，CONTEXT-MAP 今天没有这条箭头（有 PS → CC）；建模到那一步再判。

### 提议的 PS CONTEXT 词条（未写入，等你点头）

> **资料修订阶段** 一份已接受委托或其包裹在资料补充/更正请求到达时所处的生命周期位置，是判断「此刻允许改什么」的维度之一。封闭六格：已接受尚未收寄、已收寄或已测量、已制签或已装袋、关务资料形成中尚未提交、已提交关务、案件已关闭或服务已完成。阶段由 `parcel-shipment` 按自有事实与邻接上下文交出的事实判断，判断不成时修订请求停在待复核，不得默认为最早阶段；允许什么由所采用接单规则包版本的资料修订允许声明说。

## 要你答的问题

1. **归属确认**：矩阵正文归 PC（接单规则包版本下第三族声明）、阶段判断归 PS——同意吗？替代是 PS 自建矩阵登记册，代价是与端口头注、UC「依据来自已登记规则」和 ADR-0058 三处相违，且关务/线路那部分规则要由 PS 替它们登。
2. **阶段封闭集**就取 UC-PS-002 的六格原词？「已制签或已装袋」把 PS 事实（面单结果）与 NO 事实（装袋）合在一格，是照 UC 保留还是拆开？我的倾向：照 UC 保留，拆是规则出现后的事。
3. **缺格语义**：声明父行在、但（资料组, 阶段, 意图）那一格没有子行——是 `NotDeclared`（待复核）还是 `Disallowed`（拒绝）？我的倾向：**默认 `NotDeclared`**，另给父行一个「封闭」标记，标了封闭的声明缺格才读作 `Disallowed`——「未登记的字段和阶段不默认允许」两种读法都满足，差别在客户看到的是「等复核」还是「被拒」，那是登记方该显式说的。
4. **关务与装袋两处阶段事实怎么进 PS**：随 CC/NO 的信封携带过界（ADR-0075 对 NR 的裁法），还是 PS 建消费侧读口（ADR-0025）？这决定阶段判断能不能同步完成；答完再定要不要 ADR。

## 红线

- 不内置任何矩阵格；`NotDeclared` 仍是零值；判不出阶段不猜。
- 不改 `AT-PS-020`、UC-PS-002 阶段表与 PS CONTEXT 硬句；词条只作提议。
- PC 半边不在 PS 地盘动手。

## 参照

`ports.SourceDataRuleDeclaration` / `SourceDataAmendmentQuery` / `SourceDataAmendmentAllowance` 头注；`application/amend_customer_source_data.go` 矩阵那一格；`domain/customer_source_data.go` 的 `SourceDataScope` 与 `AmendmentIntent`；`adapters/partycommercial/stage_content_declarations.go`；`migrations/party_commercial/0013_stage_content_declarations.sql`；ADR-0058、ADR-0063、ADR-0062、ADR-0025、ADR-0075；UC-PS-002「资料范围与阶段边界」「待确认业务决策」（`BD-PS-010`）；`PAR-COM-13`。

## Comments

- 2026-09-07 · 通道 2：立票（draft），一次 `/domain-modeling` 的产物。**只写票面，未动代码，未改 CONTEXT。** 能力边界：读过端口与编排、PS 领域两型、PC 0013 表名与 `DeclaredStageContent`、ADR-0058 全文、UC-PS-002 全文、PC CONTEXT 相关句；**没读** CC/NO 侧今天有哪些能答「阶段」的读口——问题 4 因此留给建模那一步。
- 2026-09-07 · MCP-1 代裁，owner 授权（task-b77525c9，由通道 2 落票面）：**Q1** 同意——矩阵正文归 PC 作第三族阶段内容声明，阶段判断归 PS；**Q2** 阶段封闭集照 UC-PS-002 六格原词，不拆；**Q3** 缺格默认 `NotDeclared`，父行带「封闭」标记时缺格读 `Disallowed`；**Q4** CC/NO 阶段事实进 PS **取消费侧读口（ADR-0025 形）**——阶段是修订请求到达那一刻同步要问的，不是事件便车能保证齐全的；PS 立 `ports.*StageView` 一类读口，适配器读 CC/NO 已有读面，读面不存在或未接就是「判不出阶段 → 未决」；它给 CONTEXT-MAP 加 CC→PS、NO→PS 两条消费箭头，**要 ADR，取预留号 0118**；建模时以 CC/NO 代码为准，若发现既有信封已携带阶段事实且同步性不成问题，可在 ADR 里改选并写理由。上面「提议的 PS CONTEXT 词条」按 Q2 进 CONTEXT。Status 由 draft 改 in-progress（PS 不依赖 PC 的那段开工）。
