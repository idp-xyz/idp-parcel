# 28 渠道择优链的组合根与调用入口：`cmd/` 下整条面单渠道写链至今零装配，择优是运营端点还是面单交易编排的前置步

Category: enhancement
Status: resolved——2026-09-10 21:3x 通道 1 推送方重放进 main（非作者评审 ← 通道 3 两轴 0 阻断；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」；评审 Spec 非阻断 2 把「生产可达」改读为「可装配、未触发」，触发面归后继票）。此前 resolved——2026-09-10 23:0x（作者自标，git 提交时刻 20:58）通道 4（task-03394bb6，分支 `mcp4-lc28` 基远端 main `53173062`，代码 tip `05afb2d1`）：06 建立一步改收整份「择优结果」、前置步编排（择优 → 29 翻译 → Establish）、`cmd/parcel-api` 组合根（择优 + 两适配器 + 留痕三件 + 29 翻译器 + 06 + 07 未配置壳 + 三取数口显式未配置 + 事务壳），真库验收路径两条各停点具名；完成记录与判断题见 Comments 末条。此前 in-progress——2026-09-10 22:0x 通道 4 认领：29 已于 19:34 进 main（`32778486`），阻塞解除，由通道 1 派单 task-03394bb6 直转 in-progress（原派通道 3 task-049e6dcd 未开工即 crash）；分支 `mcp4-lc28` 基远端 main `53173062`，隔离树 `D:/tops/idp-parcel-mcp4-lc28`。此前 blocked——2026-09-10 17:3x 通道 3 按通道 1 两条裁决改写（task-bc04bfc2；用户 17:0x 经队列授权「你自决」，读法在 `.scratch/tasks.md` 16:5x–17:0x 节）：「要裁的」1 取**乙**（择优作 06 编排的前置步）并带一条编排输入的形状约束，「要裁的」2 **另立 [`29`](./29-channel-selection-result-to-label-transaction-basis-translation.md)**；本票 Blocked by 29，29 进 main 后转 ready-for-agent。裁决原文见「要裁的」下「裁决」，实施判据已按裁决补齐。此前 draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；只写票面，未动代码
Blocked by: 无（29 已进 main `32778486`；此前 Blocked by 29——候选标识 → 七类依据引用的翻译适配器，乙路的前置步没有它接不到 `Establish`）

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

### 裁决（通道 1 推送方代裁 · 2026-09-10 17:1x · 用户 17:0x 经队列授权「你自决」；由通道 3 照写，task-bc04bfc2）

**28-1 取乙，带一条形状约束。** 口径（产品流程题按「机制半边现在做、不替产品选路」定）：

1. 乙让生产写链在**一个组合根**里可达：择优 → 建立 → 提交渠道 → 07 出向 → 记录结果 → 票 [`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md) 的定案触发；甲不让写链可达（本票面「两条路」自己写的）。
2. **形状约束：06 编排的新输入是一个「择优结果」对象——候选 + 七类依据引用 + 评价痕迹引用——编排不关心谁产生它。** 系统择优作前置步产出它（乙）；日后运营端点人工择优也可产出同一个对象（甲）而不必再改 06。这样 06 的输入形状只改这一次。
3. 首发只接乙的组合根；甲作叠加项**不立票**（今天没有运营角色事实可依）。
4. 三取数口（渠道约束、计价输入、`BUY` 价卡）按实例半边**显式未配置**装配（票面红线照旧），生产上链在「未配置」诚实停点。
5. 与票 `26` 的关系：`26` 的 ADR-0134（通道 2 在写）已取乙（定案后置一拍），与本票取乙可共存——择优前置与定案后置是同一条链的两端。

**28-2 另立 [`29`](./29-channel-selection-result-to-label-transaction-basis-translation.md)，本票 Blocked by 29。** 理由：地盘不同（PS `adapters/partycommercial/` vs `cmd/` 组合根）；它读的全是 PC 对象，写开时「七类引用各从 PC 哪个对象取」可能冒出 B 类问题（29 立票时已量到四条，见其「要裁的」），不该挡组合根票的形状定下来；两票可并行。

**越权风险点（供 owner 复核）**：是否需要运营人工择优（甲），属产品 owner；本裁决只定「首发不接甲、且形状上不堵甲」。

## 红线

- 不填任何候选、渠道账号、映射、价卡、约束取值（实例半边 `PAR-INT-02` / `PAR-COM-10` / `PAR-SET-03`）；三取数口未配置即恒答未配置，不改成默认放行。
- 不新造第二套映射或第二个择优口径；候选收窄归 PC、评价归 PP、并列归 PS 比较器（票 `01` 裁决），组合根只接线不加规则。
- 不在本票硬造端点：入口形状随「要裁的」1 定，定之前不动 `cmd/`。
- 择优编排 `Decisions` 三件要么全装、要么全不装（其头注原句）；生产装配一律全装——留痕是 `23` 读面的唯一来源。

## 完成判据（按 2026-09-10 裁决写的实施判据；29 进 main 前不认领）

1. **「择优结果」对象由 [`29`](./29-channel-selection-result-to-label-transaction-basis-translation.md) 定义并产出（通道 1 17:4x 补强 ①，免成环），本票只接线**：候选 `ChannelCandidateID` + 七类依据引用（与 `EstablishLabelTransactionCommand` 七格同型）+ 评价痕迹引用（可缺席）。06 编排的建立一步改收它（`EstablishLabelTransactionCommand` 的七格从它取，或命令直接嵌它，作者定）；**编排不问它从哪来**——头注写明系统择优与日后人工择优都产同一个对象。
2. **前置步**：一个应用层编排（或组合根内的一段装配）把 `SelectChannelCandidateHandler.Handle` 交回的候选，经票 `29` 的翻译适配器换成「择优结果」对象，再交 `Establish`。择优交回冲突 / 未配置 / 翻译停下时**不建立交易**，停点各自可分辨（不折成一格）。
3. **装配函数清单**（`cmd/parcel-api`，照既有 `build*Orchestration` 家族）：择优编排 + 其两个适配器（`channel_candidate_assembly.go` / `cost_source.go`）+ 留痕三件全装（`Decisions` / `DecisionIDs` / `Clock`，红线原句）+ 29 的翻译适配器 + 06 编排 + 07 出向端口的未配置壳 + 三取数口显式未配置（照 `UnconfiguredIntakeQualificationEvidence{}` 那一路）。`unwired_orchestration.go` 里两张读面的隔离占位随之改口或摘掉，以其头注约定为准。
4. **验收路径**：合成租户下走一遍前置步——三取数口未配置 → 链在择优装配那一格停在「未配置」，不建立交易、不写决定记录；测试替身把三口配上（合成串）→ 择优落定 → 翻译 → `Establish` → `Submit` → 07 壳答未配置 → 停在出向缝；每个停点断言到具名错误或结果格，不看日志。
5. **三处共享接线的占号方式**：`isolated_read_test.go` / 端点表 / 放行表若要动，动手前在频道占号、只改自己那一行、不动邻行（`docs/agents/parallel-sessions.md`「第三类：索引文件不属任何人」）；本票预期**不新增端点**（乙路无运营端点），若最终一行都不用动，完成记录写明。
6. **验证**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS `application` + `adapters/partycommercial` + `adapters/parcelpricing` + `cmd/parcel-api`（带 DSN）+ `./internal/architecture/...`（接线基线与可达性门禁会因新装配变动，按其头注重数、带 SHA）。
7. 完成记录逐笔 SHA、逐项对上面六条、写明「择优结果」对象落在哪一层及理由。

## 地盘

裁定后（乙）：`cmd/parcel-api/` 的装配文件与 `unwired_orchestration.go`；`internal/parcelshipment/application/`（06 编排输入形状 + 前置步编排；`select_channel_candidate.go` 的交回值 expand 归 `29`）；端点表 / 探针 / 放行表三处共享接线**预期不动**，要动先占号。**不含**「择优结果」对象本身与 `internal/parcelshipment/adapters/partycommercial/` 的翻译适配器——那是 `29` 的地盘。

## 参照

票 `29`（翻译适配器，本票的阻塞项）；票 `12` 收口两条 Comment（2026-09-03，MCP-6）——代价分析权威；票 `06` 编排头注；票 `07`（出向缝落点）；票 `13`（成本分值桥）；票 `14` / `23`（留痕与读面）；`internal/parcelshipment/application/select_channel_candidate.go`（`SelectChannelCandidateDeps` 头注）；`internal/parcelshipment/ports/channel_selection.go`（`ChannelSelectionQuery` 头注）；`internal/parcelshipment/application/operate_label_transaction.go`（`EstablishLabelTransactionCommand` 七类引用）；`cmd/parcel-api/main.go` 两张读面的装配与 `unwired_orchestration.go`；`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 1 条；ADR-0088 Consequences；ADR-0025；`PAR-NET-16`。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/12 全文、`select_channel_candidate.go` 全文、`channel_selection.go` 的查询类型、`operate_label_transaction.go` 的 `EstablishLabelTransactionCommand`、`cmd/parcel-api` 对面单 / 择优的全部命中、`cmd/` 对上列名字的 grep；**没读** `channel_candidate_assembly.go` / `cost_source.go` 全文与 `cmd/parcel-api` 的 `build*Orchestration` 家族——三取数口「今天是否仍未配置」因此写成「未重核」。「顺带量到」那段翻译缺口是本票新量到的，lc/12 两条 Comment 没提。
- 2026-09-10 17:3x · 通道 3（task-bc04bfc2，取证锚远端 main `062f5228`）：照写通道 1 两条裁决（见「要裁的」下「裁决」），Status draft → blocked（Blocked by 29），完成判据换成实施判据，地盘按乙路改写并把翻译适配器划给 `29`。**只写票面，未动代码。** 越权风险点一条写在「裁决」尾供 owner 复核。本票没重核三取数口的配置状态，仍以开工时 `cmd/` 装配处为准。
- 2026-09-10 23:0x · 通道 4（task-03394bb6，分支 `mcp4-lc28` 基远端 main `53173062`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证）：`b760212` docs Status → in-progress；`c2f44ba8` feat 06 建立一步改收整份「择优结果」（判据 1）；`7f821cdd` feat 前置步编排 `EstablishSelectedLabelTransactionHandler`（判据 2）；`d96e4793` feat `cmd/parcel-api` 组合根 + 真库用例 + 两处占位注释改口（判据 3、4、5）；`3c50706a` docs 清点在 `d96e4793` 干净检出重生成（PS 文件 +1、cmd 文件 +1、显式口具缺 8 → 7）；`05afb2d1` fix 自评修一处（07 壳 `QuerySubmission` 不主张查询口）。代码 tip = 分支 tip = `05afb2d1`。
  **「择优结果」对象落在哪一层及理由**：对象本身是 `29` 在 PS domain 定义的 `SelectedChannelBasis`（本票只接线）。06 的 `EstablishLabelTransactionCommand` 把七格换成一格 `Basis`（命令直接嵌它，不拆七格再拼——拆开就又有了一份可以漏格的形状），`Establish` 从它取七格交聚合构造门；候选不进聚合（参过选的候选与理由在决定记录里）。产出它的是应用层新编排 `EstablishSelectedLabelTransactionHandler`：择优（`ChannelSelector` 窄面）→ `ports.ChannelSelectionBasisTranslator`（29）→ 建立（`LabelTransactionEstablisher` 窄面）。落应用层而不是组合根内的一段装配，理由两条：停点的分格（并列 / 无人参选是结果格，择优 / 翻译没走完是带段标的错误、成因原样带出）要能用替身证，组合根里的一段 if-else 证不了；它不持 Transactor——两个窄面让组合根在外面各套一个事务壳（择优一段、建立一段分开成两笔），编排不知道壳的存在（ADR-0134 决定三的同一条纪律）。
  **完成判据逐项**：
  1. `EstablishLabelTransactionCommand.Basis domain.SelectedChannelBasis`（候选 + 七类依据引用 + 评价痕迹引用），06 头注写明「不发起渠道调用，也不择优……系统择优与日后人工择优都产同一个对象」；零值 Basis 由聚合构造门拒成 `INPUT_NOT_ACCEPTED` 不落库（`TestEstablishingWithoutASelectedBasisIsNotAccepted`）。
  2. 前置步编排三段各调一次、对象原样传递（`TestASelectedCandidateIsTranslatedAndHandedToEstablish`）；并列 → `SELECTION_TIED`、无人参选 → `NO_QUALIFIED_CANDIDATE`，不翻译不建立；择优 / 翻译没走完 → `ErrChannelSelectionStopped` / `ErrChannelBasisTranslationStopped` 各包成因（`%w: %w`），调用方对段标与成因各自 `errors.Is`；建立答复透传；三口任一 nil → `ErrSelectedLabelTransactionFlowMisconfigured`。**本层认不出适配器层的错误值（架构门禁不许应用层导入 adapters）**，所以「未配置」与「依赖故障」在本层同为择优段标——具名的那一格在成因里，真库用例对着 `pscommercial.ErrChannelConstraintNotConfigured` 断言。
  3. `buildLabelChannelOrchestration(db)`：择优编排（装配适配器读 PC `ProductChannelMappings` / `CommercialPublications` 真读口 + `unconfiguredChannelConstraints`；成本适配器 + `unconfiguredPricingInput` / `unconfiguredBuyPlans`）、留痕三件全装（`pspostgres.ChannelSelectionDecisions` / `psidentity.ChannelSelectionDecisions` / `systemClock`）、29 翻译器（`ChannelAccountUseAuthorizations` / `SupplierAgreementContents` 两个 PC 读口真装——lc/29 评审点名；三源 nil = 显式未配置）、06（`pspostgres.LabelTransactions` / `OutboxLabelTransactionJudgmentHandoff` / 时钟，装配点断言非 nil——lc/26 评审 Standards 4）、07 `unconfiguredLabelChannelGateway`（三操作一律 `outbound.Unconfigured()`、不发起调用）、事务壳 `transactionalChannelSelection` + `transactionalLabelTransactions`。`unwired_orchestration.go` 的 `unwiredLabelTransactions` 与 `assemble_channel_selection_decisions.go` 的头注「写那一半今天没有生产装配点」改口为「组合根已在、今天没有触发面」，占位类型本身留着（仍只供装配测试）。
  4. 真库（`TestTheProductionLabelChannelChainStopsHonestlyAtTheUnconfiguredSources` / `TestTheLabelChannelChainWalksToTheOutboundSeamOnceTheSeamsAreConfigured`）：生产装配 → `Flow.Establish` 停在 `ErrChannelSelectionStopped` ∧ `ErrChannelConstraintNotConfigured`（约束先问先停），库里零决定零交易，07 壳答 `NOT_CONFIGURED`；缝配上替身 → 决定记录 `SELECTED` 落库 → 翻译 → `ESTABLISHED` 落库（费率来自择优结果）→ `SubmitToChannel` → `SUBMITTED` → 07 壳 `NOT_CONFIGURED` 且 `!AdmitsResend()`；翻译停下 → 段标 + 成因、决定记录仍在、无交易；并列 → `SELECTION_TIED`、`TIED` 落库、无交易。**替身换在端口这一层**（`labelChannelSeams` 的 `Assembly` / `Costs` / `Translator`，头注写明只给测试）而不是三取数口那一层：真装配与真翻译读的 PC 映射 / 发布册 / 账号使用授权 / 供应商协议是实例半边、仓里没有种子；那三只适配器各自的包有真库与替身用例。这一处偏离票面字面，见判断题 (a)。
  5. 端点表 / 探针 / 放行表 / `main.go` **一行未动**，无新增端点（乙路无运营端点）。
  6. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；带 DSN `-p 1 -count=1`（隔离树 `mcp4-lc28@d96e4793`）：PS 全部包 + 反向依赖（含 `cmd/*` 十一包）+ `internal/architecture` 共 32 包：30 ok / 0 FAIL / 2 无用例 / 0 cached，41 s；`-v` PASS 2357 / SKIP 0 / FAIL 0（cmd/parcel-api 70、PS application 338、PS postgres 171、PS adapters/partycommercial 191、PS adapters/parcelpricing 15、architecture 146）。**接线基线与可达性基线零改动**：本票不加导出领域工厂、不改导出领域类型的可达路径（`EstablishLabelTransaction` 早已由 06 引用），两道棘轮不响、不必重数。`05afb2d1` 后复跑 `cmd/parcel-api` 带 DSN ok。
  7. 本条即完成记录。**三取数口今天的配置状态实测**：`cmd/` 下此前对择优 / 06 / 07 零装配（票面事实节原句今日仍成立，`git grep -n 'NewSelectChannelCandidateHandler\|NewLabelTransactionHandler' -- cmd/` 在 `53173062` 上零命中），三口从未配置过；本票装配后三口一律显式未配置，生产上链停在 `ErrChannelConstraintNotConfigured`（约束先问）——计价输入与 BUY 价卡两口今天连问都问不到。
  **真库用例先红后绿钉住的一处设计问题**：择优编排「先记决定再以具名错误交回」并列 / 无人参选，套进事务壳后若照失败回滚，刚记下的 `TIED` 决定随事务消失——票 23 读面上「交人工裁决」的唯一来源就没了。`transactionalChannelSelection` 对这两格提交、错误带出（头注写理由）。
  **自评（/code-review 两轴串行自跑，子代理鉴权错）**：Standards 一条已修（07 壳 `QuerySubmission` 原答 `Support: Offered` 是编事实，改留零值）；Spec 无阻断。非阻断留票：事务壳三处同形（与 build* 家族一致，未抽）；`labelChannelOrchestration` 无生产调用方（见下）。
  **给评审的判断题**：(a) 验收路径的替身换在端口层（Assembly / Costs / Translator）而不是票面写的三取数口层——理由是 PC 行无种子；若要求字面，需在 cmd 测试里造 PC 映射 + 发布版本 + 账号使用授权 + 供应商协议四类行，另立票还是补进本票？(b) `buildLabelChannelOrchestration` 今天没有生产调用方（`main.go` 未动）——「生产可达」在本票读成「组合根存在且真库可走通」，触发面归后继票；要不要让 `main` 在启动时装配它以便 `unwired*` 门禁将来能量到？(c) 择优壳对并列 / 无人参选提交而不回滚，是把「哪些错误是业务答案」的知识放进了组合根；更干净的形是让 `SelectChannelCandidateHandler.Select` 把这两格改成结果而非错误（改 12/29 的既有签名），归 PS owner。(d) 前置步的择优与建立分两笔事务：翻译停下保留决定、建立失败也保留决定——与「择优留痕在择优那一步已写完」一致，但一次「建立版本冲突后重放」会在决定册上留下第二条 SELECTED；可否接受，或建立重放该复用首次择优？
  **不做的**：触发面（运营端点 / 进程内触发）；07 真适配器；三取数口与三源的真实取数路径（实例半边）；26 的定案消费者接线（lc/26 已在 `cmd/parcel-dispatch`）。
  **地盘外零改动**：`internal/parcelshipment/adapters/partycommercial/channel_selection_basis.go`、PC / PP / TF / SA、端点表 / 探针 / 放行表 / `main.go`、`internal/architecture/*_baseline.txt`。
- **评审 ← 通道 3 · 钉 `a35bc375`（代码 tip `05afb2d1`，基线 `53173062`）· 2026-09-10 21:3x**（task-220c3eb5，非作者，隔离树 `%TEMP%\idp-review-lc28`，不带 DSN；由推送方代落）。评审侧实测：`go test -count=1` PS application / `cmd/parcel-api` / `internal/architecture` 三包 ok；`go vet` 退 0、`gofmt -l` 空；`git diff --stat 53173062 a35bc375` 11 件，无 `main.go` / 端点表 / 探针 / 放行表 / `isolated_read_test.go`；`git grep buildLabelChannelOrchestration a35bc375 -- cmd/ ':!*_test.go'` 只命中自身文件。两条真库用例未跑（无 DSN），以作者记录为准。
  - **Standards**：**阻断 无**。**非阻断 4**（均判断题级）：① Repeated Switches——`ErrChannelCandidateCostTied` / `ErrNoQualifiedChannelCandidate` 这对分类在三处各写一遍：`select_channel_candidate.go` `Select`、`establish_selected_label_transaction.go` `Establish`、`assemble_label_channel.go` `transactionalChannelSelection.Select`。根因是 `Select`「业务答案当 error 交回」的签名，见判断题 (c)。② 票面红线「组合根只接线不加规则」——`transactionalChannelSelection` 决定哪两个错误提交、其余回滚，是业务分类进了 cmd。头注写了理由、真库用例钉住，可接受为非阻断。③ `labelChannelSeams.Assembly / Costs / Translator` 是生产文件里只给测试的替身位；同家族先例 `buildClaimsOrchestrationWith(db, keys)` 的额外参数是生产也填的真缝，此处是新形（Speculative Generality 味）。头注诚实，非阻断。④ `labelChannelSources` 六缝的非 nil 分支（`constraintsOrUnconfigured` 等）在本包零用例触及，「缝有没有真穿到适配器」在组合根层未证。**无发现**：注释中文、跨文件引用用符号名无行号（AGENTS.md）；应用层新文件只导入 domain / ports（架构门禁过）；lc/26 S4「Judgments 非 nil 断言」已做（`transactions == nil || judgments == nil` 检）；lc/29「两 PC 读口真装」已做（`NewChannelAccountUseAuthorizations` / `NewSupplierAgreementContents`）；留痕三件全装；07 壳不读 Configuration、不发起调用（ADR-0090 决定五）；编排不持 Transactor（ADR-0134 决定三）。
  - **Spec**：**阻断 无**。**非阻断 3**：① 判据 4 原句「测试替身把三口配上（合成串）」未按字面做——替身换在端口层，真成本适配器与真翻译器在本包从未以「已配置」态跑过；成因（PC 行无种子）成立，但属偏离须记入完成记录（已记）。② 裁决 28-1.1「生产写链在一个组合根里可达」——`buildLabelChannelOrchestration` 无生产调用方，`parcel-api` 二进制从不构造这条链：构造期错误（如 `NewChannelAccountUseAuthorizations` 失败）生产上不会在启动时暴露；`production_wiring_ratchet_test.go` 只量导出领域工厂，不会为此变红。判据 3 / 4 / 5 字面满足、判据 5 明写无端点，故非阻断；但完成记录应改口为「可装配、未触发」，并点名后继票号（记录只写「归后继票」无票号）。③ 判断题 (d) 所述重放行为**无用例钉住**：`Flow.Establish` 重放会再写一条 SELECTED，且 `ChannelSelectionDecision` 以 (scope, mapping) 为对象、不带交易标识——若重放时成本已变，决定册与交易可能不一致且无法对账。票面无重放判据，非阻断，须立票。**无发现**：判据 1（`EstablishLabelTransactionCommand.Basis` 整份嵌入不拆七格；零值 Basis 由构造门拒 `INPUT_NOT_ACCEPTED` 有用例）；判据 2（并列 / 无人参选成结果格不翻译不建立、段标 + 成因 `%w: %w`、三口任一 nil → `ErrSelectedLabelTransactionFlowMisconfigured`，单测覆盖）；判据 3 清单齐；判据 6 可跑的部分绿；判据 7 完成记录在。重点核 ⑤ 两层 `errors.Is` 可分：Go 1.26 多 `%w` 生成多 Unwrap，bento `rollbackWithCause` 回滚成功时原样返回 callback error，段标与成因都穿过事务壳（单测已证、真库用例断言同款）。⑥「端点表 / 探针 / 放行表 / main.go 一行未动」属实；`unwired_orchestration.go` 头注改口不触及任何门禁（架构包量的是导出工厂不是 `unwired*` 占位类型）；「地盘外零改动」清单漏列 `docs/product/MECHANISM-INVENTORY.md`（重生成件，事实精度问题非违规）。
  - **判断题**：(a) **同意**偏离且不并进本票：cmd 测试里造 PC 四类行是 PC 夹具的第二份，若要字面另立票、且先等 PC 出合成种子助手；本票补一条「配上 Constraints 替身即穿过 `ErrChannelConstraintNotConfigured`」的小用例即可证六缝真穿。(b) **不同意**作者读法——「组合根存在且真库可走通」= 可装配，不等于生产可达；非阻断，因判据字面满足且票面自写无端点。倾向让 `main` 启动时构造（零行为变化、构造期错误 fail-fast），但那动共享 `main.go` 须占号，归后继票并给票号。(c) **同意**更干净的形是 `Select` 把并列 / 无人参选改成结果格（三处重复分类即代价）；代码现状可接受为非阻断——有真库用例钉、头注写因，归 PS owner 改 12 / 29 签名。(d) **同意**是真缺口且未钉：非阻断（票面无重放判据），须立票——二选一：建立重放先查交易已存在则跳过择优；或决定记录带交易引用让决定册可对账。
  - **结论**：Standards 0 阻断 / 4 非阻断（最重：三处重复分类，根因 (c)）；Spec 0 阻断 / 3 非阻断（最重：无生产调用方而完成记录写「生产可达」，(b)）。可重放进 main。
- **进 main 记录（通道 1 推送方，2026-09-10 21:3x）**：通道 4 前一会话 20:59 在 `%TEMP%\idp-replay-lc28` 把七笔叠到 main `9220ab06`（tip `83124308`，随后 crash）；本会话核过内容（11 件对分支 tip 只差清点与 main 自己改的 lc spec 30 行）后**改叠在 sa-cc/03 链 `343b997b` 之上**，六笔 cherry-pick 零冲突：`b7602123→f3d938ac`、`c2f44ba8→7019f51f`、`7f821cdd→dd54ad63`、`d96e4793→38324ce1`、`05afb2d1→32eecea3`、`a35bc375→002d3135`；作者清点笔 `3c50706a` 不带，tip 重生成 `784ad076`（PS 文件 +1、cmd 文件 +1、显式口具缺 8 → 7，与作者那份口径同）。本票 Go 文件 `git diff a35bc375 784ad076 -- <files>` 零行。**验证钉 `784ad076`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（21:27:45→21:29:38，113 s）；`cmd/parcel-api -run LabelChannelChain -v` PASS 2 / SKIP 0（两条真库用例在推送方这跑实跑）。`ls-remote` 核 `9220ab06` 未动 → ff → `push <sha>:main`。分支 `mcp4-lc28@a35bc375` 作封存出处、改名 `merged/`、远端删；`merged/mcp3-lc28@221576cc` 是通道 3 早先只写票面那支，与本次无关。**推送方对评审的处置**：两轴 0 阻断，非阻断七条随票记、不挡合入。Spec ② 采纳评审读法——本票交付的是「组合根**可装配**且真库可走通」，**未触发**：`main.go` 未动、生产进程不构造这条链；触发面（`main` 启动时装配以 fail-fast，或运营 / 进程内触发）归后继票 [`34`](./34-label-channel-chain-startup-assembly-fail-fast.md)（2026-09-10 22:0x 通道 4 立，本条与判断题 (b) 已搬过去；34 只做启动时装配那一步，谁发起归产品题另议）。Spec ③ / 判断题 (d)（建立重放在决定册留第二条 SELECTED、无用例钉）须立票，同归 PS owner，候选与 (c)（`Select` 把并列 / 无人参选改成结果格）合一张 PS 小票。Standards ④（六缝非 nil 分支零用例）随 (a) 的小用例一并补。
