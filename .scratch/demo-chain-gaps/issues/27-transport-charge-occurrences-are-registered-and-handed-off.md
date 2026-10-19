# 27 运输收费发生项显式登记与已形成信封（失败尝试、实际履约）

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `98affcab`：148 ok / 0 FAIL；TF 链第二波同批，见文末「进 main 记录」）。此前 in-progress——完工、评审已修订，待进 main，分支 claude-dcg27（评审记录见文末）。
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
- 演示：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) → BUY 请求 → 计价评价 → ADR-0200 形成供应商预期成本（需 [30](./30-buy-cost-card-declares-amount-rounding.md)）。**本票走到计价评价为止；「评价 → 供应商预期成本」一段移交 [43](./43-demo-cost-card-binds-the-zone-catalogue.md)**（演示成本卡绑分区目录，Blocked by routing-first-cut/20；出自评审）。
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
   - **甲（建议，演示租户取值）**：演示成本卡绑分区目录 `SYN-CAT-ZONE-CN-SG`，走 ADR-0189 决定一第一条。按发生项造的输入已带邮编路线（`200000`→`018000`），经目录解出 `Z1`，与今天三段各 `Z1` 同价。**要先等 [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md) 落地**：在那之前候选成本取数不经读数补全，绑了目录的卡恒答待判断，初始路由会退回 `CANDIDATE_COSTS_PENDING`。改的是 `seedgen` 的成本卡与模板夹具同摘要，地盘同票 30。**已立票 [43](./43-demo-cost-card-binds-the-zone-catalogue.md)**（评审时立），Blocked by routing-first-cut/20。
   - **乙（产品机制，留给 PP、SA、NR owner）**：按发生项取段计价区域。实际履约段要经计划段连到 NR 的线路段，再读段计价区域，跨三个上下文，需要新 ADR。真实租户若用按线路计价、不绑目录的干线卡，迟早要这一条；ADR-0189 越权风险点 3 一直挂着。
3. 结果代数多出的细格 `MEMBERS_ALREADY_COVERED` 未在管理台出现：管理台没有发生项登记页，本票不建（地盘外）。

## 评审记录（2026-10-19，隔离评审一轮；修订提交 `abede89f` 至 `72f5b083`）

阻断 0 项。非阻断 10 项：评审建议进 main 前修的 3 项全修，其余 7 项修 5 项，1 项（只读首版）登记去向，1 项（进 main 协调）按任务口径留到进 main。

### 建议进 main 前修的（全修）

1. **自营两格先按身份查、后取锁，并发下错答不适用**（已修，`abede89f`；用例 `d39079d7`）。`selfOperated` 改为先问交集（`FindCovering` 取事实依据锁），再按身份查；READ COMMITTED 下锁后的新语句读得到对方已提交的那一行。真库两事务用例 `TestASelfOperatedDeclarationRacingAnExternalRegistrationIsAConflict`：A 落册后挂住，B 声明自营、等在锁上，A 提交后 B 答冲突并回显 A。把次序临时改回旧版重跑，该用例失败（B 答 `NOT_APPLICABLE`/`SELF_OPERATED_DECLARED`），证明它钉得住。ADR-0207 决定二写明这个次序与理由。
2. **断言人身份未入册，ADR 与迁移头注言过其实**（已修，`2e923bea`、`72f5b083`）。评审给了两条路，取第二条（让断言人入册），理由见下面代决 1。
   - TF 运营决定认证方 `tfhttp.OperatorAuthenticator` 改交 `OperatorIdentity`（租户与操作者，操作者写成「发行方#sub」），与 PS、NO 的运营决定认证方同一个写法；其余各口仍经 `tenantFor` 只取租户，签名不变（dcg23 新加的委托提交口照旧编译）。
   - 命令加 `Registrant`（取自认证结果，载荷里不收；载荷带 `registrant` 键答 400）；领域加 `RegistrantReference`，`NewAssertedOccurrenceCondition(registrant, assertion)`，`OccurrenceCondition.AssertedBy()`；缺断言人答未受理 `REQUEST_MALFORMED`。
   - 迁移 `transport_fulfillment/0032` 就地加 `condition_asserted_by` 一列（尚未进 main，故不另起迁移号），CHECK 改为断言那一格两件必带、不得为空；库面 CHECK 用例补两条。HTTP 回显 `conditionAssertedBy`。
   - ADR-0207 决定三、六、Consequences 与越权风险点 1 改成如实的说法；迁移头注同步。
3. **端到端判据没达成，缺口只留在完成记录里**（已处理，`6cf5cb0b`、`72f5b083`）。立票 [43](./43-demo-cost-card-binds-the-zone-catalogue.md)「演示成本卡绑分区目录」，Blocked by routing-first-cut/20，地盘同票 30；本票完成判据那一条标「移交 43」，「偏离与待裁」第 2 条、ADR-0207 Consequences 与种子 README「已知边界」都指向 43。

### 其余非阻断

4. **按段计费逐件登记会把同一段算多次**（已修，文档，`72f5b083`）。ADR-0207 越权风险点 4 补一句，交 TF、PC owner 定，并写明 ADR-0209 数量形落地时 `PER_SEGMENT` 应在段级去重；种子 README 数据故事第 8 步补登法提示（同段几件包裹、按段计费时作为一项的成员一并登记）。
5. **重放与自营只读首版 `V1`**（登记去向，未改代码）。今天没有生产修订入口（`ReviseValidity` 没有生产调用方），也没有修订入口的票。写进 ADR-0207 Consequences 的「已知限制」一条：修订入口落地时，重放与自营两条路径改读链头，链头已失效时另行作答。不另立票，理由是改法依赖修订入口的形状，修订入口立票时从 ADR 后果取这一条即可。
6. **咨询锁没有两事务用例**（已修，`d39079d7`）。`TestConcurrentRegistrationsWithOverlappingMembersSerializeOnTheFact`：A 登一个包裹、落册后挂住，B 登它和另一个包裹，B 等在事实依据锁上，A 提交后 B 答冲突 `MEMBERS_ALREADY_COVERED` 并回显 A，册上一项、信封一封。两条并发用例共用 `cmd/parcel-api` 已有的 `awaitAdvisoryWaiters` 与 `awaitDone`，按锁空间认等锁；装配为此拆出 `chargeOccurrenceDeps`。
7. **UC-TF-002/006 步 7 指错**（已修，`72f5b083`）。改为「登法见 ADR-0207 决定一，AT 见本用例 `AT-TF-094`（`AT-TF-095`）」。
8. **协议核对之后形成失败被说成请求有错**（已修，`abede89f`）。改为返回 error，由装配点回滚整笔、响亮报 500；用例 `TestAnAgreementCheckHandingBackNoLegalEntityIsRaised`（核对口答在册却不带法人与快照：上抛、什么都不写）。
9. **测试注释计数**（已修，`d39079d7`）。改为「未配置与渠道换口的用例都钉在编排之前」。
10. **进 main 协调**（未做，按任务口径留到进 main）。机制清点未重生成；`seed.sh` 的 governance 追加次序、`docs/adr/README.md` 行序、UC-TF-005 与 32 的合并照方案「并行车道」的令牌顺序办。评审修订新增两处要合并的：
    - `internal/transportfulfillment/adapters/accessidentity/operator_authenticator_test.go` 的决定映射表，本票补了发生项登记一行，dcg23 在同一位置追加委托提交一行，取并集。
    - `internal/transportfulfillment/adapters/http/operator_decision_intake.go`(+test)：本票改了认证方的返回类型与替身，dcg23 追加的委托提交口经 `tenantFor`、用同一个 `authenticatorFake{}`，合并后照旧编译，文本冲突取并集。
    - 票号：本次立的 43 已避开在途分支的 40（dcg17、dcg23 各一张，已撞号）与 42（dcg32）；进 main 时若 41、43 另有人占，全仓搜替。

### 代决与越权风险点（评审修订新增）

- **代决 1：断言人入册，不收窄说法。** 方案 D07-1 要的是「断言记在运营决定上」；断言期内条件由人判断，只留「凭什么」不留「谁」，审计查不到该去问谁，收窄说法等于把方案要的那一半删掉。PS、NO 的运营决定口已经把操作者带进命令（复核人、决定方、处置人），TF 照同一写法补上，不是新造机制。
- **代决 2：断言人放在发生条件里，不放在登记记录上。** 断言人是「登记者断言」这一格依据的一部分；ADR-0209 的「按协议声明判」那一格没有人断言，库面 CHECK 只要求断言那一格带断言人。日后若要「无论哪种依据都记登记者」，另加登记者一列，不改这一列的含义。
- **代决 3：断言人不进内容比对。** 另一位操作者照原样重报答`已存在`、回显原断言人；原话不同才是异内容。越权风险点 7（TF owner）：第二位操作者的断言不另留痕；断言人随发生项回显在登记答复里，持这一种决定授予的操作者都看得到。
- **代决 4：缺断言人答 `REQUEST_MALFORMED`，与缺租户同一格。** 两者都取自认证结果，缺了是入口漏了一道；HTTP 入口在编排之前已答 `ErrOperatorIdentityMissing`，编排这一格只防直调。
- **越权风险点：认证方返回类型改动波及 TF 全部运营决定口的接线**（TF owner）。其余各口行为不变、仍只取租户；改动只在 TF 的 HTTP 适配器与 access-identity 适配器两处。

### 验证（修订后）

- `go build ./...` 干净；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/settlementaccounting/adapters/...`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./scripts/demo-seeds/...`，干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- 带真库 `go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./internal/accessidentity/...`、`./internal/architecture/`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/settlementaccounting/adapters/transportfulfillment/`、`./internal/settlementaccounting/adapters/inbox/`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./scripts/demo-seeds/...`，全绿。未跑全量，未重生成机制清点。
- 一次性库 `dcg27rev` 上 `seed.sh` 退 0，发生项表三列 `condition_basis`、`condition_asserted_by`、`condition_assertion` 在、0 行；用完已删库。
- 演示动线没有在修订后重走：修订只动了登记答复多一个键、册上多一列，以及自营路径的查询次序，HTTP 与装配两层的用例已经覆盖到。

## 进 main 记录（2026-10-19）

- TF 链第二波 17、23、27、32 与 first-party-api/00 合成一批，按 17 → 23 → 27 → 32 → first-party-api/00 叠到 main 上，整批快进推送，与本记录同一次推送。
- 合批时的处理：
  - README 的 ADR 行按号合并。
  - 决定种类：23 与 27 各加一种，枚举与测试两边都留。access_identity 迁移 0008、0009 与另一会话的 0011 三份 CHECK 全集一致（15 种）。
  - `endpoints.go`、`main.go`、`unwired_orchestration.go`、演示种子 JSON 与 `seed.sh` 的追加行两边都留。
  - 23 与 27 在 TF application 包里同名：27 的协议读不动一格改名 `ChargeSupplierAgreementUnavailable`，测试替身加 `charge` 前缀。
  - UC-TF-005 的 AT 表按号排：32 的 110–116，27 的 120–125。
  - AT 覆盖盘点计数表按两侧增量重算。
  - 种子 README 文件表 `operator-channel` 一行去重。
  - 23 立的 40 号票与 17 立的 40 号票撞号，23 那张改为 41，ADR-0202 风险点 11 同步。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `98affcab`：148 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
