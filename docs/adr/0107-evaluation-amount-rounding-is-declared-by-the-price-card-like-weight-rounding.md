# ADR-0107: 评价金额的精度与取整由价卡内容声明，与重量取整同形，进内容摘要；未声明即不取整并如实记问题项；不编币种小数位表，消费方不得在评价外取整

Status: Accepted
Date: 2026-09-04

## Context

[parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md) 不变量原句：「价表区间、分区、重量策略、附加费、折扣、最低/最高收费、燃油、组合方式、**精度和取整顺序**必须可解释、可复算，并由版本化规则声明。页面显示精度不能改变评价结果。」`价格评价` 词条也写着它「包含……费用组成、**精度、取整**和解释」。这两句是 [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 那一代定下的口径。

代码只落了重量那一半。票 [pricing-amount-precision/01](../../.scratch/pricing-amount-precision/issues/01-evaluation-amount-scale-has-no-declared-source.md) 的取证（本记录复核于 `main = 512b419`，各条仍成立）：

- `Decimal` 是 coefficient 文本加 scale，上限常量的注释原句「这些上限是技术保护限制，不是币种的小数位数」；
- 费用合计是 `runningAmount.Add(...)` 裸累加，`Decimal.combine` 取两操作数 scale 的较大者——合计的 scale 等于价表里写得最细的那个金额，`12.5` 与 `12.50` 算出来的合计 scale 不同；
- 换算 `convertAmount` 与百分比 / 系数附加费 `effectiveRate` 都走 `Decimal.Mul`，scale 是两者之和——两位小数的 USD 乘四位小数的汇率得到六位小数的 CNY，不取整；
- `RoundToIncrement` / `DivRoundToIncrement` 只被重量取整策略与体积系数的商使用；`RoundingMode` 封闭集只有 `NONE` 与 `CEILING`。

**这是机制半边的缺口，不是实例半边等参数。** 后果是一个自称「可复算的最终价格」的评价可以带着六位小数的 CNY 出来；任何需要最小币单位的消费方（`settlement-accounting` 存 int64 分）只能在评价之外再取一次整——那个数评价没算过，争议时没人能复算它。票 [label-channel/13](../../.scratch/label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md) 2026-09-02 的裁决已经堵住了最省事的那条路：**不编币种小数位表**，没有任何桥能正确填它，编一张就是把未确认参数写死成生产默认。

于是要裁两件：金额取整的槽落在哪，以及要不要进 ADR。第二件先答：它改评价结果的形状（合计与换算后金额的 scale），`settlement-accounting`、报价面、任何读 `Total()` 的地方都受影响，且改规范化形状——按 [AGENTS](../../AGENTS.md) 的门槛属难逆转取舍，进 ADR。

## Decision

**一、金额取整策略是价卡内容，与重量取整同形，随卡版本化。** 采购方向的策略属价卡内容，销售方向的属商业价格政策——与体积系数的两侧归属逐字同构（CONTEXT「体积系数」词条）。本上下文不内置任何金额精度常量，也不按币种查任何表。

**二、一条金额取整策略声明三件：模式、进位单位、应用点。** 模式取 `RoundingMode` 封闭集，实施时按首份真实价卡需要扩集合（商业取整至少要 `HALF_UP`；集合怎么扩由实施票按 CONTEXT「特征」「判定条件」那种封闭集写法定）；进位单位以该卡币种的金额表示（`0.01`、`1` 之类），是卡上声明的实例值；应用点是一个封闭集合，**合计**必声明，**逐行**与**换算后**可选声明。「取整顺序」就是应用点在评价里的固定先后（逐行 → 换算后 → 合计），不是卡上再声明一次顺序——顺序属机制，点属实例。

**三、策略进内容摘要与规范化版本。** 它是价卡内容的一部分，进 `PricingPlanVersion` 的内容摘要；规范化文档因此换形状，`canonicalization` 按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 换号。既有评价不追溯改写，按原版本重放；旧规范化版本的卡按 `CANONICALIZATION_VERSION_UNSUPPORTED` 既有语义处置。

**四、未声明即不取整，且如实记问题项；不给默认。** 没有声明金额取整策略的卡，评价按今天的行为算（精确十进制，不取整），评价完成，同时带一条结构化问题项（形照 [ADR-0105](./0105-evaluation-issue-carries-a-structured-series-subject-and-lands-in-a-child-table.md)）「金额精度未声明」，让消费方看得见这个金额没有被任何规则取过整。首发不在发布门上强制声明——那会把今天所有 SYN 夹具卡一起判为不合法，而它们记的本来就是 `S`。

**五、消费方不得在评价之外取整。** `settlement-accounting` 与任何读 `Total()` 的一侧照 label-channel/13 的立场原样保全十进制（coefficient + scale，或 `Decimal.String()`）；本记录落地后，声明了策略的卡算出的合计 scale 就是进位单位的 scale，消费方直接采用；未声明的卡由问题项告知，消费方要么原样保全、要么把「精度未声明」如实交给自己的下游，不得自己补一次取整。

## Consequences

- **「可复算的最终价格」这句话第一次对金额成立**：合计怎么取的整、在哪一步取的，都在卡上与评价的解释项里，争议时按同一张卡复算得到同一个数。
- **SA 的 int64 分不再是一次没人算过的取整**：声明了策略的卡，`Total()` 已落在最小币单位上；SA 侧只做单位换写不做算术。
- **规范化版本换号一次**；旧评价与旧卡快照按原版本重放，`registered_price_card.go` 对 `canonicalizationVersion` 的守卫照旧拦不支持的版本。
- **报价面与择优的成本分值桥读到的金额 scale 变稳定**，label-channel/13 的成本分值桥不必再对 scale 作任何假设。
- 代价：价卡内容多一个槽，逐字段登记表单（ADR-0101 决定八）与 JSON 登记口都要加它；E2 转换工具把客户价卡的取整条款转进这个槽，转不出的如实列「未声明」。
- 代价：`RoundingMode` 扩集合是领域封闭集改动，打到重量取整的守卫与快照——实施走三步法或单独 worktree（[parallel-sessions](../agents/parallel-sessions.md) 的判据）。
- **本记录不填任何取值**：模式集合扩到哪几个、某张卡进位单位是多少，都随首份真实价卡来；SYN 夹具只记 `S`。

## Alternatives considered

- **槽落评价请求，由合同 / 结算侧随 `PricingInputSnapshot` 带进来**（票面选项 2）。否决：同一张卡对不同合同算出不同精度，卡的「可复算」少了一格；且取整是价卡条款的一部分（承运商价卡通常明写「按 0.01 进位」），把它挪到请求侧等于让调用方替卡声明内容。
- **不在 PP 取整，归 SA 的财务采用**（票面选项 3）。否决：与 CONTEXT「评价包含精度、取整」「可复算的最终价格」两句正面冲突，要改 ADR-0013 那一代定下的口径；而计价重量那条「财务采用归 SA」成立的理由是计费重量确实是结算侧按合同另行采用的量，金额合计不是——合计就是评价要交出去的那个数。
- **编一张币种小数位表当缺省。** 否决：label-channel/13 已裁，一字不改——没有任何桥能正确填它。
- **未声明即拒绝发布。** 否决（首发）：SYN 夹具全部要重登；且「必须声明」是可以后加的门，「有槽可声明」是现在就缺的机制。留给发布门随首份真实价卡再定。
- **只给合计一个点，不给逐行与换算后。** 否决：承运商卡有「每项附加费单独进位到分」的写法，也有「换算后进位」的写法；点集合封闭但不止一个，顺序由机制固定。

## Links

- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：不变量「精度和取整顺序……由版本化规则声明」、词条「计价重量」「体积系数」——本记录沿用其形状与两侧归属
- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：换算在评价内完成——换算后金额的 scale 因此是本记录要管的
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本换号的依据
- [ADR-0105](./0105-evaluation-issue-carries-a-structured-series-subject-and-lands-in-a-child-table.md)：结构化问题项的形状，「金额精度未声明」照它落
- [票 pricing-amount-precision/01](../../.scratch/pricing-amount-precision/issues/01-evaluation-amount-scale-has-no-declared-source.md)：取证与两问
- [票 label-channel/13](../../.scratch/label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)：「不编币种小数位表」的裁决出处
- [票 price-card-shape-gaps/report.md](../../.scratch/price-card-shape-gaps/report.md)：E1 核对第 19 项把金额精度指到本票
