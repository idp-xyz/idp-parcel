# 23 节点作业隔离命令 Intake 退场：共享译装挪出，用例迁到作业事实收寄 Intake，删类型与门里的构造

Category: enhancement
Status: in-progress——2026-10-16 Claude Code 会话立并认领（用户「针对上面的问题，你能自决，全部解决吗」），分支 `claude-oc21`。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施（收口后的清扫）
地盘：`internal/nodeoperations/adapters/http` 的 `isolated_command_intake*.go`、`work_fact_intake*.go`、`receive_delivered_unit.go` 头注；`cmd/parcel-api/assemble_isolated_write.go` 与 `isolated_write_test.go` 里节点作业那一格。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三；[15](./15-operation-decision-faces-take-operator-intake.md) 完成记录「发现，不做」第二条。

## 为什么

收寄口随票 10 换作业事实渠道之后，`nodeopshttp.IsolatedCommandIntake` 只剩隔离写门里的一次构造，装配不读它（`isolatedWriteAdmission` 头注自承）。收寄载荷的译装（`receptionDocument`、`receptionCommand`、`applyReceptionClaim`、`decodeClosedDocument`）住在隔离 Intake 的文件里、被作业事实收寄 Intake 共用，用例也只挂在隔离类型上。

## 做什么

1. 收寄载荷的译装挪到独立文件，行为不变。
2. 隔离 Intake 的用例改挂 `WorkFactReceptionIntake`（替身认证方交回合成租户与节点）；补作业事实那几格（三个头缺一即坏报文、作用域种类只认 `NODE`）。
3. 删 `IsolatedCommandIntake` 类型；隔离写门里节点作业那一格（字段、访问器、构造、合成节点常量）与「随门构造」用例一并删去。

## 完成判据

- 原隔离用例的每一格在作业事实收寄 Intake 上都有对应用例。
- 全仓无 `nodeopshttp.IsolatedCommandIntake` 的引用；本包与 `cmd/parcel-api` 测试绿。

## 实施中量到的（不在本票做）

收寄命令的 `ServiceMarkers`（PS 服务结果引用：已取消 / 终局 / 无路由 / 受限）按 `ReceiveDeliveredUnitCommand` 的契约「由接入层查好带入」，它不阻止接收、只限制后续方向性作业（UC-NO-002 的 `AT-NO-020`–`022`）。票 08 当时记过「隔离形态没有那道查询，留空」；票 10 换上作业事实渠道之后，`WorkFactReceptionIntake` 同样不查——生产上已取消或已终局的包裹到站收寄时不带标记。立为 [25](./25-node-reception-carries-parcel-shipment-service-result-markers.md)。
