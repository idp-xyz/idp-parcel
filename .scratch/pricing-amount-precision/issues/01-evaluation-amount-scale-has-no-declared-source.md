# 评价金额的精度没有任何声明来源：CONTEXT 要求由版本化规则声明，代码只落了重量那半

Category: enhancement
Status: draft——需 owner 裁两件：金额取整槽落在哪（价卡内容 / 评价请求 / 留给 SA 的财务采用），以及是否进 ADR（settlement-accounting 是消费方）；MCP-5 2026-09-04 立票，只写票面未动代码
Blocked by: 无

## 为什么立

MCP-3 做 [mechanism-executor-triage/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)
SA-c（BUY `PricingEvaluation` → SA 入向缝）时问：**评价已完成时 `Total().Amount()` 的 scale 是否已由价卡声明的
取整精度固定到币种最小单位？** 答案是否，而且不是「还没配」，是**领域里没有这个槽**。取证于 `mcp5-pr08 @ e6e56eb`
（与 main 在这几处无差）：

| 事实 | 出处 |
|---|---|
| `Decimal` = coefficient 文本 + scale，上限常量的注释原句「这些上限是技术保护限制，不是币种的小数位数」 | `internal/parcelpricing/domain/decimal.go` |
| 费用合计是 `runningAmount.Add(rule.amount.amount)` 裸累加；`Decimal.combine` 取两操作数 scale 的较大者，所以合计 scale = 价表里写得最细的那个金额（`12.5` 与 `12.50` 结果 scale 不同） | `evaluation.go`、`decimal.go` |
| 换算 `convertAmount` 是 `original.amount.Mul(reading.value)`；`Decimal.Mul` 的 scale 是两者之和——USD 两位 × 汇率四位 = CNY 六位，不取整 | `reference_series.go` |
| 百分比 / 系数附加费 `effectiveRate` 同为 `Mul`，同样抬高 scale | `reference_series.go` |
| `RoundToIncrement` / `DivRoundToIncrement` 只用于**重量**（重量取整策略、体积系数的商）；`evaluation.go` 里 `rounded` 只指 `pricingWeight` | `weight_rounding.go`、`plan.go`、`evaluation.go` |

而 [parcel-pricing CONTEXT](../../../docs/domain/parcel-pricing/CONTEXT.md) 不变量原句：「价表区间、分区、重量策略、
附加费、折扣、最低/最高收费、燃油、组合方式、**精度和取整顺序**必须可解释、可复算，并由版本化规则声明。页面显示精度
不能改变评价结果。」`PricingEvaluation` 的定义也写着「包含……费用组成、**精度、取整**和解释」。文档要求金额精度由
版本化规则声明，代码只实现了重量那半——**这是机制半边的缺口，不是实例半边等参数。**

**后果**：一个自称「可复算的最终价格」的评价可以带着六位小数的 CNY 出来。任何需要最小币单位的消费方
（SA 领域存 int64）只能在评价之外再取一次整——那个数评价没算过，争议时没人能复算它。

## 要裁的两件

**一、金额取整槽落在哪。** 三条互斥：

1. **价卡内容**：与重量取整同形（模式 + 进位单位，如 `HALF_UP` + `0.01`），随卡版本化、进内容摘要；评价在声明的
   点上取整（合计？逐行？换算后？——「取整顺序」本身就是 CONTEXT 要求声明的东西）。销售方向的对应物落在商业价格
   政策，与体积系数的两侧归属同构。改规范化版本（`canonicalization` 换号），旧评价不追溯改写。
2. **评价请求**：由合同/结算侧随 `PricingInputSnapshot` 带进来（像 `WithSettlementCurrency` 那样）。代价：同一张卡
   对不同合同算出不同精度，卡的「可复算」少了一格。
3. **不在 PP 取整，归 SA 的财务采用**：CONTEXT 对计价重量已经这么裁——「计价重量是评价内的中间结果，客户与供应商
   计费重量的财务采用由 settlement-accounting 分别形成」。金额若同理，PP 交精确十进制、SA 按合同条款采用。代价：
   与「评价包含精度、取整」「可复算的最终价格」两句正面冲突，要改 CONTEXT 措辞。

倾向（不是裁决）：1。理由是 CONTEXT 现有两句都指向评价自己带精度，且与重量取整、体积系数「随工件版本化、不内置
常量」的立场一致；3 是最省代码的，但要先改文档口径，而那两句是 ADR-0013 那一代就定下的。

**二、要不要进 ADR。** 它改评价结果的形状（合计与换算后金额的 scale），SA、报价面、任何读 `Total()` 的地方都受
影响；按 AGENTS「难逆转技术或产品取舍 → 新 ADR」的门槛，倾向要。

## 红线

- **不编币种小数位表。** [label-channel/13](../../label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)
  2026-09-02 那条裁决照旧成立：没有任何桥能正确填它，编一张就是把未确认参数写死成生产默认。取整精度是**每张卡声明
  的实例值**，机制只给槽。
- 不给未声明取整的卡一个默认精度；未声明就按今天的行为算（不取整），SYN 夹具只记 `S`。
- 既有评价不追溯改写；取整进内容摘要与规范化版本。
- 裁决落地前，任何消费方**不得在评价之外取整**——SA-c 适配器照 label-channel/13 的立场原样保全十进制
  （coefficient+scale 或 `Decimal.String()`），或留空并把这一问写进票面。

## 验证

裁决记进 CONTEXT（必要时 ADR 编号落进上面某一条），再按所选拆实施票；本票 `resolved` 的判据是那条引用在。

## Comments

- 2026-09-04 · MCP-5：立票。起因是 MCP-3 于通道广播问 SA-c 缝的 scale；答案与证据已在通道回过一遍，此处落为票面。
  **只写票面，未动领域代码。**
