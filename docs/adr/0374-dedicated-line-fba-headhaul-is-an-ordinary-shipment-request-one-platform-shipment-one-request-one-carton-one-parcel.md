# ADR-0374：专线型 FBA 头程是普通委托——一个平台货件一份委托、一箱一个包裹，差异由服务产品的规则承载；平台货件号与箱标号是外部标识，仓配平台备货整份提交一份委托；号码的界面叫法一并定下，货代型头程不在产品内

Status: Accepted（2026-11-10，Claude Code 会话代决：用户原话「这三点我们都要参照头部系统的做法确定掉， 我们这个系统也需要支持FBA的」，随后问「国际小包和FBA你觉得需要分成两个独立的入口，还是一致处理呢，头部系统的做法是什么呢」「你觉得是科学，专业的做法吗」「FBA是不是传统的货代业务啊」，会话答模型统一、入口按场景分，只承接专线型、不承接货代型，用户答「可以，按这个范围写吧」。取舍与参照记在票 [declared-goods-lines/01](../../.scratch/declared-goods-lines/issues/01-decide-goods-details-order-references-and-fba-headhaul.md)。部分停用 [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) 决定四一句、[ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md) 决定一、决定二与决定五各一句、[ADR-0371](./0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md) 决定一一句，并对 ADR-0371 决定一另一句作读法限定，被停原句、改法与适用场景见「对已接受记录的作用」。代小包托运、仓配履约 owner 与产品 owner 决，越权风险点见文末，待小包托运、仓配履约、节点作业、参与方与商业、全程追踪与异常 owner 与产品 owner 复核；2026-11-10 已由子代理代小包托运、仓配履约与产品 owner 复核（报告 [owner-review-2026-11-10](../../.scratch/declared-goods-lines/owner-review-2026-11-10.md)），十条阻断随同笔改掉。实现另立票，本记录不动 `internal/**`。起草能力边界：同 [ADR-0373](./0373-a-shipment-requests-goods-details-are-invoice-lines-with-a-packing-allocation-kept-verbatim-and-replaced-as-a-whole.md) 所列，另读过开发主线的目标客户群、能力范围判据与产品需求假设表，仓配履约 CONTEXT 的打包、装箱记录、仓配出库提交、平台备货与出库请求、装箱记录两段生命周期；没读：运输履约与路由 CONTEXT 全文、计价对整票汇总重量的实现。）<!-- 索引注记 -->｜其决定四「平台箱标号与客户包裹件号走同一套：随首次提交声明在委托上，不随提交版本重报；」一句已由 [ADR-0376](./0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定二停用，**适用场景**：受理前（委托`已提交`），平台号可经受控补充补报或更正，接受后不再改；决定四其余各句不变。｜**判据**：专线型 FBA 头程（快递头程，以及尾程走快递派送入仓的空派、海派：按箱计价与操作、尾程逐箱打末端渠道面单送平台仓）与国际小包同一个委托模型、同一条受理链，下单入口可按场景分；一个平台货件一份委托、一箱一个包裹，收件资料是平台仓；不设 FBA 标志，平台货件号、箱标号、装箱分配报全等必填由服务产品的接单规则包说；外部标识加平台货件号、平台参考号（标识委托）与平台箱标号（标识包裹），分配方是销售平台，箱标号与客户包裹件号同一套（接受时形成关联、已提交期间算歧义信号）；仓配平台备货经本产品网络整份出库请求打包完成后提交一份委托，装箱记录逐箱记所贴箱标，不预取面单，来源请求键（出库请求，提交轮次）；界面上运单号指委托、跟踪号指渠道包裹件号，多件委托逐件列；整柜、海卡与尾程走卡派的那一截不在产品内，送仓预约、进口商、平台收货差异另定

## Context

用户要求系统支持 FBA。FBA 是亚马逊的仓储与代发货服务；物流公司卖的是「FBA 头程」——把卖家的货从起运地运进平台仓。头程按产品形态分两类：

- **专线型**：快递头程、空派、海派。按公斤卖、有固定价表与最低计费重，揽收、按箱扫描测量、集运、干线、清关，尾程逐箱打 UPS 或 FedEx 一类末端渠道面单送仓，按箱追踪。经营者常常就是小包专线公司，与小包共用网络、场站与尾程渠道。
- **货代型**：海运整柜或拼箱、海运加卡车送仓（海卡）、按票报价的空运。订舱、提单、箱号、托盘、零担或整车、预约送仓，按柜或按票追踪。

[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)把「小包专线与货代：头程自营或半自营、尾程交末端渠道」列为目标客户群，能力范围判据要求一项能力对应目标客户群里的常规商业模式。专线型头程是这类客户的常规产品线，链路与小包重合；货代型是另一种生命周期。

头部系统的做法：国际快递（DHL、FedEx、UPS）与多承运商平台（EasyPost、Shippo）一套下单模型，FBA 就是一票多件寄到平台仓地址、箱标作参考号；国内跨境物流商与卖家 ERP 对卖家分开「小包下单」与「FBA 头程下单」两个入口，字段差别大。规律是承运方统一模型，面向卖家的入口分开。亚马逊要求每一箱都有唯一的 FBA 箱标、每一箱都给箱内明细；同一个入库计划分到几个平台仓时，每个仓各是一个货件、各有货件号。

现状里与此相抵的是仓配履约：平台备货经本产品网络发出时逐包裹各提交一份委托（ADR-0370 决定一、决定五），一个五十箱的平台货件成了五十份委托，平台货件号无处挂，整票报关、整票计费都要从五十份委托里拼。号码方面，委托的单据号界面词是「运单号」（[ADR-0355](./0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md)），而国内小包行业说运单号常指每件的跟踪号，一单多件时容易混。

## Decision

**一、专线型 FBA 头程用委托承接，货代型不在产品内。**

- 快递头程，以及尾程走快递派送入仓的空派、海派，与国际小包同一个委托模型、同一条受理链，受理、关务、计价、追踪、结算与异常都复用。空派、海派里尾程走卡派（托盘、预约送仓）的那一截不在内，直到运输履约另定。
- 海运整柜或拼箱、海卡与其他以订舱、提单、托盘、预约送仓为核心的货代型头程不在产品内；要做，按能力范围判据另立决定，像仓配那次的 ADR-0368。
- 这一判断登记为开发主线的产品需求假设 `PA-FBA-01`，第一个真实客户到来时复核。

**二、一个平台货件一份委托，一箱一个包裹。**

- 委托的收件资料是平台仓（仓代码与地址）；委托的成员共享收件关系，所以一份委托只去一个平台仓。平台把一个入库计划分到几个仓时本就给出几个货件号，各成一份委托。
- 货物明细按 ADR-0373：商品与申报值写在委托的货物明细行上，每箱装了什么写在装箱分配里。
- 整票受理照 `BD-PS-007`：不支持只受理一部分箱子，一箱资料有问题就整票补正或拆票。
- 一个平台货件一份委托是常态，不是不变量：补正时拆票、客户加箱形成关联的新委托、仓配拆单，都会让同一个平台货件号挂在几份委托上；按平台货件号查的读口要交回多份。

**三、不设 FBA 标志，差异由服务产品的规则承载。** 「这是 FBA」由服务产品说。平台货件号、每箱的平台箱标号、装箱分配报全、收件人是平台仓等要求，是该服务产品所采用接单规则包的必填与跨字段条件（`BD-PS-006`），产品不内置为常量，没配置就不判。按整票汇总计费重量由价卡声明（[ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)）。下单入口可以按场景分（小包下单与 FBA 货件下单两页、两套批量导入模板），首方 API 用同一个提交端点、带可选的平台段；入口只是呈现，提交的都是同一种委托。

**四、外部标识加三种类型。**

- 平台货件号（如亚马逊的 FBA Shipment ID）与平台参考号（如亚马逊的 Reference ID）标识委托。
- 平台箱标号（如亚马逊每箱的 FBA 箱标）标识包裹。
- 分配方是销售平台，适用范围是该平台的编号空间；销售平台是开放取值、原样保存。号由客户交来，本产品不向平台核对。
- 平台箱标号与客户包裹件号走同一套：随首次提交声明在委托上，不随提交版本重报；委托接受时按接受基线形成关联，依据是接受决定；委托`已提交`期间，被声明的箱标号在按号核对包裹身份时同样算歧义信号——FBA 箱子在委托未接受时到站，节点据此答身份冲突、等声明落定；委托一经拒绝或撤回，声明不再算。它的适用范围不是客户账户的编号空间，所以与渠道包裹件号一样恒范围相容。同一委托里两箱报同一个箱标号答请求形状不对：平台给每箱唯一的箱标，委托内重号只能是录错；这一点与客户包裹件号不同，那边重号留给节点答身份冲突。
- 平台货件号与平台参考号在委托接受时形成关联；按它们查委托的读口另定。

**五、仓配平台备货经本产品网络整份提交一份委托。**

- 平台备货的出库请求在整份打包完成后，以货主名义提交一份委托，带全部包裹（整箱出时一箱一个包裹）；货物明细按 ADR-0373 决定十由各包裹的装箱记录（整箱出取各箱箱单）按商品汇成行、逐箱成分配。
- 打包时装箱记录逐箱记下所贴的是哪一张平台箱标（标签文件里的箱标号），提交时作该包裹的平台箱标号；平台货件信息里的货件编号作平台货件号；平台货件信息不带平台参考号，仓配交来的委托这一格照实空着。
- 代理仓出库的平台备货，等代理仓报来的装箱结果覆盖整份出库请求再提交。
- 平台备货不预取面单：预取按一个包裹先提交，装不下一个平台货件。
- 来源请求键是（出库请求，提交轮次），首次为 1；委托被拒绝或撤回后经运营决定重新提交时加一，同一键不复用。重提与把问题箱拆包回库可以并用。请求证据指名出库请求、提交轮次与所含的全部包裹序号。
- 一件代发等其余出库请求照旧逐包裹提交。

**六、号码的界面叫法。**

| 界面词 | 指什么 |
|---|---|
| 运单号 | 委托的单据号，一份委托一个 |
| 客户单号 | 外部标识「客户参考号」，客户系统里这一票的号 |
| 包裹号 | 包裹的本系统号（[ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 提交答复逐件带回的号） |
| 客户件号 | 外部标识「客户包裹件号」 |
| 跟踪号 | 外部标识「渠道包裹件号」（含尾程件号），收件人在末端承运商处查得到的号 |
| 平台货件号、平台参考号、平台箱标号 | 决定四的三种外部标识 |

多件委托在列表上显示运单号与件数，详情逐件列包裹号与跟踪号，有平台箱标号的一并列。运单号不当跟踪号显示。客户面按运单号追踪的答复（ADR-0356）今天逐件只带包裹号、来源关系与客户包裹件号，要列跟踪号与箱标号得扩那份答复，属全程追踪与异常的读口，待 VE owner 会签。

**七、下列不在本记录。**

- 送仓预约、托盘与零担或整车：运输履约与路由另定。
- 进口商（IOR）与按一般贸易清关的角色资格：关务与参与方另定。
- 平台收货差异与短少索赔：全程追踪与异常、结算另定。
- 与平台对接：箱内明细由货主自己报给平台，本产品不推送，照 ADR-0370 决定四不对接平台。
- 按平台货件号查委托的客户读口：随实现另定；客户眼下可按运单号查，逐件列出全部箱子（[ADR-0356](./0356-customer-tracking-locates-by-the-shipment-document-number-and-lists-the-current-effective-parcels-of-an-accepted-shipment.md)）。

## 对已接受记录的作用

下列各句部分停用，正文不改写，被停原句逐字引用：

- [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) 决定四「这份委托的来源是「仓配出库」，来源请求键取自（出库请求，包裹序号）。」——改为：平台备货的出库请求，来源请求键取自（出库请求，提交轮次）。**适用场景**：平台备货经本产品网络发出。其余各条不变。
- [ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md) 决定一「**本产品网络**：出库请求带要求的服务产品，逐包裹以（出库请求，包裹序号）提交委托，来源「仓配出库」，照 ADR-0368 决定四。」——改为：平台备货的出库请求整份打包完成后提交一份委托，带全部包裹，来源请求键（出库请求，提交轮次）。**适用场景**：平台备货。其余各条不变。
- ADR-0370 决定二「只对发运方式为本产品网络、已全部分配的出库请求，按一个包裹（包裹序号 1）提交委托。」——改为：平台备货的出库请求不预取面单。**适用场景**：平台备货。其余各条不变。
- ADR-0370 决定五「发运方式为本产品网络时一箱一份委托，货物内容取该箱的箱单，缺的申报格取箱单所列商品的商品申报资料，都没有的照实空着。」——改为：平台备货整箱出时整份出库请求一份委托、一箱一个包裹，货物明细由各箱箱单汇成行与装箱分配，缺的申报格照旧取商品申报资料，都没有的照实空着。**适用场景**：平台备货的整箱出。其余各条不变。
- [ADR-0371](./0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md) 决定一「提交的来源请求键是（出库请求，包裹序号）；其余各请求的来源请求键照 ADR-0310 决定一加命令前缀，不与提交键相撞。」——改为：平台备货的提交来源请求键是（出库请求，提交轮次）；其余各请求照旧加命令前缀。**适用场景**：平台备货。其余各条不变。

另对 ADR-0371 决定一「请求证据指名出库请求与包裹序号」一句作读法限定（不是停用）：平台备货的请求证据指名出库请求、提交轮次与所含的全部包裹序号。指针挂在 ADR-0371 的索引注记。

## 同笔改动的文件

- 上列三份记录的 `Status` 行索引注记与 Links 各加一条前向指针，ADR-0371 另加一条读法限定，正文不动。
- [GLOSSARY](../domain/GLOSSARY.md)「歧义信号」：声明的号加平台箱标号。
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)「给开发的交接」：客户面逐件列跟踪号与箱标号的差距；[UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md)「给开发的交接」：平台号接受后更正的差距。
- [小包托运 CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「来源请求身份」补平台备货的键；「外部标识关联」与 Rules 外部标识一条加三种类型；新词条「号码的界面词」。
- [GLOSSARY](../domain/GLOSSARY.md)：「外部标识」举例加平台号；「出库请求」提交委托一条按决定五改。
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：仓配履约到小包托运一条按决定五补。
- [仓配履约 CONTEXT](../domain/warehouse-fulfillment/CONTEXT.md)：发运方式、平台备货、打包、装箱记录、仓配出库提交词条，Rules 的本产品网络、预取面单、委托被拒绝或撤回、整箱出各条，出库请求生命周期一条。
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：验收示例与给开发的交接。
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：三层表「小包专线与货代」一行补专线型 FBA 头程，产品需求假设表加 `PA-FBA-01`。
- ADR 索引与 AT 盘点计数由推送方重生成，不手改。

## Consequences

- 专线型 FBA 不用另建一套流程：受理、关务、计价、追踪、结算与异常都复用，整票计费与整票报关有了单一的委托可挂。
- 平台备货的委托数从按箱计变成按货件计；仓配要等整份打包完成才提交，FBA 这一类失去预取面单的并行。
- 一箱资料有问题，整票不能受理；要么补正，要么拆票。
- 外部标识多三种类型，客户核对与歧义规则照旧适用。
- 界面上运单号与跟踪号分清了；管理台与客户面的现有页面要按决定六改称呼与多件展示。
- 货代型头程的租户要接外部货代系统，或等另一个能力范围决定。

## Alternatives considered

- **FBA 单独一套入口与模型。** 国内物流商对卖家就是两个入口。否决：生命周期与规则相同，拆开要把受理、关务、计价、追踪做两遍，同一件事两处权威；入口分开放在呈现层就够了。
- **委托上加一个 FBA 标志。** 下游好判断。否决：差异全是规则（必填项、计费汇总、送仓要求），放在服务产品上已经能表达；标志会与服务产品说的话重复、还可能矛盾。
- **一箱一份委托（现状）。** 仓配已经这样做。否决：平台货件号挂不上、整票报关与计费要拼，五十箱就是五十次受理。
- **把整柜、海卡一并做进来。** 货代客户也常做。否决：订舱、提单、托盘与预约送仓是另一种生命周期，不在委托的模型里；按能力范围判据要单独定。
- **平台货件号与箱标号写进货物明细或备注。** 不用加类型。否决：号码要能被核对、有分配方与适用范围、有歧义规则，那正是外部标识的职责。
- **平台备货的来源请求键只用出库请求。** 最简单。否决：委托被拒绝后要以关联的新委托重新提交，键不能复用，所以要带提交轮次。

## 越权风险点

1. 范围边界（决定一）：专线型与货代型的分界按经营形态划，空运整票报价、海派里带托盘送仓的边缘情形要逐个判。归产品 owner。
2. 整票受理（决定二）：一箱有问题整票不能受理，依赖 `BD-PS-007`。归 PS owner。
3. 外部标识三种类型（决定四）：分配方是销售平台、号由客户交来不核对。归 PS owner。
4. 平台备货整份提交（决定五）：要等整份打包完成、不预取、键带提交轮次；部分箱子先发的需求要拆出库请求。归仓配履约与 PS owner。
5. 界面词（决定六）：管理台与客户面现有页面的称呼要跟着改。归 PS owner 与管理台。
6. FBA 必填由规则包承载（决定三）：产品不内置，未配置时不判，FBA 服务产品上线前租户要配好。归 PC 与 PS owner。
7. 不在本记录的几项（决定七）：送仓预约归运输履约，进口商归关务，平台收货差异归全程追踪与异常和结算。各归其 owner。
8. 平台箱标号算歧义信号（决定四）：节点按号核对包裹身份时多一类号进歧义判断，FBA 箱子在委托未接受时到站会答身份冲突。归 PS 与节点作业 owner。
9. 客户面逐件列跟踪号与箱标号（决定六）：要扩按运单号追踪的答复。归 VE owner。
10. 逐包裹的代办请求（包裹取消一类）在平台备货下的来源请求键：要带包裹序号，免得同一委托逐箱取消两次撞键；随仓配切片定。归仓配履约与 PS owner。

## Links

- 票 [declared-goods-lines/01](../../.scratch/declared-goods-lines/issues/01-decide-goods-details-order-references-and-fba-headhaul.md)：逐题裁决、真实场景走查与参照出处
- [ADR-0373：委托的货物明细](./0373-a-shipment-requests-goods-details-are-invoice-lines-with-a-packing-allocation-kept-verbatim-and-replaced-as-a-whole.md)：同日，货物明细与装箱分配
- [ADR-0368：商品库存与仓配作为新的限界上下文](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md)：部分停用决定四一句
- [ADR-0370：仓配出库按发运方式分三种](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md)：部分停用决定一、决定二、决定五各一句
- [ADR-0371：来源为「仓配出库」的委托由仓配履约以货主名义代办](./0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md)：部分停用决定一一句
- [ADR-0223：首方提交答复逐件带回包裹号](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)、[ADR-0355：单据号](./0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md)、[ADR-0356：客户追踪按运单号查](./0356-customer-tracking-locates-by-the-shipment-document-number-and-lists-the-current-effective-parcels-of-an-accepted-shipment.md)：决定六的号码出处
- [ADR-0364：价卡声明整票重量汇总](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)：决定三的整票计费
- [开发主线：能力范围判据与产品需求假设](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
- [owner-review-2026-11-10](../../.scratch/declared-goods-lines/owner-review-2026-11-10.md)：子代理代各 owner 的复核报告
- [ADR-0376：FBA 提交条件按必要资料判，平台号受理前可补报，规则包随服务产品选](./0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md)：部分停用本记录决定四一句
