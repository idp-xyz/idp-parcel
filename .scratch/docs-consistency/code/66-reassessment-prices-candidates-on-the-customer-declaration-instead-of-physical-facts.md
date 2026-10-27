# 66 NR 复核给候选出价用的是客户声明，不是收寄时已取得的物理事实或当前有效实测

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code66。**NR 侧完工；物理事实的真实来源仍缺**（`node-operations` 没有实际测量登记册，票 pp-pricing-input-seams/04），生产装配里复核恒退回客户声明并如实标明，登记册与只读口立起来后接线这一处，接线前本票不关。此前 ready-for-agent——建议 R2（上线后尽快）：答案不错在结构上，错在依据。复核的排序与改善判断建立在客户声明上，实测与声明不符的包裹，复核比出的成本、选出的候选与按 CONTEXT 应得的不同；没有响亮失败，审计时从计划出处才看得出。2026-10-21 协调方裁定立（票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 隔离评审 B1；用户「你作为业务和系统专家自决……解决所有的问题」授权），115 作者子代理执笔，分支 claude-dcg115。
Blocked by: [pp-pricing-input-seams/04](../../pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md)（`node-operations` 实际测量登记册与「仍有效的实际测量」只读口；没有它，到站复核那一半无从取数）
出处：demo-chain-gaps/115 隔离评审 B1。

## 现象

票 115 让复核向候选成本取数侧取成本。取数侧与初始路由是同一种装法，包裹事实一律取客户声明。NR CONTEXT 写的是按复核点分来源：

> 预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测。

ADR-0148 决定四第 7 条引同一句；UC-NR-003 目标与边界一节写「本用例必须重新使用已经发生的接货位置、控制依据和节点实测」。复核今天用的是预路由那一格的来源。

115 把它留作过渡做法（见 115 完成记录的评审修订代决），没有改成「没有物理事实即成本待判断」：那样复核一律排不了序，115 修的问题原样回来。

## 证据（读码，钉 `378b9b2dc`，证据 `S`）

- `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 的 `reassessRouteHandler` 接 `routeCosts(db, clock)`；`routeCosts`（`assemble.go`）的包裹事实取 `routeParcelFacts`，即 `nrpricing.NewDeclaredParcelFacts`。
- `internal/networkrouting/adapters/parcelpricing/declared_parcel_facts.go`：`DeclaredParcelFacts` 按判断键问 parcel-shipment 的客户声明，头注写明对应 CONTEXT「预路由使用客户声明快照」。
- 取数侧端口 `ParcelFactsSource.ParcelFactsFor(ctx, key)` 只收判断键，不知道这是哪一种判断（预路由、收寄复核、到站复核），也就选不了来源。
- `git grep -n -i -E 'measure|weight|dimension' -- internal/nodeoperations/ports/ internal/nodeoperations/domain/ migrations/node_operations/` 零命中：`node-operations` 没有实际测量登记册，NR 读不到实测（pp-pricing-input-seams/04）。
- 收寄采用记录（PS）与权威运输交接（TF）随复核触发进来，都不带重量或尺寸。

## 三类

机制。「哪个复核点用哪种物理事实」是 CONTEXT 已定的规则，不是租户取值。物理事实本身是作业事实，租户上线时由节点实测产生。

## 规模

M。NR 侧改取数侧的事实来源选择、复核编排把判断种类交给取数侧；NO 侧只加 ports 与只读查询（登记册本身归 pp-pricing-input-seams/04）；PS 侧若收寄采用要带「当时已取得的物理事实」，另议。

## 最小的诚实修法

1. 取数侧的包裹事实口带上判断种类：预路由、收寄复核、到站复核。初始路由照旧取客户声明。
2. 收寄复核取收寄时已取得的物理事实，到站复核取 `node-operations`「仍有效的实际测量」，都经只读口；事实引用标明来源，随出处留痕。
3. 该取的物理事实没有时怎么落，要裁：
   - 退回客户声明，并在出处与复核记录里标明退回（可审计，排序照常）；
   - 或该候选成本待判断（照 CONTEXT 字面，复核排不了序，候选评估记未决）。
   归 NR owner，NO、PS owner 会签。
4. 用例：收寄复核与到站复核各一条，实测与声明不符时，复核按实测出价、选中的候选随之不同。

## 相关

- demo-chain-gaps/115：引入复核取成本、留下本票这一格。
- pp-pricing-input-seams/04：NO 实际测量登记册。
- UC-NR-003「给开发的交接」：「复核取物理事实」一行指本票。

## 完成记录（2026-10-26，作者，分支 claude-code66，起点 `7457d23e9`）

### 提交

- `7450ecd58` feat(network-routing)：判断点与依据领域型、`RouteCandidateCostSource` 取成本口加判断点、取数侧按判断点合成两路来源、复核记录与成本出处留依据、迁移 `network_routing/0032_route_reassessment_candidate_facts_basis.sql`、全部用例（含 `cmd/parcel-dispatch` 真库）。
- `a87b6961d` docs(adr)：ADR-0294（取预留号），ADR-0148 的 Status 注记与 Links，NR CONTEXT 一条，UC-NR-003（`AT-NR-106`–`110`、「给开发的交接」一行），AT 盘点分桶行与追记。
- 其后一笔 test(parcel-dispatch)：真库判断点用例改按各点核物理事实截至时刻（-count=3 下暴露的 map 遍历次序依赖）。
- 最后一笔：票面完成记录。

### 做了什么

| 层 | 改动 |
|---|---|
| 领域 | `ParcelFactsPoint`（预路由、首次收寄复核、到站复核、授权改路决定；零值不合法）、`ParcelFactsBasis`（`DECLARATION`、`PHYSICAL`、`PHYSICAL_WITH_DECLARATION`、`DECLARATION_FALLBACK`、`PHYSICAL_UNRESOLVED`、`NONE`）；`ReassessmentTrigger.ParcelFactsPoint()` 由控制依据定判断点；成本出处 `PlannedLegCostCitation` 必带出了价的依据 |
| 端口 | `RouteCandidateCostSource.LoadCandidateCosts` 多收判断点；`RouteCandidateCosts.FactsBasis`、`ReassessmentRecord.CandidateFactsBasis` |
| 编排 | 初始路由与并列裁决指名预路由，授权改路决定指名决定点，复核按触发定点；复核三条线（失效、仍适用含没切换的、无路由）各把依据记在复核记录上；仍适用线原先没切换时提交传入的记录，现提交带依据的那一份（语义不变） |
| 取数侧适配器 | `ParcelFactsSource` 多收判断点；`PointedParcelFacts` 合成客户声明与物理事实两路；`PhysicalParcelFactsSource` 端口（一项都没取得、取得了、在而定不下）与「没有来源」的 `NoPhysicalParcelFacts`；`DeclaredParcelFacts` 改为只答声明（方法改名 `DeclaredFactsFor`） |
| 库 | 迁移 `0032`：`candidate_facts_basis` 列与封闭集 CHECK（不含 `DECLARATION`）；成本出处 JSON 多一个可缺的 `facts_basis`，缺席读回为客户声明 |
| 装配 | `cmd/parcel-dispatch` `routeCosts` / `routeCostsOver`（真库用例交入预置实测的来源）、`cmd/parcel-api` `rerouteCosts`：物理事实来源装 `NoPhysicalParcelFacts` |
| PP 域 | 一个常量 `ArtifactActualMeasurement`（事实引用种类，与 `ArtifactDeclaredMeasurement` 成对） |
| HTTP、管理台、读面 | 无落点：没动（越权风险点 4）；`apps/admin-web` 未碰 |
| 取号 | NR 迁移 main 末号 0031，在途分支无 0032 以上，取 0032；ADR 取预留 0294；AT 取 `AT-NR-106`–`110`（全仓最大为 105）。没有新写者，不涉事务外拒跑与锁序 |

### 代决（详见 ADR-0294）

1. **物理事实一项没取得：退回客户声明并标明，不是待判断。** 票面两条路里选前者。理由：「没称过」是收寄当时的常态、场外揽收必然；一律待判断则复核一律排不了序，115 修的问题原样回来。成本比较本来就是只为排候选的试算，退回出的是一笔标明来源的估价；错的是不说。标 `DECLARATION_FALLBACK`，与预路由的 `DECLARATION` 分开，审计分得出。
2. **读物理事实读不回不是「没取得」。** 原样上抛，按既有依赖故障落法处置；折成「没取得」会在依赖故障时悄悄按声明出价。
3. **物理事实在而定不下用哪条（多于一条而没有已登记的取用规则）：不退回，各段待判断。** 票面没列的第三格。客户声明不得顶替实测；机制不替租户挑「最近一次」（pp-seams/04 形第 4 条）。确定性缺口，落候选成本既有的待判断，不进未决哨兵表。
4. **只取得一部分逐项取，声明只补缺的项。** 只称了重、没量外廓时，体积重用声明外廓算。
5. **判断点由触发的控制依据定。** 节点收寄与场外揽收同属首次收寄复核（CONTEXT「有效网络收寄」那一条把二者放在一起），权威运输交接进接收节点是到站复核。
6. **时点：首次收寄复核截到收寄的业务时间，到站复核取判断此刻。** 前者是「当时已经取得」的字面，且重放不变；后者是「当前有效」。
7. **授权角色据建议形成的改路决定是第四个判断点，与到站复核同取判断此刻。** CONTEXT 只列三个；决定由操作者此刻发起，新计划从决定时刻生效，按此刻事实出价才对得上。初始路由的最低成本并列裁决仍属预路由（初始路由的延续）。
8. **依据落两处：逐段成本出处与复核记录。** 复核不一定形成计划，不形成计划的复核不留成本，只有复核记录上这一格能查出候选评估是按声明还是按实测判的。没向取数侧要过成本的复核不记。
9. **生产装配没有物理事实来源，如实装「一项都没取得」。** 价与此前相同，区别是出处与复核记录标明；不假装读过一遍。
10. 本实现立 ADR-0294：把 ADR-0148 决定四第 7 条「包裹事实按 CONTEXT……取」接上了缺席与取用不定的落法，也给授权改路决定指名了判断点，属部分停用（接上）。

### 越权风险点（详见 ADR-0294 文末）

1. 退回声明而不是待判断（NR owner，NO、PS owner 会签）：声明严重失真的包裹，复核据以失效、改路或形成首个计划。要宁缺毋滥，改一处。
2. 授权改路决定取判断此刻的实测，是 CONTEXT 没列的第四点。
3. 逐项取把声明与实测混在一份输入里。
4. 依据只落库，复核册读面与管理台不展示。
5. 「截到收寄的业务时间」「判断此刻仍有效」的口径取决于 NO 登记册怎样记「仍有效」（pp-seams/04 待裁 1）。
6. **本票没有把真实来源接上。** NO 实际测量登记册、「仍有效的实际测量」只读口与消费侧适配器（实现 `PhysicalParcelFactsSource`）都不在本票：登记册归 pp-pricing-input-seams/04，其「待裁」四条与登记入口未裁。接线只动 `cmd/parcel-dispatch` `routeCosts` 与 `cmd/parcel-api` `rerouteCosts` 两处装配。接线之前，生产里复核的出价与此前相同，只多了如实的标明。
7. PS 收寄采用记录与 TF 揽收结果不带物理事实，首次收寄复核能取到的只可能来自 NO 在收寄那一刻登记的实测；PS 侧要不要带（票面「另议」）没做。
8. 到站复核与授权改路决定取判断此刻，回滚重投前后可能出不同的价（只影响未提交的那一次）。
9. 真实实测证明的缺口：`AT-NR-106` 的「按实测出价」在 `cmd/parcel-dispatch` 真库里由替身来源预置实测证明（生产装配的来源恒答没取得）。

### 判据对用例

| 判据 | 用例 |
|---|---|
| 判断点由控制依据定；各取成本调用点指名自己的判断点 | 领域 `TestAReassessmentTriggerNamesItsParcelFactsPoint`；应用 `TestEachControlEvidenceNamesItsOwnJudgmentPointToTheCostSource`、`TestTheCarriedProjectionReachesTheCostSource`、`TestAnAuthorizedRoleDecidesALowestCostTie`、`TestAnAuthorizedRoleDecidesACurrentSuggestion` |
| 票面判据 4：声明与实测不符，按实测出价，选中的候选随之不同 | 适配器 `TestTheJudgmentPointChangesWhichCandidateIsCheapest`（声明 1kg 时 light 最便宜，实测 12kg 时 flat 最便宜，首次收寄、到站、授权决定三点都翻）；真库 `TestTheJudgmentPointDecidesWhetherTheDeclarationOrTheMeasurementPricesTheCandidate`（25500 → 78000，出处标 `PHYSICAL`） |
| 首次收寄截到收寄的业务时间，其余取判断此刻 | 适配器 `TestPhysicalFactsAreReadAsOfTheMomentTheJudgmentPointNames`；真库同上 |
| 没取得：退回并标明；读不回上抛 | `TestWhenNoPhysicalFactWasObtainedTheDeclarationStandsInAndSaysSo`、`TestAReadFailureIsNeverTurnedIntoAFallback`、`TestAFallbackPricesLikeTheDeclarationAndSaysSo`；真库 `TestWithoutAPhysicalFactSourceTheProductionWiringFallsBackToTheDeclarationAndSaysSo`；整条生产路由 `TestAfterALapseTheDemoNetworkFormsTheFirstPlanThroughTheProductionRoute` 读回复核册与计划出处 `DECLARATION_FALLBACK` |
| 定不下：不退回 | `TestUnresolvedPhysicalFactsAreNotReplacedByTheDeclaration`、`TestUnresolvedPhysicalFactsLeaveEveryCandidatePendingWithoutCitations`、真库 `TestUnresolvedPhysicalFactsAreNotPricedOnTheDeclarationInProductionWiring` |
| 只取得一部分：逐项取 | `TestADeclarationOnlyFillsTheItemsThePhysicalFactsLack` |
| 依据留痕与读回 | 应用 `TestTheCandidateFactsBasisStaysOnEveryRecordThatPricedCandidates`、`TestARecordThatNeverPricedCandidatesCarriesNoBasis`；postgres 四条 `route_reassessment_candidate_facts_basis_test.go`；取数侧自报依据与有无事实不符 `TestAFactsSourceThatMisreportsItsBasisIsUnusable` |

### 测试与变异

- 用例写在实现编译之后（取成本口签名一改全仓测试替身随之改，先红的是编译），敏感性靠变异证：下列 17 处变异全部被杀，每处用私有目录备份、改后跑、拷回并 `cmp` 核一致。物理事实读不回折成退回；定不下时退回声明；声明顶替已有重量；并用不标；退回不标；首次收寄截至改成判断此刻；交接触发改成预路由点；授权决定改指预路由；并列裁决改指到站；无路由线丢依据；仍适用线提交原记录；失效线丢依据；删依据自洽核对；成本出处依据写死；保存丢依据；CHECK 放进 `DECLARATION`；旧出处读回成退回。
- 全量 `scripts/test-full.sh`：改动包 + 反向依赖（`go list -test` 算出，含 `cmd/parcel-api`、`cmd/parcel-dispatch` 与各 `cmd` CLI、`internal/parcelpricing/...`、`internal/settlementaccounting/adapters/...` 等）+ `./internal/architecture/...` + `./migrations/...` + `./scripts/demo-seeds/...` 全绿；新用例 `-count=3`，其中真库判断点用例首跑暴露出自己的 map 遍历次序依赖（原写法取第一次被问的时刻当首次收寄的），已改并 `-count=5` 过；种子在一次性库灌入通过（迁移 0032 随之落库）。`go build ./... && go vet ./...`、`gofmt -l` 无输出；`go run ./tools/adr-index`（生成成功，README 不提交）、`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，分桶行齐）。

### 评审记录与修订轮

- 独立评审结论：可合入。评审对实现做了三处变异，三处都存活，列为 N1–N3，均是测试缺口，代码无改动，下列用例补上，补后各自对变异变红（备份、改后跑、拷回并 `cmp` 核一致）：
  - N1 `cmd/parcel-api` 的 `rerouteCosts` 物理事实来源接线没有用例（来源改成空，包内全绿）：补真库用例 `TestTheRerouteDecisionCostSourceFallsBackToTheDeclarationBecauseProductionHasNoPhysicalFactSource`，委托已接受且带客户声明，授权改路决定的判断点经生产装配取成本，断言取数侧不是装配失败的那一份、成本依据标 `DECLARATION_FALLBACK`。来源改成空：装配当场失败，用例红。
  - N2 「声明读不回上抛」只测了物理事实没取得那一支：`TestAReadFailureIsNeverTurnedIntoAFallback` 加一格，物理事实只有重量、补外廓的声明读不回，断言上抛（ADR-0029：读不动是未决，不静默按「只有实测重量、无外廓」出价）。补外廓那一读的错误吞成声明没有：用例红。
  - N3 savepoint 装饰器的无事务一支没有断言判断点透传：`TestASavepointPassesCostsThroughOutsideATransaction` 断言内层收到的就是调用方指名的判断点（种类与业务时点都核）；事务里的可读一支 `TestASavepointReleasesReadableCostsUnchanged` 同样加了断言。无事务一支与事务里调内层的两处各自换成预路由点：对应用例各自红。
- 信息项，不另立票：复核册读面（`query_route_plans.go` 的 `reassessmentBody`）不透出 `candidate_facts_basis`，已在越权风险点 4 列明。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code66` 由修订代理 rebase 到 origin/main `6092075c1` 后快进，tip `c07f2a4c0`。迁移 `network_routing/0032_route_reassessment_candidate_facts_basis`（加 `candidate_facts_basis` 列与封闭集 CHECK，不含预路由的 `DECLARATION`）；新立 ADR-0294（接上 ADR-0148 决定四第 7 条一句，入向注记与 Links 已挂 ADR-0148）。AT-NR-106–110。
- 隔离评审（Sonnet）可合入，无阻断；核实了 ADR-0294 的出向/入向指针与被停原句逐字相同、ADR-0148 正文未改、四种依据值在所有构造与读回处有落点、读不动上抛而没取得才退回声明（ADR-0029 分格对）、迁移与种子、ADR-0222 装配合规、手算两例（称重 12kg 外廓取声明；只量外廓重量取声明 1kg）符合决定五。评审自做 16 处变异，13 处被杀；存活的三处（N1 `rerouteCosts` 生产装配无用例、N2 声明读不回只测了物理没取得一支、N3 savepoint 无事务分支未断言判断点透传）由 Sonnet 修订代理补用例，三处变异均变红。
- 协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；种子灌一次性库退 0，`candidate_facts_basis` 列已落库。无 admin-web 改动。
- 去向：无新票。读面（`query_route_plans.go` 的 `reassessmentBody`）不透出 `candidate_facts_basis`，运营分不出按声明退回还是按实测判，已列 ADR-0294 越权风险点，随接上真实物理来源的那张票一并做。越权风险点：生产里物理事实来源装 `NoPhysicalParcelFacts`，复核恒退回声明并标 `DECLARATION_FALLBACK`，出价与此前相同；NO 实测登记册（pp-pricing-input-seams/04）立起来后只需换接 `routeCosts` 与 `rerouteCosts` 两处装配；授权改路决定作第四判断点、口径取决于 NO 登记册怎样记「仍有效」。
