# 03 对照现有实现复核仓配履约文档

Category: docs
Status: in-progress
Blocked by: 01, 02

依据：用户 2026-11-06 答「请你结合现在的实现复核吧」，对象是随票 01、02 进 main 的仓配履约 CONTEXT、ADR-0370 与几处邻接文档。仓配履约本身没有代码，复核对照的是它要接的邻接上下文的现有实现。代码取证于 `4286c140d`（origin/main），外加票里点名的文件与符号。

## 复核结果

改了的：

1. **委托接受是异步的，仓配履约怎么得知没写。** UC-PS-001 写明提交编排「只答生产归属与`已提交`」，接受链由「委托已提交」信封驱动、异步形成。小包托运已有接受决定意图的信封 `parcel-shipment.acceptance-decision.formed`（`internal/parcelshipment/adapters/postgres/acceptance_decision_handoff.go`），网络与路由（`internal/networkrouting/adapters/inbox/acceptance_consumer.go`）与全程追踪（`internal/visibilityexception/adapters/inbox/acceptance_decision_consumer.go`）都在消费。改：仓配履约 CONTEXT 补一条消费同一封信封、不读委托内部状态，生产归属落在别的权威时按委托未接受办；CONTEXT-MAP 关系图加小包托运到仓配履约的边，「关系约束」加对应一条。
2. **面单正文取不取得到。** [ADR-0092](../../../docs/adr/0092-channel-label-payload-lands-as-a-reference-and-the-body-store-is-an-unconfigured-outbound-seam.md) 定渠道面单在领域与库里只落引用，本体存放是一条未配置的出向缝。仓配履约要打印面单，取不到正文时答未配置、不出库确认，写进 CONTEXT 与那条关系约束。
3. **结算没有按仓储作业收费发生项发起的 SELL 路径。** 结算的评价请求只有按运输收费发生项的 BUY（`internal/settlementaccounting/domain/evaluation_request.go`「合格来源引用三件」）与按包裹计费时点的 SELL（ADR-0190）两路；结算 CONTEXT 的费用来源随 ADR-0368 接受加了仓储作业收费发生项，差距却没进任何 UC。改：UC-SA-002「给开发的交接」补一行。
4. **以货主名义撤回或取消委托的请求方。** 出库截单要撤回或取消已提交的委托，与资料修订一样是小包托运没有的请求方。改：UC-PS-005、UC-PS-006「给开发的交接」各补一行（资料修订那一行已在 UC-PS-002）。
5. **客户原始资料里货物与申报的条目名。** 小包托运只给地址要素（`internal/parcelshipment/domain/address_element.go`）与声明测量（`declared_measurement_resolution.go` 的 `DECLARED_MEASUREMENT` 资料组）定了名，货物与申报是通用的名值条目。仓配履约交来的商品明细与申报值落在哪些条目，要随仓配出库那条路径定，并与关务读申报资料的口径对齐。改：UC-PS-001 仓配出库那一行补上。
6. **仓配的单据没有单据号。** 各上下文在 `document_number_types.go` 里登记自己的单据类型（如小包托运的委托、节点作业的送站预报与送站交接单），仓配履约 CONTEXT 没提单据号。改：入库预报、出库请求与退货预报在受理的同一事务内取得单据号（ADR-0355），号里的日期按仓的业务时区，没登记时按租户的单据编号时区，与送站预报按网点时区、没有就按租户时区的做法一致。

核过、不用改的：

- 小包托运的「来源」是开放的必填串（`internal/parcelshipment/domain/identity.go` 的 `NewSource`），来源「仓配出库」不需要改模型。
- 声明测量是一个可以资料修订的资料组（`internal/parcelshipment/adapters/http/customer_amendment_intake.go` 认 `DeclaredMeasurementDataGroup`），预取面单打包后补正有路可走；放不放行由资料修订允许矩阵定，CONTEXT 已写更正不放行就撤回改提。
- 全程追踪的事实类型在代码里是开放集合（`internal/visibilityexception/domain/tracking_projection.go` 的 `NewSourceFactKind`），新类型词靠各来源的适配器接入，差距已记在 UC-VE-002。
- 小包计价的评价对象在 `4286c140d` 上是四种（`internal/parcelpricing/domain/input.go` 的 `EvaluationSubjectKind`），仓储作业收费发生项不在其中，与 CONTEXT「在那之前小包计价的评价对象不含它」一致。
- 参考配置经普通登记采用已有先例（`internal/networkrouting/adapters/registrationjson`），分配策略的参考配置照此做。
- `apps/` 下只有管理台，一线作业端还没有应用；仓内作业进作业端是产品要求，不是现状描述。

## 完成判据

- 每一处改动都能指到上面一条复核结果；只改 `.md`，不动 `internal/**`。
- 进 main 前要一份非作者评审；起不来时自审并如实写明。

## 完成记录

## 评审记录

## 进 main 记录
