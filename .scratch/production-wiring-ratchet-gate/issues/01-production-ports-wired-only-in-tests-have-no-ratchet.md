# 只在测试里接线的生产端口没有棘轮，接线退化与新增未接线口都不会红

Category: enhancement
Status: ready-for-agent

来源：T3 分区键碰撞票（[partition-key-space-collision 票 01](../../partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)）
第三问取证的副产物，基线 `9e5c5c0`。本票**不解**那张票，两者判据不同。

## 形状

**测试自己补上了生产缺的那半根线。** 某个生产端口的全部构造调用点都在 `_test.go` 里：适配器
自测与集成夹具各自手工接线，用例因此永远绿——但它们证的是「若接上则可用」，不是「已接上」。
生产进程里那根线不存在，而没有任何东西会因此变红。

两个已知实例，成因不同而形状相同：

- **TF 两口零生产装配**（本票来源）：`NewOutboxTransportHandoverRegistrationHandoff` 与
  `NewOutboxEffectiveDeliveryHandoff` 在全仓非测试代码里零构造调用点，只出现在各自适配器自测与
  `cmd/parcel-dispatch` 的集成夹具。分区键碰撞票据此把「今天就在发作」写成了事实，实际不可达。
- **PS←PG 桥恒答未解析**（syn-wall-door-audit 票 02，MCP-5 报）：某读口没实现，`nil` 被归进
  「显式未配置」，整座桥恒答未解析，四十一条用例全绿一条照不到。

## 现状：普查数字（对 `9e5c5c0`）

`cmd/parcel-dispatch/assemble.go` 是全仓唯一装配 outbox 交接口的地方——`cmd` 下非测试代码里
`Handoff` 只出现在这一个文件，`cmd/parcel-api`、`cmd/parcel-commercial`、
`cmd/parcel-pricing-register` 三个进程零处。它装配五口，六个调用点：

- `nrpostgres.NewOutboxInitialRouteHandoff`
- `vepostgres.NewOutboxCustomerViewHandoff`（两处）
- `vepostgres.NewOutboxProjectionHandoff`
- `pspostgres.NewOutboxFinalOutcomeHandoff`
- `pspostgres.NewOutboxNetworkIntakeHandoff`

而 `internal/` 下共 **46** 个 `NewOutbox*Handoff` 构造函数。**41 个在全仓非测试代码里零调用点。**

**这 41 个不是 41 个缺陷。** 它们绝大多数是 SYN-WALL-DOOR-AUDIT 十八墙里还没建门的口，属**缺席**
（门还没建）而非**在场且错**。这条界线要写进门禁注释，否则下一个人会把清单长度当成待修工量。

## 要建的

一条纯句法门禁，形状照抄 `internal/architecture/envelope_partition_gate_test.go` 的
`allowedSameExpression`：中心例外清单 + 可复核的判据前缀 + **只许变短** + 一条证明门禁真能红的
自测。

判据：某个生产端口的构造函数，其全部调用点是否都在 `_test.go` 里。

**能力上现成，不需要类型信息**：`parseRepositorySources` 已经在遍历全仓 `.go` 并**跳过
`_test.go`**——那正好是这条门禁要的那一半，非测试调用点是否存在只需 AST。

## 为什么它不是一张「先放着」清单——救它的是「只许变短」

第一天 41 行例外，看起来正是 `envelope_partition_gate_test.go` 点名否掉的那种清单。**换个叫法
救不了它**：叫「进度盘点」而它仍然永远不会红，那就是同一份文件里那句「一个从来不会失败的门禁
比没有门禁更坑人，因为它还会取信」。

真正救它的是**只许变短**这一条，加上之后 41 行不是清单，是一把**棘轮**。三种真红，且今天一种
都没人守：

1. **新增第 47 个零生产调用点的交接口** → 不在清单里 → 红。新来者拿不到例外，与
   `allowedSameExpression` 同一口径。
2. **某口从已接线退回未接线**（装配点被删或改掉） → 红。这一种今天完全无人守：`assemble.go`
   删掉一行接线，全仓依旧全绿。
3. **接上一口却忘了从清单删** → 红。与现有门禁后半段同一机制。

十八墙重核变成派生的（今天由人工每轮重核，如 `ee58c1a` / `d6453a7` 那一轮）是**附赠收益，不是
建它的理由**。按棘轮建，不按盘点建。

## 边界

- **本票不接任何一根线。** 只建门禁与清单；把某口从缺席改成在场是各自那张墙票的事。
- 清单每行按 `allowedSameExpression` 的口径带一句**可复核的判据**（这一口在等哪张票 / 哪堵墙），
  不写「无害」这类复核不了的结论。
- 不改 `cmd/parcel-dispatch/assemble.go`。
- 端口族先划清扫描范围：本票**只扫 outbox 交接口**（`NewOutbox*Handoff`）。扩到全部 ports 适配器
  是后续，范围一次放太大会让第一版清单失去可读性。

## 参照

`internal/architecture/envelope_partition_gate_test.go`（`allowedSameExpression`、
`TestEveryExceptionCarriesACheckableVerdict`、`TestTheEnvelopePartitionGateCanActuallyCatchAViolation`）；
`internal/architecture/rehydration_gate_test.go` 的 `parseRepositorySources`；
`.scratch/syn-wall-door-audit/report.md` 十八墙清单。
