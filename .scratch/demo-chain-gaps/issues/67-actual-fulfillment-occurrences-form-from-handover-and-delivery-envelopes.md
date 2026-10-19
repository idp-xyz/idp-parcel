# 67 实际履约发生项随交接与有效交付信封自动形成

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)、[28](./28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md)
地盘：TF 新 `application/form_actual_fulfillment_on_participation_end.go`、新 inbox 消费者两只（交接、有效交付，inbox 名各自分家）；`cmd/parcel-dispatch/assemble.go`（交接 FanOut 与 `deliveryFan` 各加一路）+ 新装配文件 + 新真库用例；文档 ADR-0218（ADR-0207 Links 加后继指针、README 状态格注明部分停用）、UC-TF-005「应用流程」步 9 与 AT、演示脚本（干线成本那一步改为自动）。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」TF 自消费交接 / 有效交付信封；方案「复核」等 peer 各项现状，「代决」C15，「需要的 ADR」ADR-0218。

## 做什么

- 只对已采用委托的段，在参与结束时经既有登记用例形成实际履约发生项；采购上下文、旅程与各格答法照方案 C15。
- 演示约定的旅程拼写随之改为派生拼写。
- 文档：起草 ADR-0218。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 已采用委托的段参与结束：发生项 `REGISTERED`，与已形成信封同笔。
- 无采用连线：零写入，入账。
- 协议未声明收费发生条件：`未配置`，入账。
- 人工同身份重登：`EXISTING`。
- PC 读不动：登哨兵重投。
- 真库测试。
- 演示：干线段采用委托后在 SIN-HUB 交接 → 派发两拍 → BUY 请求 `FIRED`，无需人工登记发生项。

## 规模

M
