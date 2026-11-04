# 01 后端：商业版本按身份读回单条

Category: enhancement
Status: done-uncommitted
父票：[spec](../spec.md)

## 要做的

照 spec 决定 1、2：ADR（会话代决）、`ports` 三个目录读口各加 `Find…`、`OperationsCatalogue` 的 Postgres 实现、HTTP 处理器 `NewQueryCommercialVersionEndpoint` 与装配（`cmd/parcel-api/endpoints_commercial_catalogue.go` 加一行与 probe）、处理器测试（读口替身）、Postgres 测试（若该包已有带库的测试则照它的方式加一条）。

## 完成判据

- `go build ./...`、`go vet ./...`、`go test ./internal/partycommercial/... ./cmd/...` 退 0。
- 对演示库：`GET /commercial-version-readbacks/SERVICE_PRODUCT/SYN-PROD-CN-SG-EXPRESS/v1` 答 `COMMERCIAL_VERSION_READ`，身份改一字答 `COMMERCIAL_VERSION_NOT_FOUND`。

## 完成记录（2026-11-04）

ADR 取号 0361：`docs/adr/0361-a-commercial-version-can-be-read-back-by-identity-for-operator-review.md`（未跑 `tools/adr-index`，未改 `docs/adr/README.md`）。

动过的文件：

- `internal/partycommercial/ports/ports.go`——三个目录读口各加 `Find…`（服务产品 1、关系 2、策略 7）。
- `internal/partycommercial/adapters/postgres/operations_catalogue.go`、`credit_policy.go`、`customer_service_rule.go`、`pre_acceptance_financial_control_policy.go`——十个 `List…` 各折成私有装载函数 + `List…` / `Find…` 两入口；`versionKey` / `keyOf` / `firstRow` 把键过滤接到同一条语句上，语句与扫描不复制。`ListPreAcceptanceControls`、`ListAsOfPolicyDeclarations` 不动（声明无自己的版本）。
- `internal/partycommercial/adapters/http/query_commercial_version.go`（新）——`NewQueryCommercialVersionEndpoint`，结果码 `COMMERCIAL_VERSION_READ` / `COMMERCIAL_VERSION_NOT_FOUND`，`kind` 回显领域原词。
- `internal/partycommercial/adapters/http/query_service_products.go`、`query_commercial_relations.go`、`query_commercial_policies.go`——读口接口加 `Find…`；结算政策行转写抽成 `settlementPolicyBodyOf`。
- `internal/partycommercial/adapters/http/query_commercial_catalogue_test.go`、`query_commercial_relations_test.go`、`query_delivery_condition_read_back_test.go`——既有替身补 `Find…`。
- `internal/partycommercial/adapters/http/query_commercial_version_test.go`（新）——4 个处理器用例。
- `internal/partycommercial/adapters/postgres/operations_version_readback_test.go`（新）——`FindServiceProduct` 与 `FindSupplierAgreement` 各一条（在册与列表行逐字段相同、不在册、跨租户、别的类别同键）。
- `cmd/parcel-api/endpoints_commercial_catalogue.go`——端点表一行（带探针 `/commercial-version-readbacks/SERVICE_PRODUCT/SYN-PROD-CN-SG-EXPRESS/v1`）与装配；`cmd/parcel-api/unwired_orchestration.go`——`unwiredCommercialCatalogue` 补 `Find…`。

门禁：

- `go build ./... && go vet ./... && go test ./internal/partycommercial/... ./cmd/... -count=1` 退 0（无 DSN，Postgres 包按既有方式跳过）。
- `scripts/test-full.sh -count=1 -v ./internal/partycommercial/adapters/postgres/`（专用一次性库）：312 PASS / 0 FAIL / 0 SKIP，含两条新用例。
- `internal/partycommercial/adapters/http` 191 PASS；`cmd/parcel-api` 88 PASS；`gofmt -l` 空。

未验证：对演示库的 `GET /commercial-version-readbacks/SERVICE_PRODUCT/SYN-PROD-CN-SG-EXPRESS/v1`（演示 API 在鉴权之后，本票未走真渠道；端点表探针已由 cmd/parcel-api 的隔离读与不可存储路径用例经真路由证过形状）。


### 会话补核（2026-11-04）

- 用 land 树的构建另起一只 `parcel-api`（127.0.0.1:19081，环境与演示 API 同一份、同一演示库），管理台 vite 5175 指向它，Playwright 以配置操作者登录后经页面走真读口：`SERVICE_PRODUCT/SYN-PROD-CN-SG-ECON/v1`、`SUPPLIER_AGREEMENT/SYN-SUPPLIER-LASTMILE-SG-01/v1`、`CUSTOMER_CONTRACT/SYN-CONTRACT-01/v1`、`ACCEPTANCE_RULE_PACKAGE/SYN-RULEPKG-01/v1` 均答 200 `COMMERCIAL_VERSION_READ`，`SERVICE_PRODUCT/NOPE/v9` 答 200 `COMMERCIAL_VERSION_NOT_FOUND`；共享的演示 API（19080）未动。
- 非作者评审：见票末「评审」。
