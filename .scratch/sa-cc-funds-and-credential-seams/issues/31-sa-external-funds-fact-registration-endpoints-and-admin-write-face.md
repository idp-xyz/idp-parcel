# SA 外部资金事实采用与更正的在线登记面：`parcel-api` 端点两行 + `settlementhttp` 第一份命令文件 + `UnconfiguredIntake` 长出命令 Intake + 管理台写签两阶段（sa-cc/27 裁决 2 第二步）

Category: enhancement
Status: resolved——**2026-09-15 15:2x 通道 3**（task-cbea007d；分支 `mcp3-sacc31` 基 main `93840328`，代码 tip `08e82a88`，五笔：`4c466db4` settlementhttp 命令文件 + UnconfiguredIntake → `a698f961` parcel-api 事务壳与真库装配用例 → `9c065fb0` 端点表两行 + main（占号内一笔）→ `8ae3a9a4` admin-web 登记签 → `08e82a88` 两处注释去计数 + 清点重生成；本完成记录随第六笔）。此前 in-progress——**2026-09-15 15:06 通道 3 认领**（task-cbea007d；分支 `mcp3-sacc31` 基 main `93840328`，隔离树 `%TEMP%\idp-parcel-mcp3-sacc31`；按 /implement 走，判据 3 各格状态码与判据 4 真库先写 red；每个可编译点一笔并推）。此前 ready-for-agent——**2026-09-15 13:3x 通道 3 随 sa-cc/27 完成记录同笔立（27 裁决 7 (7)）**。族别与形已由 27 裁决 1 / 2 定（ADR-0085 读作「算」；入口两者都要、分两步），本票是第二步，没有再要裁的；只写票面未动代码；取证锚分支 `mcp3-sacc27@aa48912e`（基 `bb7268f0`）与 27 取证条（钉 `7160fe67`）
Blocked by: 无——**27 已进 main（第六批，批 tip `de822820`，2026-09-15 14:4x），本票可派**。此前：27（本笔同时把 27 转 resolved；**开工要等 27 进 main**——端点消费的译装 `adapters/registrationjson`、拒 nil 的 `NewMapExternalFundsHandler` 与真库垂直用例都是 27 落的，在它进 main 前开工会在两条分支上各长一份）

## 缺口（27 取证条钉 `7160fe67`，本票立票时在 `aa48912e` 复量同形）

- `internal/settlementaccounting/adapters/http/`：非测试文件全是 `catalogue_intake.go` / `isolated_read_intake.go` / `unconfigured_intake.go` / `query_*`；`git grep -n -E 'MethodPost|MethodPut|MethodDelete|MethodPatch' -- internal/settlementaccounting/adapters/http ':!*_test.go'` **零命中**；包内 Intake 接口只有 `CatalogueQueryIntake`。SA 今天没有任何写端点。
- `settlementhttp.UnconfiguredIntake` 头注原句「本包只有查阅端点、没有命令面，故本类型也只实现 CatalogueQueryIntake；将来长出命令 Intake 时本类型刻意不实现，未配置装不进命令端点由编译期决定」——**这句在本票落地时变成假话**：ADR-0085 决定二「写准入不另立准入形」，`UnconfiguredIntake{}` 正是命令端点的起步装配（CC `customshttp.UnconfiguredIntake` 同时实现该包全部 `*RegistrationIntake`，装配注释「写准入不另立形，同挂字面量 UnconfiguredIntake{}」）。同包 `IsolatedOperationsReadIntake` 头注「本包没有命令面，将来长出命令 Intake 时本类型刻意不实现，放行装不进命令端点由编译期决定」——**这一句要原样保持成立**（ADR-0078 隔离读准入不扩到写行，编译期排除保持）。
- `cmd/parcel-api/endpoints.go` 装的 SA 端点全是 `settlementhttp.NewQuery*Endpoint(settlementCatalogueIntake, …)`；`cmd/parcel-api/main.go` 里 SA 只装四只只读目录；没有 `assemble_settlement_registration.go`。
- `apps/admin-web/src/pages/settlement/` 四页全是读面（`ChargesBillingPage` / `ReconciliationPage` / `SettlementApplicationPage` / `OperatingMetricsPage` + `api.ts`），无写签。
- 27 落下的、本票要消费的：`internal/settlementaccounting/adapters/registrationjson.ExternalFundsFactFromJSON` / `ExternalFundsFactCorrectionFromJSON`（载荷形状与 CLI 同源——本票端点**不得**另写一份译装）；`application.NewMapExternalFundsHandler(deps) (*Handler, error)` 拒 nil；`cmd/parcel-settlement-register/main.go` `buildRegistrar` 六口装配与 `fundsAnswer` 归格原则（HTTP 状态归格照它的恢复动作判据转写，不另立第二套）。

## 做法（27 裁决 2 第二步原文转写，作者按代码定细节并写判断项）

1. `internal/settlementaccounting/adapters/http/` 新建第一份命令文件（形照 CC `register_credential_and_duty.go`）：两个 Intake 接口（各交回 `application.AdoptFundsFactCommand` / `CorrectFundsFactCommand`，方法名形如 `IntakeExternalFundsFactRegistration(ctx, *http.Request)`）、两个 Registrar 接口（方法名取用例方法名 `AdoptFact` / `CorrectFact`，理由同 CC 头注「为什么不叫 Handle」）、两个端点构造、一个只认 `MethodPost` 的端点体（Intake 失败 → 既有 `writeProblem`，编排返 error → 500 `NO_ANSWER_FORMED`）、一个封闭响应形状（`FundsOutcome.String()` 原名 + `undecidedReason,omitempty` + 续办引用那一格——`FundsHandoffReference()` 非空是「行已落、信封未出」，CLI 归未决格并打出引用，端点要不要在 2xx 里带它还是折成 5xx，作者裁并写判断项，判据与 `fundsAnswer` 同一条：它需要人重跑）。`FundsUndecided` 里哪几格算业务未决按 CC `businessUndecided` 的判据——今天与采用相关的只有 `FundsFactStoreUnavailable`，它是依赖故障那半。
2. `cmd/parcel-api/assemble_settlement_registration.go`（形照 `assemble_customs_registration.go`）：`transactional*Registration{transactor bentoapp.Transactor; inner *application.MapExternalFundsHandler}` 包装实现两个 Registrar、`WithinTransaction` 内调用例；六口全接真（`NewExternalFundsFacts` / `NewFundsMappings` / `NewSettlementApplications` / `outbox.NewStore(db)` + `NewOutboxExternalFundsFactHandoff` / `NewOutboxSettlementApplicationHandoff`、`systemClock{}`），与 `parcel-settlement-register` `buildRegistrar` 同一套；`handler, err :=` 接构造门。
3. `cmd/parcel-api/endpoints.go` 加两行 `settlementhttp.NewRegister*Endpoint(settlementhttp.UnconfiguredIntake{}, …)`，`main.go` 装配入口加一处——**共享接线文件，动前占号**（parallel-sessions「共享接线文件：占号、同笔、逐块核」）。
4. `settlementhttp.UnconfiguredIntake` 加两个方法实现新 Intake（不读请求、只答 `ErrAccessChannelNotConfigured`，参数刻意匿名——同它既有 `IntakeCatalogueQuery` 的形），**并改写头注那句**：不再说「将来长出命令 Intake 时本类型刻意不实现」，改说「命令 Intake 的未配置实现也在此（ADR-0085 决定二写准入不另立形）；隔离读放行 `IsolatedOperationsReadIntake` 不实现命令 Intake，编译期排除保持」；`IsolatedOperationsReadIntake` 一字不动，其头注那句原样成立。
5. `apps/admin-web/src/pages/settlement/` 写签两阶段（ADR-0085 决定三）：表单页如实呈现三态——未配置（403 `ACCESS_CHANNEL_NOT_CONFIGURED`）、登记册治理答案（`FUNDS_FACT_ADOPTED` / `EXISTING_FUNDS_FACT` / `FUNDS_FACT_CONFLICT` / `SOURCE_NOT_ACCEPTED`）、未决；两条命令两张表单或一张带模式切换，作者定。`internal/architecture/admin_web_endpoint_consumers_gate_test.go` 会数 `parcel-api` 端点表与 admin-web 消费方的对应，新端点没有管理台消费方时那道门会说什么，开工先读它。
6. 测试随形：`settlementhttp` 端点用例（先例 `register_credential_and_duty_test.go`：未配置 403、Intake 拒、编排各格到状态码）、`unconfigured_intake` 用例加两格、`IsolatedOperationsReadIntake` 不实现新 Intake 的编译期 / 用例证据、`cmd/parcel-api` 装配用例（真库：经端点体 + 真 Registrar 采用一条 → 版本行 + 信封，与 `parcel-settlement-register` 的 vertical 同判据不同入口）。

## 红线

- 不自动采用：任何回调 / 文件到达都不在本票（UC-SA-005「不因接收回调自动采用」）；端点收的是人工 / 受控写面的载荷。
- 译装只用 `adapters/registrationjson` 那一份——端点侧不得另写一份 JSON → 命令（ADR-0085 决定一「同一登记用例」，27 分家理由）。
- 不改 `FundsOutcome` 代数与采用 / 更正的答案；不改 `NewMapExternalFundsHandler` 之外的 SA 构造器；不改 CC；`cmd/parcel-settlement-register` 不退场、不改（CLI 与端点是同一能力的两口）。
- 不预填真实来源 / 账户 / 币种，夹具全 `SYN-`。
- `cmd/parcel-api/endpoints.go` / `main.go` 动前占号；`IsolatedOperationsReadIntake` 零 diff。

## 完成判据

1. `git grep -n -E 'settlementhttp\.NewRegister' -- cmd/parcel-api/endpoints.go` 命中两行且都挂 `settlementhttp.UnconfiguredIntake{}`；`git grep -n MethodPost -- internal/settlementaccounting/adapters/http ':!*_test.go'` 非零。
2. `settlementhttp.UnconfiguredIntake` 实现两个新 Intake（编译期断言 `var _`）；`IsolatedOperationsReadIntake` **不**实现（用例以类型断言证否）；`unconfigured_intake.go` 头注「将来长出命令 Intake 时本类型刻意不实现」那句已改，`isolated_read_intake.go` 零 diff。
3. 端点用例：未配置 → 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；Intake 拒 → 4xx 带 problem；`FUNDS_FACT_ADOPTED` / `EXISTING_FUNDS_FACT` / `FUNDS_FACT_CONFLICT` / `SOURCE_NOT_ACCEPTED` / `FUNDS_UNDECIDED` 各到作者裁定的状态码，续办引用那一格单独一例。
4. `cmd/parcel-api` 真库装配用例：经真 Registrar 采用一条 → 版本行 + 一封 `settlement-accounting.external-funds-fact.adopted`，重放无第二封（与 `cmd/parcel-settlement-register/vertical_test.go` 同判据）。
5. admin-web：写签页在，`node node_modules/typescript/bin/tsc --noEmit` 0（本机 `pnpm build` 坏在环境，见 workflow.md）；`internal/architecture` 门禁绿（含 `admin_web_endpoint_consumers_gate_test.go`）。
6. `gofmt -l` 空、`go vet ./...` 0；带 DSN `cmd/parcel-api` + SA `adapters/http` + `adapters/postgres` PASS 非 SKIP；`internal/customscompliance` / `cmd/parcel-settlement-register` 零 diff。
7. 清点在 tip 干净检出重生成：SA 适配器 +N（`adapters/http` 命令文件）、`cmd/parcel-api` +N、端点声明 +2，作者预报。

## 地盘

`internal/settlementaccounting/adapters/http/`（新命令文件 + `unconfigured_intake.go` 两方法与头注 + 用例）、`cmd/parcel-api/{assemble_settlement_registration.go（新）, endpoints.go, main.go}`（后两者共享，占号）、`apps/admin-web/src/pages/settlement/`（写签）+ 本票 .md。**零 diff**：`internal/settlementaccounting/adapters/http/isolated_read_intake.go`、`internal/customscompliance/**`、`cmd/parcel-settlement-register/**`、`internal/settlementaccounting/application/**`、迁移。撞点：pp-seams/05（PS）/ sa-cc/29（CC）零重叠；`endpoints.go` 与任何同期动 `parcel-api` 端点表的票撞——占号时点名。

## 参照

[27](27-sa-external-funds-fact-adoption-and-correction-registration-face.md) 裁决 1 / 2 / 3 / 5 / 8 与「完成记录」判断项（`fundsAnswer` 归格、`registrationjson` 字段名、续办引用格式）；[07](07-cc-credential-and-duty-reconciliation-registration-faces.md)（CC 步二端点 + 管理台的先例票）；`internal/customscompliance/adapters/http/register_credential_and_duty.go`（端点体、Registrar 命名、`writeDutyReconciliationAnswer` 的状态码判据）与 `unconfigured_intake.go`；`cmd/parcel-api/assemble_customs_registration.go`（事务壳与 Outbox 同 db）；`internal/settlementaccounting/adapters/http/{unconfigured_intake.go, isolated_read_intake.go, catalogue_intake.go}`（`writeProblem` / `writeJSON` / `codeNoAnswerFormed` / `ErrAccessChannelNotConfigured` 已在）；ADR-0085 决定一至三；ADR-0078（隔离读准入不扩到写行）；ADR-0055；ADR-0137 决定四；UC-SA-005。

## 完成记录（2026-09-15 15:2x 通道 3，task-cbea007d；分支 `mcp3-sacc31` 基 `93840328`，代码 tip `08e82a88`；带 DSN 验证钉 `8ae3a9a4`，其后一笔只改两处注释与清点）

**逐条对判据**：

1. ✅ `git grep -n -E 'settlementhttp\.NewRegister' -- cmd/parcel-api/endpoints.go` 命中两行（`/settlement-external-funds-fact-registrations` / `/settlement-external-funds-fact-correction-registrations`），都挂 `settlementhttp.UnconfiguredIntake{}`；`git grep -n MethodPost -- internal/settlementaccounting/adapters/http ':!*_test.go'` 命中 `register_external_funds_fact.go`（`newFundsRegistrationEndpoint` 的方法门）。
2. ✅ `UnconfiguredIntake` 实现 `ExternalFundsFactRegistrationIntake` / `ExternalFundsFactCorrectionRegistrationIntake`（`unconfigured_intake.go` 两条 `var _`）；`IsolatedOperationsReadIntake` 不实现（`TestIsolatedReadIntakeCannotServeExternalFundsFactRegistration` 以类型断言证否）；头注「将来长出命令 Intake 时本类型刻意不实现」那句已改为「查阅端点与命令端点的未配置实现都在本类型上……隔离读放行 IsolatedOperationsReadIntake 刻意不实现命令 Intake」；`isolated_read_intake.go` 对 `93840328` 零 diff。
3. ✅ 端点用例 `register_external_funds_fact_test.go`：未配置 → 403 `ACCESS_CHANNEL_NOT_CONFIGURED` 且不读体、对一切自报一致；Intake 拒 → 400 `MALFORMED_REQUEST` / 500 `INTAKE_FAILED`；`FUNDS_FACT_ADOPTED` 201、`EXISTING_FUNDS_FACT` / `FUNDS_FACT_CONFLICT` / `SOURCE_NOT_ACCEPTED` 200 原名、`FUNDS_UNDECIDED`（资金事实库不可用）500 `NO_ANSWER_FORMED` 不带 outcome、零值答案 500 `UNNAMED_OUTCOME`；续办引用格单独一例 `TestAnAdoptedFactWhoseHandoffDidNotLeaveIsNotReportedAsAdopted`（首版与重放两路）→ 500 `HANDOFF_NOT_SENT`。各格由真 `MapExternalFundsHandler` 接存储替身造出，不伪造 `FundsResult`。
4. ✅ `cmd/parcel-api/assemble_settlement_registration_test.go`（`TestTheWiredExternalFundsFactRegistrationsRecordAgainstARealDatabase`）：经端点体 + 真 Registrar 采用一条 → 版本行 1 + 一封 `settlement-accounting.external-funds-fact.adopted`（信封 ID 带版本维）；重放 → 200 已存在、无第二封；同版本换金额 → 冲突不覆盖；更正回指链头 → 第二行第二封；回指非链头 → 未受理零行零封。带 DSN PASS 非 SKIP。
5. ✅ admin-web：`SettlementApplicationPage` 加「采用资金事实」签（`MultiRegistrationPanel` 两册）；`node node_modules/typescript/bin/tsc --noEmit` 对本票三个文件零报错（仓级见判断项 (7)）；`internal/architecture` 门禁绿（含 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`）。
6. ✅ `gofmt -l ./internal/ ./cmd/` 空、`go build ./...` 0、`go vet ./...` 0；带 DSN `-p 1 -count=1 -v` `./cmd/parcel-api/... ./internal/settlementaccounting/adapters/http/... ./internal/settlementaccounting/adapters/postgres/... ./internal/architecture/...` → PASS 474 / FAIL 0 / SKIP 0，退出码 0（15:17:53–15:18:07）；`internal/customscompliance/**`、`cmd/parcel-settlement-register/**`、`internal/settlementaccounting/application/**`、`migrations/` 对 `93840328` 零 diff。
7. ✅ 清点在代码 tip 干净检出重生成（`08e82a88`）：settlementaccounting 生产 99→100、测试 80→81、http 适配器 7→8；`cmd/` 生产 63→64、测试 88→89；接入面端点 122→124（settlementaccounting 4→6）。推送方在重放 tip 上按既有流程再生成一次即可，本笔的数只对本树成立。

**逐条对做法**：

1. `internal/settlementaccounting/adapters/http/register_external_funds_fact.go`：`ExternalFundsFactRegistrationIntake` / `ExternalFundsFactCorrectionRegistrationIntake`（方法 `IntakeExternalFundsFactRegistration` / `IntakeExternalFundsFactCorrectionRegistration`）、`ExternalFundsFactRegistrar.AdoptFact` / `ExternalFundsFactCorrectionRegistrar.CorrectFact`、`NewRegisterExternalFundsFactEndpoint` / `NewRegisterExternalFundsFactCorrectionEndpoint`、泛型端点体 `newFundsRegistrationEndpoint`（只认 POST；Intake 失败走既有 `writeCatalogueIntakeProblem`；编排返 error → 500 `NO_ANSWER_FORMED`）、封闭响应 `fundsRegistrationResponse{outcome, undecidedReason omitempty}`、转写 `writeFundsAnswer` 与业务未决名单 `businessUndecidedFunds`。
2. `cmd/parcel-api/assemble_settlement_registration.go`：`fundsRegistrationInTransaction` 事务壳 + `transactionalExternalFundsFactRegistration` / `transactionalExternalFundsFactCorrectionRegistration` 两包装 + `buildSettlementRegistrationOrchestration` 六口全接真（`NewExternalFundsFacts` / `NewFundsMappings` / `NewSettlementApplications` / `outbox.NewStore(db)` + `NewOutboxExternalFundsFactHandoff` / `NewOutboxSettlementApplicationHandoff` / `systemClock{}`），`handler, err :=` 接构造门。
3. `endpoints.go` 两行（形参 `externalFundsFactRegistration` / `externalFundsFactCorrectionRegistration`）+ `main.go` `buildSettlementRegistrationOrchestration(db)` 一处；占号 → 一笔 `9c065fb0` → 释号。
4. `unconfigured_intake.go` 两方法 + 头注改口；`isolated_read_intake.go` 一字未动。
5. `apps/admin-web/src/pages/settlement/`：`api.ts` 加 `FundsRegistrationKind` / `fundsRegistrationEndpoints` / `registerExternalFundsFact` / `fundsRegistrationOutcomeLabels`；`presentation.ts` 加 `problemCodeNotes` / `problemNote` / `fundsRegistrationTitles` / `fundsRegistrationSnapshotHints`；`SettlementApplicationPage.tsx` 改为两签（读签 `ExternalFundsFactsTable` 原样 + 登记签 `MultiRegistrationPanel`）。
6. 用例随形：端点用例（做法 1 文件的 `_test`）、`IsolatedOperationsReadIntake` 证否、与 CLI 同形编译期断言 `TestOnlineExternalFundsFactRegistrationTakesTheSameSnapshotShapeAsTheCLI`、`cmd/parcel-api` 真库装配用例、`endpoints_test.go` 探针表两行、`unwired_orchestration.go` 两占位型。

**判断项**（作者按代码定，票面未裁之处）：

1. **续办引用那一格折成 5xx，不进 2xx。** `FundsHandoffReference()` 非空（行已落、信封未出）→ 500 + 专名 `HANDOFF_NOT_SENT`、不带 outcome。判据与 `fundsAnswer` 同一条：它需要人重发同一份补发同一封，而 HTTP 上「重发同一份」正是 5xx 的恢复动作（ADR-0022）；答 2xx 带原名会让只看状态与 outcome 的调用方（`RegistrationPanel` 正是这样读的）把 CC 永远等不到的那封当成已出。用专名而不折进 `NO_ANSWER_FORMED`：后者说「登记与否未知」，这一格登记是确定的，续办说法不同（管理台 `problemCodeNotes` 逐格分说）。续办引用串本身不进响应：它由租户、事实引用、版本拼成，调用方手里的正是这三样。
2. **`FUNDS_UNDECIDED` 今天全部折 500 `NO_ANSWER_FORMED`。** `FundsUndecidedReason` 三格全是存储不可用（依赖故障），没有业务未决；`businessUndecidedFunds` 名单为空但保留（与 `customshttp.businessUndecided` 同形），日后多一格未决原因默认落进「没形成答案」一侧。响应形状保留 `undecidedReason,omitempty`（票面做法 1 指定），今天无路径填它。
3. **状态码**：`FUNDS_FACT_ADOPTED` 201；`EXISTING_FUNDS_FACT` / `FUNDS_FACT_CONFLICT` / `SOURCE_NOT_ACCEPTED` 200 原名；映射 / 核销那族格两口交不回、不列分派，交回时原名 200 过线；`String()` 为空 → 500 `UNNAMED_OUTCOME`（`codeUnnamedOutcome` 本包此前没有，随本文件加）。
4. **Intake 失败映射复用 `writeCatalogueIntakeProblem`**：三格（未配置 / 畸形 / 故障）是接入渠道这一层的状态，读面与写面同一套；不另立第二份映射，函数名里的 Catalogue 是历史名，本票不改它。
5. **路径**：事物词取 CLI 命令名 `external-funds-fact` / `external-funds-fact-correction`，前缀随本上下文读口 `/settlement-`（CC 那族取 `/customs-<命令名>-registrations` 同一条规则）。
6. **一张表单还是两张**：两册经 `MultiRegistrationPanel` 选册切换（换册即换草稿）——采用与更正的登记输入形状互不相容（更正只带回指与金额，其余键按未知字段拒），一张带模式的表单会把一册草稿送进另一口。登记签挂 `SettlementApplicationPage`（收付款核销页，资金事实册的读签所在），选册词取 `fundsRegistrationTitles`。
7. **门禁对新端点怎么答**：`admin_web_endpoint_consumers_gate_test.go` 只断言前端路径 ⊆ 端点表、不反向——新端点没有管理台消费方时那道门什么也不说；本票加了消费方，两条路径字面量在 `api.ts` 的 `fundsRegistrationEndpoints`，门禁绿。**量到与票面不符之处**：(a) 做法 3 说「`endpoints.go` 两行 + `main.go` 一处」，但 `assembleBusinessEndpoints` 加两个形参必然牵动 `endpoints_test.go`（探针表 + `assembleUnwiredBusinessEndpointsWith` 实参）与 `unwired_orchestration.go`（两占位型），四件在同一占号笔内改完；(b) 判据 5「`tsc --noEmit` 0」在本票三个文件上成立，但**仓级退出码在 `93840328` 已是 2**：`apps/admin-web/src/pages/customs/register-rows.test.ts` 一处 `DutyVerificationRecord.fundsVersion` 必填与夹具可省不合（实测：把本票 admin-web 改动 stash 掉后同一报错仍在），属 CC 页面地盘、非本票；(c) 做法 6「`unconfigured_intake` 用例加两格」——SA 此前没有 `unconfigured_intake_test.go`，两格（403 不读体、对一切自报一致）落在端点用例里，不另立文件。
8. **真库用例的 Intake**：`translatingFundsIntake` 把请求体经 `registrationjson` 译成命令——不是第二份译装，是让用例穿过端点体；生产上那一格仍是 `UnconfiguredIntake{}`。

**验证**：见判据 6 / 7；不带 DSN `go test ./cmd/parcel-api/ ./internal/settlementaccounting/... ./internal/architecture/...` 全 ok；`tsc --noEmit` 本票文件零报错。

**能力边界**：读过 27 全文、CC `register_credential_and_duty.go` 与 `_test`、`assemble_customs_registration.go` 与 `_test`、`unconfigured_intake.go`（两包）、SA `catalogue_intake.go` / `isolated_read_intake.go`、`registrationjson/translate.go`、`parcel-settlement-register/main.go` 与 `vertical_test.go`、`map_external_funds.go` 采用 / 更正两方法、`admin_web_endpoint_consumers_gate_test.go` 全文、admin-web `RegistrationPanel` / `MultiRegistrationPanel` / customs `api.ts` 写签段 / `CustomsCasesPage` 登记签；**没读** ADR-0085 / 0022 / 0137 正文（按 CC 先例头注转写的判据办）、`apps/admin-web` 的路由装配（登记签挂既有页，未加导航项）。

## Comments

- 2026-09-15 13:3x · 通道 3（task-43a6ae28，随 27 完成记录同笔）：立票，Status 直接 ready-for-agent（要裁的为零——族别与两步序 27 已裁）。**只写票面，未动代码。** 能力边界：读过 27 全文与取证条、SA `unconfigured_intake.go` 头注、CC `register_credential_and_duty.go` 与 `assemble_customs_registration.go` 全文；**没读** `settlementhttp` 的 `catalogue_intake.go` 里 `writeProblem` 的签名、`admin_web_endpoint_consumers_gate_test.go` 对无消费方端点怎么答、admin-web CC 写签先例文件——做法 5 与判据 5 因此写成「开工先读」。
