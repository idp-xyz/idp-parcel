# 53 委托查阅三口要一个运营侧整租户的读面，随后隔离读开关整体退场

Category: enhancement
Status: needs-triage——2026-10-16 Claude Code 会话立（票 [35](./35-remaining-operations-read-faces-take-operator-intake.md) 受众核对：委托查阅三口受众是运营，但读口作用域按客户账户切，操作者信封给不出，本票接）。
Blocked by: 无
父票：无。接 [35](./35-remaining-operations-read-faces-take-operator-intake.md)「受众核对」里 `shipmentViewsIntake` 那一行。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（最后一口换完的那一笔删开关）；[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)；[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四；PS CONTEXT「授权查询作用域」。

## 现状（读码，钉 `627dde00`；35 落地后仍成立）

`/shipment-request-views`、`/acceptance-review-queue`、`/authorized-disposition-queue` 三口共用 `shipmentViewsIntake`。生产装配下它是未配置 Intake，只有隔离读开关开着时才换成注入式放行。

- 三口的读口都收 `domain.AuthorizedQueryScope`：租户加可见客户账户集，账户至少一个。SQL 在 `customer_account_id = ANY($2)` 上过滤。
- 操作者信封只有租户。操作者册的授予按能力面登记，没有客户账户这一维。
- 同一租户的运营决定口（复核完成、主动拒绝、授权处置）走的是整租户寻址，读口另立为 `ports.OperatorDecisionTargets`，头注：「拿一组账户去凑等于造一个作用域」。

后果有三。

1. 生产形态下，真实租户的运营在委托查阅、接受前复核、授权处置三页答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
2. 演示里这三页只在开隔离读时看得见，且只看得见注入的 `SYN-ACCOUNT-01` 一个账户的委托。运营能对整租户的委托作决定，队列却只列一个账户。
3. ADR-0078 的隔离读开关删不掉：ADR-0150 决定三要最后一口换完那一笔才删。

## 做什么

1. **定运营侧委托查阅的作用域形状。** 候选是整租户，与运营决定口同界；也可能有更窄的运营可见集。它要过 PS CONTEXT：「授权查询作用域」今天只说「共享身份认证/授权技术能力已经判断并传入的可见范围引用」，客户侧按账户切；运营侧是另立一个值对象，还是同一个对象多一种形状，由 PS owner 定，建议走 `/domain-modeling`。不得给 `AuthorizedQueryScope` 塞一个空账户集当「全部」——它的构造器注释正是为了不让每个读口各自决定空集合的意思。
2. **读口照定下的形状补齐**：列表、详情、复核队列与授权处置队列一起补——它们挂在同一只 Intake 变量上，只补一部分会装出「同一个 Intake 两种答案」。
3. **三口换操作者查阅读**：三格测试同 05 与 35；隔离读放行同笔撤下。
4. **隔离读开关整体退场。** 这是 ADR-0150 决定三说的最后一口，清单：
   - `IDP_PARCEL_ISOLATED_READ_TENANT` 与它的解析、启动日志、`isolatedReadIntakes`；
   - 隔离写那条两开关一致性校验，以及隔离写借用读开关常量的那两处：`isolatedReadTenantEnv`、`isolatedReadCustomerAccount`；
   - 各包残余的 `IsolatedOperationsReadIntake` 类型：PS 那一只（只剩委托查阅的方法），以及 05 换口后留下的计价、网络、关务、商业四只。四只自 05 起无装配使用者，35 已把 `cmd/parcel-api` 里构造它们的四格删掉。各包「隔离读 Intake 装不进写口」的类型锁用例要改挂操作者查阅读 Intake，不要连同性质一起删；
   - 演示动线脚本「前置」里的读开关一行。

## 不做

- 不碰提交口与隔离写开关：提交口的真渠道归客户渠道（ADR-0139 一族）。
- 不改客户面 `/customer-tracking-view`。

## 完成判据

- 委托查阅三口：三格各答其格，授予齐备的合成操作者答查阅结果；运营看得见整租户（或定下的那个可见集）的委托，不只一个注入账户。
- 全仓没有 `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IsolatedOperationsReadIntake`；装配与各包测试绿。
- 演示动线照脚本起进程，只设写开关与操作者渠道参数，各查阅口带配置员令牌答 200。证据只记 `S`。
