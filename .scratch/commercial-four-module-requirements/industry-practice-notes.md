# 行业做法笔记：报价、一票多件、签署证据与价卡授权

本文件是 [spec](./spec.md)「用户的决定」Q1 至 Q4 的取证附件，不是规则，也不是[竞品公开事实台账](../../docs/product/COMPETITOR-FACT-LEDGER.md)的一部分。要不要把其中某几条升进台账，另行决定。

**为什么查**：这四问原本要向给出要求清单的一方确认。2026-11-11 用户在会话里答「没有可问的人，分析，参考行业头部软件的做法」。

**核对日期**：2026-11-11；「客户价的声明处、回退与取价时点」一组是 2026-11-12。公开文档随时会改，引用任何一条都连这个日期一起引；照台账的规矩，超过一个季度没重核的视为过期。

**取法**分三种，写在每条的「取法」一栏：

- **直接抓取**：用抓取工具读过该页正文。
- **存下的全文**：搜索工具把该页全文存成了本地文件，在文件里检索到了引的那句。
- **摘录**：只看到搜索工具返回的该页正文摘录，没有另行抓取。PDF 都属这一种。

**状态**沿用台账的三态：`已核实`、`部分未核`、`未取到`。

**看了谁**：SAP TM、Oracle OTM（运输管理软件）；Salesforce CPQ、Conga CPQ（报价到合同）；金蝶云星空（国内企业软件的合同与签章）；易仓 TMS（国内跨境快递、小包、专线系统）；DHL、FedEx、UPS（一票多件的计费规则）。**没看**：CargoWise、Magaya、K5，以及 nShift、Metapack 这类多承运商发货软件。台账里已有的 Shippo 与 Easyship 两家，取价都发生在建运单那一步，没有报价对象，本文件不重复登。

## 核到的事实

### 报价

| # | 来源 | 取法 | 状态 | 核到了什么 |
|---|---|---|---|---|
| R1 | [SAP Learning：Creating Forwarding Quotations](https://learning.sap.com/courses/exploring-sap-s-4hana-transportation-management-for-logistics-service-providers/creating-forwarding-quotations) | 直接抓取 | 已核实 | Forwarding Quotation 针对一次具体运输的询价。可设有效期以保证算出的价格；客户须在有效期内接受或拒绝；接受后由报价生成 Forwarding Order，费用取自报价。报价上不能开始计划 |
| R2 | [SAP Learning：Managing Forwarding Agreements](https://learning.sap.com/courses/exploring-sap-s-4hana-transportation-management-for-logistics-service-providers/managing-forwarding-agreements) | 直接抓取 | 已核实 | Forwarding Agreement 是与客户的长期合同，是计算对客费用的依据。客户发来询价后，承运方建 Forwarding Agreement Quotation；原文：「If the customer accepts the offer and signs a contract, the forwarding agreement quotation is turned into a forwarding agreement」。也可以从一份已有协议里发起协议报价，用来更新这份协议 |
| R3 | [SAP TM 9.1 SP04 Release Notes](https://help.sap.com/doc/0b591ffbcf6845dea7e2c5554406a6d4/9.1/en-US/Release_Notes_SAP_Transportation_Management_91,_SP04E.PDF) | 摘录 | 已核实 | 「You use forwarding agreement quotations to publish your transportation rates in a trade lane.」 |
| R4 | [Oracle OTM：Quote Request](https://docs.oracle.com/en/cloud/saas/transportation/26c/otmol/planning/order_manager/quote/quote_request.htm) | 直接抓取 | 已核实 | 报价请求页用来「search for valid rates in order to provide the customer with the quote they requested」，必填的只有起运地与目的地；可勾选只取与该客户所在域相同的专属费率；分采购与销售两个视角 |
| R5 | [Salesforce Trailhead：Create Account-Based Contracted Pricing](https://trailhead.salesforce.com/content/learn/modules/pricing-methods-for-salesforce-cpq/create-account-based-contracted-pricing) | 直接抓取 | 已核实 | 合同价是挂在客户账户上的记录，可带生效日与失效日；给这位客户的报价加行时自动取用；已加的行是快照，之后改合同价不影响它。文中提醒不要让同一账户的两条合同价作用于同一产品，否则「there's no way to dictate which contracted price is used」 |
| R6 | [Conga CPQ：Configure Contract Pricing through Price Agreements](https://documentation.conga.com/en/cpq-for-salesforce/current/cpq-for-administrators/managing-pricing/contract-pricing/when-to-use-contract-price-lists/configure-contract-pricing-through-price-agreements) | 摘录 | 已核实 | 意图为价格协议的报价定稿并被接受后，系统按报价里谈定的价格自动生成一份合同价目表，带合同号，供以后的报价取用 |
| R7 | [易仓：销售操作教程](https://www.eccang.com/manual-24.html) | 直接抓取 | 已核实 | 客户协议价格维护：「系统默认的最先取这里面的价格，其次是等级价，最后才是销售价」。协议等级价审核：「只有在这里面审核了客户协议等级价才会生效」。运费报价申请：「根据运单号来调整运费金额」，其后是审核与审批，「建议不同的操作人员来进行申请，审核，审批」。另有销售运费试算，用来验证价格表 |

### 一票多件

| # | 来源 | 取法 | 状态 | 核到了什么 |
|---|---|---|---|---|
| R8 | [DHL UK：Weight and Dimensions](https://www.dhl.com/discover/en-gb/ship-with-dhl/products-and-services/weight-and-dimensions)（页面日期 2023-11-21） | 直接抓取 | 已核实 | 「If you are sending multiple packages, calculate each weight separately and this is the 'total chargeable weight'. The chargeable weight is always the greater of the actual or volumetric weight.」 |
| R9 | [FedEx 亚太：多件货件计费重逻辑调整说明](https://www.fedex.com/content/dam/fedex/apac-asia-pacific/downloads/fedex-invoice-sample-en-jp.pdf) | 摘录 | 已核实 | 此前多件货件按整票合计的实重与体积重取大者计费；自 2022-04-25 起改为逐件取实重与体积重的大者，各件相加作为整票的计费重 |
| R10 | [UPS 2026 Tariff Guide（英国）](https://assets.ups.com/adobe/assets/urn:aaid:aem:9b3de62a-351c-48f8-bcd0-07203835d6a2/original/as/tariff-guide-in-gb-en.pdf) | 存下的全文 | 已核实 | 逐件比较实重与体积重取大者；「For multiple-package shipments, total the billable weight of all packages in the shipment.」 |
| R11 | [易仓 TMS 11 月版本功能更新要点](https://www.eccang.com/bbgx_107) | 直接抓取 | 已核实 | 「按票分泡时，先将总实重和总材积重进位，再按分泡比例分泡，最后再进位取整」；同一票货可以用两个进位值不同的计费重，分别算干线运费与末端派送运费 |
| R12 | [易仓：服务渠道](https://bbs.eccang.com/notbook/112) | 摘录 | 未取到 | 直接抓取答 403。搜索引擎的摘录里有：「票总重：一票货里面所有的件之和来进行进位之后产生计费重。」「件总重：货物每件进行进位之后相加的和再进行进位，最后产生计费重。」这一条没有核实，下面凡用到它的地方都注明 |

### 签署

| # | 来源 | 取法 | 状态 | 核到了什么 |
|---|---|---|---|---|
| R13 | [SAP Signature and Agreement Management by Docusign](https://www.sap.com/products/crm/signature-and-agreement-management.html) | 直接抓取 | 已核实 | SAP 的签署能力是与 Docusign 合作的产品，嵌在 SAP Sales Cloud 与 SuccessFactors 的流程里：在 SAP 里发起、发送、跟踪，签完的数据同步回 SAP |
| R14 | [金蝶官网：金蝶云・星空合同中心](https://www.kingdee.com/resources/articles/1426272354573791137)（2025-10-09） | 直接抓取 | 已核实 | 「起草过程中，用户可根据需求自由选择针对本合同启用电子签章，或直接采用手签方式。」电子签章的合同附件同步为业务合同附件 |
| R15 | [金蝶云社区：合同电子签章使用方式](https://vip.kingdee.com/article/651377910552445696) | 摘录 | 已核实 | 要先启用电子签章集成配置，并到电子签章服务商处认证；合同保存、提交、审核之后上传到服务商，参与方凭短信或邮件里的链接签署 |

### 客户专属价怎么取到

| # | 来源 | 取法 | 状态 | 核到了什么 |
|---|---|---|---|---|
| R16 | 同 R2 | 直接抓取 | 已核实 | 「The system determines the agreement based on the organization, business partner, validity period, and preconditions.」找到协议与协议项之后，按它的计算表取费率表 |
| R17 | [Oracle OTM：Rate Offering](https://docs.oracle.com/en/cloud/saas/transportation/26c/otmol/planning/rate_manager/create_rate_offering.htm) | 存下的全文 | 已核实 | 费率协议可带客户域约束：有约束且订单所在域相容时，只返回带约束的那一份。视角分采购、销售与两者皆是。另有一格「Contract Rate」标明这是客户专属费率，可配合权限检查，按销售运单用的是不是客户专属费率来拦订舱动作 |
| R18 | 同 R5、R7 | 直接抓取 | 已核实 | Salesforce 的合同价挂在客户账户上，给这位客户报价时自动取用；易仓的客户协议价先于等级价与销售价被取用，且要审核后才生效 |
| R19 | 同 R7 | 直接抓取 | 已核实 | 价格套餐维护：「在这里面可以将销售产品设置不同的等级和绑定不同的杂费，销售价格启用的时候也可以绑定这里面的价格套餐。」客户价格维护：「根据不同的客户去设置不同的价格套餐。」客户协议价格维护：「引用了公布价之后这里面就只能是在公布价的基础上进行折扣」。成本运费的名词解释：「服务商给货代公司的一个价格，会低于公布价和销售价。」 |

### 客户价的声明处、回退与取价时点（2026-11-12 补，票 15）

这一组的核对日期是 2026-11-12。它们由会话派出的检索子代理取得，会话没有逐页重开。「直接抓取」指子代理读到了该页正文；Oracle 的三页是用只读 `curl` 取 HTML 读的。同一行里读到原文与只见摘录的句子分开标。

| # | 来源 | 取法 | 状态 | 核到了什么 |
|---|---|---|---|---|
| R20 | [SAP Learning：Setting Up Access Sequences](https://learning.sap.com/courses/implementing-sap-s-4hana-cloud-public-edition-sales-configuration/setting-up-access-sequences-for-price-determination_db10f482-810c-4a93-8837-926550f85958) | 直接抓取 | 已核实 | 取数序列是一串编号的步骤，按编号从小到大逐个问：「access 10 might point to a table with keys 'Customer/Material', while access 20 points to a table with only 'Material'.」每一步可设命中即停：「it will stop searching further down the sequence for that particular condition type」 |
| R21 | [SAP Learning：Creating Freight Agreements](https://learning.sap.com/courses/charges-and-settlement-in-sap-s-4hana-transportation-management/creating-freight-agreements) | 直接抓取；后两句摘录 | 部分未核 | 读到：「for forwarding agreements, the standard determination rules use sales organization and customer.」只见摘录：多份协议命中时按协议优先级定先后（[来源](https://learning.sap.com/courses/charges-and-settlement-in-sap-s-4hana-transportation-management/determining-agreements-and-charge-calculation-rules)）；费率、汇率与协议有效性按哪个日期取可以配置，例如订单日、发票日、主运段预计结束日（[来源](https://learning.sap.com/courses/business-processes-in-sap-s-4hana-transportation-management/calculating-charges-in-freight-documents_af5f5f28-7b6e-40bb-ba2d-5e2681edb163)） |
| R22 | [Oracle Advanced Pricing User's Guide：Price Lists](https://docs.oracle.com/cd/E26401_01/doc.122/e48845/T328362T328369.htm) | 直接抓取 | 已核实 | 「If the item you are ordering is not in the primary price list, the pricing engine looks for the price on any attached secondary price list.」多份次级价目表按 precedence 一格取先后。主价目表上有这一项但已过有效期，也转用次级价目表：「If an item appears on the primary price list but is not active (the effective end date has passed), the pricing engine uses the price on the secondary price list.」 |
| R23 | [Oracle Advanced Pricing User's Guide：Agreements](https://docs.oracle.com/cd/E26401_01/doc.122/e48845/T328362T328372.htm) | 直接抓取 | 已核实 | 协议写与客户谈定的价格、付款条件与运费条件；「Define new prices or attach an already existing price list to the agreement.」订单引用协议才用得上协议价；协议价目表「Not usable to price orders not associated with an agreement.」 |
| R24 | [Using Oracle Workflow in Oracle Order Management](https://docs.oracle.com/cd/E26401_01/doc.122/e48844/T393423T393427.htm) | 直接抓取 | 已核实 | 取价日期默认保留订单行原来的那一天，可以配置成实际发运日、计划发运日、履约日、承诺日、请求日或系统日；重新取价时可以只重算基础价、保留下单时给的折扣 |
| R25 | 同 R5；另一页 [Identify Exceptions to Standard Pricing](https://trailhead.salesforce.com/content/learn/modules/pricing-methods-for-salesforce-cpq/identify-exceptions-to-standard-pricing) | 直接抓取；后一页摘录 | 部分未核 | 读到：合同价可以写成单价，也可以写成折扣；按报价行创建的时刻取，之后改合同价不影响已有的行。只见摘录：价目表里缺某个产品的条目时不回退，这个产品加不进报价 |
| R26 | 货代好帮手 ITS：[客户专属报价](http://doc.goto56.com/project-3/doc-196/)、[客户等级报价](http://doc.goto56.com/project-3/doc-195/) | 直接抓取 | 已核实 | 「客户计费获取逻辑：客户专属报价优先，客户等级报价处之，最后公布价。」等级报价靠「授权客户群使用该客户等级报价」指派。文档没写专属报价缺线路或重量段时回不回退，也没写按哪个时间取价 |
| R27 | [易仓：价格体系配置](https://www.eccang.com/manual-18.html) | 直接抓取；末句摘录 | 部分未核 | 读到：「系统按到货时间查找生效的价格计费，确保在修改重量等信息时重新计费，还是按交货时的价格计费。」「对于生效状态的价格表进行修改，系统将进行时间分隔的方式进行处理。」只见摘录：「关联了公布价输入的则是折扣，不关联公布价输入的则是价格」 |
| R28 | [FedEx Pricing Agreement（SEC 备案，2005）](https://www.sec.gov/Archives/edgar/data/1263756/000119312505184853/dex1039.htm)；另两份 FedEx 协议 PDF | 直接抓取；两份 PDF 摘录 | 部分未核 | 读到：价格自协议签署后的约定日起生效；「Only the account and barcode numbers verified by FedEx will participate in the pricing.」货件适用「the FedEx Service Guide in effect at the time of shipment」。只见摘录：折扣是公布价上的百分比，公布价取交运当日有效的那一版（[来源](https://www.fedex.com/content/dam/fedex/us-united-states/accounts/images/3015ProgramAgreement.pdf)）；协议没覆盖的服务按公布价（[来源](https://www.fedex.com/content/dam/fedex/us-united-states/accounts/upload/Program_agreement_US10704SAM.pdf)） |
| R29 | [Oracle OTM：Rate Preference](https://docs.oracle.com/en/cloud/saas/transportation/25a/otmol/planning/rate_manager/rate_preference.htm) | 摘录 | 部分未核 | 「A rate is considered a contract between the user and a service provider or a user and their clients.」客户专属费率怎么挂，没有取到 |

没取到的：UPS 与 DHL 的协议价规则；`help.sap.com` 与 `help.salesforce.com` 的原文（两家只取到官方学习站）；Oracle 的折扣修饰符；K5 只见产品介绍里一句「客户等级及特殊报价」，不足为证；递四方、燕文、云途等没有查。

## 据此的读法

下面是会话的读法，不是事实。Q1 至 Q3 由会话代答，用户 2026-11-11 认可；认可的是读法，它们仍然没有经过任何一位真实客户。Q4 的读法撤回过一次，留下的那个决定用户同日已定：重开。

### Q1 报价：报一套价，不做逐票锁价

- 头部软件把两种报价做成两个对象。针对一次运输的报价，终点是一张订单（R1、R4）。针对一段期间费率的报价，终点是一份协议或一套合同价（R2、R3、R5、R6）。
- 要求清单写的是「客户接受与价格生效申请」。接受之后生效的是价格，不是订单，对上的是后一种。
- 国内的同类系统里，对客价是销售价、等级价与客户协议价三层，协议价审核后生效（R7）。它没有事前的逐票报价；按运单的那一种是事后改价，走申请、审核、审批。
- 所以 [ADR 草案](./adr-draft-quotation-offers-a-set-of-prices.md)的决定一不变。按运单的事后改价不归报价，归费用调整。

### Q2 分票：两种算法都要，由价卡声明

- 三大快递现行的口径都是逐件取大者再相加（R8、R9、R10）。FedEx 在 2022 年之前是整票合计后取大者（R9）。
- 国内系统里两种并存：分泡按票合计（R11）；服务渠道上分票总重与件总重（R12，未核实）。
- 这与 [ADR-0364](../../docs/adr/0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md) 决定三一致：聚合方式由价卡声明，两种都给。
- 三大快递对多件货件都按票计费，这是采购成本。采购一侧算不了一票多件，成本就对不上。见下面「多出来的三处」。
- 是不是主营：会话的判断是小包多为一票一件，一票多件主要在快递代理与专线渠道。这一句没有找到一手来源，所以它不拿来给分票排先后。用户 2026-11-11 选定全部激活，分票几张的先后只看阻塞边。

### Q3 签署证据：先登记，电子签章以后做成可选的对接

- 头部企业软件不自建签署，都对接专门的服务商（R13、R15），并且允许不走电子签章、直接手签（R14）。
- 所以票 03 的形态不变：登记签署信息与签署件索引。将来接电子签章时，它的回执落成同一种签署证据，只是来源不同。

### Q4 价卡授权：原读法已撤回；留下的那个决定，用户已定重开

- 头部软件里，客户专属价都经一个受管的客户绑定取到：协议（R16）、带客户约束的费率协议（R17）、挂在账户上的合同价、审核后生效的客户协议价（R18）。
- 这几条只说明客户专属价要有一个受管的绑定，说明不了本产品该把它放在哪一处。会话 2026-11-11 先据此推出「不必让评价去读合同，在两道登记门上核对」，那一步没有依据，同日撤回。
- 本产品今天有两处：合同的价格政策上的定价方案绑定，与结算账户登记的销售计价范围（[ADR-0280](../../docs/adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md)）。起作用的是后一处。[ADR-0260](../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 的「候选与反方」否决过「两处都留，计价核两处相等，不等即冲突」，越权风险点 2 写明不加两侧一致性核对。它把按合同选卡留作重启条件，理由是首发没有客户差异化销售价的证据。
- 国内同类系统的客户价是分层的：销售价、等级价与客户协议价，取价有先后；协议价可以写成公布价上的折扣；按客户挂价格套餐（R7、R19）。本产品的销售价只有一层：同一个计价范围上只有一张在用的销售卡，结算账户登了范围之后不能换。
- 所以这里要的不是一道门，是先定两件事：哪一处说了算，客户价要不要分层与回退。它们归票 [15](./issues/15-one-authority-for-which-price-a-customer-uses-and-the-customer-price-structure.md)。用户 2026-11-11 定：重开。
- 2026-11-12 补查的 R20 至 R29 给出三点：客户价声明在合同或协议上的与声明在客户记录上的都有（R21、R23、R28 对 R25、R26）；有回退的，顺序都是配置或声明出来的，或者写死成固定几层（R20、R22、R28 对 R26、R7），Salesforce 不回退（R25）；取价时点各家不同（R21、R24、R27、R28）。票 15 的 [ADR 草案](./adr-draft-the-customer-contract-declares-ordered-sell-price-sources.md)用到的是前两点；它把声明放在合同上、让合同声明回退顺序，是对着本仓已接受的记录定的，这几条只是旁证。

## 多出来的三处

1. **采购一侧的一票多件。** 立票 [14](./issues/14-buy-side-multi-piece-charge-occurrences-cannot-be-priced.md)。
2. **进位叠不叠两次。** 易仓的件总重写的是逐件进位、相加之后再进位一次（R12，未核实）。ADR-0364 决定三写的是逐件进位后求和，或求和之后进位一次，「不叠两次」。进位按重量分段时，两种读法结果不同。记在票 [06](./issues/06-shipment-level-weight-aggregation-lands-in-the-domain-and-the-import-template.md)「要裁的」。
3. **客户价的层次与回退。** 易仓是销售价、等级价与客户协议价三层，并按客户挂价格套餐（R7、R19）；本产品只有一层。立票 15。这里原来写的是「登记客户用哪套价时核对合同，并进票 04」，已撤回。

另有一条顺带的对照：Salesforce 对重叠的合同价是任取一条、且无法指定取哪条（R5）。本产品对重叠的在册版本答适用冲突、不任取，票 [02](./issues/02-price-card-preview-and-publication-show-overlap-with-registered-versions.md) 只是把这个冲突提前到发布时提示。

## 这份笔记的边界

- R12 没有核实，用到它的两处都已注明。
- 没看的那几家写在开头。
- 各家文档描述的是它们自己的产品，面向的行业与本产品不尽相同：SAP TM 与 OTM 主要服务货代与大宗运输，Salesforce 与 Conga 是通用的报价软件。只有易仓与三大快递的规则直接落在跨境小包与快递上。
- 「小包多为一票一件」是判断，不是核到的事实。
- 客户价分层在 2026-11-11 只核了易仓一家；2026-11-12 补了 R20 至 R29，其中半数带只见摘录的句子，逐行标了。这一组不是会话自己逐页读的。
- 这些事实回答得了「同行怎么做」，回答不了「本产品的某一格该放在哪个上下文、由哪一处说了算」。后一类是模型取舍，要对着本仓已接受的 ADR 定。Q4 第一次的读法错在这里：拿行业做法直接推出了一个本仓 ADR 已经否决的做法。
