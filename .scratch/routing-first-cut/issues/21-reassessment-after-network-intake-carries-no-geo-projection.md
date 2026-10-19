# 21 收寄后的路由复核不带地理投影：关务适用性答两端未知，复核停在看不清

Category: bug
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权），分支 `claude-rfc21`。出自演示动线重走（[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)），停点取证记在 [product-strategy-boundary/32](../../product-strategy-boundary/issues/32-demo-rule-package-declares-no-unprovable-intake-qualification.md) 的完成记录里。
Blocked by: 无
地盘：`internal/parcelshipment/ports` 的网络收寄发布意图；`internal/parcelshipment/application` 的采用编排；`internal/parcelshipment/adapters/postgres` 的两份发布载荷；`internal/networkrouting/adapters/inbox` 的两份译码；`internal/networkrouting/adapters/parcelshipment` 的复核触发；`internal/networkrouting/application` 的复核编排；UC-NR-003 输入语义契约一行。
出处：[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md) 决定一、三；[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)。

## 现状（实测，证据 `S`）

- 演示委托初始路由 `ROUTE_FORMED` 之后，经收寄口收寄、PS 采用落行。
- 采用信封 `parcel-shipment.network-intake.recorded` 交路由复核，答 `consumer stalled on its own dependency: … route reassessment is undecided: PLAN_REVIEW_INCONCLUSIVE`。三次后落 `ABANDONED`。
- 成因（读码，钉 `45b9a088`）：
  - 复核编排取证据时交的是空的携带内容。
  - 关务适用性按候选两端国家问，两端取自携带的地理投影。没带，关务答状态未知。
  - 计划适用性复核遇到所选候选的硬约束状态未知，就答未决。
- 初始路由那条链没有这个问题：接受决定信封带着地理投影（ADR-0075 决定一），采用信封只带采用键。

## 裁定（代决）

复核与初始路由一样，按随请求携带的地理投影取证据。投影由 PS 随采用的发布意图带上，取采用所对那一版提交的投影。

- 不在 NR 留存投影：ADR-0075 决定三只许留引用与摘要。
- 不让 NR 回读 PS 地址：ADR-0075 决定一。
- 投影进复核触发的指纹，理由同初始路由的交接指纹：同一触发换了地址就不是重放。
- 信封形状两口同一份：PS 侧一个载荷类型，NR 侧一个译码类型、一个投影译法。

## 完成判据

- 采用的发布意图与载荷带上那一版提交的投影；没携带时载荷里没有这一格。
- NR 译码把它交到复核取证据那一步；同一触发换了投影答触发冲突。
- 演示实走：采用之后复核越过 `PLAN_REVIEW_INCONCLUSIVE`。证据只记 `S`，走完删库。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-rfc21` 基 `45b9a088`）

- PS：
  - `NetworkIntakeHandoffIntent.Geo`。采用编排在取回委托之后按命令指名的提交版本取投影（`carriedGeoOf`），新采用、不采用、更正、重放各路都交同一份。
  - 载荷 `geo` 一格与接受决定信封共用 `carriedGeoPayload`；没携带即缺席。
- NR：
  - 译码共用 `carriedGeoBody`，`AdoptedNetworkIntake.Geo`。
  - 复核触发适配器把它译成携带内容（与初始路由共用 `geoFromCarried`）。
  - `ReassessRouteCommand.Carried` 交给 `LoadInitialRouteEvidence`，并进 `triggerDigest`。
- 文档：UC-NR-003 输入语义契约加「客户地理投影」一行。
- 用例：
  - PS 采用编排：意图带那一版的投影，新采用与重放各一份。
  - PS 真库：载荷带投影、没携带时缺席。
  - NR 真库消费门：信封上的投影原样到处理方，旧信封照常译码。
  - NR 复核适配器：携带内容到取证据那一步。
  - NR 复核编排：取证据用携带的投影；同一触发换投影答触发冲突。
