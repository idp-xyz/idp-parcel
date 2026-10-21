# 52 NR 路由复核读的是初始路由那一行，不是当前有效计划或无路由：失效之后的复核回滚重投到耗尽，或对一份已失效的计划答仍适用

Category: bug
Status: in-progress——第 1 轮评审已修，待进 main，分支 claude-code52（已 rebase 到 main `4e7874ba0`），见文末「完成记录」「评审记录」。此前 ready-for-agent——建议 R1（上线前必修）：答错且会卡住分区。原计划经一次复核失效之后，下一次控制触发（收寄采用或交接）还是拿那份已失效的初始计划去复核：限制仍在时答 `UNDECIDED/REASSESS_STORE_UNAVAILABLE`，占着交接分区或小包托运分区回滚重投，直到 `ABANDONED`，同一分区后面的信封跟着推迟，失败码还指向一个没坏的依赖；限制解除时对已失效的计划答 `STILL_APPLICABLE`，复核册说这份计划仍适用，适用性册说它已失效，两册打架。ADR-0249 决定二「交进首个节点时，交接复核走『原先无当前有效路由』那条线」那一格，代码里不存在，code/29 的绕法与 ADR-0249 越权风险点 3 的缓解都靠它。没有绕法。2026-10-21 由 code/16 的隔离评审发现（N1），作者会话立票（用户「你作为业务和系统专家自决」授权），分级由协调方定稿。
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

## 完成记录（2026-10-21，分支 claude-code52）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 NR owner 作出，待 NR owner 复核。照票面「最小的诚实修法」与 ADR-0249 决定二、CONTEXT 生命周期实现，读码后没有发现与已接受 ADR 相抵之处。

### 做了什么

- 复核册加读口 `ports.ReassessmentStore.FormedPlans`（评审 N2 后由「取最近一份」改成）：按判断键交回形成过的全部新计划（`REROUTED` 或 `FIRST_PLAN_FORMED` 那一行的 `new_plan`），迁移 0022 之前没有业务时间的行一并读回；先后只为读来可复现。在册计划重建不出来交 `ports.ErrStoredRoutePlanUnreadable`。真库实现在 `internal/networkrouting/adapters/postgres/route_reassessment.go`。没有加表，也没有迁移。
- 复核编排 `Handle` 改由 `startingPoint` 定判断起点：路由历史里的计划是初始计划加复核形成过的全部新计划，逐份按计划版本读适用性册。恰一份当前有效（含没有登记行）就复核它；一份都没有就走 `reassessNoRoute`；不止一份、某份已被替代而接班的不在路由历史里、某份已结束，都是坏起点，答 `ErrInconsistentRouteStart`、响亮报错。重建不出来的在册计划也响亮报错，不折成未决。起点在 `LatestTriggerAt` 取的判断键锁里读。
- 已有结果分界挪进判断键锁里：取锁之后先按关联读复核册。指纹原先就在册的是重放，按原结论作答。指纹是本笔刚登的，说明并发里输给了赢家，答 `EXISTING_RESULT`，与提交时撞键同一格。这样同一触发的后到一方不会按赢家换过的起点再判一次、再写一次适用性。
- `reassessPlan` 遇 `ErrPlanNoLongerCurrent` 不再折成「登记册读不回」，响亮报错：起点在锁里读、只取当前有效的计划，重复触发也已在锁里读回，这个错误不再有正当来由，是编程错误。离场门的其余拒法也响亮报错，但不是编程错误一类，例如离场早于生效（见 code/60）。去掉 `loadApplicability`，适用性随起点交给失效与改善替代两处，不读第二次。
- 复核形成的计划改为从这次触发的业务时间起生效，判断时刻仍取处理时钟；改善替代时原计划的离场时刻也取触发业务时间。单一权威是 [ADR-0272](../../../docs/adr/0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)（评审 R1 后立）。
- 测试：
  - 编排 `internal/networkrouting/application/reassess_from_current_route_test.go`，钉以下各格：
    - 票面探针三步：失效之后限制仍在答 `PLAN_LAPSED/NO_QUALIFIED_CANDIDATES`，没有被复核计划，不答未决；限制撤掉形成首个计划、换新的计划版本，原计划仍 `LAPSED`；再下一次复核引用新计划。
    - 失效之后候选评估未决或最低成本并列，都仍无路由、不挂建议。
    - 失效之后评估补齐，形成新计划版本，从触发业务时间起生效，无路由期间那一行保留。
    - 失效线自动改路与改善替代形成的新计划都成为下一次复核的起点。
    - 坏起点响亮报错：替代的接班不在路由历史里、不止一份当前有效，都答 `ErrInconsistentRouteStart`；在册计划重建不出来答 `ErrStoredRoutePlanUnreadable`。
    - 同一业务时刻形成的两份计划，处理时钟回拨，起点仍是当前有效的那一份。
    - 积压之下（处理时钟比触发晚十小时）失效、形成首个计划、再失效、走无路由线，评审 R2 那条用例；生效改回判断时刻即红。
    - 读不出新计划答 `ROUTING_HISTORY_UNAVAILABLE`，读不出适用性答 `APPLICABILITY_STORE_UNAVAILABLE`。
    - 并发输家在锁里读回赢家，不写适用性。
    - 测试替身补了 `FormedPlans`（排法与真库同：业务时间，同一时刻按处理时刻）与锁时钩子。
  - 复核册真库 `route_reassessment_formed_plans_test.go`：按判断键全部读回，迁移 0022 之前的旧行也在（排在最前）；不带新计划的复核、别的判断键、别的租户都不掺进来；重建不出来的计划交 `ErrStoredRoutePlanUnreadable`。
  - 派发真库 `cmd/parcel-dispatch/reassess_after_lapse_test.go`，在 `reassessBothLanesFixture` 上依次投四次：
    - 场外揽收，限制淘汰选中候选，原计划失效；
    - 交进新加坡枢纽，限制仍在，入账一行 `PLAN_LAPSED`，没有被复核计划、没有失效依据、`NO_QUALIFIED_CANDIDATES`，inbox `PROCESSED`；
    - 限制撤掉再交进，`FIRST_PLAN_FORMED`，换新版本，`SYN-RPV-63` 仍 `LAPSED`；
    - 又一次交进，`STILL_APPLICABLE`，引用新计划。
  - 同一文件另一条派发真库用例：路由历史里有一份旧行（没有业务时间、已失效）与同一业务时刻的两份计划（先判的已失效、后判的当前有效，处理时刻倒序），交接复核的起点是当前有效的那一份。
  - 先跑红：把新测试放到 main `e8cd3de65` 的导出副本上跑，编排新用例 8 条全红；派发那条在第二次投递答 `REASSESS_STORE_UNAVAILABLE`，与探针一致。
- 文档：
  - UC-NR-003「给开发的交接」去掉「依赖 code/52」一行，已实现一句补上复核起点；`AT-NR-039`、`AT-NR-053` 的现状注改为已补，仍点名 demo-chain-gaps/115。
  - code/29 Status 行绕法里的依赖改为已补。
  - ADR-0249 Status 行索引注记的「落地依赖」改为「落地依赖已补」，ADR 正文没动。
  - 新立 ADR-0272，NR CONTEXT「路由计划适用性」首条补一句引它，UC-NR-003 `AT-NR-053` 改为引它。
  - `docs/adr/README.md` 索引未重生成，由协调方进 main 时重生成。
  - CONTEXT「此前处于无当前有效路由时……」一句与代码相符，未改。
- 装配、迁移、派发路由、未决哨兵名单、`internal/nodeoperations/**` 都没动。

### 代决

1. **起点按联查读，不加当前路由表。** 路由历史里的计划取自复核册与初始路由，状态取自适用性册。适用性册按全局唯一的计划版本取行，版本来自本租户的判断行，不构成跨租读路，口径同 `RoutePlanCatalogue`。
2. **起点按适用性认，不按先后取「最近一份」**（评审 N2 后改）。路由历史里每份计划各问一次适用性，恰一份当前有效即起点，一份都没有即无路由。初稿按触发业务时间取最近一份、同一时刻取处理时刻较晚的，同一业务时刻的平局就靠处理时钟分，时钟回拨即取错。协调方建议的修法是选中那份若已离场就顺接班走，但失效之后从无路由线形成的首个计划不记接班（复核册那一行没有被复核计划），顺链走不到它。所以改成逐份问。代价是每份计划一次适用性读，一件包裹通常不过三份。
3. **适用性没有登记行即当前有效。** 失效线改路与首个计划形成时不写行，改善替代写接班行；离场必写。所以凡已离场的计划都有一行，没有行的就是当前有效的那一份。
4. **路由历史自相矛盾，响亮报错，不折未决、不进哨兵名单。** 包括不止一份当前有效、某份已被替代而接班的不在路由历史里、某份已结束。照「当前有效」判是复核一份离了场的计划，照「无路由」判会凭空另形成一份计划。已结束今天没有写入方。在册计划重建不出来同样响亮报错（评审 N3）：坏数据重投推不动。
5. **已有结果分界挪进锁里，重放也取判断键锁。** 并发输家答 `EXISTING_RESULT`，与提交撞键同一格；重放仍答原结论。票面修法第 3 条的两种选法里取「串行之后改成响亮报错」。
6. **复核形成的计划从触发的业务时间起生效，被它替代的计划同刻离场。** 评审 R1 后立为 [ADR-0272](../../../docs/adr/0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)，理由与越权风险点以它为准，这里不复述。
7. **失效之后形成的计划仍答 `FIRST_PLAN_FORMED`、落「首个计划」那一形状。** 票面修法第 2 条已核过逐结论 CHECK。这里的「首个」按 CONTEXT 读作「无当前有效路由之后的首个当前有效计划」。

### 越权风险点

1. **「首个计划」不等于包裹史上第一份计划**（NR owner）。复核册里一件包裹可以先 `PLAN_LAPSED` 再 `FIRST_PLAN_FORMED`。只读这一行做审计时，要联看前面的失效行才知道它接的是哪份已失效的计划。owner 若要区分，加一格结论要一次迁移。
2. **新计划的生效时刻改用业务时间**（NR、TF、VE owner）。各点见 ADR-0272 越权风险点，含装载并发裁决接上线后的「改路先有效」误裁，与初始路由口径不一（票 code/60）。
3. **坏起点响亮报错，照样占着分区到 `ABANDONED`**（NR owner）。今天到不了：仍适用线的改善替代那一行落不了复核册（code/53），已结束没有写入方。code/53 落地之后，替代必带接班行，仍到不了。到了就是数据坏了，不是等租户登记。
4. **每次复核多读几次，重放也排在判断键锁上**（NR owner）。多出的是复核册一次与路由历史里每份计划各一次适用性；重放原先不取锁。同一判断键的触发本就串行，代价是重放等前一笔提交。
5. **复核之外的读方仍读初始路由那一行**（NR、TF owner）。本票只改复核取起点。运输准备与查阅面今天读到的「当前路由」仍是受理时那份计划，不知道它已失效或已被替换。这一缺口归 demo-chain-gaps/34，本票未动。
6. **演示库要重灌**（NR owner）。本票之前落过的演示数据里，失效之后的那次复核若停在未决、或对已失效的计划答过仍适用，复核册与适用性册会留着那一行。新起点按适用性认，不受它干扰，但查阅面会看到旧答法。`scripts/demo-seeds --reset` 重灌即清。

### AT

- `AT-NR-039`、`AT-NR-053`：AT 盘点由 ② 移入 ①。`AT-NR-039` 点名编排（失效之后那一格）与派发真库用例，`AT-NR-053` 点名编排用例，两行都注明生产接法上复核不取候选成本，归 demo-chain-gaps/115。总览计数未动，由协调方进 main 时重算。

### 验证（分支 claude-code52）

- `go build ./...`、`go vet ./...`、`gofmt -l`（本分支改动的全部 Go 文件）干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/architecture/... ./migrations/...`：21 个包 ok，退出码 0。
- 新真库用例 `TestAfterALapseTheNextTriggersReassessFromNoRouteOnTheDatabase`、`TestTheLatestNewPlanIsReadPerJudgmentKeyByBusinessTime` 另跑 `-count=3`，都 ok（后者在评审修订里换成 `TestFormedPlansAreReadPerJudgmentKey`，见下节）。
- 中途一次整包跑里 `cmd/parcel-dispatch` 的 `TestAClosedAgreementVersionIsTracedAndRecoveredByRevision`（TF 实际履约补形成，不经 NR 复核）红过一次。单跑 `-count=3` 绿，整包重跑绿，判为与本票无关的偶发，交协调方留意。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审，协调方转达）

无阻断。三条合入前修（R1–R3），四条建议（N1–N4），逐条处置如下：

| 条 | 意见 | 处置 |
|---|---|---|
| R1 | 「复核计划从触发业务时间起生效」只写在票面代决与 UC 里，没有单一权威 | 立 [ADR-0272](../../../docs/adr/0272-a-plan-formed-by-route-reassessment-takes-effect-at-the-trigger-business-time.md)。核过 ADR-0172、0173、0174、0215、0217、0235、0249，没有一句定过复核形成的计划取哪个时钟生效，无条款停用，不挂指针。越权风险点含装载并发裁决接上线后的误裁（待 NR、TF、VE owner 会签）与初始路由口径不一。NR CONTEXT「路由计划适用性」首条补一句引它，UC-NR-003 `AT-NR-053` 改引它，代码注释改引它，代决 6 改为指向它 |
| R2 | 积压场景用例（失效 → 首个计划 → 处理时钟 +10h 下再失效 → 无路由）正式入册 | 加进 `reassess_from_current_route_test.go`，名 `TestUnderABacklogAFormedPlanLapsesAtItsTriggersBusinessTime`。把生效改回 `judgedAt` 时它与 `AT-NR-053` 那条一起红，已复验 |
| R3 | 两处注释与代码不符 | `startingPoint` 头注与代决 3 改为「失效线改路与首个计划形成时不写行，改善替代写接班行；离场必写」；`reassessPlan` 里「编程错误」收窄到 `ErrPlanNoLongerCurrent` |
| N1 | 改善替代取触发业务时间而初始计划按处理时钟生效，新出一种响亮失败，失效线同病 | 另立 [code/60](./60-initial-route-takes-effect-at-processing-time-unlike-reassessment.md)，建议 R2，注明今天走不到（依赖 code/53、demo-chain-gaps/115） |
| N2 | 同一时刻的平局靠应用时钟决定 | 改法比建议多走一步，理由见代决 2：读口改为交回全部新计划，起点逐份问适用性，不再取「最近一份」，平局与时钟无关；不加迁移。补编排平局用例、多份当前有效用例，与派发真库平局用例（处理时刻倒序、同一业务时刻，外加一行没有业务时间的旧行）；复核册真库用例覆盖旧行读回。头注那句「后判的那一次读过先判的结果」随读口一起删去 |
| N3 | `rebuildPlan` 失败不该折成 `RoutingHistoryUnavailable` | 适配器包 `ports.ErrStoredRoutePlanUnreadable`，编排遇它响亮报错；编排与复核册真库各补一条 |
| N4 | 坏起点用例改问错误身份；两处适配器替身补同一时刻规则；「三本册」点名；越权风险点补演示库重灌 | 用例改名 `TestASupersededPlanWhoseSuccessorIsMissingIsLoud`，改用 `errors.Is(err, application.ErrInconsistentRouteStart)`；两处替身随读口改成 `FormedPlans`，排法与真库同；头注点名 `initial_route`、`route_reassessment`、`plan_applicability`；越权风险点 6 补 `--reset` |

修订后已 `git rebase main`（`4e7874ba0`），无冲突。

验证（修订后）：

- `go build ./...`、`go vet ./...`、`gofmt -l`（相对 main 改动的 Go 文件）干净。
- 同一组包 `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/architecture/... ./migrations/...`：21 个包 ok。
- 新用例另跑 `-count=3`：派发真库两条、复核册真库两条、编排新增四条，都 ok。
- `tools/adr-index`：在 scratchpad 的 `git archive HEAD` 副本上重生成之后 `-check` 退 0，ADR-0272 进索引，标〔代决·待 NR owner 复核〕。本分支的 `docs/adr/README.md` 未动。

