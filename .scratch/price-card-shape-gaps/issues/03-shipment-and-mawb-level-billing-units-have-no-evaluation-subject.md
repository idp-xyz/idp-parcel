# 清关报价按票、按 MAWB 计费的项目没有评价主体：计价只逐包裹

Category: enhancement
Status: draft——需 owner 裁：票级/主单级计费单位落 parcel-pricing 的聚合方式，还是落 settlement-accounting 的费用发生项 + 分摊（MCP-1 2026-09-04 立票，只写票面未动代码）
Blocked by: 无

## 为什么立

[E1 形状核对](../report.md) 第 17 项。清关报价表（T01 / T11 两种模式）的计费单位有四种：按 MAWB、按票、按 KG、按件。后两种今天装得下——按 KG 走 `RateTableFamilyUnitPrice`，按件走 `FixedChargeRule`（聚合方式 `AggregationPerPackage`）。**前两种装不下**（核于 `a17bfac`）：

- `AggregationMode` 唯一取值 `AggregationPerPackage`，`PricingPlanVersion` 构造时硬写；CONTEXT 与 [`pp-pricing-rule-model-final-design.md`](../../../docs/design/pp-pricing-rule-model-final-design.md)「概念集合」节都写「聚合方式仍只有逐包裹」。
- `EvaluationSubjectKind` 只有已受理包裹与试算，没有委托（票）或主单（MAWB）这一级的评价对象；`PricingInputSnapshot` 一份对应一个包裹的重量与尺寸。

一笔「每 MAWB 若干美元」的清关费如果按包裹评价，要么每个包裹各收一次（多收），要么由调用方平摊后写成定额（摊法没人声明过）。

## 要裁的一件：落在哪一侧

1. **parcel-pricing 加聚合方式与评价主体**：`AggregationMode` 加 `PER_SHIPMENT` / `PER_MAWB`，`EvaluationSubjectKind` 加对应主体，评价输入快照带成员清单（件数、总重）；费用行上标聚合单位。代价：改规范化形状（`canonicalization` 换号）、CONTEXT「定价方案」与「计价评价」词条改、评价快照与读面全跟；设计文档「本设计不包含…周期级费用范围或独立计价服务形态」那句要看是否被触及（票级不是周期级，但要写清）。优点：价卡语言在一处，SA 继续「不拥有可执行价卡或第二套算价规则」（SA CONTEXT 首段硬句）。
2. **settlement-accounting 以费用发生项 + 版本化分摊规则承接**：清关行的按 MAWB/按票费用作为供应商费用发生项进 SA，按 SA CONTEXT 已有的「成本分摊结果 / 未分摊余额」机制归因到包裹；对客侧同法。代价：**费率本身还是得有地方算**——SA 不拥有算价规则，「每 MAWB 若干美元」这个数从哪张卡查出来仍然没有答案；等于把问题挪到 SA 而不是解决。
3. **两侧各半**：PP 只评价单价与规则（票级评价主体），SA 负责把票级金额分摊到包裹作经营归因。这实际是 1 + SA 既有分摊，不是新选项。

倾向（不是裁决）：1（配合 SA 既有分摊即 3）。判据是 SA CONTEXT 那句硬句——算价规则只能在 PP。

## 红线

- 不写任何真实清关费率进仓；SYN 夹具只记 `S`。
- 裁定前 E2 转换工具**不把按 MAWB / 按票项平摊成按件定额**，如实列「未转换：等本票」。
- 不给 SA 加第二套算价规则。

## 验证

裁决记进 CONTEXT，必要时 ADR 编号落进上面某一条，再按所选拆实施票；本票 `resolved` 的判据是那条引用在。

## Comments

- 2026-09-04 · MCP-1：立票。起因是 E1 核对第 17 项。**只写票面，未动代码。**
