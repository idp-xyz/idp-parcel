# 07 只因歧义信号落下的身份冲突，在那份声明随委托被拒绝或撤回之后自动解开

Category: enhancement
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理补齐评审要求（rebase 到 origin/main `38916b00`；管理台认冲突来由），待复核。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领（ADR 占号 0211），分支 `claude-nri07`
Blocked by: 无
父票：[04](./04-identity-conflicts-have-a-disposition-and-release.md)
出处：ADR-0210 决定一「不做自动解除……记作后续方向」；ADR-0195 歧义信号。
地盘：
- docs/adr/0211-*.md（新）、docs/adr/README.md；ADR-0210、ADR-0199 文末复核记录与前向指针（部分停用照 adr/README 约定）
- docs/domain/node-operations/CONTEXT.md、docs/domain/parcel-shipment/CONTEXT.md、docs/domain/CONTEXT-MAP.md（PS → NO 新交接一条）、UC-NO-002 / UC-NO-004 相应格
- internal/parcelshipment/adapters/postgres/shipment_request.go 及其测试（委托离开`已提交`而未接受时交声明撤除信封；信封装在新文件）
- internal/nodeoperations/**（冲突来由一格；重核编排认「只因歧义信号」的冲突；inbox 消费门）
- migrations/node_operations/（冲突来由一列，下一个空号）
- cmd/parcel-dispatch/assemble.go（路由表加一行；装配进已有的 assemble_node_identity_recheck.go）——另一会话也在追加，只加行
- internal/architecture/partition_subject_registry_test.go（新分区键一行）
- docs/product/MECHANISM-INVENTORY.md、.scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「需要」。

## 为什么

ADR-0195 让「号码另有一份`已提交`委托声明」的唯一命中落成身份冲突（歧义信号）。那份委托后来被拒绝或撤回，歧义就不在了，这件实物其实可以唯一关联；可 ADR-0210 只给了人工处置一条解开的路，ADR-0199 决定三又规定记冲突的不重核——于是每一件都要人去点，信号来得快去得也快的场景（客户先报错号、马上撤单重报）会堆出一串人工活。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

1. **冲突落下时记来由。** 收寄与重核落身份冲突时，在判断上记封闭的冲突来由：只因歧义信号（恰一个候选带信号）、多候选、一码两物（ADR-0196）。既有冲突行来由缺席、不回填——它们只经人工处置解开。
2. **只有「只因歧义信号」的冲突可以自动解开，且只经重核。** 重核对象放宽到「当前有效版本记冲突、来由是只因歧义信号」的待识别；重核照旧走与收寄同一份身份判断（含 ADR-0195 的 `Unique()`、ADR-0196 的按包裹认与包裹锁）。结果唯一且无信号 → 追加关联版（业务时间原收寄时刻、不另起控制起点、意图一封），冲突随链头解开；信号仍在 → 不追加；变成多候选或一码两物 → 照现有追加冲突版、来由随之更新。
3. **触发：PS 在声明消失时交门铃。** 一份带客户包裹件号的委托由`已提交`转为`已拒绝`或`已撤回`（含补充期满形成的拒绝）时，与状态变化同一笔事务，按它声明的每个号各交一封只带（租户，号码）的信封；NO 用已有的重核消费者接，按号找来由为「只因歧义信号」的冲突行重核。委托被接受走的是已有的「外部标识关联已形成」门铃，重核时那件会成为多候选或唯一（看另一侧），本票不另起触发。人工重核口（`PENDING_IDENTITY_RECHECK`）对这类冲突行同样放开。
4. **与人工处置的关系**：已经人工处置过的链，链头不再记冲突，不在重核对象里；自动解开不撤销任何处置。
5. 写 ADR-0211：部分停用 ADR-0210「不做自动解除」与 ADR-0199 决定三「记身份冲突的……不重核」两处（限只因歧义信号这一来由），照 adr/README 约定加前向指针、追加复核记录、README 标注。

越权风险点：代 NO 与 PS owner 定了「撤单即自动解开」——客户撤单重报同号的场景下，那件实物会在没有人看过的情况下关联到第一份委托的包裹；owner 复核时看是否要求这类自动解开也进复核目录供人事后核。

## 完成判据

- [x] ADR-0211 与 CONTEXT / CONTEXT-MAP / UC 同步（先 CONTEXT 后 UC）；ADR-0210、0199 部分停用照约定。
- [x] 冲突来由在收寄、重核、ADR-0196 冲突版、处置版上都记对（领域与编排用例）；迁移只加列不回填。
- [x] PS：委托拒绝、撤回各交声明撤除信封，接受不交；信封 ID 用 `outboxintent.FingerprintEventID`，分区键登记。
- [x] 真库端到端（经生产派发装配）：号码被另一份已提交委托声明 → 送站落冲突（来由只因歧义信号）→ 那份委托被拒绝 → 门铃 → 重核追加关联版，身份一格成立，意图一封；另有一格「拒绝之后仍有第三份已提交委托声明同号」不解开；一格「一码两物的冲突」不被这道门铃解开。修前先红。
- [x] Go 门禁：`gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`migrations/...`、`internal/architecture` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nri07`，基于 main `a7dfad8f`）

**提交**（评审后 rebase 到 origin/main `38916b00`，下列是 rebase 之后的 SHA；rebase 之前依次是 `e7f631ee`、`dba5a0f2`、`0b739c5f`、`8e17ebb0`）

| 提交 | 内容 |
|---|---|
| `59f8a34d` | 文档：新 ADR-0211；ADR-0210、ADR-0199、ADR-0195 部分停用（Status 行与 Links 前向指针、文末复核记录、adr/README 该行）；NO / PS CONTEXT、CONTEXT-MAP、GLOSSARY、UC-NO-002（含 `AT-NO-062`）、UC-NO-004。rebase 时 adr/README 与 main 新加的 ADR-0207 那一行相撞，两边都留 |
| `e039e96b` | 实施与用例：冲突来由、重核对象放宽与 `STILL_AMBIGUOUS`、迁移 `node_operations/0012`、PS 声明撤除门铃、重核消费者第三扇门、路由表一行、分区主体登记一行、复核目录与人工重核答复带来由 |
| `71d5a980` | 完成记录初稿；AT 覆盖盘点 `AT-NO-062` 入①（追记十三）。rebase 时盘点总数与 main 的 demo-chain-gaps/32 追记相撞，两边都留、总数重算（①328→329，合计 1120→1121） |
| `0325d95c` | 管理台：复核页认冲突来由（评审要求，见下「管理台」） |
| `dedf346c` | 机制清点在 `0325d95c` 上重生成：node_operations 迁移 11→12、全库 242→243，消费适配器 43→44 个生产文件，直投路由表 31→32 条。rebase 前那一笔 `0b739c5f` 钉的是旧基线的数，rebase 时跳过、在新树上重算 |
| 本笔 | ADR-0211 文末追加复核记录（管理台缺口已补）；本完成记录按评审补齐 |

**落点**

- **端口**（`internal/nodeoperations/ports`）：`ports.go` 加封闭类型 `IdentityConflictCause`（`ConflictByAmbiguityOnly` / `ConflictByMultipleCandidates` / `ConflictByOneMarkOnTwoUnits`，线上名 `AMBIGUITY_ONLY` / `MULTIPLE_CANDIDATES` / `ONE_MARK_ON_TWO_UNITS`，零值不带）与 `ReceptionRecord.IdentityConflictCause`；`PendingIdentityLookup.FindRecheckableByMark` 头注改为放宽后的对象。`catalogue_read.go` 的 `UnidentifiedItemCatalogueRow` 多一格来由。
- **编排**（`internal/nodeoperations/application`）：`identity_judgment.go` 的 `identityJudgment` 多带歧义信号，`conflictCause()` 分来由（一码两物 → 多候选 → 只因歧义信号的次序）。`receive_delivered_unit.go`：明确接收支记来由；`appendIdentityConflicts` 给先到那件的冲突版记一码两物。`recheck_pending_identity.go`：重核对象的冲突一格只拦来由不是只因歧义信号的；被重核的是只因歧义信号的冲突、这一回仍只差信号时答新的一格 `RecheckStillAmbiguous`（`STILL_AMBIGUOUS`）、不追加；追加版照这一回的判断记来由。处置编排不改：处置版不记冲突，来由零值。
- **收寄库**（`adapters/postgres`）：`reception.go` 的列清单、行扫描、写入各加一格，`identityConflictCauseColumn` / `identityConflictCauseFrom`；`reception_recheck_lookup.go` 的按号找谓词改为 `(NOT r.identity_conflict OR r.identity_conflict_cause = 'AMBIGUITY_ONLY')`；`review_catalogue.go` 的待识别册多读一列。
- **HTTP**（`adapters/http`）：人工重核答复与复核目录待识别行各多一格 `identityConflictCause`（omitempty）。
- **入站**：新文件 `adapters/inbox/customer_parcel_number_declaration_retired_consumer.go`（`CustomerParcelNumberDeclarationRetiredConsumer`，与关联已形成那一扇同用消费者名；两扇门的载荷译码共用 `decodeTenantAndNumber`，`external_identifier_relation_formed_consumer.go` 只改用它）；新文件 `adapters/parcelshipment/recheck_on_declaration_retired.go`（`RecheckOnDeclarationRetiredAdapter`，转交按号重核）。
- **PS**：新文件 `internal/parcelshipment/adapters/postgres/customer_parcel_number_declaration_retired_handoff.go`（事件类型常量、`declarationsRetiredBy`、`ExternalIdentifierDoorbell.RingDeclarationsRetired`、信封组装）；`shipment_request.go` 的 `Save` 把 `Exec` 换成 `UPDATE … FROM (预期版本那一行的 state) … RETURNING prior.state, saved.saved_at`，零行即版本冲突，之后按保存前后两个状态响门铃。
- **管理台**（`apps/admin-web/src/pages/operations`，评审要求在本票内做）：新模块 `identity-conflict-cause.ts`——冲突来由三值各一句（只因歧义信号写明「那份委托被拒绝或撤回后可再重核」，多候选、一码两物写明「只经处置解开」），记冲突却不带来由的写「来由未记：此前落下的冲突，只经处置解开」，未收录的码原样示出；`conflictReleasableByRecheck` 只认 `AMBIGUITY_ONLY`。`identity-recheck.ts`：`recheckAvailabilityOf` 对只因歧义信号的冲突行放开（其余几道照旧），其余冲突行的不给理由点名来由、指向下方「处置」；`STILL_AMBIGUOUS` 的标题「仍有未决的同号声明——暂不能关联，判断不变」，细节说当前有效版本照旧、冲突照留、那份委托被拒绝或撤回后服务端自动再核、现在就要定用「处置」；「追加冲突版」与「记着身份冲突」两格那一句怎么解随来由走。`records-api.ts`、`identity-recheck-api.ts` 镜像 `identityConflictCause`；`NodeOperationsReviewPage.tsx` 待识别实物区加「冲突来由」一列，区注补一句。
- **装配**：`cmd/parcel-dispatch/assemble_node_identity_recheck.go` 的 `recheckGates` 多一扇门；`assemble.go` 路由表只加一行，接在「键比上面各行都长，单起一段」那一段的末尾（键比那一段的键短，gofmt 不重排别的行）；`internal/architecture/partition_subject_registry_test.go` 登一行。

**冲突来由的存储**

迁移 `migrations/node_operations/0012_reception_identity_conflict_cause.sql`：加列 `identity_conflict_cause text`；约束 `reception_identity_conflict_cause_closed`（取值封闭）与 `reception_identity_conflict_cause_conflict_only`（不记冲突不带来由）；记冲突的行不强制带——**既有冲突行留空、不回填**，理由写在迁移头注（恰一个候选的冲突行可能是只因歧义信号也可能是一码两物，信号从不落册）。部分索引 `reception_recheckable_by_mark` 先删后建，部分条件 `kind = 'PENDING_IDENTIFICATION' AND (NOT identity_conflict OR identity_conflict_cause = 'AMBIGUITY_ONLY') AND source_document IS NOT NULL`，与按号找那一问的谓词逐字同形，键仍是（租户，号码的 md5）。

**信封形状与路由行**

| 格 | 值 |
|---|---|
| 事件类型 | `parcel-shipment.customer-parcel-number-declaration.retired`（PS 常量 `CustomerParcelNumberDeclarationRetiredEventType`；NO 自写同字面，组合根用例 `TestTheRecheckConsumerAndThePSAgreeOnTheRetiredDeclarationType` 钉住） |
| 何时 | 委托由`已提交`转为`已拒绝`或`已撤回`的那一次 `ShipmentRequests.Save`，与状态变化同一笔事务；按声明的每个号去重后各一封。接受、已终局再保存、版本冲突那一笔都不交 |
| 载荷 | `{"tenantId", "number"}`，号码原样 |
| ID | `outboxintent.FingerprintEventID("customer-parcel-number-declaration-retired", 租户, 委托标识, 号码)` |
| 分区键 | `FingerprintEventID("external-identifier-number", 租户, 号码)`——与关联已形成门铃同一个式子、同一个口名，同一个号两种门铃排一条队；登记行「主体：租户/外部标识号码（定长指纹）」 |
| Subject / Scope | 号码原样，越界或带控制字符换 `NUMBER-SHA256/…`（共用 `externalIdentifierSubject`）/ 租户 |
| OccurredAt / RecordedAt | 撤下那一刻的决定时刻（拒绝取接受决定的决定时刻，撤回取撤回决定时刻）/ 那一行的 `saved_at` |
| 路由行 | `noinbox.CustomerParcelNumberDeclarationRetiredEventType:  identityRechecks,`（`cmd/parcel-dispatch/assemble.go`） |
| NO 处理 | 消费者名 `node-operations/recheck-pending-identities` 的第三扇门 → `RecheckOnDeclarationRetiredAdapter` → `RecheckByMark`；身份核对读不动照旧译成未决 |

**重核对象怎样放宽**

`RecheckPendingIdentityHandler.recheck` 里原来「记身份冲突即答 `IDENTITY_CONFLICT_RECORDED`」一格收窄为「记身份冲突且来由不是只因歧义信号」；按号找的谓词与部分索引同步放进来由为只因歧义信号的行（仍要求当前有效版本、不带关联、来源已保全、号码逐字相等）。两种 PS 门铃与人工指名都经这一份对象定义。被重核的是只因歧义信号的冲突时：唯一且无信号 → `INTAKE_FORMED`（关联版，前版是冲突那一版，业务时间与控制照原收寄，意图一封）；唯一而包裹已关联到别的实物 → `IDENTITY_CONFLICT`（两边各追加，来由一码两物）；仍带信号 → `STILL_AMBIGUOUS`（不追加，答被重核那一版本身）；多候选 → `IDENTITY_CONFLICT`（来由多候选）；零候选 → `STILL_UNIDENTIFIED`（不追加，冲突照留）。来由是多候选、一码两物或缺席的冲突照旧 `IDENTITY_CONFLICT_RECORDED`。

**ADR-0211 摘要与部分停用的句子**

- 摘要：决定一冲突记封闭来由、既有不回填；决定二重核对象放宽与各格；决定三 PS 在委托仓储保存那一处交声明撤除门铃、NO 同一个重核消费者接、接受不另起触发、人工重核口同样放开；决定四不撤销处置。越权风险点七条，含票面「撤单即自动解开」那一条原样。
- 部分停用（适用场景一律只限来由为只因歧义信号的冲突，自本票进 main 起）：
  - ADR-0210 决定一「**不做自动解除。** 歧义信号消失之后自动解开……本记录不做、不立票。」；同条「**重核与再次收寄都不解除冲突。**」里重核那半句；「能力边界」首项里「不做自动解除」。
  - ADR-0199 决定三「**触发只有三处，不做后台扫表。**」里「只有三处」；「重核对象」一项里「记身份冲突的（多候选、一码两物）不重核」。
  - ADR-0195 决定四「信号消失不交信封。……冲突行不经重核解除……」里「信号消失不交信封」与「冲突行不经重核解除」，连同越权风险点「信号消失后，已记的冲突不自动解除」——**超出裁定字面**，理由在 ADR-0211 越权风险点「停用 ADR-0195 一句」。

**作者代决的形状**（裁定没写到、实施时定下，都已写进 ADR-0211）

1. 来由的分格次序：一码两物只从唯一命中来，多候选压过信号。
2. 信号仍在答新的一格 `STILL_AMBIGUOUS`，不借用 `IDENTITY_CONFLICT_RECORDED`（那一格说的是「不重核」，这一格是重核过了）。
3. 零候选那一格照 ADR-0199 决定三判断不变，冲突照留（裁定没列这一格）。
4. 声明撤除门铃按号重核同一组对象（含零候选的待识别），不另立只找冲突行的一问。
5. 门铃落在委托仓储 `Save`，以同一条 UPDATE 交回的原状态判转移；分区与关联已形成门铃同队。
6. 来由作信息格进复核目录待识别行与人工重核答复，管理台据它才分得开哪一行可再重核。
7. 停用 ADR-0195 决定四一句（见上）。

**各完成判据格对应的用例**

| 判据 | 用例 |
|---|---|
| ADR 与 CONTEXT / CONTEXT-MAP / UC 同步，部分停用照约定 | 文档，`e7f631ee` |
| 来由在收寄、重核、ADR-0196 冲突版、处置版上都记对 | `internal/nodeoperations/application/identity_conflict_cause_test.go`：`TestAReceptionConflictRecordsItsCause`（收寄各格与一码两物两边）、`TestAnAmbiguityOnlyConflictFollowsTheNewAnswer`（重核冲突版来由随之更新）、`TestADispositionVersionCarriesNoConflictCause`（处置版不带）；存储面 `adapters/postgres/reception_conflict_cause_test.go`：`TestTheConflictCauseIsReadBackUnchanged`、`TestTheReceptionTableRefusesAMisshapenConflictCause` |
| 迁移只加列不回填 | `TestTheConflictCauseMigrationLeavesExistingConflictsWithoutACause` |
| 重核对象放宽、只因歧义信号之外照旧 | `TestAnAmbiguityOnlyConflictIsReleasedOnceTheDeclarationIsGone`、`TestAnAmbiguityOnlyConflictStaysWhileADeclarationRemains`、`TestConflictsOfOtherCausesStillWaitForADisposition`；存储面 `TestAnAmbiguityOnlyConflictIsFoundByThePreservedMark`；人工重核口 `adapters/http/recheck_ambiguity_only_conflict_test.go::TestAManualRecheckReleasesAnAmbiguityOnlyConflictOnceTheSignalIsGone`；复核目录 `TestUnidentifiedItemsCarryTheConflictCause`（HTTP）、`TestUnidentifiedItemsCarryTheirConflictCause`（PG） |
| PS 拒绝、撤回各交，接受不交；ID 用 FingerprintEventID；分区键登记 | `internal/parcelshipment/adapters/postgres/customer_parcel_number_declaration_retired_handoff_test.go`：`TestARejectionOrWithdrawalRingsADoorbellPerDeclaredNumber`、`TestOnlyLeavingSubmittedUnacceptedRingsTheRetiredDoorbell`（接受不交、同号同队、再保存不交、无号不交）、`TestARevisionConflictRingsNothing`、`TestTheRetiredDoorbellRefusesToRingOutsideATransaction`；登记行由 `internal/architecture` 的分区主体门禁核 |
| 入站门 | `adapters/inbox/customer_parcel_number_declaration_retired_consumer_test.go::TestARetiredDeclarationDoorbellIsHandledExactlyOnce`；`adapters/parcelshipment/recheck_on_declaration_retired_test.go::TestARetiredDeclarationAsksTheRecheckByTheVerbatimNumber` |
| 管理台认冲突来由（评审要求） | `apps/admin-web/src/pages/operations/identity-conflict-cause.test.ts`：冲突来由一列、可经重核解开的判据、重核动作随来由放开或指向处置、行转写、`STILL_AMBIGUOUS` 答复、冲突两格那一句随来由走（六条）；既有 `identity-recheck.test.ts` 的「记身份冲突不给按钮」照旧过 |
| 真库端到端（经生产派发装配），修前先红 | `cmd/parcel-dispatch/node_identity_ambiguity_release_test.go`：`TestTheWiredDispatchReleasesAnAmbiguityOnlyConflictOnceTheDeclarationIsGone`（被拒绝、被撤回两格：冲突来由只因歧义信号 → 门铃 → 关联版、身份一格成立、意图一封）、`TestTheWiredDispatchKeepsConflictsTheRetiredDeclarationDidNotCause`（第三份声明仍在不解开，第三份也撤回后解开作反证；一码两物不被这道门铃解开）、`TestARetiredDeclarationReachesTheRecheckConsumerThroughTheRouteTable`、`TestTheRecheckConsumerAndThePSAgreeOnTheRetiredDeclarationType` |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri07-red.log`（作者会话的 scratchpad，不入库）。四个内部包编译红（`undefined: ports.IdentityConflictCause`、`undefined: noinbox.RetiredCustomerParcelNumberDeclaration`、`undefined: adapter.CustomerParcelNumberDeclarationRetiredEventType` 等）；`cmd/parcel-dispatch` 编译过、行为红：路由表那条 `published = 0, want 1；失败码 = "dispatch.no_subscriber"`，端到端几格 `读冲突来由：ERROR: column "identity_conflict_cause" does not exist (SQLSTATE 42703)`；补跑一个不入库的临时探针单看门铃：`委托被拒绝之后声明撤除门铃 0 封, want 1——状态变化那一笔没入队`。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri07-gate-2.log`（钉 rebase 前的 `dba5a0f2`）；rebase 之后重跑见 `nri07-gate-3.log`（同目录，钉 `0325d95c`）。
- 管理台红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri07-red-admin-web.log`——写完 `identity-conflict-cause.test.ts`、改源码之前 `node scripts/run-tests.mjs` 退 2，编译红：`error TS2561: Object literal may only specify known properties, but 'identityConflictCause' does not exist in type 'UnidentifiedItemRecord'`（同形还有 `IdentityRecheckResponseBody`），新模块也不存在。绿：同目录 `nri07-admin-web-gates.log`。

**门**

- Go，钉 `0325d95c`（rebase 到 `38916b00` 之后）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/parcelshipment/... ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/...` 33 个包 ok、0 FAIL，退 0。main 在 `a7dfad8f..38916b00` 之间没改 `ShipmentRequests.Save` 所在的 `shipment_request.go`，`RETURNING prior.state` 那一处照旧成立；`assemble.go` 路由表那一行与分区主体登记那一行自动合入。
- Go，rebase 之前钉 `dba5a0f2`：同一组命令 33 个包 ok、0 FAIL。
- 管理台三道门（`apps/admin-web` 下，`~/.local/node` 的 Node v22.23.3；本工作树先 `pnpm install --frozen-lockfile`，没动 lockfile）：`node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 571 pass、0 fail，退 0；`pnpm exec vite build` 退 0（只有既有的 chunk 体积提示）。main 工作树里的 Vite 开发服务器没碰。
- 没跑全量 `./...`。机制清点在 `0325d95c` 上重生成，差异单独一笔 `dedf346c`。

**证据层级**：全部合成夹具，`S`。没有租户取值进代码：来由、门铃形状都是机制。

**越出地盘的改动**

- ADR-0195：Status 行、Links 加前向指针，文末追加「复核记录二」，adr/README 该行加部分停用——票面地盘只列 ADR-0210、ADR-0199；理由见上「部分停用的句子」末项。
- `docs/domain/GLOSSARY.md`「待识别实物」「身份冲突处置」两处——那两句写着「只经处置解除」「不自动解除」，不改就与 CONTEXT 相反。
- `cmd/parcel-dispatch/node_identity_ambiguity_release_test.go`（新）——票面 `cmd/parcel-dispatch` 只列了两个装配文件，真库端到端判据要这份用例。
- `internal/nodeoperations/adapters/http` 的两个答复格在 `internal/nodeoperations/**` 之内，列出只为评审好找。
- `apps/admin-web/src/pages/operations/` 下五个文件改、两个文件新增——评审要求在本票内做，随本分支走 Go 那条路落，不走前端切片在 main 上直接做的路。
- ADR-0211 文末追加一节复核记录：「能力边界」末项与越权风险点「新增重核结果一格」说的管理台缺口已补，原句不动。

**未做**

- ~~管理台认冲突来由~~：已做（`0325d95c`，见「落点」管理台一条）。此前记的是「复核页按 `identityConflict` 把冲突行的重核按钮置灰、不认 `STILL_AMBIGUOUS`，随前端切片补」，评审要求在本票内补齐。
- 完成判据末格「走 Go 那条路」归推送方：本作者不 push、不进 main。
- 越权风险点「撤单即自动解开」的 owner 复核（是否要求这类自动解开进复核目录供人事后核）。
- 按设计不做：既有冲突行不回填来由；零候选那一格不解；信号仍在时不刷新候选。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，后端钉 `8e17ebb0`，补管理台后钉 `3f403518`）

两轴合审，0 阻断。

- **Spec**：裁定五条落到。冲突来由封闭三值，收寄、重核冲突版、ADR-0196 冲突版都记，处置版不记，既有冲突行不回填；重核对象只对「只因歧义信号」放宽，按号查找与人工指名共用一份定义，部分索引谓词与查询逐字同形。PS 只在委托由`已提交`转为`已拒绝`或`已撤回`时、与保存同一笔按号交声明撤除门铃（接受、已终局再保存、撞版本冲突都不交），分区键与关联已形成门铃同式、同号排一条队。ADR-0211 部分停用 ADR-0210、0199、0195 相抵句子，一律限此来由；多停 ADR-0195 一句有理由（否则两处现行说法相反）。端到端真库用例经生产派发装配覆盖拒绝、撤回、第三份声明仍在、一码两物不受影响。
- **评审补（已做）**：rebase 到另一会话 TF 链第二波与首方渠道之后；管理台复核页「重核」只对只因歧义信号的冲突放开，认 `STILL_AMBIGUOUS`，显示冲突来由（旧行写「来由未记」），三道门过。
- **与 ps-external-mark-relations/07 的交叠（推送方处理）**：两边都新增 `AT-NO-062`；本票先进，07 只改文档，进 main 时把它的 AT 顺延。两边都改 ADR-0199 的 Status 行与 adr/README 那一行，拼接即可。
- **越权风险点留给 owner**：「撤单即自动解开」要不要进复核目录供人事后核。
