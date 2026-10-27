# 99 交接范围汇总把同一范围下同一对象的两次首登各算一次

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-19 票 [95](./95-node-operations-reads-a-new-handover-under-the-same-scope-as-a-correction.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0220](../../../docs/adr/0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 越权风险点「TF 不拒同一（对象，范围）下另起的首登」。
Blocked by: 无
地盘：TF `domain/transport_handover.go` 的 `SummarizeHandovers`，或 `application/register_transport_handover.go` 首登那一步。

## 现象（读码，钉 `5f0aa66d`）

- 交接范围是「一次交接的共同范围（一车、一批、一次交出）」，整批结论只能由对象级结果派生。
- 首登口不问同一（对象，范围）下有没有别的版本：同一件包裹在同一范围下换个版本号再登一次，答 `201 HANDOVER_REGISTERED`，是另一次首登。演示动线重走（psb/05「重走（钉 `8197fa2f`，2026-10-19）」停点一）里交进深圳口岸与从深圳口岸交出都写了 `SZX`，就是这样。
- `SummarizeHandovers` 只按（对象，版本）折掉被回指的那一代，不按对象去重。上面那件包裹在 `SZX` 的汇总里算两个`已交接`，`GET /transport-fulfillment-handover-scope-summary?scope=SZX` 的计数比对象数多。
- 节点控制一侧已由票 95 修好（只按更正回指认链），不受这一格影响。

## 三类

- 机制：一件对象在一个交接范围里有几个结果，TF 只能有一种答法。两条路：
  - 首登口遇到同一（对象，范围）已有未被更正的版本时答一个指名结果（例如要求走更正口），不落册、不发信封；
  - 汇总按对象取一个结果（例如业务时间最晚、未被回指的那一版），并在答复里带出「同一对象多次首登」的计数。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 走哪一条。前一条改首登口语义，设备离线补登、把节点代码当范围填的租户会撞上拒绝；后一条要裁「取哪一版」。ADR-0220 决定四已裁节点控制一侧不依赖它，这里只管 TF 自己的汇总。
- 存量：演示库与开发库按一次性库用，生产没有存量。

## 规模

S
