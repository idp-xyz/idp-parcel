# 14 TF 其余构造门：判零仍按纳秒、三处自由文本不拒 NUL

Category: bug
Status: resolved——2026-10-19 进 main（代码验证钉 `63106e28`），见「进 main 记录」。此前 in-progress——2026-10-19 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-tfa14`）。此前 needs-triage——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/01 实施中清点出的地盘外同病，未逐口实测）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/01 完成记录「同形门清点」。
地盘：
- internal/transportfulfillment/domain（各口构造门与重建门）

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

照 tf-face-algebra/01 的裁定办：落 timestamptz、重建复用构造门的时刻非零门改按 `AtRegistryPrecision` 判零（只改判定、不改存值，扩展同一条规则不另立定义）；三处自由文本拒 NUL 答未受理；`TransportMovementFact.Correct` 补「更正时刻早于发生时刻」的拒，与重建门口径一致。落 jsonb 快照或不复用构造门的口逐口写明不改的理由。属机制。

## 现象（读码清点，未逐口实测）

- **判零同病**：tf-face-algebra/01 只改了装载分配与移动事实。场外揽收的 `FormOffsitePickup` 与其 `Correct`，以及履约段、承运商判断、揽派任务、履约尝试与交付、外部凭证、外部跟踪、主单、委托与订舱、交接、费用发生、班次运力、替代路程、受监管处置各口的时刻非零门仍按纳秒判。判据依 `AtRegistryPrecision` 头注「本上下文登记册的时刻一律落在 timestamptz 列上」的前提；落 jsonb 快照的口不在此列，要逐口核。
- **NUL 同病**：三处自由文本不经 `newRequiredValue`，只判空不判 NUL：`CarrierEvidenceSpec.Material`、`CarrierFirstEffectivePickupSpec.Material`、`RegulatoryTransportDispositionSpec.DeclineBasis`。
- **顺带**：`TransportMovementFact.Correct` 不挡「更正时刻早于发生时刻」，`RehydrateMovementFact` 却拒；目前没有生产调用方。

## 应当

同 tf-face-algebra/01 的裁定：判零按登记册精度（扩展 `AtRegistryPrecision` 头注那条规则，不另立定义），库收不下的取值在构造门答未受理。

## 完成判据

- [ ] 逐口核：时刻是否落 `timestamptz`、重建是否复用构造门；落的就改判零，不落的写明。
- [ ] 三处自由文本拒 NUL。
- [ ] `TransportMovementFact.Correct` 与重建门对「更正时刻早于发生时刻」口径一致。
- [ ] 每类至少一条真库回归；Go 门禁照常。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa14）

基线 `c4c89a8c`。提交：`23e9f8cf`（判零）、`3d406a16`（NUL）、`687de76d`（移动事实更正门）、`d685e077`（真库回归）、`167e3699`（机制清点）。

### 逐口核：改了的
判据：时刻落 `timestamptz`（按 `migrations/transport_fulfillment` 逐列核过），且重建对读回值判零——复用构造门，或重建自带一道 `IsZero`（同样写得进、读不回，`AtRegistryPrecision` 头注那句据此补成「复用构造门还是自带一道都一样」）。下列门改判 `AtRegistryPrecision(t).IsZero()`，存值不变：
- 场外揽收：`FormOffsitePickup`（发生时刻）、`OffsitePickup.Correct`（更正时刻）↔ `RehydrateOffsitePickup` 自带判零。
- 履约段：`ActualFulfillmentSegment.join` / `rederive`（入场时刻）、`end`（离场时刻）、`CloseSegment` ↔ `rehydrateParticipation`、`RehydrateActualFulfillmentSegment` 按零值认在场与否。
- 承运商判断：`NewCarrierEvidence`（重建复用）、`OpenActualCarrierJudgment`（段成立与形成时间）、`ActualCarrierJudgment.Consider` / `RecogniseCarrierIdentity` / `WithdrawEvidence`（形成时间）↔ `RehydrateActualCarrierJudgment`、`rehydrateJudgmentVersion`。
- 承运商首次有效收寄（票面未列，属「其余各口」）：形状判据 `CarrierFirstEffectivePickup.valid` 的判断时间与已形成那一支的业务时间——它就是三道构造门本身，重建复用。
- 揽派任务：`OpenDispatchTask`（时间窗两端、建立时刻）、`DispatchTask.Reschedule`（时间窗两端）、`DispatchTask.close`。
- 履约尝试与交付：`FormFulfillmentAttempt`（计划窗两端、到场时刻）、`FormAttemptObjectResult`、`FormDeliveryAttemptResult`（重建都复用，揽收与派送两册同走 `rebuildAttempt`）、`EffectiveDelivery.CorrectProof`。
- 外部承运凭证：`NewCredentialApplicability`（起点，重建复用）、`CredentialApplicability.closedAt`。
- 外部跟踪事实：`AdoptExternalCarrierTracking`（接收时间；发生时间见下）、`JudgeEffectiveTimeExplicitly` / `JudgeEffectiveTimeByRule`（重建复用）。发生时间恰为零值仍答 `ErrOccurredAtNotGivenBySource`（走留痕），源给了、登记册上为零点的那一格归形状错误 `ErrInvalidExternalTrackingFact`——它不是「未给」，不该进留痕。
- 主单：`MasterDocument.nextVersion`。
- 委托与订舱：`SubmitTransportCommission`、`TransportCommission.MarkTransportStarted` / `Cancel`、`SubmitBookingRequest`、`BookingRequest.Cancel`（重建复用两者）、`FormCarrierAcceptance`。
- 交接：`FormTransportHandover`、`TransportHandover.Correct`。
- 费用发生：`FormTransportChargeOccurrence`、`TransportChargeOccurrence.ReviseValidity`。
- 班次运力：`FormTransportSchedule`（重建复用）、`CapacityPool.Reserve` 的有效期（`RehydrateCapacityPool` 自带判零）。
- 替代路程、受监管处置：`FormAlternateJourney`、`FormRegulatoryTransportDisposition`（适配器读回都复用构造门，01 记的「重建路径未核」已核）。

### 逐口核：不改的与理由
- `DispatchTask.Reschedule` 的改约时刻、`CapacityPool.Reserve` 与 `CapacityPool.convert` 的调用时刻：不入册（`dispatch_task` 只存时间窗与改约次数，`capacity_reservation` 只存有效期），不是登记册时刻。
- `EffectiveTimeRule.EffectiveTimeFor` 的锚点：现算输入不入册；算出的有效时间经 `JudgeEffectiveTimeByRule`，已按登记册判。
- `CarrierFirstEffectivePickup.valid` 待确认与失效两支的 `occurredAt.IsZero()`：断言缺席，构造门从不设它。
- 取值访问器（`…At()`、`Closure`、`Cancelled`、`Until`、`End`、`Active` 等「零即缺席」）、已成形值上的形状不变量（`CarrierEvidence.valid`、`EffectiveTimeJudgment.valid`、`CredentialApplicability.valid`、`established`）、各 `Rehydrate*` 自身（读回值本就是微秒）：不属非零门。
- 顺带所见（未改，不属非零门）：`NewCredentialApplicability` 的终点是可缺席格（零即开放）；零点之后不足一微秒的终点只在起点早于公元 1 年时立得住，那时写的是有终点、读回是开放。建议需要时另议。

### 其余两格
- NUL：`NewCarrierEvidence` 的名称素材、`HoldCarrierFirstEffectivePickupPending` 的名称素材（`CarrierFirstEffectivePickup.HoldPending` 同走此门）、`FormRegulatoryTransportDisposition` 的拒因，含 NUL 在构造门拒，错误同时包住本口哨兵与 `ErrValueContainsNUL`，编排照旧折成`未受理`。受监管处置的 NUL 门排在三格各守其形之前，含 NUL 的部分承接答`未受理`而不是`待补授权`。
- 更正门：`TransportMovementFact.Correct` 拒更正时刻早于更正后那一版自己的发生时刻，与 `RehydrateMovementFact` 和库面 `corrected_at >= occurred_at` 同一道。按纳秒比，与同类非严格先后门（`LoadAssignment.ReviseMembers`、`TransportHandover.Correct`）一致；非严格门按纳秒只会比库面严，不会写得进、读不回。tf-face-algebra/01 的 `TestAMovementFactCorrectionAtTheZeroInstantOnTheRegistryIsRefused` 原以 2026 年的发生时刻配零点后一微秒的更正时刻作照常格，正是这道门要拒的一对；改取零点前一纳秒作发生时刻，判更正时刻那几格只剩非零门在判。

### 各格对应用例
- 领域判零：`registry_zero_gates_test.go` 的 `TestOffsitePickupGatesJudgeZeroOnTheRegistry`、`TestFulfillmentSegmentGatesJudgeZeroOnTheRegistry`、`TestActualCarrierJudgmentGatesJudgeZeroOnTheRegistry`、`TestCarrierFirstEffectivePickupGatesJudgeZeroOnTheRegistry`、`TestDispatchTaskGatesJudgeZeroOnTheRegistry`、`TestFulfillmentAttemptAndDeliveryGatesJudgeZeroOnTheRegistry`、`TestExternalCarrierCredentialGatesJudgeZeroOnTheRegistry`、`TestMasterDocumentChangeJudgesZeroOnTheRegistry`、`TestCommissionAndBookingGatesJudgeZeroOnTheRegistry`、`TestTransportHandoverGatesJudgeZeroOnTheRegistry`、`TestTransportChargeOccurrenceGatesJudgeZeroOnTheRegistry`、`TestScheduleAndCapacityGatesJudgeZeroOnTheRegistry`、`TestAlternateJourneyAndRegulatoryDispositionGatesJudgeZeroOnTheRegistry`；包内 `external_tracking_registry_zero_test.go` 的 `TestExternalTrackingGatesJudgeZeroOnTheRegistry`。每道门一个子测：恰为零点与零点后 1ns / 999ns / `+14:00` 写法 500ns 拒，零点后 1µs 照常；转换门的前一时刻取零点前一纳秒，让先后门放行。`join` / `rederive` 的入场时刻取自已过构造门的控制事实，领域里没有独立入口，未单钉。
- 领域 NUL：`free_text_nul_test.go` 的 `TestCarrierEvidenceMaterialWithNULIsRefused`、`TestCarrierPickupPendingMaterialWithNULIsRefused`（首登与链尾再长一版）、`TestRegulatoryDispositionDeclineBasisWithNULIsRefused`（拒接与部分承接）；NUL 在首、中、尾三处都拒，不含 NUL 照常。
- 领域更正门：`load_assignment_movement_test.go` 的 `TestAMovementFactCorrectionCannotPrecedeItsOwnOccurrence`（早于更正后发生时刻拒；同刻、早于原发生时刻但不早于更正后的照常，且照常形成的那一版经 `RehydrateMovementFact` 重建得回）。
- 真库：`adapters/postgres/registry_zero_remaining_gates_regression_test.go` 的 `TestADispatchTaskWindowAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry`、`TestADeliveryAttemptPlanAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry`：零点格答 `INPUT_NOT_ACCEPTED`（派送尝试那口线上写作 `SOURCE_NOT_ACCEPTED`），事务提交成功、所列各表零行；零点后 1µs 登记后原样重放答已开立 / 原结果，读回是登记册上的样子。NUL 与更正门按派单只做领域层。

### 先红后绿
- 领域判零：实现前跑新用例，判零子测全红，失败只落在三格不足一微秒的值上（`err = <nil>`），恰为零点与零点后 1µs 两格不红——红落在行为上；每道门各自一个子测，等同逐调用点变异（同一处被两个子测打到的：揽收更正后的发生时刻走 `FormOffsitePickup`，收寄两种判断时间走同一 `valid`）。
- 领域 NUL：实现前五条路径 × 三个位置全红（`err = <nil>`）。
- 更正门：实现前两格拒格红（`err = <nil>`），照常格与重建格不红。
- 真库：把 `dispatch_task.go` 与 `fulfillment_attempt.go` 退回 `c4c89a8c` 跑，揽派任务零点格答 `DISPATCH_TASK_OPENED`、派送尝试零点格答 `ATTEMPT_RECORDED`；恢复后绿。

### 门
- 改动的 `.go` 跑 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok。未跑全仓全量。
- 机制清点重生成：只有测试文件计数变（`167e3699`），生产文件不变。
- 地盘：只动 `internal/transportfulfillment/domain`（含测试；另改了 tf-face-algebra/01 的一条领域用例，理由见上）、`adapters/postgres` 新测试文件、机制清点与本票面；编排、端点、适配器生产代码未改。

证据层级：`S`（隔离合成，`SYN-` 值）。

评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `4c31ebfc`（基线 `c4c89a8c`）· 2026-10-19**：代码出自本会话起的实施子代理，会话读 diff 审，不算非作者评审，如实记下。
  - **Spec**：阻断无。领域里各口落 timestamptz 且重建复用构造门的时刻非零门改按 `AtRegistryPrecision` 判零，存值不变；外部跟踪发生时间恰为零仍答「源未给」、登记册上为零点的归形状错；三处自由文本拒 NUL；`TransportMovementFact.Correct` 补拒「更正时刻早于发生时刻」，与重建门一致。不改的口（改约与预占调用时刻不入册、规则锚点、访问器、形状不变量）逐处写了理由。
  - **Standards**：阻断无。越出地盘的一处：tf-face-algebra/01 一条领域用例原照常格那一对正是新更正门要拒的，改了用例，认可。
  - **留作后续**：凭证终点落在零点后不足一微秒时写进去有终点、读回成开放区间（不属非零门），只记录未改；`join`、`rederive` 领域层无独立入口未单测。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-tfa14` rebase 到远端 main 之上快进推送；评审钉的是 rebase 前的提交，代码内容未变（rebase 只撞机制清点，在新树上重生成解决）。验证钉 `63106e28` 之后远端只前进了一笔纯票面（票 50、39 认领），再 rebase 一次，`git diff 63106e28 HEAD -- . ':!.scratch'` 为空，未重跑。
- 推送方全量一次：钉 `63106e28` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
