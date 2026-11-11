# 06 票级计价重量的聚合方式已有决定、代码未到——票级卡今天只会合计后派生，按件算计费重再相加的卡算不对

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 无。[ADR-0364](../../../docs/adr/0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md) 已接受（会话代决，越权风险点待计价 owner 复核）；激活本票即承接冻结清单「记录」里出处写它决定三的那一行
地盘：`internal/parcelpricing/domain`、`internal/parcelpricing/adapters/pricecardtemplate`、`apps/admin-web/src/pages/pricing`

对应要求：Rating「分票计费」的领域一半。生产路径在票 [07](./07-shipment-level-sell-charging-enters-production.md)。

## 现象

取证钉 `11bfa1ffb`。

- ADR-0364 决定三定了票级主体的计价重量由价卡声明聚合方式：合计实重与合计体积重之后按重量策略派生，或逐件按重量策略派生之后求和；后者另声明进位点。计价 `CONTEXT.md`「计价重量」词条已经照此写成现行。
- `UC-PP-001`「给开发的交接」写着这一组（档位边界、总额上下限的基数、票级聚合方式、「其余邮编」行）「代码尚未做到」。
- 领域今天只有合计后派生一种读法（`AggregationPerShipment`，[ADR-0111](../../../docs/adr/0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md) 决定二）。
- 冻结清单「记录」里有它的一行，分级 R3。

## 要做的

照 ADR-0364 决定三与决定五实现，不另作设计：

1. 票级卡声明聚合方式；逐件求和时再声明进位点（逐件进位后求和，或求和之后进位一次）。
2. 逐件求和的输入快照另带逐件实重与尺寸；卡要体积重而缺任一件的尺寸时评价待判断，不退回合计。
3. 条件最低计价重量照旧作用在方案级计价重量上；判定条件读到的自身计价重量照决定三的定义。
4. 新键写下一版规范化版本，旧卡字节不动，按合计后派生读；评价解释如实记用的是哪一种。
5. 导入模板、模板规格口与管理台编辑器能表达这两格。
6. 主单级主体不开这一选项。

## 范围说明

ADR-0364 同一篇里还有档位边界、总额上下限的基数、「其余邮编」行三项，冻结清单各有一行。它们不在客户要求清单里，本票不承接；但它们与本票共用同一次规范化版本换号（ADR-0364 决定五），激活时要一起排，不要各换一次号。

## 三类

聚合方式与进位点这两格的存在是产品策略，读法是机制；每张卡选哪一种是租户取值。

## 完成判据

- 同一组多件输入，两种聚合方式各得各的计价重量，进位点两种读法各有用例；与 ADR-0364 给的口径一致。
- 缺一件的尺寸而卡要体积重时答待判断。
- 旧票级卡的内容摘要与评价结果一字不变。
- `UC-PP-001`「给开发的交接」里那一条随之更新；冻结清单那一行标已处理。
