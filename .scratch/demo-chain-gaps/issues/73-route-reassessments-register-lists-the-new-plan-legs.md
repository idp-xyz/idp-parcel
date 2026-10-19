# 73 路由复核册上形成了新计划的行列出新计划版本与各段

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [64](./64-route-plan-carries-a-delivery-leg.md) 的隔离评审。
Blocked by: [64](./64-route-plan-carries-a-delivery-leg.md)（段的读法与 `legs` 的行形状随它进 main）
地盘：NR `ports/route_plan_read.go`（`RouteReassessmentCatalogueRow`）、`adapters/postgres/route_plan_catalogue.go`（`ListRouteReassessments`）、`adapters/http/query_route_plans.go`（`reassessmentBody`）及其测试与契约夹具；管理台 `apps/admin-web/src/pages/network/**` 的复核册类型与契约测试。diff 不全在 `apps/admin-web/**`，照常开分支；与 [72](./72-admin-web-route-plans-shows-planned-legs.md) 动同一批前端文件，宜同一会话先后做。
出处：[ADR-0217](../../../docs/adr/0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 越权风险点 6。

## 现象

ADR-0217 决定五的理由是「登记方要声明哪一段，得先看得见有哪几段」。票 64 只让初始路由册的成计划行逐段列出。复核形成的新计划（改路，或原来无路可走、复核时首次成计划）住在复核判断行的 `new_plan` 列，换了版本号，改路时序位也可能变（从下一个可控节点起算，ADR-0131 决定二）。`GET /route-plans?register=reassessment` 的行只有被复核的原计划版本，不给新计划版本，更不列段。登记方在任何查阅口上都拿不到新计划的段引用，末端交接无从声明新计划的交付段。

演示走不到这两种结论，今天不阻断演示。

## 三类

机制（查阅口如实呈现计划内容）。

## 做什么

- 复核册上 `new_plan` 在场的行（结论 `REROUTED` 或 `FIRST_PLAN_FORMED`）带新计划版本，并照初始路由册同一读法逐段列出：按判断口的装载纪律把 `new_plan` 整份过构造门重建，再逐段转写引用、种类、起讫、责任、窗口与依据。坏行让整页答不出，不跳过。复用票 64 的 `catalogueLegsOf` 与 `RoutePlanLegCatalogueRow`，不另立一套段行形状。
- HTTP 复核行加可缺席的新计划版本与 `legs`，键形与初始路由册的 `plannedLegBody` 同一份。
- 契约：Go 侧复核册夹具用例带一条改路行（含交付段），前端复核册类型与 `KeyShape` 同一笔补齐。

## 完成判据

- 真库：一次改路复核落册后，复核册的那一行列出新计划版本与各段，末段是交付段时种类为 `DELIVERY`；没有新计划的行没有这两格。
- 前端契约测试与 Go 夹具用例都绿，任一侧改新加键的键名两侧都红。

## 规模

S
