# 14 BUY 评价采用不了之后请求卡死，补登卡上取值也不回补

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md) 的隔离评审。
Blocked by: 无
出处：[ADR-0200](../../../docs/adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 越权风险点 3；SELL 那一路同一格见 [ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 越权风险点 3。

## 现象

BUY 评价不可计价、待判断、跨币种没带换算步骤或卡没声明金额取整时，结算入账、不形成成本。之后租户补登了卡上的取整声明或汇率序列，同一组来源引用再请求会命中原请求，评价也不会重来，这件发生项永远没有预期成本。SELL 那一路的请求同样卡死。

## 三类

机制（续办路径）。规模 M。

## 最小的诚实修法

照 [ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 的续办做法：参数登记之后发一类「参数已登记」信封，由请求侧按自然键另起一份新请求（新请求标识，旧请求留作历史），不改写旧评价。BUY 与 SELL 两路共用同一个续办形状。

## 要裁的

续办由谁触发（登记口同笔发信封，还是运营手动续办）；新请求怎样与旧请求关联（自然键加代次）。归 SA、PP owner。

## 既有票

none
