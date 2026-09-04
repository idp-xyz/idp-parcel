# demo 里自动改路那条路走不到，而装配点看着像活的

Category: enhancement
Status: resolved——MCP-5（2026-09-04，task-f9e0bd40）：推进表已逐行重核（见「复核结论」，核于 main `6f9436f3`），依赖链全部已落；剩下的不是依赖而是一条要跑通的运行时路径，拆到 [02](./02-syn-vertical-run-reaches-reroute-after-lapse.md)（ready-for-agent）。本票不再承接实现
Blocked by: 无（原「见 `synthetic-vertical-closure/design.md` 那条链」已由下文复核收掉）

## 症状

隔离演示环境里 `ReassessRouteHandler` 永远走不到自动改路那一支：事实目录恒答未配置，
于是「失效照常落库、改路评估整段不做」，`rerouteAfterLapse` 的三态分派（自动改路 /
建议 / 禁行）一次也执行不到。

**而这在装配点上读不出来。** `cmd/parcel-dispatch/assemble.go` 已经把
`ReassessRouteDeps.AutoReroute` 从 `nil` 换成真适配器（`NewAutoRerouteFactsCatalog`），
读那一行只看得见「已经不是 nil 了」。这是本仓反复记的那一族：**已接线看着像可用。**

## 成因不是缺种子

立票 [`admin-write-faces/05`](../../admin-write-faces/issues/05-auto-reroute-facts-has-no-registration-entry.md)
时先把它当成「seed 加一行」，实现时才核出来建不了。取证：

- 读口按判断键取行——`reassess_route.go` 调 `LoadAutoRerouteFacts(ctx, trigger.Key())`。
- 那个键是 `InitialRouteJudgmentKey` 六维，后四维（委托请求、接受基线版本、申报包裹、
  服务目的）是**运行时产物**：委托提交 → 接受决定形成基线 → 逐包裹派生判断范围。
- 而 `seed.sh` 灌的全是配置类主数据。全仓种子数据里 `shipment_request` /
  `declared_parcel` / `acceptance_baseline` 一个都不出现（代收那份 `SYN-PARCEL-COD-01`
  是代收册自己发明的引用，不是路由过的真包裹）。

**所以静态种子只能造一个永远命不中的键，而那比空表更坏**：表非空了，目录看起来已配置，
每次真实复核仍拿到 `configured=false`——「未配置」与「配置了但不是这个键」在读口答案上
从此同形。空表至少如实说「这个判断键从未登记过事实」。

**这张票要的是一条运行时路径**，不是一行数据：委托提交 → 接受 → 初始路由 → 触发复核，
然后在那一刻按真实键登记事实。登记入口已经有了（`parcel-network-register -kind
auto-reroute-facts`，`admin-write-faces/05` 交付），缺的是把键喂给它的那条路。

`seed.sh` 不是这条路的落点——它是主数据灌入脚本，不是流程驱动器。

## 开工前先复核依赖

`.scratch/synthetic-vertical-closure/design.md` 的推进表里有一行 `CONS-INTAKE-REASSESS`
（「采用信封已有消费者；接 CONS-INTAKE 后跑第二跳」），它记的依赖是 `CONS-INTAKE`，而
`CONS-INTAKE` 又记着依赖 `PS-PARCEL-INDEX`（按包裹反查来源身份）。

**但那张表不能直接采信。** 同一行的「期望业务结果」写着「复核行落库；`AutoReroute`
仍 nil」——这句在 `syn-wall-door-audit/05` 交付四件并把 `assemble.go` 的 nil 换成真适配器
之后就不成立了，而写它的人没回来改。一行已知过期，其余行的现状同样得重取证再用。

所以：**认领本票的第一步是回到那张表逐行重核，而不是照它排期。** 复核完把结论写回本票，
再决定要不要拆实现票。

## 复核结论（2026-09-04，通道 5，task-f9e0bd40；核于 main `6f9436f3`，只读 `main` 树，不含任何会话的在途改动）

`.scratch/synthetic-vertical-closure/design.md` §4 推进表逐行对今天的代码。表本身已由通道 2 于同日加了「止于 `1fff679`，此后不维护」的头注，下表是它最后一次被逐行取证；**数字与「在/不在」只作此刻取证，不作别人的基准。**

| 行 | 表里写的「内容 / 依赖 / 验绿」 | 今天在哪（文件 + 符号） | 生产接线 | 结论 |
|---|---|---|---|---|
| SYN-V0 | 提交 → 决定 → 真 Outbox → 真 Dispatcher → NR 消费门 → 诚实未决；依赖无 | `cmd/parcel-dispatch/synthetic_v0_test.go`：`newSYNVerticalFixture`、`TestSYNIncompleteJudgmentsStayUndecidedWithoutAnAcceptanceEnvelope`、`TestSYNAcceptedDecisionStopsAtUnconfiguredRouteEvidence`（停在 `ROUTE_EVIDENCE_NOT_CONFIGURED`） | 进程测试，经生产 `wireDispatcher` | **已落** |
| SYN-PC-SEED | SYN 商业版本种子走真登记解析，替换替身；依赖 V0 | `cmd/parcel-dispatch/syn_pc_seed_test.go`：`seedSYNPCEligibility`（真 PC 登记册种下资格声明，`assertSYNPCEligibilitySeeded` 证 `configured=true`）；价卡/序列种子在 `scripts/demo-seeds/seedgen` | 测试装配 + demo 种子 | **已落**（资格声明列了硬资格，采用因而停在 `NOT_ESTABLISHED`——那是有意的诚实停点，不是缺口） |
| PS-PARCEL-INDEX | 按包裹反查来源身份 + 委托号；依赖无 | `ports.CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel`，`pspostgres.ShipmentRequests` 实现，迁移 `parcel_shipment/0006_current_accepted_parcel_projection.sql` | `cmd/parcel-dispatch/assemble.go` 的采用消费者装配传 `NewShipmentRequests(db)` 作目标反查 | **已落** |
| VE-ACCOUNT-INDEX | 投影按包裹反查客户账户；依赖无 | `veports.ParcelCustomerAccountView.FindCustomerAccount`，`internal/visibilityexception/adapters/parcelshipment/customer_account_lookup.go` 的 `ParcelCustomerAccountLookup` | `assemble.go` 的 `deriveCustomerViewConsumer` / `deriveCustomerViewOnAcceptanceConsumer` | **已落** |
| NR-APPLICABILITY-S | SYN 判断键→解析标识映射（测试装配），过适用性关停在网络定义未配置；依赖 SYN-PC-SEED | SYN-V0 那条已走到 `RouteEvidenceNotConfigured`（`CreateInitialRouteHandler.Handle` 经 `InitialRouteEvidenceView.LoadInitialRouteEvidence` 答未配置）；生产 `assemble.go` 不带任何 SYN 映射 | 测试装配 | **已落**（第二道哨兵就是它） |
| NR-RESOLVER | `PAR-NET-14` 解析层机制；有定义时交出候选或显式未配置 | 网络定义登记册（迁移 `network_routing/0007_network_definition.sql`、`0008_network_catalog.sql`）与 `cmd/parcel-network-register` 的目录登记口在；`CreateInitialRouteHandler` 只认 `InitialRouteEvidenceView` 一口 | 生产 | **机制在、实例留空**：实例（`PAR-NET-14`）无处登记属产品基线的事；「有定义时交出九族」的形状本复核未逐字段核，不在此担保 |
| CONS-INTAKE | `node-intake.formed` → PS 采认消费者 + 路由表条目；依赖 PS-PARCEL-INDEX | `psinbox.NodeIntakeConsumer`（`NodeIntakeFormedEventType = node-operations.node-intake.formed`） | `assemble.go` 路由表 `psinbox.NodeIntakeFormedEventType: nodeIntakeFan`（`adoptNodeIntakeConsumer`） | **已落** |
| CONS-DELIVERY | `effective-delivery.registered` → 终局；依赖 PS-PARCEL-INDEX | `psinbox.EffectiveDeliveryConsumer`（`EffectiveDeliveryRegisteredEventType`） | 路由表 `psinbox.EffectiveDeliveryRegisteredEventType: deliveryFan`（`adoptEffectiveDeliveryConsumer`） | **已落** |
| CONS-INTAKE-REASSESS | 采用信封的消费者，接 CONS-INTAKE 后跑第二跳；表写「`AutoReroute` 仍 nil」 | `nrinbox.NetworkIntakeConsumer`（`AdoptedNetworkIntakeEventType = parcel-shipment.network-intake.recorded`）→ `nrparcelshipment.NewReassessOnNetworkIntakeAdapter` → `ReassessRouteHandler`；`ReassessRouteDeps.AutoReroute` 接 `nrpostgres.NewAutoRerouteFactsCatalog(db)`（迁移 `network_routing/0009_auto_reroute_facts.sql`） | 路由表 `nrinbox.AdoptedNetworkIntakeEventType: routedIntakes`（`networkIntakeConsumer`） | **已落**；表里「仍 nil」那句过期（票面「症状」节已指出） |
| CONS-ROUTE-NO | `initial-route.formed` → NO；先要 NO 的「接收路由指令」UC | `internal/nodeoperations/adapters` 下只有 http / identity / postgres / transportfulfillment，无 inbox；路由表里 `InitialRouteFormedEventType` 只投给 VE 投影（`veinbox`） | 无 | **未开始**（表写「不可抢跑」，今天仍然如此） |
| CONS-EVAL-SA | `evaluation.recorded` → SA | `internal/settlementaccounting/adapters` 下只有 http / partycommercial / postgres，无 inbox；`saports` 里只在注释提到 `parcel-pricing.evaluation.recorded` | 无 | **未开始** |
| HTTP-SYN-INTAKE | 测试专用 Intake，永远不进 `assembleBusinessEndpoints` | `cmd/parcel-api/endpoints.go` 全部端点挂字面量 `UnconfiguredIntake{}`，无 syntest 变体 | 无 | **未建，且按表就不该建进生产** |

**对本票「症状」的重核**：`ReassessRouteHandler.rerouteAfterLapse` 只在 `PlanReview` 判 `PlanNoLongerApplicable` 且 `reviewCandidates` 答 `CandidatesAvailable` 之后才会被调，进去之后再按 `trigger.Key()`（`InitialRouteJudgmentKey` 六维）问 `AutoRerouteFactsView.LoadAutoRerouteFacts`。今天能让它读到 `configured=true` 的只有两处：`internal/networkrouting/application/reroute_after_lapse_test.go` 的替身，与 `internal/networkrouting/adapters/postgres/auto_reroute_facts_test.go` 用合成键（`autoRerouteKey`）直登的真库用例。**没有任何一条路把真实接受形成的键喂进 `parcel-network-register -kind auto-reroute-facts` 再触发复核**——症状成立。

**`unresolved-review-20260904/report.md` A 组留的那一问（「六维从哪个读面取出来」）今天有答案**：路由计划读面 `nrports.RoutePlanRow`（`internal/networkrouting/ports/route_plan_read.go`）与 `query_route_plans.go` 的载荷已透出 `customerAccountId` / `shipmentRequestId` / `acceptanceBaseline` / `declaredParcelId` / `servicePurpose`，加作用域里的租户，六维齐了。不必加读面，也不必让 CLI 由「委托号 + 包裹号」反解。

**所以剩下的不是依赖，是一条要跑通的运行时路径**：网络定义（SYN，S 级）登进去让初始路由成立 → 采用成立让复核触发 → 定义换版让计划失效且候选可用 → 从路由计划读面取键登事实 → 再触发一次复核走到 `rerouteAfterLapse`。每一步的取值都属实例半边、只记 S；没有一步要改生产装配。拆到 [02](./02-syn-vertical-run-reaches-reroute-after-lapse.md)。

**能力边界**：读了 `assemble.go` 路由表与各消费者装配函数、`reassess_route.go`、`create_initial_route.go` 的未决格、NR ports 与路由计划读面、NO/SA 适配器目录、`cmd/parcel-dispatch` 全部测试函数名与 SYN 夹具；**未读** `PlanReview` 判失效的具体依据（哪些证据变化算失效——02 开工第一步要读）、`reviewCandidates` 的候选来源、`parcel-network-register` 七族登记口各自的载荷。

## 红线

- 不得为了让路走通而在 `seed.sh` 里塞一行事实——理由见上，那正是本票要避免的形状。
- 阈值与条件取值全属实例半边（`PAR-NET-14` 待提供）；跑通用的四条件只记 `S`。
- 不改 `cmd/parcel-dispatch/assemble.go` 里已经接好的那一行；本票缺的是上游，不是装配。

## Comments

- 2026-09-03 · MCP-4：立票。发现于 `admin-write-faces/05` 的实现过程——那张票原本写着
  「seed 加一份合成条件」，核不过去，遂撤销该项并把成因分出来独立成票。本票只写票面，
  未动任何代码，也未碰 `synthetic-vertical-closure/design.md`（不在本会话地盘；那行过期
  的话在此指出，由该目录的持有者决定改不改）。
- 2026-09-04 · 通道 5（task-f9e0bd40）：按票面第一步逐行重核推进表（见「复核结论」），依赖
  链全部已落，A 组报告留的「六维从哪读」一问由路由计划读面答了；实现拆到 02（ready-for-agent），
  本票转 resolved。只写票面，未动代码，未碰 `design.md`（其头注已由通道 2 同日加了「止于
  `1fff679`，此后不维护」）。
