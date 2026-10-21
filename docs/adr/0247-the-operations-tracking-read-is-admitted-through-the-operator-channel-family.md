# ADR-0247：运营追踪查阅面的真 Intake 是操作者渠道 Intake——运营查阅的认证属产品交付，不再等接入渠道实例半边

Status: Accepted（2026-10-21，Claude Code 会话代 VE owner 决（代决）：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/04，审计条目 [audit/ve.md](../../.scratch/docs-consistency/audit/ve.md) VE-15（归 B）。部分停用 [ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md) 决定三两句与 Consequences 一句、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四标题句的覆盖面（被停原句与适用场景见两者的 Status 行）；两者正文不改，只在其 Status 行与 Links 加前向指针。代码已是本记录所述的样子，不立代码票。越权风险点见文末，待 VE owner 复核，第 1 点同时待接入身份 owner 会签。）<!-- 索引注记 -->｜**归族**：运营追踪查阅（ADR-0076 那一族端点，`/tracking-projections` 及沿其模式立的 VE 运营查阅各口）的认证与授权归操作者渠道族，能力面是 ADR-0100 决定二已列的「主数据与运营查阅读」；判据同 ADR-0100——信任锚、校验方式与授权模型都由产品定，属机制半边（按 ADR-0146 读作产品交付）。**答复**：照 ADR-0100 决定四三格——发行方未配置 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`，令牌无效 `401`，不在册、不绑这个租户或无此能力面授予 `403`。**不变**：运营查阅仍是租户级、无客户维的独立作用域，作用域只在真 Intake 之后由认证结果铸造，不落「开发用」采信实现；客户查阅面 `/customer-tracking-view` 照旧归首方客户渠道族（ADR-0139）。某租户的操作者是谁、授予了什么是租户取值。

Date: 2026-10-21

## Context

取证于 main `205eb254`，只记 `S`。

- ADR-0076 决定三写的是：运营接入面的认证方式「属接入渠道实例半边、未登记」，「未登记前装配 UnconfiguredIntake，一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`」。Consequences 写的是「接入渠道认证参数未登记前，没有任何选项能让页面显示真轨迹——本记录不改变该深度上限，真轨迹属实例半边」。ADR-0076 的 Status 行只有 `Accepted`。
- 九天后，ADR-0100 决定一定了「运营操作者身份是产品自有的接入渠道族，属机制半边」；决定二列出的授予能力面有「主数据与运营查阅读」一格。但它的决定四只把覆盖面写成「ADR-0085 Decision 一那一族登记端点」与 ADR-0077 那一族目录查阅端点，没有点名 ADR-0076 的 `/tracking-projections`，部分停用清单里也没有 ADR-0076。
- 代码早已按操作者族做：
  - `internal/visibilityexception/adapters/http/operator_read_intake.go` 的 `OperatorOperationsReadIntake` 自称「操作者渠道在运营查阅面的 Intake（ADR-0100 决定四）」，作用域引用取能力面名 `MASTER_DATA_AND_OPERATIONS_READ`。
  - `cmd/parcel-api` 把 `/tracking-projections` 与 VE 的 `/exception-triage-records`、`/exception-case-records`、`/claims-recovery-records`、`/visibility-catalogues` 都挂在 `operationsReadFace()` 上，答 `401` / `403` 各格。
- 于是两份已接受记录一个说「实例半边、未登记即一律 403」，一个的判据与能力面已经包住了它，却没有明文停用它。读 ADR-0076 的作者会以为运营追踪页在租户登记接入参数之前看不到真轨迹，会以为那一面没有 `401`。

## Decision

**一、运营追踪查阅面归操作者渠道族。**（机制）

- ADR-0076 那一族运营查阅端点的真 Intake，就是 ADR-0100 的操作者渠道 Intake。这一族是 `/tracking-projections`，以及沿 ADR-0076 模式立的 VE 运营查阅各口。授予的能力面是 ADR-0100 决定二已列的「主数据与运营查阅读」，不另立一格。
- 判据同 ADR-0100：一族身份的三件（信任锚、校验方式、授权模型）由产品定，就属机制半边（按 ADR-0146 读作产品交付）。运营追踪查阅的操作者与管理台登记册写面、目录查阅面是同一批人、同一个发行方，三件没有一件取决于租户。

**二、答复照 ADR-0100 决定四三格。**（机制）

- 发行方未配置 → `403` `ACCESS_CHANNEL_NOT_CONFIGURED`。这是 ADR-0055 原格，恢复动作是配部署参数。
- 令牌缺失、过期或校验不过 → `401`。
- 令牌有效，但操作者不在册、不绑这个租户或无此能力面的授予 → `403`，不披露哪一半不对。

**三、停用与不变。**

- ADR-0076 决定三中「属接入渠道实例半边、未登记」与「未登记前装配 UnconfiguredIntake，一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`」两句停用；Consequences 中「接入渠道认证参数未登记前，没有任何选项能让页面显示真轨迹——本记录不改变该深度上限，真轨迹属实例半边」一句停用。
  - 部署参数与操作者授予登记齐备之后，运营追踪页能显示真轨迹。某租户的操作者是谁、授予了什么，仍是租户取值。
  - 决定三其余各句不变：沿 ADR-0055 的 Intake 接口；禁落任何「开发用」采信实现；作用域只在真 Intake 之后由认证结果铸造；角色与权限细分不在本记录。
- ADR-0100 决定四标题句「登记册配置写面与主数据查阅面的真 Intake 就是操作者渠道 Intake，在装配点逐端点换」的覆盖面扩到运营查阅面（适用场景：ADR-0076 那一族运营查阅端点）。
  - 三格答复、「ADR-0055 Decision 五两项未决对登记册配置写面不适用」与其余各条不变。
  - 两项未决对运营查阅面同样不适用，理由与 ADR-0100 Context 对登记册写面的那两条同向：查阅没有客户提交载荷要摘要，也不形成任何生产事实，准入范围管的对象它一样都不碰。
- ADR-0076 决定一、二、四、五（独立读口、租户级无客户维作用域、不承袭三义合并、列表读面键形状）全部不变。客户查阅面 `/customer-tracking-view` 照旧归首方客户渠道族（ADR-0139 决定四）。

三类：决定一、二是**机制**；某租户的操作者主体、绑定与授予是**租户取值**（参数登记册「运营操作者账户与授予」一行），演示租户以 `SYN-` 登记，证据只记 `S`。

## Consequences

- 代码不改：`OperatorOperationsReadIntake` 与 `cmd/parcel-api` 的装配已是本记录所述的样子。本记录补的是它的依据：头注引 ADR-0100 决定四，今天成立了。
- 文档：VE CONTEXT「运营追踪查阅」一条照旧写「认证与授权由共享身份能力提供」，无须改；UC 里没有为运营追踪查阅立的用例，不动。
- ADR-0077 决定三（目录查阅面「运营接入认证属接入渠道实例半边、未登记」）早已被 ADR-0100 决定四点名覆盖，却缺一条前向指针；ADR-0078 决定一排除 `/customer-tracking-view` 的理由「那正是 `PAR-INT-01` 拥有的实例半边语义」已被 ADR-0139 决定一收窄。两处都不是本记录的停用，交票 docs-consistency/10 补指针。

## Alternatives considered

- **维持 ADR-0076 决定三：运营追踪查阅等接入渠道实例参数。** 否决。代码已走操作者族，管理台已按三格呈现；回去等实例参数，就是让运营追踪页在租户登记接入参数之前一律 `403`，而登记册写面、目录查阅面由同一批操作者用同一个发行方早已能登进去。
- **为运营追踪查阅另立一格能力面。** 否决。ADR-0100 决定二的「主数据与运营查阅读」本就带「运营查阅」；另立一格，要租户为同一批操作者多登一份授予，换不来任何隔离。按客户分区的需求（如客服只看某几个客户）属于 ADR-0076 决定二否决过的「客户维可选」，不在这里开口。
- **只在 ADR-0076 的 Status 行写「已由 ADR-0100 覆盖」。** 否决。ADR-0100 正文没有点名这一族，把它说成 ADR-0100 的停用，等于替一份已接受记录补写它没作过的决定。

## 越权风险点

1. **运营追踪查阅与目录查阅共用一格授予**（同时待接入身份 owner 会签）。有目录查阅授予的操作者也能读全部投影，反过来也一样。租户若要把追踪查阅单独授给客服，需要另一格能力面，那会改操作者册的授予模型。
2. **「沿 ADR-0076 模式立的 VE 运营查阅各口」是按今天的装配列的。** 异常分诊、案件、索赔与追偿各查阅口没有各自的 ADR 归族，本记录把它们一并归进这一族。将来若某一口要客户维，要另立记录。
3. **ADR-0055 Decision 五两项未决对运营查阅面不适用，是本记录顺带作出的。** 理由借 ADR-0100 对登记册写面的那两条；运营查阅比写面更远离生产事实，风险更小。

## Links

- [ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md)：决定三两句与 Consequences 一句由本记录部分停用；其余各条不变
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：决定一归族判据、决定二能力面「主数据与运营查阅读」；决定四标题句的覆盖面由本记录扩到运营查阅面
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格与 Decision 五两项未决
- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：决定三与 ADR-0076 决定三同形，已由 ADR-0100 决定四覆盖；指针交票 10
- [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：决定一排除 `/customer-tracking-view` 的理由；指针交票 10
- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：客户查阅面归首方客户渠道族
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：「机制半边」读作产品交付
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：「运营追踪查阅」词条与规则
