# 59 权威交接声明交出、接收、去向节点

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（TF 规范化令牌排在 [21](./21-load-assignment-allocates-against-capacity-reservations.md) 之后）
地盘：TF `domain/transport_handover.go`、`handover_rehydration.go`、`payload_canonicalization.go`（交接出新形状版本）；`application/register_transport_handover.go`（命令三格，更正承继）；`adapters/postgres/transport_handover_registry.go` + TF 迁移（认领时现取）；tfhttp `register_transport_handover.go`、`query_transport_fulfillment_records.go`（交接册列三格）；`cmd/parcel-api` 新真库用例文件；文档 ADR-0213、TF CONTEXT「权威运输交接」词条一句、UC-TF-005 输入与 AT、`docs/design/synthetic-demo-journey-script.md`。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「复核」04「票面漏掉的真阻断」，「代决」C1，「需要的 ADR」ADR-0213。

## 做什么

- 交接登记加三个可选声明：交出节点、接收节点、去向节点；保存、摘要、更正与缺席的答法照方案 C1。
- 文档：起草 ADR-0213（要点见方案「需要的 ADR」，C13、C14 两条由 [65](./65-handover-records-an-outbound-release-gate-reading.md)、[66](./66-effective-delivery-records-a-final-delivery-gate-reading.md) 落，ADR 里写成决定并标明后继票）。
- 进 main 的令牌次序与波次见方案「波次与令牌」。

## 完成判据

- 三格缺席照登，答复不变。
- 同内容重放答 `EXISTING_VERSION`，含新形状之前入库的行。
- 同键换节点答 `SOURCE_CONFLICT`。
- 更正承继前版的节点声明。
- 查阅口 `registry=transport-handover` 回显三格。
- 真库测试。
- 演示：SHA 收寄后 `POST /transport-fulfillment/handovers` 带 `releasingNode=SYN-NODE-SHA-HUB` 答 201 `HANDOVER_REGISTERED`；原样重发答 200 `EXISTING_VERSION`；改 `receivingNode` 重发答 200 `SOURCE_CONFLICT`；查阅口列出三格。

## 规模

M
