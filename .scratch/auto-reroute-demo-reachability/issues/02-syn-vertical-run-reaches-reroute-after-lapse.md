# 用 SYN 实例把纵向跑到 `rerouteAfterLapse`：进程测试 + 演示步骤，让自动改路那一支第一次被真键走到

Category: enhancement
Status: ready-for-agent——由 [01](./01-no-runtime-path-reaches-the-reassess-auto-reroute-branch.md) 复核结论拆出（2026-09-04，通道 5，task-f9e0bd40）；依赖链已落，无待裁项，开工第一步是读 `PlanReview` 的失效依据
Blocked by: 无

## 要做什么

票 01 复核完剩下的是一条路而不是一段代码：让 `ReassessRouteHandler.rerouteAfterLapse` 在**真实接受形成的判断键**上读到 `configured=true`，走进自动改路 / 建议 / 禁行三态分派中的至少一格。今天它只被替身（`reroute_after_lapse_test.go`）与合成键直登（`auto_reroute_facts_test.go`）走过。

交付两件：

1. **一条进程级用例**（放 `cmd/parcel-dispatch/`，与 `synthetic_v0_test.go` 同款、复用 `newSYNVerticalFixture`），在真 PostgreSQL 上按顺序走：
   - SYN 网络定义登进 `network_routing` 定义登记册，让 `CreateInitialRouteHandler` 不再停在 `ROUTE_EVIDENCE_NOT_CONFIGURED`，初始路由成立（`initial-route.formed` 入队、路由计划有行）；
   - SYN PC 资格声明让采用成立（`seedSYNPCEligibility` 今天列了硬资格，采用停在 `NOT_ESTABLISHED`；本用例要一份**空硬资格清单**的 SYN 声明，或一份能被 SYN 证据证明的——两者都只记 S），节点收寄或揽收信封经生产路由表投递后 `intake_adoption` 有 adopted 行、`network-intake.recorded` 入队；
   - 让计划失效且候选可用：给同一作用域登一版**变了的**网络定义（或其它会让 `PlanReview` 判 `PlanNoLongerApplicable` 的证据变化——开工第一步读清是哪些），再让复核触发（第二封采用信封，或票 01 症状节说的那条第二跳）；
   - 从路由计划读面（`nrports.RoutePlanRow` / `query_route_plans.go`）取回六维键，喂 `parcel-network-register -kind auto-reroute-facts` 的载荷形状（走 `RegisterAutoRerouteFactsHandler`，不走 seed）；
   - 再触发一次复核，断言 `ReassessmentRecord.RerouteState` 不再是零值、`RerouteBlockers` 与所登事实一致，且按事实分别落进三态之一（至少覆盖「条件全立 → 自动改路决定」与「有阻塞 → 建议」两格）。
2. **一页演示步骤**（放本目录 `demo-runbook.md`，不进 `docs/`）：把上面每一步对应到 `cmd/` 下的受控口与信封类型，写清每一格取值属实例半边、演示用 SYN 只记 S。

## 开工第一步

读 `internal/networkrouting/application/reassess_route.go` 里 `PlanReview` 与 `reviewCandidates` 的依据：**哪一种证据变化让计划失效、候选从哪里来**——票 01 复核没读这一段，本票不许猜。读完把结论写进本票「裁决」节再动代码。

## 红线（照 01）

- 不往 `seed.sh` 塞事实行；事实只经 `RegisterAutoRerouteFactsHandler` 以真键登。
- 阈值与条件取值属 `PAR-NET-14` 实例半边；跑通用的四条件只记 `S`。
- 不改 `cmd/parcel-dispatch/assemble.go`——本票缺的是上游数据，不是装配；生产 `assemble.go` 不带任何 SYN 映射（`NR-APPLICABILITY-S` 那一行的纪律）。
- 不把测试专用 Intake 挂进 `cmd/parcel-api` 的 `assembleBusinessEndpoints`。
- 用例里的 SYN 夹具不得进生产组合根。

## 参照

票 01「复核结论」表；`cmd/parcel-dispatch/synthetic_v0_test.go`、`syn_pc_seed_test.go`、`offsite_pickup_adoption_test.go`（诚实停点的写法）；`internal/networkrouting/application/reassess_route.go`；`internal/networkrouting/adapters/postgres/auto_reroute_facts.go`；`cmd/parcel-network-register/main.go` 的 `executeAutoRerouteFacts`；ADR-0052（网络定义未配置的诚实停点）。

## Comments

- 2026-09-04 · 通道 5：由 01 拆出立票，只写票面，未动代码。
