# ADR-0050: 服务产品形态随解析闭包可观察

Status: Proposed  
Date: 2026-08-14

## Context

`network-routing` 有两个端口要商业侧回答「这个服务要不要判断网络可达性」：`CommercialEligibilityView`（UC-NR-002 步骤 4）与 `RoutingApplicabilityView`（UC-NR-001 步骤 3），两者共用 `NetworkEligibility`（要求/不要求加依据）。消费侧的翻译表已定：`NetworkServiceForm` → 要求，其余取值报错不吸收（ADR-0025 要求翻译是全函数）。

**它翻译不了，因为拿不到要翻译的那个值。** `ServiceProductForm` 已经建好模、在 `NewServiceProduct` 构造期受护，但没有任何读取路径能把它交到消费方手上：

- `AdoptedBasis` 带 `kind` 与 `CommercialVersion`，另有 [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md) 补的 `pricePolicy` 与 [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md) 补的 `settlementPolicy`——**没有 `ServiceProduct`**。
- `CommercialRegistry` 存 `versions`、`corrections`、`policies`、`settlementPolicies`——**也不存 `ServiceProduct`**。

于是解析一个 `ServiceProductObject` 只交回一份版本身份，形态不可观察。这与 0034 落地前的价格规则、0044 落地前的结算政策是同一形状，`AdoptedBasis` 自己的注释就写着那句判据：「方向与定价方案绑定可被观察，**而不是只剩一份 `CommercialVersion`**」。服务产品形态是同一个洞的第三个实例。

不能用「首发只有一个可达答案」绕过。`ServiceProductForm` 今天只有 `NetworkServiceForm` 一个合法取值（独立面单渠道服务被 `PAR-COM-12` 排除在首发外，枚举有意不列），因此翻译结果恒为`要求`——但把适配器写成常量`要求`就是一个默认值，而红线要求实例位置留空并拒绝默认。适配器必须读一份真声明再翻译。

## Decision

**一、`AdoptedBasis` 在采用了服务产品时携带 `ServiceProduct`。** 形状照 0034/0044：`AdoptedBasis.ServiceProduct() (ServiceProduct, bool)`，仅 `ServiceProductObject` 那一项在场，其余依据缺席。携带整个 `ServiceProduct` 而不只是形态值，与前两条先例一致——后续产品属性追加时不必再改一次闭包形状。

**二、登记册增设服务产品通道。** `RegisterServiceProduct` 与 `RegisterPricePolicy` / `RegisterSettlementPolicy` 同一分工：登记册只收已构造的对象，构造期不变量归领域类型自己守。

**三、缺席即不可观察，不是某种默认。** 只登了版本而没登产品时，`ServiceProduct()` 交回 `false`。**消费方必须把缺席当依赖不可用处置**（NR 侧形成`未形成判断`），绝不读成`要求`或`不要求`——后者正是本记录要堵的默认值。

**四、不改解析结果的判定。** 产品缺席不使解析从`唯一解析`退化为`无适用依据`。理由见备选一：那会改变每一个解析产品的既有闭包（含 `parcel-shipment` 的接受闭包）的结果，波及面远大于要修的缺口，而「绝不默认」这条保证由第三条与 `ServiceProductForm` 零值不合法共同交付，不需要靠退化解析来实现。

**五、本记录只解可观察性，不新建声明册、不新开用例。** 服务产品形态是产品版本的既有正文，读取纪律沿用 [ADR-0042](./0042-acceptance-content-declarations-by-owning-object.md)：内容只在唯一选出之后按已选对象读取，因此键取闭包里已唯一选出的那份产品版本，不另开独立查询键——独立键会让一次「校验」拿到与解析所得不同的产品，而 UC-PC-002 步骤 8 存在的全部理由就是堵这个。

## Consequences

- MCP-6 的两个 NR 适配器可以落地：它们要翻译的值终于有来源，且翻译仍是全函数（新增形态时 default 分支报错，不静默归入某一格）。
- 闭包夹具凡请求服务产品依据、且消费方要读形态的，须一并登记产品；不登记的既有夹具行为不变（第四条）。
- 服务产品是否参与 `ViewRevision` 派生需与 0044 的处置对齐（那条让结算政策参与，于是只改政策不动版本时解析身份仍变）。本记录倾向同样参与，但它改变解析标识的取值，落地时须与既有 `AT-PC-024`（相同输入与修订返回原解析语义）一并核。

## Open questions

**一、「网络使用资格」在 `party-commercial` 侧没有模型，而它不是服务产品形态。**

`docs/domain/party-commercial/CONTEXT.md` 全文没有这个词，只有「网络服务产品」这一种**服务形态**。该词只出现在 [CONTEXT-MAP](../domain/CONTEXT-MAP.md) 的 `party-commercial → network-routing` 边、`UC-NR-001`、`UC-NR-002` 与 `pn-02-w04`——**全是消费侧与关系侧的措辞，提供方从未接下过这个概念**。

它与服务产品形态是两件事，四处出处都把两者并列：`UC-NR-002` 步骤 4「判断**产品形态**、责任法人、合同约束和**网络使用资格**」；`UC-NR-001` 层次 1「**服务产品**、责任法人、**网络使用资格**……」；`UC-NR-002`「商业资格」输入行把它与「服务产品与合同版本」分开列。最硬的一条在 `pn-02-w04` 条款 3：「产品、合同、责任法人、线路和**网络使用资格必须与 W01、W03 的同一版本及有效期间相容**」——**它有自己的版本与有效期间**，而形态是产品版本的内在属性，不会相对产品另有一个需要与之相容的有效期。

本记录因此**只解形态的可观察性，不声称解决了网络使用资格**。后者是一个具名的提供方缺口，需要先经领域建模进入 PC 的语言（按 AGENTS.md，改领域语言要改对应 `CONTEXT.md`），再谈端口。

**二、它是产品属性还是客户级授予，本轮不送评审。**

两种读法都讲得通：或是「这个服务由不由运营企业自己的网络履约」，或是「某客户在某合同下获准使用运营网络，可能还按区域收窄」。**判据现在不足，但也不阻塞**：首发期 `NetworkEligibility` 的`不要求`唯一来源是面单渠道服务形态，而它被 `PAR-COM-12` 排在枚举外，因此无论哪种读法都不改变任何首发行为。不阻塞的问题不占决策位，等它真阻塞时再拍。

一条已排除的读法可留作输入：PC CONTEXT 里唯一的客户级收窄机制叫**渠道约束**，而它在 CONTEXT-MAP 那条边上与网络使用资格**并列列出**，两者因此不是同一件。这只排掉了一种候选，不构成裁定。

## Alternatives considered

- **把「网络使用资格」等同于服务产品形态，据此宣布不存在缺口。** 否决：见 Open questions 一。四处出处一致并列，`pn-02-w04` 条款 3 更给了它自己的版本与有效期间。这是本记录起草过程中先得出又被证据推翻的结论，留在此处以免再次被得出。
- **新建一本「网络使用资格声明册」。** 否决：会在 PC 语言里造一个提供方从未定义的概念，且首发期它只有一个取值——那正是「不得把未确认参数写成生产默认」要拦的形状。先建模，后开册。
- **产品缺席时让解析退化为`无适用依据`（照抄 0044「光有版本没有政策 = 没有可用依据」）。** 否决：0044 那条只在请求结算依据时触发，而服务产品几乎每个闭包都解析，退化会改变 `parcel-shipment` 既有接受闭包的结果。缺席由消费方按依赖不可用处置即可，不必动解析判定。
- **只在 `AdoptedBasis` 上带形态值而不带整个 `ServiceProduct`。** 否决：与 0034/0044 携带整个政策对象不一致，且产品下一次追加属性时要再改一次闭包形状。

## Links

- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)：第一个实例（价格政策）
- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md)：第二个实例（结算政策），镜像先例
- [ADR-0042](./0042-acceptance-content-declarations-by-owning-object.md)：内容只在唯一选出之后按已选对象读取
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器与全函数翻译
- [UC-NR-001](../application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md) / [UC-NR-002](../application/network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md)：两个消费端口的出处
