# 14 BUY 评价采用不了之后请求卡死，补登卡上取值也不回补

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg14。此前 in-progress——2026-10-21 认领；此前 ready-for-agent——2026-10-20 上线就绪评估归 R1（上线前必修），裁定与批次见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md) 的隔离评审。
Blocked by: 无
出处：[ADR-0200](../../../docs/adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 越权风险点 3；SELL 那一路同一格见 [ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 越权风险点 3。

## 现象

BUY 评价不可计价、待判断、跨币种没带换算步骤或卡没声明金额取整时，结算入账、不形成成本。之后租户补登了卡上的取整声明或汇率序列，同一组来源引用再请求会命中原请求，评价也不会重来，这件发生项永远没有预期成本。SELL 那一路的请求同样卡死。

## 三类

机制（续办路径）。规模 M。

## 最小的诚实修法

照 [ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 的续办做法：参数登记之后发一类「参数已登记」信封，由请求侧按自然键另起一份新请求（新请求标识，旧请求留作历史），不改写旧评价。BUY 与 SELL 两路共用同一个续办形状。

## 要裁的

续办由谁触发（登记口同笔发信封，还是运营手动续办）；新请求怎样与旧请求关联（自然键加代次）。归 SA、PP owner。

## 既有票

none

## 完成记录（2026-10-21，分支 claude-dcg14）

口径与授权：用户常设指令「你作为业务和系统专家自决……解决所有的问题」；本票由 2026-10-20 上线就绪评估（代决）归 R1 并扩大范围，裁定见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」；另一协调会话按用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」把 M15（BUY 触发册修订形）并进本票、代决取第一条。决定落在 [ADR-0229](../../../docs/adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md)。

### 做了什么

- **代次与停因**（迁移 `settlement_accounting/0041`）：BUY 与 SELL 两本评价请求册加代次、前一代、续办依据与停因；（自然键、代次）唯一，一份请求至多一个下一代；SELL 册加`未形成`，停下的各格必带停因（CHECK 配对）。迁移前的`不可计价`、`冲突`照填停因，`待判断`标「停因未记」。
- **发起停办册**（迁移 `0042`）：SELL 计费时点与 BUY 发生项走到发起就答`未配置`（含包裹没有商业解析）时记下那一拍的输入与停因；`采用了别的时点`、修订版不记。
- **计价答不出评价不再进未决名单**：PP 对请求答没有在用价卡、适用冲突、输入不可得时入账、交「评价未形成」，SA 记停因；PP 请求两路的未决名单只剩「请求还读不回」与「形成依赖故障」。
- **采用不了记停因**：BUY 形成那一半在请求册上记停因；SELL 评价失败改记`未形成`。
- **参数已登记**：结算三本评价参数册（SELL 计费时点采用、费用项目映射、BUY 触发册）落行同笔交「评价参数已登记」；结算账户已登记多一路 SA 续办门；PP 价卡登记（含草稿发布，带计价范围）与序列、目录复核通过（含免复核代写）同笔交「参数已登记」。三类信封各一扇续办门，按停因（价卡另按计价范围）取队列，逐件一笔事务续办：停下的请求另起下一代、发起停办按记下的输入重跑；同一依据对同一条代次链至多续办一次。
- **运营续办口**：`parcel-settlement-register evaluation-resume`，按包裹（SELL）或发生项版本（BUY）点名，续办引用必填，同一引用重跑是重放。
- **M15**：BUY 触发册按（租户、原因）排版次，没登费用项目的那一版可追加一版带费用项目的，读口取最新一版，已登的换另一个仍冲突。
- **续办收窄**（评审一轮）：评价待判断按计价的问题项分格（目录读不出、序列读不出、价表没有这一段、包裹事实不全、笼统），哪一类登记解得开哪一格照表收紧（序列解不开缺区，评价失败只认价卡）；信封带收窄格——PP 价卡带计价范围与方向，序列与目录复核按绑了它的卡（`PriceCards.FindPricedScopesBinding`）每个范围、方向各交一封，内部方向不交；SA BUY 触发册带发生项原因、SELL 计费时点采用带户；续办门缺必带格判毒丸，队列按范围、方向、原因、户收窄。
- **SELL 发起停办落终止**（评审一轮）：停在计费时点没采用的那一拍记下那一户；续办重跑答`采用了别的时点`时落终止（`closed_at`），不再进队列，再停一次清掉终止。
- 文档：ADR-0229 新立；ADR-0190、0198、0200、0208 的 Status 行与 Links 加前向指针、被停条款进索引注记（正文不改，ADR-0190 文末「复核记录」一节保留在 Links 之后）；UC-SA-002 补 `AT-SA-188`–`AT-SA-199`、改记 `AT-SA-180`、`183`、`186`；UC-PP-001 补 `AT-PP-001`–`AT-PP-006`；GLOSSARY 立「停因」「代次」「发起停办」「评价未形成」「参数已登记」五条，SA、PP `CONTEXT` 与 `CONTEXT-MAP` 引用它们；AT 盘点①加行（总览计数未改）。

### 代决与越权风险点

代决全文与逐条越权风险点在 ADR-0229 文末，摘要：

1. 停因与「哪类登记解得开哪些停因」那张表、待判断的分格是产品策略（不可计价与评价失败只认价卡、目录读不出只认目录等）。
2. 发起停办续办出来的是带依据的首代，不「回指前一代」——那一拍没有请求可回指。
3. SELL 下一代照抄账户、币种与计价范围，不重走结算账户链（账户、采用册今天不可变）。
4. 收窄到哪一层：序列与目录按绑了它的卡收窄、不分卡的版本与适用期（同范围另一版卡上停在序列读不出的也会多推一代）；求范围要读遍租户的卡；结算账户与映射登记不收窄（多推的发起停办只换依据与时刻，多推的请求多一代）。
5. 「评价未形成」按请求分区、「参数已登记」按租户分区：登记恰在评价形成与记停因之间时，那一份要等下一次登记或运营续办。
6. 取整未声明、不可计价、评价冲突、价表没有这一段都要新一版卡：同期间的新版答适用冲突，期间不重叠的新版不覆盖停着的请求的基准时点，这几格续办不出评价，等票 [150](./150-price-card-has-no-controlled-cut-off-for-a-superseded-version.md)（R1）。首周四格里只有缺区与汇率晚到会自动续办出评价；缺重量段等 150；缺尺寸只能运营续办。
7. 迁移前停在`已提交`而实际卡住的请求不回补、不能点名续办。
8. 运营续办只有受控进程入口，没有查阅面与管理台入口——另立票 [151](./151-stalled-evaluation-queue-has-no-operator-view.md)。
9. `采用了别的时点`不记发起停办，重跑答它就落终止；日后采用册若加修订，这两处要回头补。
10. BUY 触发册只允许补空格。
11. 评价失败只认价卡登记。
12. 续办门里一件矛盾让整封落发布失败，已续办的那几件不回滚。
13. 结算政策没采用今天没有续办路：答案在委托接受时固定的商业闭包上，结算的登记与运营续办重跑都改不了它；出路（重新接受或闭包修订形）归 SA、PC owner，另立票 [158](./158-settlement-policy-not-adopted-has-no-resumption-path.md)。

装配上的偏离（给协调方）：`cmd/parcel-dispatch/assemble.go` 改了——既有的 SA/PP 消费者构造（发起编排加发起停办册、BUY 形成加停因写口、PP 请求适配器加「评价未形成」交接口）与五份既有未决哨兵名单及头注就在这个文件里（ADR-0222 决定二「既有的哨兵名单留在原处」），本票的核心之一正是改这几份名单；BUY 发起编排的构造搬进新文件 `assemble_evaluation_resumption.go` 由两处共用。新路由在 `routes_evaluation_resumption.go`。

### 用例（真库）

- `cmd/parcel-dispatch` `TestABuyRequestStalledOnAMissingPriceCardResumesAfterTheCardIsRegistered`：BUY 发生项 → 请求 → PP 答没卡（入账、交评价未形成、不再 ABANDONED）→ SA 记停因 → 登演示成本卡（同笔交参数已登记）→ 续办出第二代（回指第一代、依据 `PARAMREG/PRICE_CARD/…`）→ PP 形成评价 → SA 以 `BUYSC-<第二代>` 形成预期成本；第一代原样；再来一封价卡登记不出第三代。
- `TestABuyOccurrenceStalledOnAMissingFeeItemResumesAfterTheTriggerIsRevised`：触发册先登无费用项目 → 发生项答未配置、记发起停办 → 追加一版带费用项目（旧版留历史、重放幂等）→ 续办发出首代 → 评价 → 预期成本。
- `TestASellMomentStalledOnAMissingAdoptionResumesAfterTheAdoptionIsRegistered`：SELL 计费时点答计费时点没采用 → 发起停办 → 补登采用（重放两次）→ 恰一份首代请求。
- `TestASellRequestPendingOnAnIncompleteMappingResumesAfterTheMappingIsRegistered`：SELL 映射不全记`待判断`+停因 → 补登映射 → 第二代形成预估客户费用，第一代仍`待判断`；几行映射几封信只续办出一代。
- `TestASubmittedEvaluationRequestStopsHonestlyAtThePricingInputSeamThroughTheRouteTable`（改写）：没卡与输入不可得都入账、交评价未形成、SA 记停因；SA 还没有的请求仍是 `dispatch.consumer_undecided`（真正的依赖读不动仍按未决重投）。
- `cmd/parcel-settlement-register` `TestTheOperatorResumesAStalledBuyRequestOnceOnRealPostgres`：运营续办口续办一代、同引用重跑不出第三代。
- `TestAnExchangeRateFeedResumesOnlyWhatItsBindingCardsCanUnblock`（评审一轮）：经真复核用例连喂两期汇率序列，PP 按绑了它的卡交面——绑了汇率序列那张 BUY 卡所在范围上缺换算步骤的请求续办出下一代；同停因、范围上的卡没绑它的不动；缺区的请求两期都不动。
- `TestTheSettlementAccountRegisteredEnvelopeDrivesTheEvaluationResumption`（评审一轮）：账户没登 → 发起停办 → 经 `RegisterSettlementAccountHandler` 与真交接口登账户 → 信封扇出到小包托运与结算两路、各一行 inbox、结算那一路处理完毕 → 发起停办换成计费时点没采用并记下那一户 → 登采用（按户收窄）→ 首代请求。
- `TestAStallWhoseAccountAdoptsAnotherMomentIsClosed`（评审一轮）：停在计费时点没采用 → 这一户登的是另一种时点 → 重跑答`采用了别的时点`、落终止、零请求；运营点名这件包裹时终止的那一拍不进队列。
- SA postgres：代次链、停因写口、发起停办册（含户）、续办队列口径（含按位置翻页与整链排除依据；按方向、发生项原因、户收窄，落终止出队、终止重放不改时刻、再停一次又进）、BUY 触发册修订形、评价参数已登记交接口（收窄格必带、缺或多都拒）；PP postgres：两只交接口（每一面带范围与方向、内部方向拒）、按绑定找卡所在范围（`TestTheCardsBindingASeriesOrCatalogueAreFoundWithTheirScopes`）。续办门翻页、收窄与毒丸另有单元用例；结算读计价评价那一层按问题项分待判断有单元用例。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 只读评审子代理过了一遍 Go 与 SQL 的差异，报两处：续办门每页重读队首、办过却没写回的件会把后面的挡住（重要）；排除依据只看链头（次要）。两处都已改：队列按位置翻页、请求两支查整条代次链，另补翻页与整链排除的用例；其余各点（迁移对存量行、CHECK 的 NULL、保存幂等、同事务交信封、信封标识、哨兵分格）评审确认无误。
- `scripts/test-full.sh` 跑 `./internal/settlementaccounting/...`、`./internal/parcelpricing/...`、`./internal/architecture/...`、`./migrations/...`、`./tools/...` 与反向依赖 `./cmd/parcel-api`、`./cmd/parcel-dispatch`、`./cmd/parcel-pricing-feed`、`./cmd/parcel-pricing-register`、`./cmd/parcel-pricing-template`、`./cmd/parcel-settlement-register`、`./internal/customscompliance/adapters/settlementaccounting`、`./internal/networkrouting/adapters/parcelpricing`、`./internal/parcelshipment/adapters/parcelpricing`、`./internal/parcelshipment/adapters/settlementaccounting`、`./scripts/demo-seeds/seedgen`：40 个包全绿。
- 演示种子在 55432 的一次性库 `dcg14_seed` 上灌了一遍，零报错（价卡登记交 5 封参数已登记、SA 评价参数册交 4 封），用完 `DROP ... WITH (FORCE)`。
- 未跑全量；未重生成机制清点、ADR 索引与 AT 计数（`go run ./tools/adr-index -check` 如预期报 README 待重生成）。
- 评审一轮之后（rebase 到 main 之后重跑）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`scripts/test-full.sh` 跑同一组 40 个包全绿；派发端到端的新用例另做过变异检验（去掉续办门的范围收窄，`TestAnExchangeRateFeedResumesOnlyWhatItsBindingCardsCanUnblock` 转红）。演示种子在 55432 的一次性库 `dcg14_seed` 上重灌一遍零报错：计价交 5 封参数已登记（两张卡、燃油序列绑在 SELL 卡上一面、分区目录绑在两张卡上两面，汇率序列没有卡绑、一封不交），结算评价参数册交 4 封（采用带户、触发册带发生项原因），用完 `DROP ... WITH (FORCE)`。

### 评审记录（2026-10-21，隔离评审一轮，无阻断项）

逐条处置：

1. **结算政策没采用是永久停**：采纳。ADR-0229 新立越权风险点 13（没有续办路，出路归 SA、PC owner：重新接受或闭包修订形），决定五、决定六改写，不再说运营续办口能兜它；本票「没做与去向」改写；去向另立票 [158](./158-settlement-policy-not-adopted-has-no-resumption-path.md)（needs-info）。代码里 `EvaluationStopCause.ResumedBy` 头注、`sellRequestConsumption` 头注同口径。
2. **越权风险点 6 改写**：采纳。取整未声明、不可计价、评价冲突、价表没有这一段（缺重量段）都要新一版卡；同期间答适用冲突、不重叠的不覆盖基准时点；等票 150（R1）。首周四格只有缺区、汇率晚到会自动续办出评价，缺重量段等 150，缺尺寸只能运营续办。票 150 的现象一并补齐这几格。
3. **序列与目录登记放大在代码里收住**：采纳。PP 新开 `ports.PricingParameterBindingView`（`PriceCards.FindPricedScopesBinding`），序列、目录复核通过时按绑了它的卡的计价范围与方向一面交一封、没有卡绑它不交，价卡也带方向；SA 待判断按计价问题项分格（`pendingCauseOf`：目录读不出、序列读不出、价表没有这一段、包裹事实不全，分不出落笼统），`ResumedBy` 收紧到序列解不开缺区、评价失败只认价卡。真库用例 `TestAnExchangeRateFeedResumesOnlyWhatItsBindingCardsCanUnblock`；越权风险点 4 改写成「收窄到哪一层」。
4. **更多收窄**：采纳。价卡分方向；BUY 触发册按发生项原因；SELL 计费时点采用按户（发起停办记户）；计价三类缺范围或方向、触发册缺原因、采用缺户都判毒丸。哪一类带哪几格只在 `EvaluationParameterKind.Narrowing` 定一处，SA 交接口与续办门都读它。用例：SA postgres `TestTheStalledQueueNarrowsByDirectionReasonAndAccountAndSkipsClosedStalls` 与交接口用例、inbox `TestTheResumptionGateNarrowsByWhatTheEnvelopeCarries`、PP 交接口用例、领域 `TestAParameterKindDeclaresWhatItsEnvelopeNarrowsBy`。结算账户与费用项目映射两类不收窄（发起停办上记不下等哪一户、请求上没记缺哪个代码），理由与代价写进越权风险点 4。
5. **「册不设修订」一并停用**：采纳。ADR-0229 决定七与「对已接受记录的作用」、ADR-0208 Status 索引注记与 Links 都改成停用两句、适用场景「同原因换一个已登的费用项目仍答冲突」。
6. **分区登记表那一行**：采纳。裁段改成「与计价的参数已登记、结算账户已登记同字符串同队；评价未形成另队」，注释去掉「上一行」；那一行挪到「受理前估价采用已登记」之后——原位置会让那一行既有的「同上一行」改指本行。
7. **assemble.go 形成消费者头注**：采纳，改为「采用不了时在请求册上记停因（ADR-0229）」。
8. **parcel-settlement-register「同上」**：采纳，点名 `accountHandoff`。
9. **SELL 发起停办死行**：采纳，落终止。迁移 0042 加 `closed_at`，`SellEvaluationInitiationStallRegister.CloseSellInitiationStall`，续办重跑答`采用了别的时点`落终止（新结局`CLOSED`）、队列不再取，再停一次清掉终止。用例 `TestAStallWhoseAccountAdoptsAnotherMomentIsClosed` 与真库队列用例。没另立票。
10. **ADR-0200 按原句引**：采纳，ADR-0229 与 ADR-0200 Status 都引「评价停下的各格入账，不重投，不改记任何册」。
11. **旧注释**：采纳。`form_estimated_customer_charge_test.go` 头注改为`未形成`连同停因；`ErrSellEvaluationRequestUndecided`、`ErrBuyEvaluationTriggerUndecided` 与 `chargeOccurrenceFormedUndecidedSentinels` 改为点名各自的未决原因类型、说明发起停办册记不下也在其中，不再列举各口；`sellRequestConsumption` 头注写明结算政策没采用与包裹无商业解析不经参数已登记续办。
12. **真生产方驱动 SA 续办门的端到端用例**：采纳，`TestTheSettlementAccountRegisteredEnvelopeDrivesTheEvaluationResumption`（含扇出两路的 inbox 行）。
13. **GLOSSARY 五条**：采纳，「停因」「代次」「发起停办」「评价未形成」「参数已登记」立在 GLOSSARY「计量与结算」一节，SA、PP `CONTEXT` 改为引用。

没有不采纳的条目。

### 没做与去向

- 价卡截止旧版的受控口 → 票 [150](./150-price-card-has-no-controlled-cut-off-for-a-superseded-version.md)。
- 停着的评价请求查阅面、管理台运营续办（运营决定种类）→ 票 [151](./151-stalled-evaluation-queue-has-no-operator-view.md)。没有新开 access_identity 迁移。
- 「结算政策没采用」没有续办路：闭包在委托接受时固定，任何登记都改不了那一拍的答案，运营续办重跑读到的是同一份闭包、照样停下；本票不改 PC、不立替代出路，去向 → 票 [158](./158-settlement-policy-not-adopted-has-no-resumption-path.md)（等 SA、PC owner 裁）。
- 迁移前已卡住、停在`已提交`的请求不回补（越权风险点 7）。
