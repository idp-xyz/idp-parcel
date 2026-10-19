# 66 交付门禁：有效交付读 FINAL_DELIVERY 准入并照记

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)、[65](./65-handover-records-an-outbound-release-gate-reading.md)
地盘：TF `application/register_effective_delivery.go`、新 `application/read_delivery_gate.go`、新 `adapters/customscompliance/delivery_gate.go`；`adapters/postgres/effective_delivery.go` + TF 迁移（认领时现取）、tfhttp 交付响应；`cmd/parcel-api/assemble_delivery.go`；文档 UC-TF-006 与 AT。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」有效交付的 `FINAL_DELIVERY` 门禁；方案「代决」C14，「需要的 ADR」ADR-0213。

## 做什么

- 有效交付按交付意图问 CC，节点取对象进入该派送段那次交接的交出节点，读数照记；节点不可得的答法照方案 C14。
- 读数形状复用 65 的内核。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 交付节点取进派送段那次交接的交出节点。
- 不受门禁、阻断、未决、读不动各一格，均照登。
- 凭收寄进段或未声明节点：记未决「交付节点不可得」。
- PS 终局照常。
- 真库测试。
- 演示：有效交付 → 201 `DELIVERY_REGISTERED`，`gateReading` 为 `NOT_GUARDED`；PS 终局照常形成。

## 规模

M
