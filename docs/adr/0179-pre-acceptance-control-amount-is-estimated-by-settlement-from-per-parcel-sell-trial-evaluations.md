# ADR-0179：受理前控制金额由结算按委托当前提交版本的逐件 SELL 试算合计形成——估价归结算、试算归计价，选卡与正式 SELL 评价同口径，计价范围随采用登记给出，没登记答未配置

Status: Accepted（2026-10-12 20:4x，用户经 IDP 队列授权通道 1 代裁接受，原话「你是业务和系统专家，你来做这个评估吧」；接受时改了决定二、决定六各一处，补了 ADR-0048、ADR 索引与 PP `CONTEXT` 的指针，接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-12，通道 3 随票 `product-strategy-boundary/06` 第 2 项分诊，派单 `task-85d7350d` ← 通道 1。起草方裁决能力边界：读过 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0152](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 全文，ADR-0048、[ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)、[ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 的 Decision，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」与 PN-02 行及其补记，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-15` 行，PS、SA `CONTEXT` 里接受前财务控制与估价的词条和规则，PP `CONTEXT` 试算对象那条规则，[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) 的输入表、主流程与接受前财务控制一节；代码读过 PS 的 `EstimationAmountSource`、`PreAcceptanceControlAdapter`、`AdvanceFinancialControlJudgmentHandler`，PP 的 `FormEstimateEvaluationsHandler`、`readingCompletion`、`FormEvaluationFromRequestHandler`、`PriceCardInForceResolver`，PS 的申报测量与地址要素读口头注，`cmd/parcel-dispatch` 的 `acceptanceFinancialControl`。没读：SA `ApplyPreAcceptanceControlHandler` 冻结与信用两支的金额用法全文、SA 冻结与信用暴露两张表的列、PC 价格政策发布翻译、PP `CONTEXT` 全文。拿不准的列在文末「越权风险点」。）
Date: 2026-10-12

## Context

**受理前控制金额今天在生产上没有来源。** `cmd/parcel-dispatch` 的 `acceptanceFinancialControl` 把 PS 消费侧适配器的金额源留空，控制停在 `CONTROL_AMOUNT_NOT_CONFIGURED`。演示动线停在这一格（[psb/05](../../.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 5）：实跑于 `ff3b01fa`，只记 `S`，记录在票 [routing-first-cut/19](../../.scratch/routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md)「分诊」一节。登记册 `PAR-COM-15` 已按 ADR-0146 把「按价卡估出控制金额的方法」划为产品策略。

**现成的执行器把方法的每一格都当成了租户参数。** ADR-0048 决定三的第三增量落了 `EstimationAmountSource`：声明画像译计价输入，评价合计译最小币单位，缺件停在未形成。价卡、计价范围、区域、结算币种、最小币单位位数与证据层级都经 `EstimationBasisSource` 交进来，那时这几格整块算实例半边，适配器因此只翻译。这个接口至今没有实现，`NewEstimationAmountSource` 也没有生产调用点。按 ADR-0146 重看，下面几格各有现成的答案或缺口：

- **归属与权威相抵。** SA `CONTEXT` 写「本上下文拥有接受前估价、冻结、信用暴露和限制结果」，PS `CONTEXT` 写 PS「不拥有金额、余额或冻结」，UC-SA-002 主流程把「形成估价、冻结、信用暴露或限制」排在结算的一步里。ADR-0048 把金额缝装配器放在 PS 消费侧，当时成立，因为它只翻译。一旦选卡、选区域、逐件合计成了方法，判断就落进适配器，与 ADR-0025「适配器只翻译，不判断」相抵。冻结金额从哪份价卡、哪几份评价来，今天哪一侧都不留痕。
- **它绕过了共用的补齐读数。** ADR-0152 决定三让正式评价与试算共用 `readingCompletion`。`EstimationAmountSource` 直接调纯函数 `EvaluatePricing`，输入里不带序列取值与目录读数。卡上绑了序列或分区目录时，纯函数落 `PENDING`（`REFERENCE_SERIES_UNRESOLVED` 一类），金额永远形不成。演示售价卡 `SYN-PLAN-CN-SG-01` 正绑着燃油序列 `SYN-SERIES-FUEL-01`。
- **区域给法与正式路径不同。** 估价基给整份委托一个区域串。地址要素按件申报。正式 SELL 评价（ADR-0171）用申报的邮编路线造输入，分区由价卡绑定的分区目录解析（ADR-0152 决定五）。ADR-0171 的申报测量与地址要素读口头注写明「包裹 → 委托只经接受基线的声明成员走」，受理前没有接受基线，它们答「无」，受理前估价用不上。
- **币种有两处来源。** 估价基带一个币种，控制作用域（结算账户所在的那组资金坐标）也带一个。冻结占的是作用域那一户的钱。最小币单位位数是 ISO 4217 的公开数据，PP 领域的 `Currency.MinorUnitScale` 已经内置，不是租户取值。
- **选卡没有从商业闭包回指的路。** 受理链闭包的目的是接受控制，PS 解析键登记面不收 `PRICE_RULE`，`FormResolutionKey` 头注写着计价目的的键「届时按新决定另开登记面」。正式 SELL·客户费用评价按（租户、计价范围、方向、目的、时点）经 `PriceCardInForceResolver` 解析在用价卡，不走 PC 价格政策的定价方案绑定（ADR-0034）。SA 主要计费范围与 PP 计价范围是不是同一标识空间，PP 的 SA 消费侧适配器（`evaluation_request.go`）注释写明它「是否同一标识空间归两侧 owner」裁、「这里不猜任何映射」。
- **受理前没有预估客户费用可引。** UC-SA-002 写「合同要求估价时，估价可以引用预估费用」。SELL 评价的发起时点是发生项形成（ADR-0169），受理前没有发生项，也就没有 SELL 评价请求与预估客户费用。BUY 与 SELL 两张触发册（ADR-0164、ADR-0169）帮不上这一格。

## Decision

**一、估价归结算，试算归计价。** 受理前估价由 settlement-accounting 形成。策略要求冻结或信用校验时，结算先形成估价，本次各控制项用同一份估价金额。试算评价由 parcel-pricing 形成，只算不存、不交付（ADR-0152 决定二的结构）。parcel-shipment 发起控制时不再交金额，只交委托、提交版本、判断时点与商业解析回指。估价不形成费用、预估费用或应收应付。

**二、首版内置一种形态：逐件 SELL 试算合计。** 对委托当前提交版本的每一件声明包裹，用同一张在用价卡各形成一份试算评价，按结算币种合计成控制金额。停法照搬现有执行器：任一件缺申报测量或邮编路线、任一份评价没有合计（待判断、不可计价、冲突、失败）、任一份合计的币种不是控制作用域的币种、合计不是正数，估价都答未形成。不拿零、部分合计、换算或另一种给法顶替：要以结算币种出价，靠价卡自己声明的计价参考序列（CONTEXT-MAP「计价消费汇率是为了让 `SELL` 成本加成评价在形成时就产出结算币种金额」），结算不换算。

**三、选卡与正式 SELL·客户费用评价同口径。** 按（租户、计价范围、`SELL`、客户费用、时点）经 `PriceCardInForceResolver` 解析唯一在用价卡。时点取本次控制回显的判断时点，即规则包时点格采用的形态（ADR-0157）。未配置与适用冲突各自停，不挑卡。估价与随后形成的客户费用因此按同一条规则选卡。正式路径将来若改走商业价格政策的绑定，两处同一笔改。

**四、输入按提交版本取，分区与币种不另设来源。** 逐件的申报测量与收寄两端邮编按委托当前提交版本读，由计价一侧造输入快照，PS 为此开一只按（租户、委托、提交版本）作答的读口。分区按价卡绑定定：绑了分区目录的卡由邮编路线解析分区；没绑的卡在本形态下没有调用方给的分区，估价答未形成。结算币种取本次控制作用域的币种，最小币单位按 ISO 4217，由产品内置。证据层级由装配声明。

**五、采用即登记，没登记答未配置。** 结算新开一册：键是（租户、结算账户），值是（形态、计价范围），不设修订，照 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 的不可变元组先例。册上没有这一户时，估价答未配置，控制停在金额未配置这一格（今天由 PS 适配器答 `CONTROL_AMOUNT_NOT_CONFIGURED`，改由结算交出后照适配器译提供方原因的规矩译），不默认、不拿零顶。登记动作与登记行同一事务发出「参数已登记」类信封，供小包托运续办（ADR-0094 决定四）。金额这一格因此可以与票 routing-first-cut/19 的分格同笔挪进`等待运营登记`。不登任何租户的行，不预选形态。

**六、估价随控制结果留痕。** 形态、计价范围、价卡版本引用、逐件试算评价的标识、版本清单与合计，跟冻结与信用暴露的结果一起保存。试算评价本身不进计价的评价册，所以版本清单只在这份留痕里：同一输入与版本清单可以复算出同一金额。

## Consequences

- 接受后部分停用 ADR-0048 决定三第三增量的落点：金额缝装配器不再放在 PS 消费侧，`ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 随实施退场。ADR-0048 决定一、二（声明测量是成员画像、随提交版本保全）不变。ADR-0048 的 Status 区与 Links、ADR 索引两行随接受补了指针。
- PS 开一只按提交版本作答的申报测量与邮编读口，不经接受基线。控制请求不再带金额。
- PP 开「按委托提交版本形成受理前试算」的入口：单卡在用解析、共用补齐读数、不入册、不交付。PP 的 PS 消费侧适配器读上一条读口。「对给定价卡形成试算并走共用补齐读数」那一步与本记录的去留无关，先行，见票 [psb/24](../../.scratch/product-strategy-boundary/issues/24-plan-given-estimate-evaluations-use-shared-reading-completion.md)。
- SA：估价端口、采用登记册（迁移）、`parcel-settlement-register` 的登记命令、施加编排在要金额时先形成估价、估价留痕，以及 SA 的 PP 消费侧适配器。实施见票 [psb/25](../../.scratch/product-strategy-boundary/issues/25-pre-acceptance-estimate-formed-by-settlement.md)。
- 要随实施改的文档，本记录不改：UC-SA-002 输入表「财务控制请求」一格的「金额或额度」去掉 PS 交金额的含义；CONTEXT-MAP 补 SA → PP 的受理前试算与 PP → PS 的提交版本申报读口。PP `CONTEXT` 不等实施：「评价对象」词条与「评价对象为试算对象时」那条规则写的是不变式，留着原句，接受后就与本记录字面相抵，所以两处例外句随接受当时补上。
- 演示租户的售价卡绑分区目录、演示委托带邮编、结算账户的采用行，都是演示租户的租户取值，随种子灌 `SYN-` 值，证据记 `S`，见票 [psb/26](../../.scratch/product-strategy-boundary/issues/26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)。
- 执行器落地后，开发主线 PN-02 行「受理前控制金额 `Amounts` 为 nil」补带日期的补记，登记册 `PAR-COM-15` 行的「〔ADR-0146 拆分〕」一句收短。

## Alternatives considered

- **留在 PS 消费侧，补一个估价基源（ADR-0048 决定三现状）。** 改动最小，执行器已在。否决：选卡、选区域、逐件合计都成了判断，落在适配器里与 ADR-0025 相抵；PS `CONTEXT` 明写不拥有金额；冻结金额的来历两侧都不留痕。
- **选卡走 PC 价格政策的定价方案绑定（ADR-0034）。** 合同协商价能按客户区分。暂不取：受理链要为计价目的另解一次闭包，解析键登记面要另开；正式 SELL 评价也不走它，估价单独走就会与随后的客户费用用不同的卡。
- **计价范围从本轮已采用商业解析的范围回指（照 ADR-0156）。** 不必另登。否决：商业范围与计价范围是否同一标识空间没有裁，回指等于替两侧 owner 定了。
- **按户登记一个固定分区**（票 routing-first-cut/18 候选一在客户侧的对应物）。否决：分区是件的目的地经价卡目录得出的属性，按户登一个分区对多目的地客户必然估错，估错占的是客户的钱。routing-first-cut/18 问的是网络段起讫，节点没有邮编；客户委托有申报邮编，不同题。
- **首版就内置加成、按件定额等形态。** 不取：没有形态证据，加了就是替租户猜（ADR-0146 越权风险点 1）。它们是同一扇门的后继形态，加时不改本记录。
- **用预估客户费用作估价。** 受理前不存在，见 Context 末条。

## 越权风险点

1. 决定一改了 ADR-0048 决定三的落点，并要求 UC-SA-002 输入表改写。归 SA、PS owner 复核。 **owner 复核 2026-10-12 认可**（用户授权通道 1 代裁，理由见「接受记录」）
2. PP `CONTEXT` 写「评价对象为试算对象时，评价不得进入结算」。本记录按 ADR-0152 Context 的展开（`settlement-accounting`「不得据其形成任何费用、预估费用或应收应付」）读它，冻结不在其中；结算保存试算评价标识作留痕算不算「进入结算」，归 PP owner 复核。 **owner 复核 2026-10-12 认可**（用户授权通道 1 代裁，理由见「接受记录」）
3. 决定四的 PS 读口不经接受基线。本记录把 ADR-0171 读口头注里「只经接受基线的声明成员走」读作它们的今日形状，不是不变式；这一读法归 PS owner 复核。 **owner 复核 2026-10-12 认可**（用户授权通道 1 代裁，理由见「接受记录」）
4. 决定五的登记键是结算账户。同一账户下不同产品要用不同售价卡时，只能分账户或分计价范围。归 SA、PC owner 复核。 **owner 复核 2026-10-12 认可首版**（用户授权通道 1 代裁，理由见「接受记录」）
5. 决定三把估价绑在正式 SELL 评价的选卡口径上，后者改则两处同改；决定二首版只有一种形态。 **owner 复核 2026-10-12 认可**（用户授权通道 1 代裁，理由见「接受记录」）

## 接受记录（2026-10-12 通道 1）

用户经 IDP 队列授权通道 1 代裁，原话「你是业务和系统专家，你来做这个评估吧」。接受方读过：本记录全文；SA `CONTEXT` 的接受前财务控制词条、所有权那一句、「资金冻结与核销」生命周期与冻结转扣费那条规则；PS `CONTEXT` 写 PS 不拥有金额、余额或冻结的两句；PP `CONTEXT`「评价对象」词条与「评价对象为试算对象时」那条规则；ADR-0152 的 Context 与决定六；ADR-0158 决定一；ADR-0048 全文；CONTEXT-MAP 里计价与结算之间的各条。起草方列为「没读」的几项，接受方同样没读。

越权风险点逐条：

1. 认可。SA `CONTEXT` 写明本上下文拥有接受前估价，PS `CONTEXT` 写明 PS 不拥有金额、余额或冻结；估价放进结算是照所有权放，不是新取舍。ADR-0048 决定三那一格是在「适配器只翻译」的前提下放的，前提一变，落点跟着变。
2. 认可。PP `CONTEXT` 那条规则挡的是试算形成费用、预估费用、应收应付与客户承诺。冻结与信用暴露是占用与额度，SA `CONTEXT` 写冻结在费用确认后才转为正式扣费，而费用照旧要另有一次针对已受理包裹的评价，那条规则要守的东西本记录一样不碰。结算保存试算评价的标识与版本清单只作留痕，不算进入结算。两处原句随接受补了例外句，免得字面相抵。
3. 认可。受理前没有接受基线，提交版本就是 ADR-0048 决定二随之保全画像的那一版；ADR-0171 读口头注说的是接受后正式评价那条路今天的形状。
4. 认可首版。登记册照 ADR-0158 不设修订、键只到结算账户：同一账户下按产品分卡今天只能分账户或分计价范围，给一户换形态或换计价范围也没有路。首个要按产品分卡或要换范围的租户出现时，另立记录给键加维度、给出停用与接续；首版没有这类形态证据，现在加就是替租户猜（ADR-0146 越权风险点 1）。
5. 认可。

接受时的改动（起草方的取舍不变）：

- 决定二补一格停法：任一份合计的币种不是控制作用域的币种，也答未形成，结算不换算。原稿写了「按结算币种合计」，没写币种对不上时怎么办，留空会被读成可以相加。
- 决定六的留痕补版本清单。原稿说「同一输入与版本清单可以复算出同一金额」，而试算评价不入册；版本清单不随留痕存，事后就没有可复算的那一份。
- 指针：ADR-0048 的 Status 区「Partially suspended by」与 Links；ADR 索引 0048、0179 两行；PP `CONTEXT` 两处例外句；票 product-strategy-boundary/25、26 转 ready-for-agent，25 的「做什么」第 4 条与完成判据照改，06 状态行记一句。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：分界检验，本记录的形态与登记属产品策略
- [ADR-0048](./0048-declared-measurements-are-verbatim-member-profiles.md)：决定三第三增量的落点被本记录部分停用（接受后）
- [ADR-0152](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)：试算只算不存、共用补齐读数、分区给法逐卡定
- [ADR-0171](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md)：正式评价的输入来自申报测量与地址要素
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：登记动作发信封续办
- [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md)：结算登记册的不可变元组先例
- [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md)、[ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：发生项之后的两张触发册
- 票 [product-strategy-boundary/06](../../.scratch/product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项与「第 2 项分诊」一节
