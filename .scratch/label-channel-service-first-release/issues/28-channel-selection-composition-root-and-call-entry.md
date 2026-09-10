# 28 渠道择优链的组合根与调用入口：`cmd/` 下整条面单渠道写链至今零装配，择优是运营端点还是面单交易编排的前置步

Category: enhancement
Status: draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** 本票只写事实、两条路的代价与「要裁的」；入口归产品流程决定，**留 draft 等 owner**
Blocked by: 无（是决定不是阻塞）。裁定后的实施票是否就是本票、还是按裁定另拆，归派单方

## 事实（`c7e3522c` 上量，逐符号名）

票 [`12`](./12-channel-candidate-assembly.md) 收口 Comment（2026-09-03，MCP-6）写「生产可达仍差两步：组合根 + 调用入口」，并在下一条把两个候选入口各自的代价写完了。本票承接那两步；lc/12 那两条 Comment 是代价分析的**唯一权威**，下面只列今天与那时不同的事实，不复制第二套。

**`cmd/` 下零命中的名字（择优链）**：`SelectChannelCandidate`、`AssembleChannelCandidates`、`NewSelectChannelCandidateHandler`、`NewChannelCandidateAssembler`；**（面单交易写链）**：`NewLabelTransactionHandler`、`EstablishLabelTransaction`、`OperateLabelTransaction`；**（出向缝）**：`outbound`、`LabelFetch`。lc/12 在 `c25d145` 上量的零命中，一周后原样成立。

**`cmd/` 下已经接上的只有两张读面**：`cmd/parcel-api/main.go` 的 `pspostgres.NewLabelTransactionViews`（`/label-transactions`，票 admin-skeleton-closure-batch/08）与 `buildChannelSelectionDecisionRead`（`/channel-selection-decisions`，票 [`23`](./23-channel-selection-decision-operations-read-face.md)）；`unwired_orchestration.go` 里 `unwiredLabelTransactions` / `unwiredChannelSelectionDecisions` 是它们的隔离读占位。读面能看的是一张今天没有任何写入方的表。

**择优编排今天的形状**：`application/select_channel_candidate.go` 的 `SelectChannelCandidateHandler.Handle(ctx, ports.ChannelSelectionQuery{Tenant, Scope, Mapping, At}) (domain.ChannelCandidateID, error)`；`SelectChannelCandidateDeps` 五口：`Assembly`（`adapters/partycommercial/channel_candidate_assembly.go`）、`Costs`（`adapters/parcelpricing/cost_source.go`）、留痕三件 `Decisions` / `DecisionIDs` / `Clock`（票 [`14`](./14-rejected-candidate-trace-object.md)，一起可缺席、只到一半即 `ErrChannelSelectionRecordingMisconfigured`）。它交回的是**一个候选标识**。

**自 lc/12 收口以来变了的三件**：

1. 票 `14`（落选留痕）已 resolved 并进 main——lc/12 乙路「必须先有票 14 的落选留痕，否则并列→冲突→人工裁决那条路在编排里没有落点」这一前置**今天已满足**；并列冲突已能落成 `TIED` 决定记录并在 `23` 的读面上查到。人工裁决动作本身仍「留待 `/domain-modeling`」（`23` 状态行原句）。
2. 票 `23` 读面进了 `cmd/parcel-api`——甲路「择优可独立回放与审计」这一半的读侧已在。
3. 三个取数口（渠道约束、计价输入、`BUY` 价卡）lc/12 收口时量为「今天全未配置」；**本票未重核**那三口的配置状态，开工时以 `cmd/` 装配处为准。

**顺带量到、两路共有的一段缺翻译**：择优交回 `ChannelCandidateID`，而票 `06` 的 `EstablishLabelTransactionCommand` 要七类依据引用（`ChannelAccount` / `AccountHolder` / `ServiceProvider` / `SettlementCounterparty` / `Contract` / `Rate` / `ResponsibilityBasis`）——从「选中了哪个候选」到「这笔交易的七类依据是什么」中间没有适配器。候选来自 PC 产品—渠道映射，七类引用多半也在 PC 那一侧（映射、渠道账号、供应商协议、价卡）；这段翻译落哪、算不算本票，见「要裁的」2。

## 两条路（代价见 lc/12 2026-09-03 两条 Comment，此处只标今天的差）

**甲 · 运营端点**（择优查询口）：lc/12 所列代价照旧——传输层 + 端点表 / 探针 / 放行表三处共享接线 + Intake；三取数口未配置则端点上线恒答「未配置」（那是诚实停点，与 `cmd/parcel-api` 既有受控编排壳同款）。今天多出来的好处：`23` 读面已在，端点写下的每条决定当场可查。它**不**让面单交易写链可达——06 / 07 的组合根仍要另接。

**乙 · 面单交易编排的前置步**（在 `Establish` 之前择优）：lc/12 所列代价里「先有 14」已清；仍要改 06 编排的输入形状（渠道从调用方给改为编排选出），**再加上**「顺带量到」那段候选标识 → 七类引用的翻译。好处：生产可达一步到位——择优 → 建立 → 提交渠道 →（07 出向）→ 记录结果 → 票 `26` 的定案触发，整条链在一个组合根里；坏处：06 编排头注「它不发起任何渠道调用……只留缝」那一格与择优前置的关系要重写，且 06 的调用方形状随之变。

两路都要的：`cmd/parcel-api`（或裁定的进程）里为择优编排与其两个适配器、06 编排、07 出向端口（未配置壳）各立装配函数，照既有 `build*Orchestration` 家族；三取数口按实例半边**显式未配置**装配（照 `UnconfiguredIntakeQualificationEvidence{}` 那一路的写法），不为变绿种任何映射、价卡或约束。

## 要裁的

1. **甲还是乙**（或先甲后乙）。产品流程决定：择优是运营侧一次可回放的独立动作，还是每笔面单交易建立时的隐含前置步。本票不给倾向——lc/12 收口把它明写为「产品流程决定」，且它决定 06 编排要不要改输入形状。
2. **候选标识 → 七类依据引用的翻译归哪张票。** 乙路必需、甲路也终要（运营端点选出后总得有人建交易）。它读的全是 PC 的东西，按 ADR-0025 落 PS `adapters/partycommercial/`；是并进本票、还是另立一张「择优结果到交易依据」的适配器票，归派单方。若另立，本票 Blocked by 加它。

## 红线

- 不填任何候选、渠道账号、映射、价卡、约束取值（实例半边 `PAR-INT-02` / `PAR-COM-10` / `PAR-SET-03`）；三取数口未配置即恒答未配置，不改成默认放行。
- 不新造第二套映射或第二个择优口径；候选收窄归 PC、评价归 PP、并列归 PS 比较器（票 `01` 裁决），组合根只接线不加规则。
- 不在本票硬造端点：入口形状随「要裁的」1 定，定之前不动 `cmd/`。
- 择优编排 `Decisions` 三件要么全装、要么全不装（其头注原句）；生产装配一律全装——留痕是 `23` 读面的唯一来源。

## 完成判据（裁定后写实施判据；现阶段只有两条）

1. 「要裁的」两条各有裁决写进 Comments（谁、何时、口径），本票据以转 ready-for-agent 或按裁定拆票。
2. 转 ready-for-agent 时补齐：装配函数清单（择优 + 两适配器 + 06 + 07 壳 + 三取数口未配置）、端点或前置步的验收路径、`isolated_read_test.go` / 端点表 / 放行表三处共享接线的占号方式（并行会话纪律）、`gofmt` / `build` / `vet` / `test` 四项。

## 地盘

裁定前**无**。裁定后：`cmd/parcel-api/`（或裁定进程）的装配文件与 `unwired_orchestration.go`、端点表 / 探针 / 放行表三处共享接线（占号）；乙路另加 `internal/parcelshipment/application/operate_label_transaction.go`（输入形状）与 `internal/parcelshipment/adapters/partycommercial/`（若「要裁的」2 并进本票）。

## 参照

票 `12` 收口两条 Comment（2026-09-03，MCP-6）——代价分析权威；票 `06` 编排头注；票 `07`（出向缝落点）；票 `13`（成本分值桥）；票 `14` / `23`（留痕与读面）；`internal/parcelshipment/application/select_channel_candidate.go`（`SelectChannelCandidateDeps` 头注）；`internal/parcelshipment/ports/channel_selection.go`（`ChannelSelectionQuery` 头注）；`internal/parcelshipment/application/operate_label_transaction.go`（`EstablishLabelTransactionCommand` 七类引用）；`cmd/parcel-api/main.go` 两张读面的装配与 `unwired_orchestration.go`；`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 1 条；ADR-0088 Consequences；ADR-0025；`PAR-NET-16`。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/12 全文、`select_channel_candidate.go` 全文、`channel_selection.go` 的查询类型、`operate_label_transaction.go` 的 `EstablishLabelTransactionCommand`、`cmd/parcel-api` 对面单 / 择优的全部命中、`cmd/` 对上列名字的 grep；**没读** `channel_candidate_assembly.go` / `cost_source.go` 全文与 `cmd/parcel-api` 的 `build*Orchestration` 家族——三取数口「今天是否仍未配置」因此写成「未重核」。「顺带量到」那段翻译缺口是本票新量到的，lc/12 两条 Comment 没提。
