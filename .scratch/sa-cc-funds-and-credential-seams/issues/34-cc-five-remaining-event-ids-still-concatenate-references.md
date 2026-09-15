# CC 其余五只交接信封 ID（`customsCaseEventID` / `manifestEventID` / `declarationSubmissionEventID` / `caseClosureEventID` / `externalResultEventID`）仍是引用串接形——与 29 修掉的三只同一潜伏缺口：真长度引用下超 `eventing.MaxEventIDLength` 被确定性拒

Category: bug
Status: draft——**2026-09-15 14:4x 通道 1 立票**（sa-cc/29 评审 ← 通道 6 Spec ④ 转记；归 CC owner）。只写票面未动代码；取证锚第六批 tip `de822820`
Blocked by: 无（[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 已进 main——`fingerprintEventID` helper 已在 `internal/customscompliance/adapters/postgres/external_result_handoff.go`，本票只是让其余五只也走它）

## 缺口（评审 ← 通道 6 钉 `b2229462`，逐符号名；推送方未复量）

- 29 把 `dutyPaymentVerificationEventID` / `gateVerificationEventID` / `verificationEventID` 三只改成 `fingerprintEventID(口名前缀, 维…)`（口名前缀 + `sha256` 十六进制，定长 ≤ 128）。同包里 `customsCaseEventID`（五个引用串接）、`manifestEventID`、`declarationSubmissionEventID`、`caseClosureEventID`、`externalResultEventID` **仍是串接形**——ID 长度随租户 / 案件 / 申报等引用的真长度增长，那些是实例半边，本仓给不出上界（29 裁决 1 的同一句理由）。
- 29 裁决 1 的字面只裁了「三只核对交接口」；这五只当时不在 19 量出来的路上，所以不在 29 的地盘，评审把它记为「同一张脸的余数」。
- 这五只的**失败响法**各是什么（折续办 / 翻结果 / 硬失败）本票立票时**没读**——它们不都在重派路上，29 裁决 2 的两格不能照抄，作者开工先逐只量。

## 做法候选（只列不选，归 CC owner）

1. **五只一次同改** `fingerprintEventID`，各给口名前缀常量、维度全进哈希；载荷 / `Subject` / `PartitionKey` 不动。要量：每只的消费方按信封 ID 去重时换形前后同一事实会被当两封——各消费方是否有 ID 之外的幂等键。
2. **只改今天能从生产到达的那几只**，其余等它们的写面接上再改——省的是当下的量，欠的是「同一张脸再长几回」。

## 红线

- 不改载荷与可读形；不改迁移；不改任何消费方语义。
- 实例半边不写死。
- 与 [33](33-sixth-batch-review-standards-tails-test-currency-code-count-words-sentinel-count-and-unanchored-byte-claim.md) 同碰 `external_result_handoff.go`（33 只改一句头注），先后进即可。

## 完成判据（待裁后写实）

1. 五只 ID 定长且 ≤ `eventing.MaxEventIDLength`，超长合成引用下仍入队成功；同输入同 ID。
2. 每只的既有用例改断言形（定长、可重算），不断言字面串接。
3. 带 DSN CC postgres + 各自生产调用方所在 `cmd/` 包全 PASS 非 SKIP；清点零差。

## 地盘

`internal/customscompliance/adapters/postgres/` 五只 `*EventID` 所在文件与测试；各消费方只读不改（若要改幂等键另票）。

## 参照

[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 裁决 0 / 1 与完成记录「裁决 1——helper 与三只口」、Comments「评审 ← 通道 6」Spec ④；[32](32-sa-external-funds-fact-envelope-id-has-no-length-bound-and-a-rejected-envelope-folds-into-a-continuation.md)（SA 侧同款）。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（29 评审 Spec ④ 转记）。只写票面，未动代码。**能力边界**：五只符号名取自评审原文，推送方**未读**这五只的定义、调用方与失败响法；作者开工第一步在 `de822820` 逐只量。
- **2026-09-15 15:05 · 取证 ← 通道 6 · 钉 `93840328`**（task-a2f70396；只读隔离树 `%TEMP%\idp-parcel-mcp6-sacc34-evidence`，未跑真库、未跑 `go test` / `go build` / `go vet`、未占 55432、未动任何代码；只报事实，不裁、不提方案。每条写「命令 / 文件 + 符号名 → 结果」，引用一律符号名或引文，不写行号；计数只在数本身是论点处写并钉 SHA。）

  **共同的底（五只都成立）**

  - **入队与拒收**：五只都把 `eventing.Envelope` 以结构体字面量构造、交 `outboxintent.EnqueueOnce(ctx, db, store, envelope)`；`EnqueueOnce` 按（`ccEventSource`, event_id）先查后插，`outbox.Store.Enqueue` 第一步 `envelope.Validate()`、拒收原样返回，`EnqueueOnce` 以 `%w` 包。**五只都只写 `fmt.Errorf("hand off …: %w", err)`，没有一只像 `duty_payment_verification_handoff.go` 那样对 `eventing.ErrInvalidEnvelope` 分格包 `ports.ErrHandoffEnvelopeRejected`**——确定性拒收与存储不可用在这五只上是同一个错误。
  - **构造门**：五只 ID 里的引用维（`TenantID` / `RegulatoryJurisdictionReference` / `CustomsProcedureReference` / `ObligationScopeReference` / `ExternalManifestID` / `ManifestSourceVersion` / `DeclarationUnitID` / `SubmissionVersionID` / `CustomsCaseID`）全经 `domain.newRequiredValue`——只查 `strings.TrimSpace(value) == ""`，**无长度门**；`ManifestDirection` 是 `uint8` 封闭二值、`String()` 只出 `"IMPORT"` / `"EXPORT"`；`caseClosureCycle` 是 `int`；`ExternalResultKey.SourceID` 是裸 `string`、**无构造函数**（下条细说）。
  - **失败响法**：五只的编排调用方全是「`err == nil` 返 `""`、否则返续办字串」的同一形（05 之形），结果 outcome 不翻、不返 error、不回滚；续办字串挂在各自结果的 `HandoffReference()` / `SubmissionHandoffReference()` / `ResultHandoffReference()` 上。**没有一只在重派一类的编排内被调**——`git grep -n -E '\.(HandOffCase|HandOffManifest|HandOffDeclarationSubmission|HandOffClosure|HandOffExternalResult)\(' -- internal/customscompliance ':!*_test.go'` 的六处命中（申报提交有两处）全在 `application/` 各自 handler 的私有 `handOff*` 方法里，调用者是该用例的 `Handle` 本体，不是另一条编排。
  - **生产装配（数即论点，钉 `93840328`）**：`git grep -n -E 'ccpostgres\.NewOutbox\w+Handoff\(' -- cmd/ ':!*_test.go'` → **3** 处：`NewOutboxDutyPaymentVerificationHandoff` 两处（`cmd/parcel-api/assemble_customs_registration.go`、`cmd/parcel-dispatch/assemble.go`）、`NewOutboxExternalResultHandoff` 一处（`cmd/parcel-api/assemble_external_results.go`）。**五只里只有 `externalResultEventID` 那一只有生产构造**；其余四只的交接与其编排（`NewEstablishCaseHandler` / `NewReceiveManifestHandler` / `NewSubmitDeclarationHandler` / `NewCorrectDeclarationHandler` / `NewCloseCustomsCaseHandler`）在 `cmd/` 非测试**零构造**，今天从任何生产入口都到不了。

  **逐只**

  - **`customsCaseEventID`**（`customs_case_handoff.go`）。(a) `TenantID + "/" + Jurisdiction + "/" + Direction + "/" + Procedure + "/" + Obligation`，五维串接、无指纹；**`PartitionKey: eventID`——ID 本身就是分区键**，`Subject` 取 `intent.Case.ID()`。(b) 四个引用维无长度门，`Direction` 定长二值。(c) 调用方 `EstablishCaseHandler.handOffCase`（`establish_customs_case.go`），结果 `EstablishCaseResult.HandoffReference()`；`cmd/` 零构造。(d) 失败 → `"CONT-CASE/" + customsCase.ID()`，outcome 不翻；今天无生产调用方，续办引用只有测试在读。(e) 消费方：`cmd/parcel-dispatch/assemble.go` 路由表 `veinbox.CustomsCaseEstablishedEventType`（VE 自己写死 `"customs-compliance.customs-case.established"`，不导 CC 常量）→ `veCustomsCaseRouted` → `DeriveOnCustomsCaseAdapter.HandleEstablishedCustomsCase`：按载荷五维 `cases.FindByKey` 重读案件、逐成员调 `derive.Handle` → VE `DeriveProjectionHandler.Handle` 有**自己的幂等键** `ports.FactKey{Tenant, Source, Fact, Version}` + `factContentDigest`：同键同摘要答 `FactExistingResult`（引文「重放：按当前投影作答，不重复派生」），同键异摘要答 `FactSourceConflict`。换 ID 形后同一事实各发一封 → Inbox 门放行第二封 → VE 答已存在。(f) 用例 `customsCaseHandoffEventID(key)` 按同一串接公式重算后以 `event_id` 查行，无字面 ID 断言。
  - **`manifestEventID`**（`manifest_handoff.go`）。(a) `tenant + "/" + manifest + "/" + version`，三维串接、无指纹；分区键另算 `manifestPartitionKey(tenant, manifest)`（不含版本），`Subject` 取舱单身份。(b) `ExternalManifestID` / `ManifestSourceVersion` 无长度门。(c) 调用方 `ReceiveManifestHandler.handOffManifest`（`receive_manifest.go`，两处调用），结果 `ManifestResult.HandoffReference()`；`cmd/` 零构造。(d) 失败 → `"CONT-MANIFEST/" + manifest + "/" + version`，outcome 不翻。(e) 消费方：`git grep -n 'customs-compliance.manifest.recorded' -- cmd/ internal/ ':!*_test.go'` 只命中 `manifestEventType` 常量自身——**零消费者**。(f) 用例有**字面 ID** `"tenant-a/carrier-manifest/MAWB-123/manifest/v1"` / `".../v2"` 与字面分区键 `"tenant-a/carrier-manifest/MAWB-123"`（`partitionKeyOf` 断言），另有 `manifestHandoffEventID(tenant, intent)` 重算公式。
  - **`declarationSubmissionEventID`**（`declaration_submission_handoff.go`）。(a) `declarationSubmissionPartitionKey(key) + "/" + version` = `tenant/unit/procedure/version`，四维串接、无指纹；分区键另算三维目标键，`Subject` 取版本 ID。(b) `DeclarationUnitID` / `SubmissionVersionID` 无长度门。(c) 调用方两只：`SubmitDeclarationHandler.handOff`（`submit_declaration.go`）→ `SubmitDeclarationResult.SubmissionHandoffReference()`；`CorrectDeclarationHandler.handOffCorrection`（`correct_declaration.go`）；两只 handler 与交接在 `cmd/` 零构造。(d) 失败 → `declarationContinuation("DECLARATION_SUBMISSION_HANDOFF", tenant, unit[, version])` = `"CONT-" + sha256 前八字节十六进制`，outcome 不翻。(e) 消费方：路由表 `veinbox.DeclarationSubmissionFormedEventType` → `veDeclarationSubmissionRouted` → `DeriveOnDeclarationSubmissionAdapter`：`submissions.FindByVersion` 重读、再 `derive.Handle` → 同上 VE `FactKey` 幂等。(f) 用例 `declarationEventID("tenant-a", "unit-1", "export-procedure/v1", "version-1")` 以字面各维重算串接后查 `payload` / `event_type`。
  - **`caseClosureEventID`**（`case_closure_handoff.go`）。(a) `tenant + "/" + caseRef + "/" + strconv.Itoa(closureCycle)`，两引用 + 一整数、无指纹；`caseClosureCycle(closure) = len(closure.Reopenings()) + 1`；分区键另算 `caseClosurePartitionKey(tenant, caseRef)`，`Subject` 取案件引用。(b) `CustomsCaseID` 无长度门。(c) 调用方 `CloseCustomsCaseHandler.handOffClosure`（`close_customs_case.go`，两处调用）→ `CloseCustomsCaseResult.HandoffReference()`；`cmd/` 零构造。(d) 失败 → `"CONT-CLOSURE/" + caseRef`，outcome 不翻。(e) 消费方：`"customs-compliance.case-closure.recorded"` 只命中常量自身——**零消费者**。(f) 用例 `closureEventID("tenant-a", caseRef, 1|2)` 与 `closurePartitionKey` 重算，断言两周期同分区键；无裸字面 ID。
  - **`externalResultEventID`**（`external_result_handoff.go`）。(a) `TenantID + "/" + SourceID`，两维串接、无指纹；**`PartitionKey: eventID`——ID 本身就是分区键**，`Subject` 取 `SourceID`。(b) `SourceID` 在 `ports.ExternalResultKey` 上是裸 `string`：唯一的门是 `ReceiveExternalResultHandler.Handle` 里 `strings.TrimSpace(command.SourceID) == ""` 与交接口的 `== ""`，**无构造函数、无长度门**。(c) 调用方 `ReceiveExternalResultHandler.handOff`（`receive_external_result.go`）→ `ReceiveExternalResultResult.ResultHandoffReference()`。**生产装配**：`cmd/parcel-api/assemble_external_results.go` `buildExternalResultsOrchestration` 构造 `NewOutboxExternalResultHandoff(db, store, clock)` + `NewReceiveExternalResultHandler(deps)`，包进 `transactionalResults`（`WithinTransaction`）；`endpoints.go` 表 `{Pattern: "/customs/external-results", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.UnconfiguredIntake{}, results)}`——五只里**唯一**从生产入口到得了的一只；Intake 是字面量 `UnconfiguredIntake{}`。(d) 失败 → `resultContinuation("EXTERNAL_RESULT_HANDOFF", tenant, sourceID)` = `"CONT-" + 八字节十六进制`，outcome 不翻；`transactionalResults` 头注原句「意图投递失败不翻结果——编排吞成续办引用后照常作答，重放重发同一份」；HTTP 响应 `adapters/http/receive_external_result.go` 的响应体有 `HandoffReference` 字段（标签 `json:"handoffReference,omitempty"`），取自 `result.ResultHandoffReference()`——续办引用**有读者**（在线调用方），与 05 / 29 裁决 3 的人重核路同形。(e) 消费方：`"customs-compliance.external-result.received"` 只命中常量自身——**零消费者**（Outbox 行会入队、无人认领）。(f) 用例 `resultEventID("tenant-a", "resp-1")` 重算后 `countResultIntents` 计数，无裸字面 ID。

  **另量：同包第六只以上**

  - `git grep -n -E 'func \w+EventID\(' -- internal/customscompliance/adapters/postgres ':!*_test.go'`（钉 `93840328`）→ **9** 只函数：指纹形 3（`dutyPaymentVerificationEventID` / `gateVerificationEventID` / `verificationEventID`）、helper 1（`fingerprintEventID`）、串接形 5（本票五只）。**但这条 grep 漏两只不经 `*EventID` 函数、在结构体字面量里直接赋 `ID:` 的交接**：① `follow_up_handoff.go`——`eventID: base` / `base + "/replacement-proposed"` / `base + "/replacement-effective"`，`base := followUpPartitionKey(intent.Key)` = `TenantID + "/" + Trigger + "/" + Version + "/" + Kind`，**串接形第六只**；② `restriction_handoff.go`——`ID: eventing.EventID(intent.Restriction.ID().String())`，单一 `RestrictionID`（`NewRestrictionID` 经 `newRequiredValue`，无长度门），不是串接但同样无上界。同包 `*_handoff.go` 非测试文件 **10** 份（钉 `93840328`）；`NewOutboxFollowUpHandoff` / `NewOutboxRestrictionHandoff` 在 `cmd/` 非测试零构造。
  - 与 29 裁决 1 的一个差别只记事实：29 把 `PartitionKey` 保留可读形，三只的分区键本来就是另算的函数；本票五只里 `customsCaseEventID` 与 `externalResultEventID` 两只把 **ID 直接当分区键**，其余三只分区键另算。

  **能力边界**：只读 `93840328` 隔离树；未跑真库、未跑 `go test` / `go build` / `go vet`、未占 55432、未动代码。读了：五只 `*_handoff.go` 从载荷结构到 `HandOff*` 方法尾的正文；`ports.go` 里 `CustomsCaseKey` / `DeclarationSubmissionKey` / `ExternalResultKey` 与五只 `*HandoffIntent` 的声明；`domain` 里 `newRequiredValue`、`NewTenantID` … `NewRestrictionID` 各构造函数与 `ManifestDirection.String()`；六处 `handOff*` 调用方法全文与各结果的 `*HandoffReference()` 访问器名；`cmd/parcel-api/assemble_external_results.go` 全文、`endpoints.go` / `main.go` 命中行；`cmd/parcel-dispatch/assemble.go` 路由表；`veinbox.customs_case_consumer.go` 头注与载荷形、`DeriveOnCustomsCaseAdapter.HandleEstablishedCustomsCase` 正文、`DeriveOnDeclarationSubmissionAdapter` 的 `FindByVersion` / `derive.Handle` 两行、VE `DeriveProjectionHandler.Handle` 的 `FactKey` 查重段；五只 `*_handoff_test.go` 的 ID helper 与字面断言行；框架 `outbox/enqueue.go` `Store.Enqueue` 首段与 `outboxintent/enqueue_once.go` 包错行。**没读**：`follow_up_handoff.go` 与 `restriction_handoff.go` 的 `HandOff*` 方法正文；`customshttp.NewReceiveExternalResultEndpoint` / `UnconfiguredIntake` 对 `/customs/external-results` 的准入行为；VE 两只消费者的测试；`DeriveOnDeclarationSubmissionAdapter` 正文其余段；ADR-0043 / ADR-0069 原文（只经代码注释转述）；五只编排各自的 `Handle` 本体（只读了 `handOff*` 私有方法）。grep 结果以上列命令在 `93840328` 复跑为准。
