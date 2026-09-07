# 授权动作没有「资料修订」这一格、裁定结果不带实际决定方、合同委派只有语言没有执行器——PS 资料修订授权那口因此接不上

Category: enhancement
Status: in-progress——六问由 [ADR-0116](../../../docs/adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 一次答完（2026-09-07，MCP-6 按 MCP-1 派单 task-aafca372「owner 授权自决口径」裁，越权风险点三条单列在 ADR 里供 owner 复核），PC CONTEXT 新词条「合同委派」+ Rules 加一句已随 ADR 同笔落；实施（迁移 → 领域 → 端口 / PG → 发布用例 / 批文 → 真库端到端）按下面「要建什么」逐笔接，每笔完工报。此前 draft：PC 半边的三件由 MCP-1 2026-09-07 代裁归 PC 批（原文在 [ps-port-remainder/03](../../ps-port-remainder/issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) Comments，立票时该目录只在分支 `mcp2-ps-ports` 上，16:12 随 `61344989` 进 main）；迁移号以开工那刻 `party_commercial` 最大序号 + 1 重取（立票时最大 `0024`）
Blocked by: 无（PC CONTEXT 「授权动作」那句先改再动代码，是本票内部顺序，不是阻塞边）

## 从哪里来

通道 2 在 `ps-port-remainder/03` 用一次 `/domain-modeling` 量出 PS 的 `ports.SourceDataAmendmentAuthorizer` 今天接不到任何
生产实现，原因**全在 PC 一侧**；三问经 MCP-1 代裁（owner 授权，2026-09-07）：**Q1** 实际决定方 = 委派方；**Q2** PC 现在就立
合同委派执行器，并入 PC 批；**Q3** 资料修订的生产入口现在接（拆为 ps-port-remainder/04，通道 2 做）。本票只承接 PC 那一半。

## 缺口事实（锚 main `92579b0a`；分支事实锚 `mcp2-ps-ports` `71b8817b`）

- `internal/partycommercial/domain/authority_grant.go`：`AuthorizedAction` 封闭集只有 `ManualReviewAction` / `ActiveRejectionAction`；
  `AuthorityGrant.permits` 按动作精确匹配。`Authorization` 五格（动作、grant、原因、证据、时刻）**没有任何一格答「实际决定方是谁」**；
  `AuthorizationRequest` 的头注写着「有意不携带参与方角色」，它也不带请求方是谁。
- `migrations/party_commercial/0003_authorization_grant.sql`：`authorization_grant_action_closed` 的 CHECK 镜像两值，头注明写
  「撤回与资料修订不在集内，本表不预开空格」——加格要新开迁移重建这条 CHECK，不改 `0003`。
- `application/adjudicate_commercial_authorization.go`：`AdjudicateCommercialAuthorizationHandler.Handle` 装载 grants 后交
  `domain.Authorize`，四格（许 / `ErrNotAuthorized` / `ErrAuthorityRulesNotConfigured` / error）；没有委派参与。
- PC CONTEXT Boundaries 把「版本化角色权限等级、合同委派」列为 PC 拥有；`internal/partycommercial/**` 与 `migrations/party_commercial/`
  里「委派」零命中（`git grep 委派|Delegat`）——语言有、代码无。
- PS 消费侧：`ports.SourceDataAmendmentAuthorizationQuery{Identity, Scope, Requester, Reason}` → `SourceDataAmendmentAuthorization{Outcome,
  Authority, Decider}`，头注「`Authority` 与 `Decider` 两项一起由 party-commercial 给出，不由调用方声明：`UC-PS-002` 要求『登录操作人不能
  替代实际决定方』」。先例适配器 `adapters/partycommercial/withdrawal_authorization.go` 头注已记「PC 的 `AuthorizedAction` 今天也没有
  撤回动作」——同一格缺口的另一面，本票不接它。

## 为什么是机制半边

- 「哪些动作可被授权」是 PC 领域的封闭集（CONTEXT Rules：主动拒绝授权按责任法人、角色、适用范围和有效期间版本化）；资料修订是
  `UC-PS-002` 点名的一个授权动作（步骤 4「校验请求方、实际决定方、授权角色、委派和适用时点」），集里没有它是形状缺口，不是取值缺口。
- 「实际决定方」是 UC 的硬句，靠委派解出；委派是 CONTEXT 声明拥有的对象。**没有执行器，租户根本没有办法登记一条委派**——这与
  spec 四票同一句：类型都还没有，何谈取值。
- 实例半边照旧留空：谁委派给谁、哪个范围、`PAR-COM-13`「客户/运营代录请求方与实际决定方、委派和权限」、`PAR-COM-14` 授权规则取值，
  全部待提供；拒默认落在既有的 `ErrAuthorityRulesNotConfigured` → PS `AuthorizationRulesNotConfigured` → 未决。

## 要你答的问题（`/domain-modeling` 一次答完；答案落 PC CONTEXT + ADR-0116）

1. **委派的受托方用什么身份表达？** 候选：(a) `AuthorityLevel`（CONTEXT「版本化角色权限等级」，与「合同委派」并列为 PC 拥有）；
   (b) ADR-0100 的操作者主体。倾向 **(a)**：PC 只说「这个范围的资料修订委派给持这一等级的运营角色」，某个操作者持哪一等级是
   accessidentity 授予（ADR-0100 Decision 二）那一格的事，PC 不存操作者。
2. **委派挂在哪个对象下？** 候选：(a) 客户合同版本下的声明通道（CONTEXT 就叫「合同委派」；形照 `0007` / `0012` 挂 `CUSTOMER_CONTRACT`，
   随合同版本发布、随 `PublishCommercialAuthorityHandler` 加一条 `DeclarationChannel`）；(b) 新开第十一类商业对象。倾向 **(a)**：
   ADR-0042 归属纪律——委派是这份合同说的话；不为它另起版本生命周期。
3. **委派的格。** 倾向：委派方（客户账户引用或责任法人引用，恰一）× 动作（首发只开「资料修订」）× 商业范围引用 × 受托权限等级 ×
   有效区间；同一（动作 × 范围 × 等级）在同一合同版本内唯一。**要不要**再按资料组细分（`PAR-COM-13` 提到「允许资料组与字段」）？
   倾向不——资料组是允许矩阵（票 [10](./10-source-data-amendment-allowance-is-a-third-stage-content-family.md)）那一维，委派答
   「谁能替谁提」，矩阵答「这一格能不能改」，两问分开。
4. **`Authorization` 长出 `Decider` 之后，请求怎么带请求方？** `AuthorizationRequest` 今天不带请求方。倾向：加「请求方主体 + 种类
   （客户账户 / 运营角色）」——它不是「参与方角色」（头注禁的是拿承运商代理商之类的关系角色换授权），是主体引用；授权仍只来自
   grant + 委派。裁定：请求方是客户账户自己 → `Decider` = 请求方；请求方是运营角色 → 在该合同版本的委派里找（动作 × 范围 × 等级）
   命中的委派方 → `Decider` = 委派方；找不到 → `ErrNotAuthorized`（不是未配置——grant 在场说明范围已被表过态）。
5. **既有两格（人工复核、主动拒绝）的裁定要不要也带 `Decider`？** 倾向：带——无委派时 `Decider` = 请求方，四格代数一字不改，
   `ManualReviewRequirementFor` 不动；消费方不读它就不变。撤回动作（`PAR-COM-14`「客户及其授权代表」）日后同走这条委派路，
   **但本票不加撤回格**：一票一格，撤回归它自己那条线。
6. **要不要 ADR。** `AuthorizedAction` 加格不要（封闭集扩一格是 CONTEXT 改动，与 0003 头注的预告一致）；委派 → 实际决定方**要**——
   PC 授权模型多一维（谁替谁），且撤回那口日后复用。MCP-1 已预留 0116。

## 裁决（2026-09-07，MCP-6；正文在 ADR-0116，此处只对号）

① 受托方 = `AuthorityLevel`（权限等级），不存操作者（Decision 二）；② 委派挂 `CUSTOMER_CONTRACT` 版本下作声明通道，形照 `0007` / `0012`，不新开对象类别（Decision 二）；③ 委派格 = 委派方（客户账户 | 责任法人，恰一）× 动作（首发只开资料修订）× 商业范围 × 受托等级 × 有效区间，（动作 × 范围 × 等级）版本内唯一，**不按资料组细分**（Decision 二）；④ `AuthorizationRequest` 带请求方主体 + 种类 + 代录时所代客户账户，`Authorization.Decider`：客户自己 → 请求方；运营角色 → 有效委派解出委派方；grant 在场而委派缺席 → `ErrNotAuthorized`，grant 缺席照旧未配置（Decision 三）；⑤ 既有两格也带 `Decider`（无委派 = 请求方），四格代数不改，撤回格不加（Decision 一、三、五）；⑥ 要 ADR，即 0116。**越权风险点三条**（委派缺席归拒绝而非未配置 / 请求方进请求与头注「不携带参与方角色」的读法 / 受托方 = 权限等级的解释）单列在 ADR-0116，等 owner 复核；任一条被推翻都是一处改动，ADR 相应改一句。

## 要建什么（裁决已落，逐笔实施；每笔 pathspec 提交）

PC CONTEXT（授权动作那句加「资料修订」；「合同委派」词条补形状）→ ADR-0116 → 迁移（重建 `authorization_grant` 的动作 CHECK；
新表 合同委派 父子行，`object_kind = 2`，形照 `0012`）→ 领域（`SourceDataAmendmentAction`；`ContractDelegation`；`Authorization.Decider`；
`Authorize` 长委派解析）→ 端口（委派读口 + 发布用例 `ContractDelegationChannel`）→ postgres → `cmd/parcel-commercial` 批文一节 →
真库端到端（形照 pc-gaps/07 四笔 `6dc3db12..265da8d8` 进 main 的那一组）。PS 适配器不在本票。

**签名纪律（实施前读）**：`domain.NewAuthorizationRequest` 与 `Authorization` 在 PS 地盘有调用点（`internal/parcelshipment/adapters/partycommercial/withdrawal_authorization.go`、`active_rejection.go` 及其测试）。给请求带请求方走**三步法**——先加带请求方的新构造器、旧构造器原样保留（旧路裁出的 `Decider` 为空且只对既有两格动作成立），PS 侧迁完调用点后再删旧的；或与 MCP-1 约一个「全仓可能编不过」的窗口一次改完。**不在 PC 分支上直接改 PS 文件**（parallel-sessions「会让旧调用点对不上」那一类）。

## 在等本票的

- `ps-port-remainder/03` PS 半边：`adapters/partycommercial/source_data_amendment_authorization.go` 照撤回那只，`Decider` 取 PC 交回的。
- `ps-port-remainder/04`（资料修订生产入口）接上后编排会停在授权未决，本票落地后那一格才能真的答出「许 / 拒 / 决定方」。
- 连带（不是等）：`admin-write-faces/17` 的取消授权表单与本票的授权动作格无关，票面已写明别混。

## 红线

- 不复用既有两格授权动作顶替资料修订（「获准复核并不等于获准直接拒掉」对这一格逐字成立）；不让 PS 自判决定方。
- 无规则仍是`未配置`不是拒绝；不为任何租户拟一条委派或授权规则；实例值留空。
- 不改已施加迁移 `0003`；CHECK 重建走新序号。
- 领域包不依赖 HTTP / pgx；集外取值报错不吸收；不开行级 UPDATE / DELETE，委派的更正走新合同版本。
- 撞上要改「主动拒绝授权必须按责任法人、角色、适用范围和有效期间版本化」这类硬句、或委派归属要跨出 PC → 停下报 MCP-1。

## 参照

`internal/partycommercial/domain/authority_grant.go`（`AuthorizedAction` / `AuthorityGrant.permits` / `Authorize` / `Authorization`）；
`application/adjudicate_commercial_authorization.go`；`migrations/party_commercial/0003_authorization_grant.sql` 头注；PC CONTEXT Rules
「接单规则包必须明确……哪些授权角色可以主动拒绝」与 Boundaries「版本化角色权限等级、合同委派」；`UC-PS-002` 步骤 4 与 `BD-PS-009`；
`PAR-COM-13` / `PAR-COM-14`；ADR-0025、ADR-0042、ADR-0100；PS `ports.SourceDataAmendmentAuthorizer` 一族与
`adapters/partycommercial/withdrawal_authorization.go`。

## Comments

- 2026-09-07 · MCP-6（task-aafca372，分支 `mcp6-awf07`）：立票（draft）。**只写票面，未动代码、未改 CONTEXT。** 素材是
  `ps-port-remainder/03` 的裁决与 Comments（`git show mcp2-ps-ports:…`），PC 侧事实在 `92579b0a` 上逐条重取（`authority_grant.go`
  全文、`0003` 全文、裁定编排全文、CONTEXT 两句、`git grep 委派`）。能力边界：**没读** `AuthorityGrantStore` 的 postgres 实现与
  三处 `PublicationRegistry` 替身——委派表的 Save 形状开工时以代码为准。「要你答的问题」里的倾向是本会话的建模意见，不是裁决。
- 2026-09-07 · MCP-6（同一会话，稍后）：**裁决落 ADR-0116**，六问按上面「倾向」取（能力边界与越权风险点写在 ADR 头部与文末），
  PC CONTEXT 新词条「合同委派」+ Rules 加一句，本票转 in-progress。同笔只有文档；代码从下一笔起。
- 2026-09-07 18:09 · MCP-6（17:4x 新绑会话，按 MCP-1 18:05 广播补记）：**封存出处。** 立票笔 `8c1e6ed5`（分支 `mcp6-awf07`）
  → main `52001f02`；ADR-0116 与本票转 in-progress 那一笔 `c9ec35d7` → main `e374b1ea`（远端 main，MCP-1 `git ls-remote` 18:04:16）。
  分支指针留着，树已拆；对照全表在 `admin-write-faces/07` 的同时刻 Comment。
- 2026-09-07 20:0x · MCP-5（18:59 新绑会话，接 18:5x 旧会话对 task-48e13cf1 的接管；分支 `mcp5-pcgaps08`，基线 `299f2a2e`）：
  **代码半边三笔落分支，未进 main。** 「要建什么」逐项对号：
  - `1e182860`：领域——`AuthorizedAction` 加 `SourceDataAmendmentAction`；`NewAuthorizationRequestBy` 带请求方（`RequestedByCustomerAccount` /
    `RequestedByOperatorRole(operator, onBehalfOf)`），旧 `NewAuthorizationRequest` 原样保留、拒资料修订（`ErrAuthorizationRequesterRequired`）；
    `Delegator`（客户账户 | 责任法人，两构造器分立）、`Decider`（客户账户 | 责任法人 | 运营角色，只由 `Authorize` 解出、无公开构造器）、
    `ContractDelegation` / `ContractDelegationContent`（版本内（动作 × 范围 × 等级）唯一、至少一条）；`Authorize(grants, delegations, request)`
    ——客户自己 → 决定方 = 请求方；运营角色请求既有两格 → 决定方 = 运营角色；运营角色代录资料修订 → 有效委派解出委派方；grant 在场而
    委派缺席 → `ErrDelegationAbsent`（Is `ErrNotAuthorized`）；grant 缺席照旧未配置。端口 `EffectiveContractDelegationView`。编排
    `NewAdjudicateCommercialAuthorizationHandlerWithDelegations`，只在运营角色代录资料修订时装载委派；旧构造器装配的编排走到那一格报错、
    不装成空切片。postgres `authorizedActionFrom` 认第三值。迁移 **0025**：`authorization_grant_action_closed` DROP + ADD 三值（不改 0003）；
    `contract_delegation_content` / `contract_delegation` 父子表，`object_kind = 2`，外键回 `commercial_version`，子表主键含
    （action × scope_ref × authority_level），action CHECK 首发只 `SOURCE_DATA_AMENDMENT`，委派方「种类 + 引用」两列。
  - `6b43484c`：端口 `ContractDelegationContentView.LoadContractDelegations` + `PublicationRegistry.SaveContractDelegations`；postgres
    `CommercialPublications.SaveContractDelegations`（先读后写，重放 / 冲突一行不写）+ `ContractDelegations` 读适配器（点读 + 按范围时点装载，
    联 `commercial_version` 快照重建合同版本）；两处 `PublicationRegistry` 替身 + transaction_guard 一行；真库七组含裁定端到端。
  - `df923d16`：发布用例 `CommercialDeclarations.ContractDelegations` + `ContractDelegationChannel`（`CONTRACT_DELEGATION`）；受控 CLI 批文
    `declarations.contractDelegations[] { delegatorKind, delegator, action, scope, level, effectiveStartsAt, effectiveEndsAt? }`；真库端到端两读口读回。
  **验证强度**：每笔 gofmt 空 / build 0 / vet 0；`go test -count=1` PC 全部包 + PS-PC 适配器 + SA-PC 适配器 + migrations + cmd/parcel-commercial
  全 ok，DSN 已设、PG 用例 `-v` 下 PASS 非 SKIP；`df923d16` 另在 detached 树 `idp-parcel-mcp5-verify-08` 跑全仓 `go test -p 1 -count=1 ./...`
  （含 DSN）：99 ok / 16 无测试 / 0 SKIP / **1 FAIL**——`internal/architecture` 的 `TestNoNewProductionFactoryGoesUnwired` 点名
  `RequestedByCustomerAccount` / `RequestedByOperatorRole` 无生产调用点且不在基线里。第四笔把两条加进 `production_wiring_baseline.txt`
  并写明为什么现在不接（消费适配器在 PS 地盘，ps-port-remainder/03 落地那天出名单），detached 检出上两法同得 6 → 8；该笔只动 `.txt` 与本
  `.md`，无 `.go` / `.sql`，全仓结果沿用 `df923d16` 那一跑 + `go test ./internal/architecture/` ok。**一字未动**：PS 两只适配器及其测试、
  `cmd/parcel-api` 两处装配（仍走旧构造器，三步法的迁移与删旧归 ps-port-remainder/03 或一张 PS 侧票）；`docs/product/MECHANISM-INVENTORY.md`
  （推送方在 tip 上重生成兑底）。**能力边界**：没读 `AuthorityGrants` 之外任何消费方对 `Authorization` 的读法，`Decider` 的 PS 侧翻译按
  ADR-0116 Decision 四留给 ps-port-remainder/03。票面 Status 待 MCP-1 重放进 main、广播远端 SHA 后转 resolved（新旧 SHA 对照记在那一条）。
