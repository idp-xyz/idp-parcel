# UC-CC-009 的税费付款核对形成后不向 `settlement-accounting` 交接：`VerifyPayment` 落库即止，SA 的实际代垫成立判断拿不到「关务税费及付款核对」这一项输入

Category: enhancement
Status: resolved——2026-09-11 15:0x 通道 5 完工，分支 `mcp5-sacc05` tip `27b30cd8`、代码 tip `353597a2`，等非作者评审与推送方重放进 main（完成记录见 Comments）。此前 in-progress——2026-09-11 14:1x 通道 5 按通道 1 派单 task-3de4ab39 开工，分支 `mcp5-sacc05`（派单写基 `51ca1270`，开工时 `ls-remote` 远端 main 已到 `5d6fda3f`，其间两笔只动 `.scratch/`，故基 `5d6fda3f`）。此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：分区主体取租户 / 申报范围、SA 侧消费者另立 [09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md)（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `VerifyPayment`（步 7）写 `ports.DutyVerificationStore`，**不调任何 `HandOff*`**（`git grep -n -i handoff -- internal/customscompliance/application/reconcile_duty_payment.go` 零）。
- CC 今天的 handoff 只有三种：`CustomsCaseHandoff` / `DeclarationSubmissionHandoff` / `CaseClosureHandoff`（`git grep -n -i 'HandOff' -- internal/customscompliance/application`）。
- SA 侧 `UC-SA-001` 的实际代垫判断按 CONTEXT 要「关务税费及付款核对」作输入之一；今天没有信封把它送过去。
- mech/07「没做」第 3 条：「UC-CC-009 步 8 核对向 `settlement-accounting` 的交接（outbox 意图）……一张」。

## 语言从哪里来

- SA `CONTEXT.md`：「实际代垫成立判断使用付款方身份、外部资金事实、关务税费及付款核对和合同责任，不由任一单项输入直接推导」；「结算输入已接收：固定关务交接、税费、付款核对……的采用版本；接收不表示代垫或回收已经成立。」
- CC `CONTEXT.md`：「税费付款核对……分别判断覆盖状态、差额和事实有效性，不形成实际付款、客户回收或监管放行」；「本上下文只拥有监管核定、法定义务、税费付款核对和放行门禁判断及其与外部资金事实的可追溯关系」。
- UC-CC-009 标题所指的「结算交接」（范围节第 9 条：「……财务接入、结算交接和恢复测试骨架」）。

## 做法

1. `ports.DutyPaymentVerificationHandoff` + `adapters/postgres/duty_payment_verification_handoff.go`，事件类型 `customs-compliance.duty-payment-verification.formed`，载荷只带引用（租户、申报范围、核对版本、监管核定税费版本、资金事实引用）；形照 CC 既有 `case_closure_handoff` 那一族。
2. `VerifyPayment` 在核对**形成**（新版本）那一格同事务交意图；`已存在` 不重发；未决两格（资金事实未接收 / 协作事项未形成）不发。
3. SA 侧消费者**不在本票**（见「要裁的」第 2 条）；本票只让信封出得去。

## 红线

- 信封不带金额结论（覆盖 / 差额 / 有效性三态由 SA 按引用读 CC 读口——CC 是权威）。
- 不把「核对形成」解释成「代垫成立」：CONTEXT 明写任何单项不能推导。
- 不动三表分层。

## 完成判据

1. 应用层：形成 → 一封；`已存在` → 不发；未决 → 不发；交接失败 → 技术未形成 + 续办引用（仓内既有形）。
2. 真库：入队一封、分区键如裁、`EnqueueOnce` 不翻倍。
3. 清点 tip 重生成（outbox handoff +1）。

## 地盘

`internal/customscompliance/ports/`、`internal/customscompliance/application/reconcile_duty_payment.go`、`internal/customscompliance/adapters/postgres/`（新文件）、`reconcile_duty_payment` 的装配点（先核在哪，`git grep -n DutyPaymentReconciliationHandler -- cmd/` 今天零——装配随 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md) 的登记面一起落，本票的信封在那之前只有测试路径能触发，票面如实记）。

## 要裁的

1. **分区主体**：租户 / 申报范围（同范围的核对版本有序）还是租户 / 监管核定税费版本。归 CC owner，一句。
2. **SA 侧消费者归谁立**：本目录再加一张，还是并进 SA 的 `UC-SA-001` 实际代垫编排票（今天那条编排是否存在要先核 `assess_advance_recovery.go`）。归 SA owner。

### 裁决

（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类两条均 A、task-9a2ff746 落笔。A 类不落 ADR。）

- **1 → 租户 / 申报范围。** 同票 02 的口径：「ID 管幂等、分区键管顺序：同一主体的版本链排一条队」（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二）；核对版本按申报范围逐笔形成（CC CONTEXT「按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态」），有版本链的主体是「这一申报范围的付款核对」，不是税费版本（税费更正只是新核对版本的来源之一）。主体名「租户 / 申报范围」按 [ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五进中心登记表，随本票落地同笔。
- **2 → 本目录加一张 [09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md)「SA 消费付款核对信封 → 接 `assess_advance_recovery.go`」。** `internal/settlementaccounting/application/assess_advance_recovery.go` 已存在（通道 5 于 66cad4c4 核），消费者接它即可，不重开已 resolved 的 UC-SA-001 票；SA CONTEXT「不由任一单项输入直接推导」仍由那条编排守，消费者只译不判。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c 与「没做」第 3 条；UC-CC-009；UC-SA-001；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ③；`internal/customscompliance/application/close_customs_case.go` 的 `handOffClosure`（本上下文 handoff 先例）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 14:1x–15:0x · 通道 5（task-3de4ab39）· **完成记录**。分支 `mcp5-sacc05` 基 `5d6fda3f`（派单写 `51ca1270`，开工 `ls-remote` 已到 `5d6fda3f`，其间 `0b027ab8` / `5d6fda3f` 两笔只动 `.scratch/`，含本 spec 的 15 行，故基新 tip 免那一行冲突）。三笔：`f4713583` 票面 in-progress；**`353597a2` 代码**；`27b30cd8` 清点在 `353597a2` 干净检出重生成（CC 生产文件 83→84、适配器 37→38、**outbox handoff 9→10**、端口 390→391）。
  - **做法 1**：`ports.DutyPaymentVerificationHandoffIntent{Key DutyVerificationKey; Verification domain.DutyPaymentVerification}` + `ports.DutyPaymentVerificationHandoff.HandOffDutyPaymentVerification`；`adapters/postgres/duty_payment_verification_handoff.go` 的 `OutboxDutyPaymentVerificationHandoff`（构造 `(db, store, clock)` 逐口拒 nil、`var _` 断言、`outboxintent.EnqueueOnce`，形照 `case_closure_handoff.go` / `gate_verification_handoff.go`）。事件类型 `customs-compliance.duty-payment-verification.formed`；载荷恰是 `tenantId / scope / duty / funds / digest`（三维键 + 内容指纹即核对版本），三轴与依据不进信封（红线 ①，真库用例 `TestDutyVerificationPayloadCarriesOnlyReferences` 钉键集）。信封 ID = 分区键 + `/税费引用/资金事实引用/指纹`（每版一封，改判不被 EnqueueOnce 吞）；**分区键 `租户/duty-payment-verification/申报范围`**——主体按裁决 1「租户 / 申报范围」，带口名段的理由写在 `dutyPaymentVerificationPartitionKey` 头注（`restriction_handoff` 按同一个 `DecisionScopeReference` 串分区、不带段即共队，两口无消费方依赖跨口序，ADR-0074 决定二同一条）。主体名进 `internal/architecture/partition_subject_registry_test.go` 一行（ADR-0074 决定五）；仓内无第二上下文同名，不需裁段。不加迁移：outbox 表通用。
  - **做法 2**：`DutyPaymentReconciliationDeps` 加 `Handoff` 口，`NewDutyPaymentReconciliationHandler` 表驱动逐口点名加 `duty payment verification handoff`；`VerifyPayment` 只在 `SaveVerification` 答 `Registered`（`DutyVerificationFormed`）时调 `handOffVerification`，`已存在`不调，待关联 / 资金事实未接收 / 协作事项未形成 / 三轴集外 / 核对库故障皆不调；交接失败结果不翻、`DutyReconciliationResult.HandoffReference()` 留 `CONT-DUTY-VERIFICATION/<范围>/<指纹前八>`（形照 `close_customs_case.go` `handOffClosure`）。
  - **做法 3**：SA 一个文件未动；消费者归 09。
  - **判据逐项**：**1** 应用层（`reconcile_duty_payment_test.go`）——`TestAFormedVerificationHandsOffOneEnvelopeAndReplayDoesNotResend`：形成 → 一封且键 = 刚落册那一版、重放 `已存在` → 替身调用次数仍 1、改判 → 第二封指纹不同；`TestNoEnvelopeLeavesWhenNoVerificationIsFormed`：五个「没形成」格零封；`TestAFailedHandoffLeavesTheVerificationFormedWithAContinuationReference`：结果仍 `DUTY_VERIFICATION_FORMED`、续办引用非空、核对已落。构造门用例加 `Handoff` 一行。**2** 真库（`duty_payment_verification_handoff_test.go`，带 DSN -v 全 PASS）：与核对行同一提交（先铺外键前置资金事实）、回滚双消、同一份重发不翻倍、**两版同区不同 ID**（分区键实测 `tenant-a/duty-payment-verification/SYN-UNIT-01`）、载荷键集恰五引用、无事务拒、缺键 / 缺核对时刻响亮。**3** 清点 `27b30cd8`（见上）。
  - **两处装配点**：`cmd/parcel-dispatch/assemble.go` `receiveExternalFundsFactConsumer` 多收 `outboxStore *outbox.Store`、构造 `NewOutboxDutyPaymentVerificationHandoff(db, outboxStore, clock)` 传 `Handoff`（该函数一块 + 调用行一处，共两 hunk、均本票）；`cmd/parcel-customs-register/main.go` `buildRegistrar` 新建 `outbox.NewStore(db)` + 交接口、传 `Handoff`（只加那一段，其余未动）。**三处夹具**：`fullDutyDeps` 的 `dutyStoreDouble` 加交接半边（记意图与次数）；`receive_on_adopted_funds_fact_test.go` `unreachedDutyStores` 加 `HandOffDutyPaymentVerification` 碰到即 Fatal（消费者不核对也就无物可交）；`duty_registers_test.go` `fakeDutyBook` 加交接半边，并在 `TestExecuteDutyPaymentVerificationLandsReplaysAndAppendsVersions` 里加三条断言（形成一封 / 重放仍一 / 新版本两封）证 CLI 接通了。
  - **验**（作者侧，不跑全量）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0。带 DSN（占 / 释 55432 两轮已广播）在 `353597a2` 隔离 detached 树 `%TEMP%\idp-replay…` 同法的 `idp-verify-sacc05`：CC `adapters/postgres` + `application` + `adapters/settlementaccounting` + `cmd/parcel-dispatch` + `cmd/parcel-customs-register` + `./internal/architecture/...` 六包 ok，**-v PASS 659 / FAIL 0 / SKIP 0**（14:53:31→14:53:53）；其中 `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable`、`TestCustomsRegisterVerticalOnRealPostgres`、`TestTheComposedDispatcherRunsABeatAgainstARealDatabase` PASS（两处生产装配点带新口真库跑通）。两道门禁核过：`envelope_partition_gate`（ID 与分区键各绑局部变量——首版把两个函数调用直接写进字段，门禁把 `eventID(key)` / `partitionKey(key)` 剥成同一个 `key` 报红，按 `gate_verification_handoff.go` 的形改绑局部）与 `partition_subject_registry`。
  - **判断项（报评审 / 推送方，不是待裁）**：① **`已存在`不重发**按票面字面实现（替身调用次数为证），与本上下文其余编排「重放重发同一份」不同形；成立的前提写在 `VerifyPayment` 头注——信封与核对版本同一笔事务，库侧入队失败使事务中止（bento `WithinTransaction` 的 `Commit` 错误原样返回，已核 `transaction.go`），重跑仍走`形成`格再铸同一封；续办引用因此只覆盖非库侧失败，是运维坐标不是重投指令。若评审认为应与既有形一致（`已存在`也交、靠 EnqueueOnce 吞），改动是 `VerifyPayment` 两行 + 两条断言。② 分区键带 `/duty-payment-verification/` 口名段是作者在裁决主体之内做的选择（理由同 ADR-0074 决定二），登记表行的括号已写明；若推送方要光的 `租户/申报范围`，改一处函数与一条断言。③ `parcel-customs-register` 的答复面未透出 `HandoffReference()`——派单写「`buildRegistrar` 只加那一口」，`translate.go` 未动；真库上该引用几乎不可达（见 ①），建议随 07 步二或 15 顺手加，不单立票。④ `duty_registers_test.go` 在既有用例里多了三条交接断言，超出「加夹具」一格，属证接通、无行为改动。
  - **能力边界**：读过 CC 既有 handoff 一族全部构造与 `case_closure` / `gate_verification` 两份测试、`close_customs_case.go` / `verify_release_gate.go` 的续办形、SA `map_external_funds_handoff_test.go`（02 的「重放不交」实现读法与评审结论）、ADR-0069 / 0074 全文、两道架构门禁全文、bento `WithinTransaction`；**没读** SA `assess_advance_recovery.go`（消费侧归 09）、CC CONTEXT 全文（只查「税费付款核对」「技术未形成」两处）。子代理评审通道本轮鉴权失败，两轴自查串行完成，**不代替非作者评审**。
