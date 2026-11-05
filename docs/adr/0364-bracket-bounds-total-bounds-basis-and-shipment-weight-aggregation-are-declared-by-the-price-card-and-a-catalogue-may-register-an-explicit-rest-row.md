# ADR-0364：价表档位边界、总额上下限的基数与票级计价重量的聚合方式改由价卡声明；分区与偏远档位目录可登显式「其余邮编」行，不做未命中兜底

Status: Accepted（2026-11-05，Cursor 会话代产品 owner、PP owner 决（代决）：用户原话「你作为业务专家，你能帮我拍板吗」，随后在追问里选定把决定写成代决 ADR、实现项在冻结清单记录区各记一行；起因是对 `docs/wms/` 两份外部文档的评估，档位边界一项是评估中复核计价代码时发现的。部分停用 [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md) 决定二两句与决定六一项，并回答其越权风险点 0 与 2；部分停用 [ADR-0111](./0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md) 决定二一句；部分停用 [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定二一句。越权风险点见文末，待 PP owner 复核。）<!-- 索引注记 -->｜**判据**：判法由价卡或目录声明，产品给出全部形态、不给默认——重量段阶梯的档位边界取 `[下界, 上界)` 或 `(下界, 上界]`；总额上下限的基数取整票全部费用行或只对基础运费，后者在基础运费之后施加、补差进燃油一类百分比基数；票级计价重量取合计后派生或逐件派生后求和，后者另声明进位在逐件上还是在和上；目录每个始发维度至多一条「其余邮编」映射，前缀映射优先，没登照旧待判断，不做未命中兜底。新键写下一版规范化版本，旧卡与旧目录字节不动，登记照 ADR-0352 要部署开关

Date: 2026-11-05

## Context

取证于 main `dde9b0c65`，只记 `S`。

- **档位边界写死成左闭右开。** `internal/parcelpricing/domain/rate_table.go` 的 `RateEntry.contains` 判「下界 ≤ 重量 < 上界」，用例 `TestRateTableUsesLeftClosedRightOpenIntervals` 钉着它。本仓权威文档里没有一句定过这条；外部参考设计的 Rating Runtime 计算语义规范写的是「区间为左闭右开」。承运商价表多数写「不超过 X」，USPS 价目表的列头就是「Weight Not Over」；`docs/wms/` 里原型手册的阶梯也是「下限不含、上限包含」。向上进位后的计价重量常常正好落在档位上界（0.95 kg 按 0.1 kg 进位成 1.0 kg），边界怎么判直接决定收哪一档。今天转抄一张「不超过」的卡，只能把每档平移一个进位单位，转抄者要自造下一个可表示值，进位规则一改又会失效。判定条件已经为同一理由同时提供严格与含端两种比较（[PP CONTEXT「判定条件」](../domain/parcel-pricing/CONTEXT.md)）；同一份 CONTEXT 的「价表版本」词条也把边界语义列为价表版本要明确的内容。
- **最低收费只能对整票。** [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md) 决定二把总额上下限放在全部费用行之后，补差不进任何费用依赖的基数；决定六写明不对运费单独设基。它的越权风险点 0 与 2 点名了表达不了的两类卡：最低收费只对运费一项，以及最低运费计入燃油基数（「最低运费 × 燃油率」）。原型手册的计费顺序正是后者：基础运费不足最低收费先补差，燃油按补差后的运费计。
- **票级计价重量只有一种聚合。** [ADR-0111](./0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md) 决定二让票级与主单级快照带合计实重与合计体积重，按重量策略派生一个合计计价重量。原型手册的「合票」是另一种：逐件取实重与材积重的较大值，再求和进位。两种在市场上都是常规形态：多件快递按件算计费重再合计，空运整票按合计实重与合计体积重取大。逐件取大再求和永远不小于合计后取大。BUY 要和承运商账单对得上，SELL 要和客户合同一致，产品替卡挑哪一种都会在一侧算错。
- **「其余地区」没有写法，兜底被提议过。** [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定四定了绑了目录的卡查不到即待判断、不给默认。承运商分区表里常见「其余地区一律某区」一类条款；原型手册另有「缺价兜底 Zone」：卡上声明兜底区，查不到就落兜底。[ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一已为同一理由拒绝给按目的邮编分区的卡登固定区：那会静默算出一个错价参与择优，而不是待判断。
- **规范化。** 基础版是 `PPC-5`，`PPC-6` 由 ADR-0348 引入；规范化文档加键照 [ADR-0267](./0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md) 决定一换号、取能表达内容的最低一版；登记较新版本的价卡要部署开关（[ADR-0352](./0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)）。
- **能力范围判据**（[开发主线「能力范围判据」](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#能力范围判据)）：指得出目标客户群里的常规商业模式即可进，取值仍由价卡携带。下面每一项都指得出模式。

## Decision

**一、重量段阶梯的档位边界由价表声明。**（产品策略；机制见决定五）

- 两种读法：含下界、不含上界（`[下界, 上界)`，今天唯一的读法）；不含下界、含上界（`(下界, 上界]`，即「不超过上界」）。一张价表只用一种，不逐档混用。
- 新规范化版本里，重量段阶梯价表必须声明读法；不声明的旧版价表按 `[下界, 上界)` 读，字节、摘要与评价结果不动。
- 连续性与重叠照旧按声明的读法判：相邻两档的同一个界点只归一档，空档与重叠照旧拒，末档可以照旧开放上界。
- 首重加续重与计费重乘单价两族不受影响：前者的续重份数按步长向上取整，后者没有档位。

**二、总额上下限的基数由价卡声明。**（产品策略）

- 两种基数：整票全部费用行之和（今天的读法，ADR-0348）；只对基础运费。上下限两端共用一个基数。
- 基数为基础运费时，上下限在基础运费之后、固定规则之前施加，比的是基础运费那一行；补差行或冲减行紧跟基础运费，并计入把基础运费列入基数的费用依赖，燃油一类百分比费用随之按调整后的运费算。费用代码与方向、补差额而非整笔、不改写基础运费那一行、不触发也留痕，都照 ADR-0348。
- 新规范化版本里，声明了总额上下限的卡必须声明基数；旧卡按整票读。

**三、票级主体的计价重量由价卡声明聚合方式。**（产品策略）

- 两种聚合：合计实重与合计体积重之后按重量策略派生（今天的读法，ADR-0111）；逐件按重量策略派生之后求和。
- 逐件求和时，价卡另声明进位点，二选一：逐件进位后求和，或求和之后进位一次。不叠两次。
- 逐件求和的快照另带逐件实重与尺寸（成员清单已在快照里），体积重按卡的体积系数逐件算；卡要体积重而缺任一件的尺寸时，评价待判断，不退回合计。
- 主单级主体不开这一选项，照旧合计后派生，即空运按整票计费的口径。
- 新规范化版本里，票级卡必须声明聚合方式；旧卡按合计后派生读。

**四、分区与偏远档位目录可登显式「其余邮编」映射；不做未命中兜底。**（产品策略）

- 一版目录在每个始发维度上至多登一条「其余邮编」映射，接住本版里没被任何前缀映射覆盖的目的邮编；任何前缀映射都优先于它。它是目录内容，随目录版本化、进摘要；它对应哪个区或档是租户取值。
- 没登这一行的目录，查不到照旧待判断，ADR-0109 决定四不变。
- 不做「查不到就落某区」的兜底，价卡也不声明兜底分区：兜底把数据缺口变成有价，错价会进择优与结算。命中的区在价表里没有费率，照旧待判断。

**五、新键写下一版规范化版本，旧卡与旧目录字节不动。**（机制）

- 决定一至三在定价方案的规范化文档里加键，照 ADR-0267 决定一换号并取能表达内容的最低一版，三项同批一次换号。决定四加在目录登记的规范化形状上，同此办。
- 不带新键的方案与目录，规范化版本、内容摘要、评价语义摘要与快照逐字节不变；本构建认全部既有版本。
- 方案记着较新版本的价卡照 ADR-0352 要部署开关才能登记，不另立门。

三类：

- **机制**：新键与换号、评价按声明分支、票级快照补逐件量、目录「其余」行的解析先后。
- **产品策略**：决定一至四给出的全部形态，以及不做未命中兜底。
- **租户取值**：每张卡选哪种读法、基数、聚合方式与进位点；目录登不登「其余」行、它对应哪个区或档。演示租户不因本记录改种子。

## 对已接受记录的作用

- 部分停用 ADR-0348 决定二「先后固定如下，卡上不再声明顺序：基础运费 → 固定规则 → 附加费（互斥组裁决已完成，百分比费用已按各自基数算出）→ **总额上下限** → 结算币种换算与换算后取整 → 比较币种 → 合计取整。」与「**补差行 / 冲减行不进任何费用依赖的基数。**」两句、决定六「总额上下限只对「各行之和」整票生效，不对运费一项单独设基；见风险点 0。」一项。适用场景：只限声明了「只对基础运费」基数的卡；整票基数的卡照原句，ADR-0348 其余各条不变。回答其越权风险点 0 与 2。
- 部分停用 ADR-0111 决定二「携带成员包裹引用与合计量（件数、总实重、总体积重——按重量策略派生的合计计价重量是评价内的中间结果，与逐包裹时同形）」一句，扩成：声明逐件聚合的票级卡另带逐件实重与尺寸，计价重量为逐件派生之和。适用场景：只限声明了逐件聚合的票级卡；主单级与不声明的票级卡照原句。
- 部分停用 ADR-0109 决定二「目的邮编前缀 → 值的映射」一句，扩成：外加每个始发维度至多一条「其余邮编」映射。适用场景：只限登了这一行的目录版本；决定四「查不到即待判断，不给默认」不变。

## Consequences

- 能表达三类今天表达不了的卡：「不超过 X」的阶梯、最低运费计入燃油基数、多件按件计重。转抄不再需要平移档位。
- 原型手册那一套「缺价兜底」不进本产品；承运商表里的「其余地区」经显式一行表达，审得到、复算得出。
- 价卡导入模板（`PPT-2`）与管理台编辑器不认新键，导入暂不支持这几项；要经导入接，另随模板换版（ADR-0349 的路子）。
- 票级逐件求和要求输入解析器把成员逐件的实重与尺寸交进快照，取源与逐包裹评价相同。
- 目录匹配粒度那一格（ADR-0109 决定二）仍未定。定的时候前缀比较要按国家的邮编格式进行：英国邮编去掉空格后按前缀比，`SW1` 会同时命中 `SW10` 至 `SW19`。

## Alternatives considered

- **档位边界一律改成含上界。** 否决：会改变已登记卡的读法；左闭右开的写法同样存在，声明式两读法让旧卡原样成立。
- **档位边界保持写死，靠转抄平移。** 否决：转抄者要自造下一个可表示值，进位规则一改就错；判定条件已为同一理由提供两种比较。
- **最低运费用条件最低计价重量模拟。** 否决：最低收费是金额下限，不是重量下限；跨重量段或附加费按重量计时两者不等价。
- **票级只留合计后派生。** 否决：多件按件计重的卡算不对，BUY 与承运商账单对不上。
- **票级一律逐件。** 否决：空运主单与整票按合计计费的卡会算错。
- **价卡声明兜底分区（原型手册的做法）。** 否决：兜底把缺数据变成有价，错价进择优与结算；与 ADR-0189 决定一拒绝固定区同一理由。

## 越权风险点

1. **档位边界只开两种读法。**（决定一）不开两端都含（相邻两档重叠）与两端都不含（留缝）；真实卡另有写法时另立。归 PP owner。
2. **进位分段的分界照旧写死为不含。**（决定一）本记录只动价表档位，不动重量取整的分段分界（`TestSegmentedRoundingTreatsTheSegmentBoundaryAsExclusive`）；真实卡要含端时另立。归 PP owner。
3. **只对基础运费设基时，上限同样只比运费，补差一律进以运费为基数的依赖。**（决定二）上下限共用一个基数、不分端声明；有卡要「补差不进燃油」时另立。归 PP owner。
4. **逐件求和只许一处进位。**（决定三）真实卡若逐件进位后再对和进位，另立。归 PP owner。
5. **主单级不开逐件。**（决定三）归 PP、TF owner。
6. **「其余邮编」行每个始发维度至多一条，前缀之间的重叠与优先另随粒度那一格定。**（决定四）归 PP owner。

## Links

- [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md)：总额上下限与 `PPC-6`；本记录部分停用其决定二两句与决定六一项，回答越权风险点 0、2
- [ADR-0111](./0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md)：票级与主单级评价主体；本记录部分停用其决定二一句
- [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：计价参考目录；本记录部分停用其决定二一句，决定四不变
- [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)：决定一拒绝给按目的邮编分区的卡登固定区，本记录决定四同一理由
- [ADR-0267](./0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md)、[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：换号与取能表达内容的最低一版
- [ADR-0352](./0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)：登记较新规范化版本价卡的部署开关
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类
- [ADR-0365](./0365-an-origin-dimension-zone-catalogue-takes-the-origin-the-price-card-declares-sender-postal-code-or-the-billing-origin-of-the-legs-start-node.md)、[ADR-0366](./0366-margin-is-neither-an-acceptance-nor-a-selection-gate-and-transit-time-is-an-eligibility-constraint-not-a-ranking-dimension.md)：同批代决
- [冻结清单「记录」](../../.scratch/launch-line-freeze-2026-10-27.md)：实现项与建议分级
