# 69 设备出发前门禁问询

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（装配令牌排在 [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md) 之后）
地盘：TF 新 `application/preview_departure_gate.go`（复用 `DepartureGateReader`）、新 tfhttp `preview_departure_gate.go`；新 `cmd/parcel-api/assemble_departure_gate_preview.go` + `endpoints.go` 一行；文档 ADR-0219、UC-TF-005 AT。不改 `internal/accessidentity`。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」设备出发前门禁读口；方案「代决」C17，「需要的 ADR」ADR-0219。

## 做什么

- 作业事实渠道开只读问询，交回与出发登记同一套读数；TF 不落记录，授予与记录照方案 C17。
- 文档：起草 ADR-0219。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 答复与随后同参出发的读数同格。
- TF 零写入；CC 判断册照记。
- 未授予答 403。
- 真库测试。
- 演示：出发前问询 → 200，读数 `NOT_GUARDED`；随即出发 → 201，读数相同。

## 规模

M
