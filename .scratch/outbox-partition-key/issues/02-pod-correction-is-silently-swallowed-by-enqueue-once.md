# 交付生效的 POD 更正会被 EnqueueOnce 静默吞掉，永远到不了下游

Category: bug
Status: resolved

自查 [01](./01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md) 时在自己
地盘里查出来的。它与 01 相邻但不是同一件事：01 是**顺序**问题（信封都发出去了，只是可能
乱序），这一张是**丢失**问题（第二份信封根本没入队）。

今天不显形，因为全仓还没有 `transport-fulfillment.effective-delivery.registered` 的消费者。
**派发接线那天它就会显形**，而且不报任何错。

## 证据链，三步

**一、意图身份不含版本。** `effective_delivery_handoff.go`：

```go
func effectiveDeliveryEventID(key ports.EffectiveDeliveryKey) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String() +
		"/effective-delivery"
}
```

`EffectiveDeliveryKey` 只有（租户+对象+尝试）三维，没有 `DeliveryResultVersion`。

**二、更正走的是同一个键。** `register_effective_delivery.go` 的 `Correct`：拿 `deliveryKey`
算出的键与首登完全相同，`CorrectProof` 换出新版本后构造的 `record.Key` 还是那一个，最后
照样调 `handler.handOff(ctx, record)`。

**三、`EnqueueOnce` 查到同 ID 就静默返回。** `internal/platform/outboxintent/enqueue_once.go`：

```go
if exists {
    return nil
}
```

**合起来：首登入队一份，更正算出同一个 `(source, event_id)`，于是第二份不入队，函数返回 nil，
编排看到的是「交接成功」。** 更正后的 POD 永远不会通知下游。

## 为什么载荷是指针式并不能救它

这几个 handoff 的载荷刻意只带键（`effectiveDeliveryPayload` 只有 tenant/object/attempt），
下游按键 `FindByKey` 读当前版本。这个设计本身没问题，而且**在更正发生于消费之前时它恰好
自愈**——消费者那一次读到的就是更正后的版本。

但反过来那一半不成立：**消费者若已经处理过首登信封（inbox 已入账），更正不产生新信封，
它就再也不会回来重读**。而「先登记、隔一段时间再更正 POD」正是这条链的常态用法
（`UC-TF-006` / `AT-TF-072`：POD 被更正或失效时保留原证据和判断，形成新版本）。

## 同一个上下文里的另一半，做法不同且不丢

`transport_handover_registration_handoff.go` 的 `TransportHandoverKey` **含 `Version`**，
所以更正版本算出的是另一个信封 ID，会照常入队。它因此不丢——但它落进另一个分区，于是
撞上 01 记的那个顺序问题（更正可能先于原判断送达）。

**两个更正入口、两种失败模式、同一个上下文**。这个对照本身值得留：它说明问题不在「更正」
这个概念上，而在「意图身份取什么维」这一个选择上，两处各选了一次，选法不同。

## 修法有两条，我不替领域拍

**A. 意图身份改取结果版本。** 让 `effectiveDeliveryEventID` 带上 `DeliveryResultVersion`，
与交接侧对齐。这更贴 [ADR-0043](../../../docs/adr/0043-publish-intent-claimed-by-result-identity.md)
的「意图由**结果标识**认领」——一次交付生效的结果标识是它的版本，不是它的键；按键认领等于
把首登与更正两个结果压成一个意图。
代价：落进 01 那个顺序问题（首登与更正分属两个分区）。所以 A 单独做完，问题从「丢失」
变成「可能乱序」——是改善但没完。

**B. 意图身份取版本，分区键取业务键。** 两者本就不必相同：信封 ID 管幂等（不重发同一份），
分区键管顺序（同一对象的先后拍排队）。`effective_delivery` 现在把两者设成了同一个字符串
（`PartitionKey: eventID`），这才是它同时踩中两个坑的原因。
拆开之后：ID 带版本 → 更正照常入队；分区键取 `tenant/object/attempt` → 首登与更正同分区
保序。**这一条同时解掉本票与 01 在这个 handoff 上的那一格。**

B 看起来更完整，但它改的是意图契约（下游拿到的信封 ID 语义变了），而且同一形状在全仓有
三十多处。**要不要按 B 统一，是跨地盘的决定，不该由我在自己那一格里先斩。**

## 顺带查清的：TF 另外七个不受影响

自查了本上下文全部九个 handoff，只有上述两个有第二次状态变化：

- `offsite_pickup_registration`：键无版本，但**没有更正入口**（`register_offsite_pickup.go`
  只有 `Register`，同键异内容答冲突且不落库），所以一个键只会有一份意图。
- `transport_commission`：委托有 `Replace`（取消/开始），但**那两条路不发布意图**
  （`commission_transport.go` 里 `handOff` 只在首次提交与重放两处调用）。这本身是另一个
  问题——取消从不通知 settlement-accounting——但那属「该不该发」，不属本票的「发了却丢」。
- `offsite_pickup`、`capacity_consumption`、`exception_journey`、`disposition_execution`、
  `regulatory_acceptance`：一个业务对象一份意图，无后续状态变化。

## 结果

按 **B** 修的（意图身份取版本、分区键取业务主体），因为人类已定「分区键现在改」，而 B
一次解掉本票与 01 在这一格上的两个问题。

- `effectiveDeliveryEventID` 加入 `DeliveryResultVersion`，更正因而自成一份信封。
- `PartitionKey` 改取 `租户/载运对象`，两代排同一个队。理由与交接登记那一格相同：一个
  对象的交付结果是一条链，下游据它形成终局判断，更正先于首登送达会让终局落在已被取代
  的那一版上。
- 载荷**未动**，仍只带键。指针式意图是有意的：版本进 ID 是为了让两份都入队，载荷要的是
  「去重读」而不是「这是第几版」。
- 版本缺席从此响亮报错而不是悄悄退化成旧行为。
- 已从 `internal/architecture/envelope_partition_gate_test.go` 的例外清单里删去本行——
  那份清单只许变短，修好却留着同样会红。

回归用例：`TestAPODCorrectionEnqueuesItsOwnEnvelopeInTheSamePartition`（两份都入队、且
同分区）。另有三处既有断言随 ID 形状更新。

## 建议的处置顺序（已按此执行）

1. 先定 01 里那张「按什么维分区」的表（要各地盘主人回答因果先后）。
2. 再定意图身份与分区键该不该拆开（本票的 B）。两件事定完，改动是同一笔。
3. **在接派发线之前改完。** 现在没有存量 PENDING、没有消费者，改动零迁移成本；接线之后
   改，要连带考虑已入队条目的分区归属。
