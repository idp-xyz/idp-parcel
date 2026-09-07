# ADR-0118: 「资料修订阶段」由 parcel-shipment 判断，关务与装袋两处阶段事实经消费侧读口同步进入——customs-compliance → parcel-shipment、node-operations → parcel-shipment 两条消费缝；读面未接即判不出阶段、停在未决

Status: Accepted  
Date: 2026-09-07

> 裁决授权：owner 于 2026-09-07 授权 MCP-1 代裁票 [ps-port-remainder/02](../../.scratch/ps-port-remainder/issues/02-source-data-amendment-allowance-is-a-third-stage-content-declaration.md) 四问（记在该票 Comments），本记录把其中 Q2（六格原词）与 Q4（跨上下文输入缝取消费侧读口、要 ADR）落成决策；Q1（矩阵正文归 `party-commercial`）与 Q3（缺格语义）归 PC 半边落地时另记，不在此。

## Context

`UC-PS-002`「资料范围与阶段边界」按业务阶段分六行写允许结果；`parcel-shipment` CONTEXT 硬句说「显式清空必须由版本化**字段和阶段**规则允许」；`BD-PS-010` 的建议是「版本化阶段矩阵；未登记的字段和阶段不默认允许」。而 `ports.SourceDataAmendmentQuery` 此前只有范围与意图两维，没有阶段——矩阵登记不了 UC 写的任何一行，这是端口形状上的缺口，票 02 已裁：矩阵正文归 `party-commercial`（接单规则包版本下第三族阶段内容声明，照 ADR-0058），**「此刻在哪个阶段」的判断归 `parcel-shipment`**（与 ADR-0063「PC 声明硬资格、PS 取证判断」同一分工）。

六格里的事实跨三个上下文：接受基线、有效网络收寄采用、面单交易包裹结果、终局服务结果归 PS；装袋归 `node-operations`；申报资料形成、提交、案件关闭归 `customs-compliance`。CONTEXT-MAP 此前有 `node-operations → parcel-shipment`（节点收寄、实测与物理拆合事实）与 `customs-compliance → parcel-shipment`（关务限制及解除结果）两条边，都不覆盖这几件事实；PS 的 `adapters/` 下没有 `customscompliance` 目录。

两处输入缝的形状有两种先例可照：随提供方的信封携带过界（ADR-0075 对客户地址的裁法，事件便车），或消费侧读口加适配器（ADR-0025）。分岔点是同步性：阶段是修订请求**到达那一刻**要问的，问的是「此刻」——事件便车能保证的是「某件事发生过我会知道」，保证不了「我此刻已经知道了全部发生过的事」，而阶段判断恰恰要排除靠后的格。

取证于 `main = 08f54867`（逐符号名）：`customs-compliance` 的案件键 `ports.CustomsCaseKey` 是（租户、管辖、方向、程序、义务范围），申报单元 `domain.DeclarationUnit` 记成员 `DeclaredParcelReference` 却没有按包裹反查的口（`DeclarationUnitStore` 头注写明「今天没有消费方，端口不预设方法」）；`node-operations` 的 `ports.ContainmentIndex.CurrentParent` 按作业实物 `HandlingUnitID` 答直接父级，正式包裹与作业实物之间是识别成功后建立的版本化关联，PS 不推断它（`adapters/nodeoperations` 的 `ErrUnidentifiedHandlingUnit` 那一路就为此拒绝）。也就是说，**两处今天都没有按正式包裹键的已导出读面**。

## Decision

**一、「资料修订阶段」是 `parcel-shipment` 的词条与判断，六格取 `UC-PS-002` 原词，靠后的格压过靠前的格。** 领域封闭集 `AmendmentStage`（零值`判不出`，六格进枚举门禁）；判断函数 `JudgeAmendmentStage` 收六格三态事实（`StageFact`：不知道 / 不在 / 在），从最后一格往前看，第一格`在`即阶段，途中任何一格`不知道`即判不出。次序照 CONTEXT「已收寄、制签、装袋、运输、申报、提交、案件关闭或服务完成后」那一句，不另立。作用于整份委托的范围按接受基线成员逐件判、取最靠后一格（`FurthestAmendmentStage`），任一成员判不出即判不出。两格各由两个上下文出半边（「已制签或已装袋」「案件已关闭或服务已完成」）按「任一在即在、两半都不在才不在、其余不知道」并成（`EitherStageFact`）——一边已知`不在`替不了另一边作答。

**二、阶段随查询进矩阵，且在问矩阵之前判出；判不出停在未决，不默认最早阶段。** `ports.SourceDataAmendmentQuery` 加 `Stage`；`AmendCustomerSourceDataHandler` 在步骤 5（接受基线核验）之后、步骤 6（矩阵）之前判阶段，判不出交回 `SourceDataAmendmentUndecided` 携封闭原因 `SourceDataAmendmentStageUndetermined`（事实里有`不知道`，等读面接上）；某个事实口调不通交回 `SourceDataAmendmentStageFactUnavailable`（等依赖恢复）。两格分开的理由与矩阵那两格（`SourceDataRuleUnavailable` 对 `NotDeclared`）一字不差。矩阵实现方收到零值 `Stage` 应拒答，不当最早阶段查——测试替身 `sourceDataRuleDouble` 已照此拒答。委托未接受时上抛 `ErrShipmentRequestNotAccepted`，与后面 `AmendCustomerSourceData` 那道门同一答案，只是早说。

**三、关务与装袋两处阶段事实经消费侧读口同步进入 PS（ADR-0025 形），不走事件便车。** PS 立两个读口：`ports.CustomsStageView`（交 `CustomsStageFacts`：形成中 / 已提交 / 已关闭三格三态）与 `ports.ConsolidationStageView`（交装袋一格三态）；读口只交事实不交阶段，邻接上下文只出它拥有的那几件事。适配器落在 `internal/parcelshipment/adapters/customscompliance`（新目录）与 `adapters/nodeoperations`，只翻译不判断。PS 自有的三项走既有登记册的读半边（`ResponsibilityStartView`、`LabelTransactionsByParcelView`、`CurrentFinalView`），本上下文自己的册子读回来了就是知道，没有`不知道`这一格。

**四、读面不存在或未接，适配器如实答`不知道`；缺口归提供方立票，PS 不绕过已导出端口去读表。** 今天两只生产适配器是 `UnconnectedCustomsStageView` 与 `UnconnectedConsolidationStageView`，一律答`不知道`、不读询问——`不知道`不是`不在`：答`不在`会让一个没接读面的关务把每个包裹都判成「尚未申报」，最早阶段就此成了默认值，正是 `BD-PS-010` 在阶段这一维上明禁的事。`customs-compliance` 要立「按正式包裹反查申报单元 / 提交 / 案件关闭」的读面，`node-operations` 要立「按正式包裹答当前所在集运单元」的读面（经版本化关联），两处缺口记在 [ps-port-remainder/05](../../.scratch/ps-port-remainder/issues/05-customs-and-node-operations-need-parcel-keyed-stage-fact-read-faces.md)，由各自 owner 认领；读面立起来时只换装配点上那一只适配器，端口、编排、判断函数都不动。

**五、CONTEXT-MAP 记两条消费缝。** `customs-compliance → parcel-shipment` 在既有「关务限制及解除结果」之外加「申报资料形成、提交与案件关闭的阶段事实」；`node-operations → parcel-shipment` 在既有「节点收寄、实测与物理拆合事实」之外加「装袋事实」。方向都是提供方出事实、消费方判断，不改任何所有权。

## Consequences

- **今天的生产装配在授权之后必然停在`判不出阶段`。** 两只未接适配器让每次修订在问矩阵之前停下——这是如实的：在关务与节点作业按包裹键的读面立起来之前，PS 无从排除「这个包裹其实已经提交了关务」。停点从「矩阵未登记」往前挪了一格，`cmd/parcel-api` 的装配用例照此改写（授权过了停在判不出阶段；阶段已知才停在矩阵未登记）。
- **提供方两条缺口是本记录的前置而不是它的一部分。** 票 05 立起来是为了让「读面接上」有落点；接上之前，CC/NO 的任何既有信封都不被拿来推断阶段。若日后取证发现某条既有信封确实同步携带阶段事实且不存在「到达那一刻尚不齐全」的问题，可对那一格改选事件便车，改选要另记（本记录 Context 已写下判断依据是同步性）。
- **「靠后压过靠前」是本记录写死的读法。** 它意味着矩阵登记方（`PAR-COM-13`）只需对每个阶段登记一行，不需要登记「同时处于两格」的组合；也意味着委托级资料的允许性由最靠后的成员决定。若 owner 要的是相反方向（按最早成员放宽），改的是决策一，不是判断函数的某个分支。
- **UC-PS-002 与 `AT-PS-020` 一字未动。** 阶段表六行的原词进了封闭集，表本身不改；未决结果的结果语义契约已有「规则或授权无法确定」一格，`判不出阶段`落在它里面。
- **矩阵那半（PC 第三族声明、缺格语义 Q3）与消费适配器那半仍归票 02 的 PC 侧。** 本记录只把阶段这一维长出来并接到查询上；`UnconfiguredSourceDataRuleDeclaration` 仍答 `NotDeclared`，与阶段无关。
