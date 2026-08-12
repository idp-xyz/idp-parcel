# ADR-0036: 发布前必须确认正文指名引用已发布，未确认不得建立生产引用

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-005`：「合同引用尚未发布的规则包 → 合同发布未决，不建立悬空生产引用。」

[ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md) 已在**解析闭包**侧用 `namedReferencesConfirmed` 挡住「合同已在册、却采用了正文未指名的规则包」（`AT-PC-022`）。它**不**挡住发布：今天 `Publish` 从不看 `References`，一份草稿合同可以指名一份从未发布的规则包并成功变成 `PUBLISHED`，悬空生产引用就此成立——解析侧永远看不见「发布本就不该成功」这件事。

两道闸门回答不同问题：005 问「能不能发布」；022 问「已经发布的闭包有没有采用错对象」。一道不能代替另一道。

## Decision

**一、`Publish` 在固定正文之前，必须确认每一个正文指名引用此刻已发布。** 答复由入参 `NamedReferenceStandingLookup` 带入（与 ADR-0032 / ADR-0035 同判据：本上下文不猜登记册外的事实）。零值 = 未确认。

**二、未确认或明确未发布 → 独立哨兵 `ErrNamedReferenceNotPublished`。** 不得压成 Incomplete 或 ApprovalRoleNotConfirmed。草稿与导入来源原样保留。

**三、无指名引用的版本不询问 lookup。** lookup 为 `nil` 且存在指名引用 → 视为未确认（安全方向）。

**四、不改弱 ADR-0032。** 解析侧确认继续存在；本记录只补发布闸门。

## Consequences

- 全仓 `Publish` 签名多一个 lookup；无引用的调用点可传 `nil`。
- AT-PC-005 由红转绿；与 AT-PC-022 对照用例必须同时存在。

## Alternatives considered

- **只靠解析侧 022。** 否决：悬空引用已经以 `PUBLISHED` 身份进入生产集合。
- **Publish 直接收 `*CommercialRegistry`。** 否决：版本值对象不该依赖登记册；入参 lookup 与跨上下文 standing 同形。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-005`
- [ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md)：解析侧确认
- [ADR-0035](./0035-publication-approval-role-is-separate-from-incomplete-basis.md)：发布未决分格先例
