# 60 NR 初始路由计划按处理时钟生效，复核形成的计划按触发业务时间生效：口径不一，原计划离场会被离场门拒

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code60。R1（上线前必修；2026-10-21 由 R2 升级，协调方据 code/53 隔离评审裁定）。升级理由：code/53 之后，改善替代一路落得了复核册，触发面从「硬约束失效 + 初始路由交接积压」放宽到「任一更便宜候选 + 初始路由交接积压」，声明了自动改路的租户在可控节点上的每次触发都可能走到离场门；失败时整封信封到 `ABANDONED`、占住分区；没有绕法；规模 S。code/53 的上线闸门不改挂到本票：离场门不专属自动改路，挂在闸门上挡错了对象。此前建议 R2（上线后尽快）：响亮失败；首发链路上通常撞不上，纯失效一路积压、离线补登时撞得上。初始路由计划的生效时刻取 NR 处理初始路由交接那一刻的处理时钟；ADR-0272 起，复核形成的计划从所判触发的业务时间起生效，被替代的计划也在那一刻离场。原计划经复核离场（改善替代、硬依据失效）时，若触发的业务时间早于初始路由的处理时刻，领域离场门拒（`leaveCurrent` 头注「离场必须带依据与不早于生效的时刻」），整封信封响亮失败、占着分区重投到 `ABANDONED`。改善替代一路写得进复核册已由 code/53 补齐（2026-10-21），失效线上自动改路复核取得到候选成本已由 demo-chain-gaps/115 补齐；纯失效一路今天就在：首发链路上受理与首个控制触发之间通常隔几小时，通常撞不上；初始路由交接积压、离线补登时撞得上。绕法：无。2026-10-21 协调方据 code/52 隔离评审 N1 立，作者会话起草（用户「你作为业务和系统专家自决」授权）。
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

- docs-consistency/code/53：改善替代那一行落复核册，已补（2026-10-21）；改善替代一路由此走得到这里。
- demo-chain-gaps/115：复核不取候选成本，失效线自动改路今天形成不了。
- docs-consistency/code/52：复核起点改读当前有效计划，复核形成的计划成为下一次起点；ADR-0272 随它立。

## 并入（2026-10-21，协调方，出自 code/52 第 2 轮修订）

- 初始路由编排 `CreateInitialRouteHandler`（UC-NR-001）遇 `InitialRoutes.FindByKey` 出错仍一律答 `ROUTE_STORE_UNAVAILABLE`、回滚重投；code/52 之后复核一路对同一类坏行（`ports.ErrStoredRouteRecordUnreadable`）响亮报错。本票落地时一并让初始路由编排 `errors.Is` 这一哨兵后响亮报错，其余读不回照旧答未决，补一条用例。

## 完成记录（2026-10-21，作者会话，分支 claude-code60，基于 main `c89830c72`）

### 做了什么

- 初始路由编排 `planRecord`：计划的生效时刻取路由交接带来的接受时刻 `RouteHandoff.AcceptedAt()`，判断时刻仍取处理时钟。`routeOneParcel`、`judgeParcel` 把接受时刻往下传，不另读。
- 并入一项：`routeOneParcel` 判断之前按键找、`commit` 里并发败方读回赢家，两处遇 `ports.ErrStoredRouteRecordUnreadable` 都上抛，其余读不回照旧答 `ROUTE_STORE_UNAVAILABLE`。这个哨兵不进派发的未决哨兵名单：坏行是确定性错误，重投推不动，进名单会把它记成可等的未决。
- 派发两路复核夹具 `newReassessBothLanesFixture` 拆出 `reassessBothLanesOver`：只接两路、不落计划，路由历史由用例自己备。原有用例的行为不变。
- 文档：
  - NR CONTEXT「路由计划适用性」首条：生效边界的时间取业务时间，判断时刻另记处理时钟；初始路由计划从委托接受的时刻起生效，复核形成的计划照 ADR-0272。
  - UC-NR-001「计划内容与责任边界」的「计划版本、判断时间、生效边界」一条引上面那条 CONTEXT；验收表立 `AT-NR-086`。
  - ADR-0272 正文没动，Status 索引注记加「越权风险点 2 已答」一段。
  - UC-NR-003 没动：它没有讲初始路由生效时刻的句子；复核一侧的生效与离场已由 ADR-0272 写进 `AT-NR-053`、`AT-NR-085`。
  - 指向本票的现状缺口注只有 ADR-0272 正文（Context、Consequences、越权风险点 2）与已 resolved 的 code/52、code/53 票面：ADR 正文按约不改，已收口的票面是历史，都没改。
- 不加迁移：计划本体存在 jsonb 里，生效时刻随计划落库。存量不回填：已在册的初始计划生效时刻只会更晚，离场门只会多拒，不会放过不该放的。演示种子不受影响：种子不造 `initial_route` 行（`scripts/demo-seeds/README.md` 写明那几张是运行时记录），因此没灌一次性库。

### 代决

1. **生效起点取委托接受的时刻。** 票面要判的两个候选「受理时刻」与「接受基线形成时刻」在本产品是同一刻：PS 的接受基线在接受决定那一刻固定，重建时校验基线固定时刻等于决定时刻（`rehydration.go` 里 `fixedAt` 对 `decidedAt` 那道校验），NR 的路由交接取的正是基线固定时刻（`route_on_acceptance.go` 的 `AcceptedAt: baseline.FixedAt()`）。理由同 CONTEXT「委托接受后……形成初始路由计划」：计划是对已接受委托的路由，业务上从接受起算数。否决的取法：
   - 交接信封的发生或记录时刻：那是 PS 发件箱的写入时刻，答的是「什么时候交出去」，不是业务边界；
   - 判断时刻（原写法）：积压之下离场早于生效，就是本票的病；
   - 接受时刻与判断时刻里较晚的那个：通常就是判断时刻，等于没改（ADR-0272 否决同一条）。
2. **不立新 ADR，也不在 ADR-0272 上挂「扩成」指针，规则只落 CONTEXT。** 核过：已接受记录里没有一份定过初始路由取哪个时钟生效，ADR-0272 Context 那句「初始路由今天也按处理时钟生效」是现状陈述，不是条款，所以不需要改任何已接受口径。ADR-0272 的条款只管复核形成的计划。初始路由一侧是领域生命周期上的一条规则，归 CONTEXT 一处定义，UC 只引它。ADR-0272 那边照协调方的话只在索引注记记「越权风险点 2 已答」。
3. **坏行读回两处都响亮。** 票面只点了 `FindByKey`。并发败方读回赢家同样经 `FindByKey`，读到坏行一样推不动，不响亮就漏一处，所以一并改了。
4. **`AT-NR-086` 号自取。** 取号时查过全部本地分支与 `merged/*`，没有用过这个号。协调方若要另分号段，改 UC-NR-001、AT 盘点与两条 Covers 即可。

### 越权风险点

1. **业务时间早于委托接受的控制触发仍会被离场门拒**（NR、PS owner）。PS 采用收寄与揽收时不拿业务时间比接受时刻，TF 登交接也不比。先揽后受理、离线补登的时间写早了，触发的业务时间就可能早于接受；那次复核让原计划失效或被替代时，离场早于生效，照旧整封响亮失败。本票之前同一格也拒，而且拒得更多：生效取的是更晚的处理时刻。这一格要一条领域裁定：这类触发是按「早于当前控制位置」入账不判（ADR-0235 的同类做法），还是复核时把离场夹到生效时刻，还是由 PS 在采用时就拒。本票不定，建议另立票。
2. **生效早于判断**（NR、VE、TF owner）。积压之下初始计划「从何时起算数」早于「何时判的」，与 ADR-0272 越权风险点 3 同形。VE 由初始路由派生的源事实，发生时间取判断时刻，有效时间取生效时刻（`derive_on_initial_route.go`），所以有效时间会早于发生时间。VE 两格分开存，有效时间只进内容指纹与查阅面，投影排序不读它，行为不变；查阅面上读者要按这条读两格。存量事实不重派生，指纹不受影响。TF 读初始路由只取段链（`planned_leg_source.go`），不读计划时间，不受影响。不积压时两者只差处理延迟。
3. **证据选版仍取判断时刻**（NR owner）。计划从接受时刻起生效，所用网络与限制版本却按判断时刻选（UC-NR-001 启动条件「初始计划必须使用接受后本次判断时适用的网络与限制版本重新评估」）。积压很久时，接受到判断之间换过版，计划会被读成「接受时就按新版生效」。复核形成的计划同样如此（ADR-0272），本票不动。
4. **接受时刻取自 PS 的时钟**（NR、PS owner）。PS 与 NR 两处时钟若有偏差，生效时刻可能比判断时刻还晚一点。这不破坏离场门：随后的触发总不早于接受的话，照样过得去。来源报错的接受时刻会原样成为生效时刻，同 ADR-0272 越权风险点 4。

### AT

UC-NR-001 立 `AT-NR-086`：路由交接积压，本上下文晚于委托接受才判初始路由，随后的控制触发业务时间落在两者之间。计划从委托接受的时刻起生效，判断时间记处理时刻；那次触发照常复核，失效或被替代都在触发的业务时间离场，不被离场门拒。编排与派发真库两条新用例的 Covers 指它。AT 盘点①加一行，文末加追记，总览计数没动。

### 用例

- 编排 `internal/networkrouting/application/initial_route_effective_from_test.go` 的 `TestUnderAHandoffBacklogTheInitialPlanTakesEffectAtAcceptance`：初始路由编排的处理时钟比接受晚十小时，计划落进复核夹具的路由库，触发取接受后五小时。
  - 「生效与判断」：生效等于接受时刻，判断等于处理时刻；
  - 「失效」：揽收按硬限制让原计划失效，适用性 `LAPSED`，转移时刻是揽收业务时间；
  - 「改善替代」：节点收寄、阈值 0（本测试交入的租户取值），原计划 `SUPERSEDED`，转移时刻是触发业务时间。
- 编排 `create_initial_route_test.go` 的 `TestAnUnrebuildableInitialRouteRowIsLoud`：判断之前读到坏行、败方读回赢家读到坏行，两格都答 `ErrStoredRouteRecordUnreadable`，不写、不交意图；读不回仍答 `UNDECIDED/ROUTE_STORE_UNAVAILABLE`。
- 派发真库 `cmd/parcel-dispatch/initial_route_backlog_test.go` 的 `TestUnderAHandoffBacklogTheInitialPlanTakesEffectAtAcceptanceOnTheDatabase`：
  - PS 真形成接受决定；那封信封投给生产形状的接受决定消费门（PS 委托仓储 → 初始路由编排 → NR 路由库、交接登记、标识、发布意图），处理时钟比接受晚十小时；只换证据视图、成本取数侧与商业适用性视图。
  - 落库的计划从接受基线固定时刻起生效，判断不早于接受后十小时。
  - 揽收业务时间取接受后五小时，选中候选被硬限制淘汰：复核落一行 `PLAN_LAPSED` 并复核那份计划；适用性册里那份计划 `LAPSED`，转移时刻是揽收业务时间。
- 先跑红：
  - 编排三格在修前：生效与判断不等；失效答 `lapse plan applicability: invalid plan applicability`；改善替代答 `supersede plan applicability: invalid plan applicability`。
  - 坏行两格在修前答 nil。
  - 派发那条把生效改回判断时刻后，计划生效等于处理时刻，断言不过。

### 验证

- `go build ./...`、`go vet ./...` 干净；改动的 Go 文件 `gofmt -l` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/visibilityexception/... ./internal/architecture/... ./migrations/...`：退 0，35 个包 ok，1 个包无测试文件。
- 三条新用例 `-count=3`（同一脚本，`-run 'TestUnderAHandoffBacklog|TestAnUnrebuildableInitialRouteRowIsLoud'`，两个包）：全过，9 次 PASS。
- 没跑全量；没重生成 ADR 索引、AT 计数与机制清点（`go run ./tools/adr-index -check` 因 ADR-0272 注记变了而退 1，留推送方重生成）。
