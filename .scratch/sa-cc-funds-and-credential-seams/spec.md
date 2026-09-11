# `settlement-accounting` ↔ `customs-compliance` 资金与凭证那条缝：七张机制半边的票

Category: chore
Status: in-progress——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）把十三条「要裁的」全部写入各票「裁决」小节（B 类四条落 [ADR-0137](../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)），01–07 转 ready-for-agent（03 等 02、07 步二等 10），另立 08 / 09 / 10（09 ready-for-agent 等 05；08 / 10 draft 各有要裁的）。此前 in-progress——2026-09-10 15:1x 通道 4 立目录与七张子票（全部 draft，task-9880bbc9）；子票全 resolved 本 spec 才 resolved。只写票面未动代码；取证锚 `3f485e97`

## 从哪里分出来

[unresolved-review-20260904/remaining-work-dd5ed934.md](../unresolved-review-20260904/remaining-work-dd5ed934.md) 表里两行判「仍余且机制半边且无票」：

- **五-7** SA 两件——BUY 评价 → SA 的 inbox 消费者；SA 外部资金事实采用**发信封** + CC 侧 inbox 消费者。
- **五-8** CC 四件机制——UC-CC-003 步 7「记录凭证门禁」的持久化；UC-CC-009 核对向 `settlement-accounting` 的交接（outbox 意图）；放行门禁核对读付款核对；凭证登记 / 关税协作 / 资金事实 / 付款核对的在线登记面与 admin 写面。

两处的源头都是 `mechanism-executor-triage` 三张实现票收口时写下的「不在本票 / 留给后继票的」（[06](../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「不在本票」节、[07](../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)「没做、留给后继票的」五条）。那两张票 2026-09-04 resolved 之后这些后继没有任何票面在盯，a3a4814 与 dd5ed934 两次重核都量到它们仍在。

**为什么并成一个目录**：七件全在同一条缝上（五-7 的第二件拆成「发信封」与「消费者」两张）——PP 评价进 SA 形成预期成本、SA 采用的外部资金事实进 CC 做税费付款核对、CC 的核对结果回 SA 判实际代垫、凭证与核对各自的门禁持久化、以及让租户能登记这些事实的入口。拆散到 SA / CC 各自目录，谁先谁后的边就没人写。

## 取证（`3f485e97`，逐条可重跑）

- `git ls-files internal/settlementaccounting/adapters` → 只有 `http` / `parcelpricing` / `partycommercial` / `postgres`，**无 `inbox`**；`git ls-files internal/customscompliance/adapters/inbox` → 空。
- `git grep -n 'eventing.EventType' -- internal/settlementaccounting/adapters/postgres` → 十七个事件类型常量，无一是「外部资金事实已采用」。
- `git grep -E 'SupplierExpectedCost|BuyEvaluationAdoption' -- cmd/` → 只命中 `cmd/parcel-api/unwired_orchestration.go` 的读面桩。
- `git grep -E 'RegisterCredential|DutyPaymentReconciliation|ReceiveFundsFact|JudgeCredentialApplicability' -- cmd/` → 零。
- `cmd/parcel-api/endpoints.go` 关务行：`/customs/external-results`、四类目录登记（解释规则 / 门禁目录 / 候选口岸 / 申报路径）、`/customs-case-requirement-registrations`——没有凭证、协作、资金事实、付款核对的登记端点。
- `internal/customscompliance/application/judge_credential_applicability.go` 头注：「本用例只判、不记：步 7 写的『记录凭证门禁』那半留给就绪判断的编排」。
- `internal/customscompliance/application/verify_release_gate.go` 不引用 `DutyVerificationStore`（`git grep -n DutyVerificationStore -- internal/customscompliance/application/verify_release_gate.go` 零）。
- `internal/customscompliance/application/reconcile_duty_payment.go` 三方法 `FormCollaboration` / `ReceiveFundsFact` / `VerifyPayment` 在；无任何 `HandOff*` 调用（`git grep -n -i handoff -- internal/customscompliance/application/reconcile_duty_payment.go` 零）。

## 子票

| 票 | 一件机制 | 上下文 | Blocked by |
|---|---|---|---|
| [01](issues/01-buy-evaluation-to-sa-inbox-consumer.md) | `parcel-pricing.evaluation.recorded` → SA inbox 消费者 → `FormSupplierExpectedCostHandler` | SA | 无（另有一格要裁，见票） |
| [02](issues/02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) | SA 外部资金事实采用同事务经 Outbox 发信封——**resolved，2026-09-10 19:4x 进 main**（分支 `mcp5-sacc02`，代码 tip `1af6a226`，main 重放 tip `cdd6504c`；非作者评审 ← 通道 6 两轴 0 阻断） | SA | 无 |
| [03](issues/03-cc-inbox-consumer-receives-external-funds-fact.md) | CC inbox 消费者收那封信封 → `ReceiveFundsFact`——**resolved，2026-09-10 21:3x 进 main**（分支 `mcp5-sacc03`，代码 tip `f96169d2`，main 重放 tip `343b997b`、与 lc/28 同批推出；非作者评审 ← 通道 6 两轴 0 阻断；实施中新裁付款人维 A、可缺席，CC 放宽另立 12） | CC | 无 |
| [04](issues/04-cc-credential-gate-persists-in-readiness-assessment.md) | UC-CC-003 步 7「记录凭证门禁」的持久化——**resolved，2026-09-11 16:1x 等非作者评审进 main**（分支 `mcp4-sacc04`，基 `7c37253f`，代码 tip `d69e18c7`；独立「凭证门禁判断」登记册 + 迁移 0018 + `RecordCredentialGateHandler`，`RegisterReadiness` 的形不动；装配点今天无、如实记在票面判据 1） | CC | 无 |
| [05](issues/05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) | UC-CC-009 核对向 SA 的交接（outbox 意图）——**resolved，2026-09-11 15:1x 进 main**（分支 `mcp5-sacc05`，代码 tip `353597a2`、分支 tip `33265d0c`；main 重放 tip `4ab49c65`；非作者评审 ← 通道 1 推送方自跑两轴 0 阻断、Spec 2 非阻断随票记；分区主体 租户 / 申报范围 带口名段） | CC | 无 |
| [06](issues/06-cc-release-gate-reads-duty-payment-verification.md) | 放行门禁核对读付款核对 | CC | 无 |
| [07](issues/07-cc-credential-and-duty-reconciliation-registration-faces.md) | 凭证 / 协作 / 付款核对三册的在线登记面（CLI + 端点 + 管理台；资金事实人工口按 ADR-0137 决定四去掉）——**步一 CLI 已进 main，2026-09-11 13:4x**（分支 `mcp2-sacc07-cli`，代码 tip `40f3d6d3`；main 重放 tip `3ec86aaa`、清点 `fa386a29`；非作者评审 ← 通道 1 推送方自跑两轴 0 阻断，非阻断两条随票记）；**步二已进 main，2026-09-11 16:0x**（`mcp2-sacc07-web` 基 `7c37253f`，代码 tip `bf8bd5ea`、清点 `56674625`；main 重放 tip `25377eec`；非作者评审 ← 通道 1 推送方自跑两轴 0 阻断，非阻断三条随票记；三端点 + `cmd/parcel-api` 装配含 05 的 Handoff 口 + 真库装配用例 + 管理台三册写签）——**两步全部 resolved** | CC | 步二 Blocked by 10 → 10 已进 main，不再阻；步一无 |
| [08](issues/08-sa-evaluation-request-orchestration-records-source-references.md) | UC-SA-002 步 2 请求评价编排——评价请求登记册（铸造 ID + 自然键唯一）同事务发信封给 PP（01 裁 (c) 时另立）——**resolved（作者侧），2026-09-11 16:3x**（分支 `mcp6-sacc08` 已 rebase 到 main `807de571`，代码 tip `e81e1240`、清点 `7dfba37d`；判据 1 / 2 / 3 / 5 落地，判据 4 标「等 11」；装配点取 `cmd/parcel-api` fail-fast 一格；等非作者评审 → 重放进 main） | SA | 11 只阻判据 4「请求 → 评价引用」；登记册与发信封半边已落 |
| [09](issues/09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md) | SA inbox 消费付款核对信封 → `AssessAdvanceRecoveryHandler`（05 裁「本目录加一张」） | SA | 05 → **05 已进 main（2026-09-11 15:1x），不再阻** |
| [10](issues/10-cc-credential-collaboration-and-verification-read-faces.md) | 凭证 / 协作 / 付款核对三册的读面（伴生列表读口 + 查阅端点 + 管理台读签：凭证进 customs-cases、协作与核对进 customs-restrictions；07 裁「读面另立」）——**resolved，2026-09-11 10:5x 进 main**（分支 `mcp5-sacc10`，代码 tip `0017f49b`；main 重放 tip `99ceb975` 含清点；非作者评审 ← 通道 2 两轴 0 阻断，非阻断六条随票记） | CC | 无 |
| [11](issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md) | PP inbox 消费评价请求信封 → 形成计价输入快照与评价并回指请求（08 裁「走信封」时因 PP 无 inbox 先例另立） | PP | 08 的发信封半边（要裁一条 PP 入口形状，见票） |
| [12](issues/12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) | CC 入向登记放宽付款人可缺席（03 裁决 2 取 A 时「CC 放宽另立 draft」）；要裁一条「真实程序要求付款人」是实例半边还是登记规则，归 CC owner | CC | 无（draft） |
| [13](issues/13-cc-correction-version-inbound-registration-and-rereconciliation.md) | CC 入向登记加版本维——SA 更正版本今天到 CC 落成 `内容冲突`、只留 inbox 痕、到不了 UC-CC-009 重新核对（03 非作者评审 Spec ①；与 12 同根异题）；要裁「CC 登记要不要版本维」+ 一条范围题，归 CC owner | CC | 无（draft） |
| [14](issues/14-adopt-digest-header-and-duty-reconciliation-handler-rejects-nil.md) | 03 非作者评审 Standards ① ② 两处小改：SA `adoptDigest` 头注补「0018 起含付款人、存量零不回算」；CC `NewDutyPaymentReconciliationHandler` 构造期逐口拒 nil（形照 SA `NewApplyPreAcceptanceControlHandler`，调用点三处、`assemble.go` 一块占号）。要裁的为零——**resolved，2026-09-11 11:5x 进 main**（分支 `mcp5-sacc14`，代码 tip `62420c95`；与 lc/34 同批，批 tip `72a7ef49`；非作者评审 ← 通道 2 两轴 0 阻断） | SA 一句注释 + CC | 无 |
| [15](issues/15-cc-comments-cite-context-line-numbers-and-count-other-packages.md) | 10 评审 Standards ① 与 07 步一评审 Standards ① 的同族全收：CC 注释里「硬句 NNN」行号引用（`0b027ab8` 实测 76 + 2 + admin-web 6 处，ADR-0137 插句后 218 / 219 一族已指错一行）换引文；`parcel-customs-register` 头注「八格 / 十三格」跨包计数换点名。零行为，只注释与测试文本。2026-09-11 14:2x 通道 1 立，ready | CC（只注释） | 软阻：等 04 / 05 / 07 步二进 main 后再开工，避免撞 rebase——05 15:1x、07 步二 16:0x 已进 main，**只剩 04（通道 4 在做）** |

**留空位、不立票的一件**：放行层的代码映射 `PAR-CUS-01/02`（`receive_external_result.go` 注释「真实代码映射属实例半边，没有它接入侧拆不出种类」）——登记册待提供，到位后接入侧译装出 `ReleaseContent`，编排不改。它在这里只占一行，不成票。

## 边界（七票共用）

- 七件全是**机制半边**：让事实能进、能出、能记。真实资金事实、真实凭证、真实付款条件与关联规则、真实程序的门禁清单属实例半边（`PAR-CUS-*` / `PAR-SET-*`），一律不写默认值。
- 不拆两侧已落的形状：SA 的 `map_external_funds.go` 四格负向代数、CC 的 `reconcile_duty_payment.go` 三方法与三表分层（UC-CC-009「七层对象必须分离」）都照用；本目录只接线、只补记、只开入口。
- 信封形状照仓内既有 handoff（`ports.*HandoffIntent` + `adapters/postgres/*_handoff.go` + `EnqueueOnce`），分区主体在各票「要裁的」里单列，不在 spec 代裁。
- 不改任何已 resolved 票的 Status；mech/06、mech/07 的「不在本票」节原样留着，本目录是它们的去处。

## Comments

- 2026-09-10 · 通道 4：立目录（task-9880bbc9）。七张子票的「要裁的」条数汇总见完工报；为零的由推送方直接转 ready-for-agent。
- 2026-09-10 · 通道 5（task-9a2ff746）：十三条要裁的分类（task-b941ce87：A 7 / B 5 / C 1）→ 用户 17:0x 授权后 B 类四条落 ADR-0137（六条越权风险点供 CC / SA owner 复核），A 类七条与 01-1 (c)、07-1 B 照推送方裁决写入各票「裁决」；立 08 / 09 / 10、子票表补三行。**只写 .md，未动代码。**
- 2026-09-10 · 通道 5（task-a93cb825）：08 两条（走信封；铸造 ID + 自然键唯一）与 10 一条（读签挂同族页：凭证 → customs-cases、协作与核对 → customs-restrictions）由通道 1 推送方代裁、写入并转 ready-for-agent；PP 无 inbox 先例，另立 11（PP 侧消费者，draft 要裁一条 PP 入口形状）作 08 的 Blocked by。本目录的缝由此跨到 PP 一角（01 / 08 / 11 是 SA↔PP 那条边）。**只写 .md，未动代码。**
