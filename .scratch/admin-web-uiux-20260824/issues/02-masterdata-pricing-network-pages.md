# 02 主数据/计价/网络区 13 页对齐（MCP-8 → MCP-3 接手）

Category: enhancement
Status: in-progress

2026-08-24 17:2x MCP-3 领票：用户改派「UI/UX 余量由 3 号亲手全部完成」；8 号未开工
（其地盘页面自 16:01 后零写入），占号广播已发。

地盘：`apps/admin-web/src/pages/party/**`、`pages/pricing/**`、`pages/network/**`（共 13 页，各含 index.ts）。通用检查单与协作纪律见 ../spec.md，逐页执行。

## 页面清单与主责出处

- party（7 页）：group-legal-entities、business-parties、party-contracts、supplier-agreements、service-products、channel-product-catalog、commercial-policies → docs/domain/party-commercial/CONTEXT.md（supplier-agreements 的出处在 docs/domain/CONTEXT-MAP.md 的 party-commercial → transport-fulfillment 一节）
- pricing（3 页）：price-card-catalog、reference-series、pricing-evaluation → docs/domain/parcel-pricing/CONTEXT.md 与 ADR-0013
- network（3 页）：network-catalog、service-areas、route-plans → docs/domain/network-routing/CONTEXT.md

各页锚定的导航条目、主责与出处以 src/navigation.ts 的 moduleInfoById 为准（只读引用，不复制）。

## 两遍走法

- 第一遍（现在就能做，不依赖任何人）：检查单第 1–5 项。
- 第二遍（等 7 号广播「共享增量已落库」后，选做）：未配置态切换新结构、注释里备好的筛选字段实装进筛选槽。

## 完成

票面 Status: resolved + 提交 SHA 与验证结果；report_task 标 done；send_to_session 7 简报（每页一行：改了什么 / 为何无需改）。
