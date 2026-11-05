# ADR-0365：带始发维度的分区与偏远档位目录，始发按价卡声明的来源取——寄件邮编，或这一段起始节点在卡上登记的计费起点；物理节点地址不当起点，评价带不出起始节点或卡上没登即待判断

Status: Accepted（2026-11-05，Cursor 会话代产品 owner、PP、NR 与 TF owner 决（代决）：用户原话与授权同 [ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)；起因是 `docs/wms/` 原型手册附录 D「同址多注入」。部分停用 [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定三一句与补记「邮编路线从哪来」一条的一句，并回答其越权风险点 7。越权风险点见文末，待 PP、NR 与 TF owner 复核。）<!-- 索引注记 -->｜**判据**：价卡绑了带始发维度的目录时声明始发来源，二选一——寄件邮编（今天的读法），或按这一段的起始节点取卡上「节点 → 计费起点邮编」映射；后一种只在评价带得出起始节点时可用（路由比价按段、正式 BUY 评价按发生项所在实际履约段），带不出或卡上没登该节点即待判断，不退回寄件邮编；物理节点地址不当计费起点，节点仍不加邮编；目的照旧取收件邮编

Date: 2026-11-05

## Context

取证于 main `dde9b0c65`，只记 `S`。

- 原型手册附录 D：同一目的地 10001，从 LAX 注入落 Zone 8，从 EWR 注入落 Zone 2；完整成本再加前程与注入费，按最低完整成本选 LAX。美国尾程把包裹注入末端承运商时按入口邮编分区，是「小包专线与货代」这一层客户的日常形态（[开发主线「三层与各自的常规业务形态」](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#三层与各自的常规业务形态)）。手册还把「合同计费起点」与「物理节点」分开：起点是合同约定用来查区的邮编，可以不是设施所在地。
- [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一与补记：绑了目录的卡用包裹邮编路线经目录读出区域，路线是整件包裹的、不随段变，寄件邮编作始发、收件邮编作目的。决定三不采「节点加邮编」，把「按节点地址分区」留到首个真实租户的分区表需要时再定；越权风险点 7 记下了代价：中间段并不从寄件人出发。对按入口分区的尾程卡，拿寄件邮编（在中国）当始发，查出的区是错的。
- [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定二：一版目录声明始发维度（始发邮编前缀集或始发分区），形状已在，缺的是始发输入从哪来。
- [ADR-0260](./0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)：评价按范围选卡，计价条款只从价卡读。计费起点是采购条款，该登在卡上，不登在参与方与商业的协议上。
- 正式 BUY 评价按运输收费发生项计价（[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)），发生项读口今天交成员载运对象与业务时点，不交段的起讫（CONTEXT-MAP「`transport-fulfillment → parcel-pricing`（发生项成员读口）」）。
- 能力范围判据：按入口分区指得出常规模式，形态现在做；起点取值等真实价卡。

## Decision

**一、价卡声明始发来源。**（产品策略）

- 价卡绑了带始发维度的分区或偏远档位目录时，声明始发邮编从哪里取，二选一：寄件邮编（今天的读法，ADR-0189 补记）；按这一段的起始节点取卡上登记的计费起点。
- 按起始节点取时，卡上登「节点 → 计费起点邮编」映射。节点用 `network-routing` 的节点身份，卡只引用、不拥有。一张卡可以登多个节点，对应一份采购合同覆盖几个注入点的情形。
- 新规范化版本里，绑了带始发维度目录的卡必须声明始发来源；旧卡按寄件邮编读。目的照旧取收件邮编。
- 换号与 [ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md) 决定五同一批、同一版。

**二、物理节点地址不当计费起点。**（产品策略）

- 起点是卡上登的值，与节点的设施地址无关；节点照旧不加邮编，ADR-0189 决定三「不采形态二：节点不加邮编。」不变。改节点不改任何卡的起点。

**三、评价从两路带出起始节点。**（机制）

- 路由比价：`network-routing` 按段交出这一段的起始节点，与段计价区域、邮编路线一并交给计价（ADR-0189 补记「走哪一口」那一口）。
- 正式 BUY 评价：计价经发生项读口读出发生项所在实际履约段的起始节点，这一格归 `transport-fulfillment` 答；计价不从包裹或路由计划推。
- 其余评价对象（已受理包裹的 SELL 评价、受理前估价、运营试算）没有段，带不出起始节点；声明了按起始节点取的卡在这些评价上待判断。

**四、缺与坏照旧分开。**（机制）

- 卡要起始节点而评价带不出，或卡上没登这个节点：这一段或这次评价待判断，候选缺成本依据；不退回寄件邮编，不借相邻节点的起点。
- 读口读不回是依赖故障，照 ADR-0189 补记「缺与坏分开」。

三类：

- **机制**：卡上新键与换号、NR 按段交起始节点、发生项读口补一格、评价分支。
- **产品策略**：两种始发来源、物理地址不当起点、带不出即待判断。
- **租户取值**：哪张卡按起始节点取、各节点登哪个起点邮编。演示租户不因本记录改种子。

## 对已接受记录的作用

- 部分停用 ADR-0189 决定三「按节点地址分区是另一种目录形态，等首个真实租户的分区表需要时再定（ADR-0109 决定二「粒度随首份真实分区表定」）。」一句：始发不按节点地址取，按卡上声明的来源取，形态现在定。适用场景：全部绑了带始发维度目录的卡；决定三前半「不采形态二：节点不加邮编。」与其余各条不变。
- 部分停用 ADR-0189 补记「邮编路线从哪来」一条里「路线是整件包裹的、不随段变，寄件邮编作始发、收件邮编作目的。」一句：始发随段取。适用场景：只限声明了按起始节点取始发的卡；其余卡照原句。
- 回答 ADR-0189 越权风险点 7「路线按整件包裹，不按段」：声明了按起始节点取始发的卡，始发随段取，代价由此解除；其余卡照旧承担它。

## Consequences

- 同址多注入的比价有了正确的区：LAX、EWR 两条候选各按自己的起点查区，完整成本照 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四逐段合成。
- 正式 BUY 评价与路由比价读同一张卡、同一份起点映射，两路对得上。
- 原型手册的「注入方案」不作为一个对象引入：渠道与合同归参与方与商业，节点、截单与尾程注入段归网络与路由，容量归运输履约，价卡、分区与计费起点归计价。
- 发生项读口还答不出实际履约段的起始节点；那一格补上之前，正式 BUY 评价对这类卡待判断，择优用的成本与事后的供应商预期成本一时对不齐，与 ADR-0189 Consequences 里「择优用的成本与事后记的供应商预期成本，在这类卡上会一时对不齐」同形。

## Alternatives considered

- **节点加邮编，按节点地址查区（ADR-0189 的形态二）。** 否决：合同计费起点可以不是设施所在地；节点地址一改，所有卡的区跟着变。
- **起点登在参与方与商业的供应商协议上。** 否决：ADR-0260 规定计价条款只从价卡读；路由比价与正式 BUY 评价都已经读卡，不必再跨上下文读协议。
- **起点登在网络与路由的段上，与段计价区域同一本册。** 否决：正式 BUY 评价按发生项走，读不到段上的登记，两路会对不齐。
- **照旧用寄件邮编。** 否决：按入口分区是第二层客户的日常形态，算错的是 BUY 成本与择优。

## 越权风险点

1. **卡上引用网络与路由的节点身份。**（决定一）卡只引用、不拥有；节点停用或改名不动卡。归 PP、NR owner。
2. **发生项读口补「实际履约段的起始节点」。**（决定三）取进入这一段那次`已交接`声明的交出节点，还是段自己记的起点，由 TF owner 定。归 TF、PP owner。
3. **SELL 卡不开按起始节点取。**（决定三）客户价若要按入口分区，另立。归 PP owner。
4. **一个节点在一张卡上只登一个起点。**（决定一）同一节点按服务分起点的卡另立。归 PP owner。

## Links

- [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)：逐段计价区域按卡给；本记录部分停用其决定三一句与补记一句，回答越权风险点 7
- [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：目录的始发维度
- [ADR-0260](./0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)：计价条款只从价卡读，所以起点登在卡上
- [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：正式 BUY 评价按发生项发起
- [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：决定四，候选成本逐段合成
- [ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)：同批代决，同一版规范化换号
- [冻结清单「记录」](../../.scratch/launch-line-freeze-2026-10-27.md)：实现项与建议分级
