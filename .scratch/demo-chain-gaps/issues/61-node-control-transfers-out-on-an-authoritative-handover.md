# 61 节点控制按权威运输交接转出（UC-NO-005 上半）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘（**需与 idp-parcel-9b 协调**）：NO 新 `domain/control_transition.go`、新 `ports/physical_control.go`（不改 `ports.go`）、新 `application/transfer_control_on_handover.go`、新 `adapters/postgres/physical_control_transition.go` + NO 迁移（认领时现取）、新查阅册 `adapters/postgres/review_catalogue_control_transition.go`、新 `adapters/inbox/transport_handover_registered_consumer.go`；改写 `adapters/transportfulfillment/control_transfer.go`（按键读交接，去掉字符串比对）；`cmd/parcel-dispatch/assemble.go` FanOut 加一路 + 新 `assemble_node_control_on_handover.go` + 新真库用例；文档 ADR-0214、新 UC-NO-005、NO CONTEXT「节点控制」一句、CONTEXT-MAP `node-operations ↔ transport-fulfillment` 一句、开发主线「机制半边现状」PN-04 行更正、演示脚本。
与 idp-parcel-9b 的四个共享点（详见方案「与 idp-parcel-9b 的协调点」）：
1. `adapters/postgres/reception.go` 读控制处的折叠——9b 的 ps-external-mark-relations/11 在改此文件；等它进 main，或事先约定本票只加一个辅助函数调用。
2. `migrations/node_operations/` 的号——9b 在连续取号，认领时现取。
3. NO 查阅口注册新册的那一行（`adapters/postgres/review_catalogue.go` / `ports/catalogue_read.go`）——9b 的 11 也在改。
4. NO CONTEXT「节点控制」一节与 UC-NO-005——9b 在改「送站交接单」段落；本票一律写全称「权威运输交接」。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；[勘察报告](../../no-control-transfer-out/report.md)；方案「代决」C2、C3、C4、C5、C7，「需要的 ADR」ADR-0214。

## 做什么

- 新立 UC-NO-005，转出与转入同一用例（方案 C2）。
- 交接对象按正式包裹身份经 NO 自己的当前关联对上作业实物；各格入账照方案 C3。
- 新立只追加的节点控制转移册；收寄行不改写，读控制时折叠（方案 C4）。
- 转出判据照方案 C5；交接更正照方案 C7。
- 文档：起草 ADR-0214（C6 由 [62](./62-node-control-is-established-by-handover-in.md) 落，ADR 里写成决定并标明后继票）；更正开发主线 PN-04 行「交接→控制转出」适配器已接通的说法。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 收寄控制在身、交接 `已交接`：收寄链头 `controlReleasedBy` 有值；重投幂等。
- 拒收、待确认：零写入。
- 零件、多件、集运单元对象：各一格入账，不推进。
- 撤回控制的更正：入账「控制来源链待重算」，查阅口可见。
- 同一实物并发只落一行。
- 转出之后同节点再送站，不再并入旧收寄。
- 真库测试。
- 演示：SHA 收寄 → 交接带 `releasingNode=SYN-NODE-SHA-HUB`（[59](./59-handover-declares-releasing-receiving-and-onward-nodes.md) 未落时不带）→ 派发一拍；`/node-operations-records?registry=reception` 该行 `controlReleasedBy=TRANSPORT-HANDOVER/<版本>`；新册 `registry=physical-control` 列出这次转出。

## 规模

L
