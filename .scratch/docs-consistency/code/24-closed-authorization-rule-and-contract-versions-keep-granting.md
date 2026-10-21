# 24 PC 授权规则或客户合同的版本收尾之后，授权授予与合同委派照旧生效

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code24（基于 main `79a70dbba`）。原分级 R1（上线前必修）不变；原状态行：ready-for-agent——R1（上线前必修）：答错且留错痕。收尾口（票 demo-chain-gaps/50 进 main，ADR-0233）对授权规则与合同都收；运营退役一版授权规则来收回某个等级的拒绝、关闭或重开权，这些权限实际上照旧生效。更重的一格：同起点重发 v2、退役 v1 之后，`Authorize` 取首个命中，命中的是已退役的 v1 授予，消费方保存的「所采用的授权规则版本」就是一版已退役的规则。第一次更正错登的授权规则就撞，没有绕法（只能把授予逐条改区间）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 07 已裁（[ADR-0261](../../../docs/adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)）：授予与委派**随所挂版本收尾**，多条命中答适用冲突。原 Blocked by：docs-consistency/07 先裁版本壳的截止与退役对授权授予与合同委派怎么作用。
出处：[audit/pc-pp.md](../audit/pc-pp.md) H2（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PC 代码在此期间未改）。

## 现象

收尾口对任何商业对象种类都收（含授权规则），但授予与委派的装载只按各自的区间取行，不连退役册、不连区间更正册；授予还按（对象，版本标签）排序、取首个命中，退役的旧版排在前面。

## 证据

- 文档：PC CONTEXT「商业规则与政策版本」生命周期：「已生效 → 已到期、已退役或已替代：停止用于新的解析和判断……」，「本生命周期共同适用于接单规则包、接受前财务控制策略、商业价格政策、定价方案绑定、结算政策、信用政策和授权规则」。
- 决定：ADR-0233 越权风险点 6：「**合同委派、授权授予不随版本收尾**（PC owner）。同起点退役一版合同之后，挂在旧版上的委派照它自己的区间适用。」
- 代码：
  - `internal/partycommercial/adapters/registrationjson/version_closure.go` 经 `CommercialKindFrom` 取种类，`AuthorizationRuleObject` 也收。
  - `adapters/postgres/authorization_grant.go` 的 `LoadEffectiveGrants`：`effective_starts_at <= $3 AND (effective_ends_at IS NULL OR effective_ends_at > $3)`，`ORDER BY object_id, version_label`。
  - `adapters/postgres/contract_delegation.go` 的 `LoadEffectiveDelegations`：同样只看委派自己的区间。
  - `domain/authority_grant.go` 的 `Authorize` 取首个命中。

## 三类

机制：授予与委派的装载折进版本壳的收尾。产品策略：收尾对授予的作用（待 07 裁）。租户取值：哪一版退役、何时截止，由租户的运营决定给出。

## 规模

M：两个装载连上退役册与最新区间更正册（与 ADR-0233 给版本点读的折法同一口径）；`Authorize` 在多版并存时按收尾后仍有效的那一版取，并核「同一刻至多一版有效」；补同起点重发与截止两种用例。

## 最小的诚实修法

按 07 的裁定二选一：

- 若裁为「随版本收尾」（CONTEXT 照旧）：授予与委派的装载折进版本壳的截止与退役；同一授权规则对象多版并存时只取当刻有效的那一版，取不出唯一就答适用冲突，不取首个。
- 若裁为「按自身区间」（改归 B）：CONTEXT 照改；收尾口对 `AuthorizationRuleObject` 与带委派的合同版本给出明确答复（拒收，或收尾时提示须同步截止授予），并修掉「取首个命中」——那一格两种裁向都是错的。

## 要裁的

全部在 Blocked by 那一裁里；「取首个命中」不随裁向，必修。

## 裁向（2026-10-21，文档自洽 07）

[ADR-0261](../../../docs/adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md) 取上文「最小的诚实修法」第一支（随版本收尾），并把「取首个命中」那一格一并裁成适用冲突。照此实施：

1. `LoadEffectiveGrants` 装载的授予折进退役册（`party_commercial.commercial_version_retirement`）与壳上最新一条区间更正：版本已退役的授予不交出；有更正的，判用区间终点取更正终点与授予自身终点中较早的一个，只截终点（与 ADR-0233 决定五给自带区间的政策正文的折法同一口径，宜复用同一处折法）。范围加时刻有没有现行规则，按折后的授予判。
2. `LoadEffectiveDelegations` 对所挂客户合同版本同一折法。
3. `domain.Authorize`：折后命中多于一条授予即交回新的具名错误（适用冲突），不取首个；`resolveDecider`：命中多条委派而委派方不同同答适用冲突，委派方相同取那一个决定方。新错误要与 `ErrNotAuthorized`、`ErrAuthorityRulesNotConfigured` 分得开（按恢复动作分格，ADR-0029）。
4. `parcel-shipment`（协调方 2026-10-21 裁定）：
   - `psports.AuthorizationOutcome` 加一格 `APPLICABILITY_CONFLICT`（`String()` 同名）。
   - 五只经 PC 裁定编排询问授权的适配器——`adapters/partycommercial` 下的 `active_rejection.go`、`manual_review_authorization.go`、`source_data_amendment_authorization.go`、`continued_attempt_decision_authorization.go`（受控关闭与重开同一只）、`withdrawal_authorization.go`——在 `errors.Is` 分支里把 PC 的适用冲突错误译成这一格；今天它们落 `default` 交回 error（未形成），不会被读成已授权或拒绝。
   - 有未决格的四只编排停在未决，原因码指名授权适用冲突：`reject_shipment_request.go`、`withdraw_shipment_request.go`、`amend_customer_source_data.go`（`handler.undecided` 加 `…AuthorityApplicabilityConflict` 原因，与 `…AuthorityRulesNotConfigured` 并列）、`form_continued_attempt_decision.go`（`continuedAttemptUndecided` 加同名原因）。
   - 没有未决格的两只编排——`dispose_shipment_request.go` 与 `complete_manual_review.go`——今天把未配置答成自己的一个结果码（`AUTHORITY_RULES_NOT_CONFIGURED`，不落库、状态不变）；照同一形状各加 `AUTHORITY_APPLICABILITY_CONFLICT` 一码：不落库、状态不变、不读作已授权也不读作拒绝。其中 `dispose_shipment_request` 在生产上装的是 `UnconfiguredAuthorizedDispositionAuthorizer`（不问 PC，ADR-0132 越权风险点 2），给它加的那一码今天走不到，只为闭集穷尽。
   - 各编排的 `default` 分支照旧答 `ErrUnexpectedAuthorizationOutcome`。这几处是 PS 的地盘，PS 的 UC 结果表随票 10 补（PS 组的 06 正在修订，本票与 06 都不直接改 PS 文件）。
5. 补用例：`AT-PC-042`、`AT-PC-059`、`AT-PC-060`、`AT-PC-061`（UC-PC-003），领域与真库各一，并在 `.scratch/at-coverage-inventory.md` 把这几行从④移入①。

## 完成记录（2026-10-21，分支 claude-code24）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 PC、PS owner 作出，待两位 owner 复核。

### 做了什么

- PC 领域：新 `versionFold`（`domain/version_fold.go`），是版本壳上的收尾折进自带区间正文的唯一一处折法——只截终点、整版退役与业务时点无关地排除。`SupplierAgreement` 的 `NarrowedBy` / `EndedBy` / `SupportsProcurementAt`、`CommercialRegistry.bodyExcludedAt` 与收尾门用的 `bodySelectionOf` 改用它（行为不变），`AuthorityGrant` 与 `ContractDelegation` 新长出 `NarrowedBy`、`EndedBy`、`InForceAt`，都用这一处。
- PC 领域：新错误 `ErrAuthorityApplicabilityConflict`，既不 Is `ErrNotAuthorized` 也不 Is `ErrAuthorityRulesNotConfigured`。`Authorize` 先收齐命中的授予：没有命中的照旧答`未配置`或`不允许`（范围加时刻按 `InForceAt` 判）；命中多于一条即答适用冲突，先于解决定方；恰一条才解决定方。`resolveDecider` 命中多条委派时，委派方相同取那一个，不同答适用冲突。
- PC 适配器：`LoadEffectiveGrants` 把授予行包成带对象类别（授权规则）的子查询，再连 `versionClosureJoins`（最新一条区间更正加退役册）；`LoadEffectiveDelegations` 对委派行连同样两册。读回后经领域折进收尾，`InForceAt(at)` 不成立的不交出。收尾行有却立不起来就整次上抛。端口与裁定编排的注释随之改。
- PS：`psports.AuthorizationOutcome` 加 `AuthorizationApplicabilityConflict`（`APPLICABILITY_CONFLICT`）。五只适配器（主动拒绝、人工复核、资料修订、受控关闭与重开、撤回）在 `errors.Is` 分支里把它译成这一格，不带引用。四只编排停在未决：主动拒绝 `REJECTION_AUTHORITY_APPLICABILITY_CONFLICT`、撤回 `WITHDRAWAL_AUTHORITY_APPLICABILITY_CONFLICT`、资料修订 `SOURCE_DATA_AMENDMENT_AUTHORITY_APPLICABILITY_CONFLICT`（三格续办归运营登记），受控关闭与重开 `continuedAttemptUndecided` 的 `AUTHORITY_APPLICABILITY_CONFLICT`。两只各加一码：授权处置 `AuthorizedDispositionAuthorityApplicabilityConflict`、完成人工复核 `ManualReviewAuthorityApplicabilityConflict`，`String()` 都是 `AUTHORITY_APPLICABILITY_CONFLICT`，不落库、状态不变。各编排的 `default` 照旧答 `ErrUnexpectedAuthorizationOutcome`。
- 测试先行（先写用例、确认红，再实现）：
  - PC 领域 `authority_closure_test.go`，覆盖多条命中两种形状、退役、截止、只截终点、收尾只认同一版、多条命中先于解决定方答冲突、委派随合同版本收尾、委派方异同。
  - PC 真库 `adapters/postgres/authority_closure_test.go`，经 `CloseCommercialVersionHandler` 生产写口退役、截止授权规则版本与客户合同版本；同一版先截晚再截早时授予与供应商协议正文都按后一条判。`authorization_grant_test.go` 的 `TestTwoMatchingGrantsReturnTheFirstInStableOrder` 改写为 `TestTwoMatchingGrantsAreAnApplicabilityConflict`。
  - PS 适配器 `authorization_applicability_conflict_test.go`（五只）与编排 `authorization_applicability_conflict_test.go`（六只），另有续办路径的内部用例。
  - `cmd/parcel-api` 的 `TestAReviewAgainstTwoMatchingGrantsAnswersAnApplicabilityConflict`：真授权册两条授予，经 PC 裁定编排、PS 真适配器与生产复核编排答适用冲突，不落库、不铸信封。
- 文档：UC-PC-003 补消费侧翻译与首发答复对照，`AT-PC-042`、`059`–`061` 的现状缺口改为已补。UC-PS-002、UC-PS-005 的未决原因表补新码（已逐码对照 `String()`），缺口行改为已补。
- 顺带做完 code/56 的注释与 Covers 半边（见 code/56 完成记录）。

### 代决

1. **多条命中先于解决定方答适用冲突。**（评审 B1 后改写，协调方裁定取评审的 (a)）照 ADR-0261 决定三首句原样落地：折完收尾之后命中多于一条授予就答`适用冲突`，不带例外——即便这次请求无论采用哪一条都会因决定方被拒（委派缺席、客户账户请求关闭或重开）。多条命中本身是一处配置错误，理清之后同一请求照常答拒绝。初版曾让决定方的拒绝先于冲突，与决定三首句、PC CONTEXT 收尾规则、UC-PC-003 第 4 步与结果表都相抵，已撤。
2. **三格续办路径归`等待运营登记`。** 授权适用冲突在 PS 的三个原因上 `resumePath` 取 `ResumeByOperatorRegistration`：消解它的是商业责任方经截止或同起点退役收尾旧版，是一次登记册写入，重试推不动。
3. **码名。** PS 端口一格 `APPLICABILITY_CONFLICT`（与 `RULES_NOT_CONFIGURED` 同形）；三个 `JudgmentPendingReason` 照 `*_AUTHORITY_RULES_NOT_CONFIGURED` 取 `*_AUTHORITY_APPLICABILITY_CONFLICT`；受控关闭与重开、授权处置、完成人工复核各自的集合里取 `AUTHORITY_APPLICABILITY_CONFLICT`。
4. **一处折法，顺手收拢既有两处。** ADR-0261 说「宜复用同一处折法」，供应商协议正文与政策正文选候选原各写一遍。这几处与收尾门的 `bodySelectionOf` 如今都走 `versionFold`，行为不变，既有用例全绿。
5. **退役的授予与业务时点无关地排除。** 判法与 ADR-0233 越权风险点 2 相同，退役按决定时间读。
6. **装载口与 `Authorize` 用同一个谓词各判一次。** 适配器按 `InForceAt` 不交出（票面「不交出」），领域再按同一谓词判（与既有「SQL 按区间取行、`Authorize` 再判 `Contains`」同形）。谓词只在领域定义一次。
7. **版本册上没有那一版的授予照原样交出。** 授予册与版本册之间没有外键，收尾两册以外键指向版本册，所以这样的授予不可能被收尾过。
8. **`AT-PC-062` 不在本票。** 协调方的派单写了「`059`–`062`」，但 `AT-PC-062` 在 UC-PC-001，归 code/25（价格政策口径），本票只做 `042`、`059`–`061`。
9. **不改 `apps/admin-web`。** 前端的处置与复核结果类型里没有 `AUTHORITY_APPLICABILITY_CONFLICT`。两码今天在生产上都走不到：授权处置装的是不问 PC 的未配置授权口，复核授权的请求映射是 nil。前端改动按「前端切片」另做，见未决。

### 越权风险点

1. **冲突先于拒绝的代价**（PC owner）。客户账户请求关闭或重开、又恰好两条授予都命中时，答`适用冲突`而不是 `ErrCustomerAccountCannotDecide`；运营角色代录资料修订而委派缺席、又恰好两条授予都命中时，同样答冲突而不是委派缺席。消费方停在未决，等商业责任方收尾旧版或撤去重复的那一条；理清之后同一请求照常答拒绝（用例 `TestSeveralHitsAreAConflictBeforeTheDeciderIsResolved`）。
2. **续办归运营登记，但收尾口不发「参数已登记」信封**（PS owner）。今天四只编排都是命令驱动，不在派发进程的接受链上排队，没有等门铃的委托。日后哪一支进了 `awaitOperatorRegistration` 那条路，收尾口要补门铃，否则会停在等待态。另有一层：撤回与资料修订的「重发即续办」受 [code/44](./44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md) 限制——未决那一轮之后重放同一请求不再续办，运营收尾旧版之后，客户或运营要换新的请求身份重新提交才推得动。
3. **重构动到了 ADR-0233 的读法**（PC owner）。`SupplierAgreement`、`bodyExcludedAt` 与 `bodySelectionOf` 改用 `versionFold`，判法逐条不变。既有 `AT-PC-054`–`058` 用例与运输履约的收尾核对用例全绿，演示种子灌通。
4. **适用冲突对首发租户的可见性**（PC、PS owner）。运营在旧版收尾之前先登了后继的授予，那一段时间里四只编排答未决、两只答冲突码，管理台目前把冲突码当未知码显示（见未决）。这与 ADR-0261 越权风险点 1 同一格：只在换版未收尾或重复登记时出现。
5. **收尾门不看后继版本上的授予**（PC owner）。ADR-0233 的「接得住」只比两版的壳（授权规则没有自带区间的正文），不看后继版本在同一范围登没登授予、登的授予接不接得住旧版那一段。收尾一版授权规则而后继没登对应授予，那一段的裁定就由`已配置`变成`未配置`；`Authorize` 注释已改成条件句，没在收尾门上加判，留 PC owner 复核要不要加。

### 迁移

无。折法只读既有的 `commercial_version_retirement`（0040）与 `commercial_validity_correction`。

### AT

`AT-PC-042`、`AT-PC-059`、`AT-PC-060`、`AT-PC-061` 由④移入①，各自领域与真库用例都点名。盘点总览计数未动。

### 验证（分支 claude-code24）

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 初版：`TMPDIR=… scripts/test-full.sh <动过包的反向依赖（go list -test 算出 47 个）> ./internal/partycommercial/... ./internal/parcelshipment/... ./tests/bentocontract/... ./internal/architecture/... ./migrations/...`：退 0，47 个包 ok（另 3 个无测试文件）。反向依赖已含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-commercial`。
- 初版新用例另跑 `-count=3`：PC 领域、PC postgres、PS 适配器、PS 编排、`cmd/parcel-api` 共 5 个包 ok。
- 评审修订后（已 rebase 到 main `c89830c72`）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`TMPDIR=… scripts/test-full.sh ./internal/partycommercial/... ./internal/parcelshipment/... ./cmd/parcel-api ./cmd/parcel-dispatch ./cmd/parcel-commercial ./tests/bentocontract/... ./internal/transportfulfillment/... ./internal/architecture/... ./migrations/...` 退 0，43 个包 ok（另 4 个无测试文件）；新用例与改过的用例另跑 `-count=3`，5 个包 ok。
- 演示种子在一次性库 `code24seed` 上灌通（退 0），用后已 `DROP DATABASE … WITH (FORCE)`。
- 未跑全量，未重生成 MECHANISM-INVENTORY、ADR 索引与 AT 计数。

### 未决

- `apps/admin-web` 的 `AuthorizedDispositionOutcome`、`ManualReviewCompletionOutcome` 类型与展示表补 `AUTHORITY_APPLICABILITY_CONFLICT`，属前端切片，未立票。
- 其余 PS UC（主动拒绝、人工复核、授权处置、受控关闭与重开所在的用例）的结果表随票 10 补，本票只改了 UC-PS-002、UC-PS-005 两张已列码的表。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审；协调方转达，一条阻断）

代码行为未见错，变异测试大多被杀。

| 条 | 意见 | 处理 |
|---|---|---|
| B1 | 代决 1「拒绝先于冲突」与 ADR-0261 决定三首句相抵，UC-PC-003 因此自相矛盾 | 协调方裁定取 (a)，不立新 ADR：`Authorize` 的 `len(hits) > 1` 移到 `resolveDecider` 之前；用例改为 `TestSeveralHitsAreAConflictBeforeTheDeciderIsResolved`，钉住冲突先答、只剩一条时照答拒绝；UC-PC-003 首发对照删去那句例外；代决 1 与越权风险点 1 改写 |
| N1 | `bodyExcludedAt` 自己取最后一条更正、`bodySelectionOf` 另写一遍收窄，「唯一定义」名不副实 | `versionFold` 加 `cutAtEnd` 与 `selectable`；`bodyExcludedAt` 经 `correctionEndOf` 取截止点，`bodySelectionOf` 的收窄改走 `selectable` |
| N2 | 测试文件头「四只」「两只」「六只」、ports.go「第四格」是计数 | 改为点名或去掉数字；同批去掉适配器用例的「三只」、两只编排头注的「第三个落点」、续办用例的「三格」 |
| N3 | `TestTwoMatchingGrantsAreAnApplicabilityConflict` 长度与首个合在一判 | 拆成先判长度、再取首个 |
| N4 | 「取最新一条更正」扩到授予而无用例 | 补 `TestTheLatestOfTwoCutsGovernsTheGrantAndTheBody`：同一版先截晚、再截早，授予与供应商协议正文都按后一条判 |
| N5 | `Authorize` 注释「一次成立的收尾不会把已配置变成未配置」过强 | 改为条件句（后继版本在同一范围登了接得住的授予时）；越权风险点补第 5 条 |
| N6 | 撤回与资料修订的重发续办受 code/44 限制 | 越权风险点 2 补这一句 |

收口前已 `git rebase main`（`c89830c72`），无冲突。

