# 12 settlement-accounting：金额文法、分摊与周期费用形态、经营指标方法与账务连接器

Category: enhancement
Status: in-progress——2026-09-30 第 1 项进 main（`161bb646`，ADR-0161），第 2 项进 main（`83489825`，ADR-0162），第 5 项进 main（`9a3b9e9a`，ADR-0163），第 8 项进 main（`7c084917`，ADR-0164），第 7 项进 main（`cab6613c`，ADR-0166）；第 3 项在本重放（ADR-0165）；第 4、6、9、10 项未做
Blocked by: 无（第 1、5 项的规则登记册与读口是重定级表 PN-07 行第一项的机制缺口，归[票 16](./16-mechanism-gaps-without-a-ticket.md)；缺它们时本票只能先定文法）
地盘：settlement-accounting 领域与应用层（金额、分摊、周期费用、指标），账单接入与财务交换的连接器适配器；规则正文若由 party-commercial 声明，PC 侧另开票。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-07`、`PAR-SET-05`、`PAR-SET-06`、`PAR-SET-07`、`PAR-SET-08`、`PAR-SET-09`、`PAR-SET-10`、`PAR-INT-04`、`PAR-INT-05` 行内「〔ADR-0146 拆分〕」点名的部分。[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-07 行第三项当时记「未核」，本票即其补核。

## 做什么

1. **赔付 / 退款与代垫回收金额的计算文法**（`PAR-COM-07`「赔付/退款责任、限额、比例和免赔依据」、`PAR-SET-08`「金额分配、比例/限额/免赔」）。`SettleClaimAmounts` 只核金额规则版本在不在，金额本身由命令带入（`AmountMinor`）；限额、比例、免赔怎样组成一个金额是方法，各数值是租户取值。规则版本的登记册与读口（`ClaimAmountRuleView`）是重定级表 PN-07 行第一项的机制缺口。落地：ADR-0161，先免赔、再按万分比向下取整、再以限额封顶；三项数值命令 `amount-grammar`，空册不形成金额。金额分配仍属第 2 项。
2. **成本分摊的内置形态**（`PAR-SET-06`「分摊规则」）。`AllocateCosts` 只核分摊规则版本在不在，各份额由命令带入（`Portions`）；按重、按件、按收入等分法归产品，是否适用与选哪种归租户。落地：ADR-0162，三套分法共用最大余数，余数相同按目标标识升序；选用命令 `allocation-form`，空册答未配置，不均摊。各对象权重随分摊交入。
3. **待核：周期费用的计算形态**（`PAR-SET-07`「最低消费、保底量、返利」）。落地：没有既有执行器。价卡最低重量与评价里的最低收费不是这一格。ADR-0165，三套形态加本期不适用；命令 `periodic-fee`；空册不形成周期费用。
4. **待核：经营指标各阶段口径与新指标版本的形成方法**（`PAR-SET-10` 已确认约束栏写的就是这套方法）。核现有指标派生是否按它实现。
5. **供应商账单审核的越权升级判断结构**（`PAR-SET-05`「越权升级规则」；分权的角色模型归票 07）。`SupplierAuditAuthorityView` 的登记册与读口是重定级表 PN-07 行第一项的机制缺口。落地：ADR-0163，已匹配金额小于或等于上限在权限内，大于上限必须升级且不形成应付；上限命令 `audit-escalation-ceiling`，空册不默认放行。角色模型仍归票 07。
6. **待核：费用归属日的判定形态**（`PAR-SET-09`）。各金额唯一创建用例与既有借贷项纳入后续账期已由 SA 定（机制），不再列为租户证据。
7. **供应商账单接入与财务系统交换的连接器形态**（`PAR-INT-04`、`PAR-INT-05`）。账单接收编排已有；核通用导入 / 导出形态有无，某供应商与某财务系统的格式映射留租户。2026-09-30 通道 2 按 [ADR-0166](../../../docs/adr/0166-accounting-exchange-uses-a-canonical-document.md) 落地这一项：没有通用文件导入或报文导出。内置形态是规范文书，命令 `accounting-connector`。没登记答未配置。本票第 3、4、6、9、10 项未动。
8. **BUY 评价请求的触发面**（[票 05](./05-demo-journey-criterion-evidence.md) 格 17；以下三项 2026-09-24 经用户授权自决补入）。`cmd/parcel-api/assemble_evaluation_request.go` 的 `buildEvaluationRequestOrchestration` 头注写「今天没有运营端点、也没有进程内触发面调它」「谁在什么业务时点为哪些发生项发起请求是产品题，触发面另票」——本项即那张票。落地：ADR-0164，触发面是 `evaluationRequestOrchestration.Trigger`，时点只有发生项形成；发生项原因命令 `buy-evaluation-trigger`，空册答未配置，不发起请求。
9. **SELL 评价到客户费用**（票 05 格 19）。SELL 评价没有请求面，评价已记录信封的消费门只收 BUY·供应商成本，SA 应用层没有由评价形成客户费用的编排。消费门与形成编排是机制；SELL 评价何时发起是产品策略。
10. **结算编排的生产入口与触发面**（票 05 格 20）。据票 05 取证，`NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewAuditSupplierBillHandler`、`NewSettleClaimAmountsHandler` 在 `cmd/` 零引用。装配与入口是机制；何时确认、何时截单是产品策略；账期是租户取值，演示租户经参考配置采用。

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
