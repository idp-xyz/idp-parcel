# ADR-0034: 计价闭包经商业价格政策采用，结果带回方向与定价方案绑定

Status: Accepted  
Date: 2026-08-12

## Context

[ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md) 已让 `ResolveCommercialPricePolicy` 按方向匹配并在采用前问清价卡存续（`AT-PC-036`）。单元级 `AT-PC-035` 也已钉在同一函数上。

但 `ResolveCommercialBasis` / `ResolveCommercialClosure` 仍按「范围 + 对象类型」直接采用一份 `PriceRule` 的 `CommercialVersion`，**从不调用**价格政策解析，`AdoptedBasis` 也带不出方向与定价方案绑定。于是同一范围下 SELL 与 BUY 可以共用同一份价卡版本身份——键指纹不同只保证解析标识不同，**不保证**采用了各自的政策与绑定。验收原文要的是后者（`AT-PC-035` / UC-PC-002）。

## Decision

**一、计价目的下的价格规则走政策解析。** 当解析键 `Purpose = PricingPurpose` 且必需依据含 `PriceRuleObject` 时，采用路径经 `ResolveCommercialPricePolicy`（登记册持有的政策集合 + 键上的方向/范围/锚点），不再只挑一份 `CommercialVersion`。

**二、成功结果必须可观察方向与定价方案绑定。** `AdoptedBasis`（及单依据 `Resolution`）在采用价格政策时携带该政策；调用方读 `Direction()` / `PricingPlan()`，不得再从「只有版本」反推。

**三、价卡存续答复仍是入参。** 沿用 ADR-0032：`PricingPlanStandingLookup` 传入解析函数；本上下文不查、不猜。缺答复 → `解析未决`，原因 `BoundPlanNotConfirmed`；已退役 → 原因 `BoundPlanWithdrawn`。两者都**不是** `无适用依据`（权威没说这个范围没有政策）。

**四、非计价目的下的价格规则对象仍可走版本选用。** `AcceptanceControlPurpose` 等带着 `PriceRuleObject` 的既有键不被迫造方向；本记录不扩大它们的语义。

## Consequences

- `ResolveCommercialBasis` / `ResolveCommercialClosure` / 两处提交前校验的签名多一个 `PricingPlanStandingLookup` 参数；既有非计价调用点传 `nil` 即可。
- `CommercialRegistry` 可登记 `CommercialPricePolicy`；只登记版本、不登记政策时，计价目的下的价格规则会落到 `无适用依据`——这是有意的：没有政策就没有可观察的方向与绑定。
- 新增两个 `ResolutionReason`；须补 `String()`（架构枚举门禁会守）。
- **不改** `NoApplicableBasis` 与「冲突压过缺项」的既有形状与排序。

## Alternatives considered

- **只改解析标识、仍只采用版本。** 否决：标识不同回答不了「各自绑定了哪份方案」。
- **让 `CommercialVersion` 自己带方向与方案引用。** 否决：与既有「方向与绑定在政策上、不在版本上」的分工冲突，且会把非计价用途的价格规则对象一并拖进来。
- **standing 挂在登记册字段上。** 否决：它不是权威视图的一部分，是跨上下文当次答复；入参形状与 ADR-0032 一致。

## Links

- [ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md)：价卡存续入参与不可用分格
- [UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)：`AT-PC-035`
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：BUY/SELL/INTERNAL 分别表达
