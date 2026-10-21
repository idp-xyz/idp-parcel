# 115 路由复核不取候选成本：证据里的候选成本恒空，成本单维下候选评估恒未决、无路由那条线报错

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg115（2026-10-21 作者子代理，见文末「完成记录」；进 main 时由协调方转 resolved 并注合入 SHA）。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（票 [105](./105-route-plan-receives-cost-citations-of-unselected-candidates.md) 落地时读码发现；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘：NR `application/reassess_route.go`（`ReassessRouteDeps`、`reviewCandidates`、`formPlanFromEvidence`）；生产装配 `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 给复核接成本取数侧。
出处：票 [routing-first-cut/10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md)（成本族在取数侧兑现）；`ports.InitialRouteEvidence` 的注释「CandidateCosts 按形态要的维度给出逐候选事实，每个合格候选恰一条」。

## 现象（读码，钉 `4cf203fb`，证据 `S`）

- 候选成本只由初始路由编排向取数侧要：`CreateInitialRouteHandler.judgeParcel` 调 `RouteCandidateCostSource.LoadCandidateCosts`，把事实填进 `evidence.CandidateCosts`。全仓非测试代码里给 `CandidateCosts` 赋值的只此一处，证据视图 `CatalogInitialRouteEvidence` 明写「成本不在这里折」。
- 复核 `ReassessRouteHandler` 的依赖里没有成本取数侧，排序直接用证据视图交回的 `evidence.CandidateCosts`，生产上恒空。
- `RankRouteCandidates` 遇到合格候选没有成本事实答 `ErrInvalidRanking`（「什么都没说是证据装配漏了」）。路由策略版本声明了成本单维排序时（演示策略 `SYN-RS-CN-SG-01` 第 2 版就声明了），复核各条线因此：
  - 失效线：`reviewCandidates` 把错吞成 `CANDIDATE_REVIEW_UNDECIDED`，候选可用时才做的 7B/7C 改路评估整段不做——计划失效后从不形成改路建议或自动改路。
  - 原先无路由那条线（`reassessNoRoute`）与改善改路（`tryImprovementReroute`）：`formPlanFromEvidence` 上抛 `rank route candidates`，整次复核报错，而不是形成首个计划或记未决。
- 复核形成的新计划（首个计划、自动改路）不带逐段成本出处：`formPlanFromEvidence` 不交 `CostCitations`。初始路由的计划带（票 routing-first-cut/10）。

没人看见，是因为没人断言：交接复核的真库用例走到失效那一格时，按上面的路径候选评估应落未决，用例只断言失效本身；无路由那条线没有真库用例。

## 三类

机制。规模 S–M：复核接成本取数侧（与初始路由同一口、同一份携带内容），排序前填 `CandidateCosts`，形成计划时按票 105 的口径只取选中候选的出处；取数侧没接或读不回各归一个未决格，不吞成「候选评估未决」以外的东西。

## 要裁的

- 复核的未决原因集合要不要分出「成本取数侧没接 / 读不回」两格（初始路由有 `COST_SOURCE_NOT_CONFIGURED` 与 `COST_SOURCE_UNAVAILABLE`），还是在复核这一侧并入候选评估未决。按「恢复动作不同就分格」的既有口径应分，归 NR owner 确认。

## 既有票

routing-first-cut/10（候选成本取数侧）、demo-chain-gaps/105（计划只取选中候选的出处）、demo-chain-gaps/63（交接触发复核）。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/nr-no.md](../../docs-consistency/audit/nr-no.md) H3（归 C，高）与本票同一现象，落在本票，不另立。按 main `67438a0f` 复核，现象仍在：`application.ReassessRouteDeps` 仍没有成本取数口，生产证据视图 `LoadInitialRouteEvidence` 不填 `CandidateCosts`，`domain.RankRouteCandidates` 遇合格候选无成本事实答 `ErrInvalidRanking`。审计补的两点：

- 三条路的后果各不相同：`reassessNoRoute` 上抛、整封重投到 `ABANDONED`；`reviewCandidates` 吞成 `CANDIDATE_REVIEW_UNDECIDED`，失效之后永不形成改路决定或建议；`tryImprovementReroute` 在声明了自动改路时同样上抛。演示种子 `scripts/demo-seeds/data/network/23-route-strategy-cn-sg-v2.json` 声明了 `COST_SINGLE_DIMENSION`，所以今天就走得到。ADR-0235 越权风险点 12 写「今天两种都到不了」，这是第三种。
- 应用测试用 `routableEvidence`、`cheaperAlternative` 直接往证据里塞 `CandidateCosts`，所以是绿的；本票须补一条 cmd 层用真证据跑「先前无路由 → 首个计划」（`AT-NR-039`）的用例。

同笔并入 [audit/nr-no.md](../../docs-consistency/audit/nr-no.md) L15（两处 Go 注释与 ADR 相反或过时，本票地盘内）：
- `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 给 `AutoReroute` 的注释「登记过的键才走 7B/7C」，与 ADR-0173 决定四、NR CONTEXT「自动改路事实目录不再是判断权威，复核不读它」及 `ReassessRouteDeps.AutoReroute` 字段注释相反；
- `internal/networkrouting/ports/ports.go` 的 `RequestCarriedContent` 注释「首版只含地理解析投影；服务要求与承诺上界随 routing-first-cut/08、09 加入」，结构体已有 `Commitment`。
两处改成中文写现行规则，不留变更叙述。

分级建议（文档自洽 02）：R1——CONTEXT 写的首个计划、受控改路、改路建议在生产接法上都走不通，失效之后一律报「未决」误导运营，无绕法。由协调方定稿。修好之后 [docs-consistency/code/17](../../docs-consistency/code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md)（仍适用线不出改路建议）才会暴露。

## 完成记录（2026-10-21，作者子代理，分支 `claude-dcg115`，基于 main `37f9d7747`）

### 做了什么

- 复核编排 `ReassessRouteDeps` 加 `Costs`（`ports.RouteCandidateCostSource`），与初始路由同一口、同一份携带内容。候选空间收敛、要排序时才取，取到的事实填进证据再排序；只判适用性的复核不问取数侧。三条线都走同一个取数口 `candidateCosts`，各自在调用处落取数侧没接、读不回两格，落法见下面代决 1。
- 复核形成的计划（首个计划、失效线自动改路、仍适用线改善替代）带选中候选的逐段成本出处，只取 `CitationsOf(selected)`，口径同票 105；关务出处也照初始路由一并带上。
- 生产装配 `cmd/parcel-dispatch/assemble_reassess_on_handover.go`：`reassessRouteHandler` 接 `routeCosts(db, clock)`，与初始路由同一种装法；`reassessRouteHandlerOver` 多收一口成本取数侧，真库夹具给一只转交证据替身里成本事实的替身 `reassessCostsDouble`。只改了这一个特性文件，`assemble.go` 等没动。
- 审计 nr-no L15 两处注释：`AutoReroute` 那条装配注释改成现行规则（复核不读事实目录，ADR-0173 决定四）；`RequestCarriedContent` 注释改成现在带的是地理投影与承诺上界。`InitialRouteEvidence.CandidateCosts` 的注释补一句：证据视图不折它，由编排向取数侧要来再填。
- 文档：UC-NR-003 `AT-NR-039`、`AT-NR-053`、`AT-NR-084` 的缺口注改为已补，「给开发的交接」去掉复核取候选成本一行，结果契约补上候选成本取不到落哪两格、`REASSESS_COST_SOURCE_UNAVAILABLE`。code/16、code/17、code/29 指向本票的注改为已补；code/53 的 Status 写明本票落地后的走向。NR CONTEXT 没动：成本经端口取，属证据一族（ADR-0148 决定一），CONTEXT「重投推得动的是依赖故障……证据读不动」一句已经盖住。

### 代决

用户原话「你作为业务和系统专家自决……解决所有的问题」。

1. **「要裁的」：取数侧没接与读不回分两格，按恢复动作分。** 两格都不新立未判原因，也不立 ADR，落进既有两族：
   - **读不回**是依赖故障，重投推得动，与「证据读不动」同族（成本经端口取，ADR-0148 决定一）。无路由那条线答未决 `REASSESS_COST_SOURCE_UNAVAILABLE`、整笔回滚重投，它是 `ReassessUndecidedReason` 新加的一格。
   - **没接**（`ErrRouteCostSourceNotConfigured`，或装配没给取数侧）重投推不动。无路由那条线照常入账：仍无当前有效路由，候选评估记未决。没有落`触发未判`：ADR-0235 决定一列明了未判的几格，ADR-0250 加「关务来源未接」时是另立 ADR 部分停用的；不立新 ADR 就不扩那张名单。ADR-0148 决定四第 4、5 条说缺成本依据是证据未齐、结果格由排序形态定，无路由线上「成本待判断」本来就落候选评估未决，没接与之同格。
   - **失效线**上两格都记候选评估未决，不挂建议。失效已经落了适用性，不能回滚重投（「候选评估未决拦不住失效落库」）。
   - **仍适用线**上两格都只是不试改善，原计划照旧保留，与那条线上取号故障同一处置。
2. **成本只在候选空间收敛、要排序时取。** 取数侧逐段问 parcel-pricing，开销不小；它的故障也不该拦住只判适用性的复核。场外揽收仍适用、冻结已越过、自动改路没声明这几种情形都不问它。
3. **复核形成的计划一并带关务出处。** 票面只点了成本出处。初始路由的计划两样都带，复核形成的计划只带一样，审计口径就不一。这是同一处 spec 上多填一格；`FormInitialRoutePlan` 要求关务出处的候选落在已评估空间里，复核的候选空间与关务出处来自同一份证据，满足这条。
4. **code/53 不一并实现。** 理由：
   - 本票落地后，code/53 那条线在演示链上走不到。演示策略版本 `SYN-RS-CN-SG-01` 两版都没声明自动改路，`tryImprovementReroute` 在问成本之前就因无自动改路权限返回；演示网络也只有一条候选，改善恒为 0。
   - 生产接法上走得到：策略版本声明了自动改路、原计划未越过冻结边界、另有一条更便宜且改善严格大于阈值的候选。取证见下面「验证」里的临时用例：落册被 `route_reassessment_rerouted_shape` 拒，编排把写库失败答成 `REASSESS_STORE_UNAVAILABLE` 未决，回滚重投，最终到 `ABANDONED`。修前同一条线在排序处报错（`rank route candidates`），整封信封同样失败，所以不算本票引入的回退，只是失败的位置后移了。
   - code/53 有自己的「要裁的」：改善依据单立一格，还是放宽 CHECK。这要一次迁移，还要定复核册的新形状，与本票的取数缺口不是一回事。合进本票会让本票多一个落库形状的决定。
5. **AT 盘点**：`AT-NR-037`、`AT-NR-042` 早有编排用例点名（`reroute_after_lapse_test.go`），却还列在②，借本票移入①，计数不动。

### 越权风险点

1. **成本读不回在三条线上落法不同**（NR owner）。无路由线回滚重投；失效线记候选评估未决、入账；仍适用线保留原计划、入账。后两条线上，这一次触发的改路或改善判断就此丢掉，要等下一次控制触发再判。这与取号故障的既有处置一致。owner 若要失效线也重投，就得先回答「失效已落、改路未判」那一半怎么续办，归票 154 一族。
2. **取数侧没接不入账未判，入账成「候选评估未决」**（NR owner）。运维在复核册上分不出「成本没接」与「成本待判断、不可计价、币种不齐」，都只看到候选评估未决。生产装配总接着取数侧，没接只出现在装配缺漏里；装配期失败由 `brokenCostSource` 交错误，落「读不回」那一格。owner 若要它进未判登记，须扩 ADR-0235 决定一的名单。
3. **code/53 在生产接法上变得可达**（NR owner）。见代决 4。凡声明了自动改路、又有两条以上候选的租户，上线前必须先修 code/53。ADR-0272 越权风险点 2 说改善替代「首发链路上通常撞不上」，所列两条原因里「复核不取候选成本」一条由本票去掉，剩下 code/53 那一条。
4. **复核排序比的成本按判断时点现取**（NR、PP owner）。成本由复核那一刻的价卡与汇率读数算，不沿用初始路由时的成本。失效线上判改善时，被比的失效计划候选成本也是现取的，不是它当初入选时的价。这符合 ADR-0148「判断时点」的口径；但若价卡在两次判断之间换了版，「改善」比的是新价与新价，计划里留痕的出处是新一次的评价。
5. **code/17 随之暴露**（NR owner）。仍适用线判出改善候选、却不满足自动条件或并列时，今天静默保留原计划、不出建议。本票让这条线算得出成本，那张票才有事可做。

### AT

- `AT-NR-039`、`AT-NR-053` 在 ①：去掉「生产接法上复核不取候选成本」的注，补点名编排与派发真库用例。
- `AT-NR-037`、`AT-NR-042` 由 ② 移入 ①，见代决 5。
- 总览计数没动，由协调方进 main 时重算。

### 用例

- 编排 `internal/networkrouting/application/reassess_candidate_costs_test.go`，证据一律去掉成本事实，与生产证据视图交回的一样：
  - `TestTheNoRouteLineFormsAFirstPlanFromCostsTheSourceLoads`：`AT-NR-039` 候选成本那一格。失效之后，无路由线按取数侧给的成本选出便宜的候选，形成首个计划，计划只带它的出处；取数侧收到的投影就是触发带来的那一份。
  - `TestALapsedPlanReviewsCandidatesWithCostsTheSourceLoads`：`AT-NR-042`、`AT-NR-037`。失效线候选评估判出候选可用；没声明自动改路形成建议，声明了则自动改路、计划只带选中候选的出处。
  - `TestAnImprovementRerouteComparesCostsTheSourceLoads`：仍适用线改善替代判得出。
  - `TestCostSourceGapsSettlePerLine`：三条线乘三种缺口（读不回、没接、没装配），各落代决 1 写的那一格。
  - 先跑红：加了字段与常量、编排未改时，前三条与无路由线、仍适用线报 `rank route candidates: network routing: invalid ranking facts`，失效线候选评估恒为 `CANDIDATE_REVIEW_UNDECIDED`，与票面现象一致。
- 派发真库 `cmd/parcel-dispatch/reassess_candidate_costs_test.go`：
  - `TestAfterALapseTheCheaperCandidateFormsTheFirstPlanOnTheDatabase`：在 `reassessBothLanesFixture` 上，两条候选的成本由取数侧替身给。交进吉隆坡枢纽，原计划失效并形成建议；交进新加坡枢纽，按成本选出便宜的那条，形成首个计划，计划只带它三段出处，经复核册读回；再一次交接时，以那份新计划为起点判仍适用。
  - `TestAfterALapseTheDemoNetworkFormsTheFirstPlanThroughTheProductionRoute`：演示网络加一张与演示成本卡同身份的 CNY 卡，委托带客户声明 2.5 KG，经生产路由表一拍，复核编排、证据视图与成本取数侧全用生产接法。失效线判出候选可用，随后形成首个计划，三段出处都指 `SYN-PLAN-CN-SG-COST-01/v1`、评价标识 `EST-…`。临时把生产装配的取数侧换成 nil 时，这条用例红在第一步（候选评估 `CANDIDATE_REVIEW_UNDECIDED`），证明它钉的是接线。
- 既有夹具改动：编排夹具 `newReassessFixture` 带一只默认转交证据成本的替身；收寄采用适配器的揽收用例补 `costPassthrough{}`；派发两只夹具经 `reassessRouteHandlerOver` 多给一口取数侧。断言都没改。

### 验证

- `go build ./...`、`go vet ./...` 干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`，跑的包：`./internal/networkrouting/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-network-register/`、`./internal/parcelshipment/adapters/networkrouting/...`、`./internal/transportfulfillment/adapters/networkrouting/...`、`./internal/visibilityexception/adapters/networkrouting/...`、`./internal/architecture/...`、`./migrations/...`。结果 21 ok、0 FAIL。`internal/networkrouting/application` 的反向依赖用 `go list` 算过，都在这组里。
- 新用例另跑 `-count=3`（编排四条、派发两条）：ok。
- code/53 可达性的取证：在 `reassessBothLanesFixture` 上临时写一条用例，声明自动改路、阈值 0，取数侧给两条候选 4200 与 3100，原计划 4200、交进新加坡枢纽、冻结未越过。投递答 `REASSESS_STORE_UNAVAILABLE` 未决，即复核册 CHECK 拒落改善改路那一行。用例跑完即删，没有提交。
- 没有迁移；演示种子没改，所以没在一次性库上重灌（种子只登数据，本票不改种子，也不改登记口）。没跑全量，没重生成 MECHANISM-INVENTORY、ADR 索引与 AT 计数。
