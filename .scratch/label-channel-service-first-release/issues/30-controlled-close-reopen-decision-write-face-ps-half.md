# 30 受控关闭 / 重开决定的写面（PS 半边）：命令编排 + 入口壳 + 授权适配器——请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取，PS 不自判

Category: enhancement
Status: draft（blocked by pc-gaps/13）——**要裁的已清零**：2026-09-10 17:5x 通道 1（推送方，用户授权代裁）裁「`AuthoritativeCutoffBoundary` = 关闭决定标识」，pc-gaps/13 三条同刻裁定（本票编排不比等级、PS 侧核同一货主账户 + 证据非空、例外支不开），见「裁决」；pc-gaps/13 落地即转 ready-for-agent，不必再裁。此前 draft——2026-09-10 通道 2 按通道 1 派单 task-138ab1c9 立票（[`27`](./27-controlled-close-reopen-decision-triggers-label-final-judgment.md)「裁决」把写面拆两半的 PS 半边）；取证锚远端 main `062f5228`；**只写票面，未动代码**
Blocked by: [pc-gaps/13](../../party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)（授权动作先有「受控关闭」「重开」两格，本票的适配器才有动作可请求；已 ready-for-agent）。**不阻塞但相关**：[`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md) 落下三路共用的处理方核与 handoff 形之后，本票 `Save` 后的触发尾段照抄（[ADR-0134](../../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md) 决定一 / 三）；`26` 未落时本票先落写面、尾段留缝，`27` 补

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
