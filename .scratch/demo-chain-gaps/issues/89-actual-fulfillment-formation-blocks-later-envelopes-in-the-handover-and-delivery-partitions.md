# 89 实际履约发生项自动形成停住时，挡同一对象后面的交接与交付信封：计费在分区这一层挡了履约与追踪

Category: enhancement
Status: needs-triage——2026-10-19 票 [67](./67-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 67 的隔离评审（非阻断项）。号取协调方给票 67 预分配的号段。
Blocked by: 无
出处：[ADR-0218](../../../docs/adr/0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 8；[ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md)（TF 对象链按口名段分区、头端阻塞「只有成本没有收益」的论证）。

## 现象

- 实际履约发生项的自动形成，挂在交接登记信封与有效交付登记信封的扇出上，排在各路最后（ADR-0218 决定一）。这两种信封各按（租户、对象、口名段）分区，Outbox 按分区只交出队头。
- 自动形成这一路停在未决，这封信就留在队头重投，直到成功或烧完失败预算落 `ABANDONED`。未决的来由如协议核对口读不动。响亮失败也一样，如旅程平局、连线指名的委托不在册、协议核对与委托走岔。
- 在它后面排着的，是同一对象后面的交接或交付信封。交接分区上，是下一次交接的追踪投影与派送发起（ADR-0216）；交付分区上，是 POD 更正那一封的包裹终局与追踪投影。这些都跟着等。
- 扇出里殿后，只保住同一封信上的各路不被它挡。`cmd/parcel-dispatch/assemble.go` 路由表头注说「计费不挡履约与追踪」，到分区这一层不成立。
- 放大：票 [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)、[77](./77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md) 把确定性失败答成未决。在人工口上，登记方只是看到一次未决；在自动形成这一路上，会以 `consumer_undecided` 挂在履约分区的队头烧失败预算。

## 证据（读码）

- 分区键：`internal/transportfulfillment/adapters/postgres` 的 `transportHandoverPartitionKey`、`effectiveDeliveryPartitionKey`。
- 队头认领：bento `postgres/outbox` 的 `loadActiveHead`。
- 扇出：`internal/platform/dispatch` 的 `FanOut`：每一路都调，任一路失败整封重投，已提交的那路靠 inbox 跳过。
- 自动形成的处置：`internal/transportfulfillment/application` 的 `FormActualFulfillmentResult.Disposition`；两份未决名单在 `cmd/parcel-dispatch` 的 `actualFulfillmentOnHandoverUndecidedSentinels`、`actualFulfillmentOnDeliveryUndecidedSentinels`。

## 三类

机制（派发的排队主体与转交形状）。规模 M。

## 要裁的

- **挪到自己的分区上**，两种形状：
  - **转交**：扇出上那一路在 inbox 事务里只交出一封自家的「参与已结束、待形成」信封，分区键带自己的口名段，然后入账；形成在那封信上跑。不动交接与交付编排。形状同 ADR-0208（已形成信封发起 BUY 请求）。
  - **门铃**：交接与交付编排同笔多交一封「参与已结束」门铃。要动物理编排的交出口，与在途的交接各票同一批文件。
  - 两种都要新信封类型、FingerprintEventID 与分区键门禁，派发多一类路由与它自己的未决名单。
- **维持现状**：凭运行证据判断。未决只在依赖读不动时出现，读得动就过；缺陷修好即放行。ADR-0216 派送发起那一路在同一张扇出上，是同一形状。
- 与 54、77 的先后：两票先落，「确定性失败以未决之名烧预算」这一格就没了，剩下的只有真读不动与真缺陷。
- 归 TF owner；路由表头注那句的口径归派发装配的 owner。
