# 18 段起讫折成价卡区域的折法（ADR-0148 越权风险点 5）

Category: enhancement
Status: needs-info——2026-10-10 22:2x 通道 1 自 [17](17-initial-route-pricing-input-from-customer-declaration.md) 分诊拆出（用户授权自决）；待 PP owner 或用户在「待定」两种形态间拍板，通道 1 不代裁
Blocked by: 用户或 PP owner 的决定（见「待定」）
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它挡在 [11](11-demo-network-adopted-as-reference-configuration.md) 判据一的演示动线上——17 的机制半边落地之后，演示候选的逐段区域仍答`未配置`，候选缺成本依据。
地盘：定下形态之后再写（PP 的计价输入缝；或 PP 价卡分区目录加 NR 网络节点的邮编）。
出处：[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条与越权风险点 5；17 的「分诊裁定」；通道 5 只读取证（`task-2a6b94e0`）。

## 现状（读码，钉 main `1253c768`）

- 计价输入经 `PricingInputFor(ctx, key)` 一次判断只交一份快照，`EvaluatePricingAcrossPlans` 各段共用；17 把「逐段区域」拆成单独一口，本票定下之前答`未配置`。
- 演示卡 `SYN-PLAN-CN-SG-COST-01/v1` 未绑分区目录、只有 `Z1`，只能走调用方给分区；参考配置 `SYN-CN-SG@1` 三段同引这张卡。
- `NodeDefinitionVersion` 不带邮编。
- [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项（受理前财务控制 `Amounts` 的估价方法）的估价基也含调用方给的 `Zone`，与本票同题。

## 待定

候选形态（通道 5 所列，属推测，未实测）：

1. **段依据行带区域**：区域随段依据（线路段）登记，由租户登记时给出；NR 照登记取，PP 照收。
2. **价卡绑分区目录加邮编路线**：段起讫经 PP 的分区目录按邮编折成区域；中间段的起讫是网络节点，要节点带邮编，NR 今天没有。

拍板之后：选定的折法属产品策略（[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)），本票补地盘与完成判据后转 ready-for-agent；演示各段落哪个区是演示租户的租户取值，随种子灌 `SYN-` 值。宜与 psb/06 第 2 项一起定。
