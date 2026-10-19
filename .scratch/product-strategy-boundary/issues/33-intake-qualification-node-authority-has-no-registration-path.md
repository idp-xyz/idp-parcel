# 33 收寄硬资格的节点作证认领没有登记口

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决」授权），出自 [32](./32-demo-rule-package-declares-no-unprovable-intake-qualification.md) 的取证。
Blocked by: 无
出处：[ADR-0063](../../../docs/adr/0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)；[syn-wall-door-audit/10](../../syn-wall-door-audit/issues/10-intake-qualification-evidence-source-unimplemented.md)（机制半边已结，实例半边留空）；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)。

## 现状（读码，钉 `f8351cef`）

- `cmd/parcel-dispatch` 的 `nodeQualificationAuthority` 是实例配置：节点认领的权威段前缀，加上该段下每条资格引用由哪一件节点执行事实（协作事项加动作）来证。
- 生产装配交进来的是零值，读作「本部署没认领任何段」，恒答未证明。
- 要认领一段，今天只能改装配代码重新编译。租户上线时实施方没有任何登记口能填这两格。
- 这两格本该是租户取值：段怎么起名、哪条引用算数，都随租户的资格声明。可现在它们写在部署代码里，而且不分租户，同一部署里的所有租户共用一份。
- 节点执行事实只出在关务节点协作链上（UC-NO-001）。没有别的面能登它。

## 要定的

- 认领住哪：PS 一本按租户分的登记册（资格引用 → 协作事项、动作；前缀由引用取出并校验同段），还是部署参数。按 ADR-0146 与 ADR-0150 的口径倾向前者：租户取值经普通登记落，不写进部署。
- 证据口按租户取认领：今天的 `NodeExecutionQualificationEvidence` 不带租户维。
- ADR-0063 决定五的残余要在登记门上守住：认领的段不能是正式关务判断那一段。

## 不做

- 不替任何租户认领段，不给默认前缀。
