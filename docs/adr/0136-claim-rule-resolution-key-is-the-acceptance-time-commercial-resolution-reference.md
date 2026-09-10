# ADR-0136：索赔资格规则（首次索赔期限、最低材料）按目标包裹所属委托接受时固定的商业解析回指选用——键是（租户，回指），合同由 `party-commercial` 闭包解出，`visibility-exception` 不登合同维、不登固定时刻、不自己再解析一次；客户合同版本与客户服务规则版本都要在接受时闭包的必需依据里；VE 侧不立键登记面，`RuleResolutionKeySource` 换成回指读缝

Status: Accepted（2026-09-10，通道 4 按通道 1 派单 task-d6660969「用户授权代裁，VE owner 口径，键形状连 PC owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [ve-claims/04](../../.scratch/ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md) 全文、票 [ve-claims/03](../../.scratch/ve-claims-read-seams/issues/03-claim-deadline-and-materials-read-party-commercial-rule-content.md)「裁决」与完成记录、[ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 全文、[ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md) 全文、[ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md) 决定一 / 四 / 七、[ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) 决定二、[ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md) 决定一 / 四 / 五、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[VE CONTEXT](../domain/visibility-exception/CONTEXT.md)「客户索赔项」词条与「证据、客户索赔与追偿」Rules 全节、[PC CONTEXT](../domain/party-commercial/CONTEXT.md)「商业依据解析」词条、Rules「委托被接受时固定其适用的…」与「面向具体提交的商业解析分两阶段…」两句、Boundaries「不拥有…具体委托采用的判断时点值」那句、[UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md) 两阶段机制段、步骤表与 `AT-PC-018`、`internal/visibilityexception/adapters/partycommercial/claim_service_rules.go` 全文（`RuleResolutionKeySource` 头注、`serviceRuleDimensions`、`adoptedRuleVersion`）、`internal/visibilityexception/ports` 的 `EligibilityQuery`、`internal/parcelshipment/ports/commercial_resolution_reference_view.go` 全文、`internal/transportfulfillment/adapters/partycommercial/delivery_condition_source.go` 的 `CommercialResolutionReferenceSource` 与 `DeliveryConditionReferenceSource` 两个接口、`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go` 对必需依据的校验（`validateSettlement` 收合同维那一格）。没读 PC 闭包快照的 postgres 实现（`commercial_resolution.go`）是否把已采用的客户服务规则版本落进快照并读得回；没读 VE `handle_claim.go` 全文与 `EligibilityRuleView` 的 postgres 实现。）
Date: 2026-09-10

## Context

票 ve-claims/03 把索赔资格的首次索赔期限与最低材料两维接上了 `party-commercial` 的客户服务规则正文（ADR-0104），解析走 PC 既有闭包。闭包键要租户、客户账户、责任法人候选、商业范围、目的、锚点（时刻 + 锚点策略版本）与必需依据，而 VE 的 `EligibilityQuery` 只带租户、客户账户、合同责任范围引用、目标范围、索赔类型与申请人。票 03「裁决」据此在适配器包内立了 `RuleResolutionKeySource`——「VE 词到 PC 键的翻译」——nil 或 formed=false 即显式未配置、两维如实答未登记，并把登记面另立为票 04。票 04 列了四问：锚点从哪来、键按什么维登、要不要一并要 `CustomerContractObject`、登记入口放哪。

这四问背后是同一道题：**索赔资格规则是按什么选出来的。** 两个候选：

- **候选甲 · VE 自己再解析一次**（票 03 的形）：VE 登一份键（范围、目的、法人候选、锚点策略版本、必需依据），按它到 PC 走第一阶段解析选出客户服务规则版本。锚点从哪来是这条路上必答的：登固定值（PS 的解析键登记面就是这么登的，`AT-PC-018` 不拿系统时间顶锚点），还是按某件事实取（索赔提交时刻？包裹收寄时刻？）。
- **候选乙 · 读接受时固定的闭包**（ADR-0079 / ADR-0133 的形）：委托被接受时 PS 固定了一份商业解析回指，PC 按它持有闭包快照，闭包里已采用的成员含服务产品版本、客户合同版本、接单规则包版本等（ADR-0062 决定一的路：回指 → `LoadResolution` → `AdoptedFor(...)`）。索赔资格规则若也是接受时采用的成员之一，VE 只需拿到回指，一样都不必登。

用三个场景试过：

- **接受后客户服务规则发了新版本再索赔。** 委托在规则版本 R/v1 下接受，闭包 C1 固定了 R/v1；随后 R/v2 发布并生效于新接受；客户此时对该包裹提索赔。按 R/v1 还是 R/v2？PC CONTEXT Rules「委托被接受时固定其适用的服务产品版本、客户合同版本、…」与客户合同版本生命周期「合同版本到期或被后续版本替代，不改变已经接受委托所保存的合同依据」说的是同一件事：接受时固定的依据不随后来的版本改口。索赔资格规则是卖出去的服务形态的一部分（ADR-0104 把它归为「产品或合同采用的服务规则」，ADR-0133 决定四把交付条件与它列为同族），客户下单时买到的就是 R/v1 说的期限与材料；按索赔提交时刻选 R/v2，客户会拿到一份他下单时不存在的规则——期限变短是单方面收紧，变长是单方面让步，两种都不是合同说过的话。所以规则版本钉在接受时。**这一句一旦成立，候选甲里「锚点从哪来」就没有独立答案了**：锚点就是接受时那一份，它已经固定在回指所指的闭包里；VE 再登一份锚点策略版本、再解析一次，是 ADR-0079 决定五点名的「分两口取就给出了两次解析的机会」。
- **目标不是包裹。** VE CONTEXT「客户索赔项」允许目标是「一个包裹或明确服务责任范围」。目标是明确服务范围时，没有任何已接受委托的包裹身份可问 PS，回指取不到。候选乙对这一格答不出规则版本；候选甲能答，但答的是「这个客户账户此刻在某范围下当前有效的规则版本」——那正是第一个场景否掉的读法，换了个目标就成立不了。这一格今天的行为是 `Keys == nil` 那一行：两维如实答未登记、编排停在指名到维的未决、索赔项一字不动。本记录让它保持这一行为，并把「明确服务范围的索赔项按什么选规则」单列为越权风险点——它需要 VE owner 先说清那种索赔项的商业依据是什么，不是一个键形问题。
- **接受时闭包没采用客户服务规则版本。** PS 的解析键登记面（`commercial_resolution_key`）由租户登记必需依据集合；`CustomerServiceRuleObject` 在封闭集内（ADR-0104 决定四），但没有哪一句要求它必在。租户没把它列进去，接受时闭包就没有这一成员，按回指走到闭包再 `AdoptedFor(CUSTOMER_SERVICE_RULE)` 答 `ok=false`。这是「没登」的一种：缺的不是规则正文，是租户没让接受时解析把这一类依据固定下来；恢复动作是去 PS 的解析键登记面把它列进必需依据——仍是登记动作，不是修代码，所以按 ADR-0029 归「未登记」那一格，与正文未登记同格不同因（适配器注释要把两因写开）。已接受的委托不会因此追溯取得规则版本：接受时没固定的，就是那次接受没有采用的依据，与 PC CONTEXT「某项依据对该服务形态确实不适用时，必须显式记录『不适用』及原因，不能以缺失代替判断」同一立场——本记录不替它补。

## Decision

**一、索赔资格规则（首次索赔期限、最低材料两维）的适用规则版本，是该索赔项目标包裹所属委托被接受时固定的商业依据里已采用的客户服务规则版本；选用时点就是那次接受的商业选择锚点，已固定在闭包里。** VE 不登固定锚点值、不取索赔提交时刻、不取包裹收寄时刻、不取系统当前时间（`AT-PC-018` 同一句）。判据在第一个场景：接受时固定的依据不随后来的版本改口（PC CONTEXT Rules「委托被接受时固定其适用的…」、客户合同版本生命周期那句），索赔资格规则是卖出去的服务形态的一部分（ADR-0104、ADR-0133 决定四的分族）。UC-PC-002 第二阶段「由已选规则包为各类下游判断声明 `asOf` 策略，`parcel-shipment` 逐项形成语义和值」说的是接受流各判断的时点，索赔资格不是那一族里的一项判断——它不形成新的时点值，它读接受时已固定的那一份；PC CONTEXT Boundaries「不拥有…具体委托采用的判断时点值」因此不被本记录触碰。VE CONTEXT「证据、客户索赔与追偿」Rules 加一句时点语义（随本记录同笔）。

**二、VE 到 `party-commercial` 取索赔资格规则的键是（租户，商业解析回指）；回指由 `parcel-shipment` 按（租户，包裹身份）答；合同由 PC 闭包解出，VE 不持有合同维、不翻译合同标识、不自己再解析一次。** 三段与 ADR-0133 决定二同形：

- PS 那一头：VE 把索赔项的目标范围引用原样作包裹身份问 `CommercialResolutionReferenceView`（ps-port-remainder/07 已落地的窄读口），封闭三格照它——回指 / 没有（对象不属任何已接受委托的成员集合，含目标是明确服务范围、集运单元、不可见对象；不区分不存在、他租户与未授权）/ error（`已接受`而接受决定缺回指）。VE 不猜目标范围引用是不是包裹（判据同 TF 交付适配器「载运对象……今天不替它猜」）：PS 答「没有」就是没有。
- PC 那一头：按（租户，回指）`LoadResolution` 取闭包，`AdoptedFor(CUSTOMER_SERVICE_RULE)` 取已采用的规则版本，再按版本点读正文（ADR-0104 决定四的点读口，不换）。合同版本是这份闭包的结论不是输入（ADR-0080 决定一 / 四）；「VE 的合同责任范围引用与 PC 客户合同对象标识是否同一标识空间」这道题（票 03「裁决」明写今天没有文档立过）因此**消掉**——VE 不翻译合同标识，只递回指。`EligibilityQuery.Contract` 留在查询上，是 VE 自己两本册（合同责任范围、申请人授权目录）的键，本记录不动它、不拿它去 PC 过滤（判据同 ADR-0079 决定二）。
- 结果代数按恢复动作分格（ADR-0029）：PS 答没有 → 两维未登记（与今天 `Keys == nil` 同一行为）；闭包在场却未采用客户服务规则版本 → 两维未登记（恢复动作：去 PS 解析键登记面列进必需依据；注释与正文未登记分开写因）；规则版本在场而正文未登（`LoadCustomerServiceRule` found=false）→ 两维未登记（去 PC 登正文，票 03 原格）；`LoadResolution` found=false 对一份已接受委托的回指 → error（提供方缺数据，ADR-0133 决定二同一格）；闭包在场却未采用客户合同版本 → error（决定三）；PS 答 error → error。**VE 不再走 PC 第一阶段解析**：`adoptedRuleVersion` 对闭包各结局的分派整段退役，`ErrCustomerServiceRuleUnresolved` 随之退役——读一份已固定的闭包没有`适用冲突` / `解析未决` / `输入未受理`可答。

**三、客户合同版本与客户服务规则版本都必须在接受时闭包的必需依据里；「规则版本指名的合同就是采用的那一版」由 PC 闭包解析时的指名引用核对守，VE 不重核。** 客户合同版本：PC CONTEXT Rules「委托被接受时固定其适用的…客户合同版本」说它恒在，闭包在场却没采用是接受流的装配缺陷，答 error（判据同 ADR-0133 决定二「闭包在场却未采用客户合同版本 → error」、ADR-0062 决定三）。客户服务规则版本：租户在 PS 解析键登记面把 `CustomerServiceRuleObject` 列进必需依据，接受时闭包才有它（第三个场景，答未登记那一格）。合同列进必需依据的意义在 ADR-0104 决定四那条既有闭包路上：规则版本指名的合同由闭包 `namedReferencesConfirmed` 在解析时核过，VE 读到的规则版本与合同版本必然是同一次解析采用的一对；不列合同，规则与合同的对应就无人核——这一句是票 03「裁决」原话，本记录把它从「归登记方」改成「必须」。

**四、VE 侧不立键登记面；实例半边落在 `parcel-shipment` 既有的解析键登记面（`commercial_resolution_key`，经 `parcel-commercial` CLI 登记）——那一行的必需依据集合要含 `CustomerServiceRuleObject`（与 `CustomerContractObject`）；`parcel-ve-register` 不加命令。** 票 04 预设的登记面（必需依据集合、锚点、范围、法人候选）登的五样，在决定一 / 二之下全是接受时闭包已经固定的东西：VE 再登一份就是第二次解析（ADR-0079 决定五），且两份键一旦分歧，同一个包裹的交付条件按一套依据、索赔规则按另一套——ADR-0133 与本记录同族，键该同源。`RuleResolutionKeySource` 接口退役，换成与 TF `delivery_condition_source.go` 同形的两个包内窄接口：`CommercialResolutionReferenceSource`（到 PS 取回指，真实装配交 PS `adapters/postgres` 里实现 `CommercialResolutionReferenceView` 的那一只，只 import PS `domain` 不 import PS `application` / `ports`，判据同 TF 那一只的头注）与 PC 的 `CommercialResolutionView`。两个接口都留在 `internal/visibilityexception/adapters/partycommercial/` 不进 VE `ports`（ADR-0025：签名引用提供方类型）。装配处 `buildClaimEligibilityRules` 的 `Keys=nil` 那一格随之消失：nil 半边在装配期拒（ADR-0079 决定八），装配测试里的 `syntheticRuleKeys` 退役，改为经 PS 登记面登一行含 `CustomerServiceRuleObject` 的 SYN 解析键、走一次合成接受、再按回指读到「已登记」态。

## Consequences

- 票 ve-claims/04 的「要做什么」整段改写：不建迁移、不建 VE postgres 行搬运、不建登记口；建 VE→PS 回指读缝一件（适配器包内接口 + 真实装配注入）、改 `ClaimServiceRules` 三段（回指 → 闭包 → 点读）、退役 `RuleResolutionKeySource` / `adoptedRuleVersion` / `ErrCustomerServiceRuleUnresolved`、改装配与装配测试；Blocked by 改为无（票 03 已 resolved），Status → ready-for-agent。
- VE CONTEXT「证据、客户索赔与追偿」Rules 加时点语义一句（随本记录同笔）；PC CONTEXT 不改——本记录没有给 PC 加任何语言，只读它已有的闭包。
- 生产上这条路与 ADR-0079 / ADR-0133 同一停点：PS 的解析键登记面尚未在生产上让闭包形成（ADR-0079 Consequences、ADR-0133 越权风险点 3），机制接上前 PS 那一头对每个对象都答「没有」、两维停在未登记——与今天 `Keys=nil` 的可观察行为一字不变，装配测试经合成解析键钉「已登记」态。
- 明确服务范围为目标的索赔项两维恒答未登记（越权风险点 1）；`ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED` 那一格对这一状态命名不准，与票 ve-claims/05 点名的两处同因，归那一票。
- 不在本记录内：起算事实源、业务日历、`Notice` 的来源（票 03「裁决」留格三条，各归其处）；PS 解析键登记面要不要把 `CustomerServiceRuleObject` 与 `CustomerContractObject` 从「可登」改为「必登」（越权风险点 2）；PC 闭包快照是否落已采用的客户服务规则版本并读得回（越权风险点 3）。

## Alternatives considered

- **候选甲 · VE 自登键、再解析一次（票 03 的形延伸到登记面）。** 否决：锚点若取索赔提交时刻，撞 PC CONTEXT「委托被接受时固定其适用的…」（第一个场景）；若登固定值，一份不随委托变的时刻对一切委托选同一版规则，是拿登记冒充判断；若取接受时刻，那就是回指所指闭包里已固定的锚点，再登一份是第二次解析（ADR-0079 决定五）。
- **锚点 = 索赔提交时刻。** 否决：客户拿到一份下单时不存在的规则；同一委托的两个包裹在不同时刻索赔会按不同版规则审。
- **锚点 = 包裹收寄时刻。** 否决：收寄是 TF 提供的事实，接受时固定的依据不因收寄改口；两个时刻之间发了新版本时它与接受时刻答案不同，而 PC CONTEXT 只认接受时。
- **键带 VE 的合同责任范围引用，由 VE 翻译成 PC 客户合同对象标识。** 否决：两个标识空间是否同一今天没有文档立过（票 03「裁决」），替它们假定相等就是在适配器里判断；合同是闭包的结论不是输入（ADR-0080 决定一）。
- **闭包在场却未采用客户服务规则版本 → error。** 否决：恢复动作是去 PS 解析键登记面列进必需依据，是登记不是修代码（ADR-0029）；与 ADR-0133 对客户合同版本答 error 不同，因为 PC CONTEXT 说合同恒在、没说客户服务规则恒在。记为越权风险点 2 的反面。
- **目标是明确服务范围时按客户账户当前有效的规则版本代选。** 否决：正是第一个场景否掉的读法；那种索赔项按什么选规则要先有商业语言（越权风险点 1）。
- **登记入口放 `parcel-ve-register`（票 04-4，派单方已按 A 类裁定）。** 随决定四消解：VE 没有键要登，`parcel-ve-register` 不加命令。若 owner 复核后回到候选甲，那条裁定照旧适用（ADR-0025 实例半边协作者接口留适配器包内；登键映射是写不是跨上下文读，不碰票 03 那句「受控登记口不接跨上下文读」）。

## 越权风险点

1. **明确服务范围为目标的索赔项。** VE CONTEXT 允许它存在，本记录让它两维恒答未登记（与今天同）。那种索赔项的商业依据是什么（客户账户级？合同级？按什么时点？）是 VE owner 的语言题；本记录没有替它选键，也没有替它开第二条路。
2. **PS 解析键登记面对两类依据的校验强度。** `commercial_resolution_keys.go` 今天只在请求结算依据时强制合同维（UC-PC-002「请求结算依据时还必须包含能够区分合同…的业务维度」）；按 PC CONTEXT「委托被接受时固定其适用的…客户合同版本」合同应恒在，客户服务规则则「可登」。要不要把两者改成登记面上的「必登」归 PS owner；本记录只要求「不在闭包里就按各自那一格答」，不改 PS 登记面。
3. **PC 闭包快照对客户服务规则成员的读写对称。** ADR-0079 决定九「已固定的闭包快照必须原样读得回消费方要读的每一样东西」；本记录把 `AdoptedFor(CUSTOMER_SERVICE_RULE)` 变成一条生产读路径，快照若没落这一成员就是写得进读不回。我没读 `commercial_resolution.go`，实施票开工第一件事是核它；不对称归 PC 地盘修，判据同 ADR-0079 Consequences 那次「当场兑现的实例」。
4. **派单方对登记面的预期与本记录不同。** 派单原话预期 VE 登记面仍登「必需依据集合、锚点策略版本、商业范围、目的、责任法人候选」；我按 ADR-0079 / ADR-0133 的形量出那五样全在回指所指的闭包里，再登即第二次解析，故决定四不立 VE 登记面。若 owner 认为索赔规则应按与接受不同的商业范围 / 目的选用（例如索赔专属范围），本记录决定二 / 四与票 04 改回候选甲的形，决定一（接受时锚点）不变。
5. **`EligibilityQuery.Contract` 与闭包采用的合同版本不核一致。** 票 03「裁决」原句「不核 `query.Contract` 与规则适用声明的合同是不是同一个标识」在本记录下仍成立；VE 两本册按它键入、PC 那一头按回指走，两者若指向不同合同，今天没有一格会报。

## Links

- [VE CONTEXT](../domain/visibility-exception/CONTEXT.md)：「客户索赔项」词条；「证据、客户索赔与追偿」Rules 时点语义那句（本记录的落地处）
- [PC CONTEXT](../domain/party-commercial/CONTEXT.md)：「商业依据解析」词条；Rules「委托被接受时固定其适用的…」「面向具体提交的商业解析分两阶段…」；Boundaries「不拥有…具体委托采用的判断时点值」
- [UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)：两阶段机制、`AT-PC-018`
- [ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)：决定一 / 二（本记录同形的两问）、越权风险点 3
- [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：决定二（`scope` 留签名不参与提问）、决定五（同源）、决定八（装配期拒 nil）、决定九（快照读写对称）
- [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：合同维是结论不是输入；两段式串一处拼
- [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md)：决定二（PS 按（租户，包裹身份）答）
- [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：决定一 / 四 / 五
- [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)：回指换闭包再取已采用成员的路
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：分格判据
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：接口留适配器包内
- 票 [ve-claims/04](../../.scratch/ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md)（四问出处）、[ve-claims/03](../../.scratch/ve-claims-read-seams/issues/03-claim-deadline-and-materials-read-party-commercial-rule-content.md)、[ve-claims/05](../../.scratch/ve-claims-read-seams/issues/05-application-names-registered-but-underivable-deadlines-as-not-registered.md)、[ps-port-remainder/07](../../.scratch/ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)
