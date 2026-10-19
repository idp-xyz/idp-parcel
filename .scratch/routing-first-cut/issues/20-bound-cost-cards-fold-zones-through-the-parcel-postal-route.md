# 20 绑了分区目录的成本卡：候选成本经包裹邮编路线与 parcel-pricing 读数补全取区

Category: enhancement
Status: ready-for-agent——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决」授权），ADR-0189 决定四拆出的那一步。
Blocked by: 无（[18](./18-leg-endpoints-folded-into-price-card-zones.md) 已落段计价区域那一路）
出处：[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一第一条、决定四；ADR-0152 决定三（估价与正式评价同一份读数补全）。

## 现状（读码，钉 `9063ce06`）

- `RouteCandidateCostAdapter.evaluateLegs` 直接调 `ppdomain.EvaluatePricingAcrossPlans`，不经 parcel-pricing 共用的读数补全：绑了分区目录的卡读不到目录读数，恒答 `ZONE_UNRESOLVED`（待判断）；按序列换算的卡同理（`Series` 未接）。
- NR 在初始路由请求上携带了寄件、收件两端的国家与邮编（ADR-0075，`CreateInitialRouteCommand.Carried.Geo`），但只交给证据视图，成本取数侧拿不到。

## 做什么

1. 把携带的邮编路线交进成本取数侧（`LoadCandidateCosts` 或判断键的携带内容），`LegZone` 带上 `Postal`；不新开读 parcel-shipment 地址的口（ADR-0075）。
2. 逐段评价改走 parcel-pricing 的读数补全（目录读数与序列读数），与正式 BUY 评价、受理前估价同一份实现；不在 NR 侧另写一份补全。
3. 生产装配 `routeCosts` 接上目录与序列的在用版本解析器（同 `preAcceptanceTrials` 的接法）。

## 完成判据

- 真库用例：一张绑了分区目录的 BUY 成本卡，经包裹邮编路线读出区域后出价；没绑目录、也没登段计价区域的卡仍待判断；要按序列换算的卡在序列有在用版本时出价。
- 未绑目录的卡照旧取段计价区域（票 18 的用例不变）。
