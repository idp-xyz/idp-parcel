# 01 TF 构造门按登记册存得下的值收：零时刻之后不足一微秒的业务时间写得进、读不回，引用里的 NUL 答 500

Category: bug
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 代决并认领（分支 `claude-tfa01`）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-load-assignment-registrations, /transport-fulfillment/movement-facts, /transport-fulfillment/delivery-attempts
地盘：
- internal/transportfulfillment/domain/load_assignment_movement.go
- internal/transportfulfillment/domain/offsite_pickup.go（`newRequiredValue`）
- internal/transportfulfillment/domain/（上两文件的测试，宜新增测试文件）
- internal/transportfulfillment/adapters/postgres/（新增真库回归测试文件）
- cmd/parcel-api/（新增真渠道回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

照「修复方向（机制）」四条做，没有取舍：判零按登记册精度（扩展 `AtRegistryPrecision` 头注那条规则，不另立第二处定义）；`newRequiredValue` 拒含 NUL，各构造门折成`未受理`；不放 HTTP 入口、不让 PG 适配器拒写；同形门逐处清点，地盘外的写进完成记录另立。与 customs-compliance 的 tf-face-algebra/11 同形。属机制，不涉租户取值。

## 现象

### 一、零时刻之后不足一微秒：写得进、读不回
- 装载分配：`assignedAt` 取 `0001-01-01T00:00:00.000000001Z`、`…00.000000999Z` 或 `0001-01-01T14:00:00.0000005+14:00` 时：
  - 首投答 201 `LOAD_ASSIGNMENT_FORMED`，版本行与成员行照落，库面 `assigned_at` 存成 `0001-01-01T00:00:00Z`。
  - 此后原样重放与同身份异内容都答 200 `LOAD_ASSIGNMENT_UNDECIDED`，续办引用每次相同。
  - 生产 `LoadAssignments.FindByKey` 报「find load assignment: transport fulfillment: invalid load assignment」。
  - 恰为零点答 `INPUT_NOT_ACCEPTED`；零点后恰一微秒、零点前一纳秒都照常。
- 移动事实：`occurredAt` 取 `0001-01-01T00:00:00.0000001Z` 时，首投答 201 `MOVEMENT_FACT_RECORDED`，库面 `occurred_at=0001-01-01T00:00:00Z`；原样重放答 200 `MOVEMENT_FACT_UNDECIDED`；`MovementFacts.FindByKey` 报「invalid movement fact」。
- 两册都只插不改，也没有撤回入口，所以这个身份此后永远答`未决`，产品内修不回来。

### 二、引用里带 NUL：答 500
- 装载分配：版本带 NUL、新身份的成员带 NUL，都答 500 `NO_ANSWER_FORMED`。
- 移动事实：地点带 NUL 答 500。直调生产编排得到的是 `MOVEMENT_FACT_UNDECIDED` 带续办，但事务已进中止态，提交时得「commit unexpectedly resulted in rollback」。
- 派送尝试：证据引用带 NUL，两次都答 500，库零行。
- 每次重试都是同一个答复。它属于「改输入」那一格，不是「等恢复」那一格。

## 复现（S）
命令：在只读检出 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 里执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。pgtest 每例自建物理库；探针不提交。

- 装载分配，零点与 NUL：
  - /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/probe_load_assignment_test.go 的 `TestProbeLoadAssignmentWrittenButUnreadable`、`TestProbeLoadAssignmentUnreadableCauseIsTheZeroInstant`、`TestProbeLoadAssignmentDatabaseErrorInsideTheTransaction`（同目录 overlay.json）。
  - /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-impl/adv_impl_probe_test.go 的 `TestAdvImplZeroInstantWindowThroughTheEndpoint`、`TestAdvImplDatabaseErrorBranches`、`TestAdvImplTruncationAgreesWithThePersistedPrecision`（同目录 overlay.json；日志 run-adv-impl.log）。
- 移动事实零点：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/zz_probe_zero_instant_test.go 的 `TestProbeDecide2MovementFactZeroInstant`（overlay 为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/overlay-zero.json；日志 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/synth/run-zero.log）。
- 移动事实 NUL：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/probe_movement_fact_test.go 的 `TestProbeMovementFactNulMechanism`（同目录 overlay.json）。
- 派送尝试 NUL：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-attempts/adv/probe_da_adv_test.go 的 `TestProbeDAAdvEdges`（同目录 overlay-da-adv.json；日志 run-adv3.log）。

## 应当
- 写得进就读得回。构造门按登记册存得下的值来判：零时刻之后不足一微秒的时刻，在登记册上就是零点，应答`未受理`（`INPUT_NOT_ACCEPTED`），不落库。
- 库收不下的取值，在构造门就拒：引用含 NUL 时答`未受理`（200），不落成 500。
- 依据与票 30 裁定对严格先后门的定性相同：ADR-0031「`error` 留给『这次写入没能完成』」；永远存不进去的输入，应答「改输入」。

## 根因
- 判零按纳秒，存值按微秒：
  - `FormLoadAssignment` 以「`spec.AssignedAt.IsZero()`」守门，`RecordMovementFact` 以「`spec.OccurredAt.IsZero()`」守门，都按纳秒判。
  - 库面 `timestamptz` 写入时截到微秒，存成了零点。
  - `RehydrateLoadAssignment`、`RehydrateMovementFact` 复用同一道构造门（头注「复用它而不是抄一遍」），所以读回即被拒。
  - 对抗核验实测：Go 的 `Truncate(time.Microsecond)` 与 pgx 落库后读回的值逐例相同。
- NUL 不拒：`newRequiredValue` 只拒空白（「`strings.TrimSpace(value) == ""`」），不拒 NUL。PG `text` 收不下 NUL，写入报 SQL 错，事务进中止态；编排折出的`未决`在提交时被回滚吞掉，端点按「编排交回 error」写 500。
- 票 30 的裁定收了「相等类」与「严格先后类」，没有收「判零」这一类。

## 修复方向（机制）
1. 判零按登记册精度：改成 `AtRegistryPrecision(t).IsZero()`，只改判定，不改存值。
   - 装载分配的 `ReviseMembers` / `Withdraw`、移动事实的 `TransportMovementFact.Correct`，时刻门都照同一判据改。
   - 在 `AtRegistryPrecision` 头注那条规则里补一句「时刻非零门同属此类」。这是对那条规则的扩展，不另立第二处定义。
2. `newRequiredValue` 拒含 NUL 的值，各构造门照旧折成`未受理`；错误归哪一类由实施方定。不经 `newRequiredValue` 的自由文本格要逐处核，完成记录按符号名列出。
3. 不放的位置：
   - 不放 HTTP 入口 `parseOptionalInstant`：它多口共用，精度也不属传输层。
   - 不让 PG 适配器拒写：编排会把 error 答成`未决`，落错格。
4. 同形门清点：在 TF `domain` 非测试文件里 `grep -n 'IsZero()'`，逐处看该时刻是否落 `timestamptz`、重建是否复用构造门。本票只改地盘内的两份文件；别处同病的写进完成记录，另立票守地盘。

## 是否潜伏
已可触发。输入虽然离谱，后果却是永久的。
- 装载分配挂运营决定渠道：生产装配上不问授权映射；演示部署三道都开着，即权威串、授予 `SYN-GRANT-DEX-CONFIG-LOAD-ASSIGNMENT-01`、准入对照 `SYN-MAP-OD-LOAD-ASSIGNMENT-01` 与对应治理区间。
- 移动事实、派送尝试挂作业事实登记渠道：编排没有未配置缝，铸造也不判准入；只差租户登记作业事实授予与在用设备。这两样可用随产品发布的 `parcel-access-register` 登，属租户取值。

## 完成判据
- [ ] 领域单测，零点格：装载分配、移动事实各取「零点后 1ns」「零点后 999ns」「`+14:00` 写法零点后 500ns」，答 `ErrInvalidLoadAssignment` / `ErrInvalidMovementFact`。
  - 「零点后 1µs」「零点前 1ns」照常形成；恰为零点照旧拒。
  - 替身需要模拟读回时，用测试里独立写的 `Truncate(time.Microsecond)`。
- [ ] 领域单测，NUL：`newRequiredValue` 对含 NUL 的值报错；前后带空白的非空值，行为不变。
- [ ] 真库回归（`adapters/postgres` 新测试文件，编排加真登记册，同一事务）：
  - 上列零点格答 `INPUT_NOT_ACCEPTED`，版本行、成员行、移动事实行都是零行；
  - 「零点后 1µs」首登后 `FindByKey` 读得回，原样重放答 `…_VERSION_EXISTS`。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件）：
  - 装载分配走生产端点构造、生产准入桥与 `buildSegmentOperations`；移动事实、派送尝试走 `WorkFactCommandIntake` 与生产编排。
  - 零点格答 200 `INPUT_NOT_ACCEPTED`。
  - 装载分配版本带 NUL、移动事实地点带 NUL、派送尝试证据带 NUL，各答 200 `未受理`而不是 500，库零行。
- [ ] 变异自检：把判零改回按纳秒、把 NUL 检查撤掉，对应用例都转红；完成记录写明对哪些调用点做过。
- [ ] 同形门清点结果写进完成记录，用符号名，不计数。
- [ ] Go 门禁：
  - `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；
  - 带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok，`-v` 下 TF `adapters/postgres` 无 `--- SKIP`。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`，随笔提交。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：
  - 从当时的 `origin/main` 起分支；
  - 作者自验；
  - 非作者评审（空闲通道，或 `/code-review` 的隔离子代理，两轴分开）；
  - 推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa01）

基线 `62683e80`。提交：`95433dc3`（判零）、`04c592a0`（NUL）、`aaf07c84`（真库回归）、`35b31431`（真渠道回归）、`cf36018a`（机制清点）。

### 落点
- 判零：`FormLoadAssignment`、`LoadAssignment.ReviseMembers`、`LoadAssignment.Withdraw`、`RecordMovementFact`、`TransportMovementFact.Correct`（发生时刻与更正时刻两格）的时刻非零门改判 `AtRegistryPrecision(t).IsZero()`；存值不变。`AtRegistryPrecision` 头注那条规则补「时刻非零门同属此类」，未另立定义。
- NUL：`newRequiredValue` 含 NUL 答新哨兵 `ErrValueContainsNUL`（与 `ErrBlankValue` 并列）；各构造门与编排照旧折成`未受理`，编排与端点未改。
- 未放 `parseOptionalInstant`，未动 PG 适配器。

### 各格对应用例
- 领域判零：`registry_storable_values_test.go` 的 `TestALoadAssignmentAtTheZeroInstantOnTheRegistryIsNotFormed`、`TestLoadAssignmentRevisionAndWithdrawalAtTheZeroInstantOnTheRegistryAreRefused`、`TestAMovementFactAtTheZeroInstantOnTheRegistryIsNotRecorded`、`TestAMovementFactCorrectionAtTheZeroInstantOnTheRegistryIsRefused`（零点后 1ns / 999ns / `+14:00` 写法 500ns 拒；恰为零点拒；零点后 1µs、零点前 1ns 照常）。夹具自检 `TestSubMicrosecondFixturesAreZeroOnTheRegistry` 用测试里独立写的 `Truncate(time.Microsecond)`。
- 领域 NUL：`required_value_test.go` 的 `TestRequiredValuesRefuseNUL`（首尾中间含 NUL 拒；前后带空白的非空值原样收）。
- 真库：`adapters/postgres/storable_values_regression_test.go` 的 `TestALoadAssignmentAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry`、`TestAMovementFactAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry`（零点格 `INPUT_NOT_ACCEPTED`、版本行成员行事实行零行；零点后 1µs 首登后 `FindByKey` 读得回、原样重放答 `…_VERSION_EXISTS`）、`TestValuesWithNULAreNotAcceptedAgainstARealRegistry`（装载分配版本、新身份成员、移动事实地点、派送尝试证据含 NUL 答`未受理`，事务提交成功，所列各表零行）。
- 真渠道：`cmd/parcel-api/storable_values_channel_test.go` 的 `TestLoadAssignmentUnstorableValuesAreNotAcceptedOnTheProductionChannel`（生产端点构造 + 生产准入桥 + `buildSegmentOperations`）、`TestMovementFactUnstorableValuesAreNotAcceptedOnTheProductionChannel`（生产 `buildWorkFactIntakes` + `buildMovementFactOrchestration`）、`TestDeliveryAttemptEvidenceWithNULIsNotAcceptedOnTheProductionChannel`（`WorkFactCommandIntake` + `buildDeliveryAttemptOrchestration`，任务先经操作者渠道开立）。零点格与 NUL 格答 200；派送尝试的`未受理`在线上写作 `SOURCE_NOT_ACCEPTED`（该口既有代数），另两口写作 `INPUT_NOT_ACCEPTED`。

### 先红后绿
- 领域：修复前判零各例与 NUL 那例都红（`err = <nil>`）。
- 真库（把两份领域文件退回 `62683e80` 跑）：装载分配零点后 1ns 答 `LOAD_ASSIGNMENT_FORMED`、移动事实答 `MOVEMENT_FACT_RECORDED`、NUL 那例「事务没有提交：commit unexpectedly resulted in rollback」。
- 真渠道（同法）：装载分配零点格 201 且回显 `assignedAt":"0001-01-01T00:00:00Z"`、移动事实 201 回显零点、派送尝试证据 NUL 答 500 `NO_ANSWER_FORMED`。
- 变异自检：判零逐个调用点（`FormLoadAssignment`、`ReviseMembers`、`Withdraw`、`RecordMovementFact`、`Correct` 的发生时刻与更正时刻）单独改回按纳秒，领域判零用例各自转红；撤掉 NUL 检查，领域、真库与三条真渠道 NUL 用例全红（真渠道装载分配版本、移动事实地点、派送尝试证据各答 500）。真库与真渠道里成员含 NUL 那格排在版本格之后或与之同表遍历，变异下先红的是版本格，成员格未单独取到红。

### 复现不了的格
无。票面现象各格均复现；「零点前 1ns 照常」与「恰为零点照旧拒」一致。

### 同形门清点（TF `domain` 非测试文件 `IsZero()`，本票地盘外未改）
前提按 `AtRegistryPrecision` 头注：本上下文登记册时刻一律落 `timestamptz`（未逐口真库实测）。以下构造门或转移门对入参时刻按纳秒判零，而对应重建对读回值判零（标「复用」者为重建复用构造门，其余为重建自带 `IsZero` 检查），同属写得进、读不回：
- 场外揽收（地盘文件 `offsite_pickup.go`，本票只动了 `newRequiredValue`）：`FormOffsitePickup`、`OffsitePickup.Correct` ↔ `RehydrateOffsitePickup`。
- 履约段：`ActualFulfillmentSegment.join` / `end` / `CloseSegment` / `rederive`、`FulfillmentParticipation.End` ↔ `RehydrateActualFulfillmentSegment`、`rehydrateParticipation`。
- 承运商判断：`NewCarrierEvidence`、`OpenActualCarrierJudgment`、`ActualCarrierJudgment.Consider` / `RecogniseCarrierIdentity` / `WithdrawEvidence` ↔ `RehydrateActualCarrierJudgment`、`rehydrateJudgmentVersion`。
- 揽派任务：`OpenDispatchTask`、`DispatchTask.Reschedule` / `close` ↔ `RehydrateDispatchTask`。
- 履约尝试与交付：`FormFulfillmentAttempt`、`FormAttemptObjectResult`、`FormDeliveryAttemptResult`、`EffectiveDelivery.CorrectProof` ↔ `RehydrateEffectiveDelivery` 等（派送尝试册的重建路径未逐一核）。
- 外部承运凭证：`NewCredentialApplicability`（复用）、`CredentialApplicability.closedAt` ↔ `RehydrateExternalCarrierCredential`。
- 外部跟踪事实：`AdoptExternalCarrierTracking`、`JudgeEffectiveTimeByRule` / `JudgeEffectiveTimeExplicitly`（复用）↔ `RehydrateExternalTrackingFact`、`rehydrateEffectiveTime`。
- 主单：`MasterDocument.nextVersion` ↔ `RehydrateMasterDocument`。
- 运输委托与订舱：`SubmitTransportCommission`、`TransportCommission.Cancel` / `MarkTransportStarted`、`SubmitBookingRequest`、`BookingRequest.Cancel`、`FormCarrierAcceptance` ↔ `RehydrateTransportCommission`、`RehydrateCarrierAcceptance`。
- 交接：`FormTransportHandover`、`TransportHandover.Correct` ↔ `RehydrateTransportHandover`。
- 费用发生：`FormTransportChargeOccurrence`（复用）、`TransportChargeOccurrence.ReviseValidity` ↔ `RehydrateChargeOccurrence`。
- 班次与运力：`FormTransportSchedule`、`CapacityPool.Reserve` / `convert` ↔ `RehydrateCapacityPool`。
- 替代路程、受监管处置：`FormAlternateJourney`、`FormRegulatoryTransportDisposition`（重建路径未核）。
- 不属此类：取值访问器（`…At()` / `Cancelled` / `Closure` 等「零即缺席」）、`valid` / `established` 形状不变量、`EffectiveTimeRule.EffectiveTimeFor` 的现算锚点。

不经 `newRequiredValue` 的自由文本格：`CarrierEvidenceSpec.Material`（`NewCarrierEvidence`，落 `name_material text`）、`CarrierFirstEffectivePickupSpec.Material` 与更替那一格的 `Material`（落 `claimed_material text`）、`RegulatoryTransportDispositionSpec.DeclineBasis`（落 `decline_basis text`）——都只判空不判 NUL，同病。重建侧 `EffectiveRule` / `EffectiveRuleVersion` 来自库面，不是入参。`Direction` 已经 `newRequiredValue`。

顺带所见（地盘内，未改，不属判零类）：`TransportMovementFact.Correct` 不挡「更正时刻早于发生时刻」，`RehydrateMovementFact` 却拒；目前无生产调用方（`ReviseMembers` / `Withdraw` 同样只在领域里可达）。

以上地盘外各项建议另立票按本票同法逐口收。

### 门
- `gofmt -l internal/transportfulfillment cmd/parcel-api` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok；`-v` 下 TF `adapters/postgres` 无 `--- SKIP`。未跑全仓全量。
- 机制清点重生成：仅测试文件计数变（见 `cf36018a`），生产文件不变，单独提交。
- 未做：票 15 补记下的回写行、非作者评审与推送方全量（属会话与推送方）。

证据层级：`S`（隔离合成，`SYN-` 值）。

评审：待会话读 diff 审。
