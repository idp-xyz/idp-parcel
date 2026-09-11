# 32 06 `Establish` 核继续尝试登记册：关闭生效后拒绝把该包裹纳入边界后的新交易

Category: enhancement
Status: in-progress——2026-09-11 11:3x 通道 3 按通道 1 派单 task-287429ec 认领，分支 `mcp3-lc32` 基远端 main `2c7326ef`（隔离树 `D:/tops/idp-parcel-mcp3-lc32`），按「做法」1–6 与「完成判据」1–6 走 `/implement`。此前 ready-for-agent——2026-09-10 17:5x 通道 2 按通道 1 派单 task-3ebcdc45 立票（通道 1 推送方代裁「缺门无票 → 立 32」，用户授权）；取证锚远端 main `062f5228`；**只写票面，未动代码。** 要裁的为零：形状由派单裁定（`Establish` 前读登记册当前有效关闭，在场则拒、无册照旧；不动 [`30`](./30-controlled-close-reopen-decision-write-face-ps-half.md) 的写面），余下的都是 06 编排既有代数与 lc/10 既有读口上的照抄
Blocked by: 无——[`30`](./30-controlled-close-reopen-decision-write-face-ps-half.md) 已 2026-09-11 11:1x 进 main `2c7326ef`（原阻塞理由：边界先能形成——没有写面，登记册上永远没有关闭，这道门开了也永远不关）。**不阻塞但相关**：[`28`](./28-channel-selection-composition-root-and-call-entry.md) 已进 main，组合根已装配 `LabelTransactionDeps`，本票在其上补两格；触发面归 `34`，落地前本门仍无生产事件流过，与 `26` 同款诚实句

## 缺口

PS CONTEXT Rules 硬句：「权威业务截断边界前已经形成交易建立决定或已经提交的交易……仍属于既有交易并继续参与终局判断；**关闭生效后只拒绝把该包裹纳入边界后的新重试、替代或换单交易**，不阻断既有交易的查询、确认、重打、渠道作废、渠道退款、对账和定案」；生命周期「面单继续尝试」：「开放 → 受控关闭：……边界后的新尝试被拒绝，边界前既有交易继续处理和定案」；「多包裹交易中，目标包裹关闭不关闭其他包裹或整笔交易」。

**代码里没有守卫。** `LabelTransactionHandler.Establish` 今天只做三件：按标识读原交易建关系（`priorLink`）→ `domain.EstablishLabelTransaction` → `Insert`；`LabelTransactionDeps` 只有 `Transactions` 与 `Clock`——它不读登记册，也不读当前有效终局。一个包裹被受控关闭之后，任何调用方仍能为它建立新交易并推到`已提交渠道`，硬句靠调用方自觉。lc/30 立票时量到这道缺门（其「要裁的」1 原文），通道 1 裁「立 32」。

**读口已在。** `ports.ContinuedAttemptRegisterView.FindByParcel`（lc/10 落的只读半边，`pspostgres.NewContinuedAttemptRegisters` 同一适配器满足）；`domain.ContinuedAttemptRegister.Judge(currentFinalPresent)` 现算`包裹级继续尝试判断`两格（`开放` / `受控关闭`），`StandingClosure()` 交回生效的那份关闭本体；`ports.FinalOutcomeStore.FindCurrentFinal` 答当前有效终局（`record.Finalized`）。**判断口径只有一个**：`Judge` 的头注写明规则逐字取自 CONTEXT「当前无有效终局且没有生效关闭时派生为开放，仍有生效关闭时保持受控关闭」；本票不另判，只调它。

## 做法

1. **`LabelTransactionDeps` 长两格只读口**：`Registers ports.ContinuedAttemptRegisterView`、`Finals`（`FinalOutcomeStore` 的只读半边，与 `JudgeLabelServiceFinalDeps` / `FormParcelFinalDeps` 用同一个适配器）。头注随改：这两口只在 `Establish` 用、只读，不是第二处口径——`Establish` 之后四步一律不核册（硬句「不阻断既有交易的……定案」）。
2. **`Establish` 在 `priorLink` 之后、`domain.EstablishLabelTransaction` 之前多一步**：对 `CoveredParcels` 逐件——`Finals.FindCurrentFinal` 取 `currentFinalPresent`（读不回 → error 上抛，不吸收）→ `Registers.FindByParcel`（未开册即 `OpenContinuedAttemptRegister` 空册，照 `JudgeLabelServiceFinalHandler.Handle` 那一行）→ `register.Judge(currentFinalPresent)`；任一件为`受控关闭` → **拒绝整笔建立、不 `Insert`**。用 `Judge` 而不是只看 `StandingClosure()`，是因为 CONTEXT「开放」的定义本就含「当前不存在有效终局」——当前有效终局在场的包裹同样不允许新尝试（「当前有效终局服务结果存在时……后续新服务需求进入关联的新委托」），而 `Judge` 是那一格的单一权威；只看关闭那一支就是在本编排里复述一半口径。
3. **结果代数加一格** `LabelTransactionParcelNotOpen`（`PARCEL_CONTINUED_ATTEMPT_CLOSED`）：恢复动作是「去掉该包裹另建、或走 `30` 申请重开、或把新服务需求进关联的新委托」，与 `LabelTransactionNotAccepted`「改输入重来」不是同一个动作（ADR-0029 按恢复动作分格），也不并进 `StepNotAdmitted`（那一格说的是**这笔交易**此刻的状态，这里交易还不存在）。`LabelTransactionResult` 加一个纯加法访问器交回被拒的包裹清单——恢复动作要知道是哪几件；建立被拒时交易本体照旧缺席（`labelTransactionRefused(..., nil)` 那一句的理由）。
4. **重放不受影响**：撞键读回既有那一笔的路径在 `Insert` 之后，核册在 `Insert` 之前——一笔边界前建立的交易被重放建立时会先撞核册；为免把「重放」误报成「关闭中」，**先按标识 `FindByID` 命中即走既有重放分支，再核册**（读一次的代价换一条硬句：「边界前……仍属于既有交易」）。
5. **并发窗口如实记，不在本票关**：`Establish` 读册与 `30` 的关闭 `Save` 各在自己的事务里，读到「无关闭」之后关闭才提交的那一窄格，本门拦不住；CONTEXT 为它另备了一句「关闭期间若仍发现已经实际提交的边界后交易，PS 必须形成违反截断边界的业务判断并保留该交易」——那是一次事后判断，需要建立决定与关闭决定之间的稳定领域顺序，**另一张票**（今天无票，记在 Comments），本票不加行锁、不给交易加「观察到的册版本」出生属性（那要动 ADR-0084 决定二固定的清单）。
6. **头注**：`Establish` 头注补一句为什么核册在这一步、为什么之后四步不核；`LabelTransactionDeps` 头注「只有仓储与时钟两项」若 `26` 已改口则在其上再改，若 `26` 未落则本票改口并注明 `26` 会再加 handoff 一格（两票同文件，占号）。

## 红线

- 只拒新建立，不动 `SubmitToChannel` / `MarkResultUncertain` / `RecordChannelResult` / `AppendFollowUpAction` 四步——既有交易的处理与定案不受关闭阻断（硬句）。
- 不在编排里复述关闭路径口径：`Judge(currentFinalPresent)` 是唯一判断，本票不自己看决定种类、不比时间。
- 不动 `domain/**`（`Judge` / `StandingClosure` / `EstablishLabelTransaction` 原样）；不给 `LabelTransaction` 加任何「建立时册版本」列。
- 读口读不回 → error 上抛，不译成「开放」放行，也不译成「关闭」拒绝——fail-closed 是拒绝建立这一步本身（返错），不是猜一格。
- 不写任何真实渠道、账号、结果码。

## 完成判据（非作者评审逐项对）

1. 包裹有生效关闭 → `Establish` 答 `PARCEL_CONTINUED_ATTEMPT_CLOSED`、不 `Insert`、结果带该包裹；多包裹只一件关闭 → 整笔拒且清单只含那一件；关过—重开 → 建立成功；当前有效终局在场（无关闭）→ 同样拒。
2. 无册 → 建立照旧成功（空册派生开放）；读口返错 → error 上抛且不 `Insert`。
3. 同标识重放：交易已存在（边界前建立）→ 走重放分支交回既有，**不**因册上现有关闭而答拒绝。
4. 后四步对关闭中的包裹照旧可走（`RecordChannelResult` / `AppendFollowUpAction` 用例夹具里放一份生效关闭）。
5. 两处头注改口、不留旧话；`gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿（本票不动 `.sql`，替身即可，注明含不含真库）。
6. 完成记录写明：`LabelTransactionHandler` 在 `cmd/` 仍无调用方（等 `28`）；并发窄格与「违反截断边界的业务判断」后继票无票，原句照录。

## 地盘

`internal/parcelshipment/application/operate_label_transaction.go` 与其测试（与 `26` 同文件——两票同刻开工要占号，`26` 动 `advance` 尾段与后两步，本票动 `Establish` 与 `Deps` 两格；头注那一句谁后落谁合）；`cmd/parcel-dispatch/` / `cmd/parcel-api/` 若 `28` 已装配 `LabelTransactionDeps` 则补两格（否则不动）；本票面。**不动** `domain/**`、`ports/**`（既有读口够用）、`internal/partycommercial/**`。

## 参照

PS CONTEXT Rules「权威业务截断边界前……关闭生效后只拒绝……」「多包裹交易中，目标包裹关闭不关闭其他包裹或整笔交易」「关闭期间若仍发现已经实际提交的边界后交易……」、生命周期「面单继续尝试」；`internal/parcelshipment/application/operate_label_transaction.go`（`Establish` / `priorLink` / `LabelTransactionDeps` 头注 / `labelTransactionRefused`）；`internal/parcelshipment/domain/continued_attempt_register.go`（`Judge` 头注、`StandingClosure`、`OpenContinuedAttemptRegister`）；`internal/parcelshipment/application/judge_label_service_final.go`（未开册即空册那一行）；`ports.ContinuedAttemptRegisterView` / `ports.FinalOutcomeStore`；ADR-0029；ADR-0084 决定二（为什么不加出生属性）；lc/10、lc/30「裁决」、ADR-0134 决定五（不做的）。

## Comments

- 2026-09-10 17:5x · 通道 2（task-3ebcdc45，分支 `mcp2-adr0134` 基 `062f5228`）：立票。**只写票面，未动代码。** 形状由通道 1 派单裁定，本票在其上定了三件都是 06 编排既有代数上的照抄：用 `Judge(currentFinalPresent)` 而不是只看关闭（单一口径，因此 Deps 多 `Finals` 一口）、结果代数加一格并交回被拒包裹清单（ADR-0029）、重放先于核册（硬句「边界前……仍属于既有交易」）。**后继无票**：并发窄格的事后判断——「关闭期间若仍发现已经实际提交的边界后交易，PS 必须形成违反截断边界的业务判断并保留该交易」——需要建立决定与关闭决定间的稳定领域顺序，本票明写不做，归派单方。能力边界：读过 `operate_label_transaction.go` 全文与 `continued_attempt_register.go` 的 `standingClosure` / `StandingClosure` / `Judge`；**没读** `FinalOutcomeStore` 的 postgres 实现（`FindCurrentFinal` 的只读半边按 `FormParcelFinalHandler.deriveCompletion` 的用法判）。
