# 60 NR 初始路由计划按处理时钟生效，复核形成的计划按触发业务时间生效：口径不一，原计划离场会被离场门拒

Category: bug
Status: ready-for-agent——建议 R2（上线后尽快）：响亮失败；首发链路上通常撞不上，纯失效一路积压、离线补登时撞得上。初始路由计划的生效时刻取 NR 处理初始路由交接那一刻的处理时钟；ADR-0272 起，复核形成的计划从所判触发的业务时间起生效，被替代的计划也在那一刻离场。原计划经复核离场（改善替代、硬依据失效）时，若触发的业务时间早于初始路由的处理时刻，领域离场门拒（`leaveCurrent` 头注「离场必须带依据与不早于生效的时刻」），整封信封响亮失败、占着分区重投到 `ABANDONED`。改善替代一路要 code/53 先落地才写得进复核册，失效线上自动改路要复核取得到候选成本（demo-chain-gaps/115）；纯失效一路今天就在：首发链路上受理与首个控制触发之间通常隔几小时，通常撞不上；初始路由交接积压、离线补登时撞得上。绕法：无。2026-10-21 协调方据 code/52 隔离评审 N1 立，作者会话起草（用户「你作为业务和系统专家自决」授权）。
Blocked by: 无
出处：docs-consistency/code/52 隔离评审 N1；[ADR-0272](../../../docs/adr/0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md) 越权风险点 2。

## 现象

同一条适用性时间线上混着两种时钟：

1. 初始路由计划的生效时刻是处理时钟：受理之后 NR 消费路由交接、判断初始路由的那一刻。
2. 原计划经复核离场时，离场时刻是触发的业务时间：失效一直如此（UC-NR-003 `AT-NR-064` 那条用例钉住「失效于交接业务时间」），改善替代自 ADR-0272 起也是。
3. 领域 `PlanApplicability` 的离场门要求离场不早于生效。触发的业务时间早于初始路由的处理时刻时，离场门答 `ErrInvalidPlanApplicability`，复核编排响亮报错。

会撞上的情形：初始路由交接积压或重投（受理很早，NR 很晚才处理）；离线补登的交接或收寄，业务时间早于 NR 处理初始路由的时刻；演示或测试夹具用固定的过去时间。

## 证据

- 代码：
  - `internal/networkrouting/application/create_initial_route.go` 的 `planRecord`：`JudgedAt` 与 `EffectiveFrom` 都取 `judgedAt`，即 `judgeParcel` 里 `handler.deps.Clock.Now()`。
  - 同一编排手里有受理的业务时间：路由交接 `domain.RouteHandoff.AcceptedAt()`。
  - `internal/networkrouting/application/reassess_route.go`：`formPlanFromEvidence` 的 `EffectiveFrom` 取 `trigger.OccurredAt()`；`reassessPlan` 失效时 `Lapse(basis, trigger.OccurredAt())`；`tryImprovementReroute` 替代时 `Supersede(..., trigger.OccurredAt())`。
  - `internal/networkrouting/domain/plan_applicability.go` 的 `leaveCurrent`：`at.Before(applicability.transitionedAt)` 即 `ErrInvalidPlanApplicability`；没有登记行时，起点按 `EstablishPlanApplicability(plan.Version(), plan.EffectiveFrom())` 建适用性，`transitionedAt` 就是计划的生效时刻。
- 文档：ADR-0272 决定一、三与越权风险点 2；NR CONTEXT「路由计划适用性」首条；ADR-0174 决定一（并发只比权威业务时间与生效边界）。

## 三类

机制：初始路由计划的生效时刻取哪个时钟。产品策略：无。租户取值：无。

## 规模

S：一处取值改法，加编排与真库用例各一。不加迁移：计划本体存在 jsonb 里，生效时刻随计划落库。

## 最小的诚实修法

1. 初始路由计划从受理的业务时间（`RouteHandoff.AcceptedAt()`）起生效，判断时刻仍取处理时钟，与 ADR-0272 同口径。要判的是「受理时刻」与「接受基线形成时刻」哪个才是业务生效边界：取受理时刻的理由是 CONTEXT「委托接受后……形成初始路由计划」。要不要另立 ADR 由领票时判：若只是把 ADR-0272 的口径扩到初始路由，在 ADR-0272 上挂「扩成」指针即可（部分停用的写法见 docs/adr/README.md）。
2. 编排用例：初始路由交接积压（处理时钟比受理晚十小时），收寄触发的业务时间在两者之间，原计划失效能落库。
3. 派发真库用例同一格。已在册的初始路由计划不回填：它们的生效时刻只会更晚，离场门只会拒得更多，不会放过不该放的。演示库 `scripts/demo-seeds --reset` 重灌即可。

## 相关

- docs-consistency/code/53：改善替代那一行落不了复核册，修好之后改善替代一路才走得到这里。
- demo-chain-gaps/115：复核不取候选成本，失效线自动改路今天形成不了。
- docs-consistency/code/52：复核起点改读当前有效计划，复核形成的计划成为下一次起点；ADR-0272 随它立。

## 并入（2026-10-21，协调方，出自 code/52 第 2 轮修订）

- 初始路由编排 `CreateInitialRouteHandler`（UC-NR-001）遇 `InitialRoutes.FindByKey` 出错仍一律答 `ROUTE_STORE_UNAVAILABLE`、回滚重投；code/52 之后复核一路对同一类坏行（`ports.ErrStoredRouteRecordUnreadable`）响亮报错。本票落地时一并让初始路由编排 `errors.Is` 这一哨兵后响亮报错，其余读不回照旧答未决，补一条用例。

