# 评审 #7「最小产品版本正文及持久化」设计输入包

Category: chore（只读备料）
Status: 取证快照，不随实施改写。落地清点见 [design.md §8](./design.md)（取证 `1fff679`）。

承 MCP-1 派包（task-b7bd03b6）。**取证于 HEAD `2fbe405`**；当时树上另有 MCP-2 在途未提交内容
（`internal/partycommercial/ports/ports.go` 修改、`internal/partycommercial/domain/pre_acceptance_control.go`、
`migrations/party_commercial/0007_pre_acceptance_control_declaration.sql`）。0007 收口时已入 `861280b`；
本文件其余断言仍是 `2fbe405` 上的代码事实，不是 `1fff679` 现状。
本包只报事实与引文，不提方案。

**一处对派包票面的更正**：票面写「涉及 ServiceProductForm 的 0053/0049 相关句」，但 `ServiceProductForm`
的裁决实际在 [ADR-0050](../../docs/adr/0050-service-product-form-observable-through-the-resolution-closure.md)
（0053 是网络事实族、0049 是发布通道，两份正文均无该词）。本包三份都摘：0050 全采，0053/0049 只摘与「登记什么、
怎么装载、机制半边」相关的句子。

另注：评审基准 `d40b03a` 至本包取证点已有 19 笔提交，评审 #1（`64bae12`/`7e22f27` 派发接线）、#4（`c1866c2`
迁移 0004 放宽九类）、#5（`38b9a67`/`449f9bf`/`7147465`）、#6（`1d0c036`）已各有对应治笔；#7 未治，本包即其备料。

---

## 一、硬句引文

### 1.1 评审 #7 原文（docs/review/081701.md，该文件本身未提交）

> **物流产品仍未形成完整可执行配置。**
> 当前 ServiceProduct 只有商业版本和 `NETWORK_SERVICE` 形态。服务范围、准入、主备线路、SLA、清关、轨迹、
> 赔付、退件、容量和质量规则虽然在不同上下文已有机制，但尚未形成可发布、可持久化、可完整加载的产品版本组合。
> CommercialAuthority 也明确承认价格、结算等政策持久化尚未闭合。

评审「下一步」的排序原文：「先修数据库约束、索赔并发和事件校验，再完成**最小产品版本正文及持久化**，
然后接通 Dispatcher、核心消费者和最小业务 API，最终用一个 `SYN-*` 产品、客户、线路和价卡跑通……纵向闭环。」

### 1.2 UC-PC-001（维护并发布商业权威依据）

关于产品版本/形态/发布的硬句：

- 目标与边界：「到 `party-commercial` 建立稳定身份/关系、**发布不可覆盖商业版本**、明确拒绝或冲突，或者保存本次尚未形成权威结果为止。」
- 「它不是通用后台 CRUD。每种商业对象保持独立身份、版本和批准责任；一次导入或发布批次只负责处理归组，**不能把客户、合同、产品、价格和规则合并为一条可覆盖配置**。」
- 包含清单：「分别校验并发布服务产品、客户合同、供应商协议、接单规则包、财务控制策略、商业价格政策、定价方案绑定、结算、信用和授权规则版本。」「校验跨对象引用闭合、有效区间、唯一适用性及替代/退役关系。」
- 发布单位表·服务产品版本行：「必须独立保存：产品身份、**服务形态**、责任、适用范围、版本和生效区间｜新版本不自动修改客户合同。」
- 发布单位表·其他规则或政策版本行：「类型、正文、适用键、有效区间、依赖、批准和替代关系｜接单、财务控制、结算、信用和授权规则保持独立。」
- 结果语义·已发布：「不可覆盖版本已经获准供声明范围的新选择使用｜禁止：原地编辑正文或追溯改写历史。」
- 一致性节：「发布批次不是聚合。合同、产品和规则可以在一次操作中提交，但各自拥有独立结果；引用尚未发布时，依赖对象保持未决而不是建立悬空生产引用。」
- 给开发的交接：「实现应按业务对象提供语义化命令与 Repository，**不建立一个可以修改任意商业表的通用配置接口**。首个生产持久化仍受 `PN02-W01/W03` 真实证据和技术门槛阻断；可以先实现纯领域版本、不变量、冲突分类和命名夹具，不得预置真实商业默认值。」

### 1.3 UC-PC-002（按范围与时点解析商业依据）——装载/返回面的硬句

- 「返回解析标识、判断时间、选择锚点、对象版本、有效区间、当前修订标识和**依赖闭包**。」
- 首发试点叠加条件：「真实证据未完成前，**只允许实现解析类型、两阶段端口、命名夹具和负向测试；不得返回生产“唯一已解析”**。」
- 给开发的交接：「消费者端口必须返回结构化的唯一成功、无适用依据、适用冲突、解析未决、输入未受理、依据未解析和已失效结果，不能只返回对象或通用错误。」「当前实现闸门未解除前不得建立伪造的生产商业数据适配器。」

### 1.4 PC CONTEXT.md 相关词条原文（docs/domain/party-commercial/CONTEXT.md）

- **商业版本**：「服务产品、合同、协议、规则或政策在一次受控发布中形成的不可覆盖业务定义。商业版本具有稳定身份、对象类型、适用范围、有效区间、来源与批准依据，以及与前后版本的关系；发布后的正文不原地修改。」
- **服务产品**：「运营企业面向货主客户定义和销售的国际小包服务。……不等同于外部渠道提供的产品。」
- **网络服务产品**：「由运营企业组织自己的集货、节点、干线、关务和尾程网络履约的服务产品形态。……不建立独立于服务产品的第三套产品目录。」
- **服务产品版本**：「服务产品在一次明确发布中形成、具有独立身份和适用范围的商业定义版本。」
- **客户服务规则版本**：「服务产品或客户合同在明确期间内采用的追踪披露、异常响应、客户更新、通知义务、索赔期限和最低材料等服务规则。它表达可复用的商业责任和差异化条件，不拥有具体追踪投影、异常信号、案件、通知或索赔结论。」（评审 #7 点名的「轨迹、赔付」商业半边归此词条）
- 规则不变量：「服务产品、服务产品版本、渠道产品、产品—渠道映射和客户合同版本是不同业务对象，**不能使用一个“产品配置”同时替代其身份与生命周期**。」
- 规则不变量：「服务产品、客户合同、供应商协议、接单规则包、接受前财务控制策略、价格规则、结算政策、信用政策和授权规则都遵循**商业版本共同不变量**：草稿可修订，发布后正文不可覆盖；变化形成新版本；退役、到期、替代或有效性更正保留历史关系。」
- 生命周期·商业规则与政策版本：「本生命周期共同适用于接单规则包、接受前财务控制策略、商业价格政策、定价方案绑定、结算政策、信用政策和授权规则；**各对象仍保持独立身份，不因此合并为一份“大配置”**。可执行价卡版本的校验、发布和评价生命周期由 `parcel-pricing` 管理。」
- 所有权：「`network-routing` 只消费已经确定的服务要求和适用约束形成路由计划，不拥有服务产品—渠道产品的商业映射。」（评审 #7 点名的「主备线路」运营半边不在 PC——PC 只有产品—渠道映射与渠道约束）

### 1.5 ADR 已裁口径

- [ADR-0034](../../docs/adr/0034-pricing-closure-adopts-via-price-policy.md)（价格政策）：「计价目的下的价格规则走政策解析」「成功结果必须可观察方向与定价方案绑定……不得再从『只有版本』反推」「`CommercialRegistry` 可登记 `CommercialPricePolicy`；只登记版本、不登记政策时，计价目的下的价格规则会落到`无适用依据`——这是有意的」。否决项：「让 `CommercialVersion` 自己带方向与方案引用——与既有『方向与绑定在政策上、不在版本上』的分工冲突」。
- [ADR-0044](../../docs/adr/0044-settlement-basis-adopts-via-settlement-policy.md)（结算政策）:「键上新增 `SettlementSelector`（相对方、合同版本、费用范围、币种）」「成功路径经 `ResolveSettlementPolicy` 采用……零候选→`无适用依据`，同一精确范围多候选→`适用冲突`；不同费用范围互不冲突」「结果携带政策……交回方式（PREPAID/TERMS）与完整 `SettlementApplicability`；结算政策参与 `ViewRevision` 派生」。
- [ADR-0038](../../docs/adr/0038-validity-correction-keeps-original-and-relationship.md)（区间更正）：「有效性更正是登记册上的独立事实，不改写原 `CommercialVersion` 键下的正文、批准与原区间」「接纳更正必须使该范围的 `ViewRevision` 变化」「与 `Revise` / `SupersededBy` 分清」。
- [ADR-0042](../../docs/adr/0042-acceptance-content-declarations-by-owning-object.md)（接受内容声明）：「适用校验组与人工复核指令归接单规则包正文声明」「待路由许可归服务产品，且必须携带依据引用」「读取面是独立只读端口……第一阶段用独立锚点选包/选产品，内容声明只在唯一选出之后按已选对象读取」「`found=false` 即实例未配置。本上下文不内置任何默认」。
- [ADR-0047](../../docs/adr/0047-terms-control-forms-credit-exposure-not-a-freeze.md)（账期）：「SA 控制策略答复携带方式与采用政策」「额度、逾期与账户映射的取值仍属实例半边（`PAR-SET-*`、`PAR-COM-*` 待登记）；机制以合成事实钉规则，不设任何默认额度」。
- [ADR-0054](../../docs/adr/0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)（接受前控制）：「`PreAcceptanceControlPolicyView` 增设『未配置』格」「`未配置`绝不落成`无控制`」「**本记录不解决提供方表面。** 控制策略声明族在 `party-commercial`（CONTEXT 与 CONTEXT-MAP 都已判给它），那是另一笔工作」「`PAR-COM-15`……`party-commercial` 侧连领域类型都还没有，`PreAcceptanceFinancialControlPolicyObject` 只作为封闭九类的一个枚举值存在」（该句取证于 ADR 落笔时；在途草稿已动这半边，见 §4.2）。
- [ADR-0050](../../docs/adr/0050-service-product-form-observable-through-the-resolution-closure.md)（ServiceProductForm，票面误记为 0053/0049）：「`ServiceProductForm` 已经建好模、在 `NewServiceProduct` 构造期受护，但没有任何读取路径……解析一个 `ServiceProductObject` 只交回一份版本身份，形态不可观察。这与 0034 落地前的价格规则、0044 落地前的结算政策是同一形状……服务产品形态是同一个洞的第三个实例」「把适配器写成常量`要求`就是一个默认值，而红线要求实例位置留空并拒绝默认。适配器必须读一份真声明再翻译」「产品缺席不使解析从`唯一解析`退化为`无适用依据`」。Open question 一：「网络使用资格」在 PC 侧没有模型，且不是服务产品形态——「它有自己的版本与有效期间，而形态是产品版本的内在属性」；这是一个具名的提供方缺口，需先经领域建模进入 PC 语言。
- [ADR-0053](../../docs/adr/0053-network-fact-families-are-derived-not-registrable.md) 与本题相关句：「登记册只可能存**定义原语**……原语的模式留待真有定义可登时再设计，现在不建表……没有一份真实网络定义在手，列出来的字段是替租户拟的，而模式一旦落库就按校验和固定」「错在圈的是产物不是来源」。（先例意义：登记什么/不登什么按「来源 vs 派生」分，且模式设计等真实定义。）
- [ADR-0049](../../docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 与本题相关句：「发布通道形态属机制半边（产品怎么把事件送到消费者），不是实例半边（租户的参数）。按开发主线的两半划分，机制现在就做。」（先例意义：机制/实例分线的裁法。）

---

## 二、现有形状（HEAD `2fbe405`）

### 2.1 `internal/partycommercial/domain` 的九类对象

共同壳 `CommercialVersion`（commercial_version.go）：值类型；字段为租户、kind、objectID、版本号、scope、
contentDigest、effective、status、approval（引用+来源+时间）、publishedAt/effectiveAt/closedAt、retirementRef、
successor、references（正文指名引用，有序切片，参与「同一次发布」判定）。生命周期
DRAFT→PUBLISHED→EFFECTIVE→EXPIRED/RETIRED/SUPERSEDED；`Publish` 要求批准完备+角色已确认+指名引用已发布
（ADR-0035/0036 分格）；`AppliesAt` 只认 EFFECTIVE 且落区间。`CommercialObjectKind` 封闭九类：
SERVICE_PRODUCT、CUSTOMER_CONTRACT、SUPPLIER_AGREEMENT、ACCEPTANCE_RULE_PACKAGE、
PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY、PRICE_RULE、SETTLEMENT_POLICY、CREDIT_POLICY、AUTHORIZATION_RULE。

九类各自的「正文件」（版本壳之外的内容对象）：

| # | kind | 正文件领域类型 | 关键字段与不变量 |
|---|---|---|---|
| 1 | SERVICE_PRODUCT | `ServiceProduct`（service_product.go） | version+form；form 封闭仅 `NetworkServiceForm`（面单渠道形态按 `PAR-COM-12` 有意不列）；构造要求版本 kind=产品且 EFFECTIVE。另有 `ProductChannelMapping`（product+channels+effective；只定义候选范围，不提供选择/优先级/默认/锁定；`CandidatesAt` 过期答空、`CandidatesAllowedBy` 只收窄） |
| 2 | CUSTOMER_CONTRACT | `CustomerContract`（customer_contract.go） | version+rulePackage 引用+按费用范围的 `FinancialControlBinding` map；构造期强制规则包引用（「规则或策略缺失不得被解释为允许接受」）；绑定要么适用指名策略、要么显式不适用带 `InapplicabilityBasis`，同一范围两种约定即冲突；`FinancialControlFor` 未绑定答「不存在」不答「不适用」 |
| 3 | SUPPLIER_AGREEMENT | `SupplierAgreement`（supplier_agreement.go） | version+supplier+legalEntity+scope+purchasePlan（采购方向定价方案引用）+effective+终止痕迹；不保存履约/应付事实 |
| 4 | ACCEPTANCE_RULE_PACKAGE | `AcceptanceRulePackage`（acceptance_rule_package.go） | `AssembledRule`（五分区 `RuleCategory`×`RuleReference`，结构上没有地方放阈值取值）+`RulePackageApplicability`（产品、合同、法人、范围、期间五维） |
| 5 | PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY | **策略正文领域类型不存在**（ADR-0054 自认「只作为封闭九类的一个枚举值存在」）；在途另有合同声明件，见 §4.2 | — |
| 6 | PRICE_RULE | `CommercialPricePolicy`（price_policy.go） | version+direction+plan（`PricingPlanReference`）+scope+effective；跨向绑定无声明转换即 `ErrPriceDirectionBindingConflict`（AT-PC-033）；价卡存续答复 `PricingPlanStandingLookup` 是入参，本上下文不查不猜 |
| 7 | SETTLEMENT_POLICY | `SettlementPolicy`（settlement_policy.go） | version+method（封闭二值 PREPAID/TERMS，第三值客户级默认被有意排除）+`SettlementApplicability` 六维（法人、相对方、合同、费用范围、币种、期间）；同一精确范围两法命中即 `ErrSettlementMethodConflict` |
| 8 | CREDIT_POLICY | `CreditPolicy`（credit_policy.go） | version+legalEntity+level+chargeType+limitMinor+effective；信用按费用类型不按客户授予 |
| 9 | AUTHORIZATION_RULE | `AuthorityGrant`（authority_grant.go） | version+action（封闭二值 MANUAL_REVIEW/ACTIVE_REJECTION）+legalEntity+level+scope+effective；`ErrAuthorityRulesNotConfigured` 与 `ErrNotAuthorized` 分格（未配置≠不允许） |

规则包/产品/合同之外的**阶段内容声明件**（同属 PC 领域，评审 #7 的「准入、退件」商业半边落在这些件上）：

- `AsOfPolicy`（as_of_policy.go）：规则包为各类下游判断声明时点语义（不存值）。
- `AcceptanceRuleContent` + `ManualReviewDirective`（acceptance_content.go）：适用校验组（七组封闭）+人工复核指令；`PendingRoutingPermission` 归服务产品、必带依据。
- `IntakeQualificationContent` / `FinalRuleContent` / `CancellationAuthorityContent`（service_stage_content.go）：收寄资格（PAR-COM-16）、终局规则与取消授权目录（PAR-COM-17）；声明为空集是缺件错误，真没有要求也要显式声明空清单。
- `ValidityCorrection`（validity_correction.go）：区间更正事实，指向原版本、携带新区间与更正引用（ADR-0038）。

登记册 `CommercialRegistry`（commercial_registry.go，**内存对象**）五通道：`versions`（键=租户+kind+对象+版本号，
同键同内容重放/异内容冲突）、`corrections`、`policies`（价格政策）、`settlementPolicies`、`products`（形态，ADR-0050）。
`ViewRevision` 按租户+范围从五通道内容派生（「手工递增总会有人忘记，派生值不可能与登记册实际内容脱节」）。
**注意：`ProductChannelMapping`、`CustomerContract` 正文、`SupplierAgreement`、`CreditPolicy`、`AcceptanceRulePackage`
正文、阶段内容声明件在登记册上没有通道**——登记册通道只覆盖五种。

### 2.2 端口与注释自认的收窄

`ports.go`（HEAD 版）六口：`CommercialAuthorityView`（按租户+范围取 `*CommercialRegistry`）、`AsOfPolicyDeclaration`、
`AcceptanceContentDeclaration`（两方法 found=false 语义相反：内容缺=未配置停未决，待路由缺=未许可照常推进）、
`CommercialResolutionStore`（LoadResolution+Save，ADR-0027/0031）、`PublicationRegistry`（LoadForScope+SaveVersion）、
`Clock`、`AuthorityGrantStore`（LoadEffectiveGrants+SaveGrant）。

**`PublicationRegistry` 注释自认的收窄（原话）**：

> 本口先只承载版本册；有效性更正册（ADR-0038）与价格/结算政策册（ADR-0034/0044）的持久化面另票补，
> 端口届时扩展而不是在这里预开空方法。

**`CommercialAuthority` 适配器注释自认的收窄（原话，评审 #7 点名处）**：

> **已知的收窄**：登记册的完整形状还含区间更正（ADR-0038）与价格/结算政策（ADR-0034/0044），而这三册今天
> 都还没有持久化面——本仓也没有任何生产代码调用 RegisterValidityCorrection / RegisterPricePolicy /
> RegisterSettlementPolicy。因此本口现在交回的就是全部已落库的权威事实，不是「省略了几册」。这些册子拿到
> 持久化面时，装配点在这里而不在解析侧：把它们漏在外面会让 ViewRevision 按不完整的内容派生，从而在视图
> 其实已经变了的时候答「还是同一个视图」。

（该注释写「三册」；ADR-0050 之后登记册第五通道 `products` 同样无持久化面，注释未及更新，见 §2.4 第 4 行。）

应用编排的端口依赖：`ResolveCommercialBasisHandler` 与 `ValidateCommercialBasisHandler` 依赖
CommercialAuthorityView+CommercialResolutionStore+Clock；`FormJudgmentAsOfHandler` 依赖
CommercialResolutionStore+AsOfPolicyDeclaration；`AdjudicateCommercialAuthorizationHandler` 依赖 AuthorityGrantStore。

### 2.3 migrations/party_commercial 0001–0006 一句话

| 迁移 | 表 | 一句话形状 |
|---|---|---|
| 0001（`commercial_publication`） | `commercial_version` | 发布登记册：键=租户+kind+对象+版本号，区间/摘要平铺，批准与指名引用整份进 jsonb，状态只收 2..6（草稿不入册）；kind CHECK 原为 1..7 |
| 0002 | `commercial_resolution` | 已固定解析：键=租户+解析标识，只收 outcome=1（唯一已解析），闭包正文进 jsonb snapshot，摘要平铺供重放/冲突判定 |
| 0003 | `authorization_grant` | 授权治理册：键=租户+对象+版本号，action 封闭二值镜像领域，自带足以重建 `AuthorityGrant` 的快照；注释明言「日后授权规则走发布登记册属另票」 |
| 0004（`c1866c2`，评审 #4 的治笔） | 改 `commercial_version` 约束 | kind CHECK 放宽 1..9 对齐领域封闭集；注释明言「只对齐约束，不接线：今天没有任何写入方往登记册放这两类……信用政策尚无存储口」 |
| 0005 | `as_of_policy_declaration` | 规则包时点锚声明：键含 judgment_type（同一判断不得两条结构性落库），不存时点值本身，kind 钉死=4；「行属实例半边……今天没有租户因而本表为空」 |
| 0006 | `acceptance_rule_content` + `acceptance_rule_check_group` + `pending_routing_permission` | 接受内容声明按拥有对象分表：前两表挂规则包（kind=4，组集合子表键含组名），后表挂服务产品（kind=1，basis_ref NOT NULL——「可空的依据列会让一行无依据的许可存得下来」）；两处 found=false 语义相反逐表写明 |

（0007 在途未提交，见 §4.2。）

### 2.4 「领域件在位、无持久化面、无生产调用」清单（PC 侧全量）

原票三例（出处：`.scratch/outbox-handoff-consumption-map/report.md` 附带 a，取证 `4d57ecd`）在 `2fbe405` 重验仍成立；
本节按九类+声明件补全。判据：非测试代码 grep 构造函数/登记方法（声明处与注释命中不算调用）。

| 领域件 | 登记/读取通道 | 持久化面 | 非测试生产调用 |
|---|---|---|---|
| `ValidityCorrection` | `RegisterValidityCorrection`（登记册通道在） | **无**（ports.go 注释自认「另票补」） | **零**（原票三例之一） |
| `CommercialPricePolicy` | `RegisterPricePolicy`（通道在） | **无** | **零**（三例之二） |
| `SettlementPolicy` | `RegisterSettlementPolicy`（通道在） | **无** | **零**（三例之三） |
| `ServiceProduct`（形态） | `RegisterServiceProduct`（通道在，ADR-0050） | **登记册面无**；唯一落库路径是解析闭包快照（`commercial_resolution.go` 的 closureDocument 序列化/重建已采用产品——那是「已采用」快照，不是权威登记册） | 登记方法零调用；`NewServiceProduct` 生产调用仅闭包快照重建一处 |
| `ProductChannelMapping` | **登记册无通道** | 无 | 零 |
| `CustomerContract` 正文（规则包引用+财务控制绑定） | **登记册无通道**（版本壳可入 0001，正文件无处去） | 无 | 零 |
| `SupplierAgreement` 正文 | **登记册无通道** | 无 | 零 |
| `CreditPolicy` 正文 | **登记册无通道**（0004 注释自认「信用政策尚无存储口」） | 无 | 零 |
| `AcceptanceRulePackage` 正文（五分区规则引用+五维适用性） | **登记册无通道** | 无（其**声明件** AsOf/接受内容有 0005/0006 面并有生产读口，正文本身没有） | 零 |
| `IntakeQualificationContent`/`FinalRuleContent`/`CancellationAuthorityContent` | 读口是 PS 适配器内部协作者接口（`IntakeContentSource` 等三口） | **无**（service_stage_rules.go 注释自认「存储问题属于整个阶段内容声明族（Intake/Final/Cancellation 三口一盘棋）……日后存储成片时三口同切」） | 读口无生产实现 |
| PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 策略正文 | **领域类型不存在**（ADR-0054 自认；枚举值在） | 无 | — |
| （反例对照）`AuthorityGrant` | 0003 表 + `AuthorityGrantStore` PostgreSQL 实现 | **有** | **有**（PS `active_rejection.go` 经 `AdjudicateCommercialAuthorizationHandler`） |
| （反例对照）AsOf/接受内容声明 | 0005/0006 表 + 读口 PostgreSQL 实现（`a8cf535`/`bfb2063` 换真） | **有** | 有（PS `CommercialBasisAdapter` 消费；写入方无——表空属实例半边） |

一句话汇总：**九类对象里只有版本壳（0001）、解析闭包（0002）、授权治理册（0003）、规则包与产品的三样声明
（0005/0006）有持久化面；五通道登记册里价格政策、结算政策、产品形态、区间更正四通道加上正文件六种
（映射/合同/协议/信用/规则包正文/阶段内容）全部无落库面，其登记方法在非测试代码中零调用。**
这正是评审 #7「尚未形成可发布、可持久化、可完整加载的产品版本组合」在代码上的形状。

---

## 三、机制/实例分线（按开发主线两半划分陈述现状，不提方案）

### 3.1 机制半边（产品怎么表达/装载一个产品版本组合——今天已在与还缺的）

**已在**：九类对象领域件与共同不变量（§2.1）；发布/生效/收尾生命周期与 AT-PC-001..016 行为；解析闭包与两阶段
（UC-PC-002 契约测试钉住）；版本壳持久化（0001+0004）、闭包持久化（0002）、授权册（0003）、三样声明面（0005/0006）；
`ViewRevision` 按内容派生；ADR-0034/0044/0050 的「政策/产品随闭包可观察」形状；ADR-0042 的「内容声明按拥有对象、
唯一选出后读取」纪律。

**缺**（评审 #7 所指，全部属机制半边——是「产品怎么装载」不是「租户的参数」）：

- 四通道登记册持久化面：价格政策册、结算政策册、产品（形态）册、区间更正册（§2.2 两段自认注释）。
- 六种正文件的登记通道与落库面（§2.4：映射、合同正文、协议正文、信用政策、规则包正文、阶段内容声明族）。
- 装载口：`PublicationRegistry.LoadForScope` 今天只装版本壳；`CommercialAuthority` 注释明言政策册落面时「装配点在这里」。
- 「未配置格」范式已有三处先例可循（ADR-0052/0053 网络、0054 控制策略、既有声明口 found=false）——机制上
  每个装载口都要能诚实说出「这格没人登记过」。

**边界内不含**（评审 #7 罗列但按 CONTEXT 所有权不落 PC 的）：主备线路/服务区/容量（NR/NO 拥有，PC 只有
产品—渠道映射与渠道约束）；清关执行（CC 拥有，PC 只有规则包的监管资料分区与关系/委派）；SLA 时效与轨迹投影、
赔付结论（VE 拥有；PC 拥有的是**客户服务规则版本**这个商业半边——该词条今天在 Go 里没有对应领域件，九类
枚举中也无此类，是 CONTEXT 有语言、代码无形状的一处）；质量规则（PG/NO）。「网络使用资格」是 ADR-0050
Open question 点名的具名提供方缺口，PC 语言尚无该词条。

### 3.2 实例半边（登记册 PAR-COM-* 行现状，docs/product/PILOT-PARAMETER-REGISTER.md）

| 行 | 参数 | 当前状态 |
|---|---|---|
| PAR-COM-04 | 首发服务形态 | **已确认：网络服务**（依据 PILOT-SCOPE） |
| PAR-COM-11 | COD | **本期不适用**（范围决策） |
| PAR-COM-12 | 面单渠道服务形态 | **本期不适用：首发不销售独立面单渠道服务**（范围决策） |
| PAR-COM-01/02/03 | 锚点客户/测试账户/责任法人 | 待提供（01、03 有预留标识 `CUST-PILOT-01`/`LEGAL-ENTITY-PILOT-01`，尚未指定） |
| PAR-COM-05/06/07 | 服务产品及版本/合同版本/客户服务规则版本 | 待提供 |
| PAR-COM-08/09/10 | 结算模式范围/币种/计费截单付款条件 | 待提供（08 明言「预付与账期可独立拆分只是已确认的产品设计选择，不是现实客户事实」） |
| PAR-COM-13..17 | 资料更正规则/锚点与规则包与时点策略/接受前财务控制策略/收寄资格与承诺/取消与终局 | 待提供 |

「参数就绪门槛」节把 PAR-COM-01、03、05、06、13..17 列为**进入渠道或国家专属详细设计前**必须已确认的条件——
那是门槛清单，不是现状。真实价卡、客户合同、供应商协议按 AGENTS.md「当前尚无租户」一律不可能取得；
「最小产品版本组合」里凡引用这些行的格子只能留空并拒绝默认值，可发布/可装载的**结构**与「未配置」的
诚实表达属机制半边、现在就做——这条分法即 ADR-0049 否决项五的原文判据（§1.5 末行）。评审「下一步」要求的
纵向闭环用 `SYN-*` 合成实例，按红线只记隔离 `S`。

---

## 四、升级点（若产品版本组合落 PC，现有消费侧哪些会受影响——只列事实依赖，不提方案）

### 4.1 消费侧依赖清单（今天的调用形状）

| 消费方 | 适配器 | 依赖的 PC 面 | 受什么影响 |
|---|---|---|---|
| PS | `CommercialBasisAdapter`（commercial_basis.go） | `ResolveCommercialBasisHandler`/`ValidateCommercialBasisHandler`/`FormJudgmentAsOfHandler` 编排 + `AsOfPolicyDeclaration`/`AcceptanceContentDeclaration` 端口；闭包翻译含 `settlementTermsFor`（读 AdoptedSettlementPolicy）与产品/政策观察 | 登记册通道扩持久化面后，解析候选集合与 `ViewRevision` 的内容基数变宽——ADR-0038「接纳更正必须使 ViewRevision 变化」与 `CommercialAuthority` 注释「漏在外面会让 ViewRevision 按不完整的内容派生」直接作用于它的提交前失效检测（AT-PC-026 一路） |
| PS | `judgment_as_of.go` | `FormJudgmentAsOfHandler`（0005 声明面） | 装载口扩展不动它；规则包正文若得到持久化面，其「已生效」判据来源变化会波及 `ErrUnusableRulePackage` 一格 |
| PS | `service_stage_rules.go` | `IntakeContentSource`/`FinalContentSource`/`CancellationContentSource` 三口（适配器内部协作者，无 PC 持久化面） | 注释已自认「日后存储成片时三口同切」——阶段内容声明族拿到落库面时这三口换真，属产品版本组合装载面的一部分 |
| PS | `active_rejection.go` | `AdjudicateCommercialAuthorizationHandler`（0003 册） | 授权规则版本若按 0003 注释「走发布登记册属另票」并入版本册，两册关系要重陈述 |
| NR | `eligibility.go`/`routing.go`（+form.go 翻译表） | `CommercialResolutionStore` 读闭包→`AdoptedBasis.ServiceProduct()`→翻译 Form（ADR-0050） | 产品册拿到持久化面并接进 `LoadForScope` 后，「只登版本没登产品」的 `ErrServiceProductUnavailable` 分支才可能在生产闭包里消失；翻译表本身不动（全函数、默认报错不吸收） |
| SA | **无 partycommercial 适配器**（目录不存在） | `PreAcceptanceControlPolicyView` 端口等提供方（ADR-0054 已备好三格消费面；port-inventory-r25 列为「提供方表面缺」） | 产品版本组合若含控制策略/合同控制声明的发布与装载，正是这个适配器的提供方前置 |
| PP | `parcelpricing` 无 PC 适配器；价卡存续方向相反（PC 解析以 `PricingPlanStandingLookup` 为入参问 PP） | — | 价格政策册落面后，「登记册持有的政策集合」（ADR-0034 语）从测试夹具变为库装载，Lookup 入参形状不变 |

### 4.2 与 MCP-2 在途 pre_acceptance_control 声明族的交叉（在途未提交，不作为现状断言）

在途三件的形状：领域件 `PreAcceptanceControlDeclaration`（拥有对象=**客户合同版本**，封闭二值 REQUIRED/
NOT_APPLICABLE，`不适用`必带依据、`要求`不得带，零值=未声明）；端口 `PreAcceptanceControlDeclarationView`
（found=false=实例未配置，注释明言「只答『要不要』……方式与采用政策走 ADR-0044，本口一概不碰」）；迁移 0007
（表挂 kind=2，requirement 与 basis 绑成一条 CHECK，「未声明由查无此行表达」）。

与本票主题的交叉点（均为事实陈述）：

1. **同一张「按拥有对象声明」的图上新添一口**：0005（规则包→时点锚）、0006（规则包→内容/产品→待路由）、
   0007 在途（合同→控制要不要）。产品版本组合的持久化与装载设计若动「声明按拥有对象归属」这条纪律
   （ADR-0042），0007 是它最新的适用实例。
2. **与 ADR-0054 的分工衔接**：0054 明言「本记录不解决提供方表面」；在途件正是那个提供方表面的「要不要」半边。
   SA 侧 `ControlPolicyNotConfigured` 未决格的解除依赖这一族+SA 的 PC 适配器（今天不存在，§4.1）。
   策略正文（第 5 类的「控制怎么做」）领域类型在途工作也未建——两半边各自的持久化面都会落在产品版本组合的盘子里。
3. **合同正文的双通道并存**：`CustomerContract` 正文件里已有按费用范围的 `FinancialControlBinding`（§2.1 第 2 行），
   在途 0007 是合同版本级的「要不要」声明——两者语义不同（绑定=范围级适用哪份策略/显式不适用；声明=合同版本级
   要不要），但拥有对象同为客户合同版本，产品版本组合装载合同正文时两路都在场。
4. **`ViewRevision` 基数**：在途件今天不参与 `ViewRevision` 派生（登记册无该通道）。若声明族进入权威视图范围，
   §4.1 第 1 行的失效检测语义随之变化；若不进入，「视图其实已经变了的时候答还是同一个视图」那句注释的适用范围
   需要重新陈述。此处只登记张力，不裁。

---

## 附：交叉引用

- 原票三例出处：`.scratch/outbox-handoff-consumption-map/report.md` 附带 a（取证 `4d57ecd`，本包在 `2fbe405` 重验）。
- 端口全景：`.scratch/port-inventory-r25/report.md`（盘于 `d40b03a`；PC 侧缺口 0 的判据 A 虚高说明见其第四节——
  `AsOfPolicyDeclaration`/`AcceptanceContentDeclaration` 当时无生产实现，`a8cf535`/`bfb2063` 已换真，该节那半句已过时）。
- 评审原文：`docs/review/081701.md`（工作树未提交文件）。
