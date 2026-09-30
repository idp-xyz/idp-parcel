# ADR-0159：面单择优的接受时解析回指从接受决定上读

Status: Accepted（2026-09-30。本项是机制。[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做。」通道 2 补面单择优这一条，不造解析号，不填租户截点。）
Date: 2026-09-30

## Context

面单择优链的翻译器要一份「接受时商业解析回指」，用来填责任依据。生产装配若把这个源留空，查询上也没有委托或包裹，翻译器只能答未配置。

这一问发生在接受之后。权威已经在接受决定上：`CommercialResolutionReferenceFor` 只对已接受基线的成员交回决定里固定的解析标识。ADR-0064 对初始路由用的就是这一处，不另造「范围 → 解析」映射。面单择优同一形。

可达性那条链不能照搬。它发生在接受决定形成之前，闭包标识取本轮已采用的解析（ADR-0156），不是接受决定。

调用方在查询上自报一个解析号，就是第二套解析。

## Decision

**一、回指从接受决定上读，查询只带来源身份与声明包裹。**

路径固定为：`ChannelSelectionQuery` 带 `Shipment`（来源身份）与 `Parcel`（声明包裹）→ 生产装配直接装 `NewAcceptedDecisionResolution`，不留可替换的缝 → 按来源身份取委托 → `CommercialResolutionReferenceFor`。解析标识不是查询字段，也不进 `ChannelSelectionSubject`。

不建查询到解析的登记表，不代拟标识，不读采用解析那一行（那一行在决定形成前会被改记）。不改走 `CommercialResolutionReferenceView`：那个口按（租户，包裹）找任一已接受成员，不核来源身份。择优查询已经带着这份委托的身份，用包裹反查会接受另一份委托上的同名包裹。

**二、空身份、空包裹、查无此委托、成员未接受，是未形成。** 与源没装的未配置分开报。不把缺席读成一份默认解析。

**三、租户不一致不上抛成未配置。** 查询租户与来源身份不是同一个，是写坏的查询。

**四、本记录不解除后面的实例闸门。** 账号使用授权与供应商协议仍未配置。不填租户的渠道、价卡或账户。

**五、一份接受决定上的解析对基线内每个成员相同。** 查询点名其中一个成员来读。面单交易的覆盖清单是这笔交易盖哪些包裹，不参与回指，也不与查询上的成员互核。两者对不上是写坏的命令，留给触发面，不在读口里猜。

## Consequences

- 生产装配直接装接受决定读口，不留缝。约束、计价输入、BUY 价卡、账号使用授权、供应商协议仍按未配置装配。
- 没有接受决定的查询在翻译那一格停在未形成。源没装仍是未配置。
- 不改 ADR-0064 与 ADR-0156 的正文。初始路由与可达性的回指各守各的时点。

## Alternatives considered

- **查询上直接带解析标识。** 否决：调用方可以报一个决定上没有的号。
- **留一张范围到解析的登记表。** 否决：第二套解析。权威已经在接受决定上。
- **读采用解析那一行。** 否决：那一行在决定形成前会被改记，不是接受时固定下来的引用。
- **复用 `CommercialResolutionReferenceView`。** 否决：它按（租户，包裹）答，不看来源身份。同一包裹身份若落在另一份委托上，择优会读到别人的接受决定。

## 裁决能力边界

读过 ADR-0064 的初始路由回指、ADR-0156 的可达性回指、`CommercialResolutionReferenceFor` 的三格，以及面单择优翻译器问解析的顺序（账号使用授权、供应商协议、然后才是解析）。没读：结算账户登记册、账号使用授权与供应商协议的选法。

## 越权风险点

1. 查询多了来源身份与包裹，决定记录的对象引用仍只有范围与映射。归 PS owner 复核：这不是给已落库的择优决定补一列包裹。
2. 未接受或非成员答未形成，已接受却缺回指仍是错误。归 PS owner 复核：不要把装配缺陷收成未配置。

## Links

- [ADR-0064：初始路由适用性闭包标识从已接受解析标识回指](./0064-initial-route-applicability-closure-from-accepted-resolution.md)：同一处权威，另一条链
- [ADR-0156：可达性资格的闭包标识从本轮已采用的商业解析回指](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md)：接受之前的回指，不在本记录
- [ADR-0080：引用闭包先解合同再据以解结算政策](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：回指交回标识本身，不拆合同版本
- [ADR-0133：交付条件引用是该包裹所属委托接受时固定的商业解析回指](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)：按（租户，包裹）的窄读口不用于本链
