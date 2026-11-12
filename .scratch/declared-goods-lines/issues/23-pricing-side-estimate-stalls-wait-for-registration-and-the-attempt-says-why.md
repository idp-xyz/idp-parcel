# 23 价卡一侧答不出的估价等运营登记、计价参数登记后续办；处理尝试记下原因与产品，委托详情从落处读

Category: enhancement
Status: in-progress
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`）

依据：[ADR-0380](../../../docs/adr/0380-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定一至四；ADR-0094 决定三至五（等运营登记的落库、续办触发同笔落地、先存等待）；ADR-0229（计价「参数已登记」信封）。

## 要做的

- **分法**：小包托运的结算消费侧适配器（`adapters/settlementaccounting/pre_acceptance_control.go` 的 `notFormedOutcomeFor` 一带）把下列结算原因改归「未配置、等运营登记」：
  - `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`
  - `ESTIMATE_PRICE_CARD_CONFLICT`
  - `ESTIMATE_TRIAL_WITHOUT_TOTAL`
  - `ESTIMATE_NOT_POSITIVE`
  - `ESTIMATE_TRIAL_NEGATIVE`

  其余原因照旧，`ESTIMATE_CURRENCY_MISMATCH` 也照旧内部重试（只有缺陷走得到）。受理判断沿既有的等运营登记那一路先存等待、处理尝试同笔落库。改 `TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction`。
- **续办**：
  - 新 inbox 消费门接收 `parcel-pricing.pricing-parameter.registered`。消费方自己写这个串，组合根的对照用例钉住两串。
  - 只认 `SELL` 方向；按租户重驱停在等运营登记的委托，走与结算账户、估价采用登记续办同一个重驱口。
  - 在 `cmd/parcel-dispatch` 装配，并补进路由与消费者名的登记。
- **处理尝试另记两格**：
  - 结算的未形成原因码（如 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`，不加 `SA-` 前缀，另一格就是「来自结算」）；
  - 这一版申报的服务产品（取小包托运自己的提交版本，没申报不记）。
  - 迁移给 `parcel_shipment.acceptance_processing_attempt` 加这两列与 `submission_version`（判的是哪一版），都可空，旧行不回填。
- **委托详情**：
  - `taskViewRecord` 本身改从 `acceptance_processing_attempt` 读这份委托最近一条（按尝试时刻，确定的次序）：委托详情与审核、处置、补件几个队列读的都是它，一起改。
  - `acceptanceTask` 的 `lastAttemptReason`、`lastAttemptContinuation`、`lastAttemptedAt` 照旧，另交 `lastAttemptResumePath`（续办方，取尝试上记的那一格）、`lastAttemptSettlementReason` 与 `lastAttemptServiceProduct`（可缺）。字段名钉死，管理台票 24 照它写。
  - 聚合文档里的处理尝试列表不再当读源。它没有生产写入；还有读面靠它就留着并说明。

## 完成判据

- 真库：
  - 没有在用价卡的估价 → 委托停在等运营登记，处理尝试落库、带原因码与服务产品；
  - 登一张生效期覆盖提交时点的售价卡（交「参数已登记」）→ 委托被重驱、解开后接受；只从此刻起生效的卡唤醒它、照旧停、多一条尝试；
  - `BUY` 方向的信封不重驱。
- 估价采用登记后重驱到「没有在用价卡」的那一路不再回滚（`cmd/parcel-dispatch/estimate_adoption_registration_resume_loop_test.go` 跟着改）：新的等待落库，上一次 `CONTROL_AMOUNT_NOT_CONFIGURED` 那一条被新的一条取代为最近一条。
- 依赖读不回照旧内部重试、不落库。
- 委托详情的「上次处理」在等运营登记时有值，并带两格。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

申报不全改等客户补充（票 25）；按原因或计价范围收窄重驱（ADR-0380 越权风险点「多唤醒」）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/estimate-stall-waits`（工作树 `idp-parcel-goods-t23`，基线 `cf9fdb50a`）上的 `26ae187cc`，一笔，未推送、未进 main。按评审对 ADR-0380 的更正做（改归的原因只剩五个、`lastAttemptResumePath`、`taskViewRecord` 本身改、版本列、生效期覆盖判断时点、点名改的续办用例），票面已同步。
- 拣入：在 `feat/goods-details-admin` 上是 `27e35ba25` 与评审修订 `7947bdad3`，拣入时无冲突；迁移 `0038` 两边逐字节相同（sha256 `05872582…`）。
- 落点：
  - 结算消费侧适配器（`adapters/settlementaccounting/pre_acceptance_control.go`）：`notFormedOutcomeFor` 把 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`、`ESTIMATE_PRICE_CARD_CONFLICT`、`ESTIMATE_TRIAL_WITHOUT_TOTAL`、`ESTIMATE_NOT_POSITIVE`、`ESTIMATE_TRIAL_NEGATIVE` 改归`未配置`；`ESTIMATE_CURRENCY_MISMATCH` 单列、照旧`未形成`（只有缺陷走得到）；不可见、缺件、一件都没有、溢出照旧。`assessmentFor` 在未形成时把原因码原样交在新端口格 `PreAcceptanceControlAssessment.SettlementReason`（`Reason` 照旧冠 `SA-` 进续办派生）。
  - 领域：新值 `SettlementNotFormedReason`；`ProcessingAttemptSpec` / `ProcessingAttempt` 加可缺的 `SettlementReason`、`ServiceProduct`。
  - 应用：`awaitOperatorRegistration` 收被判的版本，等待落了库时交回这一版的 `RequestedServiceProduct()`；`recordAttempt` 收版本与 `attemptDetail`（结算原因、产品）。财务控制腿在端口那一停带两格，原因换成保存那一格时两格随范围放下；时点那一停（两条腿）只带产品。决定、拒绝、撤回那几处照旧不带两格，只多传版本。
  - 端口 `AcceptanceJudgmentRecorder.RecordProcessingAttempt` 收提交版本；`AcceptanceTaskViewRecord` 加 `LastAttemptResumePath`、`LastAttemptSettlementReason`、`LastAttemptServiceProduct`。
  - 迁移 `migrations/parcel_shipment/0038_processing_attempt_settlement_reason_product_and_version.sql`：`settlement_reason`（在场不许空白）、`service_product`（不加非空白约束：客户原话原样保全，约束拦下会让整笔事务作废）、`submission_version`（在场不许空白），都可空、不回填。
  - 读面（`adapters/postgres/shipment_request_views.go`）：`taskViewRecord` 不再读文档，改由 `taskViewRecords` 一页一次从处理尝试表取各委托当前这一版任务最近一条——按 `submission_version` 认版本，无版本的旧行按任务建立时刻圈定；`attempted_at DESC, continuation_ref DESC` 定先后（与主键同列，并列也有确定答案）。委托详情与复核、授权处置、受控补充各队列一起走它（队列先读完关游标再读尝试，`scanQueueRows`）。
  - HTTP（`query_shipment_request_views.go`）：`acceptanceTask` 另交 `lastAttemptResumePath`（`INTERNAL_RETRY`、`OPERATOR_REGISTRATION`……，与 `waitingOn` 同一套原词）、`lastAttemptSettlementReason`、`lastAttemptServiceProduct`，都可缺。各队列的答复形状不动。
  - inbox（`adapters/inbox/pricing_parameter_registered_consumer.go`）：`NewPricingParameterRegisteredConsumer`，消费者名 `parcel-shipment/advance-acceptance-chain-on-pricing-parameter-registration`，类型串自写；SELL 重驱、BUY 入账不问队列、方向缺席或集外是毒丸；租户、种类同另几份门的核法，计价范围不读。`newRegistrationResumeConsumer` 改为按实例收译码函数，另几份门照旧用 `decodeOperatorRegistrationCompleted`。
  - 装配：`cmd/parcel-dispatch/assemble.go` 的 `acceptanceChainGates.pricingParameterRegistration`；`routes_acceptance_chain.go` 登 PS 这一路（主名次、同一份未决哨兵）；`routes_evaluation_resumption.go` 把 SA 评价续办收同一类信封那一路挪到计费名次（同名次装配即拒），与「结算账户已登记」同一先后。
  - 删除：聚合上那份处理尝试列表——`ShipmentRequest.RecordProcessingAttempt`、`AcceptanceDecisionTask.ProcessingAttempts`、`RehydrateAcceptanceTaskSpec.ProcessingAttempts` 与重建校验、快照文档的 `processingAttempts` 与 `attemptDocument`。委托详情与各队列都已改读处理尝试表，删前核过再无读者；生产从没写过它，旧快照里没有这一键。相应删改的测试：领域累加与已完成拒追加、重建带回与追加、零值尝试拒收、版本不动表里那一行、快照往返里那半，队列与详情用例改经生产写口记进处理尝试表。
- 判据：
  - 真库：没有在用价卡 → 停在等运营登记、提交门入账，处理尝试一行带 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`、申报产品与版本，委托详情「上次处理」带续办方与两格 ✅；`BUY` 信封入账、不重驱（试算没再被问、尝试不加）✅；登卡这一刻才生效的 `SELL` 卡（晚于这一版接收、早于重驱）叫醒委托、照旧停、多一条尝试 ✅；生效期覆盖提交接收时点的 `SELL` 卡登上 → 重驱、冻结占下、接受、队列出列 ✅。估价时点照「提交接收」折（生产 `SubmissionReceiptAsOf`），选卡走计价真价卡册的在用判断，金额由替身给；把时点换成重驱那一刻，用例当场红。
  - 估价采用登记后重驱到「没有在用价卡」不再回滚：`estimate_adoption_registration_resume_loop_test.go` 改为本封入账、尝试两行、最近一条带 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`，取代 `CONTROL_AMOUNT_NOT_CONFIGURED` 那一条 ✅。
  - 依赖读不回（试算 `ESTIMATE_TRIALS_UNAVAILABLE`）照旧未决哨兵、整笔回滚，等待态不落、尝试一行不留 ✅。
  - 委托详情「上次处理」在等运营登记时有值并带两格；读面只认当前这一版、并列按续办引用定、旧行按建立时刻圈定 ✅。
  - 组合根：登卡经计价真交接口铸的信封，类型与 PS、SA 两边自写的串对上，一拍扇出、两路各记一行 inbox ✅。
  - 全量 `go test`（带 PG）与 `go vet` 通过 ✅。
- 未做：
  - 管理台（票 24）。
  - 各队列的答复不加详情新增的那几格（读源已一起改，形状不动）。
  - 决定、拒绝、撤回那几处落的尝试不记产品：ADR-0380 决定三说的是等待落库的尝试，这里只在等运营登记那一路取。
  - 申报不全改等客户补充（票 25）；按原因或计价范围收窄重驱（ADR-0380 越权风险点「多唤醒」）。
  - CONTEXT、UC 的文档改动（ADR-0380 那一笔的事）。
- 与票面的细处：
  - 读面按「当前这一版任务」圈定（ADR-0380 决定四原话），不是整份委托跨版本取最近：新版本立起新任务之后、还没有尝试时，「上次处理」缺席，不把上一版的停因挂到新任务上。
  - 结算原因只在结算答未形成时记；作用域没形成（`CONTROL_SCOPE_NOT_CONFIGURED`）是本上下文适配器自己的停因，不记进这一格。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
- 评审（非作者，2026-11-11）：无阻断；应改两条与两处措辞，修在 `5625b7cd1`（同分支一笔，接在 `26ae187cc` 之后，未推送）：
  - 登记续办按位置翻页：`OperatorRegistrationQueue.ListWaitingOnOperatorRegistration` 收起点位置 `OperatorRegistrationQueuePosition`（提交时刻、委托标识），按行值比较往后取；`redriveWaitingRequests` 从上一页最后一行之后取到不足一页为止，去掉「本页没有新面孔即止」。原先每页重读队首：自 ADR-0380 起被重驱又停回等运营登记是常态，比满一页还多的旧委托会把后面这次登记正好解得开的那一份永远挡在页外。补测：`cmd/parcel-dispatch/operator_registration_redrive_paging_test.go` 真库页大小 2、先停四份（前三份照旧停、最新一份解得开），一封 SELL 登记之后四份各被问恰一次、最新那份接受、其余照旧等；队列读口按位置往后取；登记续办门第二页从第一页最后一份之后问。
  - 上次处理逐份取一条走索引：迁移 `0038`（原地改，未进 main）加 `acceptance_processing_attempt_latest`（租户、委托、版本、发生时刻倒序、续办引用倒序），等待运营登记的队列索引补齐标识段；`lastAttempts` 改为 `CROSS JOIN LATERAL`，有版本那一支与无版本旧行那一支（发生时刻不早于任务建立）各 `LIMIT 1` 再取一条。EXPLAIN（关掉顺序扫描）有版本那一支走新索引、不排序。改后 `0038` 的 sha256 `05872582f4e60a36c653c1e8063fd58410e03981949fb1379fadbcd6ad678de3`，拣入时两份要一致。
  - 措辞：ADR-0380 越权风险点改按标题引（「多唤醒」「哪几种原因算『登记解得开』是产品策略」），不按序号。财务控制腿 `undecided` 的 `detail` 注释照实写：结算原因在内部重试那几停也带着、不另清，那一轮整笔回滚、尝试随之不留；产品只在等运营登记的等待落库时才有（选了改注释，不改行为）。
  - 评审后全量门禁（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
