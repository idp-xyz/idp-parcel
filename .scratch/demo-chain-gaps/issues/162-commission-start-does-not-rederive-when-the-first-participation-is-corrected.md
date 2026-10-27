# 162 首个参与被更正之后，委托的开始与连线上的开始都不重新派生

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；代决复核 M9 问题二：其余三问拆成 needs-triage 子票，57 只留解除采用）。出自 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 1 与 [ADR-0232](../../../docs/adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 越权风险点 1。
Blocked by: 无

## 现象

- 采用把委托的开始依据与时刻取自已采用各段首个当前参与里最早的那一条，连线上也记着这一段在委托下的开始（ADR-0204 决定二）。此后那条参与被来源更正替代或改判失效（ADR-0112），委托上的开始与连线上的开始都不随之重新派生，也不后移。极端情形：段上的控制事实全部撤回，委托仍记着「已开始」。
- 票 57 的解除采用按其余各段**采用上记的**开始重新派生（ADR-0232 决定二），不回头按段上此刻的参与重算；那些开始若已因控制事实更正而过时，派生出的也过时。

## 要裁的

- 首个参与被更正时，委托的开始按新的首个参与重新派生，还是只在连线上记新版本、委托上保留首次。
- 重新派生若让开始后移，与 UC-TF-004「已发生事实不回退」的关系：开始依据被更正撤回，是不是就不再是「已发生事实」。
- 段上的控制事实全部撤回时，委托撤回到已提交，还是保持已开始等人处置。
- 触发：参与更正（ADR-0112 的替代 / 失效版本）落册时同笔判，还是另起一封信。
- 三类：重新派生的判法是机制；何时允许后移是产品策略。

## 规模

M
