# ADR-0042: 接受内容声明按对象归属建模，解析后经独立只读端口读取

Status: Accepted  
Date: 2026-08-12

## Context

第一个跨上下文适配器（ADR-0025）要在第一阶段唯一解析成功后构造 `parcel-shipment` 的商业依据快照。快照需要三样 PC 拥有的声明，今天都未建模：

- **适用校验组**——CONTEXT：「接单规则包必须明确哪些硬规则通过后可自动接受」；UC-PS-001 把校验组适用性判给 `PC-RULE`，消费方不得代拟默认集合。
- **人工复核指令**——CONTEXT：「哪些条件确实需要人工业务复核。人工复核不是默认步骤」。既有 `ManualReviewRequirementFor(grants…)` 属授权治理（**谁有权**复核），不是规则包声明（**要不要**复核），两者不同物。
- **待路由许可**——UC-PS-001：「只有服务产品明确允许待路由并保留该商业依据时」（AT-PS-007）；归**服务产品**，不归规则包。

适配器只翻译不判断，三样不落地它就只能发明内容。

## Decision

**一、适用校验组与人工复核指令归接单规则包正文声明。** `DeclareAcceptanceRuleContent(rulePackage, groups, directive)` 要求规则包为 `已生效`（与 `DeclareAsOfPolicies` 同判据）；组集合非空不重复，指令必须明确 REQUIRED / NOT_REQUIRED。校验组是本上下文自己的封闭集合（先例：`JudgmentType`），取值随消费方实现的校验组增长。

**二、待路由许可归服务产品，且必须携带依据引用。** `DeclarePendingRoutingPermission(product, basis)` 要求产品 `已生效`；无依据的许可与一次默认放行分不开，不可表达。零值 = 未许可。

**三、读取面是独立只读端口 `AcceptanceContentDeclaration`，不塞进第一阶段 `Resolution`。** 与 `AsOfPolicyDeclaration` 同一分界：第一阶段用独立锚点选包/选产品，内容声明只在唯一选出之后按已选对象读取；塞进解析结果，声明就有机会参与决定它自己被谁采用。

**四、`found=false` 即实例未配置。** 本上下文不内置任何默认——不默认全组适用、不默认免复核、不默认许可待路由；未配置由消费方停在未决，不是放行。

## Consequences

- 适配器可按已采用规则包/产品读取三样声明并翻译成 PS 快照；名称按 UC-PS-001 校验组清单一一对应，翻译不发明。
- `ErrUnusableRulePackage` 语义从「不能声明 asOf」推广为「不能承载声明」，两处同判据同恢复动作。

## Alternatives considered

- **塞进第一阶段 Resolution。** 否决：见 Decision 三；且 Resolution 值对象随声明膨胀。
- **复用 `RuleCategory`（装配分区）表达适用校验组。** 否决：那是规则引用的归档分区（五类），不是下游校验组适用性（七组）；两个集合语义与基数都不同。
- **人工复核并进授权治理。** 否决：授权答「谁可以」，规则包答「要不要」；并格会让「有人有权复核」被读成「本单要复核」。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)、[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：`AT-PS-007`
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：适配器在消费侧
- [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)：两阶段分界先例
