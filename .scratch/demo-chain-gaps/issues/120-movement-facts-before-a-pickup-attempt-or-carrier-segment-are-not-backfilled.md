# 120 揽收执行口与承运商首次有效收寄立的段上，先于段登记的移动事实仍进不了全程追踪

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-20 票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0224](../../../docs/adr/0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 决定七与越权风险点 2、6。
Blocked by: 无
地盘：VE 补派生（`internal/visibilityexception/adapters/transportfulfillment/backfill_movement_facts.go`）、VE 消费门（`internal/visibilityexception/adapters/inbox`）、派发路由（`cmd/parcel-dispatch/routes_*.go`）；若要执行口另交对象级信封，另及 TF 揽收执行编排（`internal/transportfulfillment/application/perform_offsite_pickup.go`）与它的交出口。

## 现象（读码钉票 53 的分支，只记 `S`）

- 票 53 起，对象凭`已交接`或对象级场外揽收（`offsite-pickup.registered`）进段时，全程追踪按段找回声明了这一段的在册移动事实补派生（ADR-0224）。
- 段还会由揽收执行那一口立起（`PerformOffsitePickupHandler.enterSegments`，交的是尝试级 `offsite-pickup.formed`）。全程追踪不认这类信封（揽收消费门头注说对象级登记信封「已覆盖同批事实」，但执行口不交对象级信封），派发路由上也没有它的 VE 那一路：执行口收寄的对象今天既不投影收寄，也不补派生先于段登记的出发。比本票更大的缺口是前半句。
- 段也由承运商首次有效收寄立起（`EstablishSegmentWithCarrierPickup`），它的信封只进小包托运。ADR-0224 判为不适用：本产品的移动事实来自自营作业事实口，外部承运商的轨迹走外部追踪事实；但移动事实声明这种段时不被拒，若某个租户的接入真这样登，那一份出发照丢。

## 三类

- 机制：补派生的判据、幂等键与处理方不变（`BackfillMovementFactsAdapter`），只多唤醒的入口；认「这一次收寄让对象进了哪一段」已有 `EnteredByPickup`，承运商收寄要一条同形的领域门。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 执行口：让它逐成功对象另交对象级登记信封（全程追踪与小包托运都按对象级消费），还是全程追踪按 ADR-0066 在消费侧展开尝试级信封。前者同时补上执行口的收寄投影。
- 承运商段：是否在移动事实登记那一侧拒收声明了承运商段的出发（把「不适用」从约定变成门），还是接上补派生。

## 既有票

- [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（交接与对象级揽收那一半）
