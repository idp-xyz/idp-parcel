# SA 外部资金事实采用与更正的在线登记面：`parcel-api` 端点两行 + `settlementhttp` 第一份命令文件 + `UnconfiguredIntake` 长出命令 Intake + 管理台写签两阶段（sa-cc/27 裁决 2 第二步）

Category: enhancement
Status: ready-for-agent——**2026-09-15 13:3x 通道 3 随 sa-cc/27 完成记录同笔立（27 裁决 7 (7)）**。族别与形已由 27 裁决 1 / 2 定（ADR-0085 读作「算」；入口两者都要、分两步），本票是第二步，没有再要裁的；只写票面未动代码；取证锚分支 `mcp3-sacc27@aa48912e`（基 `bb7268f0`）与 27 取证条（钉 `7160fe67`）
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

## Comments

- 2026-09-15 13:3x · 通道 3（task-43a6ae28，随 27 完成记录同笔）：立票，Status 直接 ready-for-agent（要裁的为零——族别与两步序 27 已裁）。**只写票面，未动代码。** 能力边界：读过 27 全文与取证条、SA `unconfigured_intake.go` 头注、CC `register_credential_and_duty.go` 与 `assemble_customs_registration.go` 全文；**没读** `settlementhttp` 的 `catalogue_intake.go` 里 `writeProblem` 的签名、`admin_web_endpoint_consumers_gate_test.go` 对无消费方端点怎么答、admin-web CC 写签先例文件——做法 5 与判据 5 因此写成「开工先读」。
