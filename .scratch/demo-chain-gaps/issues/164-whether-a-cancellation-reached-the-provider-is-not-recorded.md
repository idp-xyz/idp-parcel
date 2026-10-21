# 164 「取消送达服务方」没有登记：取消时刻早于段的开始时，结果随采用与取消的登记先后而异

Category: enhancement
Status: needs-triage——2026-10-21 票 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；代决复核 M9 问题二）。出自 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定三「采用与取消的先后」末条、越权风险点 8 后半与 [ADR-0232](../../../docs/adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 越权风险点 3。
Blocked by: 无

## 现象

- 取消时刻早于段的开始时：先登记采用，委托已开始、取消答已开始；先登记取消，委托已取消、采用答不可采用。两边都不撤回已答出去的东西，差别落在一件本上下文没有登记的事实上——取消有没有在运输开始之前送达服务方。送达了，服务方本不该开始，不可采用是对的；没送达，运输照常开始，已开始是对的。

## 要裁的

- 要不要登记「取消送达服务方」这一事实（谁报、凭什么证据、业务时间怎样取），用它裁取消时刻早于开始的那一处先后。
- 登记之前两种先后各自答出去的结果，登记之后要不要回头更正（与票 163 的取消更正、票 57 的解除采用怎样衔接）。
- 三类：送达事实的形状是机制；送达凭什么证据算数可能是租户取值（与外部服务方的约定）。

## 规模

M
