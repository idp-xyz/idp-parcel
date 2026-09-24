# 12 settlement-accounting：金额文法、分摊与周期费用形态、经营指标方法与账务连接器

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，SA 一张）；逐项先核执行器有无
Blocked by: 无（第 1、5 项的规则登记册与读口是重定级表 PN-07 行第一项的机制缺口，缺它们时本票只能先定文法）
地盘：settlement-accounting 领域与应用层（金额、分摊、周期费用、指标），账单接入与财务交换的连接器适配器；规则正文若由 party-commercial 声明，PC 侧另开票。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-07`、`PAR-SET-05`、`PAR-SET-06`、`PAR-SET-07`、`PAR-SET-08`、`PAR-SET-09`、`PAR-SET-10`、`PAR-INT-04`、`PAR-INT-05` 行内「〔ADR-0146 拆分〕」点名的部分。[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-07 行第三项当时记「未核」，本票即其补核。

## 做什么

1. **赔付 / 退款与代垫回收金额的计算文法**（`PAR-COM-07`「赔付/退款责任、限额、比例和免赔依据」、`PAR-SET-08`「金额分配、比例/限额/免赔」）。`SettleClaimAmounts` 只核金额规则版本在不在，金额本身由命令带入（`AmountMinor`）；限额、比例、免赔怎样组成一个金额是方法，各数值是租户取值。规则版本的登记册与读口（`ClaimAmountRuleView`）是重定级表 PN-07 行第一项的机制缺口。
2. **成本分摊的内置形态**（`PAR-SET-06`「分摊规则」）。`AllocateCosts` 只核分摊规则版本在不在，各份额由命令带入（`Portions`）；按重、按件、按收入等分法归产品，是否适用与选哪种归租户。
3. **待核：周期费用的计算形态**（`PAR-SET-07`「最低消费、保底量、返利」）。
4. **待核：经营指标各阶段口径与新指标版本的形成方法**（`PAR-SET-10` 已确认约束栏写的就是这套方法）。核现有指标派生是否按它实现。
5. **供应商账单审核的越权升级判断结构**（`PAR-SET-05`「越权升级规则」；分权的角色模型归票 07）。`SupplierAuditAuthorityView` 的登记册与读口是重定级表 PN-07 行第一项的机制缺口。
6. **待核：费用归属日的判定形态**（`PAR-SET-09`）。各金额唯一创建用例与既有借贷项纳入后续账期已由 SA 定（机制），不再列为租户证据。
7. **待核：供应商账单接入与财务系统交换的连接器形态**（`PAR-INT-04`、`PAR-INT-05`）。账单接收编排已有；核通用导入 / 导出形态有无，某供应商与某财务系统的格式映射留租户。

顺带（[票 01](./01-regrade-slices-under-four-criteria.md)「严格复核记录」交来，只改注释）：`PricingInputResolver` 的注释「三只读口今天都不存在」已被 `pp-pricing-input-seams` 01–03、05 推翻。

## 不做

- 不替租户定任何限额、比例、免赔、分摊依据或周期费用数值；不接真实财务系统。
- 公开计价参考序列的来源连接器（`PAR-SET-11`）已有票 `pricing-reference-series-operations/06`，不在本票。

## 完成判据

- 每项要么有执行器（带测试），要么记下已有执行器的证据；登记册对应行同步收短。
