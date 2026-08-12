# ADR-0035: 发布时「批准角色未确认」与「批准依据字段不全」分格

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-010`：「导入成功但批准角色未确认 → 保留来源，发布保持未决。」

今天 `Publish` 只认 `ApprovalBasis` 是否字段齐全（及发布时间是否早于批准）。缺字段时交回 `ErrIncompleteCommercialPublication`。**没有**「角色未确认」这一格：要么字段齐了就发布成功，要么字段不齐当 Incomplete——把 AT-PC-010 压进后一格，会让调用方去补已经齐全的字段，而真正缺的是角色确认。

两者的恢复动作不同：字段不全要补齐依据；角色未确认要等适用批准角色确认，草稿与来源必须原样保留。

## Decision

**一、`Publish` 另收批准角色存续答复。** 形状与 ADR-0032 的 `PricingPlanStandingLookup` 同判据：跨边界/未决事实由入参带入，本上下文不猜。取值 `ApprovalRoleStanding`：零值 = 未确认（安全方向），`Confirmed` = 已确认。

**二、角色未确认交回独立哨兵 `ErrApprovalRoleNotConfirmed`。** 不得 `Is` 成 `ErrIncompleteCommercialPublication`。草稿原样保留（调用方仍持有导入后的来源与正文）。

**三、字段不全仍走 `ErrIncompleteCommercialPublication`。** 判定顺序：先完备性（字段与时间），再角色确认——角色未确认不得掩盖字段不全。

## Consequences

- 全仓 `Publish` 调用点须传入角色存续；测试替身在「已确认路径」上显式传 `ApprovalRoleConfirmed`。
- AT-PC-010 由红转绿；与「字段不全」对照用例必须同时存在，防止两格再被压平。

## Alternatives considered

- **角色未确认也返回 Incomplete。** 否决：恢复动作不同，压平后调用方选错补救。
- **给商业版本加「待批准」状态。** 否决：CONTEXT 写明批准是发布的完备性条件而非独立状态；未决用哨兵表达即可，不必扩生命周期。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-010`
- [ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md)：入参带答复、零值不安全通过
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：草稿 → 已发布；批准是完备性条件
