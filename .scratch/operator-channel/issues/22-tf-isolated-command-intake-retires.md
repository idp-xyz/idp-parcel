# 22 运输履约隔离命令 Intake 退场：译装用例迁到作业事实 Intake，删类型

Category: enhancement
Status: in-progress——2026-10-16 Claude Code 会话立并认领（用户「针对上面的问题，你能自决，全部解决吗」），分支 `claude-oc21`。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施（收口后的清扫）
地盘：`internal/transportfulfillment/adapters/http` 的 `isolated_command_intake*.go`、`record_delivery_attempt_test.go`、`work_fact_intake*.go` 与几处头注。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三；[15](./15-operation-decision-faces-take-operator-intake.md) 完成记录「发现，不做」第一条。

## 为什么

`tfhttp.IsolatedCommandIntake` 已无装配使用者：作业事实各口随票 10 换作业事实渠道、运营决定四口随票 15 换操作者渠道。ADR-0150 决定三要求换口的同一笔撤下隔离 Intake 上对应的方法；票 10 那几口当时没撤。类型上余下方法的译装用例是这几口线格式的主要覆盖，`WorkFactCommandIntake` 在本包没有自己的用例，所以不能直接删。

## 做什么

1. 隔离 Intake 的译装用例改挂 `WorkFactCommandIntake`（替身认证方交回合成租户）：期望值不变，租户的来处从注入值换成认证结果；作业事实 Intake 自己那几格（三个头缺一即坏报文、作用域种类只认 `OFFSITE`、先认证后解载荷）补用例。
2. 删 `IsolatedCommandIntake` 类型与它的构造器；改准提到它的头注（交付、交接两口的 Intake 契约，运营决定 Intake 的头注）。

## 完成判据

- 原隔离用例的每一格在作业事实 Intake 上都有对应用例（自报租户、尾随内容、未知键、词表外的词、时刻缺席与解不出、各口逐字译装）。
- 全仓无 `tfhttp.IsolatedCommandIntake` 的引用；本包与 `cmd/parcel-api` 测试绿。
