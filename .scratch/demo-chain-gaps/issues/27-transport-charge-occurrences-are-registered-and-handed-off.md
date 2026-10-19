# 27 运输收费发生项显式登记与已形成信封（失败尝试、实际履约）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg27
Blocked by: [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md)、[26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md)
地盘：TF `domain/transport_charge_occurrence.go`；`application` 新 `register_charge_occurrence.go` + `register_failed_attempt_charge.go`；新 `ports/charge_occurrence.go`；新 `postgres/charge_occurrence_handoff.go`；新 tfhttp `register_charge_occurrence.go`；`operation_decision.go`；新 `cmd/parcel-api/assemble_charge_occurrence.go` + `endpoints.go` 行；`internal/architecture/production_wiring_baseline.txt`；种子；ADR-0207、UC-TF-005 步与 AT、UC-TF-006/002 指向。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-1 至 D07-9，「需要的 ADR」ADR-0207。

## 做什么

- 运营决定 `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`，一个登记用例；本票落失败尝试与实际履约两种原因，不内联进交接或交付编排（方案 D07-3）。
- 发生条件先由登记者断言并留痕（方案 D07-1）；协议文法归 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md)。
- 采购上下文由登记方给出，经 [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md) 的适配器核 PC；缺采购上下文与自营的答法照方案 D07-2、D07-7。
- 发生项身份与旅程照方案 D07-5、D07-6。
- 派送失败尝试经现有 `DeliveryAttemptView`，不建新存储（方案 D07-8）。
- 信封 `transport-fulfillment.charge-occurrence.formed` 的载荷、ID 与同笔保存照方案 D07-4；消费者 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 先登路由。
- 演示先走的原因与登记时点照方案 D07-9。
- 起草 ADR-0207（要点见方案「需要的 ADR」）；UC-TF-005 补步与 AT，UC-TF-006/002 加指向；生产接线棘轮剪掉一条。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 结果格 `REGISTERED` / `EXISTING` / `NOT_APPLICABLE`（仅显式自营）/ `UNDECIDED` / `INPUT_NOT_ACCEPTED`，加协议否定格。
- 保存与 Outbox 同一笔（真库），用指纹 ID。
- 棘轮剪掉一条。
- 不碰交接、交付编排。
- 演示：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) → BUY 请求 → 计价评价 → ADR-0200 形成供应商预期成本（需 [30](./30-buy-cost-card-declares-amount-rounding.md)）。
- 发生项上的 `agreement_ref` 照 ADR-0208 决定六写「对象/版本」；`cmd/parcel-dispatch` 有一条用例经本票的真交接口入队，钉生产方的类型串与载荷三键和 `sainbox.ChargeOccurrenceFormedEventType`、消费门认的形状相等（26 的派发用例按载荷形状手工入队，钉不住这一点；出自 26 评审）。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg27`，基于 `origin/main` `451fbf4d`）

上一位作者被用量上限中断时已留下四笔（用例与信封、决定种类、装配、种子）。本次在其上续做：先后三次 rebase 到 `origin/main`（热点文件只取两边并集），补齐其余并实走演示。

### 做了什么

- **领域**（`domain/transport_charge_occurrence.go`）：实际履约与派送失败尝试两扇构造门（`ChargeOccurrenceForActualFulfillment`、`ChargeOccurrenceForDeliveryAttempt`），依据前缀揽收与派送分写；发生条件 `OccurrenceCondition`，封闭依据只有「登记者断言」一格，原话随发生项；身份派生 `DeriveChargeOccurrenceReference`（`TCO-` 加指纹前十六字节）、首版 `V1`；`SameContentAs` 按集合比成员。
- **应用**：一个登记用例 `RegisterChargeOccurrenceHandler`，失败尝试一半在 `register_failed_attempt_charge.go`（揽收按尝试来源键、派送经现有 `DeliveryAttemptView`）。结果格：`REGISTERED`、`EXISTING`、`CONFLICT`、`NOT_APPLICABLE`（只认显式声明自营与实际承运商判为自营）、协议四个否定格、`UNDECIDED`、`INPUT_NOT_ACCEPTED`，各带细格。订舱与取消答 `REASON_NOT_REGISTRABLE`，等票 28。
- **端口与库**：新 `ports/charge_occurrence.go` 的交接口、段与承运商判断只读口，册口加 `FindCovering`；`postgres/charge_occurrence_handoff.go` 引用式三键、`FingerprintEventID`、分区键「租户/charge-occurrence/发生项」；迁移 `transport_fulfillment/0032_charge_occurrence_condition_and_coverage_index.sql`（发生条件两列、表里有行即拒；成员交集一问的索引）。
- **入口与装配**：决定种类 `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`（`operation_decision.go` 只追加；迁移 `access_identity/0009` 重建决定种类 CHECK，全集含 `PENDING_IDENTITY_RECHECK`、`IDENTITY_CONFLICT_DISPOSITION`、`TRANSPORT_COMMISSION_SUBMISSION` 与本种）；tfhttp `register_charge_occurrence.go` 与运营决定 Intake 一口；`cmd/parcel-api/assemble_charge_occurrence.go` 把登记包进一笔事务，未决整笔回滚；`endpoints.go`、`main.go` 各追加一行。
- **种子**：配置员持这一种决定——`data/access/02` 授予、`03` 准入对照、`data/governance/25` 权威区间各一行，`seed.sh` 追加一行；README 补登法与「已知边界」一条。
- **文档**：起草 ADR-0207；ADR-0098、ADR-0208 的 Links 各加一条前向指针；`docs/adr/README.md` 追加一行；UC-TF-005 补结果行、步 9 与 `AT-TF-120` 至 `AT-TF-125`；UC-TF-002、UC-TF-006 步 7 指向。

### 代决

1. **失败尝试费处理器并入登记用例**，原来独立的那个处理器构造函数随之退场。两条原因共用身份派生、采购上下文核对、落册与交意图；分两个处理器就要把这些写两遍。
2. **自营两格都先问册**：同一身份已登成外部发生项答冲突并回显。实际承运商判断可以在登记之后改判自营法人，直接答不适用会把册上那份外部成本说成没有。
3. **同一事实上成员与另一项仍有效的发生项有交集，答 `CONFLICT` 加细格 `MEMBERS_ALREADY_COVERED`**，回显那一项。身份按成员集合派生，组合一换就是另一项，不查交集，同一成员在同一段上会被算两遍。读每一项的当前版（没有后继的那一版），失效项不算。查与落册之间按（租户、原因、事实依据）取事务级咨询锁，并发两笔不会互相看不见。
4. **实际履约的依据只指段，成员进身份**，业务时间取成员终点中最晚的一刻。
5. **发生条件的依据做成封闭集合，断言原话另存一列**；ADR-0209 落地时加一格，两类在册上分得开。迁移 0032 遇表里已有行即拒：那些行没有人断言过，不替它们补「已断言」。
6. **迁移号**：TF 取 `0032`（方案给 25–31 各票留了号，0024 已被占）；access_identity 照占号表取 `0009`；governance 种子取 `25`。
7. **AT 号取 `AT-TF-120` 至 `125`**，与波 2 各票在 UC-TF-004/005 占的 100–111 隔开。进 main 时若撞号，全仓搜替。
8. **ADR-0207 不部分停用任何已接受 ADR**（方案：落在 ADR-0098 决定二之内），所以 ADR-0098 与 ADR-0208 只在 Links 加前向指针，README 不写「部分停用」。
9. **演示停点不在本票修**，见「偏离与待裁」第 2 条。

### 越权风险点

1. 第 3 条把交集也归「冲突」，与「同身份异内容」共用一个结果格、靠细格区分（TF owner）。owner 若要它单独成格，改的是结果代数。
2. 第 3 条只查同一（原因、事实依据）。同一包裹在同一段上以实际履约和别的原因各登一项不算交集（TF、SA owner，见 ADR-0207 越权风险点 5）。
3. 第 1 条的合并改了公开的应用层构造（TF owner）。生产代码里原本没有调用方，测试已随迁。
4. 演示交接走的作业事实设备：运输履约各口只认 `OFFSITE` 范围，SZX、SIN 的两次交接用了 `SYN-DEVICE-OFFSITE-SG-LM-01` / `SYN-OFFSITE-SG-LM`。设备范围与交接地点不对应，这一格属一线作业端的地盘，本票不动（TF、access-identity owner）。
5. ADR-0207 文末六条越权风险点同样待 owner 复核：断言期、旅程无人核、不自动形成、数量与单位由登记方声明、交集只在同原因同依据上查、自营只认两格。

### 用例

- 应用：`TestAnEndedTrunkParticipationRegistersAnActualFulfillmentOccurrence`（AT-TF-120）、`TestAnOccurrenceNeedsAnEndedParticipationInTheSegment`（121）、`TestMissingProcurementIsNotAcceptedWhileOnlyDeclaredOrJudgedSelfOperationIsNotApplicable`（122，含「承运商登记后改判自营答冲突」）、`TestEachAgreementRefusalIsItsOwnOutcome`（123）、`TestReRegisteringAnswersExistingOrConflict`（124）、`TestMembersAlreadyChargedOnTheSameFactAreAConflict`（125）、`TestTheOccurrenceIdentityIsDerivedFromItsFactAndMemberSet`、`TestAFailingRegistryOrHandoffIsUndecided`、`TestReasonsWithoutAFactPathAndUnassertedConditionsAreNotAccepted`；失败尝试 `TestAFailedPickupRegistersAFailedAttemptOccurrence`（AT-TF-094）、`TestAFailedDeliveryRegistersAFailedAttemptOccurrence`（AT-TF-095）、`TestASucceededAttemptDoesNotFormAFailedAttemptOccurrence`、`TestFailedAttemptFactsSeparateUnreadableFromMissingAndMalformed`。
- 领域：实际履约、派送失败尝试、身份派生、发生条件、内容比对、原因字面量各一条。
- 真库：`TestAChargeOccurrenceAndItsFormedEnvelopeCommitAndRollBackTogether`（保存与 Outbox 同一笔、指纹 ID、载荷三键）、`TestTwoValidityVersionsOfAnOccurrenceShareAPartition`、`TestTheChargeOccurrenceHandoffRefusesNoTransactionAndABlankKey`、`TestCoverageFindsAnotherLiveOccurrenceSharingAMemberOnTheSameFact`、`TestChargeOccurrenceWritesRefuseToRunOutsideATransaction`（含交集一问无事务即拒）、`TestTransportChargeOccurrenceRegistrationGrantPassesTheDecisionKindCheck`。
- 装配：`TestChargeOccurrenceRegistrationRunsOnRealPostgres`、`TestAnUndecidedChargeOccurrenceRollsBackTheWholeTransaction`；HTTP 两条（201 只给新落一版、响应无金额键）。
- **派发**：`cmd/parcel-dispatch` 的 `TestAChargeOccurrenceRegisteredByTransportFulfillmentFiresABuyEvaluationRequest`——发生项经 TF 真登记用例与真交接口入队，钉生产方类型串等于 `sainbox.ChargeOccurrenceFormedEventType`、载荷三键被消费门认下，一拍之后恰一份 BUY 请求，协议是「对象/版本」（26 评审要的那一条）。

### 验证

- `go build ./...` 干净；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/settlementaccounting/adapters/...`、`./scripts/demo-seeds/...`，干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- 带真库 `go test -p 1 -count=1`（最后一次 rebase 后在 `451fbf4d` 之上）：`./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./internal/architecture/`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/settlementaccounting/adapters/transportfulfillment/`、`./internal/settlementaccounting/adapters/inbox/`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./scripts/demo-seeds/...`，全绿。早一轮另跑过 `./internal/settlementaccounting/adapters/...` 全部，全绿。未跑全量，未重生成机制清点。
- 一次性库 `seed.sh` 三次都退 0（最后一次在 `451fbf4d` 之上），用完即删。
- **演示实走**（一次性库 `dcg27demo`，种子原样灌，起 `parcel-api` 与 `parcel-dispatch`，配置员令牌，证据 `S`；rebase 后重走一遍，结果相同）：
  1. `submit-one-shipment.sh` 提交一单，受理后形成初始路由，包裹 `DPCL-…`。
  2. 交接口登 SZX-GATE → `SYN-PARTY-CARRIER-TRUNK-01`（`HANDED_OVER`，立段 `SYN-SEG-SZX-SIN-01`、`LINEHAUL`）答 201。
  3. 此时登 `ACTUAL_FULFILLMENT` 答 200 `INPUT_NOT_ACCEPTED` / `PARTICIPATION_NOT_ENDED`。
  4. 交接口登承运商 → SIN-HUB 答 201，`participationEnd` 为 `PARTICIPATION_ENDED`。
  5. 再登 `ACTUAL_FULFILLMENT`（服务方 `SYN-PARTY-CARRIER-TRUNK-01`、协议 `SYN-SUPPLIER-TRUNK-CN-SG-01` `v1`、数量 1、单位 `SEGMENT`、旅程 `SYN-JRN-<包裹号>`、断言一句）**答 201 `REGISTERED`**：法人 `SYN-LE-01`、协议 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`、依据 `FULFILLMENT-SEGMENT/SYN-SEG-SZX-SIN-01`、业务时间取 SIN 交接时刻。
  6. 派发：`transport-fulfillment.charge-occurrence.formed` → `PUBLISHED`，结算消费门入账；**BUY 请求 `FIRED`**（`evaluation_request` 一行：`BUY_SUPPLIER_COST`、`ACTUAL_FULFILLMENT`、`V1`、费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`、协议 `…/v1`）；`evaluation-request.submitted` → `PUBLISHED`；**计价形成评价** `parcel-pricing.evaluation.recorded` → `PUBLISHED`。
  7. **停点**：评价 `PENDING`，问题 `ZONE_UNRESOLVED`（「the price card binds no zone catalogue and the input names no zone」）；结算 `form-supplier-expected-cost` 已消费并按 ADR-0200 入账，**供应商预期成本 0 行**。
  8. 同一口的对照格：同内容重登 200 `EXISTING`（Outbox 仍只一封、BUY 请求仍一份）；服务方填末端承运商 200 `PROVIDER_MISMATCH` 带回约定供应商；缺协议版本 200 `INPUT_NOT_ACCEPTED` / `PROCUREMENT_CONTEXT_MISSING`；声明自营 200 `CONFLICT` 回显原项；版本 `v9` 200 `AGREEMENT_NOT_REGISTERED`；查阅员 403 `OPERATOR_NOT_GRANTED`；无令牌 401 `OPERATOR_CREDENTIAL_REJECTED`。

### 偏离与待裁

1. **偏离：完成判据「棘轮剪掉一条」无行可剪。** `internal/architecture/production_wiring_baseline.txt` 在当前 main 上只剩两条，都不属 TF：失败尝试费那条领域工厂早在 tf-unwired-seven/04 就剪了。票面点名的失败尝试费处理器构造函数以 `New` 开头、在应用层，本来就不在这道门禁的网内（门禁只收 `internal/<上下文>/domain` 下非 `New*` 的导出顶层函数）。它现在并进了登记用例，经 `cmd/parcel-api` 接上生产，棘轮要的东西已经兑现。本票新增的领域工厂都有生产调用点，`./internal/architecture/` 全绿。
2. **待裁：演示停在 BUY 评价 `ZONE_UNRESOLVED`，到不了 ADR-0200 的供应商预期成本。** 这是 ADR-0189 越权风险点 3 与票 30 完成记录的「待裁」，不是本票的缺陷：演示成本卡没绑分区目录，按发生项造的计价输入也不带分区。两条路：
   - **甲（建议，演示租户取值）**：演示成本卡绑分区目录 `SYN-CAT-ZONE-CN-SG`，走 ADR-0189 决定一第一条。按发生项造的输入已带邮编路线（`200000`→`018000`），经目录解出 `Z1`，与今天三段各 `Z1` 同价。**要先等 [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md) 落地**：在那之前候选成本取数不经读数补全，绑了目录的卡恒答待判断，初始路由会退回 `CANDIDATE_COSTS_PENDING`。改的是 `seedgen` 的成本卡与模板夹具同摘要，地盘同票 30。建议立票「演示成本卡绑分区目录」，Blocked by routing-first-cut/20。
   - **乙（产品机制，留给 PP、SA、NR owner）**：按发生项取段计价区域。实际履约段要经计划段连到 NR 的线路段，再读段计价区域，跨三个上下文，需要新 ADR。真实租户若用按线路计价、不绑目录的干线卡，迟早要这一条；ADR-0189 越权风险点 3 一直挂着。
3. 结果代数多出的细格 `MEMBERS_ALREADY_COVERED` 未在管理台出现：管理台没有发生项登记页，本票不建（地盘外）。
