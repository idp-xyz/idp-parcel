# 67 NR 复核里候选成本读失败会让收件箱事务中毒：失效与「仍适用」一并回滚，与「候选评估未决不拦失效落库」相抵

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code67（2026-10-21 作者子代理，见文末「完成记录」）。此前 ready-for-agent——建议 R2（上线后尽快）：失败是响亮的（整封回滚重投，最终 `ABANDONED`），错在把一次可降级的读失败扩成整笔失败，并误报成写库故障。2026-10-21 协调方裁定立（票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 隔离评审建议 2；用户「你作为业务和系统专家自决……解决所有的问题」授权），115 作者子代理执笔，分支 claude-dcg115。
Blocked by: 无
出处：demo-chain-gaps/115 隔离评审建议 2。

## 现象

复核编排在三条线上对候选成本「读不回」各有处置：无路由线答未决、回滚重投；失效线记候选评估未决、失效照落；仍适用线不试改善、保留原计划。后两格只在读失败不碰事务时成立。

复核跑在收件箱消费门的那一笔事务里，取数侧经 parcel-pricing 与 NR 目录的仓储读库，读口在事务里时走同一个事务（`bentopg.DB.ReadExecutor` 有事务绑定即返回事务执行器）。PostgreSQL 上一条语句失败，整笔事务即进入中止态，此后的语句都被拒。于是：

- 失效线：适用性已写、成本读失败，随后落复核册被拒，编排答 `REASSESS_STORE_UNAVAILABLE` 未决，整封回滚重投；失效没有落库，与 CONTEXT「失效即失效：候选评估未决拦不住失效落库」相抵。
- 仍适用线：同样在落复核册时失败，原计划仍适用这个已判的结论也一并丢掉。
- 两格的未决原因都报成写库故障，运维会去查复核册而不是 parcel-pricing。

只有不碰事务的读不回（取数侧的翻译类错误，如认不出的评价形状）才真落进「候选评估未决、入账」与「保留原计划」。

## 证据（读码，钉 `378b9b2dc`，证据 `S`）

- `internal/networkrouting/application/reassess_route.go`：`reviewCandidates`、`tryImprovementReroute` 遇 `candidateCosts` 答读不回时分别记候选评估未决、交回原记录，随后 `commit` 落册；`commit` 遇写库错误答 `ReassessStoreUnavailable`。
- `cmd/parcel-dispatch/assemble.go` 的 `routeCosts`：目录、价卡、内部政策、包裹事实各口都是 `bentopg.DB` 上的仓储。
- `go.idp.xyz/idp-bento-go/postgres` 的 `DB.ReadExecutor`：上下文带事务绑定时返回事务执行器。

## 三类

机制。

## 规模

S–M：

- 在复核编排里给取成本包一层保存点（savepoint），读失败时回到保存点，事务照常可用。保存点要由平台的事务口提供，不在 NR 里直接调 `pgx`。
- 平台若还没有保存点口，先在 `internal/platform` 或 bento 侧加；领域与应用层不依赖 `pgx`。
- 改完之后，读失败照原先写定的分格落：失效线候选评估未决、失效照落；仍适用线保留原计划。

## 最小的诚实修法

1. 事务口加「在保存点里执行」的能力，失败即回到保存点、把错误交回。
2. 复核编排的 `candidateCosts` 经这一口调取数侧。
3. 真库用例：失效线上让取数侧的一次库读失败（例如在用例独占的库上撤掉一张价卡表的读权限），失效照落、候选评估未决、信封入账。

## 相关

- 初始路由取成本也在同一笔事务里读库。那里读不回本来就答未决；一份交接带几件包裹时，一件读失败会不会让同一笔里别的包裹的落库一并失败，本票没核，修时一并看。
- 同族的确定性错误落进重投桶，见 demo-chain-gaps/155 的 2026-10-21 备注。

## 完成记录（2026-10-21，作者子代理，分支 `claude-code67`，基于 main `0a7937cd5`）

### 做了什么

- 端口 `ports.RouteCandidateCostSource` 写明契约：读失败不得连累调用方的事务，生产装配给取数侧包 savepoint，编排只据错误落格、不碰事务。
- NR postgres 适配器新增 `SavepointedRouteCandidateCosts`（`internal/networkrouting/adapters/postgres/route_candidate_costs_savepoint.go`）：在调用方事务里开 savepoint，内层失败退回、错误原样交回，成功即 RELEASE；ctx 不在事务里时直通。退不回 savepoint 时两条错误一并交回。
- 派发装配：`reassessRouteHandlerOver` 给交进来的取数侧包这一层（生产与真库夹具走同一个 Over），`acceptanceConsumer` 给初始路由的 `routeCosts` 包这一层。`assemble.go` 改的是 `acceptanceConsumer` 里一处，它本就在那个文件里。
- 复核编排 `reviewCandidates`、`tryImprovementReroute` 两处注释改为现行规则（库里读失败也落候选评估未决 / 保留原计划），去掉指向本票的现状注。编排代码没改：落格本就写对了，缺的只是事务那一层。
- 文档：UC-NR-003 结果表「替代候选评估未决」「复核未决」两格的现状注改为已补，「给开发的交接」删去本票那一行、在已实现一句里补上；AT 盘点 `AT-NR-053` 行内补点名并追记（计数未动）；[code/78](./78-initial-route-dependency-undecided-drags-formed-siblings.md) 记备注：成本那一格已隔离，其余依赖读口没有。NR CONTEXT 没动：硬句本身没错，这次是让实现兑现它。

### 接票时核实

- bento `idp-bento-go@v0.1.0-rc.2` 没有 savepoint 口：`WithinTransaction` 嵌套时沿用外层事务，内层回调报错即把整笔标成只回滚（`runNested`），没有部分回滚的语义。所以保存点落在 NR 适配器里，经 `DB.RequireExecutor` 的执行器发 `SAVEPOINT`/`ROLLBACK TO SAVEPOINT`/`RELEASE SAVEPOINT`，不改外部仓库。仓里已有同形先例：NO 的 `SavepointedServiceResultView`、`SavepointedParcelIdentityView`（ADR-0186 决定七、ADR-0199 决定五）。
- 票面「相关」一节要核的初始路由：成本读不回本就答 `COST_SOURCE_UNAVAILABLE`、整封回滚重投，所以库里读失败不改提交结果；但取成本之后编排还要重读一次证据做提交前重校（`routeOneParcel`），事务中毒时那一读跟着失败，这一件的未决原因被报成 `ROUTE_EVIDENCE_UNAVAILABLE`，运维会去查网络目录。用例先红即为此（见下）。同一交接里排在它后面的包裹在中毒的事务里也各自读不动，但整封回滚、只报第一件未决的原因，报出的那一件就是读失败的那一件。

### 代决

用户原话「你作为业务和系统专家自决……解决所有的问题」。

1. **保存点做成取数侧的装饰器，不做成应用层的「在保存点里执行」端口。** 票面「最小的诚实修法」写的是事务口加能力、编排经这一口调取数侧。改用装饰器的理由：仓里同一种问题已有先例（ADR-0186 决定七），一种做法一处定义；编排不必知道事务边界，应用层也就不多一个只为 postgres 存在的依赖；隔离的承诺写在取数侧端口的契约上，装饰器兑现、装配点接上。代价是隔离靠装配：哪天有人新接一条复核或初始路由的装配而不包这一层，编排察觉不到。复核的真库夹具与生产共用 `reassessRouteHandlerOver`，生产装配那一半另由挪表用例钉住。
2. **初始路由一并包。** 票面只要复核两条线。初始路由那一件的提交结果不变，改的是未决原因：不包时报网络证据读不动，包了报成本读不回。按 ADR-0029「恢复动作不同就分格」的口径，运维据原因选的恢复动作（查 parcel-pricing 还是查网络目录）不同，这一处报错就是错的。code/78 修法 (a) 落地时也要这一层。
3. **包在 `reassessRouteHandlerOver` 与 `acceptanceConsumer` 两处，不包在 `routeCosts` 里。** 复核的真库夹具经 `reassessRouteHandlerOver` 交替身，包在那里，替身与生产走同一层隔离、用例钉的是装配本身；包在 `routeCosts` 里，替身那几条用例就证不到装配。两处各一行，决定本身只在端口契约一处写。
4. **用例里的读失败两种都用。** 夹具替身在 inbox 事务里发 `SELECT (1/0)`，钉失效与仍适用两条线；生产装配下挪开 `parcel_pricing.price_card_version`（用例独占的库，用完挪回），钉复核失效线与初始路由。票面建议撤读权限，测试库的角色是超级用户、权限检查不生效，所以改挪表；「表不存在」同样是语句级失败、同样让事务中毒。
5. **`AT-NR-053` 补点名，不新立 AT 号。** 挪表那条用例走的正是「原计划失效，候选评估未决后补齐并找到合格候选」：表读不动时失效、候选评估未决，表挪回之后下一次交接形成首个计划。其余几条是机制半边，没有对得上的 AT 行，不为它们新立号。

### 越权风险点

1. **隔离靠装配兑现**（NR owner）。见代决 1。端口注释写了契约，编排不校验；新接的装配若不包，库里读失败照旧让失效回滚、报成写库故障。今天全仓只有两处装配接取数侧，都包了。
2. **取号故障同族未修**（NR owner）。失效线自动改路与仍适用线改善替代遇取号故障也是「保留原记录、照常落册」，取号走 NR 自己的序列，`nextval` 在库里失败同样会让事务中毒。没有包：序列读失败实际上只出在连接级故障上，那时 savepoint 本身也发不出去，包了换不来什么；若 owner 认为语句超时一类也要防，照同一做法给 `RouteIdentities` 包一层。
3. **savepoint 只隔开 SQL 级失败，不隔开 bento 的只回滚标记**（NR owner）。取数侧内层若哪天调了嵌套 `WithinTransaction` 并报错，bento 把整笔标成只回滚，提交时一样失败，savepoint 救不回。今天取数侧各口都只读、没有嵌套事务（读码核过 parcel-pricing 应用层与 NR 取数侧适配器）。
4. **其余依赖读口仍直接走判断事务**（NR owner）。证据、判断库、商业适用性、交接登记读失败照旧中毒；这几格本来就答未决、整封回滚，结果不变，只是同一交接里后面几件的原因可能失真（只报第一件，不对外）。code/78 落地时要逐口重看，已记在那张票的备注里。
5. **每次取成本多三条语句**（NR owner）。`SAVEPOINT`、`RELEASE` 各一次往返，初始路由逐件、复核每次排序各一组。比起取数侧逐段问 parcel-pricing 的那些读，开销可以忽略；没有量过。

### 用例

先红：四条派发用例在加装饰器之前跑红，原因与票面现象一致——
- 两条替身用例：投递答 `REASSESS_STORE_UNAVAILABLE` 未决，失效与仍适用都没落。
- 生产装配挪表那条：交接信封 `PENDING`、失败码 `dispatch.consumer_undecided`。
- 初始路由那条：未决原因 `ROUTE_EVIDENCE_UNAVAILABLE`。

加了装饰器之后全绿；变异核对：把装饰器改成事务里也直通，四条派发用例与适配器的读失败用例全红，改回后绿。

- NR 适配器真库 `internal/networkrouting/adapters/postgres/route_candidate_costs_savepoint_test.go`：
  - `TestASavepointKeepsAFailedCostReadFromAbortingTheJudgment`：内层 `SELECT 1/0`，错误原样交回，同一事务随后写交接登记照常提交、读得回。
  - `TestASavepointReleasesReadableCostsUnchanged`：可读一支原样交回、照常提交。
  - `TestASavepointPassesCostsThroughOutsideATransaction`：事务外直通；没接的哨兵在事务里穿得过这一层。
- 派发真库 `cmd/parcel-dispatch/reassess_cost_read_isolation_test.go`：
  - `TestALapseLandsWhenTheCandidateCostReadFailsOnTheDatabase`：两腿夹具，交进吉隆坡枢纽，取数侧替身在 inbox 事务里读库失败；复核册一行`已失效`、候选评估未决、无建议无新计划，适用性册原计划已失效于触发业务时间，inbox `PROCESSED`。
  - `TestAStillApplicablePlanIsKeptWhenTheCandidateCostReadFailsOnTheDatabase`：声明自动改路、阈值 1000，交进新加坡枢纽；复核册一行`仍适用`、复核原计划、不改路，原计划没被替代，inbox `PROCESSED`。
  - `TestALapseLandsThroughTheProductionRouteWhenPriceCardsAreUnreadable`（`AT-NR-053`）：演示网络、生产路由表一拍，价卡版本表挪开时失效照落、候选评估未决、信封 `PUBLISHED`；挪回之后交进新加坡枢纽形成首个计划。
  - `TestAnInitialRouteCostReadFailureIsReportedAsTheCostSource`：生产装配的接受决定消费者，价卡版本表挪开，投递答 `ErrRouteHandoffUndecided`、原因 `COST_SOURCE_UNAVAILABLE`。
- 既有夹具改动：派发替身 `reassessCostsDouble` 加 `read`（在调用方 ctx 上先跑一读）与 `asked`；其余用例不设 `read`，行为不变。

### 验证

- `go build ./...`、`go vet ./...` 干净；`gofmt -l cmd internal migrations tools` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch/ ./cmd/parcel-api/ ./cmd/parcel-network-register/ ./internal/parcelshipment/adapters/networkrouting/... ./internal/transportfulfillment/adapters/networkrouting/... ./internal/visibilityexception/adapters/networkrouting/... ./internal/architecture/... ./migrations/...`：21 个包 ok、0 FAIL（含生产接线与类型可达两道棘轮）。
- 新用例另跑 `-count=3`（适配器三条、派发四条，连同编排 `TestAnUnreadableCostSourceSettlesPerLine`）：全绿。
- 没有迁移、没有种子改动；没跑全量；没重生成机制清点、ADR 索引与 AT 计数。

### 未决事项

1. 越权风险点 2（取号同族）要不要一并包，归 NR owner。
2. code/78 修法 (a) 落地时，其余依赖读口的隔离要另定（已记在 code/78 备注）。


## 评审记录（2026-10-21，协调方隔离评审：无阻断，三条变异都被抓住；作者子代理同分支修订）

逐条去向：

1. **`TestAnInitialRouteCostReadFailureIsReportedAsTheCostSource` 头注「与上一条」是位置引用**——已修：改为点名 `TestALapseLandsThroughTheProductionRouteWhenPriceCardsAreUnreadable`。
2. **适配器层缺「同一事务里先后几次」的用例**——已补 `TestSuccessiveCostReadsInOneTransactionEachRecover`：同一只装饰器在一笔事务里依次失败、失败、成功，两次失败各自交回错误，第三次读数照常交回，随后写交接登记、提交成功、读得回。初始路由逐件包裹、复核每次排序都重用同一个 savepoint 名，这条钉的就是这种重用。替身 `sqlCostSource` 加 `sequence`，其余用例不设，行为不变。
3. **两处超长注释**——已折行：`TestALapseLandsThroughTheProductionRouteWhenPriceCardsAreUnreadable` 与 `TestAnInitialRouteCostReadFailureIsReportedAsTheCostSource` 的头注。
4. **savepoint 装饰器三份同形副本**——已立 [docs-consistency/code/81](./81-savepoint-decorators-are-three-copies-of-one-helper.md)（R3，立前 ls 主树与本分支、看过在途分支，80 未占）：抽 `internal/platform` 泛型助手，NR 一份与 NO 两份改成薄封装。票面另记可选项：加一道 architecture 门禁，钉住「候选成本取数侧的装配必须包隔离」，即本票越权风险点 1。

**未采纳**：无。

### 验证（修订后）

- `go build ./...`、`go vet ./internal/networkrouting/... ./cmd/parcel-dispatch/` 干净；`gofmt -l cmd internal` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch/`：14 个包 ok、0 FAIL。
- 新用例与改过头注的两条另跑 `-count=3`：ok。
