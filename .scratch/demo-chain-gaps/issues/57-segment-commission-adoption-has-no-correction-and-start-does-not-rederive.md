# 57 段采用委托没有更正与解除面，首个参与被更正后委托的开始也不重派生

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md) 完成记录与 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 1、2。
Blocked by: 用户决定（TF owner 复核 ADR-0204 风险点 1、2：采用错了怎么改，「已开始」能不能回退）

## 现象

- 采用册只有插入（迁移 TF 0035）。指错了段或委托，同段再交答 `ADOPTION_CONFLICT`，没有任何一口改得了；委托一经采用就标了开始，也就取消不了。
- 采用把委托的开始依据与时刻取自段的首个当前参与，此后那条参与被来源更正替代或改判失效（ADR-0112），委托上的开始与连线上的开始都不随之重派生。极端情形：段上的控制事实全部撤回，委托仍记着「已开始」。

## 要裁的

- 解除或更正采用是不是一种运营决定（新种类），还是由控制事实的更正驱动。
- 「已开始」能不能回退：TF CONTEXT 只说已成立的实际履约段不存在「取消回未开始」，委托的开始没有同样明文；回退后此前「开始之前提出」的订舱判据随之变化。
- 首个参与被更正时，委托的开始按新的首个参与重派生，还是只在连线上记新版本、委托上保留首次。

## 三类

机制；解除的授权粒度属产品角色模型（psb/07）。
