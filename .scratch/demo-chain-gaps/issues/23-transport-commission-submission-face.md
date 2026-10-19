# 23 运输委托提交面

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md)
地盘：`operation_decision.go`；tfhttp `operator_decision_intake.go`、新 `submit_transport_commission.go`；`application/commission_transport.go`（核对、法人快照、改注释）；`domain/transport_commission.go`、`commission_rehydration.go`；`postgres/transport_commission.go`；迁移 TF 0025（法人列）；新 `cmd/parcel-api/assemble_transport_commission.go` + `endpoints.go` 行；种子；ADR-0202、UC-TF-004 AT、演示文档。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-1、D02-3、D02-4、D02-7，「跨票通则」X1、X3，「需要的 ADR」ADR-0202。

## 做什么

- 运营决定种类 `TRANSPORT_COMMISSION_SUBMISSION`（方案 D02-3）。
- 委托提交经 [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md) 的适配器核协议，否定格与未决格照方案 D02-4；采购责任法人从协议版本快照到委托（新列），不由调用方给。
- 接上 `OutboxTransportCommissionHandoff`，路由表照 ADR-0066 不登记；改掉 handler 与端口注释里「交 SA 作成本预期源」的过时说法（方案 D02-1、X3）。
- 演示干线外包写进演示文档（方案 X1）。
- 起草 ADR-0202（要点见方案「需要的 ADR」）；UC-TF-004 补 AT 行。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 四个否定格加未决格。
- 委托与 Outbox 同一笔（真库）。
- 路由不登，`no_subscriber` 如实可见。
- 演示：干线委托答 `SUBMITTED`；服务方填末端承运商答 `PROVIDER_MISMATCH`。

## 规模

M
