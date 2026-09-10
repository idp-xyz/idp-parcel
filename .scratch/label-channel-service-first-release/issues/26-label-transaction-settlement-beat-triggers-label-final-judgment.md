# 26 面单交易定案那一拍 → `JudgeLabelServiceFinalHandler`：同一次调用里判，还是落库后交一封信再判

Category: enhancement
Status: resolved——2026-09-10 19:1x 通道 1 推送方重放进 main（非作者评审 ← 通道 6 两轴 0 阻断；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」）。此前 resolved——2026-09-10 通道 2（task-02bb25bb，分支 `mcp2-lc26` 基远端 main `76932b38`）：代码七笔落分支，作者验带 DSN 全绿（见 Comments 完成记录），待推送方非作者评审 → 重放进 main。此前 in-progress——2026-09-10 18:0x 通道 2 认领，隔离树 `D:/tops/idp-parcel-mcp2-lc26`。此前 ready-for-agent——2026-09-10 通道 2 按通道 1 派单 task-138ab1c9（用户授权代裁，PS owner 口径）裁「要裁的」两条：1 取**乙**、2 **两拍都触发**，正文在 [ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)，「做法」按乙写实，见下方「裁决」与「做法（乙）」。此前 draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** 两条路的代价并列在「两条路」节，留作裁决记录
Blocked by: 无（机制半边：乙路每一件都能在替身与真库上做出来并测到）。**生产可达随 [`28`](./28-channel-selection-composition-root-and-call-entry.md)**（推送方 2026-09-10 已裁取乙：整条写链一个组合根）：`LabelTransactionHandler` 自身在 `cmd/` 零调用方，本票接上的触发点在组合根落地前没有生产事件流过它——这是事实不是阻塞，票面如实记；06 编排的事务壳（本票「做法」第 0 步）由 `28` 的组合根立，本票只要求它存在并在真库用例里自己开事务

## 缺口

票 [`11`](./11-parcel-final-across-transactions.md) Answer「不在本票」节的第二路：**面单交易定案那一拍**——票 [`06`](./06-label-transaction-write-side-executors.md) 的 `operate_label_transaction.go` 记下结果之后，没有任何东西去问「这件包裹的终局成没成」。`cmd/` 下 `NewJudgeLabelServiceFinalHandler` 零命中（`c7e3522c`）。

**定案在哪一拍。** `domain/label_transaction.go` 的 `Finalized()` 是派生谓词：`state.IsChannelResult()`，交易级结果落在三个结果格之一即定案；它没有存储列（ADR-0084 决定四），也不被 `AppendFollowUpAction` 改动（头注：「渠道退款、对账与运营结算按 CONTEXT 明文不属定案条件，因此追加后续动作不动这个谓词」）。让它由假变真的转移只有一个：`LabelTransactionHandler.RecordChannelResult`。

**但判断的输入不止定案一件。** `JudgeLabelServiceFinal` 关闭路径两条（CONTEXT）：「全部相关面单交易均已定案……不存在任何有效或结果待确认的面单结果」与「已有成功结果均已成功作废或……不可逆失效」。**作废**由 `AppendLabelFollowUpAction` 追加，它不动定案却改变「已有成功结果是否均已作废」这一格——一笔成功交易被渠道作废之后，包裹终局可能就此沿关闭路径形成。所以触发点候选有两拍：`RecordChannelResult`（定案）与 `AppendFollowUpAction`（作废 / 替代）。要不要两拍都触发，列在「要裁的」2。

**编排今天的形状决定了两条路各要加什么。** `LabelTransactionDeps` 只有 `Transactions`（`LabelTransactionRepository`）与 `Clock` 两口；后四步共用 `advance`：读回 → 转移 → 按预期版本 `Save`——**没有 Transactor、没有 handoff 口、没有任何「之后做什么」的缝**。PS 今天也没有面单交易的任何出向信封（`adapters/postgres/*_handoff.go` 的事件类型里没有 `label-transaction`）。`JudgeLabelServiceFinalCommand` 要委托来源身份与委托标识，而交易聚合只有租户与 `CoveredParcels`（覆盖可跨委托，ADR-0084 决定一）——不论哪条路，都要逐覆盖包裹经 `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 反查目标委托（与票 `25` 同一段翻译，可共用一只处理方适配器）。

## 两条路（代价并列——已裁取乙，本节留作裁决记录不改写）

**甲 · 同一次调用内**：`LabelTransactionHandler` 在 `RecordChannelResult`（及「要裁的」2 若裁为两拍，则也在 `AppendFollowUpAction`）`Save` 成功之后，逐覆盖包裹反查目标、折命令、调 `JudgeLabelServiceFinalHandler.Handle`。

- 要加：`LabelTransactionDeps` 长两格（判断口 + 目标反查口）；`advance` 长一个「写后动作」参数或在两步各自加尾段。
- **「同事务」今天不成立**：编排没有 Transactor，`Save` 是一次裸仓储调用；要真的同事务，得先给 06 编排加事务边界，并让 `FormParcelFinalHandler` 那一侧的写（`FinalOutcomeStore` / `FinalOutcomeHandoff`）跟着同一个 ctx 绑定的事务走——这是否被现有 `bentoapp.Transactor` 的用法支持要实施时核。不加事务边界就是「同一次调用、非原子」：结果已落、判断途中失败 → 判断丢失且没有重试载体（调用方拿到的 `LabelTransactionResult` 已是 `APPLIED`）。
- 多包裹交易 N 次判断串在写路径上：一个包裹的读口故障拖住整笔结果记录的回执；而 06 编排的调用方正握着 ADR-0090「答案未确定不得重发」那条纪律在等回执，回执变慢变复杂会直接压到出向那一侧。
- 06 编排从此知道终局判断存在（编排耦合）；头注「五步同一个 handler……缝一条都不少」那句要改。
- 好处：不新增信封、消费者、dispatch 路由，件数最少；因果在一处可读。

**乙 · 落库后交一封信，后置一拍**：`LabelTransactionHandler` 加一个 handoff 口（形照 `ports.FinalOutcomeHandoff` / TF `ExternalTrackingFactHandoff`：意图与登记同一事务入队），`RecordChannelResult`（及可能的 `AppendFollowUpAction`）落库同事务经 `outboxintent.EnqueueOnce` 入队一封 `parcel-shipment.label-transaction.*` 指针式信封（租户 + 交易标识 + revision，版本进事件 ID 让两代各自入队——同 `externalTrackingFactEventID` 的理由）；PS inbox 消费者收它 → 按交易读回 → 逐覆盖包裹反查 → 折命令（不带 `FirstEffectivePickup`，走关闭路径）→ `Handle`。

- 要加：端口、outbox 适配器、事件类型、inbox 消费者、处理方适配器、dispatch 路由——约六件，且 `EnqueueOnce` 要 db 与事务，06 编排**同样要先有事务边界**（这一条两路共同，不是乙独有的代价）。
- 判断延后一拍：读面上「交易已定案而包裹终局未判」有一个窗口，`LabelTransactionParcelRow` 那一行在窗口内答的是旧终局。
- 好处：重试有载体（inbox 重投）、失败不后退、逐包裹独立消费、06 编排不知道终局判断存在；与票 `25`（收寄那一路本就是 inbox）同形，`27` 也可同形——三个触发点共用一只处理方适配器与一套消费结论翻译。

## 要裁的

1. **甲还是乙。** 上面两节是全部代价；本票不给倾向——它牵动 06 编排的事务边界与 28 的组合根形状，归 owner / 产品流程。
2. **触发点范围**：只 `RecordChannelResult`（定案），还是也含 `AppendLabelFollowUpAction`（作废 / 替代改变关闭路径的输入）。倾向**两拍都触发**：判断本身不写、不形成时无副作用、采用按（租户 + 包裹 + 来源种类 + 来源版本）幂等，多判一次不会多形成一次；不触发作废那一拍，则「成功结果全部作废后沿关闭路径形成终局」要等下一笔交易定案或关闭决定才成，而那两件可能永不发生。

## 裁决

- **谁 / 何时 / 口径**：通道 2，2026-09-10，按通道 1 派单 task-138ab1c9；用户 17:0x 经队列授权「你自决」，B 类按 **PS owner 口径**代裁——硬句不改、拿不准的单列越权风险点供 owner 事后复核。正文与越权风险点在 [ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)，此处只对号。
- **要裁的 1 → 取乙**（ADR-0134 决定一、三）。理由四条：① 06 编排的调用方握着 ADR-0090「答案未确定不得重发」在等回执，N 个包裹的终局判断串在写路径上会把一个读口故障压到出向那一侧；② 甲今天「同事务」不成立，是「同一次调用、非原子」——结果已落、判断途中失败 → 判断丢失且无重试载体；乙有载体（inbox 重投）、失败不后退（ADR-0029）；③ 三个触发点（`25` 收寄本就是 inbox、本票定案、`27` 关闭 / 重开）共用一只处理方适配器 + 一套消费结论翻译（决定二）；④ 06 编排头注「不发起任何渠道调用……只留缝」仍真——handoff 是缝不是渠道调用，06 不知道终局判断存在。先例 `ports.FinalOutcomeHandoff` / TF `ExternalTrackingFactHandoff`（意图与登记同事务入队）。**本票原写「两路都要先加事务边界」那一句，ADR-0134 核过**：`LabelTransactions.Insert` / `Save` 与 `outboxintent.EnqueueOnce` 都 `RequireExecutor(ctx)`，同事务是既有约束的直接结果，06 编排**不加 Transactor**，事务由组合根事务壳开（`cmd/parcel-api` 的 `transactionalWithdrawal` 那一族形）。ADR 在票面倾向之上多定了三件：**一封一包裹**（ID 含包裹、分区键租户加包裹，让 lc/26 完成判据 2「各自成消费」成立）、**入队失败即整步回滚**（不照 `FormParcelFinalHandler.handOff` 留续办引用）、**两拍共用一个事件类型**——三件都列在 ADR 越权风险点。
- **要裁的 2 → 两拍都触发**（ADR-0134 决定四；A 类，推送方已裁，ADR 照写）。

## 做法（乙，按 ADR-0134；开工时以 ADR 决定号为准，本节是它在本票地盘上的展开）

0. **事务边界不在本票加**：06 编排不持 Transactor；生产上由 `28` 的组合根事务壳开事务。本票的真库用例自己用 `db.Transactor().WithinTransaction` 包住一次 `RecordChannelResult` / `AppendFollowUpAction`，断言 outbox 行与交易行同一事务落地；另加一条「不在事务里调用 → `Save` 处 error、outbox 零行」的用例，把 fail-closed 钉住。
1. **端口**（`internal/parcelshipment/ports/`，纯加法）：`LabelTransactionHandoff` 一族——意图带（租户、交易标识、包裹、`Revision()`、哪一拍），形照 `FinalOutcomeHandoffIntent`；头注写明它是「值得判一次」的指针，不是事实副本。
2. **outbox 适配器**（`adapters/postgres/`，新文件，形照 `final_outcome_handoff.go`）：事件类型一个（两拍共用，名字归实施，`parcel-shipment.label-transaction.` 前缀）；事件 ID = 租户 / 交易标识 / 包裹 / revision 加类型段（版本必须在里面，理由同 TF `externalTrackingFactEventID` 头注）；Subject = 包裹；**分区键 = 租户 + 包裹**（同 `OutboxFinalOutcomeHandoff` 的理由：同一包裹的多拍在一条队里）；载荷指针式 `{tenantId, transaction, parcel, revision, beat}`；经 `outboxintent.EnqueueOnce` 入队。
3. **06 编排**（`operate_label_transaction.go`）：`LabelTransactionDeps` 长一格 handoff 口；`advance` 长一个写后尾段参数——`RecordChannelResult` 与 `AppendFollowUpAction` 在 `Save` 成功后按 `CoveredParcels()` 逐件调 handoff；`Establish` / `SubmitToChannel` / `MarkResultUncertain` 不入队；`Save` 失败或版本冲突不入队；**入队返错 → 本步 error 上抛**（事务回滚，调用方重放），不交回 `APPLIED`。两处头注改口：「五步同一个 handler……缝一条都不少」改成写明后两步多一个写后入队的尾段；「`LabelTransactionDeps` 只有仓储与时钟两项」改成三项并写明第三项为何不是渠道调用。
4. **inbox 消费者**（`adapters/inbox/`，新文件，形照 `effective_delivery_consumer.go`）：稳定消费者名与既有各路不同；事件类型字符串由消费方自己写出；`Decode` 三维（租户 + 交易 + 包裹）缺一即 `ErrPoisonEnvelope`，`beat` 与 `revision` 不读（那是事件 ID 的事）；门走 `inboxconsume.New`。
5. **处理方适配器**（新文件，落 `adapters/` 下一个三路可共用的包；`25` / `27` 日后在同一只上加口）：核 = 按（租户 + 包裹）`CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 反查 → 折 `JudgeLabelServiceFinalCommand{Identity, ShipmentRequestID, Parcel}`（不带 `FirstEffectivePickup`）→ `Handle` → 五值译成消费结论（`25` 做法 3 那张表，ADR-0134 决定二逐格）：`LabelServiceFinalAdopted` → `Adoption()` 交 `finalconsume.Consumption`；`NotFinal` / `CancellationStands` → 已消费；`JudgmentUndecided` → 未决哨兵（返错重投）；`JudgmentNotAccepted` → 响亮报错不吸收。反查不中 → `ErrParcelTargetNotFound`。**不读回交易**：判断读全册且读当下，信封里的交易标识与 revision 只用于幂等与追溯。
6. **装配**（`cmd/parcel-dispatch/assemble.go`）：路由表加该事件类型一行；`JudgeLabelServiceFinalDeps` 六口按 `25` 做法 4 那一段装（`Validity` 按 ps-port-remainder/01 是否进 main 填适配器或 nil，装配处注释写明是哪一种）；`assemble_test.go` 补一条「该类型有路由」。`cmd/parcel-api` 侧 handoff 适配器装进 `LabelTransactionDeps` 归 `28`，本票只保证装配函数存在且不接受 nil。
7. **幂等 / 失败不后退**：inbox 键管一次投递只处理一次；判断本身不写；采用按四维幂等——来源版本 = 生效关闭决定标识（关闭路径），同一份重放返原；消费者返错 → dispatch 重投；毒丸 → 入账交 nil；`ErrParcelTargetNotFound` 按交付适配器那一格办。

## 红线

- 不动 `JudgeLabelServiceFinal` 的领域判断与 `Finalized()` 谓词；不给 `LabelTransaction` 加任何「已判终局」列（ADR-0084 决定四同理）。
- 06 编排仍**不发起任何渠道调用**（其头注原句）；本票加的缝只朝终局判断，不朝出向。
- 不为让路走通而在 `LabelTransactionHandler` 里猜委托：反查不中如实报 `ErrParcelTargetNotFound`（照交付适配器）。
- 不写任何真实渠道、账号、结果码（实例半边 `PAR-INT-02`）。

## 完成判据（非作者评审逐项对；乙路一组，甲路判据已随裁决作废）

1. `RecordChannelResult` 与 `AppendFollowUpAction` 两拍 `Save` 成功后，outbox 里每件覆盖包裹各一封、事件 ID 含 revision 与包裹、分区键 = 租户 + 包裹；`Save` 失败或版本冲突时**不**入队；同一拍重放不出第二封（`EnqueueOnce`）；两拍各自成封（revision 不同）——各有用例，真库实跑。
2. 不在事务里调用 → `Save` 处 error 且 outbox 零行；入队返错 → 本步 error、交易行未落（事务回滚）——两条 fail-closed 用例。
3. 多包裹交易逐包裹各成一次消费，命令不带 `FirstEffectivePickup`；反查不中那一件落 `ErrParcelTargetNotFound`，其余包裹的消费不受影响。
4. `LabelServiceFinalOutcome` 五值逐格译成消费结论，与票 `25` 做法 3 同一张表（共用核，用例逐格）。
5. inbox 消费者三维缺一毒丸、事件类型不符拒收、消费者名与既有各路不同；`cmd/parcel-dispatch/assemble.go` 路由表有该类型；`assemble_test.go` 补一条。
6. 06 编排头注「五步同一个 handler……」与 `LabelTransactionDeps` 头注「只有仓储与时钟两项」随改动改口，不留旧话；`Establish` / `SubmitToChannel` / `MarkResultUncertain` 三步不入队有用例钉住。
7. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库（本票动 outbox 入队，真库必须实跑）。
8. 完成记录写明：`LabelTransactionHandler` 在 `cmd/` 仍无调用方（等 `28`），本票的触发点因此暂无生产事件流过——与 ps-port-remainder/01 同款诚实句；`production_wiring_baseline.txt` 若因新增工厂无调用点而红，按既有纪律加行并写明等 `28`。

## 地盘

`internal/parcelshipment/application/operate_label_transaction.go` 与其测试；`internal/parcelshipment/ports/`（handoff 口，纯加法）、`internal/parcelshipment/adapters/postgres/`（新 handoff 文件）、`internal/parcelshipment/adapters/inbox/`（新消费者）、处理方适配器所在的新包（三路共用的核先在本票落；`25` / `27` 日后在同一只上加口）、`cmd/parcel-dispatch/`（`assemble.go` 路由表与一个装配函数 + `assemble_test.go` 一条）；本票面。**不动** `domain/**`、`cmd/parcel-api/**`（事务壳与 `LabelTransactionDeps` 的生产装配归 `28`）。

## 参照

票 `11` Answer「不在本票」节；`internal/parcelshipment/application/operate_label_transaction.go`（`LabelTransactionDeps` 头注、`LabelTransactionHandler` 头注、`advance`、`RecordChannelResult`、`AppendFollowUpAction`）；`internal/parcelshipment/domain/label_transaction.go`（`Finalized` 头注、`FollowUpActions`）；`internal/parcelshipment/application/judge_label_service_final.go`；`internal/parcelshipment/ports/ports.go` 的 `LabelTransactionRepository` 头注（「本口今天没有生产写入方，这是设计而不是欠账」）；`internal/parcelshipment/adapters/postgres/final_outcome_handoff.go` 与 TF `external_tracking_fact_handoff.go`（乙路的 outbox 形状先例）；`internal/platform/outboxintent`；PS CONTEXT 面单渠道服务终局规则那一段与「面单交易定案」定义；ADR-0084 决定一、决定四；ADR-0090；ADR-0043。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 `operate_label_transaction.go` 全文、`judge_label_service_final.go` 全文、`Finalized` / `ParcelResult` / `FollowUpActions` 三处访问器与头注、PS 全部 `*_handoff.go` 的事件类型常量、`LabelTransactionRepository` 头注；**没读** `FormParcelFinalHandler.Handle` 全文与 `bentoapp.Transactor` 在 PS 各编排里的用法——甲路「同事务是否可达」的那一句因此写成「实施时核」而不是结论。
- 2026-09-10 · 通道 2（task-138ab1c9，分支 `mcp2-adr0134` 基 `062f5228`；只写票面，未动代码）：**裁决落 [ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)，要裁的 1 取乙、2 两拍都触发，本票转 ready-for-agent。** 「做法（乙）」按 ADR 四决定在本票地盘上展开；完成判据改为乙路一组。上一条能力边界里留的「同事务是否可达」这一问 ADR 核过：`LabelTransactions.Insert` / `Save` 与 `outboxintent.EnqueueOnce` 都 `RequireExecutor(ctx)`，同事务由既有约束保证，编排不加 Transactor、事务由组合根事务壳开。ADR 在票面倾向之上多定的三件（一封一包裹 / 入队失败回滚 / 两拍一个事件类型）与窗口内读面旧终局、三路共用适配器是否压扣差异，共六条越权风险点列在 ADR 末，归 owner 复核；任一条被推翻都是本票「做法」改一步，不动裁决方向。**能力边界**：没读 `inboxconsume.Gate` 全文与 `bentoapp.Transactor` 实现，「消费者在事务里跑」按 `cmd/parcel-dispatch/assemble.go` 里 PS 既有的每一只 inbox 消费者都以 `db.Transactor()` 装配这一事实判。
- 2026-09-10 · 通道 2（task-02bb25bb，分支 `mcp2-lc26` 基 `76932b38`，隔离树 `D:/tops/idp-parcel-mcp2-lc26`，按 /implement 含 /tdd）：**完成记录，本票转 resolved。**
  - **逐笔**（每笔已 `push -u origin`；认领 `93842381` 只动票面）：`f172a972` 端口 + 06 编排两拍写后逐包裹入队（`LabelTransactionJudgmentHandoff` / `LabelTransactionJudgmentIntent` / `LabelTransactionBeat`；`Deps` 长 `Judgments`、不持 Transactor；`advance` 长写后动作；入队失败整步 error）→ `8a76c6bb` outbox 适配器 `OutboxLabelTransactionJudgmentHandoff` + 分区主体登记行 → `fea57cea` inbox 消费者 `LabelTransactionJudgmentConsumer` → `63581373` 三路共用处理方核 `adapters/labelfinal`（`ParcelJudgmentCore` + `Consumption` 五值翻译 + `LabelTransactionJudgmentAdapter`）→ `bc0e36df` `cmd/parcel-dispatch` 装配 `judgeLabelFinalOnLabelTransactionConsumer` + 未决哨兵名单 + 路由表一行 + `assemble_test` 一条；终局采用链抽成 `parcelFinalAdoptionChain` 供交付与判断两路共用 → `f8013a36` 清点在 `bc0e36df` 干净树重生成（PS 消费适配器 +1、直投路由 +1、端口声明 +1；缺口计数不变）→ `d6d50e30` 自审补两格（判断口未装配时后两步响亮报错不静默落库；「答案未确定」不入队有用例）。**代码 tip `d6d50e30`**；清点在其上重跑零差。
  - **完成判据逐项**：1 ✓ 两拍 `Save` 成功后每件覆盖包裹各一封，ID = 租户/label-transaction/交易/包裹/版本 加类型段，分区键 = 租户/包裹；`Save` 失败 / 版本冲突不入队；同拍重放不出第二封；两拍各自成封（真库：`TestTwoBeatsOfOneTransactionEachEnqueueBehindTheSameParcel`、`TestTheWriteSideEnqueuesEachBeatInTheSameTransactionAsItsSave`——真编排 + 真仓储 + 真 outbox，版本 3 / 4 两封同分区）。2 ✓ 不在事务里 → `Save` 处 `ErrTransactionRequired` 且 outbox 零行；入队返错 → 整步 error、交易行随事务回滚（`TestAFailedJudgmentHandoffRollsTheSaveBack`，失败替身代 outbox——真 outbox 在健康库上失败不了）。3 ✓ 一封一包裹结构上各自成消费；反查不中落 `labelfinal.ErrParcelTargetNotFound`、不猜委托。4 ✓ 五值逐格（`adapters/labelfinal` 用例经**真** `JudgeLabelServiceFinalHandler` 从夹具判出：关闭 + 明确失败交易 → 交采用路径经 finalconsume 收口；取消在先 / 不形成 → 入账；读口故障 → 未决哨兵；命令立不起 → 响亮；歧义原样上抛；译不出 → `ErrUntranslatableEnvelope`）。5 ✓ 消费者三维缺一毒丸、异类型响亮、消费者名与交付分账、两拍两次消费、缺 revision/beat 照常；路由表有该类型（`TestALabelTransactionJudgmentDueReachesTheConsumerThroughTheRouteTable`）。6 ✓ 两处头注改口（「仓储、判断意图口与时钟三项」「后两步多一段写后入队，前三步没有」），前三步不入队有用例。7 ✓ 见验证强度。8 ✓ 本条即诚实句：**`LabelTransactionHandler` 在 `cmd/` 仍无生产调用方（等 `28` 的组合根装 `LabelTransactionDeps` 与事务壳），本票的写入侧因此暂无生产事件流过；消费侧已接进 `cmd/parcel-dispatch` 路由表，信封一出现即可消费**。`production_wiring_baseline.txt` 未动（architecture 门禁绿；新工厂都是 `New*`，消费者与核在 dispatch 有生产调用点）。
  - **验证强度**（隔离树，钉 `d6d50e30`）：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -count=1 -p 1 ./internal/parcelshipment/... ./cmd/... ./internal/architecture/... ./internal/platform/...` **39 ok / 0 FAIL**（PS postgres 包 6.7 s、inbox 2.5 s、parcel-dispatch 4.8 s——非 SKIP）；反向依赖 `networkrouting/adapters/parcelshipment`、`visibilityexception/adapters/inbox`、`visibilityexception/adapters/parcelshipment`、`tests/bentocontract` 四包 ok；未跑全仓全量与 `-race`（派单只要求 PS + 反向依赖 + cmd + architecture，全量归推送方重放钉）。新 `.go` 全部 `gofmt -w` 过，`git ls-files --eol` 为 `i/lf`。
  - **在裁决之上多定的实施细节**（A 类，评审可改）：事件类型名 `parcel-shipment.label-transaction.judgment-due`；消费者名 `parcel-shipment/judge-label-final-from-transaction`；`Validity` 接了 `DeclaredLabelValidityRule`（ps-port-remainder/01 已进 main，声明缺席它答未配置，与 nil 同格）；意图多带 `OccurredAt`（渠道形成结果 / 作废发生的业务时间）作信封 `OccurredAt`；判断口未装配时后两步返错而非 nil 解引用。
  - **能力边界**：/tdd 第一片（编排 + 端口）测试与实现同笔写、只见过编译红没单独见过断言红，其余四片都先见红再绿；没读 `inboxconsume.Gate` 内部与 `bentoapp.Transactor` 实现；`parcelFinalAdoptionChain` 是对既有交付装配的抽取，行为未变（交付那条路由用例仍绿），但它是共享文件里的一次重排，评审请对着 `bc0e36df` 的 diff 看那一段。
- **评审 ← 通道 6 · 钉 `0a7ea17d`（代码 tip `d6d50e30`，基线 `76932b38`）· 2026-09-10 19:1x**（task-f8c78f66，非作者，隔离树 `%TEMP%\idp-review-lc26`；由推送方代落）。评审侧验证：`gofmt -l` 空；`go vet` PS 全部 + `cmd/parcel-dispatch` 退 0；带 DSN `go test -count=1 -p 1` application / adapters/inbox / adapters/labelfinal / cmd/parcel-dispatch / internal/architecture 五包 ok；adapters/postgres `-run Judgment|Beat|LabelTransaction -v` 全 PASS 零 SKIP；未跑全仓。
  - **Standards（红线 / PS CONTEXT / ADR-0134）**：**阻断 无**。**非阻断 4**：(1) 跨文件计数进注释——`cmd/parcel-dispatch/assemble.go` `judgeLabelFinalOnLabelTransactionConsumer` 头注「判断编排的六口」与 `labelTransactionJudgmentUndecidedSentinels` 头注「四个读口之一」、`adapters/labelfinal/judge_parcel.go` `ErrJudgmentUndecided` / `Consumption` 注释「四个读口之一」，数的是别的文件里 `JudgeLabelServiceFinalDeps` 的字段数，按 AGENTS.md 计数条与 `ports.go` `JudgmentAsOfFormation` 自家先例应去数字；(2) `assemble.go` `parcelFinalAdoptionChain` 头注称「各建一份 handler 会让…看见两处」，而两只消费者各调一次它、各得一份 `FormParcelFinalHandler`——实际共用的是 `FinalOutcomeStore` 那张表与装配代码，不是实例，注释理由与代码不一致，改句或建一次传两处；(3) 判断：`operate_label_transaction.go` `judgmentBeat` 取 `saved.Revision()+1`，把 `adapters/postgres/label_transaction.go` `Save`（`SET revision = $3 + 1`）「写回恰好加一」的实现细节抬进应用层，`LabelTransactionRepository` 头注未承诺 +1；真库用例已钉版本 3 / 4 故不阻断，`Save` 交回持久化版本可消掉这层假设；(4) 判断：`NewLabelTransactionHandler` 不校验依赖（既有风格），`Judgments == nil` 到首次 `RecordChannelResult` 才响亮——与文件一致不算违规，lc/28 装配处须断言非 nil。**无发现**：注释全中文、无行号引用；`domain/**` 与 `cmd/parcel-api/**` 零改动；06 编排不持 Transactor、不发起渠道调用（决定三）；夹具只有合成串，无真实渠道 / 账号 / 结果码。
  - **Spec（完成判据 1–8、「裁决」、「做法（乙）」）**：**阻断 无**。**非阻断 2**：(1) `bc0e36df` 把交付装配中段抽成 `parcelFinalAdoptionChain`，超出票面地盘「`assemble.go` 路由表与一个装配函数」的字面；对着 diff 逐步核过：构造顺序、构造函数、错误串一字未改（`nil,` 换成 `none,`），`processing` 用的 `chain.requests` / `chain.handler` 与旧路同一对象、交付路由用例仍绿——行为等价成立，记为范围说明不是缺陷；(2) 做法 5「反查不中 → `ErrParcelTargetNotFound`（照交付适配器）」实现用 `labelfinal.ErrParcelTargetNotFound`，与 `pstf.ErrParcelTargetNotFound` 是两个哨兵值、同一恢复格、未决名单各登各的——符合意图，指出以免日后误以为可互换 `errors.Is`。**无发现（判据逐项）**：1 ✓ 两拍 `Save` 后逐覆盖包裹一封、ID 与分区键如票面、只在写回成功后调 `afterSave`，重放 `EnqueueOnce` 与两拍相邻版本各成封真库实证；2 ✓ 无事务 `ErrTransactionRequired` 且 outbox 零行、入队失败整步 error 状态仍 SUBMITTED；3 ✓ 一封一包裹、命令 `FirstEffectivePickup` 零值；4 ✓ `Consumption` 五值逐格 + default 响亮，`Adopted` 无采用结果亦响亮；5 ✓ 三维缺一毒丸、异类型响亮、消费者名独立、路由表一行 + 用例；6 ✓ 两处头注改口、前三步不入队；7 ✓；8 ✓ 诚实句在完成记录、`production_wiring_baseline.txt` 未动且 architecture 绿、分区主体登记行带 ADR-0134 裁定句。
  - **重点核 ①–⑦**：① 等价成立；② 如票面；③ 决定三 / 四证到「回滚两样都不在」；④ 穷尽且未决不入账；⑤ 响亮不静默，但校验点在写时不在构造（Standards 4）；⑦ 无真实渠道 / 结果码。**⑥ 多定四件全同意**：事件类型 `parcel-shipment.label-transaction.judgment-due`（前缀合票面，「judgment-due」正是指针语义，两拍共用合决定四）；消费者名 `parcel-shipment/judge-label-final-from-transaction`（与交付 / 收寄 / 揽收各路不共名，分账有用例）；`Validity` 接 `DeclaredLabelValidityRule`（ps-port-remainder/01 已在 main，声明缺席与 nil 同格，装配处注释已写明）；意图带 `OccurredAt`（业务时间随命令进来不代铸，与 `LabelTransactionDeps` 头注「时钟只铸本仓自己动作」一致，不进事件 ID 不影响幂等）。
  - **结论**：两轴各 0 阻断，可重放进 main；非阻断随下一笔或另票顺手修。
- **进 main 记录（通道 1 推送方，2026-09-10 19:1x）**：隔离 detached 树 `%TEMP%\idp-replay-lc26` 在 main `dede3c2e` 上重放八笔零冲突（`76932b38..dede3c2e` 只动 `.scratch/**`，与本票文件零重叠，作者不必重验）：`93842381→bd06b91a`、`f172a972→45fa80d9`、`8a76c6bb→1a236fd3`、`fea57cea→2a52355d`、`63581373→4333543a`、`bc0e36df→b3b06226`、`d6d50e30→81c0598d`、`0a7ea17d→009ec866`；作者清点笔 `f8013a36` 不带，tip 重生成 `83ab2032`（与作者那份逐字节同）。本票全部文件（清点除外）`git diff 0a7ea17d 009ec866 -- <files>` 零行。**验证钉 `83ab2032`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:01:41→19:03:42，121 s；`1c48e4fe` 那笔写的 106 把随后探针那一行 `ok` 也数了进去，19:5x 按全量段单独重数改正）；探针 PS postgres `-run LabelTransactionJudgment -v` PASS 2 / SKIP 0。分支 `mcp2-lc26@0a7ea17d` 作封存出处、改名 `merged/`。**评审非阻断六条随票记、不挡合入**（Standards 1 / 2 是注释改句，3 是 `Save` 交回持久化版本的设计题，4 归 lc/28 装配；Spec 两条是说明）——可攒一张 lc 小票或随 lc/28 顺手修。
