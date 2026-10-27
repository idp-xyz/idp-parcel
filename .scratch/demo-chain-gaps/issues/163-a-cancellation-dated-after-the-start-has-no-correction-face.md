# 163 取消晚于开始那一格没有更正面：委托停在已取消，段挂不上它

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；代决复核 M9 问题二）。出自 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 8 前半与 [ADR-0232](../../../docs/adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 越权风险点 3。
Blocked by: 无

## 现象

- 段已凭交接开始、采用还没登记时，委托照常取消得了（201）；之后的采用答 `COMMISSION_CANCELLED_AFTER_START`，并排回显取消时刻与段的首个参与，不落连线（ADR-0204 决定三「采用与取消的先后」）。续办写的是「更正取消或更正控制事实」，但取消没有更正面，委托停在已取消、段挂不上它，只能另起委托再采用。
- 票 57 立了解除采用，没有立取消的更正。

## 要裁的

- 取消晚于开始那一格由谁、凭什么更正：撤回取消让委托回到已提交、再被采用，还是直接改标已开始。
- 撤回之前已据取消停掉的订舱、预占怎么办（取消今天不往下游传，ADR-0204 越权风险点 9）。
- 是不是一种新的运营决定（照 ADR-0151 与 ADR-0232 解除采用的先例），授予与取消分开。
- 三类：更正的形状是机制；种类是产品角色模型。

## 规模

M
