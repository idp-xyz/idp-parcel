# 03 对照现有实现复核仓配履约文档

Category: docs
Status: in-progress
Blocked by: 01, 02

依据：用户 2026-11-06 答「请你结合现在的实现复核吧」，对象是随票 01、02 进 main 的仓配履约 CONTEXT、ADR-0370 与几处邻接文档。仓配履约本身没有代码，复核对照的是它要接的邻接上下文的现有实现。代码取证于 `4286c140d`（origin/main），外加票里点名的文件与符号；下文的计数都是这一提交上的。

## 复核结果

改了的：

1. **委托接受是异步的，仓配履约怎么得知没写。** UC-PS-001 写明提交编排「只答生产归属与`已提交`」，接受链由「委托已提交」信封驱动、异步形成。小包托运的接受决定信封 `parcel-shipment.acceptance-decision.formed`（`internal/parcelshipment/adapters/postgres/acceptance_decision_handoff.go`）带决定后的委托状态，接受与拒绝共用这一类型，网络与路由（`internal/networkrouting/adapters/inbox/acceptance_consumer.go`）与全程追踪（`internal/visibilityexception/adapters/inbox/acceptance_decision_consumer.go`）都在消费。但交这封信封的只有接受链形成决定那一步（`internal/parcelshipment/application/form_acceptance_decision.go` 的 `handOffDecision`）：主动拒绝与授权处置的依赖（`RejectShipmentRequestDeps`、`DisposeShipmentRequestDeps`）里没有交接端口，撤回也不发（`handOffDecision` 头注「没有决定可交时不发意图（例如已撤回的委托）」）。改：
   - 仓配履约 CONTEXT 补一条：本上下文按这封信封推进装箱记录，不读委托的内部状态。接受链形成的决定，以及运营主动拒绝、授权处置选拒绝形成的拒绝，都经它交出。撤回与取消以小包托运对仓配履约那次请求的答复为准。
   - 生产归属落在别的权威或准入暂停时按委托未接受办；归属未决时原样重发，由小包托运按 ADR-0266 续办。准入暂停不在重判之列：ADR-0266 写明它答「`已有结果`带原归属结果与门禁原因，不重判、不再交接」。
   - CONTEXT-MAP 关系图加小包托运到仓配履约的边，「关系约束」加对应一条。
   - 两种拒绝也经这封信封交出，是替小包托运的出向定的目标（会话代决，常设授权为用户原话「你全权」），待 PS owner 复核。差距记进 UC-PS-001「给开发的交接」。这封信封是交给下游的发布意图，ADR-0086 决定三不许主动拒绝再发的是续办信封，两者不相抵。
2. **面单本体取不取得到。** [ADR-0092](../../../docs/adr/0092-channel-label-payload-lands-as-a-reference-and-the-body-store-is-an-unconfigured-outbound-seam.md) 定渠道面单在领域与库里只落引用，本体存放是一条未配置的出向缝。仓配履约要打印面单，取不到本体时答未配置、不出库确认，写进 CONTEXT 与那条关系约束。
3. **结算没有按仓储作业收费发生项发起的 SELL 路径。** 评价请求分 BUY 与 SELL 两册（`internal/settlementaccounting/domain/evaluation_resumption.go` 的 `EvaluationDirection`）。BUY 按运输收费发生项（`evaluation_request.go`「合格来源引用三件」），SELL 按包裹与计费时点（`sell_evaluation_request.go` 的 `SellEvaluationRequestNaturalKey`，ADR-0190）。结算 CONTEXT 的费用来源随 ADR-0368 接受加了仓储作业收费发生项，差距却没进任何 UC。改：UC-SA-002「给开发的交接」补一行。
4. **以货主名义撤回或取消委托的请求方。** 出库截单与预取面单的改提（ADR-0370 决定二）要撤回或取消已提交的委托，与资料修订一样是小包托运没有的请求方。改：UC-PS-005、UC-PS-006「给开发的交接」各补一行（资料修订那一行已在 UC-PS-002）。
5. **客户原始资料里货物与申报的条目名。** 小包托运只给地址要素（`internal/parcelshipment/domain/address_element.go`）与声明测量（`declared_measurement_resolution.go` 的 `DECLARED_MEASUREMENT` 资料组）定了名，货物与申报是通用的名值条目。仓配履约交来的商品明细与申报值落在哪些条目，要随仓配出库那条路径定，并与关务读申报资料的口径对齐。改：UC-PS-001 仓配出库那一行补上。
6. **仓配的单据没有单据号。** 各上下文在 `document_number_types.go` 里登记自己的单据类型（如小包托运的委托、节点作业的送站预报与送站交接单），仓配履约 CONTEXT 没提单据号。改：
   - 逐类写开立时点：入库预报进入已预报、出库请求进入已受理、退货预报经客户渠道登记时，在同一事务内取得单据号。
   - 拆单拆出的出库请求是新单据，各取新号（ADR-0355 决定二、三）。
   - 号里的日期按仓的业务时区，没登记时按租户的单据编号时区，与送站预报按网点时区、没有就按租户时区的做法一致。
   - 类型代码与内置默认模板（ADR-0355 决定五、六）留给仓配切片的实现票声明。

核过、不用改的：

- 小包托运的「来源」是开放的必填串（`internal/parcelshipment/domain/identity.go` 的 `NewSource`），来源「仓配出库」不需要改模型。
- 声明测量是一个可以资料修订的资料组（`internal/parcelshipment/adapters/http/customer_amendment_intake.go` 认 `DeclaredMeasurementDataGroup`），预取面单打包后补正有路可走；放不放行由资料修订允许矩阵定，CONTEXT 已写更正不放行就撤回改提。
- 全程追踪的事实类型在代码里是开放集合（`internal/visibilityexception/domain/tracking_projection.go` 的 `NewSourceFactKind`），新类型词靠各来源的适配器接入，差距已记在 UC-VE-002。
- 小包计价的评价对象是四种（`internal/parcelpricing/domain/input.go` 的 `EvaluationSubjectKind`），仓储作业收费发生项不在其中，与 CONTEXT「在那之前小包计价的评价对象不含它」一致。
- 参考配置经普通登记采用已有先例（`internal/networkrouting/adapters/registrationjson`），分配策略的参考配置照此做。
- `apps/` 下只有管理台，一线作业端还没有应用；仓内作业进作业端是产品要求，不是现状描述。

## 完成判据

- 每一处改动都能指到上面一条复核结果；只改 `.md`，不动 `internal/**`。
- 进 main 前要一份非作者评审；起不来时自审并如实写明。

## 完成记录（2026-11-06）

- 仓配履约 CONTEXT 补接受决定信封、撤回与取消的答复、生产归属三种答法、面单本体与单据号；CONTEXT-MAP 加小包托运到仓配履约的边与关系约束。
- UC-PS-001 补两行（货物与申报的条目名并进仓配出库那一行，另起一行记两种拒绝不交信封），UC-PS-005、UC-PS-006、UC-SA-002 各补一行。
- 只改 `.md`，不动 `internal/**`。

## 评审记录

- 2026-11-06 非作者评审（子代理，钉 `93392eb90`）：阻断 1、非阻断 6。处理如下，改在 `cursor/wf-impl-review` 上：
  - 阻断：「接受或拒绝……经接受决定已形成的信封交出」对主动拒绝与授权处置选拒绝不成立，撤回也没有信封。按评审改法 (a) 改为目标规则，差距记进 UC-PS-001，撤回与取消以答复为准（复核结果 1）。
  - 非阻断：
    - 生产归属补上准入暂停与归属未决的重发。没照评审写的「准入暂停也原样重发重判」：ADR-0266 写明准入暂停不重判，按委托未接受办。
    - UC-SA-002 去掉「两路」计数。
    - 「面单正文」改为「面单本体」，写明面单交易只落引用。
    - 单据号逐类写开立时点，补拆单取新号。
    - 规则句里点名的消费方删去，证据留在本票。
    - UC-PS-005 改「撤回请求的请求方」，UC-PS-006 补引 ADR-0370 决定二。
