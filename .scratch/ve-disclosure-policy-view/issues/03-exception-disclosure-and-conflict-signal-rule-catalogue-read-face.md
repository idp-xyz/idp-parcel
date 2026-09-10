# 异常披露规则与冲突信号规则两册的目录读面：`/visibility-catalogues` 加两个 `kind`、`CatalogueListRead` 加两法、管理台 `pages/visibility/` 各显一签——[02](./02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md) 步二的写签要跟着这两个读签走

Category: enhancement
Status: ready-for-agent——2026-09-10 通道 4 按通道 1 派单 task-d6660969 立票（推送方裁 02 要裁的 2「读面另立」），要裁的为零；取证锚 `062f5228`；只写票面未动代码
Blocked by: 无（两册的 postgres 写口与读口都已在 main，mech/08；本票只加目录上列与页面）

## 为什么立

票 02「缺口」第 4 条：`/visibility-catalogues` 读口（`VisibilityCatalogueReader`，`adapters/http/query_visibility_catalogues.go`）今天六个 `kind`——`ListMilestoneMappings` / `ListTriageRules` / `ListNotificationPolicies` / `ListClaimEligibilities` / `ListClaimAuthorizations` / `ListDisclosurePolicies`，编译期锁 `ports.CatalogueListRead`——不含异常披露规则（0023）与冲突信号规则（0025）两册。02 已裁 B（CLI + 端点 + 管理台），而伞票纪律「写签跟着读签走」要求写签落在读签旁；两册今天无读签，所以读面先行、另立本票，02 步二 Blocked by 本票（02「裁决」2）。

## 今天的形状（`062f5228` 上量，逐符号名）

- 端口：`internal/visibilityexception/ports/catalogue_read.go` 的 `CatalogueListRead` 六法各交回一种 `*CatalogueRow`；postgres 实现 `adapters/postgres/catalogue_read.go`。
- 两册的读口：mech/08 立的 `ExceptionDisclosureRuleRegistry` / `ConflictSignalRuleRegistry` 只有按键点读（供 `decide_disclosure.go` 与 `derive_projection.go` 消费），没有面向管理台的上列。
- http：`query_visibility_catalogues.go` 按 `kind` 分派到六法，`outcomeVisibilityCataloguesListed` 一格随 `kind` 回显；空册也是这一格（ADR-0077 决定四）。
- 装配：`cmd/parcel-api/endpoints.go` `/visibility-catalogues` 一行；`unwired_orchestration.go` 有它的隔离读占位；`isolated_read_test.go` / `endpoints_test.go` 钉端点表。
- 管理台：`apps/admin-web/src/pages/visibility/catalogue-api.ts` 按 `kind` 取列；三张目录页 `TrackingJudgmentRulesPage.tsx` / `ClaimPrerequisitesPage.tsx` / `DisclosurePoliciesPage.tsx` 各显几册的签，`presentation.ts` 列向。

## 做法（照六册既有的形，一字不新造）

1. **端口**：`CatalogueListRead` 加 `ListExceptionDisclosureRules` / `ListConflictSignalRules` 两法，各交回新行类型 `ExceptionDisclosureRuleCatalogueRow` / `ConflictSignalRuleCatalogueRow`——列只呈现不重建领域对象，字段照 0023 / 0025 两表登记的字面转写（规则键、版本或声明时刻、正文各格）；取值全是开放引用照登，不解释。
2. **postgres**：`catalogue_read.go` 加两条上列查询（租户条件 + `LIMIT`，稳定序照同族六法）；真库用例各一：有行 / 空册 / 他租户不渗。
3. **http**：`VisibilityCatalogueReader` 加两法（编译期锁随之），`kind` 封闭集加 `EXCEPTION_DISCLOSURE_RULE` / `CONFLICT_SIGNAL_RULE` 两值（名字照两册在 CLI 命令与批文里的既有原词，不另起）；契约测试各一例钉「空册也是 LISTED」「行逐键」。
4. **装配**：`/visibility-catalogues` 那一行不变（同一端点，多两个 `kind`）；`unwired_orchestration.go` 的隔离读占位加两法；`isolated_read_test.go` 若按 `kind` 枚举则补两行。共享接线文件动前占号。
5. **管理台**：`catalogue-api.ts` 加两个 `kind` 与两个 Record；两签落在 `DisclosurePoliciesPage.tsx` 旁——异常披露规则与披露策略相邻不同册（票 01 / 02 都强调这一点），签名与来源提示句要把「规则（0023）」与「策略」分开写；冲突信号规则落 `TrackingJudgmentRulesPage.tsx`（它裁的是投影派生里的替代链分叉，与里程碑映射、分诊规则同页）。没登显「未登记」不显默认。

## 红线

- 只读、只列：不新增写口、不动 `decide_disclosure.go` / `derive_projection.go` 的读路径。
- 规则取值全属实例半边（`PAR-VIS-*`）：页面不内置任何一条规则、不给候选。
- 异常披露**规则**与披露**策略**是相邻两册，页面、`kind` 名、提示句一处不混用。

## 完成判据（非作者评审逐项对）

1. `CatalogueListRead` 两法 + 两行类型；postgres 真库三例（有行 / 空册 / 他租户）。
2. http 契约两例；`kind` 集外仍拒（既有那一格）。
3. `cmd/parcel-api` 端点表不多一行、隔离读占位编译通过；`isolated_read_test.go` 绿。
4. 管理台 `tsc --noEmit` 0、`run-tests` 全 pass；两签各显一册、没登显「未登记」。
5. `gofmt -l` 空、`go build` / `go vet` 退 0；机制清点 tip 重生成。

## 地盘

`internal/visibilityexception/ports/catalogue_read.go`、`adapters/postgres/catalogue_read.go` + 测试、`adapters/http/query_visibility_catalogues.go` + 测试；`cmd/parcel-api/unwired_orchestration.go`（占号）、`isolated_read_test.go`（如需）；`apps/admin-web/src/pages/visibility/{catalogue-api.ts, presentation.ts, DisclosurePoliciesPage.tsx, TrackingJudgmentRulesPage.tsx}`。**不动** `adapters/postgres/exception_disclosure_rule.go` / `conflict_signal_rule.go` 与 `endpoints.go`。

## 参照

票 [02](./02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md)「缺口」第 4 条与「裁决」2；票 [01](./01-disclosure-policy-is-a-catalog-not-a-content-factory.md)（相邻不同册）；[mech/08](../../mechanism-executor-triage/issues/08-ve-six-executors-behind-existing-uc-steps.md) VE-a / VE-d；ADR-0077 决定四（空册是内容）；`internal/visibilityexception/ports/catalogue_read.go`；`adapters/http/query_visibility_catalogues.go`；admin-web-page-wiring-frontier/02（六册读面的来历）。

## Comments

- 2026-09-10 · 通道 4（task-d6660969，基 `062f5228`，分支 `mcp4-adr0136`）：立票，ready-for-agent（要裁的为零）。**只写票面，未动代码。** 能力边界：读过 `query_visibility_catalogues.go` 的读口接口与锁缝、`cmd/parcel-api/endpoints.go` 那一行、`pages/visibility/` 文件清单；**没读** `ports/catalogue_read.go` 各行类型的字段、0023 / 0025 两表列定义、三张目录页的 JSX——行类型字段与签落哪一页开工时以代码为准，本票只定「照六册的形、两签分两页、规则与策略不混」。本目录无 spec 文件，子票表无处补。
