# 05 主数据与运营目录查阅面逐口换操作者 Intake

Category: enhancement
Status: draft
Blocked by: 03
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 端点表里 ADR-0077 那一族目录查阅端点的装配行及其装配测试。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四；[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 隔离读放行。

## 做什么

1. 目录查阅面逐口换操作者 Intake（能力面：主数据与运营查阅读），三格测试同 04。
2. 与隔离读放行并存：隔离读入参非 nil 时照旧换隔离行，两者是装配点上的两行，互不替代（ADR-0100 决定五第二条）。

## 完成判据

- 已换各口对合成操作者答查阅结果、三格各答其格；隔离读放行的既有用例不改一字仍绿。
