# 65 交出门禁：已交接逐对象读交出准入并照记

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)
地盘：TF 新 `domain/handover_gate_reading.go`（与出发读数同一封闭集，可抽共用内核，出发那一格的答复字节不变）、新 `ports/handover_gate.go`、新 `application/read_handover_gate.go`、新 `adapters/customscompliance/handover_gate.go`；`application/register_transport_handover.go`（读数随记录）、`adapters/postgres/transport_handover_registry.go` + TF 迁移（认领时现取）、tfhttp 交接响应；`cmd/parcel-api/assemble_control_facts.go`（写事务之前读）；文档 UC-TF-005「当前限制」行与 AT、ADR-0206 Links 加后继指针。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」外包干线交接的跨境门禁；方案「代决」C13，「需要的 ADR」ADR-0213。

## 做什么

- 交接登记对 `已交接` 逐对象按交出意图问 CC，读数随交接同笔照记，不拒收；未声明交出节点、未声明去向的答法照方案 C13。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 同区：不受门禁。跨区无案：未决。阻断：照样 201。
- 未声明交出节点：不问 CC，记未决。
- CC 读不动：照登，记读不动。
- 重投回显首次读数，不重问。
- 真库测试。
- 演示：SZX-GATE 交给干线承运商（`releasingNode=SYN-NODE-SZX-GATE`、`onwardNode=SYN-NODE-SIN-HUB`）→ 201 `HANDOVER_REGISTERED`，`gateReading` 为未决，依据是跨关务区域移动无案。

## 规模

M
