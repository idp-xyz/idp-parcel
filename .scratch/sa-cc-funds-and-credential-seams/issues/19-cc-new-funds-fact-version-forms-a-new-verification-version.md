# 资金事实新版本到达后没有编排接着做：登记册看得见 v2 回指 v1，UC-CC-009「形成新核对版本并保留原覆盖判断」在 CC 侧仍无入口

Category: enhancement
Status: draft——2026-09-14 14:0x 通道 1 立票（按 sa-cc/13 裁决 2「触发重核对不在本票……推送方据此立票」，形取 13 完成记录「后继票的形」）。只写票面未动代码；取证锚 main `0bd86d42`（sa-cc/13 重放 tip）
Blocked by: 无（[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 已进 main：`ExternalFundsFactRegister.ListFundsFactVersions` 列得出全部版本与回指）；**要裁的 1 归 CC owner，裁前不动代码**

## 缺口（取证于 `0bd86d42`，逐符号名）

- 13 落地后：更正版本 v2 经 `settlement-accounting.external-funds-fact.adopted` 信封到 CC，`ReceiveFundsFact` 答`已接收`、`external_funds_fact_version` 落第二行回指 v1。到此为止——`external_funds_fact_consumer.go` 头注写的是「触发重核对归后继票，今天登记册上看得见、还没有编排接着做」。
- `VerifyPayment` 的三轴（覆盖 / 差额 / 有效性）与关联依据由调用方交（`VerifyDutyPaymentCommand`），调用方今天不在 `parcel-dispatch` 进程里（sa-cc/03 票面红线原句）；新版本到达时没有调用方在场，编排不算三轴。
- `DutyVerificationStore` 今天只有按幂等键的 `FindVerification`，没有「按（租户、资金事实）列全部核对版本」的读口——新版本到达时连「这条事实有没有过核对」都问不出来。
- `LoadFundsFact` 交回「最近接收的那一版」是 13 端口头注写明的临时口径：核对命令上没有版本，`duty_payment_verification` 也没有 `funds_version` 列——核对引用的是事实身份，不是事实的哪一版。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断」；集成规则「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态」。
- UC-CC-009 一致性节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖」。

## 做法（待裁后写实）

1. **触发落点**：CC 内部，`ReceiveFundsFact` 答`已接收`且该事实已有既往核对版本时——触发在消费侧适配器 `ReceiveOnAdoptedFundsFactAdapter` 之后一格另起一只编排（消费侧适配器仍只译不判，sa-cc/03 做法 3），不塞进 `ReceiveFundsFact`。
2. **要读哪几口**：`ExternalFundsFactRegister.ListFundsFactVersions`（新版内容与回指）、`DutyVerificationStore` 新增「按（租户、资金事实）列全部核对版本」读口、`DutyCollaborationStore`（协作事项仍在）、`PayerRequirementRuleView`（付款人维照 sa-cc/12 三停格）。
3. **核对按版本读**：`VerifyDutyPaymentCommand` 加 `FundsVersion`，`duty_payment_verification` 加 `funds_version` 列（新迁移，`0016` 不改），`LoadFundsFact`「最近接收」口径退役。
4. **交接不变**：新核对版本形成后走 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff`（每版一封）交 SA；SA 侧采用（sa-cc/09 那族）按版本收。

## 红线

- 不按到达顺序覆盖：原核对版本一行不动（UC-CC-009 原句）。
- 三轴与关联依据不由编排猜——真实关联规则属实例半边，一行不预填。
- 消费者与消费侧适配器只译不判。

## 完成判据（待裁后写实）

1. 应用层：v1 已核对，v2 到达 → 形成新核对版本（形随裁决 1），原核对版本仍在、内容不变；无既往核对的事实新版本到达 → 不形成、不报错。
2. 真库：新迁移往返；`0016` / `0021` 一字未动；`cmd/parcel-dispatch` 真库装配用例在 13 那一格之后再扩一格（v2 到达 → 新核对版本 / 待重核对事项落行）。
3. `LoadFundsFact`「最近接收」的临时口径退役，13 端口头注那句随之改口。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres,adapters/settlementaccounting}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-dispatch/assemble_test.go`（共享文件，动前占号）。SA 侧不动。

## 要裁的

1. **新核对版本的三轴与关联依据从哪来**——归 CC owner。(a) 复用前版三轴与依据、有效性轴标 `PENDING`，形成一版「待人判」的核对；(b) 不形成核对，只登一条「待重核对」事项（新表或核对表一格）交人 / 交规则。两条路都不让编排猜三轴；差别在「形成了一版核对」还是「登了一条待办」。
2. 触发要不要区分 `corrects` 在不在（首版到达从不触发；有回指才可能有既往核对）——可由做法 1「已有既往核对版本」一条覆盖，裁时确认。

## 参照

[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 完成记录「后继票的形」；[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 三停格；`internal/customscompliance/application/reconcile_duty_payment.go`（`VerifyPayment` / `ReceiveFundsFact`）；`internal/customscompliance/ports/ports.go`（`ExternalFundsFactRegister` 头注「两个读口分工」、`DutyVerificationStore`）；CC `CONTEXT.md` 税费付款核对与集成规则两句；UC-CC-009 一致性节。

## Comments

- 2026-09-14 14:0x · 通道 1：立票（推送方按 13 裁决 2 立）。只写票面，未动代码。
- 2026-09-14 16:0x · 通道 1（接管会话）：sa-cc/13 补评审 ← 通道 4 Spec 非阻断 2 给本票的一句——`VerifyPayment` 在生产有调用方（`cmd/parcel-api/assemble_customs_registration.go` `transactionalDutyPaymentVerificationRegistration.VerifyPayment`、`cmd/parcel-customs-register/translate.go`）；13 之后同一事实 ≥ 2 版时付款人维读的是最近接收那一版，而 `duty_payment_verification` 键（租户、税费、资金事实、范围、指纹）里没有资金版本——v2 到达后调用方以同三轴同依据重核落`已存在`，形成不出「新核对版本」。**本票立字时把「核对身份缺资金版本」写成完成判据**（做法 3 已接，判据里点名）。同一张键的另一维（程序）在 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md)，两票若同期在途改键合一笔。
- **2026-09-14 21:2x · 取证 ← 通道 4 · 钉 `bccb60a1`**。只读代码、逐符号量「要裁的」两条所需的事实；不裁、不提方案。每条写「取数方法 → 结果」。

  **要裁的 1（新核对版本的三轴与关联依据从哪来）**

  - `internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment` 入参（读源）：`duty AssessedDutyReference, funds ExternalFundsFactReference, scope DecisionScopeReference, coverage DutyCoverage, delta DutyDelta, validity DutyFactValidity, verifiedAt time.Time`——七个全部构造期必填：任一 `valid()` 为假或 `verifiedAt.IsZero()` 即 `ErrInvalidDutyVerification`。入参里**没有**程序、**没有**资金版本。
  - 三轴取值集（同文件 `String()` 词形）：`DutyCoverage` = `NONE / PARTIAL / COVERED`；`DutyDelta` = `NO_DELTA / SHORT / EXCESS / PENDING`；`DutyFactValidity` = `VALID / INVALIDATED / CONFLICTING / PENDING`。**有效性轴今天有 `PENDING` 一格**（`FundsFactPending`），差额轴也有（`DeltaPending`）；库列 CHECK `duty_payment_verification_validity_closed` 同词（`0016`）。另一侧事实：门禁读数 `domain.DutyPaymentGateReading.valid()`（`duty_payment_gate_rule.go`）明文拒 `DeltaPending` / `FundsFactConflicting` / `FundsFactPending`，`0019` `gate_condition_duty_payment_rule_validity_closed` 只放 `VALID / INVALIDATED`——一版有效性为 `PENDING` 的核对能入 `duty_payment_verification`，但按今天的门禁规则表登不进接受集合、读数上也过不了 `valid()`。
  - `DutyPaymentVerification` 结构体字段：`duty / funds / scope / coverage / delta / validity / verifiedAt`，读口各一；**没有** `Basis`——依据在 `ports.DutyVerificationRecord.Basis` 上，不在领域对象里（`ports.go` `DutyVerificationRecord` 头注原句「依据不在领域对象里」）。
  - `internal/customscompliance/ports/ports.go` `DutyVerificationKey` 组成：`TenantID / Duty / Funds / Scope / Digest`。`Digest` 由 `application.verificationDigest(command)` 算：`Coverage`、`Delta`、`Validity` 三个枚举**整数**加 `Basis`，以 `\x00` 拼接后 sha256——头注「三维身份在键上，不进指纹」。`Procedure` 与资金版本都不进指纹、也不在键上。
  - `ports.DutyVerificationStore` 方法集：`FindVerification(ctx, key DutyVerificationKey)` 与 `SaveVerification(ctx, record DutyVerificationRecord)`，共两个方法（钉 `bccb60a1`）。没有按（租户、资金事实）列全部版本的读口。**旁边两口**能按别的维读核对：`CurrentDutyVerificationView.LoadCurrentDutyVerification(ctx, tenant, scope)`（按范围取「当前」一版，头注「监管程序不是核对的维度……所以这里不按边界过滤」；postgres 实现 `duty_payment_gate_rule.go` `LoadCurrentDutyVerification` SQL `ORDER BY verified_at DESC, version_digest ASC LIMIT 1`）与 `DutyVerificationCatalogueRead.ListDutyVerifications(ctx, tenant, limit)`（按租户上列）。
  - `ports.ExternalFundsFactRegister` 头注「两个读口分工」原词：「LoadFundsFact 交回本上下文**最近接收**的那一版——核对（VerifyPayment）今天按引用读前置与付款人维、命令上没有版本，它读的就是这一版；『新版本到达 → 形成新核对版本』的编排归后继票，那张票落地时核对该按版本读。ListFundsFactVersions 按接收先后列全部版本、每版带回指前版——『登记册看得见新版本与回指』（裁决 2）就是这一口；空切片即一版都没接收。」
  - `ports.DutyCollaborationStore` 的形：`FindCollaboration(ctx, tenant, scope, duty)` / `SaveCollaboration(ctx, tenant, collaboration)`；协作事项本体 `domain.DutyCollaborationSpec` 字段 `Kind / Duty / NoPayBasis / Scope / Obligor / Requirement / Target / FormedAt`，库表 `duty_payment_collaboration` 主键 `(tenant_id, scope_ref, duty_ref)`（`0016`），`kind` 封闭二值 `ASSESSED_DUTY / EXPLICITLY_NOT_REQUIRED`，CHECK `duty_payment_collaboration_basis_matches_kind` 只放这两种形状。(b) 路若照它的形立「待重核对事项」，今天这张表上**没有**第三种 `kind`、没有资金事实列、没有「待办」状态列，一范围一税费引用至多一行。
  - [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff` 信封键（`adapters/postgres/duty_payment_verification_handoff.go`）：`dutyPaymentVerificationEventID(key)` = `dutyPaymentVerificationPartitionKey(key) + "/" + Duty + "/" + Funds + "/" + Digest`，其中分区键 = `TenantID + "/duty-payment-verification/" + Scope`。**信封 ID 带的「版本」是 `Digest`（内容指纹），不带资金事实版本**；载荷 `dutyPaymentVerificationPayload` 五维 `tenantId / scope / duty / funds / digest`。
  - SA 侧 sa-cc/09 采用登记册存的引用：`migrations/settlement_accounting/0020_duty_payment_verification_adoption.sql` 表 `duty_payment_verification_adoption` 主键 `(tenant_id, scope_ref, duty_ref, funds_ref, version_digest)`——**照抄 CC 核对主键五维**，无跨 schema 外键；`internal/settlementaccounting/adapters/customscompliance/duty_payment_verification_view.go` 回读时重铸 `ccports.DutyVerificationKey{TenantID, Duty, Funds, Scope, Digest: verification.Version().String()}`——SA 领域里叫 `Version()` 的就是 CC 的 `Digest`。
  - 原文引文（不转述）。UC-CC-009「一致性、幂等与并发」节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖。」同用例「税费付款与放行核对规则」节另一句：「税费、付款或放行事实迟到时，按业务发生时间、适用时间、当前有效性和更正关系形成新核对版本；不得按消息到达顺序覆盖。」CC `CONTEXT.md`「税费付款核对」词条：「`customs-compliance` 将监管核定税费与银行、支付或财务系统提供的外部资金事实，按明确申报范围、法定义务以及来源提供或真实程序要求的付款人、金额、币种、业务时间等维度进行的版本化比较判断。它分别判断覆盖状态、差额和事实有效性，不形成实际付款、客户回收或监管放行。」Rules「税费、放行与案件闭环」段：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断。」Lifecycles「税费付款与放行门禁」段：「外部资金事实接入 → 税费付款核对：按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态；资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态。」

  **要裁的 2（触发要不要区分 `corrects` 在不在）**

  - `application.ReceiveFundsFact`（`reconcile_duty_payment.go`）对回指的全部校验只有一条：`registration.Corrects == registration.Version` → `未受理`。`Corrects` 零值不拒、不查前版是否已到（头注「回指是提供方给的字面，照登不校验前版是否已到」）。
  - `adapters/postgres/duty_payment_reconciliation.go` `RegisterFundsFact`：同一条校验（`a version cannot correct itself`）；写口两次 `INSERT … ON CONFLICT DO NOTHING`——身份行 `external_funds_fact (tenant_id, fact_ref)`，版本行 `external_funds_fact_version (tenant_id, fact_ref, version)`；`correctsColumn` 把零值回指写成 `NULL`。
  - `migrations/customs_compliance/0021_external_funds_fact_versions.sql`：`corrects_version text NULL`，唯一 CHECK `external_funds_fact_version_corrects_another_version` = `corrects_version IS NULL OR (btrim(corrects_version) <> '' AND corrects_version <> version)`；主键 `(tenant_id, fact_ref, version)`。没有「非首版必须带回指」的约束，也没有到本表自身的外键（头注明说不设）。
  - 消费侧路径 `adapters/settlementaccounting/receive_on_adopted_funds_fact.go`：`Version` 取信封所指、`Corrects` 取 `content.Corrects`（SA 只读口交回的事实本体）原样递给 `ReceiveFundsFact`，不加校验。
  - **答案**：同一事实在已有核对版本之后到达一个**不带回指**的新版本（版本字面不同、`Corrects` 零值），代码路径 `ReceiveFundsFact → RegisterFundsFact → INSERT external_funds_fact_version` 全程放行，落成 `corrects_version IS NULL` 的第二行，`ReceiveFundsFact` 答 `已接收`（`FundsFactReceived`）。「有回指」与「已有既往核对版本」在代码上是两个互不蕴含的条件：前者看 `Corrects`，后者今天没有读口能问（`DutyVerificationStore` 无按资金事实列的方法）。

  **两路各自的实测后果（只列不选）**

  - 选 (a)「复用前版三轴与依据、有效性轴标 `PENDING`，形成一版核对」要动的符号：新编排（`application` 新文件，读 `ExternalFundsFactRegister.ListFundsFactVersions` + `DutyVerificationStore` 新增按（租户、资金事实）列全部核对的方法 + `DutyCollaborationStore.FindCollaboration` + `PayerRequirementRuleView.LoadPayerRequirement`）；`ports.DutyVerificationStore` 加方法 → 替身 `internal/customscompliance/application/reconcile_duty_payment_test.go`、`verify_release_gate_test.go`、`cmd/parcel-customs-register/duty_registers_test.go`（`fakeDutyBook`）、`cmd/parcel-dispatch/assemble_test.go` 随形；`adapters/postgres/duty_payment_reconciliation.go` 加实现与 SQL；`VerifyDutyPaymentCommand` 加 `FundsVersion`（做法 3）→ `verificationDigest` 或 `DutyVerificationKey` 之一要带它（进键则见 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md) 取证「改键会拆到谁」那段：`0016` 主键、`0019` 的 `gate_verification.duty_version_digest` 三列引用、SA `0020` 采用表主键、`dutyPaymentVerificationEventID`、`dutyPaymentVerificationPayload`、SA `decodeFormedDutyPaymentVerification`、`duty_payment_verification_view.go` 重铸键）；有效性 `PENDING` 的新版成为 `CurrentDutyVerificationView` 的「当前」后（按 `verified_at DESC` 它就是最新那版），`application.VerifyReleaseGate` 那一道读到它，`domain.DutyPaymentGateRule.Judge` 对 `DeltaPending / FundsFactConflicting / FundsFactPending` 答 `ErrDutyPaymentGateUndecided`，门禁编排折成 `gateUndecided(DutyVerificationPending)`——**该范围的放行门禁在此期间答未决，不是未满足**（`verify_release_gate.go` 该分支实测）。
  - 选 (b)「不形成核对，只登一条待重核对事项」要动的符号：新表或 `duty_payment_collaboration` 加格——后者今天 `kind` 封闭二值 + `basis_matches_kind` CHECK + 主键无资金事实维，加格即新迁移改 CHECK 与键；`ports` 新端口或 `DutyCollaborationStore` 加方法（同上替身随形）；新编排同 (a) 的读口集但不调 `VerifyDutyPayment`、不调 `DutyPaymentVerificationHandoff`（无新核对版本即无信封，[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 与 SA 侧零改动）；`DutyVerificationStore` 仍要加按资金事实列的读口（触发条件「已有既往核对」两路都要问）；`duty_payment_verification` 表与 `0016` 在这条路上不动，`LoadFundsFact`「最近接收」口径的退役与 `VerifyDutyPaymentCommand.FundsVersion` 是否仍做，随裁。

  **能力边界**：读了 `duty_release.go`、`reconcile_duty_payment.go`、`ports.go`、`duty_collaboration.go`（结构体段）、`duty_payment_gate_rule.go`（`DutyPaymentGateReading` / `DutyVerificationReference` 段）、`adapters/postgres/duty_payment_reconciliation.go`、`duty_payment_verification_handoff.go`、`duty_payment_gate_rule.go`（`LoadCurrentDutyVerification`）、`adapters/settlementaccounting/receive_on_adopted_funds_fact.go`（回指段）、`0016` / `0019` / `0020` / `0021`、SA `0020`、SA `duty_payment_verification_view.go`（键重铸段）、SA `duty_payment_verification_consumer.go`（译码段）、UC-CC-009 两节、CONTEXT 四句、13 裁决与「后继票的形」。`verify_release_gate.go`（税费付款那一道的分支）。**没读**：SA 采用编排 `AdoptOnDutyPaymentVerificationAdapter` 本体、任何测试的断言内容、真库（未跑、未占 55432）。
