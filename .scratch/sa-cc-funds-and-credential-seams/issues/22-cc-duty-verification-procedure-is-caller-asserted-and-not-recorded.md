# 税费付款核对的监管程序由调用方断言、编排不核它与案件一致、核对记录不带程序：错报 `procedureRef` 可绕过「规则要求而缺失保持未决」，事后也看不出付款人维按哪个程序判

Category: enhancement
Status: draft——2026-09-14 16:0x 通道 1（接管会话）立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ② + Spec 非阻断 ①，评审建议「合一张」；与 12 完成记录判断项 ① ④ 同根；归 CC owner）。只写票面未动代码；取证锚 main `01974923`
Blocked by: 无（12 已进 main；要裁的两条归 CC owner）

## 缺口（取证于 `01974923`，逐符号名）

- `internal/customscompliance/application/reconcile_duty_payment.go` `VerifyDutyPaymentCommand.Procedure`（sa-cc/12 之二加入）：付款人维的规则按它读 `PayerRequirementRuleView`；头注自认「范围与程序不是一对一……编排不从范围推、也不核它与案件实际程序一致」。
- `internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment(...)` 与 `DutyVerificationKey` 都不带 Procedure；迁移 `0016` `duty_payment_verification` 主键 `(tenant_id, duty_ref, funds_ref, scope_ref, version_digest)` 里没有程序——形成的核对**事后看不出付款人维是按哪个程序的规则判的**（ADR-0137 决定一「记录带依据引用」同族缺一维）。
- 后果（评审量的）：调用方错报 `procedureRef` 且该程序登了 `NOT_REQUIRED` → 对一条无付款人的事实形成核对并同事务交结算意图，而案件实际程序要求付款人——「要求而缺失保持未决」被绕；反向错报只多停一次未决，无害。
- 为什么 12 没堵：`VerifyDutyPaymentCommand` 的 Coverage / Delta / Validity / Scope / Basis 本就全由登记方断言、编排不从任一维推另一维（`DutyPaymentVerificationFromJSON` 头注原话），登记方今天已能形成任意核对，Procedure 没给它新能力，信任模型未变。堵它需要一条「范围 / 案件 → 当前有效程序」读口，CC 今天没有：`0019` 门禁规则表主键 `(tenant_id, scope_ref, action, boundary_ref)`，程序不在键里、由申报各自交进来；`domain.CustomsCase` 有 `Procedure` 一格，但核对编排今天不读案件。

## 语言从哪里来

- CC `CONTEXT.md` 税费付款核对词条：付款人是「来源提供或**真实程序**要求的」维度——「真实程序」是案件的属性，不是登记方每次核对时口头报的一个字符串。
- ADR-0137 决定一：门禁判断是登记的事实，记录带依据引用——核对按哪个程序的规则判，是依据的一部分。
- ADR-0029：结果按恢复动作分格——「报的程序与案件程序不一致」的恢复动作是改申报或改案件，与「规则未配置」（补规则）不同格。

## 做法（待裁后写实）

1. **程序来源**（见「要裁的」1）：a) 编排按（租户、范围 / 案件）经新读口取当前有效程序，`procedureRef` 从命令里退役或降为可选核对项；b) 保留调用方交、加一致性核——不一致停在新的未决 reason（如 `PayerProcedureMismatch`，落既有 `DutyReconciliationUndecided`），点名两侧各是什么。
2. **核对记录带程序**（见「要裁的」2）：`VerifyDutyPayment` 与核对记录加 `Procedure`；是否进 `DutyVerificationKey` / 主键随裁决——进键则新迁移改键（`0016` 不改），不进键则只加列 + 读口带出。
3. `registrationjson` / HTTP / CLI 随 1 的取向改口径：a) 路 `procedureRef` 不再必填；b) 路照旧必填。
4. `apps/admin-web` 核对键名帮助文本（12 判断项 ② 记为过时）随本票一并改——若 a) 路则那句直接退役。

## 红线

- 不为任何程序预填「要不要付款人」；不拿范围顶替程序（范围与程序不是一对一，12 头注原话）。
- 「不一致」停未决交人，不自动以任一侧为准。
- `0016` / `0020` 不改；新迁移序号重取。
- 不改 SA。

## 完成判据（待裁后写实）

1. 应用层：报的程序与案件 / 读口程序不一致 → 未决并点名两侧（或 a) 路：命令无程序、编排自取）；一致 → 三停格照 12。
2. 核对记录读回带程序；真库往返；`0016` / `0020` 零 diff。
3. `registrationjson` / HTTP / CLI 口径与 1 一致；`apps/admin-web` 帮助文本同笔改正（12 判断项 ② 收口）。

## 地盘

`internal/customscompliance/{domain,ports,application,adapters/postgres,adapters/registrationjson,adapters/http}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-api` / `cmd/parcel-customs-register` / `cmd/parcel-dispatch` 装配（共享，动前占号）、`apps/admin-web/src/pages/customs/presentation.ts` 一句。SA 侧不动。

## 要裁的

1. **程序从哪来**——归 CC owner：a) 编排经读口从案件 / 范围取当前有效程序（CC 拥有案件，`CustomsCase.Procedure` 在；要新立「范围 → 案件 → 程序」读口，且一范围多案件时怎么办要一并裁）；b) 调用方交 + 编排核一致（读口同样要有，只是命令仍带程序）。裁前 12 的形不动。
2. **核对身份要不要带程序**——归 CC owner：进 `DutyVerificationKey`（同三轴同依据但程序不同算两份核对，主键改）还是只作记录列（同键不同程序 → `已存在`）。与 [19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)「核对身份缺资金版本」是同一张键的两维，若两票同期在途，改键合一笔、迁移序号各自重取。

## 参照

[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（裁决 1 / 2、完成记录判断项 ① ② ④、15:47 补评审 Standards ② + Spec ①）；[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)（同一张键的另一维）；ADR-0137 决定一；ADR-0029；`internal/customscompliance/application/reconcile_duty_payment.go` `VerifyDutyPaymentCommand` 头注；`internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment` / `DutyVerificationKey`；`internal/customscompliance/domain/customs_case.go` `CustomsCase.Procedure`；`migrations/customs_compliance/0016_duty_payment_reconciliation.sql`、`0019_duty_payment_gate_rule_and_reading.sql`。

## Comments

- 2026-09-14 16:0x · 通道 1（接管会话）：立票（sa-cc/12 补评审 ← 通道 3：Standards ②「核对事后看不出付款人维是按哪个程序的规则判的」+ Spec ①「判断项 ① 评为非阻断……立票时与 Standards ② 合一张」）。只写票面，未动代码。能力边界：核过 `VerifyDutyPaymentCommand.Procedure`、`VerifyDutyPayment` 签名、`0016` / `0019` 主键、`CustomsCase.Procedure` 存在；**没读**案件与范围今天怎么关联（一范围一案件还是多案件）——「要裁的」1 那半靠 owner 与作者开工时量。
- **2026-09-14 21:2x · 取证 ← 通道 4 · 钉 `bccb60a1`**。只读代码、逐符号量「要裁的」两条所需的事实（补上一条 Comments 自报「没读」的那半）；不裁、不提方案。每条写「取数方法 → 结果」。

  **要裁的 1（程序从哪来）**

  - `internal/customscompliance/domain/customs_case.go` `CustomsCase` 字段集：`id CustomsCaseID / jurisdiction RegulatoryJurisdictionReference / direction ManifestDirection / procedure CustomsProcedureReference / obligation ObligationScopeReference / parcels []CaseParcelAssociation / roles []CaseRoleSnapshot / establishedAt time.Time`。**没有 `DecisionScopeReference` 一格。** 案件上叫「范围」的是 `ObligationScopeReference`（法定义务范围，`NewObligationScopeReference`），与核对键上的 `DecisionScopeReference`（`disposition_verification.go`：「指名决定明确覆盖的对象或范围」）是**两个不同类型**，`domain` 里没有任一方到另一方的转换函数。
  - `migrations/customs_compliance/0003_case_restriction_gate.sql` 表 `customs_case` 列：`tenant_id / jurisdiction_ref / direction / procedure_ref / obligation_ref / case_id / parcels jsonb / roles jsonb / established_at`；主键 `customs_case_pkey (tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref)`；唯一 `customs_case_id_unique (tenant_id, case_id)`；`0017` 加 GIN `customs_case_parcels_gin`。**表上没有 `scope_ref` 列**（`git grep -n 'scope_ref' -- migrations/customs_compliance/` 命中的表：`regulatory_restriction`、`gate_verification`（`0003`）、`follow_up_target` / `external_manifest_reference`（`0004`）、`disposition_verification`（`0005`）、`gate_condition_catalog` / `gate_condition_finding`（`0008`）、`association_candidate` / `execution_fact`（`0009`）、`release_outcome`（`0015`）、`duty_payment_collaboration` / `duty_payment_verification`（`0016`）、`gate_condition_duty_payment_rule`（`0019`）——`customs_case` 不在其中）。
  - **一个范围今天能不能对应多个案件（看约束）**：库上**不存在「范围 → 案件」这条边**，无外键、无列、无索引可约束它，所以「一对一」与「一对多」两个答案都没有约束在守。可量的是反向事实：`procedure_ref` **在案件主键里**——同一（租户、辖区、方向、义务范围）下每个程序各成一案；`ports.CustomsCaseKey` 头注原句「同一包裹进入另一独立监管程序自然换键（一包裹可关联多个彼此独立的案件）」。
  - 「按范围取案件」的读口：`git grep -n 'Scope' -- internal/customscompliance/ports/` 命中的方法签名带 `scope domain.DecisionScopeReference` 的共八处（钉 `bccb60a1`）：`RestrictionStore.ListByScope`、`GateConditionView.LoadPreconditionFindings`、`DutyPaymentGateRuleView.LoadDutyPaymentGateRule`、`GateConditionRegistry.RegisterGateCatalog` / `RegisterGateFinding`、`DutyPaymentGateRuleRegistry.RegisterDutyPaymentGateRule`、`DutyCollaborationStore.FindCollaboration`、`CurrentDutyVerificationView.LoadCurrentDutyVerification`。**`CustomsCaseStore` 三个方法 `FindByKey(CustomsCaseKey)` / `FindByID(tenant, CustomsCaseID)` / `Save` 都不带范围。** 唯一同时带 `scope_ref` 与 `procedure_ref` 的表是 `0009` `association_candidate`，主键 `(tenant_id, unit_id, procedure_ref, direction, scope_ref)`——它是舱单关联候选册，程序与范围在键里**并列**，不是范围推程序的映射。
  - `procedureRef` 今天的流转：
    - `adapters/registrationjson/translate.go` `dutyPaymentVerificationDocument.ProcedureRef`（`json:"procedureRef"`）→ `DutyPaymentVerificationFromJSON` 经 `domain.NewCustomsProcedureReference(document.ProcedureRef)` **必填**（空白即错），头注「procedureRef 必填（票 sa-cc/12）」；填进 `application.VerifyDutyPaymentCommand.Procedure`。
    - `cmd/parcel-customs-register/translate.go` `commandDutyPaymentVerification = "duty-payment-verification"` 那一格直接调 `registrationjson.DutyPaymentVerificationFromJSON(raw)` 再 `registrar.dutyReconciliation.VerifyPayment(ctx, translated)`——CLI 自己不另写必填，必填在 `registrationjson`。
    - `adapters/http/register_credential_and_duty.go` `DutyPaymentVerificationRegistrationIntake.IntakeDutyPaymentVerificationRegistration(ctx, *http.Request) (application.VerifyDutyPaymentCommand, error)`——仓内唯一实现是 `unconfigured_intake.go` `UnconfiguredIntake.IntakeDutyPaymentVerificationRegistration`，不读报文体、直接答 `ErrAccessChannelNotConfigured`；`cmd/parcel-api/endpoints.go` `/customs-duty-payment-verification-registrations` 挂的就是字面量 `customshttp.UnconfiguredIntake{}`。**HTTP 路今天没有任何一处解析 `procedureRef`**；`cmd/parcel-api/assemble_customs_registration.go` `transactionalDutyPaymentVerificationRegistration.VerifyPayment` 只把已成形的命令包进事务，不碰字段。
    - `application.VerifyPayment` 自己也拒空程序：`strings.TrimSpace(command.Procedure.String()) == ""` → `未受理`；随后只用它做一件事——`PayerRules.LoadPayerRequirement(ctx, tenant, command.Procedure)`；**不递给 `domain.VerifyDutyPayment`、不进 `DutyVerificationKey`、不进 `verificationDigest`**（指纹 = 三轴整数 + `Basis`）。
    - `apps/admin-web/src/pages/customs/presentation.ts` `dutyRegistrationSnapshotHints['duty-payment-verification']` 原文：「键为 tenantId / dutyRef / fundsRef / scopeRef / coverage / delta / validity / basis;三轴各取封闭词:coverage NONE / PARTIAL / COVERED,delta NO_DELTA / SHORT / EXCESS / PENDING,validity VALID / INVALIDATED / CONFLICTING / PENDING——三轴分别给出,本口不从金额相等推任何一轴。basis 是「凭什么把这笔资金关联到这版税费」的权威依据引用,空白答待关联、不关联。同三维换内容是新版本追加,不是冲突;资金事实与协作事项两道前置未齐时原名答回,等前置落册后重发同一份。」——键名清单里**没有 `procedureRef`**，与 `registrationjson` 的必填不同口。
  - `0019` 门禁规则表 `gate_condition_duty_payment_rule` 主键 `(tenant_id, scope_ref, action, boundary_ref)`，外键到 `gate_condition_catalog` 同四列；`boundary_ref` 是 `CustomsProcedureReference` 在门禁键上的名字（`ports.GateVerificationKey.Boundary`），与 `scope_ref` **并列在键里**——同一范围可登多个边界各一条规则。`0020` 付款人规则表 `duty_payment_payer_rule` 主键 `(tenant_id, procedure_ref)`，**没有范围维**——规则按程序一条，`ports.PayerRequirementRuleView.LoadPayerRequirement(ctx, tenant, procedure)` 也只按程序读。程序今天在两张键里的位置：门禁表叫 `boundary_ref` 与范围并列；付款人表叫 `procedure_ref` 独立成键。

  **要裁的 2（核对身份要不要带程序）**

  - `0016` `duty_payment_verification` 主键 `duty_payment_verification_pkey (tenant_id, duty_ref, funds_ref, scope_ref, version_digest)`；除主键外**没有其他索引**（文件内无 `CREATE INDEX`；`0017` 加的三个 GIN / 一个普通索引都不在这张表上）；外键 `duty_payment_verification_funds_fact_received (tenant_id, funds_ref) → external_funds_fact (tenant_id, fact_ref)`。列集：主键五列 + `coverage / delta / validity / basis / verified_at`——没有 `procedure_ref`、没有 `funds_version`。
  - 仓内存核对引用的表 / 列（`git grep -n 'duty_payment_verification' -- migrations/`，钉 `bccb60a1`）：
    - CC `0019` `gate_verification` 加列 `duty_ref / funds_ref / duty_version_digest`（连同门禁自己的 `scope_ref`，头注「范围与门禁同一个，不重复带」）——存的是**核对主键里除租户外的四维字串**，无外键（CHECK `gate_verification_duty_reading_paired` 只管七列同生同灭）；写入方 `application.VerifyReleaseGate` 把 `record.Key.Digest` 赋给 `reading.Verification.Version`，`domain.DutyVerificationReference{Duty, Funds, Version}` 三字段；`ports.GateVersionDigest` 把 `Verification.Duty / Funds / Version` 拼进门禁指纹。
    - SA `migrations/settlement_accounting/0020_duty_payment_verification_adoption.sql` `duty_payment_verification_adoption` 主键 `(tenant_id, scope_ref, duty_ref, funds_ref, version_digest)`——**照抄 CC 主键五维为自己的主键**，头注「键取提供方核对幂等键的全部维度」；无跨 schema 外键。回读路 `internal/settlementaccounting/adapters/customscompliance/duty_payment_verification_view.go` 用这五维重铸 `ccports.DutyVerificationKey` 调 CC `FindVerification`。
    - 信封：`adapters/postgres/duty_payment_verification_handoff.go` `dutyPaymentVerificationPayload` 五维 `tenantId / scope / duty / funds / digest`；`dutyPaymentVerificationEventID` = 分区键 + `Duty / Funds / Digest`；SA `adapters/inbox/duty_payment_verification_consumer.go` `decodeFormedDutyPaymentVerification` 五维缺一即毒丸（`ErrPoisonEnvelope`）。
    - **它们存的都是「主键五维字串」，不是代理键**——没有任何一处存自增 ID 或独立的版本引用串。
  - **改键会不会拆到谁（事实）**：今天键上五维在 CC 表、SA 表、`gate_verification` 三列、信封载荷、信封 ID 五处**各自复制一份**，彼此无外键。若主键加维（`procedure_ref` 和 / 或 `funds_version`）而 `version_digest` 算法不变，则：（i）`gate_verification` 存的（范围、税费、资金、指纹）四维不再唯一指到一行——同四维、不同程序 / 不同资金版本的两版核对可并存；（ii）SA `0020` 主键五维同样不再与 CC 一行一一对应，`duty_payment_verification_view.go` 重铸的键少维、`FindVerification` 的 `WHERE` 五列条件对不上新主键；（iii）信封载荷五维与 `decodeFormedDutyPaymentVerification` 的五维校验少维。若改为**把新维折进 `version_digest`**（`verificationDigest` 加入 `Procedure` / `FundsVersion`）而主键列不加，则五处存的字串形状不变、但同三轴同依据换程序即换指纹成新行——两条改法在这三处的波及不同，此处只列不选。
  - 与 [19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)「核对身份缺资金版本」合看，同一张键若一次加两维（`funds_version` + `procedure`），会动的符号（列文件与符号名，不估行数）：
    - `internal/customscompliance/ports/ports.go`：`DutyVerificationKey`（加字段）、`DutyVerificationRecord`（若程序 / 版本作记录列而非键）、`DutyPaymentVerificationHandoffIntent` 随键、`CurrentDutyVerificationView` 头注「监管程序不是核对的维度」那句。
    - `internal/customscompliance/domain/duty_release.go`：`VerifyDutyPayment` 签名与 `DutyPaymentVerification` 字段（若程序 / 版本进领域对象）；`duty_payment_gate_rule.go` `DutyVerificationReference`（若门禁引用要带新维）。
    - `internal/customscompliance/application/reconcile_duty_payment.go`：`VerifyDutyPaymentCommand`（加 `FundsVersion`）、`VerifyPayment`（构造键与调 `LoadFundsFact` 改按版本读）、`verificationDigest`、`handOffVerification`；`verify_release_gate.go`：`reading.Verification.Version = record.Key.Digest` 那一格与 `ports.GateVersionDigest`。
    - `internal/customscompliance/adapters/postgres/duty_payment_reconciliation.go`：`FindVerification`（`WHERE` 五列）、`SaveVerification`（`INSERT` 列表 + 键 / 对象一致性核）、`rebuildVerification`；`duty_payment_gate_rule.go`：`LoadCurrentDutyVerification`（`SELECT` 列与重铸键）；`duty_reconciliation_catalogue.go`：`ListDutyVerifications`（`SELECT` 与 `ORDER BY` 列、重铸键）；`duty_payment_verification_handoff.go`：`dutyPaymentVerificationPayload`、`dutyPaymentVerificationEventID`、`HandOffDutyPaymentVerification` 的空白校验。
    - `internal/customscompliance/adapters/registrationjson/translate.go`：`dutyPaymentVerificationDocument` / `DutyPaymentVerificationFromJSON`（加 `fundsVersion` 键；`procedureRef` 按裁决 1 去留）；`adapters/http/unconfigured_intake.go` 签名不变（返回零值命令）。
    - `internal/customscompliance/adapters/http/query_duty_verifications.go`（核对上列的 JSON 形随 `DutyVerificationRecord`）。
    - 迁移：`migrations/customs_compliance/` 新序号——改 `duty_payment_verification` 主键（`0016` 不改，判据同 `0021` 头注「改它们建的表，不回写它们」）；`gate_verification` 若要跟着带新维则同笔加列；SA `migrations/settlement_accounting/` 新序号——`duty_payment_verification_adoption` 主键随 CC 五维变七维（票面红线「不改 SA」在这条路上成立不了；若新维折进指纹则 SA 零改动）。
    - SA 侧：`internal/settlementaccounting/adapters/customscompliance/duty_payment_verification_view.go`（键重铸）、`adapters/inbox/duty_payment_verification_consumer.go`（`FormedDutyPaymentVerification` / `decodeFormedDutyPaymentVerification`）、`adapters/postgres/duty_payment_verification_adoption.go`（`INSERT` 列表）——同样只在「主键加列」那条改法上动。
    - 用例 / 替身（文件名）：`internal/customscompliance/application/reconcile_duty_payment_test.go`、`verify_release_gate_test.go`、`adapters/postgres/duty_payment_reconciliation_test.go`、`duty_payment_verification_handoff_test.go`、`adapters/http/query_duty_verifications_test.go`、`cmd/parcel-dispatch/assemble_test.go`、`cmd/parcel-customs-register/duty_registers_test.go` / `translate_test.go`（`procedureRef` 夹具）、`internal/architecture/partition_subject_registry_test.go`（若分区键形变）；`apps/admin-web/src/pages/customs/presentation.ts` 那句键名清单。
    - `0016` 的 `duty_payment_verification_funds_fact_received` 外键钉在身份表 `(tenant_id, fact_ref)`（`0021` 头注写明保留它正是选版本子表的理由）；加 `funds_version` 列后若要外键到 `external_funds_fact_version (tenant_id, fact_ref, version)`，是新增一条外键，不动既有那条。

  **两路各自的实测后果（只列不选）**

  - 要裁的 1 选 a)「编排经读口取当前有效程序」：先要有「范围 → 案件」或「范围 → 程序」这条边——今天 `customs_case` 无 `scope_ref` 列、`CustomsCaseStore` 无按范围的方法、`DecisionScopeReference` 与 `ObligationScopeReference` 无转换；新读口的键与「一范围多案件时怎么办」都无现成约束可依，`association_candidate` 与 `gate_condition_*` 两族都是范围与程序并列成键（不是映射）。`VerifyDutyPaymentCommand.Procedure` 退役则 `registrationjson` 必填、CLI 夹具、`VerifyPayment` 空白校验、`presentation.ts` 帮助文本（今天本就未列 `procedureRef`）随之。
  - 要裁的 1 选 b)「调用方交 + 编排核一致」：同样要那条读口；命令、`registrationjson`、CLI 不动；`VerifyPayment` 在 `LoadPayerRequirement` 前后加一格新 reason（`DutyReconciliationReason` 加值、`String()` 加词），`adapters/http/register_credential_and_duty.go` 结果译码若按 reason 分 HTTP 码则随之。
  - 要裁的 2 选「进 `DutyVerificationKey` / 主键」：上面「合看」清单里主键加列那条改法的全部符号 + SA 两处 + `gate_verification` 三列引用的唯一性变化。选「只作记录列」：`0016` 表加列（新迁移）、`DutyVerificationRecord` 加字段、`SaveVerification` / `FindVerification` / `LoadCurrentDutyVerification` / `ListDutyVerifications` 的列表、`query_duty_verifications.go` 的 JSON——键、指纹、信封、SA、`gate_verification` 五处不动；同键不同程序撞 `ON CONFLICT DO NOTHING` 答 `已存在`（`VerifyPayment` 头注「指纹里已含三轴与依据：撞键即同内容，不必再读回比」这句在程序不进指纹时不再成立——它是事实陈述，列在此供裁）。

  **能力边界**：读了 `customs_case.go`、`duty_release.go`、`reconcile_duty_payment.go`、`ports.go`、`duty_payment_gate_rule.go`（`DutyVerificationReference` / `Judge` 段）、`verify_release_gate.go`（税费付款那一道的分支）、`adapters/postgres/duty_payment_reconciliation.go`、`duty_payment_gate_rule.go`（`LoadCurrentDutyVerification`）、`duty_payment_verification_handoff.go`、`registrationjson/translate.go`（核对文档段）、`adapters/http/register_credential_and_duty.go`（接口段）、`unconfigured_intake.go`、`cmd/parcel-api/endpoints.go`（路由行）、`cmd/parcel-customs-register/translate.go`（分发行）、`presentation.ts` 那句、`0003` / `0009` / `0016` / `0017` / `0019` / `0020` / `0021`、SA `0020`、SA `duty_payment_verification_view.go` / `duty_payment_verification_consumer.go`（键与译码段）、CONTEXT 词条与 Rules 段、ADR-0137 决定一、ADR-0029 决定句。**没读**：`duty_reconciliation_catalogue.go` 与 `query_duty_verifications.go` 全文（只 grep 了 SQL 列）、SA 采用编排本体、任何测试的断言内容、`establish_customs_case.go`（范围如何在建案时被谈到）、真库（未跑、未占 55432）。
