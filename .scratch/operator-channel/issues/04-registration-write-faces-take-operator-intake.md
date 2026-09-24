# 04 登记册配置写面逐口换操作者 Intake

Category: enhancement
Status: draft
Blocked by: 03
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 端点表里 ADR-0085 决定一那一族登记端点的装配行及其装配测试。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四与 Consequences；ADR-0091 Consequences 的逐口纪律。

## 做什么

1. 开工第一步：逐口归类端点表里挂 `UnconfiguredIntake{}` 的写行——属 ADR-0085 决定一那一族登记写面的归本票；作业事实登记、外部结果接收、客户委托命令归 [08](./08-isolated-release-of-main-chain-command-faces.md) / [09](./09-decision-production-channel-for-business-command-faces.md)（ADR-0100 决定四、五明文不覆盖）。归类表写进完成记录。
2. 本票那一族逐口换成操作者 Intake：每换一口，该口的「未配置即拒」测试改写为三格测试；未换的口答复不变。隔离读放行表与 `SYN-` 写开关零改动。

## 完成判据

- 归类表完整；已换各口对合成操作者答业务结果、对三格各答其格（带测试）；装配测试与端点表仍一一对照。
