# 04 交接之后节点控制不转移，路由也收不到复核触发

Category: enhancement
Status: 拆分——2026-10-19 拆成 [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)、[61](./61-node-control-transfers-out-on-an-authoritative-handover.md)、[62](./62-node-control-is-established-by-handover-in.md)、[63](./63-network-routing-reassesses-on-a-handover-into-a-node.md)，代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。此前 needs-triage——2026-10-19 会话 idp-parcel-9b 已交还，归本会话；待 TF 链第一波后接（见 [tf-chain-plan](../tf-chain-plan.md)「等 peer 的」）。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【硬停·跨上下文】交接登记之后，节点控制不转出/不转入，网络与路由也不复核：交接信封只投 VE

## 证据（读码）

dispatch 路由表 `veinbox.TransportHandoverRegisteredEventType: veHandoverRouted` 只投 VE，`veHandoverUndecidedSentinels` 注释写「NO/NR 控制转移不在本票」。NO 的 `ControlTransferAdapter.TransferOut`（internal/nodeoperations/adapters/transportfulfillment）非测试调用为零，见 .scratch/no-control-transfer-out/report.md Q1；UC-NO-001/002/003 都把权威交接排除在外，没有用例承接。NR 已能处理 `domain.TransportHandoverControl` 这种触发（reassess_route.go），但只经 PS 收寄采用（nrparcelshipment/reassess_on_intake.go）进来，运输履约的交接信封到不了 NR。演示后果：交接之后节点作业仍记 SHA-HUB 持有控制；SZX-GATE、SIN-HUB 的承运方→节点交接建立不了节点控制（UC-NO-002 明禁冒充客户送站）。

## 三类

机制。规模 L。

## 最小的诚实修法

新立 UC-NO：按权威交接「已交接」推进节点控制转出与转入。NO 编排照勘察报告 Q4 表做；parcel-dispatch 把交接信封同时投给 NO（与 VE 分 inbox 名、各列未决哨兵）。NR 加一个按交接触发复核的消费者，触发种类用 TransportHandoverControl。

## 要裁的

由 NO owner 定：交接对象（包裹/集运单元）与作业实物 HandlingUnitID 怎样对上身份（报告 Q3）；是否新立 UC-NO-004；NR 是否对每次交接都复核。

## 既有票

none（只有勘察报告 .scratch/no-control-transfer-out/report.md）
