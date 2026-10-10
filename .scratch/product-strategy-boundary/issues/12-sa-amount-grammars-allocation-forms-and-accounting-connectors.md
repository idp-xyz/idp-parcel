# 12 settlement-accounting：金额文法、分摊与周期费用形态、经营指标方法与账务连接器

Category: enhancement
Status: in-progress——十项均已进 main。第 10 项 `f9c5192e`（ADR-0170）。此前各项见文末进 main 记录。
Blocked by: 无（第 1、5 项的规则登记册与读口是重定级表 PN-07 行第一项的机制缺口，归[票 16](./16-mechanism-gaps-without-a-ticket.md)；缺它们时本票只能先定文法）
地盘：settlement-accounting 领域与应用层（金额、分摊、周期费用、指标），账单接入与财务交换的连接器适配器；规则正文若由 party-commercial 声明，PC 侧另开票。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-07`、`PAR-SET-05`、`PAR-SET-06`、`PAR-SET-07`、`PAR-SET-08`、`PAR-SET-09`、`PAR-SET-10`、`PAR-INT-04`、`PAR-INT-05` 行内「〔ADR-0146 拆分〕」点名的部分。[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-07 行第三项当时记「未核」，本票即其补核。

## 做什么

1. **赔付 / 退款与代垫回收金额的计算文法**（`PAR-COM-07`「赔付/退款责任、限额、比例和免赔依据」、`PAR-SET-08`「金额分配、比例/限额/免赔」）。`SettleClaimAmounts` 只核金额规则版本在不在，金额本身由命令带入（`AmountMinor`）；限额、比例、免赔怎样组成一个金额是方法，各数值是租户取值。规则版本的登记册与读口（`ClaimAmountRuleView`）是重定级表 PN-07 行第一项的机制缺口。落地：ADR-0161，先免赔、再按万分比向下取整、再以限额封顶；三项数值命令 `amount-grammar`，空册不形成金额。金额分配仍属第 2 项。
2. **成本分摊的内置形态**（`PAR-SET-06`「分摊规则」）。`AllocateCosts` 只核分摊规则版本在不在，各份额由命令带入（`Portions`）；按重、按件、按收入等分法归产品，是否适用与选哪种归租户。落地：ADR-0162，三套分法共用最大余数，余数相同按目标标识升序；选用命令 `allocation-form`，空册答未配置，不均摊。各对象权重随分摊交入。
3. **待核：周期费用的计算形态**（`PAR-SET-07`「最低消费、保底量、返利」）。落地：没有既有执行器。价卡最低重量与评价里的最低收费不是这一格。ADR-0165，三套形态加本期不适用；命令 `periodic-fee`；空册不形成周期费用。
4. **待核：经营指标各阶段口径与新指标版本的形成方法**（`PAR-SET-10` 已确认约束栏写的就是这套方法）。核现有指标派生是否按它实现。落地：已按约束栏实现，不新造执行器，不立新 ADR。预估口径只收客户预估费用与供应商预期成本，已确认口径只收客户运营应收、审核应付与关联贷项且贷项不得独自在场，已结算口径只用这三类的核销分配角色。符号：`domain.ComponentRole.admittedBy`、`domain.DeriveOperatingResult`、`domain.OperatingResult.Rederive`、`application.AllocateCostsHandler.Derive`、`application.AllocateCostsHandler.Rederive`。测试：`TestOperatingResultIsDerivedNotEdited`、`TestOperatingResultsDeriveAndRederive`。报告币与截至时点由派生命令交入，空白被拒，没有默认币种或默认时点；组成项空集不派生。
5. **供应商账单审核的越权升级判断结构**（`PAR-SET-05`「越权升级规则」；分权的角色模型归票 07）。`SupplierAuditAuthorityView` 的登记册与读口是重定级表 PN-07 行第一项的机制缺口。落地：ADR-0163，已匹配金额小于或等于上限在权限内，大于上限必须升级且不形成应付；上限命令 `audit-escalation-ceiling`，空册不默认放行。角色模型仍归票 07。
6. **费用归属日的判定形态**（`PAR-SET-09`）。各金额唯一创建用例与既有借贷项纳入后续账期已由 SA 定（机制），不再列为租户证据。落地：原先没有归属日执行器。ADR-0168，形态是来源发生或费用确认，达到截单时刻归到下一日；命令 `charge-attribution`，空册答未配置。账户周期、时区与截单时刻仍是租户取值。
7. **供应商账单接入与财务系统交换的连接器形态**（`PAR-INT-04`、`PAR-INT-05`）。账单接收编排已有；核通用导入 / 导出形态有无，某供应商与某财务系统的格式映射留租户。2026-09-30 通道 2 按 [ADR-0166](../../../docs/adr/0166-accounting-exchange-uses-a-canonical-document.md) 落地这一项：没有通用文件导入或报文导出。内置形态是规范文书，命令 `accounting-connector`。没登记答未配置。本票第 3、4、6、9、10 项未动。
8. **BUY 评价请求的触发面**（[票 05](./05-demo-journey-criterion-evidence.md) 格 17；以下三项 2026-09-24 经用户授权自决补入）。`cmd/parcel-api/assemble_evaluation_request.go` 的 `buildEvaluationRequestOrchestration` 头注写「今天没有运营端点、也没有进程内触发面调它」「谁在什么业务时点为哪些发生项发起请求是产品题，触发面另票」——本项即那张票。落地：ADR-0164，触发面是 `evaluationRequestOrchestration.Trigger`，时点只有发生项形成；发生项原因命令 `buy-evaluation-trigger`，空册答未配置，不发起请求。
9. **SELL 评价到客户费用**（票 05 格 19）。SELL 评价没有请求面，评价已记录信封的消费门只收 BUY·供应商成本，SA 应用层没有由评价形成客户费用的编排。消费门与形成编排是机制；SELL 评价何时发起是产品策略。落地：ADR-0169。消费门只认 SELL·CUSTOMER_CHARGE，BUY 答不处理。形成编排 `FormSellCustomerChargeHandler` 按评价金额写预估客户费用。触发命令 `sell-evaluation-trigger`，空册不发起，不调用 BUY 请求。不接进进程。
10. **结算编排的生产入口与触发面**（票 05 格 20）。据票 05 取证，`NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewAuditSupplierBillHandler`、`NewSettleClaimAmountsHandler` 在 `cmd/` 零引用。装配与入口是机制；何时确认、何时截单是产品策略；账期是租户取值，演示租户经参考配置采用。落地：ADR-0170，六个构造接在 `buildSettlementOrchestrations`；没有单独的审核构造，审核是 `ReceiveSupplierBillHandler.Audit`。确认与截单命令 `settlement-moment`，空册答未配置。账期不写进触发册。

顺带（[票 01](./01-regrade-slices-under-four-criteria.md)「严格复核记录」交来，只改注释）：`PricingInputResolver` 的注释「三只读口今天都不存在」已被 `pp-pricing-input-seams` 01–03、05 推翻。

## 不做

- 不替租户定任何限额、比例、免赔、分摊依据或周期费用数值；不接真实财务系统。
- 公开计价参考序列的来源连接器（`PAR-SET-11`）已有票 `pricing-reference-series-operations/06`，不在本票。

## 完成判据

- 每项要么有执行器（带测试），要么记下已有执行器的证据；登记册对应行同步收短。

## Comments

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `d481f1fb`（通道 3 分支 `mcp3-psb12-01`，第 1 项，ADR-0161，基 `28ba713e`）· 2026-09-30 23:4x**

- **阻断**：无。
- **非阻断**（随票记）：`FormReceivable` 已写入文法结果，但没有用例钉住；若仍保存命令的 `AmountMinor`，`TestClaimAmountsUseTheRegisteredGrammarInsteadOfTheAssertedAmount` 仍绿。`FormClaimAmount` 与 `FormRecovery` 改回主张会红。`claimGrammarStop` 与 `recoveryGrammarStop` 是重复开关。
- **核过无发现**：`AmountGrammar.Compose` 先免赔（不足记 0）、再按万分比向下取整、再以限额封顶；比例封在 0–10000。三项取值在 `amount_grammar_parameter`，命令 `amount-grammar`，不写回金额规则版本册。没登记答 `CLAIM_GRAMMAR_UNCONFIGURED` / `RECOVERY_GRAMMAR_UNCONFIGURED`，不用主张顶上。算出 0 不形成金额。未登租户行。点名真库与领域用例带 DSN `-v` 为 PASS 非 SKIP；`internal/architecture` 过。第 2–10 项未做。`NewSettleClaimAmountsHandler` 仍不在 `cmd/`，属第 10 项。
- **结论：可重放**。

**完成记录（通道 3 · 据完工报转录，第 1 项）**

分支 `mcp3-psb12-01`（基 `28ba713e`），一笔快进推送、未改写：`d481f1fb` 赔付、退款与代垫回收按 ADR-0161 同一套文法算。真库 `TestAmountGrammarsRefuseToRunOutsideATransaction`、`TestEmptyAmountGrammarsStayUnconfigured`、`TestAnAmountGrammarRegistersReplaysAndConflicts`。迁移 `settlement_accounting/0024_amount_grammar_parameter.sql`。未登租户行。

**进 main 记录（2026-09-30 23:4x，通道 1 推送，第 1 项）**

分支 `mcp3-psb12-01@d481f1fb`（已推 origin）在隔离树重放到 `28ba713e` 之上，零冲突：`d481f1fb→161bb646`；清点在代码链尖重生成为 `d1f3f93a`（`settlementaccounting` 生产 110→115、测试 85→87；`settlement_accounting` 迁移 23→24）。
推送方验证：钉 `d1f3f93a`，`gofmt -l` 空，build 与 vet 退 0，点名用例与架构门禁单跑 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `6b47bd0e`（通道 2 分支 `mcp2-psb12-02`，第 2 项，ADR-0162，基 `7a28618c`）· 2026-09-30 23:5x**

- 初评 `c4c9b484`（基 `28ba713e`）两条阻断：迁移前缀与 main 上金额文法的 `0024` 重复；`AllocationForms.SaveAllocationForm` 没有无事务拒证，架构门禁红。作者先重放成 `6b47bd0e`（迁移改为 `0025`），再加 `78a2e5ab`。
- **阻断**：无。`TestAllocationFormsRefuseToRunOutsideATransaction` 与 `TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 带 DSN 为 PASS。
- **非阻断**（随票记）：`AllocateCostsHandler` 仍只收命令里的 `Portions`，不读分法册。ADR-0162 决定四写明不改编排，`UC-SA-006` 步骤 3「分法未登记则待判断」因此只在 `ApportionCosts` 上成立。
- **核过无发现**：`Apportion` 按重、按件、按收入共用最大余数，余数相同按目标标识升序。空册答 `ALLOCATION_FORM_UNCONFIGURED`，不均摊。`NOT_APPLICABLE` 是登记的不适用。权重在当次 `Bases`。未登租户行。未改金额文法与 ADR-0161。第 3–10 项未做。
- **结论：可重放**。

**完成记录（通道 2 · 据完工报转录，第 2 项）**

分支 `mcp2-psb12-02`。`c4c9b484` 已被作者重放替换为 `6b47bd0e`（父 `7a28618c`，迁移 `0025`）；`78a2e5ab` 补无事务拒证，未改写前笔。内置分法 `BY_WEIGHT`、`BY_PIECE`、`BY_REVENUE`，命令 `allocation-form`。未改 `SettleClaimAmounts` / `amount-grammar` / ADR-0161。

**进 main 记录（2026-09-30 23:5x，通道 1 推送，第 2 项）**

分支 `mcp2-psb12-02@78a2e5ab`（已推 origin）在隔离树重放到 `7a28618c` 之上，零冲突：`6b47bd0e→83489825`、`78a2e5ab→3fac7239`；清点在代码链尖重生成为 `08963b62`（`settlementaccounting` 生产 115→120、测试 87→90；`settlement_accounting` 迁移 24→25）。
推送方验证：钉 `08963b62`，`gofmt -l` 空，build 与 vet 退 0，无事务拒证与架构门禁单跑 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `73c99b30`（通道 3 分支 `mcp3-psb12-05`，第 5 项，ADR-0163，基 `801cd6d8`）· 2026-10-01 00:0x**

- **阻断**：无。`TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 带 DSN 为 PASS。`TestAnAuditedAmountAboveTheCeilingMustEscalate`、`TestAnAmountWithinTheCeilingStaysInsideAuthority`、`TestARegisteredZeroCeilingIsNotUnconfigured` 与三条真库用例 PASS。
- **核过无发现**：已匹配金额小于或等于上限在权限内，大于上限答 `AUDIT_MUST_ESCALATE`，不形成应付，也不记成拒绝。上限另册，命令 `audit-escalation-ceiling`，不写进审核授权册。没登记不默认放行。上限 0 是一份登记。未登租户行。未改金额文法、分摊分法、`assemble_evaluation_request.go`。
- **结论：可重放**。

**完成记录（通道 3 · 据完工报转录，第 5 项）**

分支 `mcp3-psb12-05`（基 `801cd6d8`），一笔未改写：`73c99b30`。迁移 `settlement_accounting/0026_audit_escalation_ceiling.sql`。

**进 main 记录（2026-10-01 00:0x，通道 1 推送，第 5 项）**

重放到 `801cd6d8` 之上，零冲突：`73c99b30→9a3b9e9a`。

**评审 ← 通道 1 · 钉 `f4f70cc0`（通道 2 分支 `mcp2-psb12-08`，第 8 项，ADR-0164，基 `801cd6d8`）· 2026-10-01 00:0x**

- **阻断**：无。架构门禁带 DSN 为 PASS。`TestTheProductionTriggerDoesNotRequestWhenTheReasonIsNotRegistered` 与触发面用例 PASS。
- **核过无发现**：触发面在 `evaluationRequestOrchestration.Trigger`，时点只有 `OCCURRENCE_FORMED`。原因没登记答 `BUY_EVALUATION_TRIGGER_UNCONFIGURED`，不发起请求。未改审核、金额文法、分摊分法。
- **重放时改了一处**：与第 5 项同占迁移前缀 `0026`。第 5 项保留 `0026_audit_escalation_ceiling.sql`，本项文件改为 `0027_buy_evaluation_trigger.sql`，ADR-0164 后果那句一并改。登记命令与越权升级命令在 `parcel-settlement-register` 里都留下。
- **结论：可重放**。

**完成记录（通道 2 · 据完工报转录，第 8 项）**

分支 `mcp2-psb12-08`（基 `801cd6d8`），一笔未改写：`f4f70cc0`。作者迁移文件名是 `0026_buy_evaluation_trigger.sql`。

**进 main 记录（2026-10-01 00:0x，通道 1 推送，第 5 与第 8 项）**

第 8 项在 `9a3b9e9a` 之上重放，文档与登记命令冲突按两边都留解开：`f4f70cc0→7c084917`（含迁移改号 `0027`）。清点在两笔代码之上重生成为 `eef58da6`（`settlementaccounting` 生产 120→130、测试 90→95；`settlement_accounting` 迁移 25→27）。
推送方验证：钉 `eef58da6`，`gofmt -l` 空，build 与 vet 退 0，两边架构门禁单跑 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。两条分支作封存出处。

**评审 ← 通道 1 · 钉 `dfd48b24`（通道 2 分支 `mcp2-psb12-07`，第 7 项，ADR-0166，基 `ca60f16d`）· 2026-10-01 00:2x**

- **阻断**：无。`TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 带 DSN 为 PASS。`TestAnUnregisteredAccountingConnectorStaysUnconfigured`、`TestAccountingConnectorsRefuseToRunOutsideATransaction`、`TestAccountingConnectorOnlyAcceptsCanonicalExchange` PASS。
- **非阻断**（随票记）：`AdmitAccountingExchangeHandler.Admit` 放行后不调用账单接收，也不调用资金事实采用。ADR-0166 决定三写明如此。
- **核过无发现**：内置形态只有 `CANONICAL`。命令 `accounting-connector`。没登记答 `ACCOUNTING_CONNECTOR_UNCONFIGURED`。迁移 `0029`，`0028` 留给周期费用。未登租户行。未改周期费用，未改 ADR-0161 至 0164 的决定正文。
- **结论：可重放**。

**完成记录（通道 2 · 据完工报转录，第 7 项）**

分支 `mcp2-psb12-07`（基 `ca60f16d`），一笔未改写：`dfd48b24`。既有通用导入/导出没有。新做规范文书形态。

**进 main 记录（2026-10-01 00:2x，通道 1 推送，第 7 项）**

重放到 `ca60f16d` 之上，零冲突：`dfd48b24→cab6613c`。清点在代码链尖重生成为 `97f37a7c`（`settlementaccounting` 生产 130→135、测试 95→97；`settlement_accounting` 迁移份数 27→28，文件号是 `0029`，`0028` 空给第 3 项）。
推送方验证：钉 `97f37a7c`，`gofmt -l` 空，build 与 vet 退 0，架构门禁与点名真库用例 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `85270ef1`（通道 3 分支 `mcp3-psb12-03`，第 3 项，ADR-0165，基 `ca60f16d`）· 2026-10-01 00:3x**

- **阻断**：无。架构门禁带 DSN 为 PASS。`TestMinimumSpendShortfallIsTheGapBelowTheFloor`、`TestVolumeFloorChargesTheMissingQuantityAtTheRegisteredRate`、`TestTieredRebateAppliesEachBandOnlyToItsSlice`、`TestEmptyPeriodicFeesStayUnconfigured`、`TestPeriodicFeesRefuseToRunOutsideATransaction` PASS。
- **非阻断**（随票记）：执行器没有接进截单或确认费用。ADR-0165 越权风险点 3 写明如此。
- **核过无发现**：没有既有周期费用执行器。最低消费是补差，保底量是不足数量乘单价向下取整，阶梯返利按档只乘本档。`NOT_APPLICABLE` 不造金额。没登记不形成周期费用。算出 0 不形成一笔费用。迁移 `0028`。未登租户行。未改连接器，未改 ADR-0161 至 0164 的决定正文。
- **重放**：父仍是 `ca60f16d`，main 已有第 7 项。文档与登记命令冲突按两边都留解开。
- **结论：可重放**。

**完成记录（通道 3 · 据完工报转录，第 3 项）**

分支 `mcp3-psb12-03`（基 `ca60f16d`），一笔未改写：`85270ef1`。命令 `periodic-fee`。

**进 main 记录（2026-10-01 00:3x，通道 1 推送，第 3 项）**

重放到 `18e39f1a` 之上，登记命令与文档冲突按两边都留解开：`85270ef1→6b3d7d80`。清点在代码链尖重生成为 `4ab9a589`（`settlementaccounting` 生产 135→140、测试 97→99；`settlement_accounting` 迁移份数 28→29，文件号 `0028`）。
推送方验证：钉 `4ab9a589`，build 退 0，架构门禁与点名用例 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `b6cd7507`（通道 3 分支 `mcp3-psb12-04`，第 4 项，无新 ADR，基 `39998c14`）· 2026-10-01 00:4x**

- **阻断**：无。只改两份文档。推送方在 main 上读过 `ComponentRole.admittedBy`：预估只收客户预估费用与供应商预期成本，已确认只收客户运营应收、审核应付与供应商贷项，已结算只收这三类的核销分配角色。与 `PAR-SET-10` 约束栏一致。
- **结论：可重放**。不重跑全量。

**完成记录（通道 3 · 据完工报转录，第 4 项）**

分支 `mcp3-psb12-04`（基 `39998c14`），一笔未改写：`b6cd7507`。不新造执行器，不立 ADR。

**进 main 记录（2026-10-01 00:4x，通道 1 推送，第 4 项）**

重放到 `39998c14` 之上，零冲突：`b6cd7507→f8ead9ad`。无代码、无清点。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `6b8f00e1`（通道 2 分支 `mcp2-psb12-06`，第 6 项，ADR-0168，基 `39998c14`）· 2026-10-01 00:4x**

- **阻断**：无。架构门禁带 DSN 为 PASS。`TestAttributionDateRollsForwardAtCutoff`、`TestJudgeChargeAttributionStaysUnconfiguredWhenTheFeeItemIsNotRegistered`、`TestChargeAttributionsRefuseToRunOutsideATransaction` PASS。
- **非阻断**（随票记）：截单编排不调用这次判定。ADR-0168 越权风险点 3 写明如此。
- **核过无发现**：原先没有归属日执行器。形态是 `SOURCE_OCCURRED` 或 `CHARGE_CONFIRMED`，达到截单时刻归下一日。没登记答 `CHARGE_ATTRIBUTION_UNCONFIGURED`。包裹创建、收寄、签收不是形态。迁移 `0031`。未登租户行。未改指标派生，未改 ADR-0161 至 0166 的决定正文。
- **结论：可重放**。

**完成记录（通道 2 · 据完工报转录，第 6 项）**

分支 `mcp2-psb12-06`（基 `39998c14`），一笔未改写：`6b8f00e1`。命令 `charge-attribution`。

**进 main 记录（2026-10-01 00:4x，通道 1 推送，第 6 项）**

重放到 `0ceacfee` 之上，票面与开发主线补记按两边都留解开：`6b8f00e1→1e3b9f9d`。清点在代码链尖重生成为 `eaf61c5c`（`settlementaccounting` 生产 140→145、测试 99→102；`settlement_accounting` 迁移份数 29→30，文件号 `0031`，`0030` 未用）。
推送方验证：钉 `eaf61c5c`，build 退 0，架构门禁与点名用例 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `93daf4d3`（通道 3 分支 `mcp3-psb12-09`，第 9 项，ADR-0169，基 `baad0fbf`）· 2026-10-01 00:5x**

- **阻断**：无。架构门禁带 DSN 为 PASS。`TestTheSellGateIgnoresABuyEvaluation`、`TestSellChargeUsesTheEvaluationAmount`、`TestAMissingSellTriggerDoesNotInitiate`、`TestSellEvaluationTriggersRefuseToRunOutsideATransaction` PASS。
- **非阻断**（随票记）：形成编排不进 `cmd/parcel-dispatch`。ADR-0169 决定四写明如此，留给第 10 项以外的后继装配。
- **核过无发现**：消费门只认 SELL·客户费用，BUY 答不处理。金额整组出自评价，命令不带金额。触发命令 `sell-evaluation-trigger`，没登记答 `SELL_EVALUATION_TRIGGER_UNCONFIGURED`，不调用 BUY 请求。迁移 `0032`。未登租户行。未改 `assemble.go`，未改 ADR-0161 至 0168 的决定正文。
- **结论：可重放**。

**完成记录（通道 3 · 据完工报转录，第 9 项）**

分支 `mcp3-psb12-09`（基 `baad0fbf`），一笔未改写：`93daf4d3`。

**进 main 记录（2026-10-01 00:5x，通道 1 推送，第 9 项）**

重放到 `baad0fbf` 之上，零冲突：`93daf4d3→bfd09015`。清点在代码链尖重生成为 `31168ea2`（`settlementaccounting` 生产 145→153、测试 102→106；`settlementaccounting`→`parcelpricing` 消费缝 2→4；`settlement_accounting` 迁移份数 30→31，文件号 `0032`）。
推送方验证：钉 `31168ea2`，build 退 0，架构门禁与点名用例 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。

**评审 ← 通道 1 · 钉 `5174732f`（通道 2 分支 `mcp2-psb12-10`，第 10 项，ADR-0170，基 `af0245bb`）· 2026-10-01 01:0x**

- **阻断**：无。架构门禁带 DSN 为 PASS。`TestSettlementOrchestrationsStayUnconfiguredWhenTheMomentIsNotRegistered`、`TestSettlementMomentsRefuseToRunOutsideATransaction` PASS。仓内没有 `NewAuditSupplierBillHandler`，审核是 `ReceiveSupplierBillHandler.Audit`。
- **非阻断**（随票记）：`adjust`、`allocate`、`supplier`、`claims` 只在装配里被构造并断言非 nil，调用方法只有 `Confirm` 与 `CutOff`。ADR-0170 决定三写明其余入口不另设触发册。
- **核过无发现**：六个构造在 `buildSettlementOrchestrations`。确认与截单没登记答 `SETTLEMENT_MOMENT_UNCONFIGURED`。没有把 SELL 形成接进派发。迁移 `0033`。未登租户行。未改 ADR-0161 至 0169 的决定正文。
- **结论：可重放**。票 12 十项到此收口。

**完成记录（通道 2 · 据完工报转录，第 10 项）**

分支 `mcp2-psb12-10`（基 `af0245bb`），一笔未改写：`5174732f`。命令 `settlement-moment`。

**进 main 记录（2026-10-01 01:0x，通道 1 推送，第 10 项）**

重放到 `af0245bb` 之上，零冲突：`5174732f→f9c5192e`。清点在代码链尖重生成为 `b91ceb3f`（`cmd` 生产 73→74、测试 108→109；`settlement_accounting` 迁移份数 31→32，文件号 `0033`）。
推送方验证：钉 `b91ceb3f`，build 退 0，架构门禁与点名用例 PASS，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。本记录一笔只多本票面 `.md`。分支作封存出处。票 12 十项到此收口。

**收口核查（通道 5，钉 `a8cf12ff`）· 2026-10-10 · 通道 1 派单 `task-050c4117`，非作者独立取证**

**结论：不收口，Status 不动。** 十项的执行器与测试都在 main 上；缺的是参数登记册五行没有同步收短，交通道 1。取证实测于 `a8cf12ff`（该提交 CI run
`38050118343` success；第 10 项 `f9c5192e` 是它的祖先），以 `git grep` 在该提交上逐项定位。

- ✅ **每项有执行器（带测试），或记下了既有执行器的证据**：

| 项 | 执行器 / 证据 | 测试 | 登记命令 · 迁移 |
|---|---|---|---|
| 1（ADR-0161） | `domain.AmountGrammar.Compose` | `TestClaimAmountsUseTheRegisteredGrammarInsteadOfTheAssertedAmount`；真库 `TestAmountGrammarsRefuseToRunOutsideATransaction`、`TestEmptyAmountGrammarsStayUnconfigured`、`TestAnAmountGrammarRegistersReplaysAndConflicts` | `amount-grammar` · `0024_amount_grammar_parameter.sql` |
| 2（ADR-0162） | `Apportion`（domain 与 application 的 `allocation_form.go`） | `TestAllocationFormsRefuseToRunOutsideATransaction` | `allocation-form` · `0025_allocation_form_choice.sql` |
| 3（ADR-0165） | `domain.PeriodicFeeForm`（`domain/periodic_fee.go`） | `TestMinimumSpendShortfallIsTheGapBelowTheFloor`、`TestVolumeFloorChargesTheMissingQuantityAtTheRegisteredRate`、`TestTieredRebateAppliesEachBandOnlyToItsSlice`；真库 `TestEmptyPeriodicFeesStayUnconfigured`、`TestPeriodicFeesRefuseToRunOutsideATransaction` | `periodic-fee` · `0028_periodic_fee_form.sql` |
| 4（既有执行器） | `domain.ComponentRole.admittedBy`、`domain.DeriveOperatingResult`、`domain.OperatingResult.Rederive`、`application.AllocateCostsHandler.Derive` 与 `.Rederive` | `TestOperatingResultIsDerivedNotEdited`、`TestOperatingResultsDeriveAndRederive` | —（不新造执行器、不立 ADR） |
| 5（ADR-0163） | `domain.AuditEscalationCeiling`（`domain/audit_escalation.go`） | `TestAnAuditedAmountAboveTheCeilingMustEscalate`、`TestAnAmountWithinTheCeilingStaysInsideAuthority`、`TestARegisteredZeroCeilingIsNotUnconfigured` | `audit-escalation-ceiling` · `0026_audit_escalation_ceiling.sql` |
| 6（ADR-0168） | `domain.ChargeAttributionForm`（`domain/charge_attribution.go`） | `TestAttributionDateRollsForwardAtCutoff`、`TestJudgeChargeAttributionStaysUnconfiguredWhenTheFeeItemIsNotRegistered`、`TestChargeAttributionsRefuseToRunOutsideATransaction` | `charge-attribution` · `0031_charge_attribution.sql` |
| 7（ADR-0166） | `domain.AccountingConnectorForm`（`domain/accounting_connector.go`） | `TestAnUnregisteredAccountingConnectorStaysUnconfigured`、`TestAccountingConnectorsRefuseToRunOutsideATransaction`、`TestAccountingConnectorOnlyAcceptsCanonicalExchange` | `accounting-connector` · `0029_accounting_connector.sql` |
| 8（ADR-0164） | `evaluationRequestOrchestration.Trigger`（`cmd/parcel-api/assemble_evaluation_request.go`） | `TestTheProductionTriggerDoesNotRequestWhenTheReasonIsNotRegistered` | `buy-evaluation-trigger` · `0027_buy_evaluation_trigger.sql` |
| 9（ADR-0169） | `application.FormSellCustomerChargeHandler` | `TestTheSellGateIgnoresABuyEvaluation`、`TestSellChargeUsesTheEvaluationAmount`、`TestAMissingSellTriggerDoesNotInitiate`、`TestSellEvaluationTriggersRefuseToRunOutsideATransaction` | `sell-evaluation-trigger` · `0032_sell_evaluation_trigger.sql` |
| 10（ADR-0170） | `buildSettlementOrchestrations`（`cmd/parcel-api/assemble_settlement_orchestrations.go`） | `TestSettlementOrchestrationsStayUnconfiguredWhenTheMomentIsNotRegistered`、`TestSettlementMomentsRefuseToRunOutsideATransaction` | `settlement-moment` · `0033_settlement_moment.sql` |

  登记命令都在 `cmd/parcel-settlement-register`，迁移都在 `migrations/settlement_accounting/`。「顺带」那句 `PricingInputResolver` 旧注释（「三只读口今天都不存在」）
  在该提交上已无命中。
- ❌ **登记册对应行同步收短**。随各项代码笔收短了的是 `PAR-SET-06`（第 2 项 `83489825`）、`PAR-INT-04` 与 `PAR-INT-05`（第 7 项 `cab6613c`）、`PAR-SET-09`
  （第 6 项 `1e3b9f9d`），写法都是「已有执行器（ADR-…）……没登记答未配置」。第 1、3、4、5 项的提交（`161bb646`、`6b3d7d80`、`f8ead9ad`、`9a3b9e9a`）都没碰
  [参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md)，下面五行还是拆分当时的指向：
  - `PAR-COM-07`：「限额、比例、免赔怎样组成一个赔付/退款金额是计算方法，归产品策略 → 票 12」——第 1 项已落（ADR-0161）。
  - `PAR-SET-05`：「『越权升级规则』的判断结构 → 票 12」——第 5 项已落（ADR-0163）。同一格里「审核角色」「申请/决定分权」指向 [psb/07](./07-pc-authorization-coordinates-and-role-models.md)，不归本票。
  - `PAR-SET-07`：「最低消费、保底量、返利的计算形态归产品策略 → 票 12（待核）」——第 3 项已落（ADR-0165）。
  - `PAR-SET-08`：「『金额分配、比例/限额/免赔』怎样组成一个回收金额是计算方法，划为产品策略 → 票 12」——比例、限额、免赔由第 1 项落（ADR-0161）；金额分配按本票
    第 1 项的写法属第 2 项（ADR-0162）。
  - `PAR-SET-10`：「各阶段的『采用规则』与……如何形成新指标版本是指标方法……→ 票 12（待核）」——第 4 项已核毕：既有执行器按约束栏实现，不立新 ADR。

**缺什么、归哪张**：只缺登记册这五行的文字同步，代码不缺。按本票完成判据它归本票自己，照已收短的四行写法改这五行即可；本核查按派单不改登记册，交通道 1 定由谁补。
补完这五行，本票判据即齐。

**未改**：Status；参数登记册。
