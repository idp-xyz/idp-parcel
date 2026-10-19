# 62 节点控制按权威运输交接转入 HANDOVER_IN（UC-NO-005 下半）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)、[61](./61-node-control-transfers-out-on-an-authoritative-handover.md)
地盘（**需与 idp-parcel-9b 协调**）：61 新立的同一批 NO 文件（`application/transfer_control_on_handover.go`、`adapters/postgres/physical_control_transition.go`、`adapters/postgres/review_catalogue_control_transition.go`、`domain/control_transition.go`）；UC-NO-005 下半；演示脚本。
与 idp-parcel-9b 的四个共享点同 [61](./61-node-control-transfers-out-on-an-authoritative-handover.md)：`reception.go` 的折叠挂点、NO 迁移号、查阅口注册行、NO CONTEXT「节点控制」与 UC-NO-005 的措辞（详见方案「与 idp-parcel-9b 的协调点」）。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「代决」C5、C6，「需要的 ADR」ADR-0214。

## 做什么

- `已交接` 且声明接收节点时，在该节点立 HANDOVER_IN；对不上作业实物的入账，不造待识别实物（方案 C6）。
- 转出时声明的交出节点与在身控制节点不符，照转并记信息格（方案 C5 后半）。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- `已交接` + 接收节点：该节点上立 HANDOVER_IN；随后以该节点为交出节点的交接把它转出。
- 没有关联作业实物、未声明接收节点：入账，不立。
- 声明的交出节点不符：照转，信息格有值。
- 真库测试。
- 演示：自营段交接进 SZX-GATE（`receivingNode=SYN-NODE-SZX-GATE`）→ `registry=physical-control` 出现 HANDOVER_IN@SZX-GATE；SZX-GATE 交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`）→ 转出；SIN-HUB 接收 → HANDOVER_IN@SIN-HUB。

## 规模

M
