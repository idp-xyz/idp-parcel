# ADR-0359：法人资料的监管登记号、收款账户与多语言开票抬头——资料层登记号从税务放宽到监管机关发放的登记号而线上名不改；收款账户是法人资料的可选版本化明细、归 party-commercial、账户币种不决定结算币种；多语言抬头挂在主抬头上而`资料不全`仍只看主抬头；产品随发布一份注册号类型参考表

Status: Accepted（2026-11-03，协调方代 party-commercial owner 决（代决），用户 2026-11-03 裁定「你自己来决策」；票 [company-subject-ux/05](../../.scratch/company-subject-ux/issues/05-cross-border-profile-extensions.md)。部分停用 [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一与决定三各一句并回答其越权风险点 5，部分停用 [ADR-0350](./0350-legal-entity-profile-representative-addresses-and-contact-purposes.md) 与 [ADR-0351](./0351-contact-social-accounts-are-optional-versioned-profile-details.md) 的快照形状句，被停原句与适用场景见「对已接受记录的作用」。裁决能力边界：读过 ADR-0145、0146、0147、0025、0267、0345、0350、0351 全文，party-commercial [`CONTEXT.md`](../domain/party-commercial/CONTEXT.md) Language「责任法人」「法人资料」与 Rules「法人资料」诸句，[GLOSSARY](../domain/GLOSSARY.md)「法人资料」「结算账户」，[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)「法人资料维护补充」，法人资料的领域、持久化、在线口与受控 CLI 代码，注册号类型目录的参考配置包与采用路径。没读：settlement-accounting 的 `CONTEXT.md` 与对账单、发票开立的代码，customs-compliance 对申报主体资料的需求，各国监管机关的一手规范原文——参考表只收本记录能从公开规范确认格式的类型。越权风险点待 PC owner 复核；收款账户与结算账户的边界待 SA owner 会签。）<!-- 索引注记 -->｜**取法**：资料层登记号放宽为税务、海关等监管机关发放的登记号，`taxRegistrationNumbers` 名窄于义、不改名；收款账户是法人自己的银行账户，归 PC、随资料修订版本化，币种是账户属性不是结算币种；多语言抬头挂在主抬头上，`资料不全`仍只看主抬头；快照含任一新格写 LEP-4；参考表按国家 / 地区出 CN@2、SG@2、HK、US、GB、JP 与欧盟各成员国，无明确公开格式的类型不随附
Date: 2026-11-03

## Context

取证于 main `eb4049625`。

- **资料层登记号只叫「税务登记号」。** ADR-0145 决定三把法人资料的登记号定为「税务登记号（可后办、可变的那类，如 GST / VAT 登记号）」，注册号类型目录的资料层也以它命名。国际小包要印到单据、报给监管方的号不止税务一类：海关注册编码、AEO 认证编号、IOSS、EORI 都是监管机关发给法人、可后办可变的号，判断方式与税务登记号一模一样——按目录里的类型判格式、随资料修订生效时点判在用。另开一组「海关登记号」是第二套同样的机制。
- **收款账户没有归属。** ADR-0145 越权风险点 5 写明「收款账户、开票语言等是否属法人资料，本记录不裁——收款账户与结算账户（SA 拥有）的关系尤其要先理清」。对账单、发票要印收款账户，今天没有任何一册登它。settlement-accounting 的结算账户是「固定责任法人、相对方、方向、币种与结算政策的业务账户」（GLOSSARY「结算账户」、ADR-0158），不是银行账户；两个概念同名不同义，先理清再落。
- **开票抬头只有一种语言。** 跨境单据常要中英双抬头，或按收件方所在地印当地语言。今天 `invoiceTitle` 一格只能放一个抬头，而 `NO_INVOICING_DETAILS` 的判据正是它有没有。
- **既有机制能接住。** ADR-0350、0351 已把法人资料扩成带可选明细、随完整修订保存、缺席即未配置、快照按最低可表达版本标形状（LEP-2、LEP-3）的形态；ADR-0147 已立注册号类型参考配置的存放、版本与采用路径，CN、SG 两份只含身份层。本记录把三件事都接在这些机制上，不另造。

## Decision

**一、资料层登记号放宽为监管登记号，线上名与类型名不改。** 法人资料里的「税务登记号」改叫**监管登记号**：税务、海关等监管机关发给法人、可后办可变的登记号——GST / VAT / IOSS / EORI、海关注册编码、AEO 认证编号都在这一组。类型来自注册号类型目录的资料层，按国家 / 地区配置，判号方式不变（按资料修订的生效时点在目录里判类型与格式）。线上字段 `taxRegistrationNumbers`、领域类型 `TaxRegistrationNumber` 与目录资料层的内部名**不改**：名窄于义是刻意付的——改名换不来任何判断上的差别，却要让已在册的快照、在线口与受控 CLI 的载荷、管理台都换一次字，还得给旧名留一条兼容路。界面词用「监管登记号（税务 / 海关）」，由管理台的票落。（机制）

**二、产品随发布一份注册号类型参考表。** 沿 ADR-0147 的存放与采用路径，`referenceconfig/party-commercial/registration-number-types/` 按国家 / 地区出：CN 第 2 版（统一社会信用代码；纳税人识别号、海关注册编码）、SG 第 2 版（UEN；GST 登记号）、HK（公司注册编号 CR No.；商业登记号码 BR No.）、US（EIN）、GB（公司编号；VAT、EORI）、JP（法人番号；适格请求书发行事业者登记番号）与欧盟各成员国（VAT、IOSS、EORI）。只给公开规范有明确格式的类型；没有统一公开格式的——AEO 认证编号、美国州注册号、欧盟各成员国的商业登记号、新加坡非 ACRA 登记实体的 GST 编号——**不随附、不猜**，各份原文的 `note` 写明缺了哪几类与原因，补上走新版本。CN、SG 第 1 版照旧可引，已采用的租户不受影响。参考表是产品附带的参考配置：租户显式采用才进目录，采不采用、采哪些是租户取值；演示租户灌 `SYN-` 版本，证据只记 `S`。（产品策略）

**三、收款账户是法人资料的可选版本化明细，归 party-commercial。** 一笔资料修订带零或多个收款账户，每个：用途标签（可选，自由文本）、户名、开户行名称、账号或 IBAN、SWIFT / BIC（可选）、开户行地址（可选）、**币种**（ISO 4217，必填）、备注（可选）。同一修订内（币种，账号）不得重复，同一账号不同币种是两个账户；各格去首尾空白，集合保留登记次序。缺席即这一版不带收款账户；旧修订读作「未登记收款账户」，不回填，不设默认账户，不增加开票门禁。

币种是账户自身的属性——这个账户收哪种钱——不是结算币种的决定。CONTEXT 原话照抄：「结算币种由结算政策按责任法人、相对方、合同与费用范围解析」，ADR-0145 决定四不变。币种的形状在领域构造门锁 ISO 4217，是否在产品内置目录里由新登记的写入边界判（ADR-0345 决定五、六）；SWIFT / BIC 与账号不锁形状，各国账号形态各异。

**所有权边界**：收款账户是法人自己的银行账户；`settlement-accounting` 的结算账户是「固定责任法人、相对方、方向、币种、结算政策的业务账户」，两者不是一个概念，也不互相推导。对账单、发票要印收款账户时，开单方按单据固定的资料修订引用（ADR-0145 决定五）与单据币种，经消费侧适配器（ADR-0025）向 party-commercial 取，不复制、不在 SA 另登一份。本记录只写这条边界，不改 SA；SA 怎么选账户、选抬头归它的开立票。（机制；边界待 SA owner 会签）

**四、多语言开票抬头。** `invoiceTitle` 保留为主抬头；`NO_INVOICING_DETAILS` 的判据不变——仍只看主抬头有没有。新增可选 `invoiceTitles`：语言标签（BCP 47，如 `zh-Hans`、`en`、`zh-Hant`）→ 抬头，以及可选 `invoiceTitleLanguage` 标主抬头的语言。同一份开票资料里语言不得重复，主抬头的语言也算在内，比对不区分大小写、原样保存；语言标签只锁 BCP 47 的形状，不查注册表。语言版本挂在主抬头上：没有主抬头就没有开票资料，这两格也无处可挂，给了即拒。哪种语言印到单据上由开单方按收件方决定，不在本记录。（机制）

**五、快照形状与兼容。** 沿 ADR-0267 最低可表达版本：不带收款账户、语言版本与主抬头语言的快照沿原有形状（无标记、LEP-2、LEP-3），字节与摘要一字不变；带任一格写 `shape: LEP-4`，摘要为 `LEP-4:<sha256>`。LEP-4 必须含三格之一；LEP-3 以下不得含它们；未知形状拒读；旧行不回写。迁移只增三个可空镜像列（收款账户、语言版本、主抬头语言），在有数据的库上可施加，既有行三列皆 NULL 即缺席。发布读取构建必须认识 LEP-4，开发演示服务在停止旧进程后统一换构建，回滚须用认识 LEP-4 的构建。

**六、三类。** 决定一、三、四、五的机制与缺席语义是**机制**；决定二的参考表与「主抬头判开票资料齐」是**产品策略**；具体账户、具体抬头、租户采用哪些类型是**租户取值**，留空、拒绝默认，演示用 `SYN-`、证据只记 `S`。

## 候选与反方

- **海关登记号另开一组。** 否决：判断方式与税务登记号完全相同（目录判类型与格式、按资料生效时点判在用），另开一组就是同一机制两份，管理台与开立方都要各接一次；一个号属税务还是海关由目录里的类型说，不必在资料上再分格。
- **改线上名为 `regulatoryRegistrationNumbers`。** 否决：已在册的快照、两个入口的载荷与管理台都要换字，旧名还得留兼容路；换来的只是名字准确，判断一处不改。名窄于义的代价是读的人要看一眼注释，比一次全链路改名小。
- **收款账户归 settlement-accounting。** 否决：它是法人自己的资料，随法人的开票资料一起变、一起按时点固定在单据上；放进 SA 会让一笔结算账户（业务账户）与一个银行账户纠缠成一个对象，而一个结算账户可以收到不同的银行账户、一个银行账户也可以服务多个结算账户。
- **收款账户另立一册、不随资料修订。** 否决：对账单固定的是资料修订引用，账户若另册可改，单据印过的账户就会被事后改写；随修订保存正是 ADR-0145 决定五要的。
- **只做单抬头、多语言由开单方自己拼。** 否决：开单方拼不出法人没登记过的另一种语言的法定名称；抬头是法人资料，归 PC 登记、归开单方选用。
- **多语言抬头替代主抬头（任一语言算开票资料齐）。** 否决：`资料不全`的判据会随「哪种语言算数」变，开立方今天的判据一动，已固定的解析记录就对不上；主抬头一格不变，多语言是它之上的可选明细。
- **币种只收内置目录、在领域构造门拦。** 否决：构造门同时是快照重建门，目录随发版变，ISO 废止一个码之后已在册的行会读不回（ADR-0345 决定六的同一理由）；目录门留在写入边界。
- **参考表给没有公开格式的类型一条宽松正则。** 否决：目录机制要求每个类型带格式，而 `.+` 一类的正则是替公开规范编了一个格式，读参考表的人会把它当真；不随附并写明缺了什么，租户要用自己登，才是「留空不猜」在现行机制下能做到的样子。目录格式要不要放宽为可选，见越权风险点 1。

## 对已接受记录的作用

- [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一「目录的每个类型标明它属身份层（终身注册号）还是资料层（税务登记号），两层各只收自己那一类。」一句：资料层由本记录决定一放宽为监管登记号（税务、海关等监管机关发放的登记号），两层各只收自己那一类照旧；决定三「内容为注册地址、税务登记号（可后办、可变的那类，如 GST / VAT 登记号）、开票资料与联系人。」一句（已由 ADR-0350 扩展）里「税务登记号」按本记录决定一读作监管登记号，「开票资料首版只含开票抬头（开票用的法人名称，可以是与参与方名称不同的语言版本）；其余开票项随开立方的实施票按需增列，不预设。」一句由本记录决定四扩成含主抬头语言与其余语言版本；越权风险点 5 由本记录决定三、四回答。其余各条不变。
- [ADR-0350](./0350-legal-entity-profile-representative-addresses-and-contact-purposes.md)「有扩展内容的快照写 `shape: LEP-2`，内容摘要为 `LEP-2:<sha256>`，不与旧版混比。」一句：含收款账户、语言版本或主抬头语言时按本记录决定五的 LEP-4（含社交账号的场景已由 ADR-0351 停用）；其余各条不变。
- [ADR-0351](./0351-contact-social-accounts-are-optional-versioned-profile-details.md)「快照按最低可表达版本写入：没有社交账号的资料沿原有无标记形状或 LEP-2，字节和摘要保持；至少一个联系人有社交账号时写 `shape: LEP-3`，摘要为 `LEP-3:<sha256>`。」一句：同时含本记录三格之一时写 LEP-4，不写 LEP-3；「LEP-3 必须包含社交账号；无标记或 LEP-2 不得包含它」照旧成立，其余各条不变。
- [ADR-0147](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md) 决定六「首版随附哪些国家与地区、要不要带校验位算法，归 PC owner 与用户」：本记录决定二定了随附范围（代决），校验位算法仍不带；无条款停用。

## Consequences

- party-commercial `CONTEXT.md` Language「责任法人」「法人资料」与 Rules「法人资料」诸句、GLOSSARY「法人资料」、UC-PC-001「法人资料维护补充」与 `AT-PC-078` 按本记录修订。
- 领域（`LegalEntityBankAccount`、`LocalizedInvoiceTitle`、`InvoicingDetails` 的语言格）、端口行、postgres 快照与 0044 迁移、在线口两个读口与写口、受控 CLI 同步扩形；旧载荷照收，旧行读作缺席。
- 参考配置包多出 CN@2、SG@2、HK、US、GB、JP 与欧盟各成员国的文件与发布清单行；演示租户经采用路径登 CN 的纳税人识别号与海关注册编码、SG 的 GST 登记号、HK 的 BR No.、US 的 EIN、DE 的 VAT / IOSS / EORI，并给五家演示法人各追加一版带监管登记号、双语抬头与 `SYN-` 收款账户的资料修订。
- 管理台的表单、读出、差异预览与列表归票 company-subject-ux/06；SA 的对账单 / 发票选账户、选抬头归 SA 的开立票。
- 本记录不改 `settlement-accounting` 的任何文档与代码。

## 越权风险点

1. **目录机制要求每个类型带格式正则**（`RegistrationNumberTypeSpec` 的格式必填，0033 的格式列非空），「其余留空不猜」在目录里表达不了；本记录按「无明确公开格式的类型不随附」处理，AEO 认证编号、美国州注册号、欧盟各成员国商业登记号因此不在参考表里。要不要把目录格式放宽为可选（另票、动目录的领域与迁移），归 PC owner 与协调方。
2. **参考表各国格式取自本记录对公开规范的认识**，没有逐国核对一手原文：VIES 各成员国 VAT 结构、EU EORI 与 IOSS 的通用结构、HK CR No. 不超过 7 位、GB 公司编号与 VAT / EORI、JP 两类号、CN 纳税人识别号 15 / 18 位、海关注册编码 10 位。任一条错了，更正走新版本（已发布文件不改）；采用了错版本的租户要再登一笔修订换版本。归 PC owner 复核。
3. **参考表不含欧盟各成员国与美国的身份层类型**，这些国家的法人要先由租户登自己的终身注册号类型才能登记；参考表对它们只覆盖资料层。
4. **收款账户与 SA 结算账户的边界**只在本记录与 CONTEXT 里写了一句，SA 的 CONTEXT 与开立用例没动；开单方按修订引用取账户、按单据币种选账户的落法归 SA owner 的票，边界待其会签。
5. **（币种，账号）按去空白后的原文比对**，IBAN 带空格与不带空格会被当成两个账号；语言标签只锁形状，`zh-CN` 与 `zh-Hans` 是两个语言。是否要规范化归 PC owner。
6. **币种目录门落在两个写入口**（在线口答 400、受控 CLI 译装拒绝），与 ADR-0345 决定五列出的币种入口同一口径；该决定没点名法人资料，本记录把它算作「新登记入口」，归产品 owner 复核。

## Links

- [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md)：身份层与资料层的切分，本记录放宽其资料层登记号、扩其开票资料、回答其越权风险点 5
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类，本记录决定六的判据
- [ADR-0147](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：参考配置的存放、版本与采用路径，本记录决定二照它出参考表
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：开单方取收款账户与抬头走消费侧适配器
- [ADR-0267](./0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md)：最低可表达版本，本记录决定五的 LEP-4 照它
- [ADR-0345](./0345-tenant-enablement-of-countries-and-currencies-is-a-tenant-value-registered-in-party-commercial-and-new-write-entries-accept-only-catalogue-codes.md)：币种目录门只在写入边界
- [ADR-0350](./0350-legal-entity-profile-representative-addresses-and-contact-purposes.md)、[ADR-0351](./0351-contact-social-accounts-are-optional-versioned-profile-details.md)：可选明细随修订保存与快照形状的先例，本记录部分停用其快照形状句
- [参与方与商业 CONTEXT](../domain/party-commercial/CONTEXT.md)「法人资料」
- [GLOSSARY](../domain/GLOSSARY.md)「法人资料」「结算账户」
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)「法人资料维护补充」与 `AT-PC-078`
- 票 [company-subject-ux/05](../../.scratch/company-subject-ux/issues/05-cross-border-profile-extensions.md)
