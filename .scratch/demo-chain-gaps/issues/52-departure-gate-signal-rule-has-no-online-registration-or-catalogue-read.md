# 52 出发门禁阻断信号规则只有受控 CLI：没有在线登记口，也不进目录查阅面

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [33](./33-movement-fact-recorded-envelope-and-tracking-projection.md) 的完成记录（代决与越权风险点）。
Blocked by: 无（要碰 `cmd/parcel-api/endpoints.go`、`main.go` 两个装配热点与 `apps/admin-web/**`，按方案「并行车道」的装配令牌与前端切片落法排期）
出处：[ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 16；ADR-0085（同一个登记用例同时出 CLI 和端点）。

## 现象

票 33 立了 VE 的「出发门禁阻断信号规则」一册（迁移 VE 0027，`PAR-VIS-04` 之下，形同冲突信号规则），登记用例 `CatalogRegistration.RegisterDepartureGateSignalRule` 已在，受控 CLI `parcel-ve-register departure-gate-signal-rule` 已接。缺两面：

- 在线登记口：冲突信号规则有 `/visibility-catalogue-conflict-signal-rule-registrations`（`operatorRegistries.visibility`），这一册没有；租户只能在数据库网络内跑 CLI 采用。
- 目录查阅面：`/visibility-catalogues` 列冲突信号规则（`ListConflictSignalRules`），不列这一册；管理台看不到演示租户采用了哪一条。

## 证据（读码）

- `internal/visibilityexception/adapters/http/register_catalog.go`：冲突信号规则有 Intake / Registrar 窄口与 `NewRegisterConflictSignalRuleEndpoint`，本册没有。
- `internal/visibilityexception/adapters/registrationjson/translate.go`：本册只有 `DepartureGateSignalRuleFromJSON`（批文带租户），没有 `…ForTenant` 那一路。
- `internal/visibilityexception/ports/catalogue_read.go`：读口与写口成镜像，本册缺 `List…` 一法。

## 三类

机制（入口族照 ADR-0151 定族判据，属登记册配置）。规模 S–M。

## 最小的诚实修法

照冲突信号规则那一册逐件补：`…ForTenant` 翻译、HTTP Intake 与 Registrar 窄口、`OperatorRegistryIntake` 与 `UnconfiguredIntake` 各一法、`cmd/parcel-api` 的事务包装与 `endpoints.go` 一行（只追加）；目录读口加一法、postgres 读面与 `/visibility-catalogues` 一个 kind；管理台 VE 目录页一节走前端切片。

## 要裁的

none（形状照冲突信号规则）

## 既有票

none
