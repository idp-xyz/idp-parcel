# 30 受控关闭 / 重开决定的写面（PS 半边）：命令编排 + 入口壳 + 授权适配器——请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取，PS 不自判

Category: enhancement
Status: resolved——2026-09-10 23:0x（作者自标）通道 4 接续通道 3 收口（用户 22:2x 经队列指示「继续 idp-parcel-mcp3-lc30 中完成」；分支 `mcp3-lc30` 基远端 main `cb467233`，代码 tip `892ffa3f`、清点 `bdfda253`）：通道 3 前会话落了三个口（`f6cf9d03`）与半份 red 测试后 crash，本会话接续落命令编排、PC 授权适配器、两个端点、组合根与共享接线四处、分区主体登记行；完成判据 1–7 全部落地，带 DSN 全量 107 ok / 0 FAIL；完成记录、偏离票面的四处与判断题见 Comments 末条；进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-10 21:5x 通道 3 认领（task-a0f51893，通道 1 改派：原派通道 2 未接到即 crash；分支 `mcp3-lc30` 基远端 main `cb467233`，隔离树 `D:/tops/idp-parcel-mcp3-lc30`）。此前 ready-for-agent——2026-09-10 20:3x（本机时钟）pc-gaps/13 进 main（PC `AuthorizedAction` 已有 `CONTROLLED_CLOSURE` / `REOPENING` 两格，客户账户请求两格由 PC 显式拒），通道 1 推送方据 Status 行原句「pc-gaps/13 落地即转 ready-for-agent」转此状态；lc/26（判断意图口）亦已进 main，尾段按 ADR-0134 乙可接。此前 draft（blocked by pc-gaps/13）——**要裁的已清零**：2026-09-10 17:5x 通道 1（推送方，用户授权代裁）裁「`AuthoritativeCutoffBoundary` = 关闭决定标识」，pc-gaps/13 三条同刻裁定（本票编排不比等级、PS 侧核同一货主账户 + 证据非空、例外支不开），见「裁决」；pc-gaps/13 落地即转 ready-for-agent，不必再裁。此前 draft——2026-09-10 通道 2 按通道 1 派单 task-138ab1c9 立票（[`27`](./27-controlled-close-reopen-decision-triggers-label-final-judgment.md)「裁决」把写面拆两半的 PS 半边）；取证锚远端 main `062f5228`；**只写票面，未动代码**
Blocked by: 无——[pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md) 已于 2026-09-10 20:3x 进 main（两格 `pcdomain.ControlledClosureAction` / `ReopeningAction`，`String()` 与 PS `ContinuedAttemptDecisionKind` 同词；此前 Blocked by 它：授权动作先有两格，本票的适配器才有动作可请求）。**不阻塞但相关**：[`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md) 落下三路共用的处理方核与 handoff 形之后，本票 `Save` 后的触发尾段照抄（[ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md) 决定一 / 三）；`26` 未落时本票先落写面、尾段留缝，`27` 补

## 缺口

lc/10 落了登记册四层（册、端口、持久化、读面派生），刻意没落写面；端口 `ContinuedAttemptRegisterRepository` 头注：「本口今天没有生产写入方，这是设计而不是欠账：形成关闭或重开决定的命令口要先过 party-commercial 的授权规则校验……那是另一张票」。lc/27 取证坐实：`application/` 里 `ContinuedAttempt` 只出现在读侧，没有任何编排调 `Append` / `Insert` / `Save`；`cmd/` 下零命中；无端点、无 CLI、无管理台写面。本票就是那张「另一张票」的 PS 半边。

**领域已经有的。** `domain.ContinuedAttemptRegister.Append(spec, currentFinalPresent)` 收 `ContinuedAttemptDecisionSpec`——两种决定共用一个 spec 由 `Kind` 分派校验：关闭必填 `CutoffBoundary` 与 `ClosureResponsibilitySource`、`Requester` 可缺席（「请求方（如有）」）；重开必填 `RelatedPriorClosure` 且生效时间严格晚于它、不带关闭责任来源；两种都必填 `Decider` / `AuthorityRole` / `AuthoritySnapshot` / `Reason` / `EffectiveAt`；`currentFinalPresent` 为真时重开不得追加（CONTEXT「只有当前不存在有效终局服务结果时，才能依据适用授权追加重开决定」）。`Append` 头注：「本方法不判断授权够不够格」。`OpenContinuedAttemptRegister(tenant, parcel)` 开册；`Insert` / `Save` 分立（开册一次、决定追加），`Save` 预期版本由聚合携带。

**PC 侧要先有的。** `AuthorizedAction` 今天没有关闭 / 重开两格——pc-gaps/13。

**先例。** 授权适配器：`adapters/partycommercial/withdrawal_authorization.go`（`WithdrawalAuthorizationAdapter`：询问经 `RequestSource` 折成 PC `AuthorizationRequest`，`AdjudicateCommercialAuthorizationHandler.Handle` 四格按恢复动作译成 `AuthorizationOutcome`）与 ADR-0116 决定四「消费方不自判决定方，PS 适配器只把 PC 交回的 `Decider` 译成自己的 `DeciderReference`」。入口壳：`cmd/parcel-api/assemble_withdrawal.go` 的 `transactionalWithdrawal`（`WithinTransaction` 包住一次编排调用，编排不持 Transactor）。

## 做法

1. **端口**（`internal/parcelshipment/ports/`，纯加法）：`ContinuedAttemptDecisionAuthorizer`——查询（委托来源身份、包裹、决定种类、请求方引用、原因引用、业务时点）→ 答复 `{Outcome AuthorizationOutcome, Authority ContinuedAttemptAuthoritySnapshot, AuthorityRole ContinuedAttemptAuthorityRoleReference, Decider DeciderReference}`；三项只在`已授权`时携带；头注照 `SourceDataAmendmentAuthorizer`：两项一起由 PC 给出、不由调用方声明。
2. **授权适配器**（`adapters/partycommercial/`，新文件，形照 `withdrawal_authorization.go`）：`RequestSource` 把查询折成 PC `NewAuthorizationRequestBy(requester=运营角色, action=CONTROLLED_CLOSURE|REOPENING, …)`（法人、等级、范围、证据、时点是实例半边映射，折不出来 → error 不译成未配置）；PC 四格译：许 → `Authority` 取 grant 版本引用、`AuthorityRole` 取 grant 的等级、`Decider` 取 PC 交回；`ErrNotAuthorized` → 不允许；`ErrAuthorityRulesNotConfigured` → 未配置；其余 error。
3. **命令编排**（`application/`，新文件；两条命令一个 handler，理由同 06 五步一个 handler）：`FormControlledClosureCommand{Identity, Parcel, Requester(可缺席), Reason, EffectiveAt, ClosureResponsibilitySource, 授权查询所需项}` 与 `FormReopeningCommand{…, RelatedPriorClosure}`。步骤：问授权（未配置 → 停在`授权未决`，不允许 → 拒绝，都不写册）→ 读当前有效终局（`FinalOutcomeStore.FindCurrentFinal`，`record.Finalized` 即 `currentFinalPresent`；读不回 → 未决）→ 开册或读回 → 铸决定标识（本上下文签发器，照 `CSDN` 那一类）→ 关闭时形成 `CutoffBoundary`（「要裁的」1）→ `Append` → `Insert` / `Save`（版本冲突是业务答案，照 06 `advance`）→ **`Save` 成功后的触发尾段**：落库同事务入队一封指针式信封（`27`「做法」，ADR-0134 决定一 / 三：一封一决定、决定标识进事件 ID、分区键租户 + 包裹、入队失败即整步回滚）。结果代数按恢复动作分格：`已形成` / `重放`（同决定标识再追加读回既有）/ `不允许`（授权拒绝）/ `授权未决` / `此刻不允许这一步`（`ErrContinuedAttemptDecisionNotAdmitted`：如当前有效终局在场时重开）/ `输入未受理` / `版本冲突`。编排不持 Transactor。
4. **入口壳**（`cmd/parcel-api/`，形照 `assemble_withdrawal.go`）：`transactionalContinuedAttemptDecision{transactor, inner}`；`POST` 两个受控编排端点（路径归实施，与既有 `cmd/parcel-api` 受控编排壳同族），Intake 起步 `UnconfiguredIntake{}` 照 ADR-0055 未配置格；端点表 / 探针 / 放行表三处共享接线**占号**（并行会话纪律）；`LabelTransactionDeps` 那一路的装配不在本票。
5. **头注改口**：`ContinuedAttemptRegisterRepository` 头注「本口今天没有生产写入方」随本票改口，不留旧话（`27` 完成判据核对它）。
6. **随 pc-gaps/13 裁决已定形的两处**（裁决见下节）：本票编排**不比等级**——只把原关闭的 `AuthorityRole` 与重开的授权答复并存供审计，不读原关闭等级、不带给 PC；重开编排读 `RelatedPriorClosure` 所指关闭的 `ClosureResponsibilitySource`，属货主指令时要求命令带该货主账户的新有效授权证据引用，核「与原关闭 `Requester` 同一账户」与「证据非空」两件，PC 仍只被问一次运营角色的 `REOPENING` 授权；不属货主指令则不加这一步。
7. **`CutoffBoundary` 的值 = 本次关闭决定的标识**（裁决）：铸决定标识之后以同一个串 `NewAuthoritativeCutoffBoundary`；与关闭路径终局的来源版本 `CONTINUED-ATTEMPT-CLOSURE/<决定标识>` 同一个标识，一个事一个名。

## 裁决（2026-09-10 17:5x · 通道 1 推送方代裁，用户授权；按 task-3ebcdc45 写入）

- **`AuthoritativeCutoffBoundary` 取什么值 → 取倾向：= 关闭决定标识。** 理由：与关闭路径形成终局时的来源版本（`responsibilityOutcomeOf` 的 `CONTINUED-ATTEMPT-CLOSURE/<决定标识>`）是同一个标识，一个事一个名；本上下文签发、与册同事务落定、可审计、与生效时间分立，不携带也不需要携带顺序信息——「边界前 / 后」由建立那一侧核册裁（lc/32）。
- **随 pc-gaps/13 三条**：① 首发机制不算等级序 → 本票不比等级；② PS 侧核同一货主账户 + 证据非空 → 本票「做法」第 6 步；③ 例外支首发不开 → 本票请求方一律是运营角色，货主只作 `Requester` 证据。
- **「06 `Establish` 不核登记册」那道缺门 → 立 [`32`](./32-establish-label-transaction-checks-continued-attempt-register.md)**（Blocked by 本票：边界先能形成），本票只形成边界不做门。

## 要裁的（已清零）

1. **`AuthoritativeCutoffBoundary` 取什么值。** → 裁 = 关闭决定标识（见「裁决」）。原文与候选留作记录： 领域只说它「不是时间戳」、是「裁决并发的新尝试是否合法」的稳定领域边界（`continued_attempt.go` 头注、CONTEXT 词条）；lc/10 把它做成必填不透明串，值由写面给。倾向：**值 = 该关闭决定的标识**（本上下文签发、与册同事务落定、可审计、与生效时间分立）；「边界前 / 后」由面单交易建立那一侧核册时读到的最新生效关闭裁——那一格立票时无票，已随裁决立为 [`32`](./32-establish-label-transaction-checks-continued-attempt-register.md)。代价：边界值本身不携带顺序信息，顺序靠册的版本链与建立决定的领域顺序，不靠比串。

## 红线

- PS 不自判决定方、不自判授权够不够格（`Append` 头注、ADR-0116 决定四）；四件从授权答复取，登录操作人只作操作证据。
- 系统不得自动形成关闭或重开决定（CONTEXT 硬句）；本票只给人形成决定的命令口，无节拍、无自动触发。
- 无授权规则 → `授权未决`不是拒绝；不为任何租户拟规则、角色、等级或范围（`PAR-COM-13` / `PAR-COM-14`）。
- 不动 `domain/continued_attempt*.go` 的校验与 `standingClosure` / `Judge`；不给决定加时钟判断。
- 不动 `internal/partycommercial/**`（pc-gaps/13 地盘）；不在 PS 分支上改 PC 文件。
- 编排不持 Transactor；事务由入口壳开。
- 不写任何真实渠道、账号、结果码。

## 完成判据（非作者评审逐项对）

1. 关闭：授权许 → 册上一条关闭决定，四件与 PC 答复一致、`CutoffBoundary` 等于该决定标识；授权拒 / 未配置 → 不写册、结果分格正确；`Requester` 缺席可关闭。
2. 重开：当前无有效终局且授权许 → 追加成功；当前有效终局在场 → `此刻不允许这一步`且不写册；生效时间不晚于关闭 → 拒；同决定标识重放返原；原关闭责任来源为货主指令而命令缺该货主账户的新授权证据、或账户与原关闭 `Requester` 不同 → 拒且不写册；编排里没有任何对等级的比较（各有用例）。
3. 授权适配器四格逐格用例；`RequestSource` 折不出 → error 不译成未配置；`Decider` / `AuthorityRole` / `Authority` 取自 PC 答复而非命令。
4. `Save` 成功后 outbox 一封、事件 ID 含决定标识、分区键租户 + 包裹；`Save` 失败 / 冲突不入队；入队失败整步回滚（真库）。`26` 未落时此条改为「尾段留缝、替身记录」并在 Comments 写明由 `27` 补。
5. 入口壳：不在事务里调用编排 → `Save` 处 error；端点表 / 探针 / 放行表三处各有一行；`UnconfiguredIntake{}` 起步。
6. `ContinuedAttemptRegisterRepository` 头注改口；`production_wiring_baseline.txt` 若有 `RehydrateContinuedAttemptRegister` 一类条目需剪按既有纪律办。
7. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 地盘

`internal/parcelshipment/ports/`（新授权口，纯加法）；`internal/parcelshipment/application/`（新编排文件 + 测试）；`internal/parcelshipment/adapters/partycommercial/`（新授权适配器 + 测试）；`cmd/parcel-api/`（新装配文件 + 端点表 / 探针 / 放行表三处占号）；`internal/parcelshipment/ports/ports.go` 的 `ContinuedAttemptRegisterRepository` 头注一句；本票面。**不动** `domain/**`、`internal/partycommercial/**`、`cmd/parcel-dispatch/**`（消费者归 `27`）。

## 参照

lc/10 完成记录「刻意没做」；lc/27「缺口」与「裁决」；`internal/parcelshipment/ports/ports.go`（`ContinuedAttemptRegisterRepository` 头注、`SourceDataAmendmentAuthorizer` / `WithdrawalAuthorizer` 一族）；`internal/parcelshipment/domain/continued_attempt_register.go`（`Append` 头注、`decisionFrom`、`OpenContinuedAttemptRegister`）；`internal/parcelshipment/domain/continued_attempt.go`（`ContinuedAttemptDecisionSpec`、`AuthoritativeCutoffBoundary` 头注）；`internal/parcelshipment/adapters/partycommercial/withdrawal_authorization.go`；`cmd/parcel-api/assemble_withdrawal.go`；PS CONTEXT「面单继续尝试决定」「权威业务截断边界」词条、Rules 关闭 / 重开那一族、生命周期「面单继续尝试」；ADR-0084 决定六；ADR-0116 决定三 / 四；ADR-0134 决定一 / 三；ADR-0055。

## Comments

- 2026-09-10 · 通道 2（task-138ab1c9，分支 `mcp2-adr0134` 基 `062f5228`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/27 全文、`continued_attempt.go` 的 spec 与两格、`ContinuedAttemptRegisterRepository` / `SourceDataAmendmentAuthorizer` / `WithdrawalAuthorizer` 头注、`withdrawal_authorization.go` 上半、`assemble_withdrawal.go` 的事务壳形状、PC `authority_grant.go` 全文；**没读** `continued_attempt_register.go` 全文（`Append` 对 `currentFinalPresent` 的全部分支按 lc/27 转述）、`cmd/parcel-api` 端点表与放行表的当前形（开工时占号再核）。「要裁的」1 与「06 `Establish` 不核册」那道缺门是立票时新量到的，已报通道 1；随 pc-gaps/13 定形的两处在其票面。
- 2026-09-10 17:5x · 通道 2 按通道 1 派单 task-3ebcdc45 写入：**通道 1（推送方，用户授权代裁）裁 `AuthoritativeCutoffBoundary` = 关闭决定标识；pc-gaps/13 三条同刻裁定，本票「做法」第 6 / 7 步据以写实；缺门立 [`32`](./32-establish-label-transaction-checks-continued-attempt-register.md)。** 本票要裁的清零，Status 仍 draft（blocked by pc-gaps/13，已 ready-for-agent），13 落地即转 ready。只写票面，未动代码。
- 2026-09-10 23:0x · 通道 4（接续通道 3 的 task-a0f51893；分支 `mcp3-lc30` 基远端 main `cb467233`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证）：`b2aa505d` docs Status → in-progress（通道 3）；`f6cf9d03` feat 三个口——`ContinuedAttemptDecisionAuthorizer` / `ContinuedAttemptDecisionIdentity` / `ContinuedAttemptDecisionHandoff` + CADN 签发器 + `OutboxContinuedAttemptDecisionHandoff`（事件类型 `parcel-shipment.continued-attempt-decision.judgment-due`）+ `ContinuedAttemptRegisterRepository` 头注改口（通道 3，做法 1、3 尾段、5）；`10bcc753` feat 命令编排 `FormContinuedAttemptDecisionHandler`（做法 3、6、7；测试文件前半——夹具与关闭四例——是通道 3 前会话 22:22 写下未提交的 red，本会话接续）；`851fe41a` feat PC 授权适配器 `ContinuedAttemptDecisionAuthorizationAdapter`（做法 2）；`18efa74e` test 分区主体登记表补 `continued_attempt_decision_handoff.go` 一行（`f6cf9d03` 新写交接口未同笔登记，`internal/architecture` 在分支上红，按登记表头注补齐）；`73a166d6` feat 两个 HTTP 入口 + `UnconfiguredIntake` 两方法（做法 4 适配器半边）；`892ffa3f` feat `cmd/parcel-api` 组合根 + 共享接线四处 + 真库两例（做法 4）；`bdfda253` docs 清点在 `892ffa3f` 干净检出重生成。代码 tip `892ffa3f`，分支 tip `bdfda253`（本笔票面在其后）。
  **完成判据逐项**：
  1. ✓ 授权许 → 册上一条关闭，四件与 PC 答复一致（`TestAnAuthorizedControlledClosureIsAppendedWithTheProvidersFourItems` 逐格比 `Decider` / `AuthorityRole` / `AuthoritySnapshot`，`Requester` 是命令事实）、`CutoffBoundary` 等于该决定标识；授权拒 → `NOT_AUTHORIZED`、未配置 → `UNDECIDED · AUTHORITY_RULES_NOT_CONFIGURED`、授权口出错 → `UNDECIDED · AUTHORITY_UNAVAILABLE`，三格都不写册、不交接、不消耗标识（`TestARefusedOrUnconfiguredAuthorizationWritesNothing`）；`Requester` 缺席可关闭（`TestAClosureMayBeFormedWithoutARequester`）。真库：`TestTheWiredContinuedAttemptDecisionWalksCloseAndReopenAgainstARealDatabase` 四件取自真授权册的 grant（决定方 = 运营角色、授权角色 = grant 等级、授权依据 = `<objectID>/v1`）。
  2. ✓ 当前无有效终局且授权许 → 追加成功、`Judge` 派生为开放（`TestAnAuthorizedReopeningIsAppendedAfterAStandingClosure`）；当前有效终局在场 → `NOT_ADMITTED` 不写册（`TestReopeningIsNotAdmittedWhileACurrentFinalStands`）；生效时间不晚于关闭 → `INPUT_NOT_ACCEPTED · DECISION_INVALID`（`TestReopeningNotAfterTheClosureIsNotAccepted`）；原关闭为货主指令而命令缺证据 → `SHIPPER_REAUTHORIZATION_MISSING`、账户与原关闭 `Requester` 不同或原关闭未记请求方 → `SHIPPER_ACCOUNT_MISMATCH`，都不写册、不消耗第二个标识；运营企业操作形成的关闭不读那两格（四子例 `TestReopeningAShipperInstructedClosureRequiresTheSameShippersNewAuthorization`）；指不到生效关闭 / 没开过册 → `NOT_ADMITTED`（`TestReopeningWithoutARegisterOrAStandingClosureIsNotAdmitted`）。**编排里没有任何对等级的比较**：重开那次授权答复换一个与关闭不同的 `AuthorityRole` 照样形成（同一用例）；`git grep -n 'AuthorityRole' -- internal/parcelshipment/application/form_continued_attempt_decision.go` 只命中把答复搬进 spec 的两行。**「同决定标识重放返原」这一格没做**：决定标识由本上下文签发器铸，命令不带标识，「同标识再追加」在数据路径上不可达；可达的两格是开册撞上别人刚开的与追加时预期版本对不上，都答 `REVISION_CONFLICT`、不交接（`TestAWriteConflictIsABusinessAnswerAndHandsOffNothing`）。见判断题 (a)。
  3. ✓ 适配器四格逐格：许（三件取自裁定：`Authority` = grant 版本引用、`AuthorityRole` = 请求等级——命中的 grant 与请求在等级上逐字相等是 PC `permits` 的判据，PC 的 `Authorization` 不交回等级、本票不动 PC，故从请求上取——`Decider` = PC 交回的运营角色，不是询问里的货主请求方）/ 拒（只授关闭问重开、只授重开问关闭各一格：一口两问互不蕴含）/ 未配置 / error；`RequestSource` 折不出 → error 不译成未配置且不问提供方；映射 nil 同格；种类零值 → error 且映射与提供方都不被问（`continued_attempt_decision_authorization_test.go` 全部）。
  4. ✓ 真库：关闭 `Save` / `Insert` 成功后 outbox 恰一封、`event_id` 含决定标识（`envelopeCountForDecision`）、分区键租户 + 包裹（由 `continued_attempt_decision_handoff_test.go` 三决定同区那一例钉，本票不重钉）；重开再一封；**入队失败整步回滚**——换会失败的交接口，关闭返回 error、册上无行；映射未配置的生产装配停在未决、册无行、outbox 零封（`TestTheWiredContinuedAttemptDecisionStopsHonestlyWhenTheAuthorizationMappingIsNotConfigured`）。替身层：`Save` / `Insert` 冲突不入队、交接失败以 error 上抛（`TestAFailedHandoffSurfacesAsAnErrorSoTheShellRollsBack`）。`26` 已进 main，尾段照 ADR-0134 接真 outbox，不留缝。
  5. ✓ 裸编排不在事务里调用 → 写口拒、错误上抛、册无行（同真库用例「判据 5」段）；端点表两行（`/shipment-requests/continued-attempt-closures`、`/shipment-requests/continued-attempt-reopenings`）+ 探针表两行 + 未接线实参一行 + `unwiredContinuedAttemptDecisions` 桩；放行表不动——写行不入隔离读放行（`isolated_read_test.go` 零 diff）；两端点 `UnconfiguredIntake{}` 起步，403 `ACCESS_CHANNEL_NOT_CONFIGURED` 由契约测试钉。
  6. ✓ `ContinuedAttemptRegisterRepository` 头注已由 `f6cf9d03` 改口指向 `FormContinuedAttemptDecisionHandler`，旧句不再在它头上（`ports.go` 里同一句「本口今天没有生产写入方」仍留在 `LabelTransactionRepository` 头注上——那是 06 的口，lc/28 装配之后它也陈旧了，不在本票地盘，记给 lc/34 / lc/35 的头注改口那一族）；`production_wiring_baseline.txt` 无继续尝试条目可剪（`RehydrateContinuedAttemptRegister` 早已剪掉，只余注释），棘轮的 stale 检查在全量里过。
  7. ✓ `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；隔离树 `mcp3-lc30@892ffa3f` 带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（22:57:56→23:00:02，含真库）；PS application `-v` 本票各例全 PASS、`cmd/parcel-api -run ContinuedAttemptDecision -v` PASS 2 / SKIP 0（带 DSN 实跑）。
  **偏离票面的四处（供评审）**：① 做法 2 把「时点」列进映射，本适配器从询问的 `At` 取（编排在决定时刻问）、映射不给——映射再给一份就是第二个来源；原因同理取自询问的 `Reason`。② 做法 3「`重放`（同决定标识再追加读回既有）」一格未做，理由见判据 2。③ 关闭责任来源写成 `<种类>/<主体>`，种类三格取自 CONTEXT（`SHIPPER-INSTRUCTION` / `OPERATOR-ACTION` / `EXTERNAL-RESTRICTION`）——票面没定形，做法 6 要认得出「货主指令」才核得了同一账户，没有种类段核不了；第三格「外部硬限制」的重开在本票不加门（解除来源今天读不到，加一道读不到来源的门等于替来源责任方作答），头注写明。④ 重开命令的货主重授权证据引用只作门、不落册——领域决定上没有它的格，加格是 `domain/**` 改动（红线）。
  **判断题**：(a) 重放格：要不要让命令可选带调用方标识以支持幂等重放（改「PS 铸标识」的口径），还是维持「HTTP 写行的幂等归接入层来源身份」——今天两个端点都 `UnconfiguredIntake{}`，接入层就位前无人重放；倾向后者、不改。(b) 领域 `Append` 允许在已有生效关闭之上再追加一条关闭（`decisionFrom` 关闭那一支不看 `standingClosure`），本编排照领域、不加门；是否该在编排拒「关上加关」归 lc/10 的领域口径，本票不替它答。(c) 关闭责任来源三格是本票新造的封闭集，落在应用层而不是领域（领域值仍是不透明串）：要不要抬进 `domain/**` 成类型，归 PS owner（本票红线不动领域）。(d) 外部硬限制来源的重开今天与运营企业操作同形（只问 PC 一次 REOPENING）；CONTEXT「先由其来源责任方形成有效解除」在代码里无落点，是否立票归 owner。(e) 事务壳把「未决 / 拒绝 / 冲突」都提交（那几格什么都没写）——与 `transactionalWithdrawal` 同款；无异议。
  **不做的**：`27` 的消费者（`cmd/parcel-dispatch` 路由表，事件类型常量已导出给它）；运营端点之外的触发面；接入层 Intake（`PAR-INT-01`）；映射（`PAR-COM-13` / `PAR-COM-14`）；管理台写面；lc/32 的建立核册。
  **地盘外零改动**：`internal/parcelshipment/domain/**`、`internal/partycommercial/**`、`cmd/parcel-dispatch/**`、`isolated_read_test.go`、`internal/architecture/*_baseline.txt`；共享接线四处各只加自己那块（`git diff cb467233 -- cmd/parcel-api/endpoints.go cmd/parcel-api/main.go cmd/parcel-api/endpoints_test.go cmd/parcel-api/unwired_orchestration.go` 全为加行）。
  **能力边界**：/tdd 的红在编排一段可见（前身 red 先在、`go vet` 报 undefined 后转绿）；适配器与两端点那两段是实现与测试同笔、红未单独可见——评审按此读。没读 `cmd/parcel-dispatch` 与 lc/27 票面全文，消费者半边只按 `f6cf9d03` 导出的常量对接。分支上 `f6cf9d03` 之前的工作（三个口）由通道 3 前会话完成，本会话逐文件读过、未改一字。
