# 35 运营查阅面余下各族换操作者 Intake：委托侧、节点、运输、追踪、代收、结算与治理的查阅

Category: enhancement
Status: in-progress——2026-10-16 Claude Code 会话认领（工作流派单），隔离工作树分支 `claude-oc35`，基 `627dde00`；受众逐口核过，见「受众核对」。此前 ready-for-agent——2026-10-16 Claude Code 会话立并分诊（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [no-directional-basis/01](../../no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md) 偏离标记半边「运营今天就该看得见」：节点作业查阅在生产装配里仍是未配置 Intake，真实租户访问答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
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

## 受众核对（读码，钉 `627dde00`）

逐只变量、逐口核。受众取管理台上消费它的那一页与该口端点构造函数的注释；能力面在 `accessidentity.CapabilityFace` 那几格里找。查阅没有比主数据与运营查阅读更窄的一格：`GOVERNANCE_REGISTRATION` 是治理登记（写）那一格，且仍预留、登不进册。

| 变量 | 口 | 受众 | 结论 |
|---|---|---|---|
| `shipmentViewsIntake` | `/shipment-request-views`、`/acceptance-review-queue`、`/authorized-disposition-queue` | 租户运营：管理台委托查阅、接受前复核、授权处置三页 | **不换，留隔离读。** 受众是运营，卡在作用域形状上：三口的读口收 `AuthorizedQueryScope`，按可见客户账户切，至少一个账户（CONTEXT「授权查询作用域」）。操作者信封只给得出租户。运营决定口的可见范围是整个租户，那一侧另立了寻址读口，理由写在 `ports.OperatorDecisionTargets` 头注：「拿一组账户去凑等于造一个作用域」。查阅口没有整租户的形状，要补就得改读口，超出本票「不改读口」。另立 [53](./53-shipment-request-views-tenant-wide-operator-read-and-isolated-read-switch-retires.md) |
| `labelTransactionIntake` | `/label-transactions` | 租户运营：面单交易页 | 换。作用域只有租户维（ADR-0084 决定七），信封给得出 |
| `channelSelectionDecisionIntake` | `/channel-selection-decisions` | 租户运营：渠道择优页，列出等人工裁决的并列冲突 | 换。只有租户维 |
| `nodeOperationsCatalogueIntake` | `/node-operations-records`（收寄、待识别实物、集运单元三册） | 租户运营：节点作业查阅页。端点头注写明它是治理查阅面，不是一线作业端（ADR-0021） | 换。no-directional-basis/01 若再加「集运成员关系」册，同端点、同变量 |
| `transportCatalogueIntake` | `/transport-fulfillment-records`、`-handover-scope-summary`、`-external-tracking-facts`、`-carrier-first-effective-pickups` | 租户运营：运输履约查阅页，以及判断人的「该判哪几条」 | 换 |
| `trackingProjectionsIntake` | `/tracking-projections`、`/visibility-catalogues`、`/exception-triage-records`、`/exception-case-records`、`/claims-recovery-records` | 租户运营：运营追踪查阅（ADR-0076）与 VE 目录、案件三页 | 换。客户面是 `/customer-tracking-view`，它不挂这只变量，挂字面量未配置，本票不碰 |
| `collectionCatalogueIntake` | `/collection-subledgers` | 租户运营：代收分户账页 | 换 |
| `settlementCatalogueIntake` | `/settlement-charges`、`-statements`、`-funds-applications`、`-operating-results` | 租户运营：结算与核算四页 | 换 |
| `governanceRegistryIntake` | `/governance-registers` | 运营方的治理操作员（ADR-0083 决定四）。册是产品实例级的，没有租户维 | 换。信封上的租户只用来认证，不进作用域：作用域只带引用一维（ADR-0083 决定二、三），读口本来就不收租户 |

受众是货主客户的口：这九只变量上一口也没有。

隔离读开关能不能整体退场：不能。委托查阅三口还挂在它上面。退场归 53。

## 不做

- 不改任何查阅端点的读口与答复形状，只换渠道。
- 不碰提交口：它的隔离写放行归客户渠道。

## 完成判据

- 已换各口：三格各答其格，授予齐备的合成操作者答查阅结果；隔离读放行同笔撤下。
- 节点作业查阅各册在生产装配下经操作者渠道可读，包括 no-directional-basis/01 落地后的「集运成员关系」册。
- 演示动线脚本的「前置」「取证」两节与代码一致：照脚本起进程，各查阅口带配置员令牌答 200。证据只记 `S`。
