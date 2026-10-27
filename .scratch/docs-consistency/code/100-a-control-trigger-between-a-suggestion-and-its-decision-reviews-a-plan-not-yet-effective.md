# 100 NR 建议与决定之间的控制触发晚到：复核起点是从决定时刻才生效的新计划，判出失效或改善替代时离场门拒、整封响亮失败

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-code100。此前 needs-triage——建议 R2（上线后尽快）：响亮失败、不静默，但占着交接或收寄采用的分区按派发预算重投到 `ABANDONED`。2026-10-26 票 [code/29](./29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md) 作者子代理立，出处为 [ADR-0284](../../../docs/adr/0284-an-authorized-role-decides-a-current-reroute-suggestion-through-an-operator-decision-face.md) 越权风险点 6，分支 claude-code29。分级由协调方定稿。
Blocked by: 无（ADR-0292 已立）
出处：ADR-0284 越权风险点 6；UC-NR-003「给开发的交接」那一行。

## 现象

- 原计划在 T1 失效、复核形成改路建议；授权角色在 T3 据它形成决定，新计划从 T3 起生效（ADR-0284 决定四）。
- 一次控制触发的业务时间是 T2（T1 < T2 < T3），处理却在决定之后：包裹在无当前有效路由期间仍被交进下一个节点（节点写口对已经发生的作业照记，ADR-0251），信封又晚到。
- 它不早于当前控制位置（当前控制位置是 T1 那次复核），照判；判断起点是从 T3 才生效的新计划。
- 判出失效时，离场时刻取触发的业务时间 T2，早于生效 T3。领域离场门（`PlanApplicability.leaveCurrent`）拒，复核编排响亮报错，整封信封响亮失败，按派发预算重投。仍适用线上判出改善替代同此。
- 判出仍适用（交进的节点就在新计划上、没有改善）不撞。
- 评审意见：晚到的触发响亮失败不止重投——同一对象的交接信封会占着分区头，推迟追踪投影与派送发起，最长到重投预算耗尽（ADR-0215 越权风险点 10）；修法优先把这一格入账为未判，别让它走重投。

## 证据

- ADR-0272 决定三的论证是「复核能判的下一次触发都不早于当前控制位置，也就不早于这一次形成的计划的生效时刻」；决定形成的计划从决定时刻起生效、而决定不推当前控制位置，这条论证对它不成立（ADR-0284「对已接受记录的作用」ADR-0272 一条）。
- 复核编排失效线 `Lapse(basis, trigger.OccurredAt())`、改善替代 `Supersede(..., trigger.OccurredAt())`；离场早于生效即 `ErrInvalidPlanApplicability`，复核答错误、不折未决。
- 决定编排判过时只看在册的复核；还在派发管道里的触发它看不见。

## 三类

机制。无产品策略；无租户取值。

## 规模

S–M，取决于取法。

## 要裁的

三种取法各改一份已接受记录的口径：

- **甲：这次触发入账为未判**，原因「触发早于当前计划生效」。扩 ADR-0235 决定一的未判原因封闭集，形同 ADR-0279 对委托接受做过的那一格。代价：交进节点那一次不判，等下一次控制触发从它的位置判。
- **乙：决定推当前控制位置。** 改 ADR-0235 决定四「当前位置」的定义，也撤 ADR-0284 决定四「不推当前控制位置」。代价：T2 那次按「触发早于当前位置」入账，与甲同一个结局，还把一个非控制事实塞进当前位置。
- **丙：这次触发对着它业务时间那一刻的状态判**，即 T2 时无当前有效路由，走无路由那条线。代价：可能另形成一份从 T2 起生效的首个计划，与决定形成的计划并存，要另裁谁让谁。

倾向甲。

## 既有票

[code/29](./29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md)（决定面）；[demo-chain-gaps/154](../../demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md)（未判之后的续办）。

## 完成记录（2026-10-26，作者，分支 claude-code100，起点 `c8f0f3522`）

### 提交

- `a171c5017` fix(network-routing)：领域 `PlanApplicability.NotYetEffectiveAt`、端口新未判原因与 `NotJudgedTrigger.PlanEffectiveAt`、复核编排分流、未判登记适配器核对、迁移 `network_routing/0031_route_reassessment_not_judged_before_plan_effective.sql`、全部用例（含 `cmd/parcel-dispatch` 真库端到端）。
- `774824b4c` docs(adr)：ADR-0292（取预留号），ADR-0235 / 0284 / 0309 的 Status 注记与 Links，NR CONTEXT 一句，UC-NR-003（结果行、一致性一条、AT-NR-104 / 105、「给开发的交接」撤一行），AT 盘点分桶表两行与一段追记。

### 裁向与新未判原因

- 采甲：这次触发入账为`未判`，原因 **`TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`**（Go 名 `NotJudgedTriggerPrecedesPlanEffectiveFrom`，业务名「触发早于计划生效」）。名字照 NR 既有未判原因构词（`TRIGGER_PRECEDES_CURRENT_POSITION`、`TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`）：派单示例的 `TRIGGER_BEFORE_PLAN_EFFECTIVE` 与既有两格用词不一致，改用 `PRECEDES`。
- ADR-0292 依据：扩 ADR-0235 决定一的未判原因封闭集（形同 ADR-0279）。ADR-0284 越权风险点 6 与 ADR-0309 越权风险点 5 同一类，一并由它回答；ADR-0272 / 0279 的「复核能判的触发都不早于路由历史里任何一份计划的生效」论证对决定形成的计划不成立，第三道下限补上，两份都无条款停用。

### 各层改了什么（ADR-0025 全函数核对）

| 层 | 改动 |
|---|---|
| 领域 | `PlanApplicability.NotYetEffectiveAt`：只有当前有效的计划才有「还没生效」，同一时刻已生效；离场门 `leaveCurrent` 也改用它比，门与分流同一条比较 |
| 端口 | `NotJudgedReason` 末尾加一格（不动既有 iota 值）、`String()`、`NotJudgedTrigger.PlanEffectiveAt` |
| 复核编排 | 失效线在 `Lapse` 之前、改善替代在 `Supersede` 与接班计划形成之前分流；`PlanStillApplicable` 与判不出改善的各格不动。改善线靠包内哨兵 `errTriggerPrecedesPlanEffective` 把「要换班了」交回 `reassessPlan` 入账，哨兵不出包 |
| 未判登记适配器 | 生效时刻与这一格原因同来同去（适配器先拒一次） |
| 库 | 迁移 0031：原因 CHECK 加一格、`plan_effective_at` 列、`reason = 新原因 <=> plan_effective_at 非空` 且 `trigger_occurred_at <= plan_effective_at`。比较用 `<=`：计划未登记适用性行时生效时刻读自计划 JSON、带纳秒，落库截到微秒（理由同 0024） |
| 消费门（收寄采用、交接两路） | 不改：两路 `reassessmentToConsumption` 对`未判`整格入账，不看原因；用例钉死（含变异） |
| HTTP、管理台、读面、cmd 装配 | 无落点：未判登记今天没有读面，`apps/admin-web` 没有未判原因的标签表（grep 全仓无 `TRIGGER_PRECEDES_*` 字面量），故没动管理台，也没碰 `pages/network/api.ts` |
| 取号 | NR 迁移 main 末号 0030（code/97），在途分支只见同一份 0030，取 0031；ADR 取预留 0292；AT 取 `AT-NR-104`、`105`（全仓与各分支最大为 103） |

### 代决

1. **判出仍适用不拦。** 拦的是会撞离场门的两件事（失效离场、改善换班），不是一次不动计划的复核。判出仍适用、以及仍适用线上判不出改善（没声明自动改路、成本读不回、改善不够阈值、已越过冻结边界、场外揽收）照常记`仍适用`，不入账为未判。比较因此放在判出之后：次序是已有结果分界 → 委托接受 → 当前位置 → 读起点与证据、判 → 计划生效。适用性看不清、冻结形态未配置先于本格答（它们不是业务时间问题）。
2. **生效时刻取适用性里「当前有效」那一刻**（有登记行取其转移时刻，没有取计划的生效时刻），与离场门比的是同一个值、同一条比较。判据不问计划是谁形成的：授权改路决定的计划（ADR-0284）与授权裁并列的首个计划（ADR-0309）同落此格。
3. **终局入账，不进 demo-chain-gaps/154 的续办触发。** 业务时间与生效时刻都不会变，重投推不动；恢复动作是「等下一次控制触发从它自己的位置判」，不是「等某个登记」，与 `TRIGGER_PRECEDES_CURRENT_POSITION`、`TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE` 同类（154 本就写「早于当前位置是终局入账，不进续办」）。154 的票面没动，归类写在 ADR-0292 决定一与越权风险点 1。
4. 未判登记另记比较所用的生效时刻（`plan_effective_at`），三格（接受、当前位置、计划生效）各记各的列，不合用。
5. 适配器与库各拒一次「时刻与原因不同来同去」。
6. 回答 ADR-0309 越权风险点 5（同一类）：在 ADR-0309 的 Status 加了一段「回答」注记与 Links，未改正文。

### 越权风险点（详见 ADR-0292 文末）

1. 交进节点的那一次不判，等下一次控制触发再判；之后若再没有控制触发，这次失效不会被判出（与 ADR-0279 风险点 1 同形，补法是「从生效时刻的位置补判一次」那种续办，另议）。
2. 真正落进这一格的，是决定落库时仍在派发管道里的触发，和离线补登、业务时间远早于处理时间的触发；窗口里按时处理的触发走无路由那条线、自动形成首个计划，建议随之过时，形不成决定，不落进这一格（并列那一路同理：按时处理的触发本是 `NO_ROUTING_HISTORY` 未判，晚到的入账为新原因，结局相同）。升 R1 的条件是派发积压或离线补登的常态时延长于授权角色的决定时延。缓解如实说：未判登记今天没有读面，只能查库，没有按包裹列出的入口。评审的严重度判断（维持 R2）与补救方向（给未判登记补读面，或在 154 里增加一类「按决定计划生效时刻的位置补判」，当前行没存触发的位置和控制种类，补判要回原信封取原料）见 ADR-0292 越权风险点 2。
3. 改善替代线上被拦的那一次照取了成本、照排了候选、照取了号（先判出改善才知道这是换班）。
4. 触发业务时间取自来源、决定时刻取 NR 时钟，时钟偏差挪动边界。
5. 决定编排的「过时」仍只看在册复核，未改。
6. 仍适用时「起点计划还没生效」不留痕（复核记录带触发业务时间，审计可自行比）。

### 判据对用例

| 判据 | 用例 |
|---|---|
| 离场门与分流同一条比较 | 领域 `TestACurrentPlanIsNotYetEffectiveOnlyBeforeItsEffectiveTime`（含与 `Lapse` 同答） |
| 早于生效且判出失效 / 改善替代 / 揽收失效 → 未判、不动适用性、不写复核；同一触发再来同答 | 应用 `TestATriggerEarlierThanTheCurrentPlansEffectiveTimeIsNotJudged` |
| 票面时序（失效 → 建议 → 决定 → 业务时间在中间的触发），随后不早于生效的触发从决定形成的计划判 | 应用 `TestATriggerBetweenASuggestionAndItsDecisionIsNotJudgedAgainstTheDecidedPlan` |
| 判出仍适用 / 改善不够阈值 → 不撞，照常记 | 应用 `TestATriggerEarlierThanThePlansEffectiveTimeThatKeepsThePlanIsStillJudged` |
| 同一刻与晚于生效照判 | 应用 `TestATriggerNotEarlierThanThePlansEffectiveTimeIsJudgedAsUsual` |
| 先比当前位置 | 应用 `TestATriggerEarlierThanTheCurrentPositionIsComparedBeforeThePlanEffectiveTime` |
| 适用性看不清、冻结形态未配置先于本格答，各点名自己的原因、不带计划生效时刻 | 应用 `TestUnclearApplicabilityAndAnUnconfiguredFreezeFormAreAnsweredBeforeThePlanEffectiveTime` |
| 消费门收工、不重投、不登未决哨兵 | 交接适配器 `TestAHandoverEarlierThanThePlanTakesEffectSettlesNotJudged`、收寄采用适配器 `TestAnIntakeEarlierThanThePlanTakesEffectSettlesNotJudged` |
| 未判登记：时刻与原因同来同去，纳秒 / 微秒舍入 | postgres `TestATriggerBeforeThePlanTakesEffectCarriesTheEffectiveTimeItWasComparedWith`（适配器与库各拒一次） |
| 真库端到端（票面时序，交接一路）：修前整封响亮失败 `lapse plan applicability: network routing: invalid plan applicability`，修后入账 `PROCESSED`、复核册只有揽收一行、决定形成的计划适用性没有登记行；之后一次不早于生效的交接判出失效、引用决定形成的计划 | `cmd/parcel-dispatch` `TestAHandoverBetweenASuggestionAndItsDecisionIsNotJudgedOnTheDatabase` |
| 真库：同一窗口里交进计划上的节点、仍适用，不入账未判 | `cmd/parcel-dispatch` `TestAHandoverBetweenASuggestionAndItsDecisionThatKeepsThePlanIsStillJudgedOnTheDatabase` |

端到端夹具的候选与硬限制准备抽成 `declareOneRestrictedOfTwoCandidates`，原 code/29 的用例改调它，断言没动。

### 自验

- 先红后绿：领域用例先因 `NotYetEffectiveAt` 未定义变红；应用层四个新用例（失效、改善替代、揽收失效、决定形成的计划）修前以 `lapse plan applicability: ...invalid plan applicability` / `supersede plan applicability: ...` 变红，「判出仍适用」「同一刻」两组修前就绿（它们守的是不过度拦）；真库端到端修前（编排文件还原到 `c8f0f3522`）以同一错误变红。
- 变异（先备份到私有目录，改完跑用例，拷回并 `cmp` 一致）：
  1. 去掉两处新判断（回到响亮失败）：应用 2 个用例、交接与收寄采用两个适配器用例变红；只去改善替代线：改善替代子用例变红。
  2. 仍适用也入账为未判：「判出仍适用」两个子用例变红。
  3. 新原因进重投名单（编排改答未决）：应用 4 个、两个适配器用例变红；消费门把新原因折成重投（交接适配器）：交接适配器用例变红。
  4. `NotYetEffectiveAt` 把同一时刻算作还没生效：领域与应用「同一时刻照判」变红；分流用的时刻与门差一纳秒：领域用例变红。
  5. 迁移 CHECK 的 `<=` 改 `<`：同一微秒子用例变红；去掉「原因与时刻同来同去」：两个绕过适配器的直插子用例变红；适配器不写 `plan_effective_at`：库 CHECK 拒写，登记用例变红。
- `go build ./...`、`go vet ./...`、`gofmt -l` 无输出。`TMPDIR=… scripts/test-full.sh ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-network-register/ ./internal/networkrouting/... ./internal/{parcelshipment,transportfulfillment,visibilityexception}/adapters/networkrouting/ ./internal/architecture/... ./migrations/...` 全过（反向依赖由 `go list -test` 算出）；新用例 `-count=3` 过（应用 / 领域 / 适配器与真库）。
- 迁移改了：一次性库灌 `scripts/demo-seeds/seed.sh` 过，已删库。
- `go run ./tools/adr-index`（本地跑过、README 未提交）无格式错；`go run ./tools/at-coverage-counts -check` 退 1（只是总览计数过期，进 main 时重生成）。
- admin-web 没改，未跑前端检查。

### 给评审的提醒

- 改善线的哨兵放在 `tryImprovementReroute` 里 `improvementBasisOf` 判出「够格」之后、形成决定与写适用性之前；这一步之前没有写。拦下时已经取过成本、取过一个计划版本号（仍适用线判不出改善时同样会取）。
- ADR-0309 不在派单点名的注记范围，但它的越权风险点 5 与本票同一类，按「回答」约定只在 Status 与 Links 加了前向指针。

### 评审记录与修订轮

独立评审结论：可合入，下列各项合入前修。

- **N1（越权风险点 2 的因果与缓解不实）**：原文「建议放几天，这几天里到的每一次交接都落进窗口」不成立——建议形成后原计划已失效，窗口里按时处理的交接走 `reassessNoRoute`、自动形成首个计划，建议按「另有业务时间不早于建议的复核」过时，决定编排答已过时，形不成决定；并列一路按时处理的触发本是 `NO_ROUTING_HISTORY` 未判。真正落进这一格的只有决定落库时仍在派发管道里的触发，和离线补登、业务时间远早于处理时间的触发。已改写 ADR-0292 越权风险点 2 与本票相应一条：升 R1 的条件改为派发积压或离线补登的常态时延长于授权角色的决定时延；缓解如实写明未判登记没有读面、只能查库；写入评审的严重度判断（维持 R2；位置类失效下一次交接自愈，限制类失效决定编排已按此刻证据重判；窗口内晚到的触发恰是最后一次控制触发时包裹卡在计划外节点，作业暂停和停滞可见）与补救方向（票 demo-chain-gaps/154 后续项）。
- **N2（ADR-0292「对已接受记录的作用」引文）**：ADR-0235 的 Status 里 ADR-0250 停的是决定一「重投推得动的照旧是未决……」那一句，只有 ADR-0279 停了本句，括注改为「该句已被 ADR-0279 扩过一格」；引 ADR-0272 决定三时补全原文的「（ADR-0235 决定四）」。
- **N3（决定二声明的次序没有用例钉住）**：评审变异（在 `reassessPlan` 的未决分支与冻结形态未配置分支之前各加「计划还没生效就答 `TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`」）全绿存活。补应用层用例 `TestUnclearApplicabilityAndAnUnconfiguredFreezeFormAreAnsweredBeforeThePlanEffectiveTime`：起点计划未生效，分别加选中候选限制状态未知与策略版本未声明冻结，断言原因为 `PLAN_APPLICABILITY_UNCLEAR` 与 `FREEZE_FORM_UNCONFIGURED`（这两个原因靠登记补齐、走 154 续办，不能被记成终局原因）。变异核验：把未决分支的分流挪到前面，「适用性看不清」子用例变红；把冻结形态未配置的分流挪到前面，「冻结形态未配置」子用例变红；备份、改、跑、拷回并 `cmp` 一致。AT-NR-104 的盘点行与 UC-NR-003 该行同步点名这条次序。
- **风格项**：`cmd/parcel-dispatch/reassess_before_plan_effective_test.go` 文件头去掉变更叙述的两句；`plan_applicability_test.go` 里「离场了的计划不再问」改为「不问」。
- **票面**：票头 `Blocked by:` 改为「无（ADR-0292 已立）」。
- **提交**：修订轮两笔，`4f2c94fb1` test(network-routing)（N3 用例与风格项）、`199952ec3` docs(adr)（N1、N2、票面）；上文「提交」一节的两个号已随 rebase 订正。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code100` 由作者 rebase 到 origin/main `ae370c593`→`891a6c349` 后快进，tip `73a7d05fb`；迁移 NR 0031；新立 ADR-0292（扩成 ADR-0235 决定一的未判原因封闭集，同 ADR-0279 的先例；回答 ADR-0284 越权风险点 6 与 ADR-0309 越权风险点 5）。新未判原因 `TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM`，AT-NR-104、105。
- 协调方分诊：R2，采票面倾向的甲——这次触发入账为未判（交进节点那一次不判，等下一次控制触发从它的位置判），不再整封响亮失败、按派发预算重投到 `ABANDONED` 占着交接/收寄采用的分区。「判出仍适用」与「判不出改善」的各格不拦，照常记；不进 demo-chain-gaps/154 的续办触发（它是白名单，恢复动作是等下一次控制触发，不是等某个登记）。
- 隔离评审（Sonnet）可合入，无阻断；核实不过度拦（失效线分流在 `Lapse` 之前、改善替代线在判出之后 `Supersede` 之前；`NotYetEffectiveAt` 与离场门 `leaveCurrent` 同一个函数；纳秒构造与迁移 `<=` 成立）、两路消费门对未判整格入账不重投。N1 ADR-0292 越权风险点 2 的因果写得不实（窗口里按时处理的交接走 `reassessNoRoute` 自动形成首个计划，真正落进这一格的只有决定落库时仍在派发管道里的触发和离线补登的触发；维持 R2）、N2 括注不实（ADR-0250 停的是另一句）、N3 「看不清与冻结未配置先于本格答」的次序补两个子用例（变异存活）、风格项，由 Sonnet 子代理收掉。
- 协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（施加 0031），用后删库。
- 去向：无新票。越权风险点：未判登记没有读面，只能查库；窗口内晚到的触发恰是最后一次控制触发时包裹会卡在计划外节点（方向性作业暂停和停滞，可见），补救方向是给未判登记补读面或在 demo-chain-gaps/154 增加一类「按决定计划生效时刻的位置补判」；改善替代线被拦的那一次已取过成本、排过候选、取过一个计划版本号。
