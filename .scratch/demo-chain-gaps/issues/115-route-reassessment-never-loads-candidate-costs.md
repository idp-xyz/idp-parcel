# 115 路由复核不取候选成本：证据里的候选成本恒空，成本单维下候选评估恒未决、无路由那条线报错

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg115（2026-10-21 作者子代理，见文末「完成记录」；进 main 时由协调方转 resolved 并注合入 SHA）。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（票 [105](./105-route-plan-receives-cost-citations-of-unselected-candidates.md) 落地时读码发现；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
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

- 复核编排 `ReassessRouteDeps` 加 `Costs`（`ports.RouteCandidateCostSource`），与初始路由同一口、同一份携带内容。候选空间收敛、要排序时才取，取到的事实填进证据再排序；只判适用性的复核不问取数侧。三条线都走同一个取数口 `candidateCosts`。读不回由各条线在调用处落格；取数侧缺席时响亮报错。落法见下面代决 1。
- 复核形成的计划（首个计划、失效线自动改路、仍适用线改善替代）带选中候选的逐段成本出处，只取 `CitationsOf(selected)`，口径同票 105；关务出处也照初始路由一并带上。
- 生产装配 `cmd/parcel-dispatch/assemble_reassess_on_handover.go`：`reassessRouteHandler` 接 `routeCosts(db, clock)`，与初始路由同一种装法；`reassessRouteHandlerOver` 多收一口成本取数侧，真库夹具给一只转交证据替身里成本事实的替身 `reassessCostsDouble`。只改了这一个特性文件，`assemble.go` 等没动。
- 审计 nr-no L15 两处注释：`AutoReroute` 那条装配注释改成现行规则（复核不读事实目录，ADR-0173 决定四）；`RequestCarriedContent` 注释改成现在带的是地理投影与承诺上界。`InitialRouteEvidence.CandidateCosts` 的注释补一句：证据视图不折它，由编排向取数侧要来再填。
- 文档：UC-NR-003 `AT-NR-039`、`AT-NR-053`、`AT-NR-084` 的缺口注改为已补，「给开发的交接」去掉复核取候选成本一行，结果契约补上候选成本读不回落哪两格、`REASSESS_COST_SOURCE_UNAVAILABLE`；评审修订时另加「复核取物理事实」「候选成本读失败不连累复核事务」两行未实现。code/16、code/17、code/29 指向本票的注改为已补；code/53 的 Status 写明本票落地后的走向。NR CONTEXT 没动：成本经端口取，属证据一族（ADR-0148 决定一），CONTEXT「重投推得动的是依赖故障……证据读不动」一句已经盖住。

### 代决

用户原话「你作为业务和系统专家自决……解决所有的问题」。

1. **「要裁的」：取数侧没接与读不回不并成一格。** 两格都不新立未判原因，也不立 ADR。（评审修订后的口径。）
   - **读不回**是依赖故障，重投推得动，与「证据读不动」同族（成本经端口取，ADR-0148 决定一）。无路由那条线答未决 `REASSESS_COST_SOURCE_UNAVAILABLE`、整笔回滚重投，它是 `ReassessUndecidedReason` 新加的一格。失效线上记候选评估未决、不挂建议，因为候选评估未决不得拦失效落库（硬句）。仍适用线上只是不试改善，原计划照旧保留，与那条线上取号故障同一处置。失效线与仍适用线这两格只在读失败不碰事务时成立，库里读失败的实际走向见越权风险点 1。
   - **没接**（装配没给取数侧，或取数侧答 `ErrRouteCostSourceNotConfigured`）三条线都响亮报错 `ErrReassessCostSourceNotConfigured`。生产装配总接着取数侧，装配期失败也由取数侧在调用时交错误、落读不回那一格，所以只有装配缺漏到得了这里。初稿把它折成候选评估未决，评审指出那是把缺一根线报成看似业务的结论，改为响亮。初始路由那一路的 `COST_SOURCE_NOT_CONFIGURED` 未决格不在本票，没动。
2. **成本只在候选空间收敛、要排序时取。** 取数侧逐段问 parcel-pricing，开销不小；它的故障也不该拦住只判适用性的复核。场外揽收仍适用、冻结已越过、自动改路没声明这几种情形都不问它。排序形态没声明时也不问：排序在比成本之前就答形态未声明。
3. **复核形成的计划一并带关务出处。** 票面只点了成本出处。初始路由的计划两样都带，复核形成的计划只带一样，审计口径就不一。这是同一处 spec 上多填一格；`FormInitialRoutePlan` 要求关务出处的候选落在已评估空间里，复核的候选空间与关务出处来自同一份证据，满足这条。
4. **code/53 不一并实现。** 理由：
   - 本票落地后，code/53 那条线在演示链上走不到。演示策略版本 `SYN-RS-CN-SG-01` 两版都没声明自动改路，`tryImprovementReroute` 在问成本之前就因无自动改路权限返回；演示网络也只有一条候选，改善恒为 0。
   - 生产接法上走得到：策略版本声明了自动改路、原计划未越过冻结边界、另有一条更便宜且改善严格大于阈值的候选。取证见下面「验证」里的临时用例：落册被 `route_reassessment_rerouted_shape` 拒，编排把写库失败答成 `REASSESS_STORE_UNAVAILABLE` 未决，回滚重投，最终到 `ABANDONED`。修前同一条线在排序处报错（`rank route candidates`），整封信封同样失败，所以不算本票引入的回退，只是失败的位置后移了。
   - code/53 有自己的「要裁的」：改善依据单立一格，还是放宽 CHECK。这要一次迁移，还要定复核册的新形状，与本票的取数缺口不是一回事。合进本票会让本票多一个落库形状的决定。
5. **AT 盘点**：`AT-NR-037`、`AT-NR-042` 早有编排用例点名（`reroute_after_lapse_test.go`），却还列在②，借本票移入①，计数不动。
6. **复核给候选出价的包裹事实暂取客户声明，是过渡做法**（评审 B1，协调方裁定保留）。复核接的成本取数侧与初始路由同一种装法，事实来源是 `nrpricing.DeclaredParcelFacts`。这与 NR CONTEXT「首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测」（ADR-0148 决定四第 7 条引同句）相抵，也与 UC-NR-003 目标与边界一节「必须重新使用已经发生的……节点实测」相抵。留着它的理由：NR 今天读不到 `node-operations` 的物理事实，NO 连实际测量登记册都没有（pp-pricing-input-seams/04）。另一种取法是没有物理事实就把成本记成待判断，照 CONTEXT 字面办；代价是复核一律排不了序，本票修的问题原样回来。改取物理事实另立 [docs-consistency/code/66](../../docs-consistency/code/66-reassessment-prices-candidates-on-the-customer-declaration-instead-of-physical-facts.md)，UC-NR-003「给开发的交接」记为未实现。

### 越权风险点

1. **成本读不回在三条线上落法不同，且后两格只在读失败不碰事务时成立**（NR owner）。写定的落法：无路由线回滚重投；失效线记候选评估未决、入账；仍适用线保留原计划、入账。实际走向（评审指出）：复核跑在收件箱那一笔事务里，取数侧的读也走这笔事务。库里读失败会让事务中毒，随后落册被拒，编排答 `REASSESS_STORE_UNAVAILABLE`，整封回滚重投，失效也一并回滚。只有不碰事务的读不回（取数侧的翻译类错误）才真落进「候选评估未决、入账」与「保留原计划」。修法是在取成本外包一层保存点，归 [docs-consistency/code/67](../../docs-consistency/code/67-a-failed-cost-read-poisons-the-reassessment-transaction.md)。没并进 code/66：两者的修法、地盘与阻塞边都不同。写定落法下，后两条线上这一次触发的改路或改善判断就此丢掉，要等下一次控制触发；owner 若要失效线也重投，就得先回答「失效已落、改路未判」那一半怎么续办，归票 154 一族。
2. **取数侧确定性错误落进重投桶**（NR owner）。取数侧的翻译类错误（认不出的评价形状、方案方向配错、候选没有线路引用）重投多少次都一样，无路由线上却答 `REASSESS_COST_SOURCE_UNAVAILABLE`、回滚重投。这是照初始路由继承来的，已在 demo-chain-gaps/155 票面加备注，归入同族。
3. **code/53 在生产接法上变得可达**（NR owner）。见代决 4。评审之后 code/53 升为 R1，并加上线闸门：code/53 落地之前，任何租户不得在路由策略版本里声明自动改路。ADR-0272 越权风险点 2 说改善替代「首发链路上通常撞不上」，所列两条原因里「复核不取候选成本」一条由本票去掉，剩下 code/53 那一条。
4. **复核排序比的成本按判断时点现取**（NR、PP owner）。成本由复核那一刻的价卡与汇率读数算，不沿用初始路由时的成本。失效线上判改善时，被比的失效计划候选成本也是现取的，不是它当初入选时的价。这符合 ADR-0148「判断时点」的口径；但若价卡在两次判断之间换了版，「改善」比的是新价与新价，计划里留痕的出处是新一次的评价。
5. **code/17 随之暴露**（NR owner）。仍适用线判出改善候选、却不满足自动条件或并列时，今天静默保留原计划、不出建议。本票让这条线算得出成本，那张票才有事可做。
6. **复核用客户声明出价，与 CONTEXT 的复核点来源相抵**（NR、NO、PS owner 复核）。见代决 6。实测与声明不符的包裹，复核比出的成本、选出的候选与按 CONTEXT 应得的不同；不会响亮失败，只能从计划出处的评价里看出来。owner 若宁可照字面办（没有物理事实即成本待判断、复核排不了序），改的是取数侧的包裹事实来源，复核编排不动。

### AT

- `AT-NR-039`、`AT-NR-053` 在 ①：去掉「生产接法上复核不取候选成本」的注，补点名编排与派发真库用例。
- `AT-NR-037`、`AT-NR-042` 由 ② 移入 ①，见代决 5。
- 总览计数没动，由协调方进 main 时重算。

### 用例

- 编排 `internal/networkrouting/application/reassess_candidate_costs_test.go`，证据一律去掉成本事实，与生产证据视图交回的一样：
  - `TestTheNoRouteLineFormsAFirstPlanFromCostsTheSourceLoads`：`AT-NR-039` 候选成本那一格。失效之后，无路由线按取数侧给的成本选出便宜的候选，形成首个计划，计划只带它的出处；取数侧收到的投影就是触发带来的那一份。
  - `TestALapsedPlanReviewsCandidatesWithCostsTheSourceLoads`：`AT-NR-042`、`AT-NR-037`。失效线候选评估判出候选可用；没声明自动改路形成建议，声明了则自动改路、计划只带选中候选的出处。
  - `TestAnImprovementRerouteComparesCostsTheSourceLoads`：仍适用线改善替代判得出。
  - `TestAnUnreadableCostSourceSettlesPerLine`：读不回在三条线上各落代决 1 写的那一格（不碰事务的读不回）。
  - `TestAMissingCostSourceIsLoudOnEveryLine`：装配没给、取数侧答没接，三条线都响亮报错。
  - `TestAnUndeclaredRankingFormNeverAsksForCosts`：排序形态没声明时不问取数侧。
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

## 评审记录（2026-10-21，隔离评审一轮，修订在分支 `claude-dcg115`，已 rebase 到 main `79a70dbba`）

阻断 1 项（披露与文档，没有代码错误），建议合入前修 4 项，顺手 3 项，都已办。code/53 分级按协调方裁定改。完成记录里相应各条已就地改成修订后的口径。

### 阻断

- **B1 复核出价用客户声明，与 CONTEXT 的复核点来源相抵，票面没记。** 协调方裁定保留「用声明」作过渡做法、不改代码路径。已补代决 6 与越权风险点 6；立 [docs-consistency/code/66](../../docs-consistency/code/66-reassessment-prices-candidates-on-the-customer-declaration-instead-of-physical-facts.md)（Blocked by pp-pricing-input-seams/04，建议 R2）；UC-NR-003「给开发的交接」加「复核取物理事实」一行，标未实现；`reassessRouteHandler` 的注释去掉「复核排序比的成本与受理后选路比的同出一处」，改为如实写过渡做法并指 code/66。

### 建议合入前修

- **成本读不回在失效线、仍适用线上的实际后果。** 库里读失败让收件箱事务中毒，最终答 `REASSESS_STORE_UNAVAILABLE`、整封回滚，失效一并回滚。越权风险点 1、UC-NR-003 结果表「替代候选评估未决」「复核未决」两格、`reviewCandidates` 与 `tryImprovementReroute` 的注释都改准。修法（保存点）另立 [docs-consistency/code/67](../../docs-consistency/code/67-a-failed-cost-read-poisons-the-reassessment-transaction.md)，没并进 code/66，理由见越权风险点 1。UC-NR-003「给开发的交接」也加了一行。
- **派发用例头注的变更叙述。** `TestAfterALapseTheDemoNetworkFormsTheFirstPlanThroughTheProductionRoute` 头注删掉「（修前恒记未决）」「（修前排序报错，整封信封失败）」。
- **`Costs` 为 nil 时静默。** 改为响亮：装配没给、取数侧答没接，三条线都交 `ErrReassessCostSourceNotConfigured`；注释写明只有装配缺漏到得了。代决 1 随之改写，原越权风险点 2「没接入账成候选评估未决」作废，换成确定性错误那一条。
- **`reviewCandidates` 注释的理由。** 改为「候选评估未决不得拦失效落库（硬句）」。

### 顺手

- **成本适配器的确定性错误落进重投桶。** 在 demo-chain-gaps/155 票面加了带日期的同族备注；本票越权风险点 2 记这一格。
- **演示 cmd 用例为什么不照抄种子价卡。** 头注写明：种子卡绑了分区目录，复核交给取数侧的输入与初始路由相同，绑目录的卡经邮编路线解区已由 `route_cost_reading_completion_wiring_test.go` 钉住。
- **排序形态未声明时不取成本。** 做了，补用例 `TestAnUndeclaredRankingFormNeverAsksForCosts`。

### code/53 分级

Status 改为 R1，加上线闸门：code/53 落地之前，任何租户不得在路由策略版本里声明自动改路。依据是评审结论：本票落地后声明了自动改路的租户会撞上，没有绕法，还会拖垮同一封信封上的其他路径；本票只是缩小了失败面，不是回归。

### 验证（修订后，rebase 到 `01babbc1b` 之后跑；随后再 rebase 到 `79a70dbba`，其间 main 只改文档、没有 Go 改动，`go build ./...` 复核干净）

- `go build ./...`、`go vet ./...` 干净；本分支相对 main 改动的 Go 文件 `gofmt -l` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`，跑同一组包：`./internal/networkrouting/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-network-register/`、`./internal/parcelshipment/adapters/networkrouting/...`、`./internal/transportfulfillment/adapters/networkrouting/...`、`./internal/visibilityexception/adapters/networkrouting/...`、`./internal/architecture/...`、`./migrations/...`。结果 21 ok、0 FAIL。
- 改动与新增的用例另跑 `-count=3`：编排七条、派发两条，ok。
- rebase 时 AT 盘点文末追记与 main 上 10a 的追记撞在同一处，两段都留，各成一段。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dcg115` tip `b18f662bc` 在 main `79a70dbba` 上快进；隔离评审 1 条阻断（复核用客户声明出价未披露）按协调方裁定以过渡做法披露并立 code/66；修订轮代码改动（取数侧缺席响亮、排序形态未声明不取成本）有用例。
- 协调方在 tip 上单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净。无迁移、无种子改动（种子不走复核取成本）。
- 去向：code/53（升 R1，上线闸门：落地前任何租户不得声明自动改路）、code/66（复核取物理事实）、code/67（失效线与仍适用线读失败的事务中毒，需保存点）。
