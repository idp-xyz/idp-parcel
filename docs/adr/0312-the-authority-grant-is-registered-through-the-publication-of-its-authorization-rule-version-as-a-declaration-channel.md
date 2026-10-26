# ADR-0312：授权册的授予经发布授权规则版本的声明通道登记，一版一条；授予不进规范化正文，故与取消授权目录不同版共存

Status: Accepted（2026-10-26，Claude Code 会话代 PC owner 决：用户 2026-10-26 定首发要资料修订口，票 first-party-api/13；越权风险点见文末，待 PC owner 复核。）<!-- 索引注记 -->｜**入口**：授权册的授予（`PAR-COM-14` 授权半边，`AuthorityGrantStore` 读的那张表）今天没有生产登记入口；本记录在发布授权规则版本时加一条授予声明通道，授予与所挂版本同一次发布、同一事务落库，不另立子命令。**形状**：一版一条，带动作、责任法人、等级、范围、有效区间；等级是租户登记的串，PC 不认任何值。**重放**：同授予是已登记，异内容是内容冲突，不覆盖，判据与授权册的 `SaveGrant` 同一处。**摘要**：授予不进规范化正文，同版再带取消授权目录整项拒。
Date: 2026-10-26

## Context

- `AuthorityGrantStore.SaveGrant` 在生产代码里没有调用方。裁定编排只读授予，发布批的 `AUTHORIZATION_RULE` 只带取消授权目录声明；`DeclarationChannel` 里没有授予通道。结果是任何租户都登不进授予，资料修订（[ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 一族）的授权判断对每次请求答 `ErrAuthorityRulesNotConfigured`。
- 授予在库里的主键就是所挂授权规则版本（租户、对象、版本标签）；装载时折所挂版本的截止与退役（[ADR-0261](./0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 决定一）。授予的生命周期因此就是那一版的生命周期。

## 候选与反方

- **`cmd/parcel-commercial` 新子命令，单独登授予。** 要另立批文形状与重放代数；授予与规则版本的生命周期绑定，却在两个入口各登一半，换版时会出现版本已换、授予未登的窗口。否决。
- **并入规范化正文（`AuthorizationRuleBody`），让内容摘要盖住授予。** 一致性最好，但要动正文规范化、操作者面的草稿载荷与既有取消授权目录版本的摘要口径（加一格会让已发布版本的摘要串变），首发不值。留作重启条件。
- **发布批的声明通道（采纳）。** 与既有取消授权目录、合同委派同一写法：声明随发布登记，重放与冲突的代数是各册既有的那一套。

## Decision

**一、授予是发布的一条声明通道。**（机制）

- `CommercialDeclarations.AuthorityGrant` 挂在授权规则版本上，通道名 `AUTHORITY_GRANT`；构造走 `NewAuthorityGrant`，挂在别的类别或版本未生效由领域构造门拒。批文键 `declarations.authorityGrant`，带 `action`（授权动作封闭集的名字）、`legalEntity`、`level`、`scope`、`effectiveStartsAt`、可选 `effectiveEndsAt`。
- 一版一条，因为授权册主键是版本；要给第二个等级或第二个法人授予，发新对象或新版本。
- 落库与授权册 `SaveGrant` 同一处（`insertAuthorityGrant`）：同授予重放是已登记，任一格不同是内容冲突，不覆盖。

**二、授予不进规范化正文，同版再带取消授权目录整项拒。**（机制）

- 授权规则版本的规范化正文只有取消授权目录，内容摘要盖不住授予。同一版两者并存会出现「摘要对得上、授予未被摘要盖住」的版本，所以整项拒，授予单独占一版。授予的完整性由重放代数（内容冲突）守，不由摘要守。

**三、重放摘要只盖授予自己的内容。**（机制）

- 授予行的内容摘要不盖快照里版本壳的发布时刻、取效时刻与状态，这些随时钟变；盖进去，同一份批文重放就被读成内容冲突。摘要取版本身份（租户、类别、对象、版本、范围、版本壳摘要）加五格授予内容。

**四、登记成功向下游交一封「授权规则已登记」。**（机制）

- 授予通道属 `AuthorityGrantRegistered` 这一族（[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 决定四）：`已保存`与`已登记`都交，`内容冲突`不交。

**五、等级不写死。**（机制）

- 等级是租户登记的字符串，PC 与翻译层都不认任何具体值。演示种子为 `SYN-TENANT-01` 登的 `CUSTOMER_SELF_SERVICE` 是 parcel-shipment 侧资料修订面所用的等级常量，只作为种子数据出现。

## Consequences

- 租户第一次能登资料修订授予；没登授予的租户照旧答未配置（`ErrAuthorityRulesNotConfigured`），不被说成不允许。
- 演示种子新增一版授权规则 `SYN-AUTH-SOURCE-AMEND-01`，并扩接单规则包的资料修订允许矩阵，规则包摘要随之重算。
- 授予改动发新版，旧版经截止或退役口收尾；本记录不增加改授予的口。

## 越权风险点

1. **授予不被内容摘要覆盖**（PC owner）。授予的完整性只靠重放代数，没有批准摘要对账；要对账须把授予并入规范化正文并处理已发布版本的摘要口径（重启条件）。
2. **一版一条**（PC owner）。同一授权规则版本不能同时授予两个等级；租户需要多个等级时要多对象多版本，适用冲突由 ADR-0261 决定三兜。运营代价：每个（法人、范围、等级）组合要一个独立的授权规则对象；同组合换版时必须先经截止或退役口收尾旧版，否则两版同时生效命中两条授予，答适用冲突（ADR-0261 决定三）。
3. **重放摘要口径改了授权册的摘要算法**（PC owner）。摘要不再盖版本壳的时间字段；生产上此前没有任何授予行（无调用方），测试库不留存量，无迁移。
4. **同版不能再带取消授权目录**（PC owner）。既有只带取消授权目录的版本不受影响；要在同一对象上两者都有，发两个版本或两个对象。

## Links

- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：资料修订授权与授予匹配
- [ADR-0261](./0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)：授予随所挂版本收尾
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：「参数已登记」交接
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)
- 票 `.scratch/first-party-api/issues/13-pc-authority-grants-have-no-production-registration-entry.md`
