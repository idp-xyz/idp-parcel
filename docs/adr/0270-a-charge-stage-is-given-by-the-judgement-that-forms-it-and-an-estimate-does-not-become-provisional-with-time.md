# ADR-0270：费用阶段由形成它的那一步给出，不随服务发生自动推进；包裹计费时点那一路形成的客户费用停在预估，直到确认

Status: Accepted（2026-10-21，Claude Code 会话代 SA owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/09，出处审计 [audit/sa-cr.md](../../.scratch/docs-consistency/audit/sa-cr.md) L8。回答 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 越权风险点 7，无条款停用：相抵的是 settlement-accounting CONTEXT「预估 → 暂估」一条的读法，代码与 ADR-0190 一致。不改代码。越权风险点见文末，待 SA owner 复核。）<!-- 索引注记 -->｜**阶段是结论**：预估与暂估都是形成费用的那一步按它的依据给出的结论，不是到了某个时点自动迁入的状态；形成之后只有「→ 确认」与追加版本。**暂估要一次判断**：服务已经发生或责任基础已经形成、只差确认依据时，形成那一路才可以给出暂估。**包裹计费时点**：那一路形成预估，不作暂估判断，费用停在预估直到确认；首发没有形成暂估的路
Date: 2026-10-21

## Context

- settlement-accounting CONTEXT「费用明细」生命周期写：「预估 → 暂估：服务已经发生或责任基础已经形成，但仍缺少允许最终确认的计量、履约、账单或其他依据。」读成状态迁移，就是时点一到预估就该变成暂估。
- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 越权风险点 7：「两种时点都在责任起点或之后，照 settlement-accounting `CONTEXT` 的生命周期已够得上暂估。本记录沿 ADR-0169 决定二只形成预估客户费用，预估到暂估的转移不在本记录。」2026-10-20 复核记录维持这一点。
- 代码：`domain.FormCustomerCharge` 在形成时收阶段，只收 `ChargeEstimated` 与 `ChargeProvisional`；`CustomerCharge.Confirm` 从任一阶段确认；领域里没有预估转暂估的方法。形成客户费用的编排只有 `application.FormSellCustomerChargeHandler`，它交 `ChargeEstimated`。
- 包裹计费时点那一路在没有实测时用申报测量（ADR-0190 越权风险点 1），金额依据本身可以是声明值；同一件包裹在不同计费时点上的依据完整程度不同，时点不能单独说明金额已够得上暂估。

## Decision

**一、阶段是形成费用那一步的结论。**（机制）

- 预估与暂估都由形成费用的那一步按它采用的依据给出；形成之后的去向只有：满足确认条件时「→ 确认」，或由唯一创建用例追加新版本、调整。
- 预估不因服务已经发生、责任起点已到或计费时点到来自动转为暂估。

**二、暂估要一次判断。**（产品策略）

- 形成那一路判定服务已经发生或责任基础已经形成、金额所依的计量与履约已是有效实际事实、只差允许最终确认的依据时，才给出暂估。
- 首发没有作这次判断的形成路。

**三、包裹计费时点那一路形成预估。**（产品策略，ADR-0190 已定的那一格）

- 这一路不作暂估判断，形成的客户费用停在预估，直到确认判断定格；预估口径的经营毛利照旧采用它。

## 候选与反方

- **包裹计费时点那一路直接形成暂估。** 否决：没有实测时用申报的那几件，金额依据是声明值，标暂估是假话；要分得出就得在形成时判依据完整程度，那正是首发没建的那次判断。
- **建一只预估转暂估的编排，时点到了就转。** 否决：转移靠时点就是把 CONTEXT 那条读成状态迁移；暂估说的是依据，不是时间。

## 对已接受记录的作用

- 回答 ADR-0190 越权风险点 7「形成的是预估，不建预估到暂估的转移」：首发不建转移，暂估是形成时的一次判断（本记录决定一、二），无条款停用。[ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 决定二「已完成的 SELL 评价形成预估客户费用。」不变。

## Consequences

- settlement-accounting CONTEXT「费用明细」生命周期的「预估 → 暂估」一条改写为一次形成判断，并写明预估不随时点自动转为暂估；UC-SA-002「生命周期与控制分支」的费用明细一条同此补一句。
- 不改代码。

## 越权风险点

1. **服务已经完成的包裹，费用仍标预估**（SA owner）。交付已终局之后形成的客户费用在确认之前一直是预估；读面与报表不能把「预估」读成「服务尚未发生」。
2. **首发没有暂估**（SA owner）。`ChargeProvisional` 有形成门、没有调用方；要用暂估的租户形态（例如按实测出暂估账）须另立形成路。

## Links

- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)：越权风险点 7 由本记录作答
- [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md)：决定二形成预估客户费用
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「费用明细」生命周期
- 票 `.scratch/docs-consistency/issues/09-sa-cr-docs.md`
