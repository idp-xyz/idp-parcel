# 111 实际履约发生项自动形成只按 `TRANSPORT-HANDOVER/<版本>` 找结束的参与：同一包裹两次交接用同一版本号，交接信封整封弃置

Category: bug
Status: needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `76686b7b`，2026-10-19）」停点一）。
Blocked by: 无
地盘：TF `application/form_actual_fulfillment_on_participation_end.go`（`formFor`）；若改依据拼法，另及 `domain/transport_handover.go`（`TransferOutBasis`）、`domain/actual_fulfillment_segment.go`（`ParticipationEndedBy`）与存量迁移。与 [89](./89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md) 相邻。
出处：[ADR-0218](../../../docs/adr/0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 找参与那一步「多于一条：库面不一致，响亮失败」；[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记停点 `ENTRY_HANDOVER_AMBIGUOUS`（交付门禁那一侧已认「入场依据只带版本、不带范围」，先按业务时间再分）；演示动线票 95、98 补记给的交接载荷（每次交接各起 `scope`，`version` 写 `v1`）。

## 现象（实测，钉 `76686b7b`，只记 `S`）

演示包裹 `DPCL-6NWOLEGPYLZ5VDROK2UL2Z52BU`（一次性库，走完已删）照动线补记登五次`已交接`，范围与版本依次是：

- `SHA`/`v1`：上海交给自营干线，立自营段；
- `SZX`/`v1`：交进深圳口岸，结束自营段上的参与；
- `SZX`/`v2`：交给干线承运商，立干线段，段采用干线委托；
- `SIN`/`v1`：承运商交进新加坡枢纽，结束干线段上的参与；
- `SINLM`/`v1`：交给自营尾程，立派送段，没结束任何参与（`NO_ACTIVE_PARTICIPATION`）。

段登记册上，自营段那条参与的入场与终点依据都是 `TRANSPORT-HANDOVER/v1`，干线段那条入场 `TRANSPORT-HANDOVER/v2`、终点 `TRANSPORT-HANDOVER/v1`。

- `SIN`/`v1` 的交接信封：消费者 `transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover` 三次都报 `ErrParticipationEndedInSeveralSegments`（`basis TRANSPORT-HANDOVER/v1` 对上两段），信封落 `ABANDONED` / `dispatch.publish_failed`。同一封信的另外几个消费者（NO、NR、VE、派送发起）各自 `PROCESSED`。干线段的 `ACTUAL_FULFILLMENT` 没形成，BUY 请求也没发。
- `SINLM`/`v1` 的交接信封：它没结束任何参与，照样对上那两段，同一错误，同样 `ABANDONED`。
- 绕法：配置员经 `POST /transport-fulfillment-charge-occurrence-registrations` 对干线段人工登 `ACTUAL_FULFILLMENT`，答 `201`；随后 BUY 请求、计价 `COMPLETED`、供应商预期成本一行依次形成。

对照：`8197fa2f` 那次重走每次交接的版本串各不相同（`H2/v1`、`H4/v1` 等），同一消费者形成正常，交接信封都 `PUBLISHED`。

## 成因（读码）

- 参与的入场与终点依据都是 `TransportHandover.TransferOutBasis` 拼的 `TRANSPORT-HANDOVER/<版本>`，不带范围。
- `FormActualFulfillmentOnParticipationEndHandler.formFor` 列出对象有过参与的各段，数终点种类与依据串都对上的参与；多于一条就报上面那个错，ADR-0218 把这一格当库面不一致。演示这组数据却是合法的：两次交接范围不同、版本串相同。
- 别处已经认了这种歧义：`FulfillmentParticipation.EnteredByHandover` 另比业务时间；交付门禁读入场交接时先按业务时间再分，仍多于一版才答 `ENTRY_HANDOVER_AMBIGUOUS`。`formFor` 没有这一步。
- 这个错误是确定性的，重投不会变，信封照重投预算烧完才弃置。

## 三类

- 机制：两条路——
  - `formFor` 照 `EnteredByHandover` 与交付门禁的做法，按这次交接的业务时间与参与的终点时刻再分；仍多于一条时答一个指名的结果（未决或入账），不整封失败；
  - 参与依据带上范围（例如 `TRANSPORT-HANDOVER/<范围>/<版本>` 一类拼法），连同存量迁移；NO 的转出引用用的是同一个串，要一并看。
- 产品策略：无。
- 租户取值：无。

## 完成判据

- 同一包裹两次结束参与的交接用同一版本号时，各自形成发生项或各自答不形成，交接信封 `PUBLISHED`；没结束任何参与的交接答「没结束参与」。
- 真库用例照上面那组范围与版本的次序复现；证据记 `S`。

## 要裁的

- 走哪条路。第二条牵动 NO 的转出引用（ADR-0214 越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」），规模更大。
- 本票落地之前，动线文档的交接载荷示例要不要先改成「同一包裹的各次交接版本串各不相同」。句子列在 psb/05 那一节的停点建议里。

## 规模

S–M
