# UC-PC-003 裁定商业授权请求

状态：裁定机制已实现（授权治理册、合同委派读口、裁定编排，`cmd/parcel-api` 已装配）；建模未决一（客户侧授权代表）与撤回动作仍**待人类评审**，那两支不得引用为已确认口径。收尾对授予与委派的作用、多条命中的答法由 [ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 裁定。真实授权规则、权限等级目录、原因目录与决定方来源待提供（`PAR-COM-14`，租户取值）

## 目标与边界

本用例从某个消费上下文带着「谁、要在什么范围上、以什么权限等级、做哪个动作、依据什么原因和证据、在什么业务时点」提交一次授权询问开始，到 `party-commercial` 交回`已授权`并指名所采用的授权规则版本与实际决定方、交回`不允许`、交回`授权规则未配置`、交回`适用冲突`，或者本次未能形成答复为止。

它回答的是**谁可以**，不回答**要不要**。这条界由 [ADR-0042](../../adr/0042-acceptance-content-declarations-by-owning-object.md) 的被否决备选写死：「授权答『谁可以』，规则包答『要不要』；并格会让『有人有权复核』被读成『本单要复核』」。因此本用例的读取面必须是独立端口，不并入 `AcceptanceContentDeclaration`，也不把授权塞回接单规则包正文声明。

本用例包含：

- 接收不携带授权引用的授权询问：调用方自带一个，就等于自己给自己签字。
- 按适用范围与业务时点装载当前已生效的授权规则版本，折进所挂版本的收尾（退役即不参与、截止即收窄）。
- 逐条比对动作、责任法人、商业权限等级、适用范围与有效区间：恰命中一条即形成授权，命中多于一条即适用冲突。
- 把「没有任何现行规则」与「有规则而本请求不在其内」分成两个答复。
- 交回所采用的授权规则版本引用，供消费方随其业务决定一并保存。

本用例不执行：

- 替消费方作业务决定。获准主动拒绝不等于这一单应当被拒绝；授权只解除禁止，不构成理由。
- 发布、修改或撤销授权规则本身——那是 [`UC-PC-001`](./UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) 的事。
- 以参与方角色、组织位置、职务名称、登录身份或技术账号顶替商业权限等级。
- 为缺失的授权规则生成任何生产默认值，包括「默认允许」与「默认不允许」两个方向。
- 落库或形成商业版本。本用例是纯读取裁定，不留下自己的状态。

## 元数据

| 项目 | 内容 |
|---|---|
| 用例 ID | `UC-PC-003` |
| 主责上下文 | `party-commercial` |
| 主要参与者 | 授权治理责任方；消费上下文（`parcel-shipment`、`settlement-accounting` 等）作为询问方 |
| 业务决定所有者 | `party-commercial`；答复只来自当前有效的版本化授权规则 |
| 触发 | 某个消费上下文在执行受控动作前询问授权 |
| 成功结束 | 形成`已授权`并指名所采用的授权规则版本 |
| 业务否定结束 | 形成`不允许`：权威已就该范围表过态，本请求不在其内 |
| 未配置结束 | 形成`授权规则未配置`：该范围在该时点没有任何现行规则 |
| 冲突结束 | 形成`适用冲突`：同一请求在该时点被多于一条授予允许 |
| 未形成结束 | 技术读取未完成；不得伪装成上述任何一格 |

## 权威依据

| 引用键 | 权威内容 | 来源 |
|---|---|---|
| `PC-LANG` | 商业权限等级、授权规则、适用范围与商业版本语言 | [参与方与商业上下文](../../domain/party-commercial/CONTEXT.md#language) |
| `PC-RULE` | 授权版本化、唯一适用性与所有权 | [参与方与商业上下文](../../domain/party-commercial/CONTEXT.md#rules-and-invariants) |
| `ADR-AUTH-SPLIT` | 授权答「谁可以」，规则包答「要不要」，两者不得并格 | [ADR-0042](../../adr/0042-acceptance-content-declarations-by-owning-object.md) |
| `ADR-RECOVERY` | 失败按消费方的恢复动作分格，不按提供方观察到的原因 | [ADR-0029](../../adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) |
| `ADR-SCOPE` | 集团租户、责任法人与客户账户三级边界 | [ADR-0003](../../adr/0003-group-tenant-legal-entity-customer-account.md) |
| `EVIDENCE` | 真实授权参数状态与受控证据引用 | [首发试点参数与证据登记册](../../product/PILOT-PARAMETER-REGISTER.md) |

## 结果语义

| 结果 | 业务含义 | 消费方的恢复动作 | 禁止行为 |
|---|---|---|---|
| 已授权 | 存在一条已生效授权规则允许该请求 | 继续执行受控动作，保存所采用的授权规则版本引用 | 把它读成「应当这么做」；不保存所采用版本 |
| 不允许 | 权威已就该范围表过态，本请求不在其内 | 停止该动作，按业务否定处置 | 让请求方换个权限等级重试直到命中 |
| 授权规则未配置 | 该范围在该时点没有任何现行规则 | 保持未决，等租户登记规则 | 压进`不允许`——那会把「尚未配置」说成「你无权这么做」 |
| 适用冲突 | 同一请求在该时点被多于一条授予允许（同一授权规则对象两版并存未收尾，或不同对象重复授予），或被委派方不同的多条委派命中 | 保持未决，交商业责任方收尾旧版或撤去重复的那一条 | 取首个命中、按版本标签或装载顺序挑一条（[ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)） |
| 未形成 | 权威读取失败、超时或答复无法翻译 | 保持未决，重试或续办 | 冒充`不允许`或`授权规则未配置` |

**`不允许`与`授权规则未配置`必须分开，这是本用例结果语义的核心。** 两者的恢复动作相反：前者是权威给出的业务答案，再登记规则也不会变；后者要租户先把 `PAR-COM-14` 的授权规则登记上。首发期尤其要紧——没有租户就没有任何授权规则，每一次询问都落在`未配置`那一格；压成`不允许`，等于把一个尚未配置的产品说成「你无权这么做」。

**「规则全部过期」与「从未登记」同落`未配置`。** 在请求那个时刻，该范围都没有一条管得着的规则，两者要做的事同为「让一条现行规则存在」——按恢复动作分格（`ADR-RECOVERY`），它们是同一格。

## 应用流程

| 步骤 | 责任方 | 应用动作 | 结果与下一步 |
|---:|---|---|---|
| 1 | 消费上下文 | 提交动作、责任法人、商业权限等级、适用范围、结构化原因、证据引用与业务时点 | 不得携带授权引用；缺结构化原因或证据即输入未受理 |
| 2 | `party-commercial` | 建立最小身份（租户、责任法人、适用范围） | 身份不成立时不查询任何权威，交回输入未受理 |
| 3 | `party-commercial` | 按适用范围与业务时点装载当前已生效的授权规则版本与需要时的合同委派，折进所挂版本的收尾（[ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)） | 读取失败即`未形成`，不冒充其余任何一格 |
| 4 | `party-commercial` | 逐条比对动作、责任法人、权限等级、适用范围与有效区间 | 恰命中一条即形成授权，记下命中的那一条并解出实际决定方；多于一条即`适用冲突` |
| 5 | `party-commercial` | 按范围加时刻判定未命中属哪一格 | 该范围该时刻无任何现行规则→`未配置`；有而不含本请求→`不允许` |
| 6 | 消费上下文 | 随本次业务决定保存所采用的授权规则版本引用 | 只保存所采用的那一份，不复制授权规则正文 |

判据取**范围加时刻**，不取整份请求：责任法人、权限等级或动作任一不符，都说明权威已经就这个范围表过态、只是没把这一项放进去——那是它给出的答案，不是它还没被问过。

## 一致性、幂等与并发

- 本用例不写入，因此没有幂等键。相同询问在规则未变时必须得到相同答复；答复随规则版本与业务时点变化是正确行为，不是不一致。
- 授权按**业务时点**裁定，不按系统当前时间。同一请求以不同业务时点重问可以得到不同答复，那正是有效区间的作用。
- 消费方保存的是**所采用的授权规则版本引用**，不是布尔结果。规则事后被截止或退役时，既有决定继续引用当时采用的那一版（与 `UC-PC-001` 的收尾语义一致）；此后的新裁定按收尾之后的授予判（[ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)）。
- 跨租户询问按 `ADR-SCOPE` 拒绝，且答复必须与询问一个不存在的范围完全一致，不泄露另一租户是否配过规则（`AT-PC-028` 同款口径）。

## 首切范围与现状

动作集合 `AuthorizedAction` 是封闭集：人工复核（`MANUAL_REVIEW`）、主动拒绝（`ACTIVE_REJECTION`）、资料修订（`SOURCE_DATA_AMENDMENT`，[ADR-0116](../../adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 决定一）、受控关闭（`CONTROLLED_CLOSURE`）与重开（`REOPENING`）。裁定交回动作、所命中的授权规则版本、原因、证据、时点与实际决定方（`Authorization.Decider()`，ADR-0116 决定三）。消费侧 `parcel-shipment` 的主动拒绝、人工复核、资料修订各有授权适配器，受控关闭与重开共用一只（`continued_attempt_decision_authorization.go`），按本用例的答复集合翻译；`适用冲突`译成 `psports.AuthorizationOutcome` 的 `APPLICABILITY_CONFLICT` 一格（撤回那只同此），消费方停在未决或答它自己的一码，不读作已授权也不读作拒绝（[ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 越权风险点 3）。撤回不在动作集内，归建模未决一；`parcel-shipment` 的撤回适配器已在，折不出请求即答未形成，不译成授权规则未配置。

## 建模未决与已作的裁定

下列五项都是**建模决定**，不是实现细节。今天仍未决的是一（以及随它的撤回动作）；其余四项已裁，裁决记录在各项下。

**一、客户侧授权代表装不下（未决）。** `parcel-shipment` 的撤回询问问的是「货主客户或其授权代表」有没有权撤回这份委托，而 `AuthorityGrant` 的字段是责任法人加商业权限等级——那是运营侧的形状。货主客户的授权代表既不是责任法人，也不持有运营侧权限等级。需要定的是：客户侧授权是同一册的另一种范围，还是另立一册。撤回动作随这一项一起定，今天不在 `AuthorizedAction` 内。

**二、决定方随不随授权交回（已裁）。** ADR-0116 决定三：裁定结果带实际决定方——客户自己请求即请求方，运营角色代录时由合同委派解出委派方，委派缺席是业务拒绝（`不允许`）不是未配置。

**三、封闭动作集合（已裁）。** 动作集合按消费方的受控动作逐个扩：资料修订由 ADR-0116 决定一加入，受控关闭与重开随其后加入（迁移 `party_commercial/0032`），各格互不蕴含。撤回见第一项。

**四、人工复核那一格的措辞与分工对不齐。** `ADR-0042` 明确把既有的 `ManualReviewRequirementFor(grants…)` 归为授权治理（**谁有权**复核），而该函数今天的名字与注释写的是「回答某个范围是否要求人工复核」（**要不要**）。两种读法在同一个函数上并存，正是 `ADR-0042` 要拦的并格。

**裁决（2026-09-08，MCP-1 代裁，owner 授权自决口径；取证钉 `96558acd`）：取「谁有权」形态、不拆两问，且不改名而是并入 `Authorize`。** `Authorize(grants, delegations, request)` 已是动作无关的「谁有权」裁定，`ManualReviewAction` 与主动拒绝同走 `permits`（按动作、责任法人、权限等级、适用范围与时刻，三值），本用例即其裁定用例；`ManualReviewRequirementFor(grants, scope, at) bool` 只按范围加时刻、无法人无等级无三值，答不了「谁有权」（无主体），也不该答「要不要」（后者单一权威在接单规则正文 `ManualReviewDirective`），改名后仍是 `Authorize` 的无主体退化副本，即 `ADR-0042` 要拦的并格。落地形状：`parcel-shipment` 侧复核授权端口与 `ActiveRejectionAuthorizer` 同形（三值），消费侧适配器调本用例带 `ManualReviewAction`；`ManualReviewRequirementFor` 及只为它写的用例在 PS 端口接真那一笔删去（今天全仓已无此符号）。不立新 ADR：`ADR-0042` 已定归类，这是应用不是新取舍。票 `wiring-baseline-remainder/04`。

**五、同一范围同一时点多条规则都允许时采用哪一条（已裁）。** [ADR-0261](../../adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 决定三：折完收尾之后命中多于一条授予即答`适用冲突`，不取首个、不按版本标签或装载顺序挑；委派方不同的多条委派命中同此。同一授权规则对象两版并存是换版未收尾，经截止或同起点退役消解（[ADR-0233](../../adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)）。

## 首发试点叠加条件

- `PAR-COM-14` 的真实授权规则、权限等级目录与原因目录仍只在参数登记册维护；本用例文档不得把候选值提升为已确认。
- 没有租户就没有任何授权规则，因此首发期每一次询问都落在`授权规则未配置`。**那是本用例唯一走得到的真实成功路径**，不是缺陷，也不得为了让链路看起来通而配一条默认规则。
- 消费侧适配器按 [ADR-0025](../../adr/0025-cross-context-adapters-live-on-the-consumer-side.md) 落在各消费上下文自己的目录下，翻译职责由它独占；本上下文不为任何消费方定制答复形状。

## 验收示例

| ID | 场景 | 预期结果 |
|---|---|---|
| `AT-PC-037` | 存在一条覆盖该范围、法人、等级与时点的主动拒绝授权 | `已授权`，并指名所采用的授权规则版本 |
| `AT-PC-038` | 该范围该时点一条已生效授权规则都没有 | `授权规则未配置`，不得答`不允许` |
| `AT-PC-039` | 该范围有已生效规则，但只覆盖人工复核 | `不允许`：权威已表态，未把主动拒绝放进去 |
| `AT-PC-040` | 规则覆盖该范围但请求方权限等级更低 | `不允许`，不按「等级越高越包含低级」自动推导 |
| `AT-PC-041` | 规则曾经生效但在请求时点已全部过期 | `授权规则未配置`，与从未登记同格 |
| `AT-PC-042` | 同一范围同一时点两条授予都允许该动作（同一授权规则对象两版未收尾，或两个对象各授一份） | `适用冲突`；不取首个、不按版本标签或装载顺序挑一条（ADR-0261 决定三） |
| `AT-PC-043` | 询问不带结构化原因或证据引用 | 输入未受理；不得以空原因形成授权 |
| `AT-PC-044` | 询问自带一个授权引用 | 输入未受理：自带引用等于自己给自己签字 |
| `AT-PC-045` | 请求方持有承运商代理或渠道服务方角色，但无授权规则 | `授权规则未配置`；参与方角色不带来授权 |
| `AT-PC-046` | 另一租户的范围被询问 | 答复与询问一个不存在的范围完全一致，不泄露是否配过规则 |
| `AT-PC-047` | 授权规则装载超时 | `未形成`，不冒充`不允许`或`未配置` |
| `AT-PC-048` | 同一询问在业务时点分别落在区间内与区间外 | 分别得到`已授权`与`不允许`/`未配置`；不按系统当前时间裁定 |
| `AT-PC-049` | 已授权后该授权规则被截止或退役 | 既有决定继续引用当时采用的那一版，不追溯改写 |
| `AT-PC-059` | 同一授权规则对象同起点重发 `v2` 并退役 `v1`，此后按两版共同的范围、法人、等级与动作询问 | 命中 `v2`，`已授权`并指名 `v2`；已退役的 `v1` 不参与，也不使这次询问成为适用冲突（ADR-0261 决定一） |
| `AT-PC-060` | 授权规则旧版被截止在后继的生效时点，分别以截止点之前与之后的业务时点询问 | 之前命中旧版，之后只命中后继；截止之后的旧版授予不算现行规则（ADR-0261 决定一） |
| `AT-PC-061` | 客户合同版本被同起点退役或截止之后，运营角色代该客户请求资料修订 | 挂在已退役那一版、或截止点之后的旧版上的委派不再解出实际决定方；后继合同版本声明了同一委派即由它解出，没有即`不允许`（委派缺席，ADR-0116 决定三）；折法见 ADR-0261 决定二 |

## 给开发的交接

读取面是独立端口：`ports.AuthorityGrantStore.LoadEffectiveGrants` 按适用范围与业务时点装载授权规则，`ports.EffectiveContractDelegationView` 只在运营角色代客户请求资料修订时装载委派，`application.AdjudicateCommercialAuthorizationHandler` 调 `domain.Authorize(grants, delegations, request)` 交回上述各格之一；授权治理册从迁移 `party_commercial/0003` 起（PostgreSQL 适配器 `postgres.AuthorityGrants`、`postgres.ContractDelegations`）。它不得与 `AcceptanceContentDeclaration` 并格，也不得复用 `CommercialAuthorityView.LoadScope`——`CommercialRegistry` 不持有 `AuthorityGrant`。

首发答复对照（`domain.Authorize` 与编排）：`已授权`是交回的 `Authorization`；`不允许`是 `ErrNotAuthorized`，含它的两个具名分格 `ErrDelegationAbsent`（委派缺席）与 `ErrCustomerAccountCannotDecide`（客户账户请求只能由运营角色形成的关闭或重开）；`授权规则未配置`是 `ErrAuthorityRulesNotConfigured`；`未形成`是装载失败的包装错误与委派读口未接线的错误；输入未受理在请求构造时就拒（`ErrInvalidAuthorizationRequest`、`ErrAuthorizationRequesterRequired`），不进编排。`适用冲突`是 `ErrAuthorityApplicabilityConflict`，它既不 Is `ErrNotAuthorized` 也不 Is `ErrAuthorityRulesNotConfigured`。授予与委派的装载先折所挂版本的收尾（`AuthorityGrant`、`ContractDelegation` 的 `NarrowedBy`、`EndedBy`、`InForceAt`，与自带区间的政策正文共用领域 `versionFold` 一处折法），折后不在业务时点现行的不交出。

现状缺口（规则见「结果语义」「应用流程」与「建模未决与已作的裁定」第五项）：

| AT | 缺口 | 承接 |
|---|---|---|
| `AT-PC-042` | 已补：`Authorize` 命中多于一条授予、`resolveDecider` 命中委派方不同的多条委派，都答适用冲突，不取首个；装载顺序仍按对象与版本标签稳定，但不再决定采用哪一版 | 票 [docs-consistency/code/24](../../../.scratch/docs-consistency/code/24-closed-authorization-rule-and-contract-versions-keep-granting.md) |
| `AT-PC-059`、`AT-PC-060`、`AT-PC-061` | 已补：授予与委派的装载连退役册与最新一条区间更正，已退役那一版不交出、截止点之后的旧版不交出；同起点重发并退役旧版之后只命中后继 | 同上 |
