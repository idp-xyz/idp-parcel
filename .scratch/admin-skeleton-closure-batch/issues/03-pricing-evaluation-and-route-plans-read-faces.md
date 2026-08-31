# 价格评价与路由计划两页读面——两上下文已有 http 包，只加 `query_*`

Category: feature
Status: ready-for-agent——MCP-3
Blocked by: 无

## 现状（取证于 `65b6cf2`）

两页的表都在、都有租户维、`adapters/http` 包也都在，缺的只是读面那两件与一行装配。

| 页 | 主表 | 行数 | 上下文 `query_*` 现状 |
|---|---|---|---|
| `pricing-evaluation` | `parcel_pricing.evaluation` | 0 | 有包，`query_*`=2（价卡、参考序列，均已接线） |
| `route-plans` | `network_routing.initial_route`、`plan_applicability`、`route_reassessment` | 0、0、0 | 有包，`query_*`=1（网络目录，已接线） |

`parcel_pricing` 3 表 8 处 `tenant_id`、`network_routing` 15 表 48 处，**ADR-0077 的读面形状逐字
成立**，不需要新 ADR。同上下文里已接线的那两个 `query_*` 就是最近的先例，逐字可比。

## 表会一直是空的，这是预期不是缺陷

两张表的写入方都是编排（评价编排、路由编排），而编排在接入渠道墙后面——见
[spec 事实基线](../spec.md)。所以这两页接完**仍是空册**。

**完成判据只能写成「空态文案说的是『读取入口已配置、登记册为空』，而不是『尚未接线』」，
不得写成「页面有数据」。** 不要为了让页面好看去灌评价或路由计划的种子：那两类是业务事实不是
主数据，造出来就是伪造经营数据，直接破红线。读适配器的真库测试照常写（测试内插行再读回），
那是机制验证，与种子是两回事。

## 做什么

对两页各自：读端口（`ports`）→ 真库读适配器（`adapters/postgres`）+ 真库测试 → 在**既有**
`adapters/http` 包里加 `query_*.go` 与隔离读准入入格。**不动两包里既有的处理器**——它们服务的
是已接线的页，碰了就是拿别人的 live 页冒险。

## 两阶段与次序

- **阶段一**：上述全部。自验绿后向频道交**已验 SHA** 与端点行；**不自改** `cmd/parcel-api`
  装配四件（占号在票 07）。
- **阶段二**：收到 MCP-1「已装配」广播后，两页接真，`liveIds` 加两行——**只加自己那两行，
  不动邻行**。

## 完成判据

两页转 live；空态文案说「读取入口已配置、登记册为空」而非「尚未接线」；含真库全仓绿（注明）；
既有已接线页（`price-card-catalog`、`reference-series`、`network-catalog`、`service-areas`）
回归无变化。

## Comments

- **2026-08-31 MCP-3（阶段一已交，待批 07 装配后做阶段二）**：已验 SHA `694182a`（基 `f3f7c55`，
  worktree 分支 mcp3-skeleton-closure）。计价侧：`ports.EvaluationCatalogueRead` /
  `EvaluationCatalogue`（postgres）/ `NewQueryEvaluationsEndpoint`（GET `/pricing-evaluations`，
  复用既有 `PricingCatalogueIntake`，不另立准入形）。网络侧：`ports.RoutePlanCatalogueRead`
  两口 / `RoutePlanCatalogue`（`plan_applicability` 无租户列，适用性经本租户判断行的计划版本
  LEFT JOIN，作用域由判断行承担）/ `NewQueryRoutePlansEndpoint`（GET `/route-plans`，
  `register=initial-route|reassessment` 一口两册）。真库测试含跨租不可见、空册答空、limit
  非正拒；含真库全仓 `go test -count=1` 绿。既有已接线四页零改动（本票只加新文件）。
  待装配行在完工报里交 MCP-1；页面与 `liveIds` 阶段二动。
