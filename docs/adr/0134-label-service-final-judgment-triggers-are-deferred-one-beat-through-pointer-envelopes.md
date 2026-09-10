# ADR-0134：面单渠道服务终局判断的三种触发都以指针式信封后置一拍——写入方落库同事务只入队信封（一封一包裹：租户 + 触发对象标识 + 包裹，版本进事件 ID），判断由 `parcel-shipment` 自己的 inbox 消费者在下一拍形成；三路共用一只处理方适配器与一套按恢复动作分格的消费结论翻译；面单交易写编排不持 Transactor，事务边界由组合根的事务壳给；定案与后续动作两拍都触发

Status: Accepted（2026-09-10，通道 2 按通道 1 派单 task-138ab1c9「用户授权代裁，PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md)、[lc/26](../../.scratch/label-channel-service-first-release/issues/26-label-transaction-settlement-beat-triggers-label-final-judgment.md)、[lc/27](../../.scratch/label-channel-service-first-release/issues/27-controlled-close-reopen-decision-triggers-label-final-judgment.md)、[lc/28](../../.scratch/label-channel-service-first-release/issues/28-channel-selection-composition-root-and-call-entry.md) 全文与 lc/11 Answer「不在本票」节、[ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md) 全文、[ADR-0090](./0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md) 全文、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 全文、[PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)「面单交易定案」「面单继续尝试决定」词条、Rules 里面单渠道终局那一族硬句、生命周期「包裹身份与服务」两条关闭路径与「面单继续尝试」「面单交易」两节、Boundaries 面单一行；代码读过 `internal/parcelshipment/application/operate_label_transaction.go` 全文（`LabelTransactionDeps` 与 `LabelTransactionHandler` 头注、`advance`）、`judge_label_service_final.go` 全文（`LabelServiceFinalOutcome` 五值、`JudgeLabelServiceFinalDeps`、`responsibilityOutcomeOf`）、`form_parcel_final.go` 的 `FormParcelFinalDeps` 与 `handOff`、`adapters/postgres/final_outcome_handoff.go` 全文、`adapters/inbox/effective_delivery_consumer.go` 全文、`adapters/transportfulfillment/adopt_on_effective_delivery.go` 的错误分格、`internal/platform/outboxintent/enqueue_once.go` 全文、`adapters/postgres/label_transaction.go` 的 `Insert` / `Save` 取执行器那一句、TF `external_tracking_fact_handoff.go` 的 `externalTrackingFactEventID` 头注、`cmd/parcel-api/assemble_withdrawal.go` 的事务壳形状。**没读** `bentoapp.Transactor` 的实现与 `inboxconsume.Gate` 全文——「消费者在事务里跑」按 `cmd/parcel-dispatch/assemble.go` 里 PS 既有的每一只 inbox 消费者都以 `db.Transactor()` 装配这一事实判，未逐行核 Gate 内部。）
Date: 2026-09-10

## Context

lc/11 把面单渠道服务的包裹终局判断落成 `JudgeLabelServiceFinalHandler`，命令四件（委托来源身份、委托标识、包裹、可缺席的收寄事实引用），并在 Answer「不在本票」节点名三个调用方——TF 实际承运商首次有效收寄到达、面单交易定案那一拍、受控关闭 / 重开决定生效。三张接线票 lc/25 / 26 / 27 于 2026-09-10 立出，`cmd/` 下 `NewJudgeLabelServiceFinalHandler` 至今零调用方。三票各自量出的事实：

- **lc/25（收寄）**：PS 半边天然是 inbox 消费者——事实在 TF，PS 只能收信。它等的那封信 TF 今天没有（`external-carrier-tracking.judged` 按 TF CONTEXT 不构成收寄），形状归 TF owner；PS 侧对信封的最小要求是「指针式、带版本、可按版本取回已判断的有效时间」。
- **lc/26（定案）**：`LabelTransactionHandler` 五步共用 `advance`（读回 → 转移 → 按预期版本 `Save`），`LabelTransactionDeps` 只有仓储与时钟，没有 handoff 口、没有「写后动作」的缝；`Finalized()` 是派生谓词，由假变真只经 `RecordChannelResult`；但关闭路径的另一格输入「已有成功结果均已成功作废」由 `AppendFollowUpAction` 改变。票面并列了甲（同一次调用内判）与乙（落库后交一封信、后置一拍）两条路，代价写全、不给倾向。
- **lc/27（关闭 / 重开）**：决定口 `ContinuedAttemptRegisterRepository` 头注自写「本口今天没有生产写入方，这是设计而不是欠账」，形成决定的写面至今无票；「生效」= 决定被追加进登记册那一刻（`standingClosure` 不看时钟），所以触发点就是写面 `Save` 成功那一拍；重开也触发、预期不形成。票面把触发形状挂在 lc/26 的裁决上，「写面票立不立、立在哪、拆几半」留为要裁的。

**为什么现在必须定。** 三路的触发形状一旦各自落地就各自成先例：甲路会让 06 编排长出判断口与目标反查口、让 lc/27 的写面编排照抄同一形；乙路会让 PS 多一种出向信封、一只消费者、一套翻译。两路在 `go build` 与 `go test` 下都绿，分叉只在集成时可见——与 ADR-0090 立记录时的理由同一条。lc/28 已裁「取乙：整条写链一个组合根」（推送方 2026-09-10 裁），06 编排从此有了生产调用方，定案那一拍很快就会真的发生。

**两条路今天各自缺什么，先说清楚。** 甲路票面写的「同事务」今天不成立：`LabelTransactionDeps` 没有 Transactor，`Save` 是一次裸仓储调用；不加事务边界就是「同一次调用、非原子」——结果已落、判断途中失败，判断丢失且没有重试载体（调用方拿到的 `LabelTransactionResult` 已是 `APPLIED`）。乙路票面写「`EnqueueOnce` 要 db 与事务，06 编排同样要先有事务边界」，并把「`bentoapp.Transactor` 能不能支持」留给实施时核。**这一句本记录核过了**：`LabelTransactions.Insert` / `Save` 今天就以 `db.RequireExecutor(ctx)` 取执行器，`outboxintent.EnqueueOnce` 同样以 `db.RequireExecutor(ctx)` 取——两者从同一个 ctx 取同一个事务执行器。也就是说 06 编排**今天已经**只能在一个 ctx 绑定的事务里写，不在事务里调它会在 `Save` 处响亮失败而不是静默落一半；「意图与登记同事务」不是要新造的机制，是既有约束的直接结果。事务边界在本仓的落点也是既定的：`cmd/parcel-api` 的每一只受控编排壳（`transactionalWithdrawal` 那一族）都是「`WithinTransaction` 包住一次编排调用」，编排本身不持 Transactor。

用三个场景试过两路：

- **五包裹交易记结果，其中一件包裹反查不到当前已接受委托。** 甲路：`Save` 已成功，逐包裹判到第三件时 `FindCurrentAcceptedByParcel` 答 `ErrParcelTargetNotFound`，本次调用要么中断（余下两件没判、且没有任何东西记得欠了它们）要么吞掉继续（吞掉就是把装配缺陷折成业务答案，ADR-0029 禁）；而调用方——07 出向那一侧——正握着 ADR-0090「答案未确定不得重发」在等回执，它拿到的回执现在还夹着两件包裹的读口故障。乙路：五封信各自消费，第三件那封按 ADR-0029 分格进重投或毒丸，其余四件已判完，结果记录那一步的回执在 `Save` 成功那一刻就干净地交出去了。
- **成功交易被渠道作废，包裹的其余交易早已明确失败。** 若只在 `RecordChannelResult` 触发，作废那一拍不判，关闭路径「已有成功结果均已成功作废」这一格在那一刻由假变真却无人去问；等下一笔交易定案或一份关闭决定来触发——那两件可能永不发生，包裹终局悬着。两拍都触发，作废那一拍多判一次，判断不写、不形成时无副作用。
- **同一包裹两笔交易先后定案，两拍几乎同时到。** 甲路两次调用各自在写路径上判同一包裹，采用路径靠（租户 + 包裹 + 来源种类 + 来源版本）幂等与冲突格兜底，但两次判断的读口结果可能各看到对方写入前的状态。乙路若信封按（租户 + 包裹）分区，同一包裹的两拍在一条队里先后消费，后一拍读到前一拍的全部结果，判断顺序稳定——这正是 `OutboxFinalOutcomeHandoff` 把分区键定成租户加包裹的同一条理由。

## Decision

**一、三种触发在 `parcel-shipment` 一侧都是「收一封指针式信封 → 下一拍判断」，判断不串在任何写路径上；PS 自己发出的两封（定案 / 后续动作、关闭 / 重开决定）由写入方落库同事务入队，一封一包裹，版本进事件 ID。** lc/26 取乙。理由按上面三个场景：判断的 N 个读口故障不得压到 06 编排调用方那一侧的回执上（ADR-0090 那条纪律要的是一份干净的回执）；失败要有载体（inbox 重投）且不后退（ADR-0029 恢复动作分格）；三路同形才能共用一只处理方与一套翻译（决定二）。信封是**指针**不是事实副本：载荷只带（租户、触发对象标识、包裹），判断的输入是「此刻」该包裹的全部相关交易与登记册（CONTEXT「必须跨该包裹全部相关交易及实际承运商收寄事实形成包裹级判断」），消费者**不按信封所指版本读回触发对象**——lc/24 那条「按信封所指版本取、不取 latest」的教训管的是「读回一份有版本链的事实」，这里信封指的是一拍而不是一份要被读回的事实版本，判断读全册本就该读当下。事件 ID 里必须有版本（面单交易那两拍取 `Save` 成功那一代的 `Revision()`，关闭 / 重开取决定标识）：`outboxintent.EnqueueOnce` 先查后插，少了版本，定案那拍与作废那拍算出同一个字符串，第二封静默不入队——与 TF `externalTrackingFactEventID` 头注同一句理由。**一封一包裹**（ID 含包裹，Subject 是包裹）而不是一封一交易：覆盖包裹在建立时固定（ADR-0084 决定二），入队时就知道全部包裹；一封一包裹让每件包裹各自成一次消费——反查不中那一件按格进毒丸或重投，不拖累其余（lc/26 完成判据 2 原句）；分区键取（租户 + 包裹），与 `OutboxFinalOutcomeHandoff` 同一选择，让同一包裹跨交易的多拍在一条队里先后消费（第三个场景）。TF 那一封（lc/25）不由 PS 定形，PS 只坚持它是指针式、带版本、可按版本取回已判断的有效时间（lc/25「前提」节）。

**二、三路共用一只处理方适配器与一套消费结论翻译；各路只有译码与取事实那一层不同。** 处理方的核是一段：按（租户 + 包裹）经 `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 反查当前已接受委托 → 折 `JudgeLabelServiceFinalCommand{Identity, ShipmentRequestID, Parcel[, FirstEffectivePickup]}` → `JudgeLabelServiceFinalHandler.Handle` → 把 `LabelServiceFinalOutcome` 五值译成消费结论。反查不中如实落 `ErrParcelTargetNotFound`（照 `AdoptOnEffectiveDeliveryAdapter`），不在适配器里猜委托。翻译表就是 lc/25 做法 3 那张，按恢复动作分格（ADR-0029）、一格不漏：`LabelServiceFinalAdopted` → 取 `Adoption()` 交 `finalconsume.Consumption`（与有效交付那一路同一处回答终局采用的各格）；`LabelServiceNotFinalOutcome` / `LabelServiceCancellationStandsOutcome` → 已消费（判断到了、不形成，无事可续）；`LabelServiceJudgmentUndecided` → 未决哨兵（四个读口之一答不出，重投会改变结果）；`LabelServiceJudgmentNotAccepted` → 命令立不起，信封与反查都过了还立不起是适配器缺陷，响亮报错不吸收。收寄那一路在核之前多一段「按信封所指版本取回 TF 事实、核键与有效时间、折 `FirstEffectivePickup`」（lc/25 做法 2，含 `ErrDeliveryNotVisible` / `ErrDeliveryRecordInconsistent` 两格的同形分立）；定案与关闭两路不带 `FirstEffectivePickup`，判断走关闭路径。三只 inbox 消费者各有稳定且互不相同的消费者名（inbox 键只由消费者名 + 来源 + 事件 ID 认领，共名会让一路把另一路的投递当重复跳过——`effectiveDeliveryConsumerName` 头注原句），各认自己的事件类型，三维（租户 + 触发对象标识 + 包裹）缺一即毒丸。

**三、面单交易写编排不持 Transactor；事务边界由组合根的事务壳给；handoff 口进 `LabelTransactionDeps`，入队失败即整步失败。** `LabelTransactionDeps` 长一格 handoff 口（形照 `ports.FinalOutcomeHandoff` / TF `ExternalTrackingFactHandoff`：意图与登记同一事务入队），不长 Transactor。同事务由既有约束保证（Context 节核过：仓储与 `EnqueueOnce` 都 `RequireExecutor(ctx)`），事务本身由调用 06 编排的组合根事务壳开（lc/28 取乙之后是面单渠道写链那只组合根，形照 `transactionalWithdrawal`）；不在事务里调用会在 `Save` 处响亮失败，是 fail-closed 不是静默。`advance` 长一个「写后入队」的尾段：`Save` 成功后按覆盖包裹逐件入队；`Save` 失败或版本冲突不入队。**入队失败即本步 error 上抛、事务回滚、调用方重放**——不照 `FormParcelFinalHandler.handOff`「失败不翻结果、留续办引用」那一形：那一形的前提是结果已提交而意图另有续办路径；本记录取乙的全部理由就是不留「结果已落、判断意图丢了」这个中间态，回滚让调用方拿到的回执要么两样都在、要么两样都不在。06 编排头注「它不发起任何渠道调用……只留缝」仍真：handoff 是缝不是渠道调用，06 编排不知道终局判断存在，只知道「这一拍值得有人看一眼」；「五步同一个 handler……缝一条都不少」与「`LabelTransactionDeps` 只有仓储与时钟两项」两句随实施改口（lc/26 完成判据 5）。

**四、定案与后续动作两拍都触发。** `RecordChannelResult`（定案由假变真）与 `AppendFollowUpAction`（作废 / 替代改变关闭路径「已有成功结果均已成功作废」那一格的输入）都入队。理由：判断本身不写、不形成时无副作用；采用按（租户 + 包裹 + 来源种类 + 来源版本）幂等，多判一次不会多形成一次；不触发作废那拍，「成功结果全部作废后沿关闭路径形成终局」要等下一笔交易定案或一份关闭决定，而那两件可能永不发生（第二个场景）。此条为 A 类，推送方已裁，本记录照写以便三路读到同一句。两拍共用同一事件类型（一个处理方、一扇 `inboxconsume` 门认一种类型），载荷可带一格「哪一拍」供追溯，消费者不据它分支——分支就是第二套关闭路径口径。

**五、不做的，逐条写明。** 不动 `JudgeLabelServiceFinal` 的领域判断与 `Finalized()` 谓词；不给 `LabelTransaction` 加任何「已判终局」列（ADR-0084 决定四）；不给判断加定时器或节拍（lc/27「生效 = 追加」已答）；不在触发处按决定种类挑（关闭与重开都触发，分格归判断）；不预判 TF 收寄事实的类别与信封（lc/25 要裁的 1，TF owner）；不写任何真实渠道、账号、结果码（`PAR-INT-02`）；不裁形成关闭 / 重开决定的写面与授权格——那两半各自成票（lc/30、pc-gaps/13），本记录只定它们落地后那一拍的触发形状。

## Consequences

- **lc/26 转 ready-for-agent**：`ports` 加 handoff 口（纯加法）、`adapters/postgres` 新 handoff 文件、`adapters/inbox` 新消费者、处理方适配器（与 lc/25 / 27 共用的核先在此落，收寄那一路日后在它上面加取事实一段）、`cmd/parcel-dispatch/assemble.go` 路由表一行 + `assemble_test.go` 一条、06 编排 `advance` 尾段与两处头注改口；真库实跑（动 outbox 入队）。
- **lc/27 要裁的清零**：触发形状随本记录决定一（乙）；写面拆两半立 lc/30（PS：命令编排 + 入口壳 + 四件从授权答复取）与 pc-gaps/13（PC：授权动作封闭集加关闭、重开两格 + CONTEXT 改句）；lc/27 Blocked by 30 + 13，30 Blocked by 13。
- **lc/25 的 PS 半边**照决定二在共用核上加取事实一段；仍等 TF 侧那张票。
- **lc/28 的组合根**要为 06 编排立事务壳（形照 `transactionalWithdrawal`）并把 handoff 适配器装进 `LabelTransactionDeps`——不装就是 nil 解引用，装配处不得以空实现顶替。
- **窗口如实记**：信封到消费之间，`LabelTransactionParcelRow` 那一行答的是旧终局（越权风险点 1）。
- `production_wiring_baseline.txt` 若因新增生产工厂无调用点而红，按既有纪律加行并写明「等 lc/28 组合根」。

## Alternatives considered

- **甲 · 同一次调用内判。** 否决：今天不是同事务而是同调用非原子，结果已落、判断途中失败无载体；N 个读口故障压到 06 编排调用方的回执上，而那一侧握着 ADR-0090「答案未确定不得重发」在等干净的回执；06 编排从此知道终局判断存在，头注那句「只留缝」要改成「留缝并调判断」。甲的唯一好处（件数少、因果一处可读）由乙的「三路共用一只处理方」补回一半。
- **甲，但先给 06 编排加 Transactor 让它真同事务。** 否决：本仓事务边界一律在组合根事务壳，编排不持 Transactor（`cmd/parcel-api` 全部受控编排壳同形）；即便同事务，N 个判断串在写路径上压回执的问题原样在。
- **一封一交易，消费者逐覆盖包裹展开。** 否决：一件包裹反查不中，整封信进重投，其余包裹被重复判（幂等所以无害，但那一封永远完成不了）；分区键只能按交易，同一包裹跨交易的两拍落在两条队里，第三个场景的顺序保证没了。记为越权风险点「一封一包裹而不是一封一交易」供 owner 复核。
- **只在 `RecordChannelResult` 触发。** 否决：第二个场景——作废那一拍改变关闭路径输入却无人去问。
- **入队失败不翻结果、留续办引用（照 `FormParcelFinalHandler.handOff`）。** 否决：那一形留的正是「结果已落、意图未交」的中间态，是本记录取乙要消掉的东西；同事务里回滚一步的代价只是调用方重放一次结果记录，渠道答案在它手上、不需要重发渠道调用。
- **消费者按信封所指 revision 读回交易再判。** 否决：判断读全册且读当下（CONTEXT 硬句），交易聚合按 ADR-0084 决定八只有当前快照一行、没有可按版本取回的历史；版本在这里的职责只是让两拍各自入队。
- **把「这一拍值不值得判」在入队处先筛（比如成功结果且无作废就不入队）。** 否决：筛就是在 06 编排里复述一遍关闭路径的口径，第二套口径；判断本身便宜且无副作用。
- **不立记录，在 lc/26 票面裁了就做。** 否决：三张票三只编排会各自照抄，触发形状是难逆转的先例（Context 节）；lc/27 的写面票与 lc/28 的组合根都要引同一句。

## 越权风险点

1. **信封到消费之间的窗口，读面答旧终局。** `LabelTransactionParcelRow` 那一行在窗口内仍显交易已定案而包裹终局未变。我按「读面显事实、不显在途判断」不加任何「判断待定」列（那是第二个来源，ADR-0084 决定四同理）；若 owner 认为运营读面要能看出「有一拍在路上」，那是读面票的事，加的是一列从 outbox / inbox 派生的投影，不动聚合与本记录。
2. **「同事务」靠 `RequireExecutor(ctx)` 的既有约束保证，而不是靠编排自己开事务。** 我按本仓组合根事务壳的成例读；不在事务里调用会在 `Save` 处 error，是 fail-closed。若 owner 认为 06 编排该自己守这一条（比如构造期就要 Transactor），改的是 `LabelTransactionDeps` 一格与 lc/28 的装配，本记录决定三改一句。
3. **三路共用一只处理方，是否压扣了 lc/25 收寄与 lc/27 关闭的差异。** 收寄那一路多一段「按版本取回 TF 事实并核有效时间」且命令带 `FirstEffectivePickup`；关闭那一路不带。我把差异放在核之前的取事实一段与命令的一格上，核与翻译表同一份。若 owner 认为收寄形成的终局（非取消终局、以事实有效时间为 `OccurredAt`）与关闭形成的终局（终局失败 / 终局服务结果）在消费结论上该分开翻译，改的是翻译表加行，不是分成两只适配器——五值是同一个 `LabelServiceFinalOutcome`。
4. **一封一包裹而不是一封一交易。** 推送方倾向写的是「租户 + 交易 / 决定标识 + revision」，我在其上加了包裹这一维并把分区键定为租户加包裹（理由在决定一与被否的「一封一交易」那条）。代价是一笔 N 包裹交易每拍入队 N 封。若 owner 取一封一交易，改的是 handoff 适配器的事件 ID 与消费者的展开方式，处理方核不变。
5. **入队失败回滚整步，与 `FormParcelFinalHandler.handOff`「失败不翻结果」不同形。** 我按「乙的全部理由是消掉中间态」取回滚；若 owner 认为两处该同形（都留续办引用），06 编排要长一格续办引用且 `LabelTransactionResult` 要能报「结果已落、意图未交」，决定三改一段。
6. **两拍共用一个事件类型。** 我按「一个处理方一扇门」取；若 owner 要两拍各一个类型以便路由表上可见，消费者要装两只门或一只门认两种类型，翻译表不变。

## Links

- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「面单交易定案」「面单继续尝试决定」词条；Rules「任何交易级结果或单笔交易中的包裹结果都不直接推动包裹终局」「首个面单渠道服务产品默认跨同一包裹的全部相关面单交易判断终局……」「迟到的渠道或运输事实可以按业务发生时间和适用时间改变……终局判断，但不能自行形成关闭或重开决定」；生命周期「包裹身份与服务」两条关闭路径——本记录全部硬句的出处
- [ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)：决定二（覆盖建立即固定——一封一包裹的前提）、决定四（定案是派生谓词——不加已判列）、决定八（当前快照一行——不按版本读回）
- [ADR-0090](./0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)：「答案未确定不得重发」——06 编排调用方等回执那一格为何不能被判断拖住
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：消费结论按恢复动作分格
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：意图由结果标识认领——版本进事件 ID 的出处
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：收寄那一路取 TF 事实的适配器落 PS 侧
- `internal/parcelshipment/application/operate_label_transaction.go`（`LabelTransactionDeps` / `LabelTransactionHandler` 头注、`advance`）、`judge_label_service_final.go`（`LabelServiceFinalOutcome`、`JudgeLabelServiceFinalCommand`）、`form_parcel_final.go`（`handOff`——被否的那一形）、`adapters/postgres/final_outcome_handoff.go`（分区键取租户加包裹的理由）、`adapters/postgres/label_transaction.go`（`Insert` / `Save` 的 `RequireExecutor`）、`internal/platform/outboxintent/enqueue_once.go`、`adapters/inbox/effective_delivery_consumer.go`（消费者名与三维毒丸的先例）、`adapters/transportfulfillment/adopt_on_effective_delivery.go`（处理方适配器先例）、TF `adapters/postgres/external_tracking_fact_handoff.go`（`externalTrackingFactEventID` 头注）、`cmd/parcel-api/assemble_withdrawal.go`（事务壳形状）
- 票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md)（做法 2 / 3——取事实一段与翻译表的出处）、[lc/26](../../.scratch/label-channel-service-first-release/issues/26-label-transaction-settlement-beat-triggers-label-final-judgment.md)（两路代价的出处、本记录决定一 / 三 / 四的驱动票）、[lc/27](../../.scratch/label-channel-service-first-release/issues/27-controlled-close-reopen-decision-triggers-label-final-judgment.md)（「生效 = 追加」、写面拆两半）、[lc/28](../../.scratch/label-channel-service-first-release/issues/28-channel-selection-composition-root-and-call-entry.md)（取乙：整条写链一个组合根——06 编排的事务壳落在那里）、[lc/30](../../.scratch/label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md)、[pc-gaps/13](../../.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)
