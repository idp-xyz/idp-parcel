# 18 段起讫折成价卡区域的折法（ADR-0148 越权风险点 5）

Category: enhancement
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话代决并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权），裁定见「裁定」与 ADR-0189，分支 `claude-rfc18`；完成记录见文末。此前 needs-info——2026-10-10 22:2x 通道 1 自 [17](17-initial-route-pricing-input-from-customer-declaration.md) 分诊拆出（用户授权自决）；待 PP owner 或用户在「待定」两种形态间拍板，通道 1 不代裁
Blocked by: 无（「待定」已由 ADR-0189 代决）
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

## 裁定（2026-10-18，Claude Code 会话，用户授权代决；权威是 [ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)，此处只摘要）

两种候选形态都不照原样采：

- **按卡给**：绑了分区目录的卡由包裹邮编路线经目录读出区域（与正式评价同一口径，归 [19](./19-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)）；没绑的卡取租户按线路段登记的**段计价区域**。
- **不随线路版本内容走**：候选一「段依据行带区域」放进线路版本，已采用、区间未闭的演示线路就改不了，所以段计价区域另立一本册，只挂在有成本依据的段上。
- **不采候选二的节点邮编**。

三类与越权风险点见 ADR-0189。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-rfc18` 基 `c783151b`）

- 段计价区域登记册：迁移 `network_routing/0019`；`NetworkCatalogRegistry.RegisterLegPricingZones` 与 `NetworkCatalog.LoadLegPricingZones`；登记用例拒因 `LEG_ZONE_MALFORMED`、`LEG_ZONE_WITHOUT_COST_BASIS`；受控登记口 `parcel-network-register -kind leg-pricing-zone`。
- 成本取数侧：`RegisteredLegZones` 实现 `LegZoneSource`，`cmd/parcel-dispatch` 的 `routeCosts` 换掉恒答未配置的那一只。
- 演示租户：种子 `22-leg-pricing-zones-cn-sg-v1.json` 三段各登 `Z1`，`seed.sh` 在线路之后调登记口。
- 文档：ADR-0189、NR CONTEXT「段计价区域」、ADR-0148 Links 前向指针、ADR 索引一行。
- 用例：真库 `TestLegPricingZonesRegisterOnlyOnSegmentsWithACostBasis`；登记门 `TestLegPricingZonesRegisterOnlyWellFormedRowsOnCostBasedSegments`；`TestRegisteredLegZonesHandTheTenantsZoneThroughVerbatim`；生产装配 `TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring`（2.5 KG 三段各 85 CNY，合 25500 minor，三条出处）。
- 实走（`S`）：2026-10-19 在一次性库 `c8_walk18` 上照演示动线「前置」灌种子（`leg-pricing-zone: REGISTERED`）、起 `parcel-api` 与 `parcel-dispatch`，提交 `SHR-7IPK7KPV75KVZTE7ZSIQMKHHOM`。`acceptance-decision.formed` 一拍 `PUBLISHED`；`network_routing.initial_route` 一行 `ROUTE_FORMED`，计划 `RPV-000000000001` 三段带时间窗口，交接记录一行。动线越过 `CANDIDATE_COSTS_PENDING`。走完已删库。
