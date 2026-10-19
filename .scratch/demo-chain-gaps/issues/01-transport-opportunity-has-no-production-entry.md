# 01 运输机会与班次没有生产入口：路由形成后运输履约不知道有包裹要运

Category: enhancement
Status: 拆分——2026-10-19 拆成 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)、[17](./17-capacity-pools-are-registered.md)、[18](./18-transport-preparation-consumes-the-initial-route.md)，后继 [34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)、[36](./36-tf-authorized-re-preparation-request-face.md)，代决见 [tf-chain-plan](../tf-chain-plan.md)。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【硬停】UC-TF-003 运输机会/班次/容量没有任何生产入口：路由形成后运输履约不知道有包裹要运，也形不成机会

## 证据（读码）

cmd/parcel-dispatch/assemble.go 路由表里，`network-routing.initial-route.formed` 只登了 `veinbox.InitialRouteFormedEventType: veInitialRouteRouted`；`veInitialRouteUndecidedSentinels` 注释写「应消费方还有 NO/TF，但两侧消费者今天不存在」。`tfapp.NewPrepareTransportOpportunityHandler` 在 cmd/ 零引用；cmd/parcel-api/endpoints.go 和 internal/transportfulfillment/adapters/http 都没有班次、容量池、预占端点；`accessidentity` 的 DecisionKind 没有班次/容量这一格。领域 `domain.TransportScheduleSpec` 只有 Schedule/Direction/DepartsAt，`EstablishScheduleCommand` 不收计划履约段引用，也不收需求量快照，所以 UC-TF-003「计划履约段 → 运输机会」这一步没有建模。cmd/ 下没有运输履约的登记 CLI，scripts/demo-seeds/data 下也没有 transport 目录。

## 三类

机制与产品策略为主，另有租户取值。规模 L。

## 最小的诚实修法

机制：机会/班次命令改收 NR `PlannedLegReference` 拼写的计划履约段引用和需求量快照，没有合格班次时答「无当前运输机会」，不建占位班次。入口二选一：parcel-dispatch 加一个运输履约侧的 initial-route.formed 消费者，逐段形成准备结果；或新增运营决定种类与端点。照 ADR-0049 决定三，消费者就位之前不登路由。租户取值：给班次/容量池开登记口，演示租户登 SYN- 班次与容量；PAR-NET-08/12 的生产值保持未配置。

## 要裁的

运输履约是自动消费初始路由形成机会，还是只由运营准备请求触发（UC-TF-003 两种触发都列了）；路由指令怎样把段引用带到运输履约（ADR-0131 越权点 4）；班次模板与日历归运输履约登记，还是随网络参考配置发布（产品策略还是租户取值）。

## 既有票

none（psb/09 第 6 项只管缩容/超配处置）
