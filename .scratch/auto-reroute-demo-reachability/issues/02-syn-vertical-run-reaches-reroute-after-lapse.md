# 用 SYN 实例把纵向跑到 `rerouteAfterLapse`：进程测试 + 演示步骤，让自动改路那一支第一次被真键走到

Category: enhancement
Status: blocked——通道 2 于 2026-09-07 按票面「开工第一步」读完 `PlanReview` / `reviewCandidates` 与生产 `InitialRouteEvidenceView`，结论写在下方「裁决」节：交付第 1 件的头一步（登 SYN 网络定义让初始路由成立）在今天的生产装配里**走不到**——唯一的证据视图实现 `nrpostgres.NetworkDefinitions` 对已登记的范围响亮上抛 `ErrNetworkDefinitionUnresolvable`（解析层不在），初始路由停在 `ROUTE_EVIDENCE_UNAVAILABLE`，没有计划就没有复核、更没有 `rerouteAfterLapse`。本票因此不是「可开工」而是等解析层；未动代码。原状态行（通道 5 2026-09-04）：ready-for-agent，「依赖链已落，无待裁项」——票 01 复核结论表 NR-RESOLVER 行自己写了「『有定义时交出九族』的形状本复核未逐字段核，不在此担保」，缺口正在那一格
Blocked by: [first-tenant-runway/03](../../first-tenant-runway/issues/03-network-resolution-layer.md)（网络解析层；它又阻断于 `PAR-NET-14` 实例半边）

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

## 裁决（通道 2，2026-09-07；只读本地 main `2fcc9378`，其与远端 main `ce47b89f` 之间只有一笔 pilot-governance 提交，与本节无交集）

**三问的答案，第三问推翻了票面前提。**

1. **什么让计划失效。** `ReassessRouteHandler.reassessPlan` 调 `domain.ReviewPlanApplicability(plan, trigger.Location(), evidence.HardConstraints)`，失效只有两条确定性依据：**触发的实际位置不在计划节点序列上**（`PLAN_ORIGIN_MISMATCH/<位置>`），或**证据里对选中候选的硬限制判为 `RestrictionApplies`**（`HARD_CONSTRAINT_RESTRICTION/<限制>`）；选中候选限制状态未知则适用性未决。**票面设想的「给同一作用域登一版变了的网络定义」不是失效依据**——`PlanReview` 根本不看定义版本或修订，修订只在 `create_initial_route` 的提交前重校里比对。要让 SYN 计划失效，正路是让采用信封携带的收寄位置落在计划节点之外（`trigger.Location()` 来自采用记录的 `Place`），或在证据里给选中候选一条适用的硬限制。

2. **候选从哪里来。** `reviewCandidates` / `evaluateCandidates` 只吃 `evidence`（`ports.InitialRouteEvidence` 的 `ServiceAreas` / `RouteRequirements` / `PathExecutability` / `HardConstraints` / `Projections` / `Scores` / `Priority` / `Paths`），而 `evidence` 是 `deps.Evidence.LoadInitialRouteEvidence(ctx, key)` 一次取回的。候选不来自任何登记册直读，全部由证据视图交出。

3. **证据视图今天交不出证据——这是票面前提的断点。** 仓内 `ports.InitialRouteEvidenceView` 唯一的实现是 `nrpostgres.NetworkDefinitions`（`grep InitialRouteEvidenceView` 只命中 ports、两个应用编排与它自己），生产 `acceptanceConsumer` 与 `networkIntakeConsumer` 都接它。它的 `LoadInitialRouteEvidence` 只有两格：`network_definition`（迁移 `0007`）无行 → `configured=false`；有行 → **返回 `ErrNetworkDefinitionUnresolvable`**，注释明写「解析层（候选生成、过滤与排序，属 `PAR-NET-14`）不在」且「必须响亮上抛，不能退成`未配置`也不能退成一份空事实」。而 `CreateInitialRouteHandler.Handle` 对该错误答 `ROUTE_EVIDENCE_UNAVAILABLE` 未决。所以：
   - 走 `cmd/parcel-network-register` 登 SYN 目录七族 → 落的是 `0008` 七表，`NetworkDefinitions` 不读它（ADR-0068 Decision 六护栏），初始路由**仍停 `ROUTE_EVIDENCE_NOT_CONFIGURED`**；
   - 直接往 `0007` 插一行（全仓只有 `network_definition_test.go` 这么干，该表零生产写入方，CLI 无此族）→ 初始路由**改停 `ROUTE_EVIDENCE_UNAVAILABLE`**，只是把诚实停点从「未配置」换成「装了却解析不了」。
   两条路都形不成计划；没有计划，`ReassessRouteHandler` 停在 `NO_ROUTING_HISTORY`，即便有历史也会在同一口证据上停 `REASSESS_EVIDENCE_UNAVAILABLE`。**`rerouteAfterLapse` 在生产装配里的可达性，前置是解析层，不是本票能补的上游数据。**

**为什么不用替身绕。** 在测试里另装一份带 SYN 证据替身的 `ReassessRouteHandler` 能让真键读到 `configured=true`，但那不是「经生产 `wireDispatcher` 投递」——`wireDispatcher` 内部固定接 `NetworkDefinitions`，替身进不去；要进去就得改 `assemble.go` 或把夹具放进生产组合根，两条都是本票红线。而且替身版绿了之后，读的人会把它当成演示路径已通——这正是票 01「已接线看着像可用」要避免的形状；`internal/networkrouting/application/reroute_after_lapse_test.go` 已经用替身覆盖了三态分派本身，再来一份只是把同一件事挪到真库上。

**处置。** 本票转 `blocked`，`Blocked by: first-tenant-runway/03`。那张票的完成判据第一句「三个证据视图之一能从合成目录产出逐候选事实并让初始路由形成计划」正是本票缺的那一步；解析层落地之日本票按原两件交付重启，第 1 件第三步（让计划失效）按上面第 1 问改写为「位置不在计划节点上」或「选中候选带适用硬限制」，不再写「定义换版」。本票**没有**可以提前做的一段——事实登记口（`parcel-network-register -kind auto-reroute-facts`）与读键的读面（`nrports.RoutePlanRow`）都已在，缺的只是能产出一份计划的那口证据。

**能力边界**：读过 `reassess_route.go` 全文、`domain/plan_review.go`、`domain/reroute.go`、`adapters/postgres/network_definition.go`、`assemble.go` 的 `acceptanceConsumer` / `networkIntakeConsumer` 与 `dispatchSettings`、`create_initial_route.go` 的证据取数格、`cmd/parcel-network-register/main.go` 的族清单、`synthetic_v0_test.go` 与 `syn_pc_seed_test.go`；`nrparcelshipment` 的 `reassess_on_intake.go` 只 grep 到 `NewActualLocationReference(source.Place().String())` 这一处（第 1 问「正路」那半句据此成立），未读该文件全文；没跑任何进程。证据等级 `S`（静态取证）。

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
- 2026-09-07 · 通道 2：按「开工第一步」读码后写「裁决」节，转 `blocked` 指向 `first-tenant-runway/03`。
  起因是用户要求把 `.scratch` 未收口票逐张收口，本票是当时唯一的 ready-for-agent；读到 `NetworkDefinitions`
  的第二格才发现票面第 1 件头一步走不到。**未动代码，未建 worktree。**
