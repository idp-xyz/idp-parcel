# 授权动作没有「受控关闭」「重开」两格——PS 形成关闭 / 重开决定的写面因此没有动作可请求

Category: enhancement
Status: resolved——2026-09-10 20:3x（本机时钟）通道 1 推送方重放进 main（非作者评审：两个隔离子代理鉴权错死掉，改由推送方按 /code-review 两轴串行自评，0 阻断；main 上 SHA 对照见 Comments「进 main 记录」）。此前 resolved——2026-09-10 20:1x 通道 4（自标 21:2x；task-46cd2d7a，分支 `mcp4-pcgaps13` 基远端 main `b40b1804`，代码 tip `80cb3ab8`）：CONTEXT 一句先进、`AuthorizedAction` 加 `CONTROLLED_CLOSURE` / `REOPENING` 两格、客户账户请求两格显式拒、迁移 `0032` 重建五值 CHECK、两处名字镜像跟随，真库往返 PASS；完成记录（含越权风险点原文）与判断题见 Comments 末条；进 main 待推送方非作者评审后重放。此前 in-progress——2026-09-10 20:3x 通道 4 认领（task-46cd2d7a），分支 `mcp4-pcgaps13` 基远端 main `b40b1804`；迁移序号重取为 `0032`（`0031` 已被 pc-gaps/12 用掉）。此前 ready-for-agent——2026-09-10 17:5x 通道 1（推送方，用户授权代裁）裁「要裁的」三条：① 取 (a) 首发机制不算等级序、「谁可重开」由授权规则登记内容承担（越权点一条归 PC owner）；② 取倾向 PS 侧核同一账户 + 证据非空、PC 不新开对象；③ 例外支首发不开、留位。见「裁决」；「要建什么」按裁决写实。此前 draft——2026-09-10 通道 2 按通道 1 派单 task-138ab1c9 立票（写面拆两半的 PC 半边，出处 [lc/27](../../label-channel-service-first-release/issues/27-controlled-close-reopen-decision-triggers-label-final-judgment.md)「裁决」）；取证锚远端 main `062f5228`；**只写票面，未动代码、未改 CONTEXT**
Blocked by: 无（PC CONTEXT 授权那句先改再动代码，是本票内部顺序，不是阻塞边）

## 从哪里来

lc/27 量出受控关闭 / 重开决定的写面「今天连写入方都没有」，PS 端口 `ContinuedAttemptRegisterRepository` 头注自写「形成关闭或重开决定的命令口要先过 party-commercial 的授权规则校验……那是另一张票」。2026-09-10 裁（lc/27「裁决」）：写面立、拆两半——PS 半边 [lc/30](../../label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md)（命令编排 + 入口壳 + 授权适配器），PC 半边即本票。lc/30 Blocked by 本票：授权格先有，PS 适配器才有动作可请求。落位照 [ADR-0116](../../../docs/adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 决定一「一票一格」与 [pc-gaps/08](./08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md) 那一路；**不另写 ADR**——ADR-0116 先例「加格不要 ADR，改裁定形状要」，本票两格都是加格，CONTEXT 改句随票面。

## 缺口事实（锚 `062f5228`，逐符号名）

- `internal/partycommercial/domain/authority_grant.go`：`AuthorizedAction` 封闭集三格 `ManualReviewAction` / `ActiveRejectionAction` / `SourceDataAmendmentAction`；`valid()` 按区间；`decidedByCustomer()` 只对资料修订为真；`AuthorityGrant.permits` 按（动作 × 责任法人 × **等级精确相等** × 范围 × 时点）匹配；`Authorize` 四格（许 / `ErrNotAuthorized` / `ErrAuthorityRulesNotConfigured` / error），命中后 `resolveDecider`：客户账户请求 → 决定方 = 请求方；运营角色请求非客户所有的动作 → 决定方 = 运营角色；运营角色代录客户所有的动作 → 委派解出。**`AuthorityLevel` 是不透明必填串（`NewAuthorityLevel`），没有序。**
- `migrations/party_commercial/0025_source_data_amendment_action_and_contract_delegation.sql`：`authorization_grant_action_closed` CHECK 三值；`contract_delegation_content` 的 action CHECK 只 `SOURCE_DATA_AMENDMENT`（头注：客户只能委派自己拥有的决定）。当前最大序号 `0030`。
- 名字镜像两处：`internal/partycommercial/adapters/postgres/authorization_grant.go` 的 `authorizedActionFrom`（default 报错不吸收）；`cmd/parcel-commercial/translate.go` 的 `authorizedActionFrom`（显式列三格）。
- PC CONTEXT Rules 授权那句（ADR-0116 加的）：「接受后客户原始资料的修订是与人工复核、主动拒绝并列的授权动作……」；Boundaries：PC 拥有「……版本化角色权限等级、合同委派、关闭责任来源规则、**关闭与重开的条件和硬限制适用规则及其依据版本**……它不拥有具体包裹的关闭或重开请求、决定及其证据」——「关闭与重开的条件」语言有、对象无。
- PS 侧要它的地方：`internal/parcelshipment/domain/continued_attempt.go` 的 `ContinuedAttemptDecisionSpec` 必填 `Decider` / `AuthorityRole` / `AuthoritySnapshot`（关闭另必填 `CutoffBoundary` / `ClosureResponsibilitySource`，`Requester` 只关闭可缺席）；`Append` 头注「本方法不判断授权够不够格。授权规则属 party-commercial」。PS CONTEXT Rules：「关闭和重开决定必须由运营企业责任法人授权的业务角色明确形成，系统不得自动形成；货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围。关闭权限可由授权运营角色单独使用；重开必须由同级或更高授权角色形成并带原因；“同级或更高”指版本化授权规则中的重开权限等级，不是人事职级。因货主指令形成的关闭，重开还必须有同一货主账户新的有效授权」；Boundaries：「形成关闭或重开决定时仍须重新校验当前角色与客户授权」。

## 为什么是机制半边

「哪些动作可被授权」是 PC 的封闭集，关闭与重开是 PS CONTEXT 点名必须「由授权的业务角色明确形成」的两个动作，集里没有它们是形状缺口不是取值缺口——与 pc-gaps/08 同一句。**两格而不是一格**：CONTEXT 把关闭权与重开权分开说（「关闭权限可由授权运营角色单独使用；重开必须由同级或更高授权角色形成」），获准关闭不等于获准重开，`authority_grant.go` 给人工复核与主动拒绝分格的那句理由逐字成立。实例半边照旧留空：哪个法人、哪个等级、哪个范围可关可重开是 `PAR-COM-14` 一类的授权规则取值，缺省落既有 `ErrAuthorityRulesNotConfigured` → PS 未决。

## 裁决（2026-09-10 17:5x · 通道 1 推送方代裁，用户授权；按 task-3ebcdc45 写入）

- **① 「重开必须由同级或更高授权角色形成」→ 取 (a)**：首发机制**不给 `AuthorityLevel` 算序**，「谁可重开」由授权规则登记内容承担——`REOPENING` 那一格的角色（等级）集合是登进来的，PC 只答「这一等级此刻在此范围有无重开授权」。理由：等级之间的高低是租户的组织事实（实例半边），机制上给不透明串定序就是替租户定组织；硬句由登记纪律守，与 PC CONTEXT Boundaries「它不拥有具体包裹的关闭或重开请求、决定及其证据」同一句的精神——PC 拥有规则版本，PS 拥有具体决定与证据。**越权风险点（归 PC owner 复核）**：要不要让 `AuthorityLevel` 变成有序的等级——那是 PC「版本化角色权限等级」的形状改动，另立 ADR；现在选 (a) 不封死它，本票不在 `AuthorityLevel` 上加任何序或比较器。
- **② 「因货主指令形成的关闭，重开还必须有同一货主账户新的有效授权」→ 取倾向 (a)**：PS 侧核（lc/30 重开编排读原关闭的 `ClosureResponsibilitySource`，属货主指令时要求命令带该货主账户的新有效授权证据引用，核「同一账户」与「证据非空」），PC **不新开对象**——具体请求与决定的证据归 PS（CONTEXT 原句）。本票据此不加任何「客户指令权」对象或读口。
- **③ 「货主经合同或明确授权直接形成决定」例外支 → 首发不开、留位**：无租户合同实例，开了也是空支。留位 = 封闭集不预占（不为它加格）、票面记一句；`decidedByCustomer()` 对两格为 false，客户账户请求方对两格加显式拒绝（「要建什么」第 2 步），日后开时改的是那一道拒绝与一条新的委派方向。
- 三条裁完，本票转 ready-for-agent；lc/30 随其「要裁的」清零转等待本票落地。

## 要建什么（按裁决写实；逐笔 pathspec 提交）

1. **PC CONTEXT**：Rules 授权那句之后接一句——受控关闭与重开是与前三者并列的两个授权动作，同样按责任法人、角色、适用范围和有效期间版本化，关闭权不蕴含重开权；两者都是运营侧凭授权规则形成的决定，合同委派不参与（原句不改）。
2. **领域** `authority_grant.go`：`ControlledClosureAction`（`CONTROLLED_CLOSURE`）、`ReopeningAction`（`REOPENING`）——名取 PS `ContinuedAttemptDecisionKind.String()` 同词，两侧一个词；`valid()` 区间随之；`decidedByCustomer()` 对两格为 **false**（CONTEXT「必须由运营企业责任法人授权的业务角色明确形成」——决定权在运营侧，与人工复核、主动拒绝同类），故 `resolveDecider` 走「运营角色请求 → 决定方 = 运营角色」既有那一支，客户账户作请求方来请求这两格时 `resolveDecider` 今天会答「决定方 = 客户」——这一支在本票**不开**，见「要裁的」3，实施时对它加一道拒绝（`ErrNotAuthorized`，客户账户不持等级本就命不中任何 grant，加拒绝是为了把理由钉在领域而不是靠命不中）。既有三格一字不动；`ManualReviewRequirementFor` 不动。
3. **迁移**（新序号，立票时为 `0031`）：`authorization_grant_action_closed` DROP + ADD 五值；**不改** `0003` / `0025`；`contract_delegation_content` 的 action CHECK **不扩**（首发只开资料修订，ADR-0116 决定二；关闭 / 重开不是客户拥有的决定，委派不得挂它们）。
4. **名字镜像**：postgres `authorizedActionFrom` 与 `cmd/parcel-commercial/translate.go` 的 `authorizedActionFrom` 各加两格；两处 default 照旧报错不吸收。
5. **测试**：领域——两格互不蕴含（持关闭 grant 请求重开 → `ErrNotAuthorized`）、不与既有三格顶替、`Authorize` 对两格走既有四格代数（无规则 → 未配置；有规则不含 → 拒绝；命中 → 决定方 = 运营角色）、客户账户请求两格 → 拒绝；postgres——五值往返、CHECK 拒集外值（真库）；CLI——两格翻译与集外报错。
6. `docs/product/MECHANISM-INVENTORY.md` 若清点门因新格而红，按既有纪律重生成。

## 要裁的（已清零——三条裁决见上节，此处保留原问与候选作记录）

1. **「重开必须由同级或更高授权角色形成」怎么判。** → 裁 (a)。 `AuthorityLevel` 是不透明串、无序，`permits` 按等级精确相等；PS 侧关闭决定上存的 `AuthorityRole` / `AuthoritySnapshot` 也是不透明引用。候选：(a) **机制不算序**——PC 只答「这一等级此刻在此范围有无 `REOPENING` 授权」，「同级或更高」由租户登记授权规则时落进规则内容（哪些等级可重开），CONTEXT 那句「指版本化授权规则中的重开权限等级」读作等级由规则版本定义、不由机制比较；PS 把原关闭的授权角色快照与重开的授权答复并存供审计。代价：机制不拦「低等级重开高等级关闭」，硬句靠登记纪律兜。(b) 给 `AuthorityLevel` 加序——改 PC CONTEXT「版本化角色权限等级」词条与授权规则表，PS 比较两个等级；代价：新建模、`permits` 的相等判据要不要变成「不低于」连带要裁。(c) PS 请求带「原关闭的授权等级」，PC 新立「重开条件」规则对象答；代价：PC 多一类对象。**倾向 (a)** 作首发机制，理由是 CONTEXT 把「关闭与重开的条件……及其依据版本」判给 PC 的**规则版本**而不是给机制的比较器；但它决定一条硬句是靠机制还是靠登记守，归 owner。
2. **「因货主指令形成的关闭，重开还必须有同一货主账户新的有效授权」怎么问。** → 裁 (a)。关闭责任来源在 PS 登记册上（`ClosureResponsibilitySource`），PC 不知道某次重开对应的关闭是不是货主指令。候选：(a) **PS 侧核**——lc/30 的重开编排读原关闭的 `ClosureResponsibilitySource`，属货主指令时要求命令带该货主账户的新有效授权证据引用，PS 只核「同一货主账户」（与原关闭 `Requester` 相等）与「证据非空」，PC 仍只被问一次运营角色的 `REOPENING` 授权；代价：客户那份授权「是否有效」PC 不核，PS 存引用不判真伪。(b) PC 对重开另答一问「该客户账户此刻对该范围的重开指令有效否」——今天没有任何对象承载「客户对自己委托的指令权」，要新建模。**倾向 (a)**，实例半边（`PAR-COM-13` / `PAR-COM-14`）留空。
3. **「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」的例外支首发开不开。** → 裁首发不开、留位。今天 `Authorize` 的 grant 按（责任法人 × 等级 × 范围）授运营角色，客户账户不持等级，命不中任何 grant；开这一支要么给客户账户等级、要么反向委派（合同把关闭 / 重开决定权授予客户），两者都是新建模。**倾向首发不开**：默认句「只能提出请求」照实现（请求方是 PS 侧证据 `Requester`，决定方是运营角色），例外句留位、`decidedByCustomer()` 对两格为 false 并对客户账户请求方加显式拒绝（上面第 2 步），日后开时改的是那一道拒绝与一条新委派方向。

## 红线

- 不复用既有三格顶替关闭或重开；关闭权不蕴含重开权。
- 无规则仍是`未配置`不是拒绝；不为任何租户拟一条授权规则；实例值留空。
- 不改已施加迁移 `0003` / `0025`；CHECK 重建走新序号；`contract_delegation_content` 的 action CHECK 不扩。
- 领域包不依赖 HTTP / pgx；集外取值报错不吸收。
- 撞上要改 PS CONTEXT 关闭 / 重开那族硬句、或「要裁的」任一条要机制算等级序 → 停下报通道 1，不自决。
- 不动 `internal/parcelshipment/**`（PS 适配器归 lc/30）。

## 完成判据（非作者评审逐项对）

1. PC CONTEXT Rules 授权那句之后有关闭 / 重开一句，与领域两格同词。
2. `AuthorizedAction` 五格，`String()` 与 PS `ContinuedAttemptDecisionKind.String()` 同词；`decidedByCustomer()` 对两格 false；客户账户请求两格 → `ErrNotAuthorized`（有用例）。
3. 新迁移重建五值 CHECK；`0003` / `0025` 未改；委派 action CHECK 未扩；真库拒集外值。
4. 两处 `authorizedActionFrom` 认五格、集外报错。
5. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1`：PC 全部包 + `cmd/parcel-commercial` + `migrations` 绿并注明含不含真库；PS–PC 适配器包编译不受影响（本票不改签名）。
6. 代码里没有任何对 `AuthorityLevel` 的排序或比较（裁决 ①）、没有任何「客户指令权」对象或读口（裁决 ②）、客户账户请求两格被显式拒绝且封闭集只加两格（裁决 ③）——评审对着三条裁决逐项核。
7. 完成记录把「越权风险点：`AuthorityLevel` 要不要变成有序等级」原样带上，归 PC owner 复核。

## 地盘

`internal/partycommercial/domain/authority_grant.go` 及其测试；`internal/partycommercial/adapters/postgres/authorization_grant.go` 及其测试；`migrations/party_commercial/0031_*.sql`（序号以开工那刻最大序号 + 1 重取）；`cmd/parcel-commercial/translate.go` 及其测试；`docs/domain/party-commercial/CONTEXT.md` 一句；本票面。**不动** `internal/parcelshipment/**`、`contract_delegation*`。

## 参照

`internal/partycommercial/domain/authority_grant.go`（`AuthorizedAction` / `decidedByCustomer` / `permits` / `Authorize` / `resolveDecider`）；`migrations/party_commercial/0025_*.sql` 两处 CHECK；`cmd/parcel-commercial/translate.go` 的 `authorizedActionFrom`；PC CONTEXT Rules 授权那句、Boundaries「关闭责任来源规则、关闭与重开的条件和硬限制适用规则及其依据版本」；PS CONTEXT Rules 关闭 / 重开授权那一条、Boundaries「形成关闭或重开决定时仍须重新校验当前角色与客户授权」；`internal/parcelshipment/domain/continued_attempt.go`（`ContinuedAttemptDecisionSpec`、`ContinuedAttemptDecisionKind`）；ADR-0116 决定一 / 三 / 五；pc-gaps/08「要建什么」与「签名纪律」；lc/27「裁决」；lc/30。

## Comments

- 2026-09-10 · 通道 2（task-138ab1c9，分支 `mcp2-adr0134` 基 `062f5228`）：立票。**只写票面，未动代码、未改 CONTEXT。** 能力边界：读过 `authority_grant.go` 全文、`0025` 的两处 CHECK、两处 `authorizedActionFrom`、PC CONTEXT 授权那句与 Boundaries 那行、PS CONTEXT 关闭 / 重开那一族硬句、`continued_attempt.go` 的 spec 与两格；**没读** `AdjudicateCommercialAuthorizationHandler` 全文与 `AuthorityGrantStore` 的 postgres 实现——「加两格不改签名」按 `Authorize` 的形与 pc-gaps/08 加第三格时的做法判。「要裁的」三条是立票时新量到的，不在 lc/27 与派单里；倾向是本会话的建模意见，不是裁决。
- 2026-09-10 17:5x · 通道 2 按通道 1 派单 task-3ebcdc45 写入：**三条由通道 1（推送方，用户授权代裁）裁定**——① 取 (a) 首发机制不算等级序，越权点「`AuthorityLevel` 要不要变成有序等级」归 PC owner；② 取 (a) PS 侧核同一账户 + 证据非空；③ 首发不开、留位。正文在「裁决」节；「要建什么」与完成判据按裁决写实；**本票转 ready-for-agent**。只写票面，未动代码。
- 2026-09-10 21:2x · 通道 4（task-46cd2d7a，分支 `mcp4-pcgaps13` 基远端 main `b40b1804`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证，推送方重放后以 main 上的为准）：`7251de45` docs Status → in-progress；`a68318cd` docs PC CONTEXT Rules 授权那句之后接一句（做法 1）；`3b738658` feat 领域两格 + 客户账户显式拒 + 用例（做法 2、5 领域半边）；`29b487d5` feat 迁移 `0032` + postgres `authorizedActionFrom` 加两格 + 真库用例（做法 3、4、5 库半边）；`a02cad7f` feat `cmd/parcel-commercial` `authorizedActionFrom` 加两格 + 用例（做法 4、5 批文半边）；`8a27c7d5` docs 清点在 `a02cad7f` 干净检出重生成（party_commercial 31 → 32）；`80cb3ab8` docs 自评修一处（CONTEXT 句裁回票面原句）。代码 tip = 分支 tip = `80cb3ab8`（`80cb3ab8` 只改 .md，Go / SQL 与 `a02cad7f` 逐字节同）。
  **完成判据逐项**：
  1. CONTEXT Rules 授权那句之后新增一条：「受控关闭与重开是与前述各动作并列的两个授权动作，同样按责任法人、角色、适用范围和有效期间版本化，关闭权不蕴含重开权，也不与既有动作互相顶替；两者都是运营侧凭授权规则形成的决定，合同委派不参与。」资料修订那条原句一字未改；与领域两格同词（受控关闭 ↔ `CONTROLLED_CLOSURE`，重开 ↔ `REOPENING`）。先于代码进（`a68318cd` 早于 `3b738658`）。
  2. `AuthorizedAction` 五格：`ControlledClosureAction.String()` = `CONTROLLED_CLOSURE`、`ReopeningAction.String()` = `REOPENING`，与 PS `ContinuedAttemptDecisionKind.String()` 同词（`TestControlledClosureAndReopeningAreTwoSeparateAuthorizedActions` 钉字面）；`decidedByCustomer()` 对两格 false（`resolveDecider` 走「运营角色请求 → 决定方 = 运营角色」，用例核决定方 = `operator-1`）；客户账户请求两格 → `ErrCustomerAccountCannotDecide`（`Is ErrNotAuthorized`，与 `ErrDelegationAbsent` 同形的具名分格；`TestACustomerAccountMayOnlyRequestClosureAndReopening`），拒绝放在命中之后——规则缺席时照旧先答未配置（同一用例钉）。新增私有谓词 `formedOnlyByOperatorRole()`；既有三格、`ManualReviewRequirementFor`、`permits`（等级仍精确相等）一字不动。
  3. 新迁移 **`0032_authorization_grant_controlled_closure_and_reopening.sql`** DROP 再 ADD 同名 `authorization_grant_action_closed` 为五值（先例 `0017` / `0019` / `0025` / `0031`）；`0003` / `0025` 字节零改动；`contract_delegation_action_delegable` 未扩。真库 `TestClosureAndReopeningGrantsRoundTripAndTheChecksHold`：两格 `SaveGrant` → `LoadEffectiveGrants` 往返；裁定编排对「范围只登了另一等级的重开授权」答 `ErrNotAuthorized`、对登了的等级答许并指名版本；裸插 `WITHDRAWAL` 被 CHECK 拒；经写口落一条合法资料修订委派后裸插 `CONTROLLED_CLOSURE` / `REOPENING` 委派行被 `contract_delegation_action_delegable` 拒。**库层未单独跑红**：CHECK 旧字面量在 `0025` 可读，红只在领域层（`undefined: domain.ControlledClosureAction`）与批文层（「集合外的授权动作 "CONTROLLED_CLOSURE"」）各观察一次。
  4. 两处 `authorizedActionFrom`（postgres / `cmd/parcel-commercial`）各加两格，default 照旧报错不吸收；批文用例：两格翻成对应枚举值、`WITHDRAWAL` 仍拒。翻译层只做名字映射，关闭 / 重开能不能委派由 `NewContractDelegation` 拒（领域用例 `neither can be handed out by a contract delegation` 钉）。
  5. `gofmt -l` 空、`go build ./...` / `go vet ./...` 全仓退 0；带 DSN `-p 1 -count=1`（隔离树 `mcp4-pcgaps13@a02cad7f`）：PC 全部包 + 反向依赖（`cmd/*` 十一包、NR/PS/SA/TF/VE 的 `adapters/partycommercial`、PS `adapters/postgres`）+ `architecture` + `migrations` + `platform/migrate` 共 25 包：24 ok / 0 FAIL / 1 无用例 / 0 cached，47 s；`-v` PASS 2316 / SKIP 0 / FAIL 0（PC domain 639、PC postgres 272、cmd/parcel-commercial 142、platform/migrate 5、cmd/parcel-api 65，SKIP 均 0）。**含真库。** PS–PC 适配器包编译不受影响（本票不改签名）。未跑全量。
  6. 代码里对 `AuthorityLevel` 无任何排序 / 比较（`permits` 仍 `==`；`git grep -n 'level' internal/partycommercial/domain/authority_grant.go` 可核）——裁决 ①；无任何「客户指令权」对象或读口——裁决 ②；客户账户请求两格被显式拒且封闭集只加两格——裁决 ③。
  7. **越权风险点（原样带上，归 PC owner 复核）**：「要不要让 `AuthorityLevel` 变成有序的等级——那是 PC「版本化角色权限等级」的形状改动，另立 ADR；现在选 (a) 不封死它，本票不在 `AuthorityLevel` 上加任何序或比较器。」本票据此：`permits` 仍按等级精确相等；「同级或更高」由租户登进 `REOPENING` 那一格的等级集合承担；CONTEXT 里**没有**写「不给等级定序」（自评时删去——那句正是留给 owner 的）。
  **自评（/code-review 两轴串行自跑，子代理不可用）**：Standards 无阻断；Spec 一条已修（`80cb3ab8`）：CONTEXT 句多写了「货主只能提出请求」（PS 既有硬句，第二套口径）与「不给等级定序」（越权点），裁回票面做法 1 原句。非阻断留票：两处 `authorizedActionFrom` 仍各自抄名单（票面如此要求；若要统一可日后给 `AuthorizedAction` 加 `*Named` 反查，形照 pc-gaps/12 对 `DeclaredResponsibilityOutcome` 的做法）。
  **给评审的判断题**：(a) 客户账户的显式拒绝放在 `resolveDecider`（命中之后）而不是 `Authorize` 入口——于是「客户 + 无规则」答未配置、「客户 + 命中」答具名拒绝、「客户 + 有规则不命中」答泛拒绝；这与红线「无规则仍是未配置不是拒绝」一致，但是否该让「客户 + 有规则不命中」也答具名拒绝？(b) 既有 `MANUAL_REVIEW` / `ACTIVE_REJECTION` 对客户账户请求方仍走「决定方 = 客户」（票面「既有三格一字不动」），与新两格的显式拒不对称——要不要另立票统一？(c) CONTEXT 句里「也不与既有动作互相顶替」是票面原句之外多出的半句（与既有条「三者互不蕴含、不得互相顶替」同义），留还是删？
  **不做的**：PS 半边（lc/30：命令编排、入口壳、授权适配器、PS 侧核同一货主账户 + 证据非空——裁决 ②）；`AuthorityLevel` 定序；委派 CHECK 扩格；例外支（货主经合同直接形成决定）。
  **地盘外零改动**：`internal/parcelshipment/**`、`contract_delegation*`、`0003` / `0025`、`migrations.go` / `plan.go`、`cmd/parcel-api` / `cmd/parcel-dispatch`、`apps/**`、`internal/architecture/*_baseline.txt`、ADR 正文。
- **评审 ← 通道 1 推送方自评（非作者）· 钉 `61008b80`（代码 tip `80cb3ab8`，基线 `b40b1804`，8 笔）· 2026-09-10 20:3x 本机时钟**：无空闲非作者通道（5 在做 sa-cc/03，2 / 3 / 6 crash），按规矩起两个隔离只读子代理跑 Standards / Spec——两个都在第一步 `Authentication error` 死掉（tasks.md 记过的同一 harness 症状），改由推送方在隔离检出 `%TEMP%\idp-review-pcg13` 上按 /code-review 两轴串行读完生产 diff（`authority_grant.go` / `authorization_grant.go` / `translate.go` / `0032` / CONTEXT 一句）与三份测试的用例名单。验证：推送方预演链带 DSN 全量（见「进 main 记录」）。
  - **Standards（红线 / PC CONTEXT / ADR-0116 / 迁移纪律 / 注释规矩）**：**阻断 无**。**非阻断 2（判断）**：(1) `decidedByCustomer()` 与新增 `formedOnlyByOperatorRole()` 是同一枚举上的两个谓词、各回答一问（决定权归不归客户 / 客户账户能不能当决定方），`resolveDecider` 里先问后者再问前者——两问确实不同（资料修订决定权归客户但客户可以是决定方；关闭 / 重开决定权归运营侧且客户不能是决定方），不并；只记一句：日后再加格时两处都要答。(2) 三处头注写「领域加格时这份名单要跟（pc-gaps/08、/13 各加过一次）」——引的是票号与历史事实，不是对别处条目的计数，合规；但「各加过一次」会随下一次加格变旧，宜只留「名单要跟」那半句。**无发现**：diff 里对 `AuthorityLevel` 零比较 / 零排序（裁决 ①）；无任何「客户指令权」对象或读口（裁决 ②）；`decidedByCustomer()` 对两格 false、客户账户请求两格在 `resolveDecider` 命中之后拒 `ErrCustomerAccountCannotDecide`（Is `ErrNotAuthorized`）、封闭集只加两格、既有三格与 `ManualReviewRequirementFor` 未动（裁决 ③）；`0003` / `0025` 字节零改动、`contract_delegation` 的 action CHECK 未扩（diff 里对它只有注释与测试）、`0032` 只 DROP + ADD 同名 CHECK 不预填任何行；两处 `authorizedActionFrom` default 照旧报错不吸收；CONTEXT 一句在授权那句之后、与领域两格同词（`CONTROLLED_CLOSURE` / `REOPENING` 取 PS `ContinuedAttemptDecisionKind.String()` 原词）；注释全中文、无行号；无真实租户规则取值。
  - **Spec（三条裁决、完成判据 1–7）**：**阻断 无**。**非阻断 0**。**无发现（判据逐项）**：1 ✓ CONTEXT 句（`a68318cd` 先于代码 `3b738658`）；2 ✓ 五格、`String()` 同词、`decidedByCustomer` false、客户账户请求两格拒（`TestACustomerAccountMayOnlyRequestClosureAndReopening` 逐动作）、关闭权不蕴含重开权双向（`TestControlledClosureAndReopeningAreTwoSeparateAuthorizedActions` 子用例「a closure grant does not authorize a reopening」/「a reopening grant does not authorize a closure」/「neither is implied by the existing actions nor implies them」）、无规则仍未配置（「no rule at all is unconfigured, not a refusal」）、命中即运营角色为决定方、合同委派拿不到两格；3 ✓ `0032` 重建五值 CHECK，真库往返与 CHECK 拒集外值（`TestClosureAndReopeningGrantsRoundTripAndTheChecksHold`）；4 ✓ 两处名字镜像 + CLI 用例（「controlled closure and reopening are names in the closed set」）；5 ✓ 作者带 DSN 25 包 24 ok / -v PASS 2316 / SKIP 0 + 推送方全量；6 ✓ 见 Standards 无发现三条；7 ✓ 越权风险点原文在完成记录。
  - **三道判断题**：(a) **同意现状**——拒绝放在命中之后正是红线「无规则仍是未配置不是拒绝」的要求；「客户 + 有规则不命中」答泛 `ErrNotAuthorized` 与运营角色不命中同格，客户不持等级本就命不中，具名拒绝只在「命中却不能当决定方」那一格才有信息量。(b) **同意另立票、不在本票**——既有两格对客户账户请求方答「决定方 = 客户」是 pc-gaps/08 之前的形，与新两格不对称属 PC owner 的口径题（人工复核 / 主动拒绝要不要也只由运营角色形成），与「`AuthorityLevel` 要不要变有序」一起归 owner 复核清单。(c) **留**——那半句是把既有条「不得互相顶替」的规则明写到新两格上，与既有条同义不是第二套口径；删了反而让新两格漏在那条规则之外。
  - **结论**：两轴 0 阻断（Standards 2 非阻断判断），可重放进 main。
- **进 main 记录（通道 1 推送方，2026-09-10 20:3x 本机时钟）**：隔离 detached 树 `%TEMP%\idp-replay-pcg13` 先叠在 lc/33 预演 `1d1d405c` 上 pick 七笔全干净（`b40b1804..53173062` 与本票同文件只有清点），作者清点笔 `8a27c7d5` 不带、tip 重生成 `537171bb`（PC 迁移 31→32）；本票全部文件（清点除外）对分支 tip 零差。**验证钉 `537171bb`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **105 ok / 0 FAIL / 15 无测试 / 0 cached**（20:21:53→20:24:00）；PC postgres + `platform/migrate` `-v` PASS 278 / SKIP 0。lc/33 进 main（`53173062`）后 `rebase --onto 53173062 1d1d405c` 零冲突，代码树对 `537171bb` 逐字节同：`7251de45→d44b8999`、`a68318cd→3fc306bd`、`3b738658→17ad88d7`、`29b487d5→ac48afd7`、`a02cad7f→7d35de84`、`80cb3ab8→dfd08f03`、`61008b80→cd43bcf7`、清点 `fd55e1fb`。分支 `mcp4-pcgaps13@61008b80` 作封存出处、改名 `merged/`。**lc/30 Blocked by pc-gaps/13 由此解除**（lc spec 30 行与票 30 的 Status / Blocked by 同笔改为 ready-for-agent）。归 owner 复核：越权风险点「`AuthorityLevel` 要不要变有序」+ 判断题 (b) 既有两格对客户请求方的不对称。
