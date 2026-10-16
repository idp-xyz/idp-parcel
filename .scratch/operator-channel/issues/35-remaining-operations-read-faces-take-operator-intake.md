# 35 运营查阅面余下各族换操作者 Intake：委托侧、节点、运输、追踪、代收、结算与治理的查阅

Category: enhancement
Status: ready-for-agent——2026-10-16 Claude Code 会话立并分诊（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [no-directional-basis/01](../../no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md) 偏离标记半边「运营今天就该看得见」：节点作业查阅在生产装配里仍是未配置 Intake，真实租户访问答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
Blocked by: 无
父票：无。[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 已于 2026-10-16 收口；本票接的是 [05](./05-catalogue-read-faces-take-operator-intake.md) 完成记录里「节点、运输、追踪、代收、结算、治理的查阅仍走隔离读」那一半。
地盘：`cmd/parcel-api` 端点表里这几只查阅变量的装配行、行注与装配测试（含隔离读用例）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」与「取证」两节。`endpoints.go` 与 `idp-parcel-9b` 会话约定：改前报行号范围；换完把「节点作业查阅」那一行写着「经写开关放行」的行注改成实情。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四、五；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三；[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)。

## 现状（读码，钉 `c68e4342`）

`cmd/parcel-api/endpoints.go` 的端点装配里，下面这些查阅变量的生产取值是各自包的 `UnconfiguredIntake{}`。只在隔离读入参非 nil 时，它们才被盖成 `isolatedRead` 的同名行：

- `shipmentViewsIntake`
- `labelTransactionIntake`
- `channelSelectionDecisionIntake`
- `nodeOperationsCatalogueIntake`
- `transportCatalogueIntake`
- `trackingProjectionsIntake`
- `collectionCatalogueIntake`
- `settlementCatalogueIntake`
- `governanceRegistryIntake`

后果：生产形态下，真实租户的运营在这几页一律答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，演示租户只在开隔离读时看得见。05 已把计价、网络、关务、商业四族换成 `operatorRegistries.*OperationsRead`，写法与测试形状都现成。

## 做什么

1. **先核每口的受众，逐口写进票面。**
   - 受众是租户运营的，换操作者 Intake。能力面取主数据与运营查阅读；某口已有更窄的能力面的，按其面。
   - 受众是货主客户的，不换，留在隔离读，等客户渠道（ADR-0139 一族）。
2. **三格测试同 05**：发行方未配置答 403；不带令牌答 401；授予不足答 `403 OPERATOR_NOT_GRANTED`。授予齐备的合成操作者答查阅结果。
3. **已换各口的隔离读放行同一笔撤下**，用例改写为真渠道答复格用例（05 的口径）。
4. **评估隔离读开关退场。** 若撤完隔离读已无行，评估 ADR-0078 隔离读开关整体退场，先例是 22、23 对隔离写的退场。同一笔做不完的，另立票。
5. **演示动线脚本改写。**
   - 「前置」改成起操作者渠道：三件发行方参数加治理自授权，并取配置员令牌。
   - 「取证」表按新答复格改写。今天只设两个隔离开关时，第 1–4 步的目录读已全答 403（2026-10-16 重走实测），脚本与代码不符。

## 不做

- 不改任何查阅端点的读口与答复形状，只换渠道。
- 不碰提交口：它的隔离写放行归客户渠道。

## 完成判据

- 已换各口：三格各答其格，授予齐备的合成操作者答查阅结果；隔离读放行同笔撤下。
- 节点作业查阅各册在生产装配下经操作者渠道可读，包括 no-directional-basis/01 落地后的「集运成员关系」册。
- 演示动线脚本的「前置」「取证」两节与代码一致：照脚本起进程，各查阅口带配置员令牌答 200。证据只记 `S`。
