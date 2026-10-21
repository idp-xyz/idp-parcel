# 52 NR 路由复核读的是初始路由那一行，不是当前有效计划或无路由：失效之后的复核回滚重投到耗尽，或对一份已失效的计划答仍适用

Category: bug
Status: ready-for-agent——建议 R1（上线前必修）：答错且会卡住分区。原计划经一次复核失效之后，下一次控制触发（收寄采用或交接）还是拿那份已失效的初始计划去复核：限制仍在时答 `UNDECIDED/REASSESS_STORE_UNAVAILABLE`，占着交接分区或小包托运分区回滚重投，直到 `ABANDONED`，同一分区后面的信封跟着推迟，失败码还指向一个没坏的依赖；限制解除时对已失效的计划答 `STILL_APPLICABLE`，复核册说这份计划仍适用，适用性册说它已失效，两册打架。ADR-0249 决定二「交进首个节点时，交接复核走『原先无当前有效路由』那条线」那一格，代码里不存在，code/29 的绕法与 ADR-0249 越权风险点 3 的缓解都靠它。没有绕法。2026-10-21 由 code/16 的隔离评审发现（N1），作者会话立票（用户「你作为业务和系统专家自决」授权），分级由协调方定稿。
Blocked by: 无
出处：docs-consistency/code/16 隔离评审 N1。main `14443ac03` 上同病，不是 code/16 引入的。

## 现象

复核编排取「这件包裹现在的路由」时，读的是初始路由判断册（`network_routing.initial_route`）的那一行。那一行只在受理时写一次，之后不变：原计划经复核失效了，它仍是 `ROUTE_FORMED`、仍带原计划；复核自动改路形成了新计划，它也仍带原计划。复核的结论和新计划只落在复核册（`route_reassessment`）与计划适用性册（`plan_applicability`）。

于是第二次复核不知道第一次复核改过什么：

1. **原计划已失效，限制仍在。** 适用性复核又判失效，`applicability.Lapse` 遇已失效的适用性答 `ErrPlanNoLongerCurrent`；编排把它当作「重复触发撞上并发赢家」去按关联读回已有结果，这次触发是新关联，读不到，答未决 `REASSESS_STORE_UNAVAILABLE`、回滚重投，次次如此，直到耗尽派发预算。
2. **原计划已失效，限制已解除。** 适用性复核答仍适用，落一行 `STILL_APPLICABLE`，被复核计划是那份已失效的原计划。CONTEXT 生命周期写的是「原路径后来重新可用时，必须重新评估并形成新的计划版本」，不得原地恢复（`domain.ErrPlanNoLongerCurrent` 的注释同此）。
3. **原计划已被自动改路替代**（读码，未跑探针）。下一次复核仍拿原计划去比：位置在新计划上却不在原计划上时判失效，撞上第 1 格；位置两份计划都在时答仍适用，复核的是一份已被替代的计划，新计划从来没被复核过。
4. **原先无路由那条线只在受理时就无路由的包裹上走得到。** 失效之后形成「无当前有效路由」的包裹走不进 `reassessNoRoute`，ADR-0249 决定二末两条、越权风险点 3，以及 code/29 的绕法「交进下一个节点时交接复核走原先无路由那条线，形成首个当前有效路由计划」，在代码里都不成立。

## 证据

- 代码：
  - `internal/networkrouting/application/reassess_route.go` 的 `Handle`：`handler.deps.Routes.FindByKey(ctx, key)` 取初始路由，`history.HasPlan` 为真即进 `reassessPlan`，否则进 `reassessNoRoute`。
  - `internal/networkrouting/adapters/postgres/initial_route.go` 的 `FindByKey`：只读 `network_routing.initial_route`。
  - `reassessPlan` 的 `PlanNoLongerApplicable` 分支：`applicability.Lapse` 遇 `domain.ErrPlanNoLongerCurrent` 时按本次关联读回，读不到即 `handler.undecided(key, ReassessStoreUnavailable)`。
  - `reassessPlan` 的 `PlanStillApplicable` 分支：不看 `loadApplicability` 的状态就提交仍适用。
  - `rerouteAfterLapse`、`tryImprovementReroute`：新计划只进复核记录的 `NewPlan` 与适用性册的接班行，不进任何「当前路由」读口。
- 探针（code/16 作者会话在分支 claude-code16 上跑，用完即删，未提交）：在 `internal/networkrouting/application` 包里用该包现成的测试夹具（`newReassessFixture`、`currentPlanRecord`、`evidenceBeforeFreeze`，以及 code/16 加的 `restrictedSelectedCandidate`、`pickupCommand`），三步：
  1. 揽收触发、选中候选被限制 → `PLAN_LAPSED`；
  2. 一小时后节点收寄（`node-origin`，在计划上）、限制仍在 → `UNDECIDED / REASSESS_STORE_UNAVAILABLE`；
  3. 再一小时后节点收寄、限制已撤 → `STILL_APPLICABLE`，被复核计划 `plan-1/v1`（已失效的那份）。
- 文档：NR CONTEXT「路由形成与选择」与生命周期「此前处于无当前有效路由时，候选评估完成且有合格候选则形成首个当前有效路由计划」；ADR-0249 决定二末两条与越权风险点 3；UC-NR-003 结果表与 `AT-NR-039`、`AT-NR-053`；code/29 Status 行的绕法。

## 三类

机制：复核按「这件包裹此刻的当前有效计划，或无当前有效路由」取判断起点。产品策略：无。租户取值：无。

## 规模

M：一个读口（或给 `InitialRouteStore` 旁另立「当前路由」读口）、编排取起点的一处改法、失效与替代之后两条线的用例与真库用例。若当前路由要单独落一张表或加索引，再加一次迁移。

## 最小的诚实修法

1. 复核的判断起点改成「当前有效计划，或无当前有效路由」，不读初始路由那一行当起点：
   - 当前有效计划：这一判断键名下各份计划里，适用性为 `CURRENTLY_EFFECTIVE` 的那一份。它可能是初始计划，也可能是某次复核形成的新计划（`REROUTED` 或 `FIRST_PLAN_FORMED` 那一行的 `new_plan`）。适用性册按计划版本建键、没有租户与判断键列（`0005_plan_applicability.sql`），要经判断行联查，口径同 `RoutePlanCatalogue` 读适用性列那一处。
   - 无当前有效路由：初始路由本就无路由，或最近一份计划已失效、没有接班。
   读法可以不加表：取初始路由那一行，再取复核册里业务时间最晚、带新计划的那一行，按计划版本联适用性册。读当前路由与随后的落册要在同一判断键的咨询锁里，口径同 ADR-0235 决定四的 `LatestTriggerAt`。
2. 起点是「已失效、无接班」时走 `reassessNoRoute`。那条线今天落库的形状是复核册 `PLAN_LAPSED`、无被复核计划，以及 `FIRST_PLAN_FORMED`、无被复核计划，两者都过得了逐结论 CHECK，不用改约束。
3. `reassessPlan` 遇 `ErrPlanNoLongerCurrent` 不再折成登记册读不回：修好第 1 条之后它只剩「并发里别人刚换了态」一种来由，要么按判断键串行之后改成响亮报错，要么重读起点重判。
4. 补用例：失效后第二次复核限制仍在（入账，不重投）、限制已解除（不答仍适用，走无路由线、有候选即形成首个计划）、自动改路后下一次复核以新计划为起点；派发真库各一条。`AT-NR-039`、`AT-NR-053` 的现状注随改。

## 要裁的

无新裁定：ADR-0249 决定二、CONTEXT 生命周期已经写明应有的行为，本票只让代码照它走。若实现时发现要改「当前有效路由」的数据所有权（例如单列一张当前路由表），那是落法，不是新决定。

## 迁移与撞号

第 1 条按联查读，不需要迁移。若实现者决定加当前路由表或判断键索引，取 `migrations/network_routing/` 现有最大号加一。code/28（适用性看不清入账未判）也要一次迁移（未判登记加缺口与再判条件两列，并改 reason 的 CHECK），两票并行时会撞号；后进 main 的那一张改号，并照 ADR-0235 那两次迁移的先例核对 CHECK 的 DROP 与 ADD 顺序。

## 相关

- code/16 评审时顺带读到一处疑似同族的问题，未跑探针：仍适用线上自动改路（`tryImprovementReroute`）形成的 `REROUTED` 记录不带失效依据，而复核册 `route_reassessment_rerouted_shape` 要求 `lapse_basis IS NOT NULL`，那一行落不了库。今天复核不取候选成本（demo-chain-gaps/115），这条线在生产上走不到；修 115 时要一起看。
