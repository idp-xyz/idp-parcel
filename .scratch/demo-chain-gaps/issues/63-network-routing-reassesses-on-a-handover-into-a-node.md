# 63 交接进入节点时路由复核

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)
地盘：NR 新 `adapters/inbox/transport_handover_consumer.go`、新 `adapters/transportfulfillment/reassess_on_handover.go`、新 `adapters/parcelshipment/accepted_carried_geo.go`；`cmd/parcel-dispatch/assemble.go` FanOut 加一路 + 新 `assemble_reassess_on_handover.go` + 新真库用例；文档 ADR-0215（ADR-0075 Links 加后继指针、`docs/adr/README.md` 状态格注明部分停用）、UC-NR-003 触发行与 AT、NR CONTEXT 一句。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「复核」04「票面漏掉的真阻断」，「代决」C8、C9，「需要的 ADR」ADR-0215。

## 做什么

- NR 消费交接信封，只对「`已交接` 且声明接收节点」形成复核触发；其余入账（方案 C8）。
- 所携地理投影经消费侧适配器取受理基线那一版提交的投影，不读地址本体（方案 C9）。
- 读的是 `ShipmentRequests.FindCurrentAcceptedByParcel` 与委托聚合，不碰 PS 外部标识读口。
- 文档：起草 ADR-0215。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- `已交接` + 接收节点：一条复核，控制证据 `TRANSPORT_HANDOVER_CONTROL`，位置为接收节点；所携投影摘要等于基线那一版的。
- 其余裁决、无接收节点、对象不是当前已接受包裹：入账，零复核。
- PS 读不动、复核未决：登哨兵重投。
- 同一交接重投答已有结果。
- 真库测试。
- 演示：SIN-HUB 接收交接（`receivingNode=SYN-NODE-SIN-HUB`）→ `/route-plans?registry=reassessment` 多一行 `STILL_APPLICABLE`@SYN-NODE-SIN-HUB。

## 规模

M
