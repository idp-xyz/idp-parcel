# 02 运输委托、订舱与承运接受没有命令面，委托也不核供应商协议

Category: enhancement
Status: 拆分——2026-10-19 拆成 [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md)、[23](./23-transport-commission-submission-face.md)、[24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)、[25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)，后继 [37](./37-confirmed-booking-capacity-relationship.md)，代决见 [tf-chain-plan](../tf-chain-plan.md)。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【硬停】UC-TF-004 运输委托/订舱/承运接受没有生产入口；就算接上，委托也不核商业侧的供应商协议

## 证据（读码）

`tfapp.NewCommissionTransportHandler` 在 cmd/ 零引用；`tfpostgres.OutboxTransportCommissionHandoff` 会发 `transport-fulfillment.transport-commission.submitted`，但 dispatch 路由表里没有这个类型。`SubmitCommissionCommand` 的 Provider/Agreement/Conditions 是自由串，`CommissionTransportDeps` 没有参与方与商业侧的读口，种子里已发布的 `SYN-SUPPLIER-TRUNK-CN-SG-01`、`SYN-SUPPLIER-LASTMILE-SG-01` 引不进来、也核不了。NR 计划段的责任方只指向连接版本（`calendar_projection.go` 里 `plannedLeg` 注释「目录没有责任方列」），推不出哪一段该委托给谁。DecisionKind 没有委托或订舱这一格。

## 三类

机制与产品策略为主，另有租户取值。规模 L。

## 最小的诚实修法

机制：开一组委托提交/订舱/承运接受答复/取消的命令面，装配 `NewCommissionTransportHandler` 和现成的 postgres 存储；委托命令加参与方与商业侧的供应商协议读口，核协议在册且适用；`transport-commission.submitted` 在有消费者之前不登路由。产品策略：订舱/承运接受的出向连接器至少有一种形态对合成对端跑通（psb/09 第 3 项）。租户取值：演示两段各采用哪份供应商协议，作为 SYN- 实施格。

## 要裁的

承运接受答复走运营决定面还是集成客户端面（ADR-0149 两族；后者依赖 psb/31）；实际履约段上要不要记采用的委托和协议快照（停点 9 形成发生项需要它）。

## 既有票

psb/09 第 3 项（只覆盖连接器那半；命令面与装配无票）
