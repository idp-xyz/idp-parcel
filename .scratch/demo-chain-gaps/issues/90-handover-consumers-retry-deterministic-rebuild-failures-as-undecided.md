# 90 交接与已接受委托的消费侧适配器把坏行重建失败当成未决重投，烧完预算才停

Category: bug
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [63](./63-network-routing-reassesses-on-a-handover-into-a-node.md) 的隔离评审。
Blocked by: 无
出处：[ADR-0215](../../../docs/adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定四「停与走」；票 63「评审记录」。

## 现象

消费侧适配器读提供方仓储时，凡读口报错一律折进本路的「读不着」哨兵，再登进未决名单重投。读口报的错里混着两种：

- 可见性滞后与连接故障：重投会变，进未决名单是对的。
- 坏行重建失败：库里那一行过不了领域构造，重投永远一样。它也被折成未决，按派发预算重投到耗尽才弃置，运维看到的是「等依赖」而不是「数据坏了」。交接信封上它还占着每对象的交接分区头，牵连同一对象之后的追踪投影与派送发起（ADR-0215 越权风险点 10）。

## 证据（读码）

- `internal/transportfulfillment/adapters/postgres/transport_handover_registry.go` 的 `FindByKey` 在 `rebuildHandover` 失败时原样返回错误，没有单独的哨兵。
- `internal/parcelshipment/adapters/postgres/current_accepted_parcel_target.go` 的 `currentAcceptedTargetFrom` 失败、委托聚合重建失败，同样没有单独的哨兵。
- 消费侧：`internal/networkrouting/adapters/transportfulfillment/reassess_on_handover.go` 把 `FindByKey` 的错误以 `%w: %v` 折进 `ErrHandoverNotVisible`；`internal/networkrouting/adapters/parcelshipment/accepted_carried_geo.go` 把反查与聚合读的错误折进 `ErrAcceptedParcelUnreadable`；`internal/visibilityexception/adapters/transportfulfillment/derive_on_transport_handover.go` 的 `ErrHandoverNotVisible` 是同一折法。别的读交接登记册的消费者照此逐个盘。

## 三类

机制（确定性错误与未决怎样分）。规模 S–M，跨 TF、PS 两个提供方仓储与读它们的各个消费侧适配器。

## 最小的诚实修法

提供方仓储把「行在、重建失败」做成自己的哨兵（例如 `ErrHandoverRowCorrupt`），与读口故障分开。消费侧适配器遇到它折进本路的「记录自相矛盾」类错误（如 `ErrHandoverRecordInconsistent`、`ErrAcceptedRequestInconsistent`），不进未决名单，响亮失败。逐个消费者补一格用例：坏行一投即停、不重投。

## 要裁的

无产品取舍。提供方哨兵的命名与是否和既有「不变量已破」类合并，归 TF、PS owner。

## 既有票

none
