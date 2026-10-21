# 53 NR 仍适用线上的改善改路落 `REROUTED` 复核记录时不带失效依据，过不了 `route_reassessment_rerouted_shape`

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code53，见文末「完成记录」；上线闸门随本票解除（见完成记录）。此前 ready-for-agent——R1（上线前必修；2026-10-21 由 R2 升级，协调方据 demo-chain-gaps/115 隔离评审结论定）：仍适用线上一旦按改善阈值自动改路，复核册那一行必然被 CHECK 拒，整笔回滚。**上线闸门：本票落地之前，任何租户不得在路由策略版本里声明自动改路。** 理由：115 落地后，声明了自动改路的租户会撞上这一格，没有绕法，整封信封回滚重投到 `ABANDONED`，还拖住同一封信封上扇出的其他各路；115 本身只是把失败的位置从排序后移到落册，失败面缩小了，不是回归。复核不取候选成本那一格已由票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 补齐（2026-10-21），这条线随之在生产接法上走得到：策略版本声明了自动改路、原计划未越过冻结边界、另有一条更便宜且改善严格大于阈值的候选时，落册被 CHECK 拒，复核编排把写库失败答成 `REASSESS_STORE_UNAVAILABLE` 未决，回滚重投到 `ABANDONED`。演示链走不到：演示策略版本没声明自动改路，演示网络只有一条候选。115 不一并修本票的理由见其完成记录。2026-10-21 协调方立（code/16 作者读码所得，见 [code/52](./52-reassessment-reads-the-initial-route-row-instead-of-the-current-plan-or-no-route.md)「相关」一节；协调方读码核实）。
Blocked by: 无
出处：code/16 修订时读码所得，不在审计报告条目里。

## 现象

仍适用线上的自动改路（`tryImprovementReroute`）形成的复核记录答 `REROUTED`，带新计划与改路决定，但不带失效依据。迁移 `0004_candidate_state_and_rerouted_shape.sql` 立的 `route_reassessment_rerouted_shape` 要求 `REROUTED` 一行 `lapse_basis IS NOT NULL`——那一行落不了库。

## 证据

- `internal/networkrouting/application/reassess_route.go`：`tryImprovementReroute` 末尾只设 `record.Conclusion = ports.ReassessmentRerouted`、`RerouteState`、`NewPlan`、`Decision`，不设失效依据。
- `migrations/network_routing/0004_candidate_state_and_rerouted_shape.sql`：`CHECK (conclusion <> 'REROUTED' OR (reviewed_plan IS NOT NULL AND lapse_basis IS NOT NULL AND new_plan IS NOT NULL AND decision IS NOT NULL …))`。
- 这条 CHECK 与「`REROUTED` 只出自失效线」的旧假设一致；ADR-0173 起仍适用线也能改路，两者没对齐。

## 三类

机制。

## 规模

S–M：要裁的是「改善改路的 `REROUTED` 用什么作依据」——另立一格依据（例如「改善超过阈值」，带被比的两个成本与阈值），还是放宽 CHECK 让 `REROUTED` 在改善线上可不带失效依据而必带改善依据。无论哪种都要一次迁移（与 code/28、code/52 撞号风险：后进 main 的改号）。补真库用例：仍适用线上改善改路落得了库。

## 最小的诚实修法

1. 复核记录加「改善依据」一格（被替代计划成本、新计划成本、所用阈值），`REROUTED` 的 CHECK 改为「失效依据或改善依据恰有其一」。
2. `tryImprovementReroute` 填改善依据。
3. 真库用例：冻结未越过、改善超过阈值时落一行 `REROUTED`、带改善依据、新计划与决定。

## 完成记录（2026-10-21，作者子代理，分支 `claude-code53`，基于 main `107310894`）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 NR owner 作出，待 NR owner 复核。照票面「最小的诚实修法」实现，读码后没有发现与已接受 ADR 相抵之处，ADR 正文没动。

### 做了什么

- 领域 `domain.CostImprovementBasis`（`internal/networkrouting/domain/cost_improvement_basis.go`）：被替代计划成本、新计划成本、币种、阈值四格。构造门 `NewCostImprovementBasis` 是「严格大于阈值」唯一的定义处，改善不够格答 `ErrImprovementNotOverThreshold`，金额、阈值为负或币种空白答 `ErrInvalidReroute`。`JudgeCostImprovement` 收两份候选成本与阈值，够格时交回依据。
- 编排 `tryImprovementReroute` 落的`已改路`：带改善依据，候选评估记 `CANDIDATES_AVAILABLE`，不带失效依据。失效线 `rerouteAfterLapse` 与仍适用线同用 `improvementBasisOf` → `JudgeCostImprovement` 判改善，原先两处各写一遍的 `improved <= threshold` 去掉。失效线改路的记录不带改善依据。
- 端口 `ports.ReassessmentRecord` 加 `ImprovementBasis`、`HasImprovementBasis`。头注改成现行规则：改路三件也出自仍适用线；`已改路`的依据恰有其一。
- 迁移 `migrations/network_routing/0023_route_reassessment_improvement_basis.sql`：
  - `route_reassessment` 加 `improvement_basis jsonb`；
  - `route_reassessment_rerouted_shape` 改为失效依据与改善依据恰有其一，其余各项与 0004 相同；
  - 新立 `route_reassessment_improvement_basis_on_rerouted`：改善依据只出现在`已改路`上，且须是 jsonb 对象。
  - 存量安全：新列全空；存量`已改路`都出自失效线、都带失效依据。两条 CHECK 都直接校验存量，不用 NOT VALID。
- 复核册适配器读写这一列，读回经 `NewCostImprovementBasis` 重走构造门。
- 文档：
  - UC-NR-003 结果表「新路由已形成」一格补上仍适用线的改善切换，必须保存一栏补改路依据：恰有其一，改善依据的四格逐一写明。
  - code/60 Status 与「相关」里指向本票的缺口注改为已补。
  - 编排用例 `TestAnImprovementRerouteIsTheNextStartingPoint` 头注里「真库上还落不了复核册」改为指向派发真库用例。
  - NR CONTEXT 没动：「当前路由仍可执行时，轻微成本、时效或可靠性改善只有在冻结边界前且超过适用策略的改善阈值时，才允许自动切换」一句已经盖住这一格。复核册记哪几格不是 CONTEXT 的口径，归 UC 的结果契约。

### 代决

1. **「要裁的」：另立改善依据一格，CHECK 改为失效依据或改善依据恰有其一。** 照票面推荐。理由：
   - 放宽 CHECK、让改善改路不带任何依据，复核册就答不出这次为什么改路。仍适用线上原计划还走得通，切换只凭成本，审计要答得出改善了多少、比的是哪道阈值。
   - 「恰有其一」让依据本身成了这一行出自哪条线的判别：只看依据就分得出原计划是走不通了，还是被更便宜的路替下，不必另加一格「来源」。
   - 不选「改善依据在所有`已改路`上必带」：存量失效线`已改路`没有这一格，新 CHECK 就拒存量，只能 NOT VALID 或回填。回填出的「当时的成本」是编出来的。
2. **改善依据存一列 jsonb，不平铺四列。** 口径同建议、决定两列：内容由读回时的领域构造门复验，库面只钉在场与类型。被比的两份计划已由 `reviewed_plan`、`new_plan` 两列拥有，依据里不复述，只记成本、币种与阈值。
3. **币种记进依据。** 票面列的是三格。成本是最小币单位整数，不带币种读不出 4200 是什么；比较本就要求两份成本同币种（`CostImprovementMinor`），记一份即可。
4. **仍适用线改善改路的候选评估记 `CANDIDATES_AVAILABLE`。** 票面没提，但这是第二条会拒的 CHECK：`route_reassessment_candidate_state_closed` 要求凡非`仍适用`的行候选评估状态在场。只修 `rerouted_shape` 的话，这一行照样落不了库。已做变异：撤掉这一句，派发真库用例照旧答 `REASSESS_STORE_UNAVAILABLE`。走到这一步时排序已选出唯一合格候选，「有候选」是实话。
5. **失效线自动改路的成本比较不进记录。** 那一行的依据是失效依据。改善严格大于阈值在那里只是自动条件之一，与可控节点、未执行部分同类，其余条件也不逐条落册。
6. **改善判据收进领域一处。** 失效线、仍适用线、适配器读回三处都经 `NewCostImprovementBasis` 判「严格大于」。阈值为负、币种空白这类构造门拒的输入，`JudgeCostImprovement` 一律按不够格处置。今天到不了：登记口已拒负阈值。方向是保守的：没有依据就不切换。

### 越权风险点

1. **改善依据恰有其一，失效线改路不留成本比较**（NR owner）。失效线自动改路同样比过成本、过了阈值，复核册却只记失效依据。owner 若要两条线都留成本比较，得把「恰有其一」改成「失效依据可选、改善比较必带」。那时存量失效线`已改路`没有这一格，CHECK 只能 NOT VALID 加，或接受存量行例外。
2. **改善依据记的是判断时点现取的成本**（NR、PP owner）。成本按复核那一刻的价卡与汇率读数算（demo-chain-gaps/115 越权风险点 4），被替代计划的成本也是现取的，不是它当初入选时的价。依据答的是「这次判断时改善了多少」。
3. **上线闸门随本票解除，但改善替代一路随即走得到 code/60**（NR owner）。本票之前，改善改路在落册处就被拒，到不了离场门。本票之后，触发的业务时间若早于初始路由的处理时刻，离场门拒原计划离场，整封响亮失败、占着分区到 `ABANDONED`，归 [code/60](./60-initial-route-takes-effect-at-processing-time-unlike-reassessment.md)。首发链路上受理与首个控制触发之间通常隔几小时，通常撞不上；初始路由交接积压、离线补登时撞得上。ADR-0272 越权风险点 2 说改善替代那一行落不了复核册（code/53），那句已不成立；ADR 正文按约不改，交协调方定是否在其 Status 行加注。
4. **查阅面不透出改善依据**（NR owner）。`GET` 复核列表（`reassessmentBody`）本就只透出头部几列，不透出新计划与决定；改善依据同样不透出。运营在查阅面上看到一行`已改路`没有失效依据，要回复核册才知道它是改善改路。要透出，就要改契约与 admin-web，不在本票。
5. **演示链仍走不到这一格**（NR owner）。演示策略版本没声明自动改路，演示网络只有一条候选。这一格只由派发真库用例在合成夹具上证，证据记 `S`。

### 上线闸门

票面 Status 的闸门「本票落地之前，任何租户不得在路由策略版本里声明自动改路」随本票进 main 解除：声明了自动改路的租户，仍适用线上按改善切换的那一行落得了复核册，信封入账，不再回滚重投到 `ABANDONED`。解除之后剩下的风险见越权风险点 3（code/60）。

### AT

UC-NR-003 的 AT 表里没有「仍适用线上改善超过阈值即自动切换」一行：`AT-NR-033` 是没有超过阈值的候选，`AT-NR-041` 是已过冻结边界。本票的用例以 ADR-0173 决定一点名，AT 盘点没动。要不要补一行 AT，交协调方定；补的话新用例即是它的覆盖。

### 用例

- 领域 `internal/networkrouting/domain/cost_improvement_basis_test.go`：
  - `TestACostImprovementQualifiesOnlyWhenStrictlyOverTheThreshold`：严格大于才够格。恰等于阈值、阈值没登记、币种不同、新候选更贵都不够格。
  - `TestACostImprovementBasisRefusesWhatCouldNotHaveSwitched`：构造门拒改善不够格与四种不成形。
- 编排 `internal/networkrouting/application/improvement_reroute_basis_test.go`：
  - `TestAnImprovementRerouteRecordsItsImprovementBasis`：改善依据 1000 → 100、CNY、阈值 500。没有失效依据，候选评估有候选。
  - `TestAnImprovementAtTheThresholdKeepsThePlanWithoutABasis`：改善恰等于阈值时仍适用。不带依据，不碰适用性。
  - `TestALapseRerouteRecordsNoImprovementBasis`：失效线改路只带失效依据。
- 复核册真库 `internal/networkrouting/adapters/postgres/route_reassessment_improvement_basis_test.go`：
  - `TestAnImprovementRerouteRoundTripsWithItsImprovementBasis`：冻结未越过、改善超过阈值的`已改路`过 CHECK，四格往返。
  - `TestALapseRerouteCarriesNoImprovementBasis`：失效线改路读回不长出改善依据。
  - `TestAReroutedRowCarriesExactlyOneReason`：库面钉恰有其一。两个正例各只带一种依据；拒两样都带、两样都缺、改善依据是数组、`已失效`带改善依据。
- 派发真库 `cmd/parcel-dispatch/reassess_improvement_reroute_test.go` 的 `TestAnImprovementRerouteBeforeTheFreezeBoundaryLandsOnTheDatabase`，在 `reassessBothLanesFixture` 上跑：
  - 前提：声明自动改路、阈值 1000，两条候选 4200 与 3100，交进新加坡枢纽、冻结未越过。
  - 交接复核落一行`已改路`：带改善依据 4200 → 3100、阈值 1000、CNY，新计划选中便宜的那条，带自动决定。交接信封 `PROCESSED`。
  - 适用性册里原计划 `SUPERSEDED`，接班的是新计划，转移时刻是触发业务时间。
  - 又一次交接时以新计划为起点，判仍适用。
- 先跑红：派发那条在 main `107310894` 的编排上（只去掉改善依据那几行断言）投递答 `REASSESS_STORE_UNAVAILABLE`，与票面一致。其余新用例引用新加的类型与字段，修前编不过。

### 验证（分支 claude-code53）

- `go build ./...`、`go vet ./...` 干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...`：22 个包 ok，退出码 0。
- 新用例九条另跑 `-count=3`，都 ok。
- 迁移在一次性库 `code53seed` 上随演示种子灌了一遍（`scripts/demo-seeds/seed.sh` 退 0，两条 CHECK 在册），用完 `DROP DATABASE … WITH (FORCE)`。
- 存量安全另在同一库上验过，事务内做、随后 ROLLBACK：
  - 先把表退回 0022 的形状：拆掉两条新约束与新列，按 0004 原文重钉 `rerouted_shape`；
  - 插入存量形状的三行：失效线`已改路`、`仍适用`、`已失效`；
  - 再施加 0023 的语句，成功。
- 没跑全量，没重生成 MECHANISM-INVENTORY、ADR 索引与 AT 计数。迁移号 0023，取证时 main 上最大是 0022；后进 main 的撞号要改号。

