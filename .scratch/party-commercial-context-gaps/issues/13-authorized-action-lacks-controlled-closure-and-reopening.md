# 授权动作没有「受控关闭」「重开」两格——PS 形成关闭 / 重开决定的写面因此没有动作可请求

Category: enhancement
Status: draft——2026-09-10 通道 2 按通道 1 派单 task-138ab1c9 立票（用户授权代裁，写面拆两半的 PC 半边，出处 [lc/27](../../label-channel-service-first-release/issues/27-controlled-close-reopen-decision-triggers-label-final-judgment.md)「裁决」）；取证锚远端 main `062f5228`；**只写票面，未动代码、未改 CONTEXT。** 加两格本身要裁的为零；票面另冒出三条 PC 拥有的「关闭与重开的条件」怎么落（见「要裁的」，倾向已写），已报通道 1——那三条裁完本票即可转 ready-for-agent，加格那一半不等它们
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

## 要建什么（加格那一半，要裁的为零；逐笔 pathspec 提交）

1. **PC CONTEXT**：Rules 授权那句之后接一句——受控关闭与重开是与前三者并列的两个授权动作，同样按责任法人、角色、适用范围和有效期间版本化，关闭权不蕴含重开权；两者都是运营侧凭授权规则形成的决定，合同委派不参与（原句不改）。
2. **领域** `authority_grant.go`：`ControlledClosureAction`（`CONTROLLED_CLOSURE`）、`ReopeningAction`（`REOPENING`）——名取 PS `ContinuedAttemptDecisionKind.String()` 同词，两侧一个词；`valid()` 区间随之；`decidedByCustomer()` 对两格为 **false**（CONTEXT「必须由运营企业责任法人授权的业务角色明确形成」——决定权在运营侧，与人工复核、主动拒绝同类），故 `resolveDecider` 走「运营角色请求 → 决定方 = 运营角色」既有那一支，客户账户作请求方来请求这两格时 `resolveDecider` 今天会答「决定方 = 客户」——这一支在本票**不开**，见「要裁的」3，实施时对它加一道拒绝（`ErrNotAuthorized`，客户账户不持等级本就命不中任何 grant，加拒绝是为了把理由钉在领域而不是靠命不中）。既有三格一字不动；`ManualReviewRequirementFor` 不动。
3. **迁移**（新序号，立票时为 `0031`）：`authorization_grant_action_closed` DROP + ADD 五值；**不改** `0003` / `0025`；`contract_delegation_content` 的 action CHECK **不扩**（首发只开资料修订，ADR-0116 决定二；关闭 / 重开不是客户拥有的决定，委派不得挂它们）。
4. **名字镜像**：postgres `authorizedActionFrom` 与 `cmd/parcel-commercial/translate.go` 的 `authorizedActionFrom` 各加两格；两处 default 照旧报错不吸收。
5. **测试**：领域——两格互不蕴含（持关闭 grant 请求重开 → `ErrNotAuthorized`）、不与既有三格顶替、`Authorize` 对两格走既有四格代数（无规则 → 未配置；有规则不含 → 拒绝；命中 → 决定方 = 运营角色）、客户账户请求两格 → 拒绝；postgres——五值往返、CHECK 拒集外值（真库）；CLI——两格翻译与集外报错。
6. `docs/product/MECHANISM-INVENTORY.md` 若清点门因新格而红，按既有纪律重生成。

## 要裁的（PC 拥有的「关闭与重开的条件」怎么落；倾向已写，归派单方 / owner；不阻塞上面加格那一半开工）

1. **「重开必须由同级或更高授权角色形成」怎么判。** `AuthorityLevel` 是不透明串、无序，`permits` 按等级精确相等；PS 侧关闭决定上存的 `AuthorityRole` / `AuthoritySnapshot` 也是不透明引用。候选：(a) **机制不算序**——PC 只答「这一等级此刻在此范围有无 `REOPENING` 授权」，「同级或更高」由租户登记授权规则时落进规则内容（哪些等级可重开），CONTEXT 那句「指版本化授权规则中的重开权限等级」读作等级由规则版本定义、不由机制比较；PS 把原关闭的授权角色快照与重开的授权答复并存供审计。代价：机制不拦「低等级重开高等级关闭」，硬句靠登记纪律兜。(b) 给 `AuthorityLevel` 加序——改 PC CONTEXT「版本化角色权限等级」词条与授权规则表，PS 比较两个等级；代价：新建模、`permits` 的相等判据要不要变成「不低于」连带要裁。(c) PS 请求带「原关闭的授权等级」，PC 新立「重开条件」规则对象答；代价：PC 多一类对象。**倾向 (a)** 作首发机制，理由是 CONTEXT 把「关闭与重开的条件……及其依据版本」判给 PC 的**规则版本**而不是给机制的比较器；但它决定一条硬句是靠机制还是靠登记守，归 owner。
2. **「因货主指令形成的关闭，重开还必须有同一货主账户新的有效授权」怎么问。** 关闭责任来源在 PS 登记册上（`ClosureResponsibilitySource`），PC 不知道某次重开对应的关闭是不是货主指令。候选：(a) **PS 侧核**——lc/30 的重开编排读原关闭的 `ClosureResponsibilitySource`，属货主指令时要求命令带该货主账户的新有效授权证据引用，PS 只核「同一货主账户」（与原关闭 `Requester` 相等）与「证据非空」，PC 仍只被问一次运营角色的 `REOPENING` 授权；代价：客户那份授权「是否有效」PC 不核，PS 存引用不判真伪。(b) PC 对重开另答一问「该客户账户此刻对该范围的重开指令有效否」——今天没有任何对象承载「客户对自己委托的指令权」，要新建模。**倾向 (a)**，实例半边（`PAR-COM-13` / `PAR-COM-14`）留空。
3. **「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」的例外支首发开不开。** 今天 `Authorize` 的 grant 按（责任法人 × 等级 × 范围）授运营角色，客户账户不持等级，命不中任何 grant；开这一支要么给客户账户等级、要么反向委派（合同把关闭 / 重开决定权授予客户），两者都是新建模。**倾向首发不开**：默认句「只能提出请求」照实现（请求方是 PS 侧证据 `Requester`，决定方是运营角色），例外句留位、`decidedByCustomer()` 对两格为 false 并对客户账户请求方加显式拒绝（上面第 2 步），日后开时改的是那一道拒绝与一条新委派方向。

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
6. 「要裁的」三条各有裁决写进 Comments（谁 / 何时 / 口径），或写明「首发不开、留位」。

## 地盘

`internal/partycommercial/domain/authority_grant.go` 及其测试；`internal/partycommercial/adapters/postgres/authorization_grant.go` 及其测试；`migrations/party_commercial/0031_*.sql`（序号以开工那刻最大序号 + 1 重取）；`cmd/parcel-commercial/translate.go` 及其测试；`docs/domain/party-commercial/CONTEXT.md` 一句；本票面。**不动** `internal/parcelshipment/**`、`contract_delegation*`。

## 参照

`internal/partycommercial/domain/authority_grant.go`（`AuthorizedAction` / `decidedByCustomer` / `permits` / `Authorize` / `resolveDecider`）；`migrations/party_commercial/0025_*.sql` 两处 CHECK；`cmd/parcel-commercial/translate.go` 的 `authorizedActionFrom`；PC CONTEXT Rules 授权那句、Boundaries「关闭责任来源规则、关闭与重开的条件和硬限制适用规则及其依据版本」；PS CONTEXT Rules 关闭 / 重开授权那一条、Boundaries「形成关闭或重开决定时仍须重新校验当前角色与客户授权」；`internal/parcelshipment/domain/continued_attempt.go`（`ContinuedAttemptDecisionSpec`、`ContinuedAttemptDecisionKind`）；ADR-0116 决定一 / 三 / 五；pc-gaps/08「要建什么」与「签名纪律」；lc/27「裁决」；lc/30。

## Comments

- 2026-09-10 · 通道 2（task-138ab1c9，分支 `mcp2-adr0134` 基 `062f5228`）：立票。**只写票面，未动代码、未改 CONTEXT。** 能力边界：读过 `authority_grant.go` 全文、`0025` 的两处 CHECK、两处 `authorizedActionFrom`、PC CONTEXT 授权那句与 Boundaries 那行、PS CONTEXT 关闭 / 重开那一族硬句、`continued_attempt.go` 的 spec 与两格；**没读** `AdjudicateCommercialAuthorizationHandler` 全文与 `AuthorityGrantStore` 的 postgres 实现——「加两格不改签名」按 `Authorize` 的形与 pc-gaps/08 加第三格时的做法判。「要裁的」三条是立票时新量到的，不在 lc/27 与派单里；倾向是本会话的建模意见，不是裁决。
