# 29 供应商协议声明收费发生条件

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg29（完成记录见文末）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)
地盘：PC `domain/publication_canonicalization_supplier_agreement.go`、`supplier_agreement.go`、发布用例、http 草稿载荷、postgres + 迁移 PC 新号；TF 登记时判条件；种子 `commercial/publish-batch.json` v2；ADR-0209、PC CONTEXT。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-1，「需要的 ADR」ADR-0209。

## 做什么

- PC 供应商协议正文加「收费发生条件」文法：逐原因声明计费与否，加数量形（方案 D07-1）；发布成新版本，旧版本不可变。
- TF 登记时按协议快照判条件，取代 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的登记者断言；没声明与声明不计费的答法照方案「需要的 ADR」ADR-0209。
- 演示干线协议以 v2 重新发布（方案 D07-1）。
- 起草 ADR-0209；PC CONTEXT 补收费发生条件。

## 完成判据

- 新摘要文法，v1 不变。
- 没声明答 `未配置`。
- 不计费的原因有业务格。
- 数量形定出数量与单位。
- 真库测试。
- 演示：用 v2 干线协议登实际履约，不再需要断言。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg29`，基于 `origin/main` `38916b00`）

### 做了什么

- **PC 文法**（`domain/supplier_agreement_charge_condition.go`，`supplier_agreement.go`）：原因词 `TransportChargeReason`（与 TF 四个原因逐字相同）、数量形 `ChargeQuantityForm`（`PER_SEGMENT` / `PER_PIECE` / `PER_ATTEMPT` / `PER_BOOKING`）、一条声明 `ChargeOccurrenceCondition`（计费带数量形 / 明确不计费）、一节 `ChargeOccurrenceConditions`（至少一条、每原因至多一条、按原因序排好）；数量形按原因配的表只在领域一处（`quantityFormsByReason`）。`SupplierAgreement.WithChargeConditions` / `ChargeConditions()`；批文、表单载荷与规范化折回共用 `ChargeOccurrenceConditionOf`。
- **新摘要文法**：带条件的正文以 `PCC-2` 规范化，不带的仍是 `PCC-1` 逐字节不变；`RehydratePublicationContent` 两版都认、标错版本即拒；对账门对本构建认得的另一版答`不等`。演示 `v1` 的摘要实测仍是 `PCC-1:72457b0d…`。
- **发布用例 / 批文 / 表单 / 库**：`SupplierAgreementBodyDeclaration.ChargeConditions`；批文 `supplierAgreementBody.chargeConditions[]`（`charged` 必给）；表单载荷同键、逐条问题落在 `supplierAgreement.chargeConditions[<序>]`；迁移 `party_commercial/0038_supplier_agreement_charge_condition.sql`（子表，原因与数量形封闭集、计费必带数量形）；两只读口与正文同一条语句读回（`jsonb_agg` 标量子查询），重放比对含条件。
- **TF 按协议判**：协议核对口在册格多交 `ChargeConditions`（适配器按字面译）；登记用例在协议否定格之后判——没声明答 `NOT_CONFIGURED` / `CHARGE_CONDITION_NOT_DECLARED`，声明不计费答 `NOT_CHARGED`，两格带回作答的协议、不落册不问册；计费按数量形数出数量与单位，依据记 `AGREEMENT_DECLARED`。命令与载荷去掉 `conditionAssertion` / `quantity` / `unit`（带来 400），`CONDITION_NOT_ASSERTED` 退出代数。
- **按范围计的覆盖**：`ChargeOccurrenceCoverageQuery` 加范围、`WholeScope`、`ScopeUnits`；按段 / 按尝试 / 按订舱计的一项盖住整个范围，冲突细格 `SCOPE_ALREADY_CHARGED`；串行锁改按（租户、原因、主要业务范围）；领域给出范围的 `AttemptOccurrenceScope` / `SegmentOccurrenceScope`（形成与登记前同一处）。
- **登记者**：迁移 `transport_fulfillment/0034_charge_occurrence_agreement_declared_condition_and_registrant.sql`——依据封闭集加 `AGREEMENT_DECLARED`（不带断言人与原话）、`registered_by` 一列（断言行由断言人回填后 NOT NULL，断言行两列相等）、按范围的索引；答复回显 `registeredBy`。
- **种子**：`commercial/publish-batch.json` 加干线 `v2`（`PCC-2:55a33e7a…`，实际履约按件计、失败尝试不计费，订舱与取消不声明；批准 `SYN-APPROVAL-2026-020`）；`v1` 不动。受控发布答复的钉值 17 → 18 项。
- **文档**：起草 ADR-0209；ADR-0207 Status 与 Links 加前向指针，`docs/adr/README.md` 该行写部分停用、追加 0209 一行；PC / TF `CONTEXT`、GLOSSARY「收费发生条件」、CONTEXT-MAP 的 PC→TF 一条；UC-PC-001 `AT-PC-051`–`053`；UC-TF-005 结果行、步 9 与 `AT-TF-166`–`170`；`PAR-SET-03` 指登口；种子 README 第 8 步与演示脚本各追加一段。
- **后续票**：[50](./50-commercial-versions-have-no-closing-entry-so-agreement-versions-pile-up.md)（商业版本没有收尾写口）、[51](./51-supplier-agreement-catalogue-does-not-show-charge-conditions.md)（协议目录不带条件），都是 needs-triage。

### 代决

1. **数量形按原因配**：订舱、取消配按订舱 / 按件；失败尝试配按尝试 / 按件；实际履约配按段 / 按件。表只在 PC 领域；TF 按数量形计数时不判原因（按范围计的三形各计 1，按件计成员数），信 PC 发布时已判——再判一遍就是第二张表。
2. **换号成 `PCC-2`，按内容取最低一版**，不沿服务产品 / 客户合同「加键不换号」的先例：`docs/adr/README.md` 里 ADR-0014 那一行写明此后照换号；只认 `PCC-1` 的构建会把条件当未知键丢掉。按构建一律写 `PCC-2` 会让 v1 的摘要变，否决。
3. **不计费单独成格 `NOT_CHARGED`**，不进`不适用`：续办不同（不适用什么都不用做；不计费要么认、要么发新版协议），也不改 ADR-0207 决定二「不适用只有两格」。没声明答 `NOT_CONFIGURED`。两格都带回作答的协议、不问册。
4. **断言期退场的落法**：三键删去、带来 400，不静默丢；册上断言项照旧可读、不改写。这部分停用了 ADR-0207 决定三的两句（方案「需要的 ADR」表写 ADR-0209 对已接受 ADR「无」作用——实际上决定三写的就是「协议能声明之前」，到期即停，照 README 顶部规则如实标部分停用）。
5. **按范围计的一项盖住整个范围，串行锁改按范围**：回答 ADR-0207 越权风险点 4。断言期登记方报的同词单位（`SEGMENT` / `ATTEMPT` / `BOOKING`）同样算整体计过。自营问册也认按范围计过的项。锁键改动部分停用 ADR-0207 决定四那一句的键。
6. **每一项记登记者**（`registered_by`）：按协议判之后不另记就找不到是谁报的服务方、协议版本与成员；照票 27 评审代决 2「另加登记者一列，不改这一列的含义」落。命令里的 `Registrant` 因此仍有用处。
7. **演示 v2 的条件**：实际履约按件计（演示 BUY 成本卡按包裹计重，按件同口径）；失败尝试不计费（干线承运方不做揽派，演示「不计费」这一格）；订舱、取消不声明（演示「未配置」这一格）。区间与 v1 同（2026-01-01 起不设终点），v1 不收尾（没有收尾写口，见 50）。
8. **适用冲突核对**：供应商协议不进任何按范围的解析（解析键的必需依据不含 `SUPPLIER_AGREEMENT`，领域解析不选这一册），各消费方按「对象/版本」点读，v1、v2 同范围并存不撞适用冲突；新版进 `SYN-SCOPE-01` 会推动该范围的视图修订，种子在任何解析之前灌完。任务口径里点名的 ADR-0063 讲收寄硬资格证明，与本票无关；实际核的是 ADR-0014、ADR-0126、ADR-0080（快照指称）与 ADR-0207。
9. **号**：PC 迁移 `0038`、ADR-0209、票 50–51、`AT-TF-166`–`170` 照协调表；`AT-PC` 取 UC 现有最大 `050` 之后的 `051`–`053`；**TF 迁移取 `0034`**——协调表没给票 29 分 TF 号，`0033` 给了票 33，取其后一号，进 main 若撞号改号。

### 越权风险点

1. ADR-0209 文末八条（文法偏简；取消的配形；按件以成员数计；按范围覆盖认单位词；v1 与 v2 并存无收尾入口；断言项不迁；登记者不进内容比对；`NOT_CHARGED` 不落记录）待 PC / TF owner 复核，第 8 条同时待 SA owner。
2. 代决 4、5 部分停用 ADR-0207，与方案「需要的 ADR」表「无」作用不一致（TF owner；方案 owner 可据此改表）。
3. 锁键从事实依据放宽到主要业务范围：失败尝试同一次到场的各对象登记互相串行（变慢、不错判）；票 28 定订舱与取消的范围时要沿用同一键（TF owner）。
4. 方案跨票通则 X1 仍写干线「采用 v1」；演示文档与种子 README 已写明自此读作 `v2`，方案原文未改（方案 owner）。
5. 管理台没有发生项登记页，也没有协议条件读面（见 51）；`NOT_CONFIGURED` / `NOT_CHARGED` 只能从 API 答复看到（PC、TF owner）。

### 用例

- PC 领域：`TestEachReasonTakesOnlyItsQuantityForms`、`TestChargeConditionsAreOnePerReasonAndOrdered`（AT-PC-051）、`TestAnAgreementTakesItsChargeConditionsOnce`、`TestChargeConditionsCanonicalizeAsTheNextShape`、`TestBothShapesRehydrateAndAMislabelledDocumentIsRefused`、`TestADeclaredDigestOfTheOtherKnownShapeIsAMismatch`、`TestChargeConditionWordsTranslateInOnePlace`（AT-PC-052）；封闭集反查表加两行。
- PC 应用 / 批文 / 表单 / 库：`TestASupplierAgreementPublishesItsChargeConditionsWithTheBody`、`TestASupplierAgreementBodyTranslatesItsChargeConditions`、`TestSupplierAgreementPayloadCarriesChargeConditions`、真库 `TestChargeConditionsRoundTripWithTheAgreementBody`、`TestSavingChargeConditionsTwiceIsAReplayAndChangedConditionsConflict`、`TestTheChargeConditionTableRefusesWhatTheGrammarCannotSay`（AT-PC-053）；`TestTheControlledPublishAnswersTheDemoSeedAlike` 钉 18 项。
- TF 领域 / 应用：`TestEachQuantityFormMeasuresItsOccurrence`、`TestAgreementChargeConditionsArePerReason`、`TestAnAgreementDeclaredConditionCarriesOnlyItsRegistrant`；`TestAnAgreementThatDeclaresNoConditionForTheReasonIsNotConfigured`（166）、`TestAnAgreementThatDoesNotChargeTheReasonAnswersItsOwnCell`（167）、`TestTheQuantityFormSetsQuantityAndUnit`（168）、`TestAScopeCountedOccurrenceCoversTheWholeScope`、`TestDeclaringSelfOperationOverAScopeChargedSegmentIsAConflict`（169）；票 27 的各条改为按协议判（AT-TF-120 断言依据、数量单位）。
- TF 适配器 / 库 / HTTP：`TestTheRegisteredCheckCarriesTheDeclaredChargeConditions`、真库 `TestTheSyntheticSupplierAgreementsAreCheckedThroughTheRealPointRead`（种子干线 v2 的条件经真库读回）、`TestAnAgreementDeclaredOccurrenceRoundTripsWithItsRegistrant`、`TestCoverageCountsAScopeChargedOccurrenceAcrossItsScope`、CHECK 表加五行；HTTP 三键退场 400、`TestUnconfiguredAndUnchargedConditionsAnswer200WithTheAgreement`（170）。
- 装配 / 派发：`TestChargeOccurrenceRegistrationRunsOnRealPostgres`（v1 答未配置、v2 登成 1 PIECE `AGREEMENT_DECLARED`）、并发 `TestConcurrentPerSegmentRegistrationsChargeTheSegmentOnce`（按段计两笔并发、成员不交，B 等在范围锁上答 `SCOPE_ALREADY_CHARGED`）；派发 `TestAChargeOccurrenceRegisteredByTransportFulfillmentFiresABuyEvaluationRequest` 照旧绿（夹具协议声明按件计）。

### 验证

- `go build ./...` 干净；`go vet` 覆盖 `./internal/partycommercial/...`、`./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-commercial/`、`./internal/settlementaccounting/adapters/...`、`./internal/parcelshipment/adapters/partycommercial/`、`./scripts/demo-seeds/...`，干净；`gofmt -l internal cmd scripts` 为空。
- 带真库 `go test -p 1 -count=1`（55432 DSN）：`./internal/partycommercial/...`、`./internal/transportfulfillment/...`、`./internal/architecture/`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-commercial/`、`./internal/settlementaccounting/adapters/transportfulfillment/`、`./internal/settlementaccounting/adapters/partycommercial/`、`./internal/settlementaccounting/adapters/inbox/`、`./internal/parcelpricing/adapters/transportfulfillment/`、`./internal/parcelshipment/adapters/partycommercial/`、`./scripts/demo-seeds/...`，全绿。未跑全量，未重生成机制清点。
- 一次性库 `dcg29seed` 上 `seed.sh` 退 0：发布批第 9、10、11 项干线 v1、干线 v2、末端 v1 都 `PUBLISHED_EFFECTIVE`；条件子表两行（v2 的 `ACTUAL_FULFILLMENT` 计费 `PER_PIECE`、`FAILED_ATTEMPT` 不计费）；干线两版摘要 `PCC-1:72457b0d…` / `PCC-2:55a33e7a…`。用完已删库。
- **演示实走**（同一次性库，起本分支的 `parcel-api`（`:19229`，操作者、集成、首方三族与治理自授权、隔离读写同取 `SYN-TENANT-01`）与 `parcel-dispatch`（七个变量照演示脚本「前置」），配置员令牌，证据 `S`）：
  1. 场外设备 `SYN-DEVICE-OFFSITE-SG-LM-01` 登 SZX-GATE → `SYN-PARTY-CARRIER-TRUNK-01` 交接，立段 `SYN-SEG-SZX-SIN-DCG29`，答 201。
  2. 此时登 `ACTUAL_FULFILLMENT`（v2）答 200 `INPUT_NOT_ACCEPTED` / `PARTICIPATION_NOT_ENDED`。
  3. 承运商 → SIN-HUB 交接答 201，`participationEnd` 为 `PARTICIPATION_ENDED`。
  4. 指名 `v1` 登记答 200 `NOT_CONFIGURED` / `CHARGE_CONDITION_NOT_DECLARED`，带回 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`。
  5. 载荷带 `conditionAssertion`、带 `quantity`/`unit` 各答 400 `MALFORMED_REQUEST`。
  6. **指名 `v2`、不带断言答 201 `REGISTERED`**：法人 `SYN-LE-01`、协议 `…/v2`、数量 1、单位 `PIECE`、`conditionBasis` `AGREEMENT_DECLARED`、`registeredBy` 为配置员的「发行方#sub」。原样重登答 200 `EXISTING`。
  7. 派发一拍：`transport-fulfillment.charge-occurrence.formed` → `PUBLISHED`；BUY 请求一份（`BUY_SUPPLIER_COST`、`ACTUAL_FULFILLMENT`、`V1`、费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`、协议 `…/v2`）；`evaluation-request.submitted` 消费答未决：计价输入不可得「carried object is not a declared parcel」——包裹是合成号，见偏离 1。
  走完已停进程并删库。

### 偏离与待裁

1. **演示没走 `submit-one-shipment.sh`，包裹用合成号 `SYN-PARCEL-DCG29-01`。** 本机的集成客户端发行方容器挂的是另一 worktree（`idp-parcel-psb31`）里早于首方客户端的 `integration-dex.yaml`（容器 05:20 起），取首方客户端令牌答 `invalid_client`；重启那只共享容器会作废其他会话的令牌，没动它。TF 登记一段不受影响；BUY 评价因此停在计价输入不可得，而不是票 27 记的 `ZONE_UNRESOLVED`。用提交过的真包裹重走时停点照票 27 / 43。
2. `NOT_CHARGED`（失败尝试不计费）没在演示里走：要一次失败的派送 / 揽收尝试事实，单元、HTTP 与真库各层已钉。
3. 方案「需要的 ADR」表说 ADR-0209 对已接受 ADR「无」作用；实际部分停用 ADR-0207 决定三两句与决定四的锁键（代决 4、5），README 已照规则标注。方案原表未改。
4. TF 迁移号 `0034` 不在协调表里（见代决 9）。
5. 种子 README 是热点文件，只追加：数据表 `data/commercial/` 一行仍写「两份供应商协议正文」，第 8 步原段仍是断言期的登法，已在其后追加一段写明作废；进 main 合批时可一并改写原行。
6. 未做：机制清点重生成、AT 覆盖盘点计数（进 main 时统一做）；管理台无相应页面（地盘外，见 51）。
