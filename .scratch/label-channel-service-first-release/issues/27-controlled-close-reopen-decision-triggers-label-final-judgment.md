# 27 受控关闭 / 重开决定生效 → `JudgeLabelServiceFinalHandler`：决定口今天连写入方都没有，先要那张写面票

Category: enhancement
Status: resolved——2026-09-11 11:5x（作者自标）通道 4 落地（task-c2983660；分支 `mcp4-lc27` 基远端 main `b68baddf`（代码 = `2c7326ef`），代码 tip `868725ca`、清点 `70d77d91`）：PS inbox 消费者 + 接共用核的处理方适配器 + `parcel-dispatch` 路由；完成判据 1–7 逐项见 Comments 末条（1、6 由 `30` 落地只核对）；带 DSN 三包 ok；进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-11 11:4x 通道 4 认领（task-c2983660，通道 1 派单；隔离树 `D:/tops/idp-parcel-mcp4-lc27`）。**两张 Blocked by 已解**（`30` 2026-09-11 11:1x 进 main、pc-gaps/13 2026-09-10 20:3x 进 main），由 draft 直转 in-progress、经 ready 不停——要裁的早已清零，票面只等它们落地。写面那半（`Save` / `Insert` 后同事务入队、事件 ID 含决定标识、分区键租户 + 包裹、入队失败整步回滚、`ContinuedAttemptRegisterRepository` 头注改口）已由 `30` 落地，本票只核对不重做；本票做消费者半边。此前 draft——**要裁的已清零**（2026-09-10 通道 2 按通道 1 派单 task-138ab1c9 裁，见「裁决」），转 ready-for-agent 只等 Blocked by 两张落地；此前 draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** 触发本身很小，但它挂的那一拍（决定被追加进登记册）今天没有生产路径能到，见 Blocked by
Blocked by: [`30`](./30-controlled-close-reopen-decision-write-face-ps-half.md)（PS 半边：形成受控关闭 / 重开决定的命令编排 + 入口壳 + 授权适配器）、[pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)（PC 半边：授权动作封闭集加关闭、重开两格）；`30` 自身 Blocked by 13。触发形状已随 [`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md) 的裁决定为**乙**（[ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md) 决定一），不另裁

## 缺口

票 [`11`](./11-parcel-final-across-transactions.md) Answer「不在本票」节的第三路：**受控关闭 / 重开决定生效**——票 [`10`](./10-continued-attempt-decision-registry.md) 的决定口。lc/11 当时就写了「那口自身还没有生产写入方」；本票取证坐实它，并把「没有」量到符号名。

**决定口今天有什么。** `ports.ContinuedAttemptRegisterRepository{FindByParcel, Insert, Save}`，postgres 实现 `adapters/postgres/continued_attempt_register.go`（`NewContinuedAttemptRegisters`，PS 迁移 `0012`）；领域 `ContinuedAttemptRegister.Append(spec, currentFinalPresent)` 收两种决定 `ControlledClosureDecision` / `ReopeningDecision`，各自必备项由 `decisionFrom` 逐条把守（关闭独有截断边界与关闭责任来源；重开须关联此前关闭且生效时间严格晚于它）。`Append` 头注：「本方法不判断授权够不够格。授权规则属 party-commercial……它也不会自动形成任何决定——CONTEXT 明写『系统不得自动形成』」。

**决定口今天没有什么。** `internal/parcelshipment/application/` 里 `ContinuedAttempt` 只出现在 `judge_label_service_final.go` 及其测试——那是**读**侧（`ContinuedAttemptRegisterView.FindByParcel`）。没有任何编排调 `Append` / `Insert` / `Save`；`cmd/` 下 `ContinuedAttemptRegister` 零命中；无 HTTP 端点、无 CLI、无管理台写面。端口头注自己写着：「**本口今天没有生产写入方，这是设计而不是欠账**：形成关闭或重开决定的命令口要先过 party-commercial 的授权规则校验……那是另一张票」；lc/10 完成记录「刻意没做」同一句。**那「另一张票」至今没立**——`.scratch/` 搜「关闭决定 / 重开决定 / 受控关闭」只命中 lc/10、lc/11、本目录 spec、余工表与几处读面票，没有一张是写面。

**余工表的一句已过期，顺手更正。** `unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条写「`RehydrateContinuedAttemptRegister` 仍在 `production_wiring_baseline.txt`」。在 `c7e3522c` 上它**不在名单**：基线文件头部流水账记 2026-09-03 随 lc/10 仓储适配器落地剪掉（「加它时写的『读它的仓储适配器落地那天这一条出名单』兑现」）；名单里的 PS 条目今天没有它。那句话在 09-04 取证时对，此后变旧；本票不改余工表（通道 4 在重核），只记在这里。

**「生效」是哪一拍——代码已答，不必裁。** `ContinuedAttemptRegister.standingClosure()` 顺序扫决定：关闭置位、重开清位，**不看时钟、不比生效时间与当下**；`Judge` 与 `StandingClosure` 都据它。所以在判断眼里，一份关闭决定**被追加进登记册那一刻**即为「生效且未被重开」；`EffectiveAt` 是决定的业务生效时间，终局形成时它成为 `verdict.EffectiveAt()` → 责任结果的 `OccurredAt`，不是一个要等到才翻转的开关。因此本票的触发点就是写面 `Save` 成功那一拍，不需要定时器或节拍去等某个未来时刻。

**重开那一拍也触发但预期不形成。** 重开清掉生效关闭后，`JudgeLabelServiceFinal` 走到关闭路径会答 `NOT_FINAL`（原因 `ContinuedAttemptStillOpen`）；判断不写、不形成时无副作用，多判一次是安全的。两种决定都触发，判断自己分格——不在触发处按决定种类挑，挑了就有第二套关闭路径口径。

## 做法（`30` + pc-gaps/13 落地后才可开工；触发形状已定为乙，按 ADR-0134）

`30` 的写面编排在 `Save` 成功之后多一个写后入队的尾段（形与 `26` 做法第 3 步同：deps 长一格 handoff 口、不持 Transactor、事务由 `30` 的入口壳开、入队失败即整步回滚）：落库同事务经 `outboxintent.EnqueueOnce` 入队一封 `parcel-shipment.continued-attempt-decision.` 前缀的指针式信封——载荷（租户、包裹、决定标识），事件 ID 含决定标识（每条决定各自入队，关过—重开—再关三封各自成封），Subject = 包裹，分区键 = 租户 + 包裹（同一包裹的关闭、重开与交易多拍在一条队里，ADR-0134 决定一）。登记册键就是租户 + 包裹，一册一件，一封一决定即一封一包裹，不必展开。

PS inbox 消费者收它（稳定消费者名与 `26` / 交付各路不同；三维租户 + 包裹 + 决定标识缺一毒丸）→ 交给与 `26` **共用的处理方核**：按（租户 + 包裹）`CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 取委托来源身份与委托标识 → 折 `JudgeLabelServiceFinalCommand{Identity, ShipmentRequestID, Parcel}`（不带 `FirstEffectivePickup`，走关闭路径）→ `Handle` → `LabelServiceFinalOutcome` 五值译成消费结论（`25` 做法 3 那张表，ADR-0134 决定二）。**不读回登记册**：判断自己读全册且读当下，信封里的决定标识只用于幂等与追溯。

采用幂等：关闭路径形成终局时来源版本 = 生效关闭决定的标识（`responsibilityOutcomeOf`：`CONTINUED-ATTEMPT-CLOSURE/<决定标识>`），同一份关闭重放返原；关过—重开—再关是新决定标识，走重派生。重开那一封消费到 `NOT_FINAL`（`ContinuedAttemptStillOpen`）→ 已消费。

## 要裁的（已清零）

1. **写面票立不立、立在哪、拆几半。** → **已裁，见「裁决」。** 它至少两半：**PS 半边**——形成关闭 / 重开决定的命令编排（读当前有效终局作 `currentFinalPresent`、开册或读回、`Append`、`Insert` / `Save`）+ 入口（端点或管理台，形照 `cmd/parcel-api` 既有受控编排壳）+ 请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取，PS 不自判；**PC 半边**——授权动作封闭集今天三格（人工复核 / 主动拒绝 / 资料修订，ADR-0116 决定一「一票一格」），关闭与重开**没有格**，要先改 PC CONTEXT 授权那句再加格，形照 ADR-0116 与 pc-gaps/08 那一路；PS 适配器照 `WithdrawalAuthorizationAdapter` 形。归哪个目录（本目录 / ps-port-remainder / party-commercial-context-gaps）、PC 半边要不要 ADR（ADR-0116 的先例是「加格不要，改裁定形状要」）归派单方与 owner。**本票不揉进这两半**——lc/10 与端口头注都把写面判为另一张票，本票只是它落地后的一拍。

## 裁决

- **谁 / 何时 / 口径**：通道 2，2026-09-10，按通道 1 派单 task-138ab1c9；用户 17:0x 经队列授权「你自决」，B 类按 PS owner 口径代裁，PC 半边的落位照 [ADR-0116](../../../docs/adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 与 pc-gaps/08 既有那一路。
- **要裁的 1 → 立，拆两半**：
  - **PS 半边 → [`30`](./30-controlled-close-reopen-decision-write-face-ps-half.md)**（本目录）：形成关闭 / 重开决定的命令编排（读当前有效终局作 `currentFinalPresent`、开册或读回、`Append`、`Insert` / `Save`）+ 入口壳照 `cmd/parcel-api` 既有受控编排壳（`transactionalWithdrawal` 那一族）+ 请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取、PS 不自判，适配器照 `WithdrawalAuthorizationAdapter` 形；`Save` 成功后的触发尾段随 ADR-0134 乙（本票「做法」）。
  - **PC 半边 → [pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)**：`AuthorizedAction` 封闭集加「受控关闭」「重开」两格，改 PC CONTEXT Rules 授权动作那一句（ADR-0116 加的「资料修订是与人工复核、主动拒绝并列的授权动作」那句之后接一句），照 ADR-0116 决定一「一票一格」与 pc-gaps/08 那一路；**不另写 ADR**——ADR-0116 先例「加格不要 ADR，改裁定形状要」，两格都是加格，票面自带 CONTEXT 改句。
  - **阻塞边**：本票 Blocked by 30 + 13；30 Blocked by 13（授权格先有，PS 适配器才有动作可请求）。
  - **两张新票的成熟度**：`30` 与 pc-gaps/13 各自票面里仍有「要裁的」——不是本票的问题，是写面自己的：「同级或更高」在权限等级为不透明串的今天怎么判、货主指令关闭后重开要「同一货主账户新的有效授权」怎么问 PC、货主经合同直接形成决定那一支首发开不开。三条都是 PC 拥有的「关闭与重开的条件」规则（PC CONTEXT Boundaries 原词），倾向写在各票面，归派单方 / owner 裁；裁完两票转 ready-for-agent，本票随之。
- **触发形状 → 乙**（随 `26`，ADR-0134 决定一 / 二 / 三），「做法」已按乙写实；「生效 = 追加」与「两种决定都触发」两条本票原有判断不变，ADR-0134「不做的」一节照录。

## 红线

- 不自动形成任何关闭或重开决定（CONTEXT「系统不得自动形成」；`Append` 头注）；本票只在人形成的决定落库之后判终局。
- 不动 `standingClosure` / `Judge` 的派生规则；「生效」以追加为准，不加时钟、不加定时重判。
- 不在触发处按决定种类挑：两种决定都触发，分格归 `JudgeLabelServiceFinal`。
- 不动 `internal/partycommercial/**`（PC 半边归写面票）。

## 完成判据（非作者评审逐项对）

1. 写面编排 `Save` 成功后 outbox 里一封、事件 ID 含决定标识、分区键 = 租户 + 包裹；`Save` 失败 / 版本冲突时不入队；入队返错 → 整步 error 且决定未落（事务回滚）——各有用例，真库实跑。
2. 关闭决定后：全部交易明确失败的夹具 → 采用路径收到 `LABEL_SERVICE_FAILURE`；成功结果全部作废的夹具 → 收到 `LABEL_SERVICE_OUTCOME`；仍有有效成功结果 → `NOT_FINAL`。重开决定后 → `NOT_FINAL`（`ContinuedAttemptStillOpen`）。四格各有用例。
3. 同一份关闭重放 → 采用返原（来源版本 = 决定标识）。
4. `LabelServiceFinalOutcome` 五值逐格译成结论，与 `25` / `26` 同一张表（共用适配器则用例证共用）。
5. 消费者三维缺一毒丸、事件类型不符拒收、消费者名与 `26` / 交付各路不同；`cmd/parcel-dispatch/assemble.go` 路由表有该类型；`assemble_test.go` 补一条。
6. `ContinuedAttemptRegisterRepository` 头注「本口今天没有生产写入方」随 `30` 落地改口（归 `30`），本票核对它已改、不留旧话。
7. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库（本票动 outbox 入队，真库必须实跑）。

## 地盘

`30` 落地后的那只编排文件（届时占号）；`internal/parcelshipment/adapters/postgres/`（新 handoff）、`internal/parcelshipment/adapters/inbox/`（新消费者）、与 `26` 共用的处理方适配器所在包（在 `26` 落下的核上加口）、`cmd/parcel-dispatch/`；本票面。**不动** `domain/**`、`internal/partycommercial/**`、`cmd/parcel-api/**`（入口壳归 `30`）。

## 参照

票 `11` Answer「不在本票」节；票 `10` 完成记录「刻意没做」；`internal/parcelshipment/ports/ports.go` 的 `ContinuedAttemptRegisterRepository` / `ContinuedAttemptRegisterView` 头注；`internal/parcelshipment/domain/continued_attempt_register.go`（`Append` 头注、`decisionFrom`、`standingClosure`、`Judge` 头注）；`internal/parcelshipment/domain/continued_attempt.go`（`ContinuedAttemptDecisionSpec`、两种决定）；`internal/parcelshipment/adapters/postgres/continued_attempt_register.go`；`internal/parcelshipment/application/judge_label_service_final.go`（`responsibilityOutcomeOf` 关闭那一支）；`internal/architecture/production_wiring_baseline.txt` 头部 2026-09-03 那条流水；PS CONTEXT「面单继续尝试」生命周期与「形成关闭或重开决定时仍须重新校验当前角色与客户授权」；ADR-0084 决定六；ADR-0116 决定一；`adapters/partycommercial/withdrawal_authorization.go`（写面 PS 半边的授权适配器先例）。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/10 全文、`ContinuedAttemptRegisterRepository` / `View` 头注、`continued_attempt_register.go` 的 `Append` / `decisionFrom` / `standingClosure` / `Judge`、`continued_attempt.go` 的 spec 与两种决定、基线文件头部流水与名单、`application/` 全目录 grep；**没读** `adapters/postgres/continued_attempt_register.go` 全文与 `label_transaction_views.go` 的读面派生。「写面无票」按 `.scratch/` 全目录 grep 判——若有人在别的目录以别的词立过，以那张为先，本票 Blocked by 改指过去。
- 2026-09-10 · 通道 2（task-138ab1c9，分支 `mcp2-adr0134` 基 `062f5228`；只写票面，未动代码）：**要裁的 1 裁为「立，拆两半」——[`30`](./30-controlled-close-reopen-decision-write-face-ps-half.md)（PS 写面）+ [pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)（PC 授权格两格，不另写 ADR）；触发形状随 `26` 取乙，落 [ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)。** 本票要裁的清零，Blocked by 改指 30 + 13，Status 仍 draft（等两张落地即可转 ready-for-agent，不必再裁）。「做法」按乙写实、完成判据去掉甲支。两张新票各自带的「要裁的」（同级或更高怎么判、货主指令关闭后重开的授权怎么问、货主直接形成决定首发开不开）是写面自己的问题，已在两票面列出并报通道 1，不回流到本票。**能力边界**：没读 `adapters/postgres/continued_attempt_register.go` 全文（同上一条），也没读 `AdjudicateCommercialAuthorizationHandler` 的装配处——`30` 的授权适配器形状按 `withdrawal_authorization.go` 头注与 ADR-0116 决定三 / 四写。
- 2026-09-11 11:5x · 通道 4（task-c2983660；分支 `mcp4-lc27` 基远端 main `b68baddf`，代码 = `2c7326ef`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证）：`510ce2cb` docs Status draft → in-progress；`7bcbacc7` feat PS inbox 消费者 `ContinuedAttemptDecisionJudgmentConsumer` + labelfinal 适配器 `ContinuedAttemptDecisionJudgmentAdapter`（含两包测试）；`868725ca` feat `cmd/parcel-dispatch` 路由表 + 装配函数 + 探针，lc/26 块抽出 `labelFinalJudgmentCore`；`70d77d91` docs 机制清点在 `868725ca` 干净检出重生成；本笔票面 + lc spec 27 行。代码 tip `868725ca`。
  **动过的 .go**：新增 `internal/parcelshipment/adapters/inbox/continued_attempt_decision_consumer.go`（+ `_test.go`）、`internal/parcelshipment/adapters/labelfinal/continued_attempt_decision_judgment_adapter.go`（+ `_test.go`）；改 `internal/parcelshipment/adapters/labelfinal/judge_parcel_test.go`（`recordingJudge` 顺带记结果）、`cmd/parcel-dispatch/assemble.go`（哨兵名单一条、`wireDispatcher` 一组、路由表一行、`labelFinalJudgmentCore` 抽出 + 新装配函数；gofmt 重排路由表 map 对齐列，各行只动空白）、`cmd/parcel-dispatch/assemble_test.go`（探针一条）。
  **完成判据逐项**：
  1. ✓（**由 `30` 落地，核对如下**）写面 `FormContinuedAttemptDecisionHandler.form` 在 `Insert` / `Save` 成功之后经 `ContinuedAttemptDecisionHandoff` 入队，`OutboxContinuedAttemptDecisionHandoff` 事件 ID = `<租户>/continued-attempt-decision/<包裹>/<决定标识>/judgment-due`（含决定标识）、分区键 = 租户 + 包裹、`EnqueueOnce` 同事务；版本冲突 / 开册撞车两格在入队之前返回不入队；入队失败原样上抛整步回滚。真库：`TestTheWiredContinuedAttemptDecisionWalksCloseAndReopenAgainstARealDatabase`（关闭一封、重开再一封、换会失败的交接口 → error 且册无行）与 `continued_attempt_decision_handoff_test.go` 三例（事务模板、三决定同区各自成封、残缺意图拒）。本票未改一字。
  2. ✓ 关闭之后四格经本票适配器 → 共用核 → 真 `JudgeLabelServiceFinalHandler` 判出：全部交易明确失败 → `LabelServiceFailure`；成功结果全部作废 → `LabelServiceOutcome`；仍有可用成功结果 → `NOT_FINAL`（`UsableLabelResultOutstanding`）；重开之后 → `NOT_FINAL`（`ContinuedAttemptStillOpen`）——`TestAClosureDecisionIsJudgedIntoTheFourContextGrids` 四子例；形成终局的两格交采用路径、不形成的两格采用零调用。分格由判断答，适配器与消费者都不读 kind。
  3. ✓ 同一份关闭信封重放两次，交给采用路径的来源版本同为 `CAD-1`、执行证据同为 `CONTINUED-ATTEMPT-CLOSURE/CAD-1`；关过—重开—再关（`CAD-3`）来源版本换成 `CAD-3`（`TestReplayingOneClosureHandsTheSameSourceVersionAndANewClosureHandsANewOne`）。「返原」本身是 `FormParcelFinalHandler` 按来源版本的既有行为（lc/11 / lc/25 用例），本票证交给它的键没变。
  4. ✓ 五值翻译经同一只 `labelfinal.ParcelJudgmentCore` → `Consumption`，本票没有自己的翻译表；`TestTheContinuedAttemptAdapterSharesTheCoreTranslation` 证读口答不出 → `ErrJudgmentUndecided`、反查不中 → `ErrParcelTargetNotFound` 与 26 同一哨兵；`cmd/parcel-dispatch` 未决哨兵名单 `continuedAttemptDecisionJudgmentUndecidedSentinels = labelTransactionJudgmentUndecidedSentinels` 同一张。
  5. ✓ 消费者名 `parcel-shipment/judge-label-final-from-continued-attempt-decision`（与 26 `…-from-transaction` / 交付 `form-final-from-delivery` 不同；`TestTheContinuedAttemptDueConsumerKeepsASeparateInboxAccountFromTheTransactionOne` 同 source 同事件 ID 两扇门各处理一次）；三维缺一 + 非 JSON 毒丸四子例；异类型响亮拒收含 26 同族 `label-transaction.judgment-due`；类型串与写侧导出常量相等（`TestTheContinuedAttemptDueConsumerAcceptsTheTypeTheWriterEmits`）；路由表有 `psinbox.ContinuedAttemptDecisionJudgmentDueEventType` 一行；`assemble_test.go` 补 `TestAContinuedAttemptDecisionJudgmentDueReachesTheConsumerThroughTheRouteTable`（真库，毒丸定稿 published = 1）。
  6. ✓（**由 `30` 落地，核对**）`ContinuedAttemptRegisterRepository` 头注已改口指向写面（`f6cf9d03`→main），旧句不在它头上。`git grep '本口今天没有生产写入方' -- ports.go` 仍一处命中——在 `LabelTransactionRepository`（06 的口）头上，lc/28 装配之后它也陈旧，`30` 完成记录已记为头注改口那一族的后继，不在本票地盘。
  7. ✓ `gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -count=1` `internal/parcelshipment/adapters/inbox`（11:42:22→11:42:27）+ `cmd/parcel-dispatch` + `internal/architecture/...`（11:45:56→11:46:03）三包 ok，`internal/parcelshipment/adapters/labelfinal` 不需 DSN ok；路由探针两例 -v PASS / SKIP 0；**未跑全量**（派单原句，推送方全量）。清点在 `868725ca` 干净检出重生成（`70d77d91`）。
  **取舍两处（供评审）**：① 事件类型串——`30` 为本票导出了 `pspostgres.ContinuedAttemptDecisionEventType`，但 inbox 包八个消费者都自写 `eventing.EventType` 常量、无一 import PS postgres 适配器（lc/26 消费者头注「消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量」）；本票按 inbox 惯例自写，两串相等由消费者测试钉（测试包 import `pspostgres` 只作对照），路由表用 `psinbox` 常量与 26 行同形。`30` 那句「导出是给 27 的消费者与路由表引同一个串」于是只兑现了测试那一半，头注要不要改口归 `30` 的后继 / PS owner。② `judgeLabelFinalOnLabelTransactionConsumer`（lc/26 块）被抽出 `labelFinalJudgmentCore`——不抽就是把判断编排六口的装配抄第二份；两路各装一只核实例（各持自己的采用链），不共享状态。
  **红线核**：不自动形成任何决定（消费者只在信封到达后判终局，无节拍、无写决定的口）；未动 `standingClosure` / `Judge`；不按种类挑（kind 不进译码、适配器不读）；未动 `internal/partycommercial/**`、`domain/**`、`cmd/parcel-api/**`、`application/form_continued_attempt_decision.go`。新增两个构造器构造期拒 nil（消费者三口、适配器一口，各有用例）。
  **地盘外零改动**：`git diff b68baddf -- internal/parcelshipment/domain internal/partycommercial cmd/parcel-api internal/parcelshipment/application` 零行；`assemble.go` 的 `receiveExternalFundsFactConsumer`（sa-cc/14 通道 5 在途块）及其头注未动。
  **不做的**：端到端真库「关闭 → 入队 → 派发 → 判 → 终局落行」一条龙（写侧真库在 `cmd/parcel-api`、消费侧真库在 `cmd/parcel-dispatch` 路由探针 + inbox 门，两半各证；一条龙要在一棵树里同时装两只组合根，归后继验收票）；接入面 Intake；管理台。
  **能力边界**：/tdd 的红在消费者与适配器两段可见（`go vet` 报 undefined 后转绿）；`cmd/parcel-dispatch` 路由探针与路由行同笔、红未单独可见。没读 `finalconsume` 全文，采用各格按其哨兵别名对接；没读 `FormParcelFinalHandler` 的返原实现，判据 3 只证键不变。
