# TF 的（租户+对象）分区与 VE 的（租户+包裹）分区是同一个键，跨上下文共链无人察觉

Category: bug
Status: needs-triage

分区由**键值字符串**决定，不由上下文、事件类型或 handoff 决定（[票 01](../../outbox-partition-key/issues/01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md)
的判据原句）。两个上下文各自算出同一个字符串，它们的信封就进同一分区并因此互相排队——包括
互相**阻塞**。今天已有两口这样：

| 口 | PartitionKey |
|---|---|
| `transportfulfillment/.../transport_handover_registration_handoff.go` | `租户/对象`（`transportHandoverPartitionKey`） |
| `transportfulfillment/.../effective_delivery_handoff.go` | `租户/对象`（`effectiveDeliveryPartitionKey`） |
| `visibilityexception/.../projection_handoff.go` | `租户/包裹`（`Projection.Parcel()`） |
| `visibilityexception/.../triage_handoff.go`、`visibility_gap_handoff.go`、`eta_handoff.go` | 同为 `租户/包裹` |

而**载运对象引用与申报包裹标识是同一个字符串**——`cmd/parcel-dispatch` 的揽收采用链里，
`offsitePickupObject` 既用来构造 `tfdomain.NewCarriedObjectReference`，也用来构造
`psdomain.NewDeclaredParcelID` 反查已接受委托。所以 TF 那两口今天就与 VE 四口共分区。

## 实测（2026-08-21，基线 `f47f698`）

`outbox-partition-key` 票 03 的裁断二本打算把 `offsite_pickup_registration_handoff.go` 的分区
主体也收窄到（租户+对象）。收窄后 `TestARegisteredOffsitePickupStopsAtUnprovenIntakeEligibility`
立刻红在重拍那句：

```
offsite_pickup_adoption_test.go:81: 重拍定稿 0 条, want 1（仅派生信封经视图链定稿）
```

把分区键换成（租户+对象+一个探针后缀）——仍是一对象一键，只是避开 VE 的键空间——同一用例
立刻转绿。**起因因此确切：不是揽收登记自身的顺序问题，是它并进了 VE 的包裹分区。**

后果的形状：一封停在 `dispatch.consumer_undecided` 的揽收信封会把同一包裹**已经由 VE 受理并
派生**的追踪投影一起堵在分区头，直到失败预算耗尽进 `ABANDONED` 才解冻。而「硬资格未证明」
是今天的常态（实例半边为空），不是罕见边缘。

## 为什么这一类没有任何东西在守

`internal/architecture/envelope_partition_gate_test.go` 拦的是「ID 与 `PartitionKey` 同源」这个
**句法**形状。跨上下文算出同一个键值是**语义**形状：两处代码各自合规、各自的用例全绿，只有
在两条链恰好同时有信封在途、且一条卡住时才看得见。票 03 末尾「一条门禁守不住、清单也不收的」
说的是键拼得太细那一半；这里是**同一个盲区的另一半——拼得太粗、跨上下文撞车**。

## 要答的

1. TF 的「载运对象」与 VE 的「包裹」在分区意义上是不是同一个排队主体？是，则共链是特性，
   头端阻塞要按特性接受并写进权威文档；不是，则键里需要上下文/流别维，两边一起改。
2. 若判为不同主体：`transportHandoverPartitionKey` 与 `effectiveDeliveryPartitionKey` 两口
   要不要跟着加维？它们今天已经与 VE 共链，只是还没有用例把它照出来。
3. 这一类要不要一条门禁。判据不是句法，可能得靠「同一 SHA 下全仓分区键表达求值后取交集」
   这类扫描，成本与收益要先估。

## 已经定死的边界（不要在本票里重开）

- `offsite_pickup_registration_handoff.go` 的分区键本轮取（租户+对象+类型段），
  理由与实测见票 03 的裁断落地 Comment 与该函数注释。本票若判 1 为「同一主体」，回来把类型段
  去掉即可；判为「不同主体」，则本口已经是对的形状，改的是另外两口。
- 同一对象两次成功揽收必须保序这一条已裁定，不因本票重开（`AT-TF-098`）。
