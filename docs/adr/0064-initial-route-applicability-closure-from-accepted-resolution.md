# ADR-0064: 初始路由适用性闭包标识从已接受解析标识回指

Status: Accepted
Date: 2026-08-18

## Context

`assemble.go` 的 `NewRoutingApplicability(closures, nil)` 把 `RoutingClosureIdentity` 留空，适用性一律 `ErrServiceProductUnavailable`。这与 [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md) 改正前把 `AdoptedStageOwner` 装成未配置是同一句错：把「判断键上没有解析标识」读成「任何地方都取不到」。

判断键 `InitialRouteJudgmentKey` 不含商业解析标识，也不该含。解析标识不是路由判断的一维，往键里塞会让同一份接受、换一次解析就变成另一个判断身份。从判断键发明 `ClosureResolutionKey`、或再建一张「判断键 → RES」登记表，都是第二套解析。

权威已经在已接受委托上：[ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 留给消费方的 `Decision.Basis.ResolutionID()`，提供方按标识持有闭包。[ADR-0050](./0050-service-product-form-observable-through-the-resolution-closure.md) 的形态翻译表已经能从闭包读出服务产品；缺的是把这份标识交到 `RoutingApplicabilityView`，而不是再要一份实例半边映射。

`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）。本记录只改初始路由适用性这一条消费链。

## Decision

**一、适用性闭包标识的权威是已接受决定上的解析标识，作为命令附加字段，不是判断维。**

路径固定为：`RouteOnAcceptanceAdapter` 按来源身份取回已接受委托 → `AcceptanceDecision().Basis().ResolutionID()` 译成 network-routing 的引用类型 → `CreateInitialRouteCommand` 携带 → `RoutingApplicabilityView.AssessRoutingApplicability(key, resolution)` 与判断键并列传入 → 译成提供方 `ResolutionID` 与租户 → `CommercialResolutionView.LoadResolution` → 既有 `eligibilityFromClosure`（ADR-0050 翻译表不动）。

不把 `ResolutionID` 塞进 `InitialRouteJudgmentKey`，不从判断键发明 `ClosureResolutionKey`，不建判断键到 RES 的登记表，不拆快照 `RulePackage`。

**二、无决定或无快照是依赖不可用，不得静默跳过。** 信封声称已接受，权威却交不出接受决定或其解析标识时，适配器上抛未决，不把这次路由义务入账丢掉。

**三、`RoutingApplicability` 只持提供方只读口。** `NewRoutingApplicability(closures CommercialResolutionView)` 一个参数。删除对 `RoutingClosureIdentity` 与 `nil` 身份映射的依赖。`LoadResolution` `found=false` 答 `ErrServiceProductUnavailable`（提供方还没有这份闭包，SYN-V0 的 `SYN-RES-01` 在未种解析行时正是这一格）。租户与调用方不一致上抛，不折成未配置。闭包在场但未采用可观察服务产品，仍走 `eligibilityFromClosure` 的缺席即不可用（ADR-0050 第三条）。

**四、本记录不解除实例闸门。** 不默认适用性，不 INSERT 网络定义，不为变绿给 SYN-PC-SEED 补种服务产品。SYN-V0 不调 `seedSYNPCEligibility`：`LoadResolution(SYN-RES-01)` `found=false` 仍是真话，停点保持 `ROUTING_APPLICABILITY_UNAVAILABLE`。

## Consequences

- 生产装配 `acceptanceConsumer` 改为 `NewRoutingApplicability(closures)`。适用性未决从「映射未配置」变成「闭包行未写入或产品不可观察」——恢复动作仍是等商业解析落地，但不再假装缺一条判断键到 RES 的映射。
- 可达性那条链的 `ReachabilityClosureIdentity` 不变。
- ADR-0062 只管收寄采用规则版本；本记录只管初始路由适用性闭包标识。两条回指同一份 `ResolutionID`，翻译表各用各的。

## Alternatives considered

- **继续装配 `nil` `RoutingClosureIdentity`。** 否决：机制缺口，与 ADR-0062 改正前同一句错。
- **把解析标识并进判断键。** 否决：解析不是路由判断维；换标识会拆开同一接受基线下的结果身份。
- **从判断键拼 `ClosureResolutionKey` 或另表登记。** 否决：第二套解析，且范围维属实例。
- **闭包未采用服务产品时默认要求网络判断。** 否决：ADR-0050 第三条，缺席即不可用。
- **为 SYN-V0 变绿去种服务产品。** 否决：那是实例半边；未种时 `found=false` 是真话。

## Links

- [ADR-0062：采用规则版本从已接受解析标识回指](./0062-adopted-stage-owner-from-accepted-resolution.md)：同一份标识，收寄采用与本记录各取各的闭包内容
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)
- [ADR-0050：服务产品形态随解析闭包可观察](./0050-service-product-form-observable-through-the-resolution-closure.md)：翻译表不动
- [ADR-0003：集团租户边界](./0003-group-tenant-legal-entity-customer-account.md)
- [ADR-0025：跨上下文适配器落在消费方](./0025-cross-context-adapters-live-on-the-consumer-side.md)
- [UC-NR-001：形成初始路由](../application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md)
