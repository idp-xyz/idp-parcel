# 03 第二期：待识别实物按存下的号重核（AT-NO-026）——PS 关联册新形成关联时交出信封、复核页人工发起，结果唯一即追加关联版

Category: enhancement
Status: in-progress——2026-10-19 评审补完工（推送方裁定两件：演示种子补齐、ADR-0199 追加复核记录；rebase 到 main `e8ef2398`），待评审。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（02 已进 main），分支 `claude-nrs03`；碰 cmd/parcel-dispatch/assemble.go 与 cmd/parcel-api/endpoints.go 已与另一会话对齐次序（本票先进）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 02 进 main（重核拿的是 02 存下的来源文档；追加版要指名前版）
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「二、按存下的号重核」与「重核由什么触发」那一问；ADR-0199 决定三。`AT-NO-026` 在 `.scratch/at-coverage-inventory.md` 里是机制候选，今天没有票、也没有人工关联口。
地盘：
- internal/parcelshipment/adapters/postgres/external_identifier.go、shipment_request.go、label_transaction.go 及其测试（关联形成同一笔事务入队信封）
- PS 侧信封的传输形状与事件类型常量（落点照 `OutboxNodeIntakeHandoff` 那一类的先例，实施时定）
- internal/nodeoperations/application/（新重核编排；与收寄步骤 4 共用的身份判断抽出来，不另写一套）及其测试
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/adapters/postgres/reception.go 及其测试（按号找待重核的待识别）
- migrations/node_operations/（按外部标识观察、只覆盖待识别行的索引）
- cmd/parcel-dispatch（NO 第一个 inbox 消费者的装配）
- internal/nodeoperations/adapters/http/ 与 cmd/parcel-api（人工重核命令口）
- internal/nodeoperations/ports/catalogue_read.go、adapters/postgres/review_catalogue.go、adapters/http/query_node_operations_records.go（待识别行带外部标识观察）
- apps/admin-web/src/pages/operations/**（待识别实物页的外部标识观察列与「重核」动作）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象

零候选的待识别实物落册后，PS 后来形成了这个号的关联，NO 没有任何路径再问一次：号码不在册上（票 02 之前），也没有触发。`AT-NO-026`「身份后来被 `parcel-shipment` 确认｜建立版本化关联，原待识别记录和既有事实保留；业务时间取原接收时刻，不另起控制起点，发布意图一封」没有机制。

## 做什么

照 ADR-0199 决定三：

1. **PS 交出信封**。`insertExternalIdentifierRelations` 的调用方在关联真插入的同一笔事务里经 Outbox 交出只带（租户，号码）的信封，信封身份由关联的键认领（ADR-0185 的键），经 `outboxintent.EnqueueOnce`；适用终态不交。信封不带包裹、分配方、标识类型或委托。
2. **NO 消费者**住 `cmd/parcel-dispatch`：按（租户，号码）找当前有效版本是待识别、不带关联、不记身份冲突、来源文档的外部标识观察与号码逐字相等的判断，逐件重核。匹配不到即什么也不做。
3. **重核编排**：拿来源文档里的外部标识观察走收寄步骤 4 同一份身份判断（含 ADR-0196 的按包裹认与 no-reception-identity/03 的（租户、包裹）锁），按 ADR-0199 决定三分格追加：唯一且包裹无别的实物关联 → 追加`节点收寄已形成`（接收时刻与控制照原判断、前版是被重核那一版、照 ADR-0186 取服务结果、意图一封）；唯一但包裹已关联别的实物 → 照 ADR-0196；多候选 → 追加冲突版；零候选 → 不追加（被重核那一版是「未核对」时追加「待识别 · 已核对」，票 05 之前不会出现）；读不动 → 不追加、交回 error。追加版的来源标识由被重核那一版的判断版本派生，带 `IDENTITY-RECHECK/` 前缀，撞键即重放。
4. **人工重核命令口**：指名一件待识别实物发起重核，对象条件同上。归族照 ADR-0151 决定一（管理台上作判断的口归「运营决定」能力面），实施时核对；对冲突行、无来源文档的旧行答哪一格照 ADR-0022 与 ADR-0186 的分格定，写进票面。
5. **复核目录与管理台**：待识别行带来源文档里的外部标识观察（旧行如实写「来源未保全」）；管理台待识别实物页加这一列与「重核」动作（走 Go 那条路，不走前端切片的 main 直做）。
6. **AT 点名**：`AT-NO-026`。

补记（2026-10-19，票 ps-external-mark-relations/05 作者子代理；出处 [ADR-0195](../../../docs/adr/0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md) 决定四）：收寄步骤 4 的身份判断认唯一改经 `ports.ParcelIdentityAnswer.Unique`，即恰一个候选、且 PS 没有答出歧义信号，不再数候选。「重核编排」那一步继承它：恰一个候选带信号落在「多候选」那一格，追加冲突版，候选记那一件；零候选带信号落在「零候选」那一格。从收寄编排抽共用判断时，这一格随之带过来，不要在重核里另按候选数分支。

## 不做

- 后台扫表（裁定否决）；PS 读不动期间落下的未核对行在号码关联早已形成时怎么自动续上（2026-10-19 代决取自投门铃，随翻转落在票 05；本票的消费者与重核要能被它复用）。
- 身份冲突的处置：记身份冲突的待识别不重核。
- 票 02 之前落下、没有来源文档的旧行。
- 第一期落地到本票落地之间已形成的关联不补发信封（ADR-0199 能力边界），那段时间的零候选待识别靠人工重核。

## 完成判据

- [x] 编排用例：零候选待识别在关联形成后重核成`节点收寄已形成`：带关联、接收时刻等于原收寄时刻、控制起点与控制依据照原判断、前版是原待识别、意图一封；同一信封重投、同一件再人工重核都不再追加（派生键撞键即重放）。
- [x] 编排用例，非唯一各格：多候选追加冲突版；唯一但包裹已关联别的实物照 ADR-0196 两边追加；零候选不追加；身份读不动不追加且交回 error。
- [x] 编排用例：记身份冲突的待识别、没有来源文档的旧行都不重核。
- [x] 真库用例（经生产装配）：零候选收寄 → PS 委托接受形成这个号的关联 → 信封入队 → `cmd/parcel-dispatch` 消费 → NO 追加关联版 → 收寄意图一封，载荷指回的收寄业务时间是原收寄时刻；`JudgeIdentityBasis` 对这件实物判成立（`AT-NO-026`）。
- [x] 真库用例：人工发起重核与信封触发落下同一结果；对冲突行与无来源旧行的答法照票面写定的那一格。
- [x] 并发：重核与同一包裹另一件实物的收寄并发，经（租户、包裹）锁串行，后到者答身份冲突或已有结果，不出现两件都关联成立。
- [x] PS 侧适配器用例：关联重放（ON CONFLICT 不插）不重复入队；适用终态不入队。
- [x] admin-web 三道门退 0（口径见 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境)）。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`internal/parcelshipment/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nrs03`，基于 main `4f11563c`）

**提交**

| 提交 | 内容 |
|---|---|
| `8106a6a5`（rebase 后 `445a2d08`） | 实施与用例：PS 门铃、NO 共用身份判断与重核编排、收寄库两问与迁移 0008、NO inbox 消费者与派发装配、人工重核命令口（运营决定能力面、新决定种类与 access_identity 迁移 0007）、复核目录与管理台 |
| ~~`c70f2ac1`~~（rebase 时撤下，由评审补那一笔 `56400c7d` 重生成取代） | 机制清点在 `8106a6a5` 的干净检出上重生成：迁移 226→228，cmd 生产 81→83、测试 161→163，nodeoperations 生产 44→51 |
| `7bf269fd`（rebase 后 `3a7b64a4`） | 本票完成记录；AT 覆盖盘点 `AT-NO-026` 由②入① |

**落点**

- **PS 门铃**（`internal/parcelshipment/adapters/postgres/external_identifier_formed_handoff.go`，新）：`ExternalIdentifierDoorbell.RingFormed`。`insertExternalIdentifierRelations` 改成 `RETURNING recorded_at`、交回这一笔真插入的那几条（`ON CONFLICT` 不插的重放不交回）；委托仓储 `Save`、面单交易仓储 `Save` 在关联（与适用变化）写完之后逐条响铃。适用终态不交：响铃前按关联键查 `external_identifier_relation_change`，有终态行就跳过——同一笔里形成又作废的渠道号、被替代的旧号都不响。门铃随仓储构造（`NewShipmentRequests`、`NewLabelTransactions` 内部各建一只），不改两者的构造签名（各几十处装配点）。
- **NO 共用身份判断**（`internal/nodeoperations/application/identity_judgment.go`，新）：收寄步骤 4 那一段（身份读口 → `Unique()` → 按包裹取锁读别的实物关联 → 唯一成立才取服务结果）原样抽成 `identityJudge.judge`，收寄 `receive` 改为调它；`appendIdentityConflicts`、`handOff`（改名 `handOffIntake`）由方法改成包级函数供重核复用。收寄的既有用例一条未改，照旧全绿。身份读口的错误带新哨兵 `ErrParcelIdentityUnreadable`。歧义信号照 ADR-0195 决定四随 `Unique()` 继承：一个候选带信号落在多候选（冲突）那一格，零候选带信号落在零候选那一格，重核里没有另按候选数分支。
- **重核编排**（`internal/nodeoperations/application/recheck_pending_identity.go`，新）：`RecheckPendingIdentityHandler`，两个入口 `Recheck`（指名一版）与 `RecheckByMark`（按号），落到同一个 `recheck`。七格结果 `RecheckOutcome`：`INTAKE_FORMED`、`IDENTITY_CONFLICT`、`STILL_UNIDENTIFIED`、`EXISTING_RESULT`、`IDENTITY_CONFLICT_RECORDED`、`SOURCE_NOT_PRESERVED`、`NOT_PENDING_IDENTIFICATION`。追加版：来源标识 `IDENTITY-RECHECK/<被重核那一版的判断版本>`，摘要与来源文档沿用被重核那一版，接收时刻、节点、交付方、证据与控制照原判断，前版是被重核那一版；唯一则带关联、照 ADR-0186 记服务结果核对态并交收寄意图一封；冲突（多候选、带信号、一码两物）不交意图，一码两物另照 ADR-0196 给先到那件追加冲突版。派生键撞上（并发里另一方先追加）读回作答 `EXISTING_RESULT`；被重核那一版已有后继也答 `EXISTING_RESULT`，带当前有效那一版。意图交不出去交回 error（两个调用方都没人读续办引用，提交一版没交出意图的关联收寄 PS 就永远等不到它）。
- **端口与收寄库**：`ports.PendingIdentityLookup`（`FindByJudgmentVersion`、`FindRecheckableByMark`）与 `ReceptionStore` 分立，生产上同一只 `Receptions`（`reception_recheck_lookup.go`，新）。迁移 `migrations/node_operations/0008_reception_recheck_lookups.sql`：`reception_recheckable_by_mark`（`tenant_id, md5(source_document ->> 'mark')`，只覆盖待识别、不记冲突、来源已保全的行）与 `reception_by_judgment_version`。按号的键取 md5 而不取号本身：设备上报的号没有长度上限，btree 一项有上限，按号本身建索引，一件贴着超长号的待识别就落不了册（用例钉了 4200 字节的号）。
- **NO inbox 消费者**（`internal/nodeoperations/adapters/inbox`，新包 `noinbox`）：`ExternalIdentifierRelationFormedConsumer`，消费者名 `node-operations/recheck-pending-identities`，舞步交 `platform/inboxconsume`；载荷缺 `tenantId` 或 `number` 即毒丸。转交 `noparcel.RecheckOnRelationFormedAdapter`（`internal/nodeoperations/adapters/parcelshipment/recheck_on_relation_formed.go`，新）→ `RecheckByMark`，号码原样。
- **派发装配**（`cmd/parcel-dispatch/assemble_node_identity_recheck.go`，新）：`recheckPendingIdentitiesConsumer` 与 `nodeIdentityRecheck`。服务结果读口照 parcel-api 收寄同法包 savepoint。身份读不动经 `dispatch.WithUndecidedSentinels(…, ErrParcelIdentityUnreadable)` 记 `dispatch.consumer_undecided`。
- **人工重核命令口**：`POST /node-operations/identity-rechecks`（`internal/nodeoperations/adapters/http/recheck_pending_identity.go`，新）。编排装配 `cmd/parcel-api/assemble_identity_recheck.go`（新，`transactionalIdentityRecheck` 包一笔事务）。
- **复核目录**：`UnidentifiedItemCatalogueRow` 加 `JudgmentVersion`、`SourcePreserved`、`Mark`、`SucceededBy`；HTTP 待识别行加 `judgmentVersion`、`sourcePreserved`（恒在场）、`externalMark`（只在来源在时出现，空号也带空串）、`succeededBy`（只在已被接续时出现）。
- **管理台**（`apps/admin-web/src/pages/operations/`）：`identity-recheck-api.ts`、`identity-recheck.ts`（纯判读：外部标识观察一格、重核可否、答复说明、问题码）、`identity-recheck.test.ts`；`records-api.ts` 待识别行加四格；`NodeOperationsReviewPage.tsx` 待识别实物区加「外部标识观察（来源原样）」「判断版本」两列与「动作」列（可发起的给「重核」按钮，不可发起的照列为什么），答复在列表下方逐格说明，追加了一版就重读列表。外部标识观察三种「看不见号」各写各的字：来源未保全、`（设备未报号）`、号两端带空白或带控制字符时加引号转义示出（身份核对逐字比对，看不见的 Tab 就是零候选的原因）。

**信封形状与事件类型**（PS → NO 门铃）

| 格 | 取值 |
|---|---|
| Source | `idp-parcel/parcel-shipment` |
| Type | `parcel-shipment.external-identifier-relation.formed`（PS 导出 `pspostgres.ExternalIdentifierRelationFormedEventType`；NO 消费者自写 `noinbox.ExternalIdentifierRelationFormedEventType`，两串由 `cmd/parcel-dispatch` 的对照用例钉住） |
| ID | `outboxintent.FingerprintEventID("external-identifier-relation", 租户, 标识类型, 分配方, 号码, 包裹, 形成依据)`——ADR-0185 的关联键，一条关联一封 |
| PartitionKey | `outboxintent.FingerprintEventID("external-identifier-number", 租户, 号码)` 的定长串：同一个号的门铃排一条队；分区登记表加一行「主体：租户/外部标识号码（定长指纹）」 |
| Scope | 租户 |
| Subject | 号码原样；号码越过 `eventing.MaxSubjectLength`、带控制字符或不是合法 UTF-8 时换成 `NUMBER-SHA256/<号码的 sha256 十六进制>` |
| OccurredAt / RecordedAt | 关联的形成时刻 / 关联行的 `recorded_at` |
| 载荷 | `{"tenantId","number"}`，号码原样；不带包裹、分配方、标识类型或委托 |

**号码越界在 PS 侧怎么处理**（票 operator-channel/54 的口径）：先查了，号码进册前没有长度门，也不拦控制字符。客户包裹件号经 PS 提交口去两端空白后只拦全空白；渠道包裹件号只拦全空白；库面 CHECK 只拦全空白；Postgres text 本身拒 NUL。门铃与关联同一笔事务，信封若被框架确定性拒收，委托接受或面单结果会一并回滚，重试多少次都一样。为一封门铃挡住一份接受，代价倒置了。所以这样的号在 Subject 上换成号码摘要，载荷照旧原样带号：NO 按载荷里的号比对，Subject 只给人看。ID 与分区键都是定长指纹，不随号码长度变。Scope 取租户，与各口同一口径，不另设门（同票 54 的记法）。用例 `TestANumberTheSubjectCannotCarryStillRingsByItsDigest` 钉 640 字节的号与带 Tab 的号：接受照常落册、号照常查得、Subject 是摘要。

**消费者装配**（`cmd/parcel-dispatch/assemble.go` 只加行）：

```go
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
```

```go
	identityRechecks, err := recheckPendingIdentitiesConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}

```

```go
			noinbox.ExternalIdentifierRelationFormedEventType:      identityRechecks,
```

路由表那一行加在第一段末尾（`veinbox.TrackingProjectionDerivedEventType` 之后），键比该段最长的键短，gofmt 不重排对齐。

**人工重核命令口的路径与授权**

- 路径 `POST /node-operations/identity-rechecks`，载荷封闭 `{"judgmentVersion": "<收寄册上那一版判断版本>"}`；租户只取认证结果，载荷带身份键答 400。
- 归族照 ADR-0151 决定一核对：它是管理台上作判断的口，形成生产事实（可能追加关联收寄并交 PS 采用），归操作者渠道「运营决定」能力面。决定种类新增 `PENDING_IDENTITY_RECHECK`（`accessidentity.DecisionPendingIdentityRecheck`，库面封闭集随迁移 `migrations/access_identity/0007_pending_identity_recheck_decision.sql` 放开）。准入范围按（`OPERATION_DECISION`，`PENDING_IDENTITY_RECHECK`）判（ADR-0151 决定三）。认证方 `noaccess.OperatorDecisionAuthenticator`（新），Intake `nodeopshttp.OperatorDecisionIntake`（新），先认证、后解载荷。
- `cmd/parcel-api/endpoints.go` 只加行：参数表末尾加 `identityRecheck nodeopshttp.IdentityRecheckHandler,`；端点表在 `/node-operations/receptions` 之后加两行注释与一行
  `{Pattern: "/node-operations/identity-rechecks", Handler: nodeopshttp.NewRecheckPendingIdentityEndpoint(operatorDecisions.nodeOperations, identityRecheck)},`。
- 答复分格（ADR-0022、ADR-0186）：

| 格 | HTTP | outcome / code |
|---|---|---|
| 唯一，追加关联收寄 | 201 | `INTAKE_FORMED` |
| 多候选、带信号、一码两物，追加冲突版 | 201 | `IDENTITY_CONFLICT` |
| 零候选，判断不变 | 200 | `STILL_UNIDENTIFIED` |
| 已重核过（已有后继或派生键已在册） | 200 | `EXISTING_RESULT`，带 `currentVersion` 与 `recordedOutcome` |
| **记身份冲突的待识别** | **200** | **`IDENTITY_CONFLICT_RECORDED`** |
| **没有来源文档的旧行** | **200** | **`SOURCE_NOT_PRESERVED`** |
| 指名的不是待识别实物 | 200 | `NOT_PENDING_IDENTIFICATION` |
| 本租户册上没有这一版 | 404 | `JUDGMENT_NOT_VISIBLE`（不带 outcome，与越权探针同答） |
| 身份核对读不动、落册或入队失败 | 500 | `NO_ANSWER_FORMED`（整笔回滚） |
| 接入格 | 403 / 401 / 403 / 403 / 503 / 400 | `ACCESS_CHANNEL_NOT_CONFIGURED` / `OPERATOR_CREDENTIAL_REJECTED` / `OPERATOR_NOT_GRANTED` / `OUTSIDE_ADMISSION_SCOPE` / `IDENTITY_DEPENDENCY_UNAVAILABLE` / `MALFORMED_REQUEST` |

冲突行与旧行答 200 带 outcome 的理由：服务端形成了答案，也知道为什么不重核。这不是调用方的错（4xx 要人改请求，而请求本身没错），也不是没形成答案（5xx 会让人重试，而重试结果不会变）。

**完成判据与用例**

| 判据格 | 用例 |
|---|---|
| 编排：零候选待识别重核成关联收寄，接收时刻、控制、前版、意图一封；同一信封重投、同一件再人工重核不再追加 | `internal/nodeoperations/application/recheck_pending_identity_test.go`：`TestAPendingUnitIsRecheckedIntoAnIntakeOnceItsMarkIsRelated`（`AT-NO-026`）；派生键撞键重放 `TestARecheckWhoseDerivedKeyIsTakenAnswersTheWinner`；服务结果读不动 `TestARecheckRecordsAnUnreadableServiceResultLikeTheReception` |
| 编排，非唯一各格 | `TestARecheckWithSeveralCandidatesAppendsAnIdentityConflict`（两个候选、一个候选带信号）；`TestARecheckOfAMarkAlreadyOnAnotherUnitAppendsConflictsOnBothSides`；`TestARecheckThatStillFindsNoCandidateAppendsNothing`（零候选、零候选带信号）；`TestARecheckThatCannotReadTheIdentityAppendsNothingAndFails`；另 `TestRecheckFailuresFormNoAnswer`、`TestARecheckWithAMissingDependencyIsRefusedAtConstruction` |
| 编排：冲突行、无来源旧行不重核 | `TestConflictedSourcelessAndFormedJudgmentsAreNotRechecked`（另含指名形成格、指名册上没有的版本） |
| 真库（经生产装配）：零候选收寄 → 委托接受形成关联 → 门铃入队 → `cmd/parcel-dispatch` 消费 → 追加关联版 → 意图一封、载荷指回原收寄时刻 → `JudgeIdentityBasis` 判成立 | `cmd/parcel-dispatch/node_identity_recheck_test.go`：`TestTheWiredDispatchRechecksAPendingUnitOnceItsMarkIsRelated`（`AT-NO-026`；末段同一个号再响一次门铃不再追加）；路由表 `TestAFormedExternalIdentifierRelationReachesTheRecheckConsumerThroughTheRouteTable`；类型对照 `TestTheRecheckConsumerAndThePSDoorbellAgreeOnTheEventType` |
| 真库：人工与信封落下同一结果；冲突行与无来源旧行的答法 | `cmd/parcel-api/identity_recheck_test.go`：`TestTheWiredManualRecheckLandsTheSameVersionAsTheDoorbell`（`AT-NO-026`）、`TestTheWiredManualRecheckLeavesConflictsAndSourcelessRowsAlone` |
| 并发：重核与同一包裹另一件实物的收寄经（租户、包裹）锁串行 | `TestARecheckAndAnotherUnitsReceptionSerializeOnTheParcel`（重核挂在读别的实物关联之后，另一件经生产装配送站等在包裹锁上；重核提交后它答记身份冲突，关联到那件包裹的实物只有一件） |
| PS 适配器：重放不重复入队；适用终态不入队 | `internal/parcelshipment/adapters/postgres/external_identifier_formed_handoff_test.go`：`TestAcceptanceRingsADoorbellPerFormedRelation`、`TestARetiredChannelNumberRingsNoDoorbell`、`TestANumberTheSubjectCannotCarryStillRingsByItsDigest`、`TestTheDoorbellRefusesToRingOutsideATransaction` |
| admin-web 三道门 | `identity-recheck.test.ts` 八条；门见下 |
| Go 门禁 | 见下 |

其余落点的用例：收寄库两问 `TestPendingUnitsAreFoundByThePreservedMark`、`TestAJudgmentIsFoundByItsVersion`；复核目录 `TestUnidentifiedItemsCarryTheMarkAndTheirPlaceInTheChain`（PG）、`TestUnidentifiedItemsCarryTheMarkObservationAndTheChainPlace`（HTTP）；命令口 `TestAManualRecheckThatFormsAnIntakeAnswersCreated`、`TestAManualRecheckAnswersEveryUnappendedCellAsAFormedAnswer`、`TestAManualRecheckMapsConflictAndUnreadableIdentity`、`TestTheRecheckEndpointMapsIntakeRefusals`、`TestUnconfiguredRecheckRefusesWithoutReadingTheBody`、`TestTheOperatorDecisionIntakeNamesOnePendingUnitForTheEnvelopeTenant`；认证方 `TestARecheckOperatorAuthenticatesForThePendingIdentityRecheckDecision`、`TestOperatorDecisionGradesTranslateIntoNodeOperationsGrades`；决定种类 `TestOperationDecisionFaceIsGrantedPerDecisionKind`（列表加一种）、`TestThePendingIdentityRecheckDecisionIsGrantedAndReadBack`；消费门 `TestAFormedRelationDoorbellIsHandledExactlyOnce`、`TestADoorbellMissingTheTenantOrTheNumberIsPoison`、`TestAFailedRecheckRollsBackAndTheRedeliveryRetries`、`TestAForeignTypeIsLoudOnTheDoorbellConsumer`、`TestADoorbellAsksTheRecheckByTheVerbatimNumber`；端点表 `businessEndpointProbes` 与 `swappedDecisionFaces` 各加一行（`TestEveryAssembledEndpointAnswersUnconfigured`、`TestSwappedDecisionFaces*` 覆盖新口）。

**先红后绿**：红在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs03-red.log`，分两轮。

- 第一轮：只写了用例，生产代码钉 `4f11563c` 一字未改，结果是编译红。原话举例：`undefined: application.RecheckPendingIdentityHandler`、`repository.FindRecheckableByMark undefined`、`undefined: adapter.ExternalIdentifierRelationFormedEventType`、`undefined: accessidentity.DecisionPendingIdentityRecheck`。
- 第二轮：只补了编译骨架（类型、字段、签名，方法体是桩），结果是断言红。原话举例：
  - 「按号重核 = [], want 恰一件 INTAKE_FORMED」
  - 「门铃的号 = [], want 两条关联各一封」
  - 「published = 0, want 1；失败码 = "dispatch.no_subscriber"——路由表没把门铃投给重核消费门」
  - 「按号找到 [], want [SYN-SCAN-NRS03-A SYN-SCAN-NRS03-B]」
  - 「status = 501, want 201」
  - 「decision kind PENDING_IDENTITY_RECHECK of ADR-0151 decision one: ("", access identity: decision kind is unknown)」

管理台的纯判读用例与 `identity-recheck.ts` 同一轮写成，没有单独跑红。

绿在同目录 `nrs03-green.log`。

**门**（`8106a6a5` 的工作树）

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./internal/parcelshipment/... ./internal/accessidentity/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/... ./tests/...` 退 0，有测试的包 39 个全 ok。`internal/accessidentity/...` 是门外加跑的：决定种类改了它。
- 管理台三道门：Node 22，在 `apps/admin-web` 下跑 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（553 过 0 败）、`pnpm exec vite build`，均退 0。日志在同目录 `nrs03-web-tsc.log`、`nrs03-web-tests.log`、`nrs03-web-build.log`。
- 机制清点在 `8106a6a5` 的干净检出上重生成，有差，单独一笔 `c70f2ac1`。
- 没有跑全量 `./...`。

**证据层级**：S（隔离测试库，夹具全为合成 `SYN-` 值；演示库 psb26_walk 未动）。

**越出地盘的改动**

- `internal/accessidentity/operation_decision.go` 及其测试、`internal/accessidentity/adapters/postgres/operator_registry_recheck_decision_test.go`（新）、`migrations/access_identity/0007_pending_identity_recheck_decision.sql`（新）：人工重核归「运营决定」能力面，要一种新的决定种类。
- `internal/nodeoperations/adapters/accessidentity/operator_decision_authenticator.go` 及其测试（新）：NO 此前只有作业事实与查阅读两种认证方。
- `internal/nodeoperations/adapters/inbox/`（新包）与 `internal/nodeoperations/adapters/parcelshipment/recheck_on_relation_formed.go` 及其测试（新）：消费门与门铃到编排的翻译。地盘只写了装配住 `cmd/parcel-dispatch`。
- `internal/nodeoperations/adapters/http/work_fact_intake.go`：`ErrOperatorNotGranted` 的错误文字从「作业事实授予」改成通用说法，这个哨兵现在两种 Intake 共用。哨兵与 code 都没变。
- `internal/architecture/partition_subject_registry_test.go`：新门铃口登记分区主体一行（门禁要求新口同笔登记）。
- `cmd/parcel-api/assemble_operator_decisions.go`：`operatorDecisionIntakes` 加 `nodeOperations` 一格，`buildOperatorDecisionIntakes` 多装一只；改了一行既有的 `return`。
- `cmd/parcel-api/main.go`（加装配与传参）、`cmd/parcel-api/endpoints_test.go`（探测表一行、装配调用多传一个参数）、`cmd/parcel-api/assemble_operator_decisions_test.go`（`swappedDecisionFaces` 一行）。

**给票 05 留的复用入口**

- **编排**：`application.RecheckPendingIdentityHandler.Recheck(ctx, RecheckPendingIdentityCommand{TenantID, Version})` 只看指名那一件，就是自投门铃要的入口。重核对象条件、当前有效版本判断、派生键与撞键重放都在共用的 `recheck` 里，票 05 不必另写。票 05 要在 `recheck` 的零候选那一支加一格：被重核那一版是「未核对」时追加「待识别 · 已核对」。今天这一支只答 `STILL_UNIDENTIFIED`。
- **身份读不动**：`Recheck` 交回带 `ErrParcelIdentityUnreadable` 的 error，正合自投门铃「留在 inbox 退避重投」。指名的版本不在册时交回 `ErrRecheckSubjectNotFound`，票 05 定是否当毒丸。
- **消费门**：`noinbox` 照 `ExternalIdentifierRelationFormedConsumer` 同法再起一扇门（自投门铃的事件类型，载荷 `{tenantId, judgmentVersion}`）。消费者名可以沿用 `node-operations/recheck-pending-identities`：inbox 键含来源，自投门铃的来源是 `idp-parcel/node-operations`，不与 PS 门铃撞账。
- **装配**：`cmd/parcel-dispatch/assemble_node_identity_recheck.go` 的 `nodeIdentityRecheck(db, outboxStore, clock)` 交回编排本身，自投门铃的消费者在这份文件里另起一个函数复用它，路由表加一行。

**未做**

- Go 那条路的后两格：非作者评审、推送方在干净检出上带 DSN 跑全量。
- ~~演示种子：`scripts/demo-seeds` 没加 `PENDING_IDENTITY_RECHECK` 的授予、准入范围对照与治理区间。演示租户在管理台点「重核」今天答 403 `OPERATOR_NOT_GRANTED`，答法如实，只是演示走不通。种子不在地盘，演示库也不许动，交推送方定是否补。~~ 已由评审补补齐，见下节。
- 票面「不做」各项照旧：后台扫表、未核对行的自投门铃（票 05）、身份冲突的处置、来源保全之前旧行的重核、第一期到本票之间已形成关联的补发。

**发现**（不改裁定）

- ~~**文档**：ADR-0151 决定一逐条列了运营决定的决定种类，本票加了第十二种「待识别重核」，归族依据就是那一条「管理台上作判断的口」。已接受的 ADR 不改写。若要在文档上留痕，可以在 ADR 索引或 ADR-0199 的关联处补一句，交推送方定。NO CONTEXT 与 UC-NO-002 的正文没碰，也没有发现必须改的句子。~~ 推送方裁定只在 ADR-0199 留一处，评审补已追加，见下节。
- **PS 侧既有的长度上限**：关联册的 `external_identifier_relation_by_number` 是按号码本身建的 btree，号长过 btree 一项上限（约 2.7KB）时关联写不进去，委托接受随之回滚。这是本票之前就有的，本票的门铃不加重它。NO 侧这次的按号索引取了 md5，避开了同一问题。
- **架构门禁 PBC-08**：门铃第一版是未导出类型，门禁要求每个写口有同包「无事务被拒」的负向证据，因此红了。改成导出的 `ExternalIdentifierDoorbell`，补了 `TestTheDoorbellRefusesToRingOutsideATransaction`。

### 评审补（2026-10-19，同一作者子代理，同一分支）

推送方裁定了回报里留下的两件，另点名一句头注，并要求先 rebase 到 main `e8ef2398`。no-reception-identity/05 已经进 main，身份一格改读链头。

**提交**（rebase 之后）

| 提交 | 内容 |
|---|---|
| `445a2d08`、`3a7b64a4` | 原实施笔与完成记录笔 rebase 后的新 SHA。rebase 只在 AT 覆盖盘点上撞了一处：main 已有「追记八」，本票那一条改记为「追记九」，计数照 main 的基数重算（①294→295，NO 21→22；②236→235，NO 33→32）。原机制清点笔 `c70f2ac1` 撤下，由本节最后一笔重生成。 |
| `0f1a3bd9` | 头注与断言（见下） |
| `1deb9017` | 演示种子 |
| `44e0c02c` | ADR-0199 复核记录 |
| `56400c7d` | 机制清点在 `44e0c02c` 的干净检出上重生成：迁移 226→228，接入面端点 141→142，直投路由表 28→29 |
| 本笔 | 票面评审补 |

**1. 演示种子**：照其余运营决定同形补齐，取值都是 `SYN-`。
- `data/access/02-grants.json`：给配置员加授予 `SYN-GRANT-DEX-CONFIG-PENDING-IDENTITY-RECHECK-01`，依据 `SYN-GRANT-BASIS-DEX-15`。各运营决定的授予都给这一个主体，NO 此前没有运营决定。
- `03-admission-scope-mappings.json`：加对照 `SYN-MAP-OD-PENDING-IDENTITY-RECHECK-01`，对象范围 `SYN-PILOT-SCOPE/operation-decisions@v1`。
- 新文件 `data/governance/23-authority-interval-operation-decision-pending-identity-recheck.json`，形状同 21、22。
- `seed.sh`：在 22 之后加一行登这份区间。
- `README.md`：授予表里配置员那一行的运营决定清单加「待识别重核」。

既有条目一条未动，也没有重排。`scripts/demo-seeds` 下没有测试文件，两个登记 CLI（`cmd/parcel-access-register`、`cmd/parcel-governance-register`）的包测试照跑、都 ok。另做了一次装载核对：
- 在 55432 上建临时库 `nrs03_seedcheck`，用 `scripts/demo-seeds/migrate` 施加全部迁移（共 229 步，含 access_identity 0007）。
- 用两个 CLI 登了 governance 20–23 与 access 01、02、03：新授予、新对照、新区间都答 RECORDED / REGISTERED，全程退 0。
- 核对完删掉了临时库。日志在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs03-seedcheck.log`。
- 演示库 psb26_walk 没碰。

**2. 决定种类只在一处留痕**：ADR-0199 在既有那节「复核记录（2026-10-19，ADR-0210 部分停用几句；决定原文不动）」之后、Links 之前，追加「复核记录（2026-10-19，决定原文不动）」一节，两句。放在 Links 之前，是照前一节复核记录的位置，让 Links 仍是最后一节。ADR-0151 与 adr/README 没动。

**3. 头注与链头读法**
- `receive_delivered_unit.go` 的 `appendIdentityConflicts` 头注改为：身份一格只读各链当前有效版本（ADR-0210 决定五）；冲突版接续被依的那条链、成了链头；先到那件没有别的链时，落成缺少·{待识别, 身份冲突}，原判断上的关联不再参与。
- 重核编排在两处注释里引的「记身份冲突的不重核」改了理由：ADR-0199 复核记录第一条停用了「在 `JudgeIdentityBasis` 里一直判身份冲突」那句，所以理由改为 ADR-0210 决定一（身份冲突只经人工处置解除）。管理台三处措辞同改。
- **重核版经身份读口会被读到。** 它带前版：前版是被重核那一版，身份读口从 main 起读回 `predecessor_version`。链头读法下照预期成立：
  - 派发真库用例 `TestTheWiredDispatchRechecksAPendingUnitOnceItsMarkIsRelated` 加了断言：身份读口读回两行，重核版带前版；身份一格成立，所依收寄是重核版。原待识别已被接续，不再参与。
  - 并发用例 `TestARecheckAndAnotherUnitsReceptionSerializeOnTheParcel` 改钉两件都是缺少·{待识别, 身份冲突}。A 那条链是原待识别 → 重核版 → 冲突版，链头是冲突版。
  - 这与 ADR-0199 复核记录第三条「重核版是链头、带关联，判成立」一致。

**门**（tip `44e0c02c` 的工作树，即本节代码与种子都在的那一版）
- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN 跑 `go test -p 1 -count=1`，包含前一轮那组包，再加跑 `./cmd/parcel-access-register/...`、`./cmd/parcel-governance-register/...`、`./scripts/demo-seeds/...`。结果退 0，有测试的包 41 个全 ok；`scripts/demo-seeds` 的两个包没有测试文件。日志在同目录 `nrs03-green2.log`。
- 管理台三道门：Node 22，`tsc -b --noEmit`、`node scripts/run-tests.mjs`（553 过 0 败）、`pnpm exec vite build`，均退 0。日志在同目录 `nrs03-web-*2.log`。
- 机制清点有差，单独一笔 `56400c7d`。

**证据层级**：S。

**越出地盘的改动**（本节新增）：
- `scripts/demo-seeds` 下五处：access 两份、governance 新文件一份、`seed.sh` 一行、`README.md` 一行。
- `docs/adr/0199-…` 追加一节。

两处都按推送方裁定动。
