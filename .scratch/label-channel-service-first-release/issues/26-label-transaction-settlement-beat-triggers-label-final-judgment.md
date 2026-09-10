# 26 面单交易定案那一拍 → `JudgeLabelServiceFinalHandler`：同一次调用里判，还是落库后交一封信再判

Category: enhancement
Status: draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** 两条路的代价并列在「要裁的」，本票不自己定
Blocked by: 无（机制半边：两条路任一都能在替身上做出来并测到）。**生产可达随 [`28`](./28-channel-selection-composition-root-and-call-entry.md)**：`LabelTransactionHandler` 自身在 `cmd/` 零调用方，本票接上的触发点在组合根落地前没有生产事件流过它——这是事实不是阻塞，票面如实记

## 缺口

票 [`11`](./11-parcel-final-across-transactions.md) Answer「不在本票」节的第二路：**面单交易定案那一拍**——票 [`06`](./06-label-transaction-write-side-executors.md) 的 `operate_label_transaction.go` 记下结果之后，没有任何东西去问「这件包裹的终局成没成」。`cmd/` 下 `NewJudgeLabelServiceFinalHandler` 零命中（`c7e3522c`）。

**定案在哪一拍。** `domain/label_transaction.go` 的 `Finalized()` 是派生谓词：`state.IsChannelResult()`，交易级结果落在三个结果格之一即定案；它没有存储列（ADR-0084 决定四），也不被 `AppendFollowUpAction` 改动（头注：「渠道退款、对账与运营结算按 CONTEXT 明文不属定案条件，因此追加后续动作不动这个谓词」）。让它由假变真的转移只有一个：`LabelTransactionHandler.RecordChannelResult`。

**但判断的输入不止定案一件。** `JudgeLabelServiceFinal` 关闭路径两条（CONTEXT）：「全部相关面单交易均已定案……不存在任何有效或结果待确认的面单结果」与「已有成功结果均已成功作废或……不可逆失效」。**作废**由 `AppendLabelFollowUpAction` 追加，它不动定案却改变「已有成功结果是否均已作废」这一格——一笔成功交易被渠道作废之后，包裹终局可能就此沿关闭路径形成。所以触发点候选有两拍：`RecordChannelResult`（定案）与 `AppendFollowUpAction`（作废 / 替代）。要不要两拍都触发，列在「要裁的」2。

**编排今天的形状决定了两条路各要加什么。** `LabelTransactionDeps` 只有 `Transactions`（`LabelTransactionRepository`）与 `Clock` 两口；后四步共用 `advance`：读回 → 转移 → 按预期版本 `Save`——**没有 Transactor、没有 handoff 口、没有任何「之后做什么」的缝**。PS 今天也没有面单交易的任何出向信封（`adapters/postgres/*_handoff.go` 的事件类型里没有 `label-transaction`）。`JudgeLabelServiceFinalCommand` 要委托来源身份与委托标识，而交易聚合只有租户与 `CoveredParcels`（覆盖可跨委托，ADR-0084 决定一）——不论哪条路，都要逐覆盖包裹经 `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 反查目标委托（与票 `25` 同一段翻译，可共用一只处理方适配器）。

## 两条路（代价并列，本票不定）

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

## 红线

- 不动 `JudgeLabelServiceFinal` 的领域判断与 `Finalized()` 谓词；不给 `LabelTransaction` 加任何「已判终局」列（ADR-0084 决定四同理）。
- 06 编排仍**不发起任何渠道调用**（其头注原句）；本票加的缝只朝终局判断，不朝出向。
- 不为让路走通而在 `LabelTransactionHandler` 里猜委托：反查不中如实报 `ErrParcelTargetNotFound`（照交付适配器）。
- 不写任何真实渠道、账号、结果码（实例半边 `PAR-INT-02`）。

## 完成判据（非作者评审逐项对；按裁定的那条路取对应一组）

1. 裁定的触发拍上，`Save` 成功后判断被调用（甲：替身记录调用；乙：outbox 里有一封且事件 ID 带 revision），`Save` 失败或版本冲突时**不**调用 / 不入队——各有用例。
2. 多包裹交易逐包裹各判一次，命令不带 `FirstEffectivePickup`；反查不中那一件不拖累其余包裹（乙：各自成消费；甲：写明是否继续）。
3. `LabelServiceFinalOutcome` 五值逐格译成调用方 / 消费者结论，与票 `25` 同一张表（若共用适配器，用例证共用）。
4. 乙路另加：inbox 消费者三维缺一毒丸、事件类型不符拒收；`cmd/parcel-dispatch/assemble.go` 路由表有该类型；`assemble_test.go` 补一条。
5. 06 编排头注「五步同一个 handler……」与 `LabelTransactionDeps` 头注「只有仓储与时钟两项」随改动改口，不留旧话。
6. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库（乙路动 outbox 入队，真库必须实跑）。
7. 完成记录写明：`LabelTransactionHandler` 在 `cmd/` 仍无调用方（等 `28`），本票的触发点因此暂无生产事件流过——与 ps-port-remainder/01 同款诚实句。

## 地盘

`internal/parcelshipment/application/operate_label_transaction.go` 与其测试（两路都要动它）；乙路另加 `internal/parcelshipment/ports/`（handoff 口，纯加法）、`internal/parcelshipment/adapters/postgres/`（新 handoff 文件）、`internal/parcelshipment/adapters/inbox/`（新消费者）、`internal/parcelshipment/adapters/transportfulfillment/` 或新包（处理方适配器，若与 `25` 共用则在 `25` 那只上加口）、`cmd/parcel-dispatch/`；本票面。**不动** `domain/**`。

## 参照

票 `11` Answer「不在本票」节；`internal/parcelshipment/application/operate_label_transaction.go`（`LabelTransactionDeps` 头注、`LabelTransactionHandler` 头注、`advance`、`RecordChannelResult`、`AppendFollowUpAction`）；`internal/parcelshipment/domain/label_transaction.go`（`Finalized` 头注、`FollowUpActions`）；`internal/parcelshipment/application/judge_label_service_final.go`；`internal/parcelshipment/ports/ports.go` 的 `LabelTransactionRepository` 头注（「本口今天没有生产写入方，这是设计而不是欠账」）；`internal/parcelshipment/adapters/postgres/final_outcome_handoff.go` 与 TF `external_tracking_fact_handoff.go`（乙路的 outbox 形状先例）；`internal/platform/outboxintent`；PS CONTEXT 面单渠道服务终局规则那一段与「面单交易定案」定义；ADR-0084 决定一、决定四；ADR-0090；ADR-0043。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 `operate_label_transaction.go` 全文、`judge_label_service_final.go` 全文、`Finalized` / `ParcelResult` / `FollowUpActions` 三处访问器与头注、PS 全部 `*_handoff.go` 的事件类型常量、`LabelTransactionRepository` 头注；**没读** `FormParcelFinalHandler.Handle` 全文与 `bentoapp.Transactor` 在 PS 各编排里的用法——甲路「同事务是否可达」的那一句因此写成「实施时核」而不是结论。
