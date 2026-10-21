# 155 NR 初始路由的未配置类未决照样重投到 ABANDONED

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——隔离评审无阻断项、评审修订完，待进 main，分支 claude-dcg155（2026-10-21，见文末「完成记录」「评审记录」）。此前完工待评审。此前 needs-triage——2026-10-21 Claude Code 会话立（票 131 实施时读代码发现，与 131 同族；用户「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）
Blocked by: 无
出处：票 [131](./131-network-reassessment-undecided-is-split-by-reason.md) 实施；[ADR-0235](../../../docs/adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定六

## 现象

初始路由一路（接受决定信封 → NR 初始路由，UC-NR-001）由 `nrparcelshipment.outcomeToConsumption` 折成消费门两格：只要有一件包裹停在未决，就交 `ErrRouteHandoffUndecided`，整封回滚重投。

包裹级未决原因里有几格重投推不动：网络证据未配置、路由策略版本没声明排序形态、候选成本取数侧没接计价输入路径。它们与票 131 修掉的复核那三格同族，信封会重投到派发预算耗尽、落 ABANDONED，同一分区之后的信封排在它后面。

初始路由一直不形成，之后这件包裹的每一次复核都落「没有路由历史」，入账未判。

## 三类

机制。规模 M。租户取值：无（未配置那几格本来就答未配置，不补默认）。

## 最小的诚实修法

照 ADR-0235 决定一、二的形状办：初始路由的未决原因按恢复动作分格，重投推不动的那几格入账、留下原因，依赖类的照旧重投。

与复核不同，初始路由是一次交接里逐件包裹的结果，一封信封里可能有的包裹判成、有的停在未配置。要先裁两件：整封入账时，判成的那几件照常提交，停住的那几件各留一行原因；原因落 NR 的哪一册（复用复核的未判登记，还是初始路由另立一册）。续办并入票 [154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md) 的同族修法。

## 要裁的

逐包裹的未判原因落哪一册；同一封交接里判成与未判并存时消费门怎样折。归 NR owner。

## 既有票

[131](./131-network-reassessment-undecided-is-split-by-reason.md)（复核一路，已修）；[154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md)（续办）。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/nr-no.md](../../docs-consistency/audit/nr-no.md) H1（归 C，高）落在本票，不另立；它把本票的范围扩大了两处：

1. **不只是未配置类**：最低成本并列（`domain.RankingTied` → `RouteCandidatesTied`）、成本待判断、不可计价、币种不齐也一样落包裹级未决并整封回滚重投，重投多少次都不变。NR CONTEXT「路由形成与选择」要的是「初始路由遇并列既不形成计划也不形成无当前有效路由，保留全部候选与并列理由并保持可续办」；今天并列候选与理由只在内存结果里，信封落 `ABANDONED` 之后什么也没留下。CONTEXT 承认首发形态下「单维恰恰最容易打平」，并列会是常见情形。
2. **兄弟包裹连坐**：`adapters/postgres/initial_route.go` 的 `InitialRoutes.Save` 走 `RequireExecutor`、与消费门同一事务，整封回滚时同一委托里已判成的包裹计划一起撤掉，与 UC-NR-001「某个包裹未决或无路由不回滚其他包裹已经合法形成的计划」、`AT-NR-012` 相反。

按 main `67438a0f` 复核，`adapters/parcelshipment/route_on_acceptance.go` 的 `outcomeToConsumption` 仍是「任一件 `ParcelRouteUndecided` 即 `ErrRouteHandoffUndecided`」，现象全在。

扩充后的最小修法：初始路由的持久未决（并列、各未配置格、成本待判断、不可计价、币种不齐）照 ADR-0235 决定一、二的形状入账并留下原因；并列候选与并列理由落库；逐包裹提交，判成的那几件照常形成计划，不连坐。「要裁的」两件不变，再加一件：并列的续办入口（授权角色人工选择，UC-NR-001「人工选择」）是否同票。

分级建议（文档自洽 02）：R1——第一天就撞（单维成本并列常见），后果是整单无路由且不留痕，无绕法。由协调方定稿。

## 备注（2026-10-21，票 demo-chain-gaps/115 隔离评审）

同族一格：候选成本取数侧的确定性错误也落进了重投桶。`internal/networkrouting/adapters/parcelpricing` 的 `errUntranslatableEvaluation`（含候选没有线路引用、比较币种译不出）与 `errNotACostEvaluation`（方案方向或用途配错）重投多少次都一样，初始路由却把它们与依赖读不回一起答 `COST_SOURCE_UNAVAILABLE`、回滚重投。票 115 让复核照同一口径接取数侧，无路由那条线上同样答 `REASSESS_COST_SOURCE_UNAVAILABLE`、回滚重投，继承了这一格。修时两路一起分：确定性错误不进未决哨兵。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg155`，基于 main `f4a7e226a`）

### 接票时核实

票面与审计扩充的三件都成立：`outcomeToConsumption` 任一件包裹未决即交 `ErrRouteHandoffUndecided`；未配置各格、并列、成本待判断、不可计价、币种不齐重投多少次都一样；`InitialRoutes.Save` 与消费门同一事务，整封回滚时同一交接里判成的计划一起撤掉。另核得一处票面没写的：SYN 纵切的委托不申报地址要素，演示网络上那一条候选落「证据未知」（`GEO_ELEMENTS_MISSING`、`CUSTOMS_ENDPOINT_COUNTRY_MISSING`），原先也走 `CANDIDATE_EVIDENCE_INCOMPLETE` 重投；受理基线一经固定不再改写，这一格永远推不动。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」，代 NR owner）

权威在 [ADR-0273](../../../docs/adr/0273-a-settled-wait-for-registration-may-enter-without-a-same-change-resume-and-adr-0094-decision-four-binds-only-the-acceptance-chain.md) 决定二（重投推不动、只等一次登记的格先入账，原因与对象同笔落库）与 ADR-0029（恢复动作不同必须分）。本票不立新 ADR、不改 ADR 正文。

1. **推不动的九格入账为`路由未形成`**：证据未配置、候选空间立不起来、候选证据不全、排序形态未声明、最低成本并列、待判断、不可计价、币种不齐、取数侧没接。判据只在编排一处：这九格从 `RouteUndecidedReason` 移到 `ports.InitialRouteNotFormedReason`，码不变；适配器只按结果折。候选证据不全入这一格，依据是 ADR-0250 决定一的那条纪律：取数侧只交「状态未知」的来源一律是重投推不动的缺口，真依赖故障在取数侧交错误。
2. **推得动的照旧未决、整封回滚重投**：判断库、证据、商业适用性、交接登记读不回，取号故障，证据连续换代，成本读不回；另加一格 `NOT_FORMED_UNRECORDED`（原因写不进未形成登记，护栏同 ADR-0094 决定五）。
3. **原因另立一册，不复用复核未判登记**：`network_routing.initial_route_not_formed`（迁移 0025），主键是完整判断键，与 `initial_route` 同一组维。复核未判登记按触发关联、答「这一次为什么没判」；这里答这件包裹「最近一次停住的原因」。所以同一判断键再停一次按最新一次改写，不像复核那样只留先到那一行。
4. **判成与未形成同在一封交接时整封入账**：判成的照常提交、交意图，未形成的那几件各留一行原因，不连坐（AT-NR-088）。
5. **并列留痕落库**：全部候选、并列的那几家、各自打平的金额与币种（AT-NR-089）。候选证据不全另记全部候选与缺口，形照 ADR-0250 的「登记同行记缺口」（AT-NR-090）。授权角色的人工选择入口不同票，仍归 [docs-consistency/code/29](../../docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md)。
6. **成本取数侧的确定性错误给一个哨兵** `ports.ErrRouteCostAnswerUnusable`：`errUntranslatableEvaluation`（含候选没有线路引用、比较币种或方案引用译不出）、`errNotACostEvaluation`（方案方向或用途配错）、`errUntranslatableParcelFacts` 都挂在它名下。初始路由与复核无路由那条线据它响亮报错，不进未决哨兵，也不落未形成登记：它不是等登记的缺口，落了就是一条点名租户没登记的假原因（AT-NR-092、093）。复核失效线与仍适用线照常落、不拦（失效必须照常落库的硬句；仍适用已判定）。
7. **取数侧没接（`COST_SOURCE_NOT_CONFIGURED`）在初始路由入账为未形成**，先例是 ADR-0250 把「关务来源未接」入账未判；装配没给未形成登记、而包裹真停在那几格时响亮报错（`ErrNotFormedRegisterMissing`）。
8. **续办在本票停下**：ADR-0273 决定四要登记信封驱动，可重驱初始路由要随请求携带的地理投影。ADR-0075 决定三不许 NR 落地址本体，ADR-0215 决定二「经消费侧适配器取受理基线那一版」的适用场景只到「不由小包托运信封触发的路由复核」，明写初始路由随请求携带。让初始路由重驱也这样取，要先有一份记录扩那条适用场景。照任务约束不立新 ADR，续办并入票 154，那张票的「要裁的」第二条正是同一问（已在 154 记「并入」一节）。ADR-0273 决定二允许先入账、续办另票，缺口就记在 154 上。

### 越权风险点

1. **入账之后没有续办**（NR owner）。登记补齐、部署接入或授权裁决之后，这几件包裹不会自动形成初始路由。此后它们的每一次复核都落「没有路由历史」未判。原先烧到 ABANDONED 同样丢了这次判断，还拖住分区；现在至少原因与对象可查。阻塞链：初始路由的续办并在票 154 里，154 又 Blocked by 票 14（同族续办形状），要先等 14、再等 154。
2. **未形成没有读面**（NR owner）。只能在库里按原因、按包裹查，读面随票 154 定。未形成也不发布任何业务事实：VE、NO 看不到这件包裹为什么没有路由，这一点与`无当前有效路由`不同。
3. **依赖类未决仍连坐**（NR owner）。依赖未决的那一件让整封回滚，同一交接里判成的计划随之回滚，重投时再判一次，可能判出别的结果。`AT-NR-012` 依赖未决那一件因此仍未实现，UC-NR-001 交接表记「待立票」。
4. **取数侧答用不了照样占着分区**（NR owner）。响亮报错按派发预算重投到 ABANDONED，失败码是 `dispatch.publish_failed`，不再是 `consumer_undecided`。方案方向配错多半是租户登记错，owner 若要它入账未形成，改的是决定 6 的分格。
5. **复核两条线吞下取数侧用不了**（NR owner）。失效线把它记成候选评估未决，仍适用线不试改善，缺陷只在无路由那条线上报出来。
6. **登记按最新一次改写**（NR owner）。第一次停住的原因不再留下，「这次为什么」只能看最新一行；并列的候选与打平金额、候选证据不全的缺口这些依据也随改写一起丢掉。
7. **候选证据不全一格混着永久缺口**（NR、PS owner）。受理基线缺国家补不上，等登记的限制状态未知补得上，两者同在一个原因码下，靠缺口那一列区分。补救在 PS：受理时就拒收缺国家的投影，同 ADR-0250 越权风险点 2。
8. **取数侧没接一格两边口径不一**（NR owner）。初始路由入账未形成，复核响亮报错（`ErrReassessCostSourceNotConfigured`，这一处没改）。生产装配今天两边都到不了这一格。
9. **装配改了 `assemble.go` 一行**（NR owner）。`acceptanceConsumer` 本就在那个文件里，没有特性文件可改，只在 `CreateInitialRouteDeps` 里加了 `NotFormed: routeStore`，同一只判断库兼作未形成登记。哨兵名单与路由文件都没动，只改了 `routes_accepted_decision.go` 的注释。

### 做了什么

- 端口 `ports.go`：`InitialRouteNotFormedReason`、`InitialRouteNotFormed`、`InitialRouteNotFormedRegister`、`ErrRouteCostAnswerUnusable`。
- 迁移 `migrations/network_routing/0025_initial_route_not_formed.sql`。原因在库里封闭，在场件逐原因钉住。
- postgres：`InitialRoutes.RecordNotFormed`，写在新文件 `initial_route_not_formed.go` 里，要事务、按最新一次改写；缺口行模型 `rowsOfGaps` 与可达性判断共用。
- 编排 `create_initial_route.go`：新结果 `ParcelRouteNotFormed`（`NOT_FORMED`）；`RouteUndecidedReason` 只留推得动的格，加 `NOT_FORMED_UNRECORDED`；`recordNotFormed` 落登记，写不进就改答未决；成本答用不了时响亮报错；`CreateInitialRouteDeps.NotFormed`。
- 复核 `reassess_route.go`：`candidateCosts` 把用不了随错误交回，无路由线上抛，失效线与仍适用线不拦。
- 取数侧 `adapters/parcelpricing`：三个确定性错误挂到哨兵下。
- 适配器 `route_on_acceptance.go`：只改注释，折法本就只看 `ParcelRouteUndecided`。
- 派发：`assemble.go` 那一行，`routes_accepted_decision.go` 只改注释。
- 文档：NR CONTEXT「路由形成与选择」补初始路由两格；UC-NR-001 结果表、未形成结束、候选判断规则、步骤 8C、提交边界、`AT-NR-012` 注记、`AT-NR-088`–`092`、交接表；UC-NR-003「复核未决」一格与 `AT-NR-093`；AT 盘点加六行、改 `AT-NR-006` 行内用例并追记（计数未动）；票 154 记并入。

### 用例

测试与实现同轮写成，没有严格做到先红后绿。判别力用变异核对：把适配器的折法临时改成「未形成也交本路哨兵」，`TestSYNAcceptedDecisionSettlesAnUnconfiguredRouteAsNotFormed` 与 `TestAnUnresolvableCandidateSettlesTheAcceptanceThroughProductionRoutes` 都红，接受信封 `PENDING` + `dispatch.consumer_undecided`；改回后绿。

- 编排（`create_initial_route_not_formed_test.go` 与改写的既有用例）：
  - 七格逐一入账，兄弟包裹照常形成计划（AT-NR-088）；取数侧没接入账。
  - 依赖四格仍未决、不留行，含原因写不进那一格（AT-NR-091）。
  - 取数侧用不了响亮报错（AT-NR-092）；装配没给登记时响亮报错。
  - 并列记候选与打平金额（AT-NR-089）；关务资格未知记候选与缺口（AT-NR-090）。
- 复核：`TestAnUnusableCostAnswerIsLoudOnlyWhereNothingIsSettled` 走三条线（AT-NR-093）。
- 取数侧：`TestDeterministicCostAnswersAreUnusableNotUnreadable` 覆盖方向配错、没有线路引用、币种译不出三种。
- 适配器：`TestANotFormedParcelIsAccountedNotRetried`（未形成入账）；`TestAnUndecidedParcelRollsTheDeliveryBack`（依赖未决回滚）。
- 未形成登记真库：一行往返且按最新一次改写、并列三列、原因与在场件封闭、无事务拒写、回滚无痕。
- 派发真库，经生产路由表：
  - SYN 空目录：接受信封 `PUBLISHED`，未形成一行 `ROUTE_EVIDENCE_NOT_CONFIGURED`，交接登记一行，再拍不翻倍。
  - 演示网络、基线缺国家：`CANDIDATE_EVIDENCE_INCOMPLETE`，记下候选与两条缺口。
  - 没种适用性（依赖不可用）：`dispatch.consumer_undecided`，NR inbox 无账，未形成零行。
  - SYN 纵切八条以第一拍定稿条数作证的用例改为数接受信封那一条（`synAcceptanceSettledOnFirstBeat`）。

### 验证

于 `ce3bc30f5`：

- `go build ./...`、`go vet ./...`、`gofmt -l cmd internal migrations tools` 干净。头两笔代码提交 `4503772b9`、`5769bffd5` 在一次性 worktree 里各自单独 `go build ./...` 并 vet NR 与派发，通过。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh` 跑了 `./internal/networkrouting/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-network-register/`，PS、TF、VE 各自的 `adapters/networkrouting`，`./internal/architecture/...`、`./migrations/...`：21 个包 ok，0 FAIL。
- 新用例另跑 `-count=3`，全绿。
- 演示种子在一次性库 `dcg155seed` 上灌一遍，零报错、迁移 0025 建表、未形成零行，灌完删库。
- 未跑全量；未重生成机制清点、ADR 索引与 AT 计数。

迁移落号 `network_routing/0025`。

### 未决事项

1. 续办：并入票 154，先裁地理投影从哪里来（可能要一份记录扩 ADR-0215 决定二的适用场景）。也要定 PP 价卡登记的信封送不送到 NR。
2. 依赖类未决连坐兄弟包裹（`AT-NR-012` 依赖未决那一件）已立票 [docs-consistency/code/78](../../docs-consistency/code/78-initial-route-dependency-undecided-drags-formed-siblings.md)。
3. 越权风险点 4、8 两处口径要 owner 定。

## 评审记录（2026-10-21，协调方隔离评审：无阻断项；作者子代理同分支修订）

评审核过九格的判定、整封部分提交与「按最新一次改写」一册的取舍，三条变异都被用例抓住。分支先 `git rebase main`（`8d68c7bab`），无冲突。逐条去向：

1. **`cost_source.go` var 块头注「下面两格」是计数式的跨行引用**——已修：头注拆进 `errUntranslatableEvaluation`、`errNotACostEvaluation` 各自的注释，各自点名挂在 `nrports.ErrRouteCostAnswerUnusable` 名下。
2. **演示动线脚本没告诉人路由未形成时去哪儿查**——已修：`docs/design/synthetic-demo-journey-script.md`「起 dispatch」一节补带日期的补记，写明接受信封答 `PUBLISHED`，原因要查 `network_routing.initial_route_not_formed`，附查询。
3. **「此刻为什么还没有初始路由」说过头了**（改写的是最近一次停住，不是此刻的权威状态）——已修：`ports.InitialRouteNotFormedRegister`、迁移 0025 头注、postgres 写口注释、NR CONTEXT 与本票代决 3 都改成「最近一次停住的原因」。迁移 0025 未进 main，就地改。越权风险点 6 补「并列与缺口的依据也随改写丢掉」。
4. **`planReference` 里 `NewVersionReferenceIdentity` 的错误没挂哨兵**——已修：照切不开那一支包进 `errUntranslatableEvaluation`；`resolveLegs` 调用处补段号与候选上下文、照旧上抛。`TestDeterministicCostAnswersAreUnusableNotUnreadable` 补一格「方案引用立不成」（引用 `plan-x /v1`）；变异核对：拿掉这一层包装，这一格红，改回后绿。
5. **越权风险点 1 没写阻塞链**——已修：初始路由的续办并在票 154，154 Blocked by 票 14，要先等 14、再等 154。
6. **AT 盘点 `AT-NR-006` 行少点一名**——已修：补 `internal/networkrouting/adapters/customscompliance/customs_source_test.go::TestAnUnreadableCatalogIsADependencyFailureNotAGap`（关务目录读不到以 `ErrCustomsCatalogUnreadable` 上抛，取数侧半边）。
7. **未决 2 另立票**——已立 [docs-consistency/code/78](../../docs-consistency/code/78-initial-route-dependency-undecided-drags-formed-siblings.md)（R3，Blocked by 票 154；修法候选 (a) 判成的照常提交、未决那件入队逐包裹重试信封，(b) savepoint 不推荐；验收 `AT-NR-012` 三件各保留自己的结果），立前 ls 确认号未占。UC-NR-001 交接表「待立票」改指 code/78；完成记录未决事项 2 同改。

**未采纳**：无。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dcg155` tip `52af7bdf7` 在 main `8d68c7bab` 上快进；隔离评审无阻断，九格判定逐格对 ADR-0273 决定二核过，修订已收。
- 协调方在 tip 上单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（迁移 `network_routing/0025` 一并施加），用后删库。
- 去向：续办并入 demo-chain-gaps/154（先等 14）；docs-consistency/code/78（依赖类未决连坐，R3）。
