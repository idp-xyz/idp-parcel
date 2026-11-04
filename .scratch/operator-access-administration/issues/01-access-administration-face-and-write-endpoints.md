# 01 后端：操作者接入管理面与操作者册的 HTTP 写面 / 读面

Category: enhancement
Status: resolved——2026-11-04 进 main（本笔提交）
父票：[spec](../spec.md)

## 契约（管理台按此开发，改动要回到这里）

- 能力面 `OPERATOR_ACCESS_ADMINISTRATION`：`accessidentity.CapabilityOperatorAccessAdministration`，可授、可撤、`HoldsAt` 认它；迁移 `migrations/access_identity/00NN` 放开 `operator_grant` 上能力面的 CHECK。
- 读口 `accessidentity.OperatorCatalogue`（或等价命名）：`ListOperators(ctx, tenantID) ([]OperatorStanding, error)`，Postgres 实现按 `operator` 左连 `operator_grant`、`operator_grant_revocation`，主体按登记时刻排序。
- `GET /operators`（面：操作者接入管理；ownIntake）答 200：
  `{ "outcome": "OPERATORS_LISTED", "issuer": "<本进程信任的发行方>", "faces": ["REGISTRY_CONFIGURATION_WRITE", …], "decisionKinds": [...], "workScopeKinds": ["NODE","OFFSITE"], "operators": [ { "issuer", "subject", "basis", "recordedAt", "grants": [ { "grantId", "capabilityFace", "decisionKind"?, "workScope"?: {"kind","reference"}, "effectiveStartsAt", "effectiveEndsAt"?, "basis", "recordedAt", "revocation"?: {"revokedAt","basis"} } ] } ] }`。
- `POST /operator-registrations` 载荷 `{ "issuer", "subject", "basis" }`；`POST /operator-grant-registrations` 载荷同 CLI 的 `grants[]` 单项（`grantId`、`issuer`、`subject`、`capabilityFace`、`decisionKind`?、`workScope`?、`effectiveStartsAt`、`effectiveEndsAt`?、`basis`）；`POST /operator-grant-revocations` 载荷 `{ "grantId", "revokedAt", "basis" }`。租户取自信封；带 `tenantId` 或未知键答 400 `MALFORMED_REQUEST`。
- 答复 `{ "outcome": <OperatorRegistrationOutcome> }`：`RECORDED` 201，其余 200（ADR-0022）；撤销落在调用者自己此刻生效的操作者接入管理授予上答 `REVOCATION_WOULD_LOCK_OUT_CALLER`（新格，加进代数）。
- 未配置 / 401 / 403 三格与 `/operator-grants` 同一套（ADR-0100 决定四）。
- 种子：`scripts/demo-seeds/data/access/02-grants.json` 给配置操作者加一条 `OPERATOR_ACCESS_ADMINISTRATION`（grantId `SYN-GRANT-DEX-ACCESS-ADMIN-01`）。

## 完成判据

- `go build ./...`、`go vet ./...`、`go test ./internal/accessidentity/... ./cmd/...` 退 0；Postgres 用例在一次性库上过。
- 处理器用例：三格准入、400、每个端点的 RECORDED / 重放 / 冲突、防锁死。
- ADR-0363、`PAR-INT-08` 状态格改写、三份生成物重生成。

## 完成记录（2026-11-04）

Claude Code 会话在 `idp-parcel-land` 工作树（main，基 `2cec95178`）就地完成，未提交。

**改动文件**

- 领域：`internal/accessidentity/operator.go`（`CapabilityOperatorAccessAdministration` 入 `checkGrantable`、`GrantableCapabilityFaces`、`RecordedGrant.LocksOutCallerIfRevoked`、`OperatorRevocationWouldLockOutCaller`）；新 `internal/accessidentity/operator_catalogue.go`（读模型 `OperatorCatalogueRow` / `RecordedGrantRow` 与读口 `OperatorCatalogue{ListOperators, FindGrant}`）；`operation_decision.go` 加 `DecisionKinds()`，`work_fact.go` 加 `WorkScopeKinds()`；用例 `operator_catalogue_test.go`。
- 迁移：`migrations/access_identity/0040_operator_access_administration_face.sql`（只重建 `operator_grant_capability_face_decided`，取并集加一词；`all:access_identity` 嵌入，无需另登）。
- Postgres：新 `internal/accessidentity/adapters/postgres/operator_catalogue.go`（`ListOperators` 一条查询左连授予与撤销，按 `o.recorded_at, o.subject, g.recorded_at, g.grant_id` 排；`FindGrant` 按（租户、授予标识））；用例 `operator_catalogue_test.go`。
- 共用翻译：新包 `internal/accessidentity/adapters/registrationjson`（`OperatorItem` / `GrantItem` / `RevocationItem` / `WorkScope` 与 `BindingFrom` / `GrantFrom` / `IntervalFrom` / `WorkScopeFrom` / `RevocationFrom`）；`cmd/parcel-access-register/translate.go`、`translate_device.go` 改为类型别名与委托，错误文案逐字不变，CLI 用例原样通过。
- HTTP：新 `internal/accessidentity/adapters/http/operator_access_administration.go`（四口、认证件 `NewOperatorAccessAdministrationAuthenticator`、未配置处理器、封闭解码、事务壳、防锁死）；用例 `operator_access_administration_test.go`。
- 装配：`cmd/parcel-api/endpoints.go`（面族 `faceOperatorAccessAdministration`、`endpointSources.operatorIssuer`）；`main.go`（`startupChannels.operatorIssuer` 自 `IDP_PARCEL_OPERATOR_OIDC_ISSUER` 取）；`endpoints_operator_identity.go`（四行 `ownIntake: true`，从 `sources.db` 建 `aipg.NewOperatorRegistry` 作登记口与读口）；新 `assemble_operator_access_administration.go` 四只 builder 与用例；`read_target_guard.go` 豁免 `/operators`（不读查询串）；测试跟上：`first_party_contract_test.go`（新族同版本发布）、`unstorable_read_targets_test.go`（豁免对照多一口）、`unstorable_request_bodies_test.go`（`everyOperatorGrant` 多持一格）。
- 种子与文档：`scripts/demo-seeds/data/access/02-grants.json` 加 `SYN-GRANT-DEX-ACCESS-ADMIN-01`；`scripts/demo-seeds/README.md` 配置员那一行点明；`docs/product/PILOT-PARAMETER-REGISTER.md` `PAR-INT-08` 两格改写（可授格多一格、状态格「机制已有在线入口；首个管理授予仍经命令行」+ 演示租户 `S`）。ADR-0363 未改（核对无事实不符）。

**门禁**

- `gofmt -l cmd internal` 空；`go build ./...`、`go vet ./...` 退 0。
- 无 DSN：`go test -count=1 ./internal/accessidentity/... ./cmd/parcel-api/ ./cmd/parcel-access-register/` 全部 ok（顶层 PASS：accessidentity 111、adapters/http 22、parcel-api 89、access-register 8）。
- `scripts/test-full.sh -count=1 ./internal/accessidentity/adapters/postgres/ ./cmd/parcel-api/ ./internal/architecture/ ./migrations/` 四包 ok；`-v` 探针 `TestListOperatorsListsTheTenantInRecordedOrder`、`TestFindGrantReadsOneGrantWithinTheTenant`、`TestOperatorAccessAdministrationAnswersFromTheOperatorChannel`、`TestProductionAssemblyMountsEveryDeclaredLine`、`TestExemptReadLinesAnswerExactlyAsTheirHandlersDo`、`TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck` PASS 6 / SKIP 0。
- `go test ./internal/architecture/` ok（管理台路径门禁对本次后端改动无响；管理台侧归票 02）。

**与契约的差异**

- 读口交回读模型 `[]OperatorCatalogueRow`（绑定 + 登记时刻 + 授予行各带登记时刻）而不是 `[]OperatorStanding`：答复里的 `recordedAt` 不在聚合上，不为此把登记时刻塞进 `OperatorGrant`。
- 时刻按 `RFC3339Nano`（UTC）写，与 `/work-devices` 的 `deactivatedAt` 同一写法；整秒值输出与 RFC3339 相同。
- 答复形状、载荷键、状态码映射与票面契约一致；`tenantId` 或任何未知键、空体、坏 JSON、尾随内容、过不了构造门的值一律 400 `MALFORMED_REQUEST`。
- 防锁死在撤销口的同一笔事务内先 `FindGrant` 再写；授予不在册交给登记口答 `GRANT_NOT_REGISTERED`。

**未做 / 未验**

- 未提交；未跑 `tools/adr-index` 与机制清点（派单明令）。
- 未对真 Dex 走通四口（无发行方环境）；真库证据止于上面的 pgtest 用例。
- `apps/admin-web/**` 未动（工作树里另一会话的改动与本票无关）。


### 会话补核与评审（2026-11-04）

- 对演示库施加迁移 0040（`scripts/demo-seeds/migrate`），经 `parcel-access-register operator-grant` 给配置操作者登了管理授予（RECORDED）；land 构建旁路实例上四个口经管理台走通（见票 02 记录）。
- 非作者评审（两票合一）：APPROVE；照改：注释里的跨文件计数去掉；`GrantableCapabilityFaces` 与 `checkGrantable` 的一致性加用例钉住；ADR-0363 对 ADR-0100 决定六的措辞由「接上」改为「回答」（决定六本身不改，不构成部分停用）；决定五改写为「操作者与授予页的未授予状态区显主体标识」。证据 S。
