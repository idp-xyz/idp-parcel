# 参与方与商业（`party-commercial`）

代码在 `internal/partycommercial`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、解析结果、发布与登记结果、入口四节是从代码里读出来的。「规则」与「生命周期」两节的句子是从旧 `docs/domain/party-commercial/CONTEXT.md` 提炼的，每条只用 grep 确认了代码里有对应的机制（类型、函数、错误或结果码），没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在最后一节。

## 管什么，不管什么

管：参与方身份（业务参与方、责任法人、货主客户账户）及其资料，参与方关系，注册号类型目录，启用范围登记册；服务产品、客户合同、供应商商业协议、接单规则包、各类政策与规则的商业版本及其发布、收尾；产品—渠道映射，渠道账号使用授权；商业依据解析与商业授权裁定。

不管：

- 可执行价卡、评价与金额——`parcel-pricing`。本上下文只有商业价格政策和定价方案绑定。
- 客户委托、接受或拒绝决定、面单交易、具体渠道选择、关闭与重开的具体决定——`parcel-shipment`。它保存所引用的商业快照，不能改商业版本。
- 实际运输委托、履约事实、收费发生项的登记——`transport-fulfillment`。
- 费用、余额、冻结、对账、核销、结算账户——`settlement-accounting`。
- 追踪、异常案件、通知、索赔结论——`visibility-exception`。
- 关务判断与监管结果——`customs-compliance`。
- 法定发票开立与真实收付——财务、税务与银行系统。
- 渠道账号凭据与接入的技术实现。凭据不等于业务授权。

## 词

| 词 | 意思 |
|---|---|
| 运营集团租户 | 最高的配置与数据隔离边界。不是责任法人、客户账户或登录组织 |
| 责任法人 | 租户内对合同、服务、结算承担责任的法人。身份上带注册国家 / 地区与终身注册号 |
| 法人资料、参与方资料 | 责任法人或业务参与方在一个生效区间内开票、签约、通知所用的资料。按生效时点成修订，不是身份；参与方资料不登法定代表人 |
| 业务参与方 | 参与商业或履约关系的企业、组织或个人。身份稳定，角色随关系变 |
| 货主客户账户 | 面向一个货主客户的业务隔离与商业关系边界。不是法人，不是登录用户 |
| 参与方关系 | 两个参与方在角色、范围、依据和有效期间内成立的时态关系。授权交付关系是其中一种：交付方可以替某个客户账户向节点送站 |
| 商业版本 | 一次受控发布形成的不可覆盖业务定义，带对象类别、适用范围、有效区间、来源与批准依据 |
| 待批准发布 | 操作者面录入的一版商业版本在发布前的载体。不是商业版本，任何解析读不到它 |
| 商业选择锚点 | 选出规则包之前用来解析的业务时点，由独立的版本化选择政策产生 |
| 商业依据解析 | 按租户、客户账户、责任法人候选、范围和锚点，答唯一适用的版本集合或明确的非成功结果。成功时带当前修订标识（`AuthorityViewRevision`） |
| 服务产品 | 运营企业对货主销售的服务。形态有网络服务产品与面单渠道服务 |
| 产品—渠道映射 | 服务产品版本与渠道产品（渠道服务方提供的可采购服务）之间带有效期的候选关系。不代表已经选了哪一个 |
| 客户合同版本 | 责任法人与货主客户账户在一个适用期内接受的商务与服务条件 |
| 合同委派 | 合同版本声明：委派方把某一授权动作的实际决定权委派给持某一商业权限等级的运营角色 |
| 客户服务规则版本 | 追踪披露、异常响应、通知义务、索赔期限、最低材料等可复用的服务规则 |
| 供应商商业协议版本 | 责任法人与供应商接受的采购条件。不是一次运输委托，不是账单 |
| 商业价格政策版本 | 销售、采购或内部方向的价格政策，旧简称「价格规则版本」，代码里是 `PRICE_RULE`。正文里的定价方案绑定说它授权哪份定价方案版本 |
| 结算政策 | 结算币种、预付或账期、付款条件等版本化约定。不拥有余额与对账单 |
| 接单规则包 | 一份新委托要采用哪些规则、何时人工复核、谁能主动拒绝、各判断时点怎么取的版本化依据 |
| 资料修订允许声明 | 规则包版本对「接受后客户原始资料能不能改」的声明：是否封闭，加若干格允许或不允许 |
| 面单服务终局规则 | 规则包版本对面单渠道服务终局边界的声明，可带一条面单有效期 |
| 接受前财务控制策略 | 接单前要执行哪些财务控制、按什么顺序、失败去哪。不拥有估价、冻结与信用结果 |
| 渠道账号使用授权 | 账号持有人允许运营企业在明确范围与期限内使用该账号 |
| 启用范围登记册 | 某租户启用或停用哪些国家 / 地区与币种。码取自产品内置目录 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 商业对象类别 | `SERVICE_PRODUCT` `CUSTOMER_CONTRACT` `SUPPLIER_AGREEMENT` `ACCEPTANCE_RULE_PACKAGE` `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` `PRICE_RULE` `SETTLEMENT_POLICY` `CREDIT_POLICY` `AUTHORIZATION_RULE` `CUSTOMER_SERVICE_RULE` |
| 商业版本状态 | `DRAFT` `PUBLISHED` `EFFECTIVE` `EXPIRED` `RETIRED` `SUPERSEDED` |
| 待批准发布状态 | `PENDING_APPROVAL` `APPROVED` `PUBLISHED` |
| 服务产品形态 | `NETWORK_SERVICE` `LABEL_CHANNEL_SERVICE` |
| 参与方身份状态 | `REGISTERED` `EFFECTIVE` `DEACTIVATED` |
| 参与方角色 | `CUSTOMER` `SUPPLIER` `CARRIER_AGENT` `RESELLER` `ACCOUNT_HOLDER` `AUTHORIZED_DELIVERING_PARTY` |
| 参与方类别（读出，不是字段） | 各角色，另加 `OUR_COMPANY` `NO_ROLE` |
| 关系状态 | `CANDIDATE` `EFFECTIVE` `EXPIRED` `REVOKED` `SUPERSEDED` |
| 注册号层 | `IDENTITY` `PROFILE` |
| 启用范围种类 / 状态 | `COUNTRY` `CURRENCY` / `ENABLED` `DISABLED` |
| 解析目的 | `ACCEPTANCE_CONTROL` `PRICING` |
| 价格方向 | `BUY` `SELL` `INTERNAL` |
| 绑定换算 | `NONE` `FROZEN_BUY_EVALUATION` |
| 税务处置 | `TAX_INCLUSIVE` `TAX_EXCLUSIVE` `TAX_NOT_APPLICABLE` |
| 结算方式 | `PREPAID` `TERMS` |
| 信用比例基数 | `POSTED_BALANCE` `PRIOR_PERIOD_CONFIRMED_CHARGES` |
| 接受前控制种类 | `PREPAID_FREEZE` `CREDIT_CHECK` |
| 控制失败处置 / 共同通过条件 | `REJECT` `AUTHORIZED_DISPOSITION` / `ALL_CONTROLS_PASS` |
| 授权动作 | `MANUAL_REVIEW` `ACTIVE_REJECTION` `SOURCE_DATA_AMENDMENT` `CONTROLLED_CLOSURE` `REOPENING` |
| 资料修订阶段 | `ACCEPTED_NOT_YET_RECEIVED` `RECEIVED_OR_MEASURED` `LABELLED_OR_BAGGED` `CUSTOMS_DATA_FORMING_NOT_SUBMITTED` `CUSTOMS_SUBMITTED` `CASE_CLOSED_OR_SERVICE_COMPLETED` |
| 修订意图 / 允许性 | `SUPPLEMENT` `CORRECTION` `EXPLICIT_CLEAR` / `NOT_DECLARED` `ALLOWED` `DISALLOWED` |
| 可声明终局的责任结果 | `EFFECTIVE_DELIVERY` `RETURN_COMPLETED` `SERVICE_TERMINATED` `REGULATORY_DISPOSITION` `LABEL_SERVICE_COMPLETED` `LABEL_SERVICE_FAILED` |
| 收费原因 / 数量形 | `BOOKING` `CANCELLATION` `FAILED_ATTEMPT` `ACTUAL_FULFILLMENT` / `PER_SEGMENT` `PER_PIECE` `PER_ATTEMPT` `PER_BOOKING` |
| 渠道账号授权状态 | `PUBLISHED` `REVOKED`；技术可用与业务授权是分开的两格 |

## 解析的七种结果

| 结果 | 什么时候 | 之后 |
|---|---|---|
| `UNIQUELY_RESOLVED` 唯一解析 | 每种必需依据恰一个候选 | 交回解析标识、采用版本与当前修订标识 |
| `NO_APPLICABLE_BASIS` 无适用依据 | 权威读到了，零个候选 | 等发布或登记；不是拒绝 |
| `APPLICABILITY_CONFLICT` 适用冲突 | 多个候选 | 由商业责任方收尾或裁决，系统不挑 |
| `RESOLUTION_PENDING` 解析未决 | 权威没读到，或指名引用、绑定方案确认不了 | 重试同一次调用 |
| `INPUT_NOT_ACCEPTED` 输入未受理 | 身份或标识立不住 | 改输入；在任何查询之前短路 |
| `STALE` 已失效 | 回指的解析与当前视图不再相容 | 回第一阶段重新解析 |
| `BASIS_NOT_RESOLVED` 依据未解析 | 回指的标识不指向调用方可用的解析 | 回第一阶段重新解析；不分「从未签发」与「属于别的客户账户」 |

未成功时的原因码：`ANCHOR_POLICY_NOT_CONFIGURED` `AUTHORITY_UNREADABLE` `CURRENT_RESOLUTION_CHANGED` `NAMED_REFERENCE_NOT_CONFIRMED` `BOUND_PLAN_NOT_CONFIRMED` `BOUND_PLAN_WITHDRAWN`。

其他结果码：

- 受控发布逐项：`PUBLISHED_EFFECTIVE` `PLANNED_EFFECTIVE` `REPLAYED` `CONTENT_CONFLICT` `PENDING` `NOT_ACCEPTED`。
- 各登记口：`REGISTERED` `ALREADY_REGISTERED` `CONTENT_CONFLICT` `NOT_ACCEPTED`，身份册另有 `DEACTIVATED` `NOT_FOUND` `ENDED`，渠道账号另有 `BUSINESS_UNAUTHORIZED`。
- 待批准发布：录入 `DRAFT_SUBMITTED` `DRAFT_REPLAYED` `DRAFT_REVISED` `CONTENT_FIXED`；批准 `DRAFT_APPROVED` `NOT_CONFIGURED` `NEEDS_ANOTHER_APPROVER` `APPROVER_NOT_QUALIFIED` `DRAFT_CHANGED` `DRAFT_NOT_READABLE`；发布 `DRAFT_PUBLISHED` `DRAFT_NOT_APPROVED` `DRAFT_AWAITS_EFFECTIVE_START` `PUBLICATION_NOT_LANDED`。
- 法人资料解析：`RESOLVED` `PROFILE_INCOMPLETE` `LEGAL_ENTITY_NOT_REGISTERED` `LEGAL_ENTITY_NOT_EFFECTIVE` `LEGAL_ENTITY_DEACTIVATED`。

## 规则

**身份与资料**

- 身份登记按修订版本化，不可覆盖。登记与停用都带显式依据；更正与停用都是新修订。停用只自其时点起不再支持新的商业决定，不删除身份。
- 责任法人必须关联一个业务参与方，货主客户账户必须关联其客户参与方，且同租户。
- 责任法人登记必须带注册国家 / 地区与至少一个终身注册号。号的类型与格式取自注册号类型目录；目录里没有该国家 / 地区的类型就拒登，不套默认格式。
- 终身注册号不作变更。录错按更正登记并带身份更正依据；业务参与方第一次补登不用带。
- 租户内一个终身注册号只指一个在用身份，业务参与方与责任法人一并判；撞上即拒并点名持有方。
- 被责任法人钉着的业务参与方不登注册号，也不登参与方资料；我方公司的这些只登在责任法人一侧。
- 资料按生效时点成不可覆盖的修订，可以登未来生效的。注册地址的国家 / 地区须与身份上的一致。
- 开单时按开立时点解析当时有效的法人资料修订并固定引用。没有有效的开票资料答资料不全，不以默认值补齐。

**关系与授权**

- 参与方关系必须有双方、角色、适用范围、依据和有效区间。到期、撤销或替代只影响此后的新决定。
- 已结束的关系不再收新修订；要恢复就登一段新关系。替代只换持有方，并在同一笔里登记接替关系。
- 渠道账号使用授权必须显式、可撤销。技术上能访问账号不等于业务上获得授权。
- 人工复核、主动拒绝、资料修订、受控关闭、重开是五个分开的授权动作，各按责任法人、等级、范围和有效期间授予，互不蕴含。
- 运营角色代客户请求资料修订时，实际决定方由合同委派解出；没有委派即无权，登录操作人不能顶替。
- 同一请求同一刻被多条授予允许，或被委派方不同的多条委派命中，答适用冲突。
- 授予随所挂授权规则版本、委派随所挂合同版本的收尾离开新的裁定。某个范围没有现行授权规则，答未配置，不答拒绝。

**商业版本与发布**

- 发布后的正文不可覆盖，变化形成新版本。同键同内容重发返回原结果，同键不同内容是内容冲突，绝不覆盖。
- 内容摘要由本上下文按对象类别算出，只在同一规范化版本内可比。批文自报的摘要与算出的不等，那一项未受理。
- 批准是与录入分立的操作者动作，两者身份都来自操作者信封。须否为不同主体、须持哪一等级，由租户的审批职责规则定；规则未登记则批准不放行。
- 批准与发布都绑定调用方看到的那份载体：内容摘要或版本壳摘要变了就答已变更，不批不发。
- 新版发布不自动收尾旧版，两版重叠也不阻发布；重叠处解析答适用冲突，直到显式收尾。
- 收尾两种：后继晚于旧版起点，旧版截止在后继起点；同起点重发，旧版退役。都要指名后继，且后继接得住旧版此后的那一段。收尾不改写既有快照，不改指既有委托。

**解析**

- 每种必需依据必须唯一适用。零个、多个、读不到分别是三种非成功结果，不任选、不默认通过。
- 解析分两阶段：先按独立的商业选择锚点选出客户、法人、产品、合同与规则包；再由选出的规则包为各下游判断声明时点策略。待选规则包不得决定自己的选择时点。
- 委托声明了服务产品身份时，候选按它收窄；没声明而同一范围有多个产品，仍是适用冲突，不补默认产品。

**产品、合同与规则**

- 服务产品、渠道产品、产品—渠道映射、客户合同、供应商协议是不同对象，各自版本化，互不自动修改。
- 产品—渠道映射按修订登记，带依据并指向已发布的服务产品版本。渠道绑定可以显式登记为「未配置」，不提供任何候选。
- 合同版本必须关联货主客户账户。接单规则包随服务产品选，合同可以不指名；指名了，版本壳与正文要一致。
- 合同按费用范围约定接受前财务控制：要么指名一份策略，要么显式不适用并留依据。没约定的范围不读作允许通过。
- 接受前财务控制策略由控制行组成，每行有种类、范围、判断顺序与失败处置；不是三选一枚举。
- 资料修订允许声明：没登的格，未封闭读作未声明，已封闭读作不允许。本上下文不内置任何一格。
- 交付条件挂在服务产品版本或客户合同版本上。合同层必须指名所收紧的产品版本，方式集合只能是产品层的子集。消费方按委托接受时固定的解析回指读它，不取当下有效的版本。
- 客户服务规则同范围有产品层与合同层时只采纳一版，合同层优先。合同层没写的行继承产品层那一版，底座在接受时一并固定。
- 供应商协议的收费发生条件每个原因至多一条。「声明不计费」与「没声明」是两回事。

**价格、结算、信用**

- 三个价格方向的政策分别表达。政策必须显式绑定方向与定价方案版本；方向与方案不一致时要声明换算，目前只有引用已冻结的采购评价一种。
- 计价目的的解析按绑定判：方案确认不了是解析未决，已退役也是，都不用默认价。
- 结算方式按责任法人、相对方、合同版本、费用范围、币种、有效期间六维唯一解析。同一范围同时命中预付与账期是冲突。相对方指业务参与方，不是货主客户账户。
- 信用政策按责任法人、等级、费用类型成版本；比例额度必须声明基数。
- 启用范围登记册只追加，修订连续，生效时点不早于前一修订；码只能取自产品内置目录。没登记过的码是未配置，不折成启用或停用。

## 生命周期

- **参与方身份**：已登记 → 已生效 → 已停用。未生效的登记也可以停用，那是撤下，不是删除。
- **法人资料、参与方资料**：修订生效后取代前一修订；所属身份停用后不再收新修订。
- **参与方关系**：候选 → 已生效 → 已到期、已撤销或已替代。
- **商业版本**：草稿 → 已发布 → 到生效边界即已生效 → 已到期或已退役。被截止只收窄可选区间，不是状态迁移。`SUPERSEDED` 一格在集合里，换版目前走截止或同起点退役。
- **待批准发布**：待批准 → 已批准 → 已发布。待批准的可以按同一版本重新录入替换；已批准的内容已固定，要改就换版本号。
- **产品—渠道映射、启用范围、注册号类型**：只增修订，旧修订保留。**渠道账号使用授权**：建立 → 撤销或自然到期；换范围是另一笔授权，不是第二版。

## 入口

在线口（处理器在 `adapters/http`，路径在 `cmd/parcel-api` 里接线）：

| 做什么 | 路径 |
|---|---|
| 身份登记与停用 | `/commercial-business-party-registrations` `/commercial-legal-entity-registrations` `/commercial-customer-account-registrations` `/commercial-party-identity-deactivations` |
| 资料登记 | `/commercial-business-party-profile-registrations` `/commercial-legal-entity-profile-registrations` |
| 关系登记与结束 | `/commercial-party-relationship-registrations` `/commercial-party-relationship-expirations` `/commercial-party-relationship-revocations` `/commercial-party-relationship-supersessions` |
| 身份与关系读口 | `/commercial-business-parties`（及 `/{partyId}/revisions` `/{partyId}/profile-revisions`） `/commercial-group-legal-entities`（及 `/{legalEntityId}/revisions` `/profile-revisions` `/profile-resolution`） `/commercial-customer-accounts` `/commercial-party-relationships` |
| 注册号类型目录 | `/commercial-registration-number-types` `/commercial-registration-number-type-registrations` `/commercial-registration-number-type-deactivations` |
| 启用范围 | `/reference-data/enablement` `/reference-data/enablement-registrations` |
| 产品与渠道 | `/commercial-service-products` `/commercial-service-product-form-registrations` `/commercial-product-channel-mappings`（及 `/{mappingId}/revisions`） `/commercial-product-channel-mapping-registrations` `/commercial-channel-account-use-registrations` `/commercial-channel-account-use-revocations` |
| 待批准发布 | `/commercial-publication-vocabularies` `/commercial-publication-previews` `/commercial-publication-drafts` `/commercial-open-publication-drafts` `/commercial-publication-draft-readbacks/{kind}/{objectId}/{version}` `/commercial-publication-draft-approvals` `/commercial-publication-draft-publications` |
| 受控发布与收尾 | `/commercial-publications` `/commercial-version-successor-cuts` `/commercial-version-retirements` |
| 商业版本读口 | `/commercial-version-readbacks/{kind}/{objectId}/{version}` `/commercial-customer-contracts` `/commercial-supplier-agreements` `/commercial-policies` |

离线口：`cmd/parcel-commercial`，运营在数据库网络内手跑，不监听端口，内容全部来自 `-input` 文件。子命令：`publish` `register-resolution-key` `register-parties` `deactivate-party-identity` `end-party-relationships` `register-products` `register-registration-number-types` `register-legal-entity-profiles` `register-version-closures` `register-approval-duty-rule`。退出码 0 全部落定，1 技术失败，2 有未决或冲突要商业责任方看。 商业依据解析、授权裁定、交付条件读取没有 HTTP 口，由其他上下文在进程内调用。

## 旧文档写了、代码里还没有的

- **销售价格约定与销售价格来源**：旧文档说客户合同版本声明一份有序的销售价格来源列表，或显式声明不计销售价。搜 `SellPrice`、`PriceSource`、`销售价格` 无一命中；`CustomerContract` 正文只有客户账户、规则包引用与财务控制约定。实现在做，未进 main。
- **保价条件**：旧文档写成服务产品与客户合同上的现行条件。搜 `Insur`、`保价`、`Deductible`、`免赔` 无一命中。
- **仓配代办授权动作**：授权动作封闭集只有五个，没有这一格；搜 `Warehouse`、`仓配` 无一命中。它属已搁置的仓配履约。
- **人工费用调整授权**：搜 `Adjustment`、`人工调整` 无一命中。信用政策正文只有信用额度。
- **客户服务规则合同层「只能收紧」**：继承是有的，没找到比较合同层与产品层孰严的校验（在 `customer_service_rule.go` 与相关适配器里搜 `收紧`、`Widened`）。
- **指定渠道产品的锁定与显式渠道变更授权**：本上下文只有渠道约束的收窄（`CandidatesAllowedBy`），该类型明说不提供锁定；搜 `锁定`、`Lock` 没有别的机制。锁定结果按旧文档本就归拥有交易的上下文，这里没核。
- **监管、安全暂停与硬限制的单独记录**：搜 `Suspension`、`Restriction`、`暂停`、`硬限制` 无一命中。
- **结算政策「明确允许哪些合同版本归入同一结算账户」**：代码里一份政策只指名一个合同版本（六维之一），没有允许清单。
- 旧文档里关于面单服务默认终局、关闭与重开时重新校验当前授权的句子，说的是 `parcel-shipment` 怎么用这些声明。本上下文只登声明，那些行为没在这里核。
