# 17 初始路由的计价输入：预路由用客户声明，逐段折成价卡区域待 PP owner 定

Category: enhancement
Status: needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证
Blocked by: [15](15-candidate-identifier-minted-with-at-but-cost-adapter-splits-on-slash.md)——计价输入一接上就会撞那条缝
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它挡在 11 判据一的路上。
地盘：`internal/networkrouting` 的计价消费方适配器与 `cmd/parcel-dispatch` 的 `routeCosts` 装配；区域折法归 parcel-pricing。
出处：[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条与越权风险点 5；[10](10-candidate-cost-from-leg-buy-evaluations.md) 的完成记录（记为消费方实例半边）；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 10-09 重走「成本一格」。

## 现象（通道 2 实测，钉 rfc/11 的 `eb57f999` 与 `bdee59cd`）

- 生产装配的真库用例里，采用演示网络之后初始路由证据已配置，唯一合格候选是 `SYN-LINE-CN-SG-01@1`；成本取数侧答 `ErrRouteCostSourceNotConfigured`，编排形成未决 `COST_SOURCE_NOT_CONFIGURED`。
- 成本单维排序只有一个合格候选也要已计价（`RankRouteCandidates`）；dispatch 的 `routeCosts` 计价输入是 `unconfiguredRoutePricingInput{}`。

## 待分诊

- 先把机制半边与越权风险点 5 分开：NR 侧计价消费方适配器、预路由用客户声明（ADR-0148 决定四第 7 条）是机制，现在就能做；「逐段折成价卡区域」是越权风险点 5，归 PP owner。
- 越权风险点 5：用户已授权通道 1 自决，分诊时可代裁并标「越权风险点 · 待 PP owner 复核」，或拆出一张等裁的票。先核 [psb/14](../../product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md)（邮编前缀粒度与公开单元参考配置）是不是那一格的依据。
- 10 把这一格记为「消费方实例半边」：分诊先判这个归类准不准——适配器是机制，区域词汇与演示租户的价卡区域才是租户取值或产品策略。
