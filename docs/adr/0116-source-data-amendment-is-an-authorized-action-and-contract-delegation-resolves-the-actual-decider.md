# ADR-0116：「资料修订」是与人工复核、主动拒绝并列的授权动作；合同委派归客户合同版本、作声明通道；裁定结果带实际决定方——客户自己请求即请求方，运营角色代录时由委派解出委派方，委派缺席是业务拒绝不是未配置

Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-aafca372「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过 `internal/partycommercial/domain/authority_grant.go` 全文、`application/adjudicate_commercial_authorization.go` 全文、`migrations/party_commercial/0003_authorization_grant.sql` 全文、PC CONTEXT 「接单规则包」词条与 Rules 授权那句、Boundaries「版本化角色权限等级、合同委派」那行、`UC-PS-002` 步骤 4 与 `BD-PS-009` 的转述（经票 ps-port-remainder/03）、PS `ports.SourceDataAmendmentAuthorizer` 一族头注、ADR-0100 Decision 二、ADR-0029、ADR-0042；**未读** `AuthorityGrantStore` 的 postgres 实现、三处 `PublicationRegistry` 替身、`UC-PS-002` 原文全文——本记录因此只裁授权动作的封闭集、委派的归属与形状、裁定结果的形状，不裁委派表的持久化细节，也不裁 PS 适配器）
Date: 2026-09-07

## Context

`parcel-shipment` 的 `ports.SourceDataAmendmentAuthorizer` 要 `party-commercial` 答两件：这次接受后资料修订「许不许」，以及**实际决定方是谁**——头注写明「`Authority` 与 `Decider` 两项一起由 party-commercial 给出，不由调用方声明：`UC-PS-002` 要求『登录操作人不能替代实际决定方』」。仓内没有生产实现，编排在 `cmd/` 也无调用方；票 [ps-port-remainder/03](../../.scratch/ps-port-remainder/issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) 量出原因全在 PC 一侧，MCP-1 代裁（owner 授权，2026-09-07）把三件归 PC 批：加动作格、立合同委派执行器、裁定结果带实际决定方。本记录是那三件的裁决，票 [pc-gaps/08](../../.scratch/party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md) 承接实施。

**今天的形状（取证锚 `92579b0a`）。** `AuthorizedAction` 封闭集只有 `ManualReviewAction` / `ActiveRejectionAction`；`AuthorityGrant.permits` 按动作精确匹配；`Authorize` 答四格（许 / `ErrNotAuthorized` / `ErrAuthorityRulesNotConfigured` / error），`Authorization` 五格里没有一格说「谁决定」；`AuthorizationRequest` 头注写「有意不携带参与方角色」，它也不带请求方是谁。`0003_authorization_grant.sql` 的动作 CHECK 镜像两值，头注预告「撤回与资料修订不在集内，本表不预开空格」。CONTEXT Boundaries 把「版本化角色权限等级、合同委派」列为 PC 拥有，而 `internal/partycommercial/**` 与 `migrations/party_commercial/` 里「委派」零命中——语言有、代码无。

**为什么不能拿既有两格顶替。** `authority_grant.go` 给人工复核与主动拒绝分两格的理由是「获准复核并不等于获准直接拒掉这单业务」；资料修订改的是客户已接受委托的原始资料，与复核、拒绝三者互不蕴含，同一条理由逐字成立。

**为什么决定方要 PC 答。** 请求方可能是客户账户自己，也可能是运营角色代录（`BD-PS-009`「客户可提交请求，授权运营角色可代录」）。代录时登录操作人恰是 `UC-PS-002` 说的不能替代实际决定方的那个人，真正的决定方是把这一决定权交给他的那一方——那是合同层面的关系，PS 不拥有，PC 拥有（Boundaries「合同委派」）。让 PS 自判就是端口头注禁的「调用方声明」。

## Decision

**一、`AuthorizedAction` 加「资料修订」一格（`SOURCE_DATA_AMENDMENT`），与人工复核、主动拒绝并列。** `AuthorityGrant.permits` 按动作精确匹配不变，既有两格一字不动；`ManualReviewRequirementFor` 不动。`0003` 的动作 CHECK 经**新迁移**重建为三值，不改已施加迁移。撤回动作不在本记录——一票一格，撤回归 `PAR-COM-14`「客户及其授权代表」那条线，日后同走本记录的委派路。CONTEXT Rules 授权那句加一句点名资料修订是并列的授权动作（原句不改）。

**二、合同委派归客户合同版本，作一条声明通道；受托方以商业权限等级表达。** 一条委派 = 委派方（该合同的货主客户账户引用**或**其责任法人引用，恰一）× 授权动作（首发只开「资料修订」）× 商业范围引用 × 受托权限等级（`AuthorityLevel`）× 有效区间；同一合同版本内（动作 × 范围 × 等级）唯一。挂 `CUSTOMER_CONTRACT` 版本下（`object_kind = 2`），形照 `0007` / `0012`：随合同版本发布登记（「声明只能随发布」），更正走新合同版本，不开行级改写。**受托方是权限等级而不是操作者主体**：PC 只说「这一范围的资料修订委派给持这一等级的运营角色」，某个操作者持哪一等级是 accessidentity 授予那一格的事（[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) Decision 二），PC 不存操作者。**不按资料组细分**：委派答「谁能替谁提」，允许矩阵（票 pc-gaps/10）答「这一格能不能改」，两问分开。

**三、裁定结果长出实际决定方；请求带请求方。** `AuthorizationRequest` 多带**请求方主体引用 + 种类（客户账户 / 运营角色）**，运营角色代录时还带**所代的客户账户引用**。它不是头注禁的「参与方角色」——那句禁的是拿承运商代理商之类的关系角色换授权；请求方是主体引用，授权仍只来自版本化授权规则加委派。`Authorization` 多一格 `Decider`：

- 请求方是客户账户自己 → 决定方 = 请求方；
- 请求方是运营角色 → 在**有效**委派中按（所代客户账户或其责任法人 × 动作 × 范围 × 等级 × 时点）解出 → 决定方 = 委派方；
- grant 在场而委派缺席 → `ErrNotAuthorized`。**不是未配置**：范围已被授权规则表过态，缺的是客户把决定权交出来——恢复动作是客户在合同里委派，不是租户登记规则（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 按恢复动作分格）；
- grant 缺席照旧 `ErrAuthorityRulesNotConfigured`。

既有两格动作的裁定也带 `Decider`：无委派参与时 = 请求方。四格代数一格不改，消费方不读它就不变。

**四、消费方不自判决定方。** PS 适配器只把 PC 交回的 `Decider` 译成自己的 `DeciderReference`，`Authority` 照撤回、拒绝两只先例取 grant 版本引用。这一条是 PS 端口头注原句的 PC 侧承诺，写在这里是为了让两侧读到同一句。

**五、不做的，逐条写明。**

- 不加撤回格；不裁撤回授权怎么走委派。
- 不做操作者主体 → 权限等级的绑定，不存操作者（ADR-0100 那一半）。
- 不为任何租户拟一条委派或授权规则；`PAR-COM-13`「客户/运营代录请求方与实际决定方、委派和权限」与 `PAR-COM-14` 的取值全部留空，缺省落在既有的 `ErrAuthorityRulesNotConfigured` → PS 未决。
- 不动 `Authorize` 的「范围加时刻」判据与四格代数。
- 不裁委派表的列名与索引——那是实施票的事，形状边界在 Decision 二。

## Consequences

- 迁移一份（序号以开工那刻 `party_commercial` 最大序号 + 1 取，立记录时为 `0025`）：`authorization_grant` 动作 CHECK 重建为三值；新表合同委派父子行，`object_kind = 2`，外键回 `commercial_version`。
- 领域：`SourceDataAmendmentAction`；`ContractDelegation`（委派方恰一由两个构造器分立，不用可空字段——分不清「法人委派」与「忘了填」）；`AuthorizationRequest` 的请求方与所代客户；`Authorization.Decider`；`Authorize` 多收一份有效委派切片。
- 端口：委派读口（按租户 + 所代客户或法人 + 动作 + 范围 + 等级 + 时点取有效委派）与 `PublicationRegistry` 的具名 Save；发布用例多一条 `DeclarationChannel`；受控批文 `declarations` 多一节；三处 `PublicationRegistry` 替身跟随。
- 裁定编排 `AdjudicateCommercialAuthorizationHandler` 多装载委派再交 `domain.Authorize`；公开口仍只有它一个。
- PS 半边（票 ps-port-remainder/03）据此解阻：适配器照撤回那只，`Decider` 取 PC 交回。生产入口（ps-port-remainder/04）接上后停点从「授权未决」变成能答出许 / 拒 / 决定方。
- 管理台：票 admin-write-faces/10 的合同表单多一节「委派」（可加行），由那张票自己加；本记录不建表单。
- CONTEXT：新词条「合同委派」；Rules 授权那句加一句。

### 越权风险点（单列，供 owner 复核）

- **委派缺席答 `ErrNotAuthorized` 而不是未配置。** 我按恢复动作分格：缺的是客户的委派，不是租户的规则。若 owner 认为「客户还没委派」也是一种配置缺席，改一格是 `Authorize` 里一个分支与 PS 适配器的一格翻译，Decision 三改一句。
- **请求方进 `AuthorizationRequest`，与其头注「有意不携带参与方角色」的关系是我读出来的。** 我读作「主体引用不是角色」；若 owner 认为请求方也不该进 PC 的请求，决定方就只能由 PS 侧解——那与端口头注「不由调用方声明」正面冲突，所以我没取。
- **受托方 = 权限等级，是对 CONTEXT「版本化角色权限等级」与「合同委派」并列那一行的一次解释。** 替代是受托方 = 操作者主体，代价是 PC 要存操作者、与 ADR-0100 分工相悖。

## Alternatives considered

- **复用人工复核或主动拒绝的授权格顶替资料修订。** 否决：三个动作互不蕴含，`authority_grant.go` 分两格的那句理由逐字适用。
- **受托方用 ADR-0100 的操作者主体。** 否决：PC 不存操作者；操作者持哪一等级是 accessidentity 授予那一格的事，两处各存一份就是两套口径。
- **委派作第十一类商业对象，有自己的版本生命周期。** 否决：委派是这份合同说的话（CONTEXT 就叫「合同委派」），ADR-0042 归属纪律——声明按拥有对象挂；另起对象类别等于让委派脱离它所在的合同版本演进。
- **委派按资料组细分。** 否决：那是允许矩阵那一维（pc-gaps/10）；两问分开，否则一处改动两张表都要跟。
- **请求方当决定方写死，先让路走通。** 否决：`UC-PS-002`「登录操作人不能替代实际决定方」是硬句；写死等于把 PS 端口头注禁的「调用方声明」搬进 PC。
- **委派缺席答未配置。** 否决理由在 Decision 三与越权风险点第一条。
- **顺手加撤回格。** 否决：一票一格；撤回那条线（`PAR-COM-14`「客户及其授权代表」）要不要走委派、走哪种委派方，归它自己的建模。

## Links

- 票 [pc-gaps/08](../../.scratch/party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md)：缺口事实、六问与倾向、实施与验收
- 票 [ps-port-remainder/03](../../.scratch/ps-port-remainder/issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md)：PS 半边与 MCP-1 代裁（决定方 = 委派方；PC 现在立委派执行器）
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者主体与授予归 accessidentity——受托方以权限等级表达的依据
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格——委派缺席归业务拒绝而不归未配置的判据
- [ADR-0042](./0042-acceptance-content-declarations-by-owning-object.md)：声明按拥有对象挂——委派归客户合同版本的依据
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：PS 适配器落在消费方、集外取值报错不吸收
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：「接单规则包」词条、Rules 授权那句、Boundaries「版本化角色权限等级、合同委派」——本记录给后者第一份执行器，新词条「合同委派」随本记录进 Language
- `internal/partycommercial/domain/authority_grant.go`：`AuthorizedAction` / `AuthorityGrant.permits` / `Authorize` / `Authorization`——本记录在其上加一格、一份委派输入与一格决定方，不改既有判据
- `migrations/party_commercial/0003_authorization_grant.sql`：动作 CHECK 的预告句，本记录兑现其中「资料修订」那一半
- `UC-PS-002` 步骤 4 与 `BD-PS-009`、`PAR-COM-13` / `PAR-COM-14`：硬句与实例半边的出处
