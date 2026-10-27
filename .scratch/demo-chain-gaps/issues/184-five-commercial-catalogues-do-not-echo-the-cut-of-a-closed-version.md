# 184 五本商业目录不回显截止点：被截止的旧版在管理台仍显原区间与「收尾…」

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-dcg184（代码最后一笔 41d543f36）。此前：in-progress——2026-10-26 会话 tops-20 认领（用户 2026-10-26「你能够来做吗」），分支 claude-dcg184。此前：needs-triage——建议 R2（上线后尽快）：首个租户发新版、收尾旧版之后，管理台这五本目录仍把旧版显成「已生效」和原区间，「收尾…」按钮与 [party-commercial-uiux/15](../../party-commercial-uiux/issues/15-existing-versions-note.md) 的重叠提示都不消，运营会以为没收尾、再点一次（服务端按册面答未受理，不写坏数据，故不列上线前必修）。2026-10-26 会话 `tops-20`（Claude Code）立：tops-54 在 [party-commercial-uiux/16](../../party-commercial-uiux/issues/16-close-out-old-versions.md) 实走发现，经 tops-26 转来；用户 2026-10-26「记」。
Blocked by: 无
地盘（建议）：`internal/partycommercial/adapters/postgres` 的五本目录读（`ListServiceProducts`、`ListCustomerContracts`、`ListAuthorizationRules`、`ListPreAcceptanceFinancialControlPolicies`、`ListCustomerServiceRules`）、对应 `ports` 目录行与 `adapters/http` 目录答复；管理台渲染照供应商协议页（前端切片，可同票或另做）。
出处：party-commercial-uiux/16 完成记录「已知·读面缺口」；[party-commercial-uiux spec](../../party-commercial-uiux/spec.md)「第三段」末条；票 [149](./149-admin-web-commercial-version-closure-forms.md)「目录查阅折入收尾的余项」；[ADR-0233](../../../docs/adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 决定五。

## 现象（读码，钉 main `28922dc48`）

- 供应商协议目录（`ListSupplierAgreements`）按版本点读的口径把收尾折进来：截止回显 `selectionEndsAt`（最新一条更正的终点），退役回显后继与退役时刻，原区间不被覆盖。管理台据此在截止后去掉该行的「收尾…」。
- 服务产品、客户合同、授权规则三本（与供应商协议同在 `OperationsCatalogue`）状态格认退役，但不回显截止点。接受前财务控制策略、客户服务规则两本截止后也不回显截止点；它们认不认退役，逐本核实（票 149 记接受前财务控制不认）。
- 于是截止之后，这五本目录照旧显「已生效」和原区间。

## 要做的

- 五本目录行照供应商协议补三格（截止点及其在场标记、后继版本、退役时刻），读法同 `ListSupplierAgreements`：原区间照旧，收尾另行回显（ADR-0233 决定五）。状态格认退役。
- HTTP 目录答复带出这三格；读面文档若列了目录字段，同步改。
- 管理台五本目录页照供应商协议页渲染这三格：截止后「收尾…」消失，重叠提示不再把被截止的旧版算进去。
- 不在本票：票 149 余项里的接单规则包、价格政策、结算政策三本。

## 完成判据

- [x] 真库用例：五本各一例。旧版按后继截止后，目录回显截止点；同起点退役后，回显后继与退役时刻；原区间都不变。
- [x] 管理台：截止后该行不再给「收尾…」，重叠提示不再算被截止的旧版。
- [ ] Go 门禁；全量 `scripts/test-full.sh`；管理台三道门。

## 三类

回显是机制；哪一版何时收尾是租户取值。

## 规模

S–M

## 完成记录

- 起点 `c8f0f3522`，分支 `claude-dcg184`；代码提交 `41d543f36`（认领提交另见分支历史）。
- 逐本：
  1. 服务产品、2. 客户合同、3. 授权规则：`operations_catalogue.go` 原有 `retiredColumn` 标量子查询换成 `versionClosureJoins("version")` 连接加 `catalogueClosureColumns` 投影（截止点、后继、退役时刻），状态格仍由 `catalogueStatusWord` 折退役；客户合同的 GROUP BY 补入三列。`retiredColumn` 删除（无其他调用）。
  4. 接受前财务控制策略（`pre_acceptance_financial_control_policy.go`）、5. 客户服务规则（`customer_service_rule.go`）：核实现状——两本都走 `commercial_version` 壳，此前**不认退役**（状态格直取版本行状态列）；已照同一形状补连接、投影，并改用 `catalogueStatusWord`，退役后状态为已退役。无册形障碍，未停。
  - 管理台「接受前财务控制策略」页实际读的是 `listCommercialPolicies('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY')`，即 `ListPreAcceptanceFinancialControlPolicies`，不是 `ListPreAcceptanceControls`（后者未动）。
  - ports 五个目录行各补 `SelectionEndsAt/HasSelectionCut/RetiredAt/Successor`；HTTP 答复体经内嵌 `versionClosureBody` 带 `selectionEndsAt/successorVersion/retiredAt`（omitempty，没登正文的壳行同样回显）。
  - 管理台只改 `api.ts`（新增 `VersionClosureEcho`，五个 Record 继承）、`known-versions.ts`（产品、合同、政策册里三本带状态的取 `selectionEndsAt ?? effectiveEndsAt`）、`known-versions.test.ts`（五条）；未动任何 `*Page.tsx`。
- AT：`AT-PC-065`（UC-PC-001，紧接 064；全仓与各 claude- 分支核过无占用），`docs/application` 与 `.scratch/at-coverage-inventory.md` ①分桶表已补行。
- 代决与越权风险点：无新决定，无迁移，无新 ADR；读法全按 ADR-0233 决定五。目录只回显截止点、后继、退役时刻，不回显更正的起点与引用（与供应商协议目录一致）。
- 判据对用例：真库 `TestFiveCommercialCataloguesEchoTheVersionClosure`（五本子用例：旧版按后继截止回显截止点、同起点退役回显后继与退役时刻与已退役状态、原区间终点不变、后继不带收尾）；HTTP `TestFiveCatalogueEndpointsEchoTheVersionClosure`；管理台纯函数五条。
- 自验：`go build ./...`、`go vet ./internal/partycommercial/...`、gofmt 通过；带 DSN `go test -count=1 -p 1 ./internal/partycommercial/... ./cmd/parcel-api/... ./internal/architecture/...` 全绿（含 PG）；管理台 `pnpm install --frozen-lockfile`、`tsc -b --noEmit`、`node scripts/run-tests.mjs`（754 过）、`vite build` 全过；`at-coverage-counts -check` 只报生成物过期（未重跑生成器）。未跑全量 `scripts/test-full.sh`，留给进 main 前。
