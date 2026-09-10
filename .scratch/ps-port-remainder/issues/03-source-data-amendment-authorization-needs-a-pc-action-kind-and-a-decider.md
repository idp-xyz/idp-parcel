# `SourceDataAmendmentAuthorizer`：PC 授权动作没有「资料修订」这一格、裁定结果不带实际决定方；PS 适配器照撤回那只

Category: enhancement
Status: resolved——2026-09-10 15:1x 通道 2 交活（task-0450f54b，分支 `mcp2-psr03` tip `27607caa`，基 `dd5ed934`；三笔 `39c1206d` / `07fe7930` / `27607caa`，见「完成记录」；进 main 的 SHA 由推送方重放后补记）；此前 in-progress——14:4x 通道 2 接手；此前 ready-for-agent——2026-09-10 14:2x 通道 1 解阻（远端 main `5dda0fb2` 上取证）：PC 半边已随 [pc-gaps/08](../../party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md) 进 main（`0cfe3571` 领域 + 端口 + 迁移 0025 / `31903cc5` 合同委派册 / `02ff5205` 发布通道 + 批文；ADR-0116，owner 复核 2026-09-09 认可），生产入口已随本目录 [04](./04-amendment-production-entry-mechanism-half.md) 落 `cmd/parcel-api` 且今天接的是 `UnconfiguredSourceDataAmendmentAuthorizer{}`；剩 PS 半边（适配器照撤回那只 + 换掉那只未配置 + 剪基线两条），见 Comments 末条。此前 blocked——三问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments）；PC 半边等 pc-gaps 批；「顺带量到」的生产入口机制半边已按代裁拆到 04 承接
Blocked by: 无（PC 半边 pc-gaps/08 已进 main）

## 端口今天说什么

`ports.SourceDataAmendmentAuthorizer.AuthorizeSourceDataAmendment(ctx, SourceDataAmendmentAuthorizationQuery{Identity, Scope, Requester, Reason}) (SourceDataAmendmentAuthorization{Outcome, Authority, Decider}, error)`。头注：`Authority`（所采用授权依据快照）与 `Decider`（实际决定方）「两项一起由 party-commercial 给出，不由调用方声明：`UC-PS-002` 要求『登录操作人不能替代实际决定方』」；「真实请求方、实际决定方与授权入口仍是 `BD-PS-009` 待确认的实例参数」。消费方 `AmendCustomerSourceDataHandler` 四格：`Granted` 继续并把 `Decider` / `Authority` 写进资料版本；`Refused` → 业务拒绝；`RulesNotConfigured` → 未决（`SourceDataAmendmentAuthorityRulesNotConfigured`）；error → 未决（权威答不出）。仓内无生产实现；编排在 `cmd/` 无调用方。

## 语言从哪里来

- UC-PS-002 步骤 4：「`party-commercial` / `parcel-shipment` 校验请求方、实际决定方、授权角色、委派和适用时点」；参与者表：`party-commercial`「提供当前客户、责任法人、合同、产品、**委派**和资料修改权限」；`BD-PS-009` 建议「客户可提交请求，授权运营角色可代录，版本决定仍由本用例按权限形成」。
- PS CONTEXT：「接受后资料版本必须明确关联……请求方、实际决定方、授权快照」；`RequesterReference` 与 `DeciderReference` 在领域里刻意分立（`customer_source_data.go` 头注）。
- PC 领域今天：`AuthorizedAction` 封闭集只有 `ManualReviewAction` / `ActiveRejectionAction`；`AuthorityGrant` 按（动作 × 责任法人 × 权限等级 × 商业范围 × 有效区间）授予；`Authorize` 只答「许不许」（`Authorization` 带 grant 版本、原因、证据、时刻），**没有任何一格答「实际决定方是谁」**；PC CONTEXT Boundaries 把「合同委派」「版本化角色权限等级」列为 PC 拥有，但代码里没有委派的执行器（`party-commercial-context-gaps` 四票也没列它）。
- 先例：`adapters/partycommercial/withdrawal_authorization.go` 与 `active_rejection.go`——`RequestSource`（实例半边：法人、等级、范围、证据、时点、**动作**）折成 `pcdomain.AuthorizationRequest`，交 `AdjudicateCommercialAuthorizationHandler`，四格按恢复动作翻译；撤回那只的头注已经记过同一件事：「PC 的 `AuthorizedAction` 今天也没有撤回动作——扩它属那笔参数落地时的 PC 侧工作（按 AGENTS 先改 PC CONTEXT 再动代码）」。主动拒绝的 `Decider` 由命令携带（运营角色自报、经 PC 校验），资料修订这一口刻意反过来——请求方可能是客户也可能是代录的运营角色，谁是「实际决定方」要看委派，所以让 PC 答。

## 裁决（通道 2，2026-09-07；owner 授权自决口径；取证锚远端 main `ffa6bd0e`）

**① 机制半边现在能立什么。** 两半，先后有序：

- **PC 半边（先，两件）**：(a) `AuthorizedAction` 封闭集加「**资料修订**」一格（先改 PC CONTEXT 授权动作那句，再动 `authority_grant.go`；`AuthorityGrant.permits` 按动作精确匹配，加格不影响既有两格）；(b) 裁定结果长出「**实际决定方**」：请求方是客户账户自己 → 决定方即请求方；请求方是运营角色代录 → 决定方由**合同委派**解出（哪个客户/法人把这一范围的资料修订委派给了这个角色），没有委派就是 `ErrNotAuthorized`——这一件要先给委派一个执行器（PC 今天只有语言没有代码），是 PC 自己的建模票，本票只点名。
- **PS 半边（后）**：适配器 `adapters/partycommercial/source_data_amendment_authorization.go` 照撤回那只：`RequestSource` 把查询折成带「资料修订」动作的 `AuthorizationRequest`（法人、等级、范围、证据、时点仍是实例半边，折不出 → error 未形成，不译成未配置）；PC 四格译四格；`Authority` = grant 版本引用（`对象/版本`，与撤回、拒绝同形）；`Decider` 取 PC 交回的实际决定方。**PS 不自己判决定方**——那正是端口头注禁止的「调用方声明」。

**② 实例半边留什么、在哪一格拒默认。** `BD-PS-009`（生产入口、请求方的采信、代录角色）与 `PAR-COM-14`（授权规则取值）待确认；拒默认落在 PC 的 `ErrAuthorityRulesNotConfigured` → PS `AuthorizationRulesNotConfigured` → 未决（已有）。**不得**为了让路走通把请求方当决定方写死，也不得复用人工复核或主动拒绝的授权格顶替资料修订——「获准复核并不等于获准直接拒掉」那句对这一格逐字成立。

**③ 形状照哪个先例。** `WithdrawalAuthorizationAdapter` / `ActiveRejectionAdapter`（RequestSource + 裁定编排 + 四格翻译），ADR-0025。

**④ 要不要 ADR。** 加动作格不要（封闭集扩一格是 CONTEXT 改动）；**委派→实际决定方的建模可能要**——它决定 PC 授权模型多一维（谁替谁），且撤回授权那口（`PAR-COM-14`「客户及其授权代表」）日后也走它；归 PC owner 在建模那一步判。

## 顺带量到、不在本票

资料修订编排没有生产入口：`NewAmendCustomerSourceDataHandler` 在 `cmd/` 零命中，与 ADR-0106 之前的受控补充编排同一形。入口的机制半边（`cmd/parcel-api` 端点 + `UnconfiguredIntake{}` + 来源保全/修订两层边界壳，形照 `supplementBoundary`）今天就能做且不依赖本票三口——接上后编排会如实停在授权未决 / 矩阵未登记，那两格正好由本目录 02、03 两票解。它是 `BD-PS-009` 的机制半边，要不要现在立票归 owner。

## 要你答的问题

1. **运营角色代录时，实际决定方是谁**：委派方（客户/其法人）还是代录的运营角色本身？我的倾向：**委派方**——UC 说「登录操作人不能替代实际决定方」，代录的人恰是登录操作人；运营角色自身作为决定方只在它以自己的权限（而非代客户）修订时成立，那是另一种授权动作。
2. **PC 要不要现在给合同委派立执行器**（表 + 登记口 + 解析）？没有它 (b) 落不了；有它撤回授权那口也一并受益。这是 PC 批的排期问题，建议并入 `party-commercial-context-gaps` 立第 08 票。
3. **资料修订的生产入口现在接不接**（上一节）？

## 红线

- 不复用既有两格授权动作；不让 PS 自判决定方；无规则仍是未决不是拒绝。
- PC 半边不在 PS 地盘动手；PC CONTEXT 先于代码改。

## 参照

`ports.SourceDataAmendmentAuthorizer` 一族头注；`application/amend_customer_source_data.go` 授权那一格；`domain/customer_source_data.go` 的 `RequesterReference` / `DeciderReference`；`internal/partycommercial/domain/authority_grant.go`；`adapters/partycommercial/withdrawal_authorization.go`、`active_rejection.go`；UC-PS-002 步骤 4、「权限、隐私与审计」、`BD-PS-009`；PC CONTEXT Boundaries「合同委派」「版本化角色权限等级」；ADR-0025。

## 完成记录（通道 2 · 2026-09-10 · 分支 `mcp2-psr03`，基 `dd5ed934` = 当时的 origin/main）

**三笔，各自可单独 cherry-pick，但前两笔请连着放**（理由见第二笔的披露）。

1. `39c1206d` feat(parcelshipment)——① 适配器。新增 `internal/parcelshipment/adapters/partycommercial/source_data_amendment_authorization.go` 与 `_test.go`；票面 Status → in-progress。
2. `07fe7930` feat(parcel-api)——② 装配 + ③ 基线。`cmd/parcel-api/assemble_customer_amendment.go`（`buildCustomerAmendmentOrchestration` 经新增的 `buildSourceDataAmendmentAuthorizer` 换真适配器；函数注释改写）、`assemble_customer_amendment_test.go`（两处停点断言改写、用例改名 `TestTheProductionAmendmentAssemblyStopsAtUnformedAuthorization`）、`internal/architecture/production_wiring_baseline.txt`（剪 PC 组两行 + 头注一段 + 该组段落改成「曾在这里」形）。**披露**：`39c1206d` 单独检出时 `TestWiringBaselineHasNoStaleEntry` 对那两条已红——适配器一接上它们就成了真引用；本笔剪掉后绿。
3. `27607caa` chore(inventory)——机制清点在 `07fe7930` 的干净 detached 检出上重生成（parcelshipment 生产文件 +1，PS→PC 消费缝文件 +1）。

**触及**：上述六个文件 + 本票面。**未碰**：`internal/partycommercial/**`（PC 半边一件没补，缺什么都在 main 上找到了）；`unconfigured_source_data_amendment.go` 及其用例（按派单保留；它头注里「提供方那半落地后……本类型随之退场」与「票 ps-port-remainder/03」那两句自本票起已是历史——不在本票改，因为两张 PS 票同期在该包各建新文件、都不碰既有文件，记在下面「给评审的判断题」末条）；`assemble_withdrawal.go` / `assemble_review.go`（仍走旧构造器，三步法删旧不在本票）；ADR-0116 / ADR-0025 正文；任何 `.sql`。

**裁决①的 PS 半边逐项对照**

- 「适配器照撤回那只」——`SourceDataAmendmentAuthorizationAdapter` 与 `WithdrawalAuthorizationAdapter` 同形：RequestSource 接口 + 适配器 + 四格翻译；`ErrUntranslatableAnswer` 复用本包既有那只。**一处有意的形状差**：撤回那只的 RequestSource 交回整份 `AuthorizationRequest`，本口的 RequestSource 交回 `SourceDataAmendmentRequestCoordinates`（法人、等级、范围、原因、证据、时点 + 请求方种类 + 代录时的运营角色引用），**动作与所代客户账户由适配器钉死**（`formSourceDataAmendmentRequest`：动作只能是 `SourceDataAmendmentAction`，所代账户只能是询问身份上的那一个）。理由两条：(a) 红线「不复用人工复核 / 主动拒绝两格顶替资料修订」从「映射自觉」变成结构——映射连挑动作的位置都没有；(b) 派单 ③ 要求 `RequestedByCustomerAccount` / `RequestedByOperatorRole` 随本适配器出名单，那只能发生在适配器自己调它们时；若照撤回那只让映射交整份请求，两扇门仍只在测试替身里被调、基线两行剪不掉。
- 「RequestSource 折成带资料修订动作的 AuthorizationRequest」——经 `NewAuthorizationRequestBy(requester, SourceDataAmendmentAction, …)`，requester 按坐标里的 `RequesterKind` 走 `RequestedByCustomerAccount(account)` 或 `RequestedByOperatorRole(coordinates.OperatorRole, account)`。
- 「法人、等级、范围、证据、时点仍是实例半边，折不出 → error 未形成，不译成未配置」——`formed=false`、映射 `nil`、映射没说请求方种类，三格都是 error 且不问提供方（各有用例）。
- 「PC 四格译四格」——许 → `Granted`；`ErrNotAuthorized`（`errors.Is`，因此含 `ErrDelegationAbsent`）→ `Refused`；`ErrAuthorityRulesNotConfigured` → `RulesNotConfigured`；其余 error（含 PC 编排「委派读口未接线」那条 error——有用例钉它不被读成拒绝）。
- 「`Authority` = grant 版本引用 `对象/版本`」——`NewAmendmentAuthoritySnapshot(ObjectID + "/" + Version)`，与撤回、拒绝同形。
- 「`Decider` 取 PC 交回的实际决定方，PS 不自判」——`authorized.Decider()` 的 `Reference()` 经 `NewDeciderReference` 转写；`named=false` 交 `ErrUntranslatableAnswer`，不拿请求方顶上。只搬 Reference 不搬 Kind（判断题 2）。

**裁决②对照**：实例半边留空拒默认——生产装配 RequestSource 为 `nil`，编排停在 `SourceDataAmendmentAuthorityUnavailable`（不是 `RulesNotConfigured`：没人问到登记册，说未配置是假话）；没有任何一处把请求方当决定方、没有任何一处复用复核 / 拒绝的授权格；`RequesterKind` 不可空（零值 → error），理由与 PC 两个请求方构造器分立同源。

**验证强度（作者层，全部在 detached 检出 `27607caa` 上，不含任何人在途改动）**：`gofmt -l ./cmd ./internal` 空；`go build ./...` / `go vet ./...` 全仓 0。`go test -p 1 -count=1 -v` 带 DSN（`IDP_PARCEL_POSTGRES_DSN` 指 55432），包组 = 动过的包 + 反向依赖（`go list -f '{{.ImportPath}} {{.Deps}}' ./...` 筛 `parcelshipment/adapters/partycommercial` 得 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/postgres`）+ `internal/parcelshipment/application` + `internal/architecture/...`：7 包全 ok，**PASS 1085 / SKIP 0 / FAIL 0**；两个真库停点用例 `TestTheProductionAmendmentAssemblyStopsAtUnformedAuthorization` / `TestTheHonestStopIsObservableAtTheAmendmentEndpoint` 在 `-v` 下是 PASS 不是 SKIP。未跑全量（推送方跑）。红先于绿：适配器用例先编不过（未定义符号），装配用例先在真库上红（读回 `RULES_NOT_CONFIGURED`），再各自转绿。

**基线重数（钉 SHA）**：`git show` 取父提交 `39c1206d` 的 `production_wiring_baseline.txt`（blob 与基 `dd5ed934` 逐字节相同）两法同得剪前 **3**，本笔 `07fe7930` 单独作用于其上剪后 **1**（party-commercial 2→0、parcel-shipment 1 不变）；两法是 UTF-8 逐行滤非空非注释与 `git grep -c '^internal/'`。只对该检出成立；重放时父提交若不是 `39c1206d`，推送方在自己那一刻的 HEAD 上重取。成因三分是第二种，剪前核过全仓两名各只一处声明、本笔新增符号不同名。

**给评审的判断题**

1. **「决定方从 PC 取、PS 一处都没判」怎么证**——三层：(a) 结构：`pcdomain.Decider` 没有公开构造器，`source_data_amendment_authorization.go` 里唯一写 `Decider` 那一格的来源是 `authorized.Decider()`，全文没有任何分支读 `query.Requester` 或 `query.Identity` 去决定「决定方是谁」（`query.Identity.CustomerAccountID()` 只用作 PC 请求方的 account / onBehalfOf，那是「请求方」不是「决定方」）；(b) 用例：`TestSourceDataAmendmentByAnOperatorRoleNamesTheDelegatorAsDecider` 两格里请求方是 `operator-1`、询问身份上的账户是 `customer-1`，法人委派那一格期望的决定方是 `legal-1`——三个串互不相同，PS 若拿请求方或身份账户顶替，两格必有一格红；(c) 反面：`TestSourceDataAmendmentByAnOperatorRoleIsRefusedWithoutADelegation` 证没有委派时 PS 不会「顺手」把谁写成决定方。评审可再核一件我没法用测试证的：适配器文件里 `NewDeciderReference` 只出现一次。
2. **只搬 `Reference` 不搬 `Kind`**：PS 的 `DeciderReference` 是单一引用串，PS 领域不按决定方种类分流。代价是客户账户引用与法人引用若同串会在 PS 留痕里不可分（两者都是租户内引用，实测碰撞概率低，但不是零）。若评审认为该带种类，改法是 `Kind/Reference` 拼串——要先改 PS 域对该串的读法，不在本票。
3. **`RequesterKind` 放在 RequestSource（实例半边）而不由适配器按 `query.Requester == Identity.CustomerAccountID()` 推断**：后者是对 `RequesterReference` 串语义的一条硬编码采信，正是 `BD-PS-009`「请求方的采信、代录角色」待确认的东西；推断一次就是一条默认。代价是坐标结构多两格。
4. **两笔连放**：`39c1206d` 单独在 main 上会让 `internal/architecture` 那道门红。若推送方要求每笔各自绿，可把 ③ 那两行的剪法并回第一笔重放——内容不变，只是分笔不同。
5. **不在本票改的陈旧注释**：`unconfigured_source_data_amendment.go` 头注仍说「提供方那半还没立」「本类型随之退场」，而它今天没有退场（派单要求保留）。建议随 psr/01 落地后、该包不再有并行新文件时，由一张簿记小票把头注改成「保留为无 PC 库路径的如实答复」。

## Comments

- 2026-09-07 · 通道 2：立票（draft），一次 `/domain-modeling` 的产物。**只写票面，未动代码。** 能力边界：读过端口、编排授权段、PS 两个引用类型、PC `authority_grant.go` 全文、两只先例适配器全文、UC-PS-002 全文；**没读** `AdjudicateCommercialAuthorizationHandler` 的编排细节与 PC 有没有任何委派相关的表——「委派无执行器」是按 `party-commercial-context-gaps` 四票清单与 PC 迁移目录名反推的，PC owner 开工时以代码为准。
- 2026-09-07 · MCP-1 代裁，owner 授权（task-b77525c9，由通道 2 落票面）：**Q1** 实际决定方 = 委派方；**Q2** PC 现在立合同委派执行器——是，并入 PC 批队列（MCP-3 当前批后，pc-gaps/08 起），MCP-1 已知会 MCP-3；**Q3** 生产入口现在接——是，拆为本目录 04 票由通道 2 实施。Status 由 draft 改 blocked（等 PC 半边）。
- 2026-09-10 14:2x · 通道 1（解阻簿记，未动代码；取证于远端 main `5dda0fb2`）：**PC 半边两件都在 main 上了**，PS 半边转 ready-for-agent。对号：(a) `AuthorizedAction` 加 `SourceDataAmendmentAction`、`NewAuthorizationRequestBy` 带请求方（`RequestedByCustomerAccount` / `RequestedByOperatorRole(operator, onBehalfOf)`）——`0cfe3571`；(b) `Decider`（客户账户 | 责任法人 | 运营角色，只由 `Authorize` 解出）、`ContractDelegation` 族、端口 `EffectiveContractDelegationView`、编排 `NewAdjudicateCommercialAuthorizationHandlerWithDelegations`（旧构造器保留，走到运营角色代录资料修订那一格报错、不装成空切片）——`0cfe3571` / `31903cc5`；发布通道 `CONTRACT_DELEGATION` + 批文——`02ff5205`。裁决口径与 ADR-0116 一致（Q1 实际决定方 = 委派方；grant 在场而委派缺席 → `ErrDelegationAbsent`，Is `ErrNotAuthorized`）。**PS 半边今天剩三件**：① 适配器 `adapters/partycommercial/source_data_amendment_authorization.go` 照撤回那只（`RequestSource` 折成带资料修订动作的 `AuthorizationRequest`；PC 四格译四格；`Authority` = grant 版本引用；`Decider` 取 PC 交回的实际决定方，PS 不自判）；② `cmd/parcel-api` `buildCustomerAmendmentOrchestration` 把 `UnconfiguredSourceDataAmendmentAuthorizer{}` 换成真适配器（`pcpostgres.NewAuthorityGrants` + 委派读适配器 + 带委派的裁定编排；`RequestSource` 与撤回那只同样留 nil——实例半边）；③ `production_wiring_baseline.txt` 里 pc-gaps/08 留下的 `RequestedByCustomerAccount` / `RequestedByOperatorRole` 两条随之出名单（那两行上方的理由写的就是「ps-port-remainder/03 落地那天」）。派单另记。
- 2026-09-10 15:1x · 通道 2（作者，task-0450f54b）：**PS 半边三件落地**，分支 `mcp2-psr03` tip `27607caa`（基 `dd5ed934`），三笔 `39c1206d` / `07fe7930` / `27607caa`，逐项对照、验证强度、基线重数与评审判断题见上方「完成记录」。走了 /tdd：适配器用例先编不过再转绿，装配用例先在真库上红（读回 `RULES_NOT_CONFIGURED`）再转绿。占号 / 释号各两轮广播过（基线文件、55432）。等推送方派非作者评审后重放进 main，进 main 的 SHA 由推送方补记。
