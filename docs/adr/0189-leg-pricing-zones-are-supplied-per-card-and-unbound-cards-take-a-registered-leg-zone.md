# ADR-0189：逐段计价区域按卡给——绑了分区目录的卡由包裹邮编路线经目录读出，未绑的卡取租户按线路段登记的段计价区域

Status: Accepted（2026-10-18，Claude Code 会话代决：用户原话「你作为业务和系统专家自决。解决所有的问题」；票 [routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md) 的产物。补 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条留给 PP 计价输入缝的那一格，越权风险点 5 由本记录接手。越权风险点见文末，待 PP、NR 与 SA owner 复核。）

Date: 2026-10-18

## Context

- 候选成本逐段向 parcel-pricing 取评价（ADR-0148 决定四）。每段要一个价卡上的区域，ADR-0148 决定四第 7 条写明「每段怎样折成价卡的区域词汇随 PP 的计价输入缝定。NR 不自算计价重量，也不自定区域」。这一格没定之前，`cmd/parcel-dispatch` 给逐段区域装的是恒答未配置的一只，演示动线停在初始路由 `CANDIDATE_COSTS_PENDING`（2026-10-16 在 `c68e4342` 上重走实测）。
- 票 18 列了两种候选形态：一，段依据行带区域；二，价卡绑分区目录、段起讫按邮编折成区域，节点得带邮编。
- parcel-pricing 已有的口径（[ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定四、[ADR-0152](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md) 决定三与决定五、[ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定四）是**分区给法逐卡定**：绑了分区目录的卡用包裹的邮编路线经目录读出区域；没绑的卡走调用方给的分区。正式评价与估价同一份读数补全实现。正式的 BUY 评价按运输收费发生项计价，发生项不带段的起讫，区域也取包裹寄件到收件的邮编路线。
- 演示网络的线路版本是参考配置的采用行：内容取自那一版参考配置，依据格写成引用串（ADR-0147 决定四）。参考配置随产品发布，装不下某个租户的计价区域——那是租户取值。把区域写进线路版本内容，租户就得另登一版不是采用行的线路版本；新版本还换出新的候选身份（候选标识带线路版本，如 `SYN-LINE-CN-SG-01@1`）。成本依据随线路版本走。演示成本卡没绑目录、只有 `Z1`。网络节点不带邮编。

## Decision

1. **逐段区域按卡给（产品策略）**，与 parcel-pricing 其余评价同一口径：
   - 卡**绑了分区目录**：区域由 parcel-pricing 用这件包裹的邮编路线经该卡的目录读出。邮编路线就是寄件到收件那两端邮编：NR 随初始路由请求携带它（ADR-0075），交给 PP 消费方适配器，经 PP 共用的读数补全评价，与正式 BUY 评价、受理前估价同一份实现。
   - 卡**没绑分区目录**：区域取租户为这一段登记的**段计价区域**（决定二），作为调用方给的分区交给 PP。NR 原样转交、不解释；区不在卡上时由计价答待判断。
   - 段计价区域只适用于**整段区域恒定**的卡，例如按线路计价的干线。按目的邮编分区的卡（典型是尾程）必须绑分区目录、走第一条：给它登一个固定区，会静默算出一个错价参与择优，而不是待判断。演示尾程登 `Z1` 是 `SYN` 简化。
   - 两样都没有：这一段待判断，候选缺成本依据（CONTEXT-MAP「候选评价为待判断或不可计价时，该候选按缺少成本依据处置」）。不从节点推邮编，不拿卡上唯一的区当默认。
2. **段计价区域登记（机制）。** network-routing 增一本册：键（租户，线路代码，线路版本，段号），值为区域词与登记依据。
   - 只登在挂了成本依据的段上：区域说的是「按那一段所引的卡计价时用哪个区」，没有卡就无从谈区。
   - 不随线路版本内容走：区域另行登记，采用行照旧是采用行，候选身份也不变（见 Context 讲采用行的那一条）。登记只增不改，同一段再登撞主键；也不推进目录修订（同成本依据）。
   - 区域词须首尾无空白，登记门与库上约束都挡：PP 的计价输入把带首尾空白的区当输入不成形，整次候选成本取数报错，而不是这一段待判断。
   - 今天只有受控登记口（`parcel-network-register -kind leg-pricing-zone`）。在线登记口随管理台那一面再开。
3. **不采形态二**：节点不加邮编。parcel-pricing 的分区目录按包裹目的邮编分区，这是承运商分区表的原义，也是正式 BUY 评价的口径。按节点地址分区是另一种目录形态，等首个真实租户的分区表需要时再定（ADR-0109 决定二「粒度随首份真实分区表定」）。
4. **分两步落。** 段计价区域那一路随票 18 落。绑目录那一路要 NR 把携带的邮编路线交进成本取数侧，并改走 PP 的读数补全，归票 [routing-first-cut/20](../../.scratch/routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)。在那之前，绑了目录的成本卡照旧待判断。

三类：

- **机制**：段计价区域登记册与读口；NR 把区域与邮编路线交给 PP；读数补全。
- **产品策略**：按卡给的规则（决定一）。
- **租户取值**：哪张卡绑哪个分区目录、各段登哪个区。演示租户经种子登 `SYN-` 值：三段各 `Z1`。

## Consequences

- 演示租户登了三段 `Z1` 之后，初始路由的候选出价（演示声明 2.5 KG 时三段各 85 CNY，合 255 CNY）。计划可以形成，往下是有效网络收寄。
- 未绑目录、按线路计价的成本卡，在候选成本里由段计价区域给区；但正式 BUY 评价按发生项计价，发生项没有段，取不到段计价区域。这类卡在正式 BUY 评价上仍是待判断，那条路要自己的区域来源，随 BUY 评价触发面（psb/20）一并定。择优用的成本与事后记的供应商预期成本，在这类卡上会一时对不齐。
- 迁移 `network_routing/0019` 立表，外键指向成本依据行。

## 越权风险点

1. **区域词住进 NR 的册**（PP owner）：区域是 PP 的词汇，本记录让 NR 按段原样持有、不解释，校验留给 PP 计价时答待判断。若 PP owner 认为未绑卡的调用方分区应由 PP 自己按（卡，线路段）登记，册要搬到 PP。
2. **另立一本册而不进线路版本内容**（NR owner）：成本依据随线路版本走，段计价区域不随。理由见 Context 讲采用行的那一条：进线路版本内容就丢了采用行、换了候选身份。代价是同一段的计价坐标分在两处。
3. **正式 BUY 评价对未绑卡取不到区**（PP、SA owner）：见 Consequences 第二条。
4. **册只增不改，登错一个区今天没有更正口**（NR owner）：只能另起线路版本、连同成本依据重登，形态一被否的那个代价在更正这一步上仍在。按修订更替一段之区的更正口，等首个真实租户要改区时再开。

## Links

- [ADR-0148：路由证据按来源分三路，候选成本由逐段成本依据合成](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：本记录补它决定四第 7 条，接手越权风险点 5
- [ADR-0109：邮编分类事实归 parcel-pricing，作为版本化的计价参考目录登记](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：决定四「分区给法逐卡定」
- [ADR-0152：运营试算是计价自有的应用用例](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)：决定三与决定五，估价与正式评价同一份读数补全
- [ADR-0179：受理前控制金额由结算按逐件 SELL 试算合计形成](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：决定四
- [ADR-0075：客户地址随判断请求在请求期携带过界](./0075-customer-address-is-carried-with-the-routing-request.md)：邮编路线的来处
- [ADR-0147：参考配置随产品版本嵌入发布](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：决定四，采用行与依据格的引用串
- 票：[routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md)（本记录）、[routing-first-cut/20](../../.scratch/routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)（绑目录那一路）
