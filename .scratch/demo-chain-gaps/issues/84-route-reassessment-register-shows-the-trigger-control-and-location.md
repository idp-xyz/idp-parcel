# 84 路由复核册列出触发的控制依据、位置与来源版本

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [63](./63-network-routing-reassesses-on-a-handover-into-a-node.md)。
Blocked by: [63](./63-network-routing-reassesses-on-a-handover-into-a-node.md)（交接触发的复核随它进 main）
地盘：NR 迁移（认领时现取）、`ports/ports.go` 的 `ReassessmentRecord`、`application/reassess_route.go` 的提交那一步、`adapters/postgres` 的复核登记册与 `route_plan_catalogue.go`（`ListRouteReassessments`）、`ports/route_plan_read.go`（`RouteReassessmentCatalogueRow`）、`adapters/http/query_route_plans.go`（`reassessmentBody`）及其测试与契约夹具；管理台复核册类型与契约测试。与 [73](./73-route-reassessments-register-lists-the-new-plan-legs.md) 动同一批文件，宜同一会话先后做。
出处：[ADR-0215](../../../docs/adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 越权风险点 8；UC-NR-003「权限、隔离与审计」节。

## 现象

`network_routing.route_reassessment` 一行只存判断键、结论与几格评估状态。触发的控制依据（节点收寄 / 权威运输交接）、实际位置与来源版本只进了交接登记册的触发指纹，那是摘要，读不回来。

结果是 `GET /route-plans?register=reassessment` 看得到「这件包裹复核过、仍适用」，看不到在哪个节点复核的：

- 收寄触发的复核，要拿关联里的收寄版本回 PS 采用册对；
- 交接触发的复核，要拿关联里的交接四维回交接册看接收节点。

票 63 的演示取证就是这样两跳对上的。UC-NR-003「权限、隔离与审计」要求审计至少关联控制和实测，今天这一格只有指纹，没有可读的列。

## 三类

机制（复核留痕与查阅口如实呈现）。

## 做什么

- 复核记录加三格：控制依据种类、实际位置、来源版本，由复核编排在提交时从触发取，不另推导。
- 迁移给复核册加三列。旧行没有这三格，读作缺席，不回填、不猜。
- 查阅册与 HTTP 复核行加三个可缺键，键名与 UC-NR-003 的原词对齐；契约夹具与管理台类型同一笔补齐。

## 完成判据

- 真库：收寄触发与交接触发各复核一次，复核册两行分别列出各自的控制依据、位置与来源版本；旧形行三格缺席。
- 演示：按演示动线走到新加坡枢纽接收交接之后，复核册那一行直接写出 `TRANSPORT_HANDOVER_CONTROL` 与 `SYN-NODE-SIN-HUB`，不再要回交接册对。

## 规模

S
