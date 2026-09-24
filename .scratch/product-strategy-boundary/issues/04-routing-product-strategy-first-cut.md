# 04 路由第一刀：拆 `PAR-NET-14`，路由策略族与首个内置策略

Category: enhancement
Status: in-progress——2026-09-24 通道 5 认领（单 task-d12bb120-aac3-40b4-b1e9-a018b77afddc），分支 `mcp5-psb04` 基 `64b37f27`。此前：ready-for-agent（演示网络参考配置那半 Blocked by 03）
Blocked by: 03（只挡演示网络参考配置那半）
地盘：network-routing 的领域、应用与取数侧适配器；参数登记册 `PAR-NET-14` 一行的拆分结论。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；票 `nr-route-evidence-views/01`（needs-info，其重启条件由本票替换）。

## 做什么

1. 把 `PAR-NET-14` 拆成产品策略与租户取值，拆分结论写回登记册那一行（与 02 同口径）。
2. 路由策略族：候选生成、硬约束过滤、排序、冻结边界与已执行前缀的判定、并发裁决、自动 / 人工改路的判断结构，落在 network-routing。
3. 首个内置排序策略沿 `PAR-NET-16` 已确认的「满足硬约束后按成本单维择优」；并列照旧交人工。
4. 把版本化网络目录折成逐候选事实的取数侧接上 `NetworkEvidenceView` 与 `InitialRouteEvidenceView`；目录为空或未采用策略时照旧答`未配置`。
5. 演示网络作为参考配置，经 03 的采用路径进入演示租户（这半等 03）。

## 不做

- 不定任何租户的阈值、日历、截单或权限分派；不预选某个租户用哪种排序策略。

## 完成判据

- 演示租户上一票已接受的委托能形成初始路由，不再停在路由证据未配置；`nr-route-evidence-views/01` 与被它挡住的两张 blocked 票各有去处。
