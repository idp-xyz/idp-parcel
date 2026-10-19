# 05 号码只在一份已接受委托里在册时，别人贴着同号的实物送站被唯一关联到那一件

Category: bug
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（ADR 用号 0195），分支 `claude-psxm05`。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 0 研判）
Blocked by: 无
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)；[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C2 第二张草案
地盘：
- docs/adr/（新 ADR，按裁定）
- docs/domain/parcel-shipment/CONTEXT.md
- docs/domain/CONTEXT-MAP.md
- docs/domain/node-operations/CONTEXT.md
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md
- internal/parcelshipment/adapters/postgres/external_identifier.go
- internal/parcelshipment/ports/external_identifier.go
- internal/nodeoperations/adapters/parcelshipment/identity.go
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/application/receive_delivered_unit.go
- cmd/parcel-api/assemble_reception_identity_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 出处

票 ps-external-mark-relations/02 的评审遗留（operator-channel/26 序 0）经研判与两路对抗核验，会话 `idp-parcel-9b`，钉 main `8d22729f`。评审原题是「决定六与 UC-NO-002 的范围冲突」；裁定维持决定六，本票收它留下的另一面。

## 现象

真库实测（经生产装配 `buildReceptionOrchestration`，合成夹具，证据只记 `S`）：

- **跨客户账户。** 客户账户一的委托已接受，客户包裹件号 N 进了册；客户账户二的委托也报了 N，只提交、未接受。客户二的实物贴着 N 送站，答`节点收寄已形成`，关联到客户一的包裹，控制建立，没有身份冲突。`deliveredBy` 写客户二还是一个不明交付方，答案一样。
- **同一客户账户跨委托。** 同一客户账户的两份委托都报了 N，只有第一份已接受。第二份的实物送站，唯一关联到第一份的包裹；同号的另一件实物再到站，也一样（一码两物见 no-reception-identity/01）。第二份也接受之后，才答身份冲突。

落点有三处：

- NO 的 `ReceiveDeliveredUnitHandler.receive` 按候选数分支，恰一个就关联。
- PS 读口只认已接受委托形成的关联（PS CONTEXT「接受前的声明包裹不是正式包裹，它的号码不进身份核对」），看不见另一方的声明。
- PS 采用这份收寄时，`AdoptOnNodeIntakeAdapter.HandleFormedNodeIntake` 按关联到的包裹，经 `FindCurrentAcceptedByParcel` 反查目标委托。所以 UC-PS-003「适用资格判断」里客户账户一致那一项，对节点来源是自己比自己。

另按派发装配读码（未实测）：错关联会随同一封收寄信封，进 PS 的采用（`adoptNodeIntakeConsumer`）与 VE 的追踪投影（`deriveProjectionConsumer`）。

够不够得着：

- 生产形态没有接单入口（`buildIsolatedSubmissionIntake` 在生产形态交回 nil），风险是潜伏的。
- 演示租户经隔离提交入口与派发受理链，走得到`已接受`（合成演示动线 2026-10-14 补记）；提交入口只在同一草案内查重，所以同客户那一格今天就走得到。
- 跨客户那一格，要等第一条多客户接单入口。

## 为什么是缺陷

- NO CONTEXT「实物身份与事实完整性」：「无法可靠识别的到站实物必须建立待识别实物」。
- UC-NO-002 应用流程：「唯一且证据充分时关联」。
- PS CONTEXT「外部标识关联」：「客户账户给的号在该客户账户内算数」。命中别人编号空间里唯一的一条，不是可靠识别。
- NO 端口 `ParcelIdentityView` 的头注「恰一个→关联」，把「唯一」当成了「充分」，仓里没有记录论证过。

## 要裁的一问

唯一命中，什么时候算「证据充分」？这属产品策略（判断方法），归开发方现在做；难逆转的取舍进 ADR。下面的候选可以组合，也可以分期：

- **(a) 歧义信号。** 号码同时被租户内一份未终局、未接受的委托声明时，核对不答唯一命中，改答冲突或待识别。只拦得住另一方已提交的那一格；而且要先改 PS CONTEXT「接受前……不进身份核对」那一句的读法：声明不进候选，但进歧义判断。
- **(b) 客户编号空间的号不自动关联。** 没有可验证的交付范围时，客户包裹件号的唯一命中也只记候选、走待识别。这是最稳的；代价是票 02 的识别效果，在交付范围建模之前整体失效。
- **(c) 先建可验证的交付范围。** 用送站预报或交接单绑定客户账户，落在范围内的唯一命中才关联。这是根治；要 NO 先建模，先改 PS CONTEXT「按外部标识核对包裹身份」那一条与 CONTEXT-MAP 同句，再另立 ADR 部分停用 ADR-0184 决定六。
- **(d) 认作已知代价。** 唯一命中照旧关联，靠一码两物检测（no-reception-identity/01）、测量复核与异常处置兜住。须写进 ADR，由用户认可。

时限：生产形态第一条接单入口（`PAR-INT-01` 真渠道或首方 API）上线之前，须裁定并修好；演示租户同客户那一格的排期，由分诊定。哪个授权交付方可以替哪个客户账户送货，属租户取值，不在本票。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取 (a) 作现在的产品策略：核对时，号码同时被租户内一份未终局、未接受的委托声明，唯一命中不再答唯一，改答身份冲突、走待识别。声明不进候选，只作歧义信号；这要先改 PS CONTEXT「接受前的声明包裹不是正式包裹，它的号码不进身份核对」那句的读法，并出 ADR（用号 0195，会话已占），写明与 ADR-0184 决定六的关系。(c) 可验证交付范围记作终局方向，随一线客户端收寄流程另立；(b) 不取（票 02 的识别效果会整体失效）；(d) 不取。时限：生产形态第一条接单入口上线之前修好。越权风险点：改 PS CONTEXT 读法属 PS owner 的领域语言，复核时看歧义信号会不会把同一客户合法的重复用号也一律打成待识别。

## 做什么（裁后）

- 裁定写进本票。难逆转的进 ADR，必要时部分停用 ADR-0184 的相应决定；先改 CONTEXT，再改引用它的 UC。
- 照裁定改核对或收寄判断；把票 04 的刻画用例按裁定有意翻转，并改写头注；跨客户与同客户两格各有回归用例。

## 不做

- 不按 `deliveredBy` 自报收窄，理由见 ADR-0184 复核记录「决定六与 UC-NO-002「权限、隔离与审计」」。
- 一码两物（no-reception-identity/01）、身份后来被确认（`AT-NO-026`）。
- 授权交付关系这类租户取值。

## 完成判据

- [x] 「要裁的一问」已裁，写进本票；难逆转的进 ADR，CONTEXT 与 UC 照 AGENTS.md「改文档」的顺序同步。
- [x] 跨客户与同客户两格按裁定作答，各有经生产装配的真库回归用例；票 04 的刻画用例按裁定翻转。
- [x] 「两份已接受委托撞号 → 身份冲突」的用例照旧绿。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑受影响包与 `internal/architecture` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，作者子代理，分支 `claude-psxm05`，基于 main `423b517a`）

### 落点

- **PS 读口**：`ExternalIdentifiers.FindParcelsByExternalIdentifier` 改答 `ports.ExternalIdentifierParcels`，即候选 `Parcels` 加歧义信号 `DeclaredBySubmittedRequest`。两半出自同一条 SQL：委托接受与关联写入同一笔事务，同一个快照里一份委托要么还是`已提交`、算信号，要么已接受、关联在册、是候选。
- **PS 迁移** `0026_submitted_request_customer_parcel_numbers.sql`：在委托表上建部分 GIN 索引，键是快照里的 `customerParcelNumbers`，用 `jsonb_path_ops`，只收 `state = 1`。临时 EXPLAIN 核过谓词能用上它（没留文件）。
- **NO 端口**：`ParcelIdentityView.ResolveParcelIdentity` 改答 `ports.ParcelIdentityAnswer`，即 `Candidates` 加 `Ambiguous`，认唯一只经 `Unique()`。防腐适配器 `ParcelIdentityView` 照转信号。
- **NO 编排**：`ReceiveDeliveredUnitHandler.receive` 只改身份核对那一段。`FindOtherUnitIntakes`、服务结果读口与关联都改用 `Unique()` 交回的包裹；冲突分支的注释补上歧义信号这一格。其余代码没有重排。

### 歧义信号的判据

- **算**：本租户里一份`已提交`（`domain.ShipmentRequestSubmitted`）委托的快照，客户包裹件号里有这个号，号码逐字相等。接受判断任务等补充、等复核、等登记、等授权处置时，委托都仍是`已提交`，都算。
- **不算**：已拒绝（含补充期限届满的确定性拒绝）与已撤回是终局。已接受的号随接受进了关联册，是候选，不经信号。他租户不算。草稿不在 PS。PS 领域里没有单独的「过期」态，过期即拒绝。
- **落在哪一层**：判据落在 PS 适配器那条 SQL 里，状态值由领域常量传参；PS 端口只交一个是否值，不交委托、客户账户或声明包裹。NO 不读 PS 的表，判法在 NO 端口值的 `Unique()` 上：收寄编排经它认唯一，重核从收寄编排抽共用判断时随之继承（票 no-reception-source/03 已补记）。

### ADR-0195 摘要

PS 随候选答出歧义信号，判据是`已提交`。候选与信号同一次读。NO 只认「恰一个候选、且没有信号」为唯一命中：

- 恰一个候选带信号，与多候选同格：答待识别、记身份冲突，候选只记那一件。不问服务结果，不交意图，也不进 ADR-0196 那一支。
- 零候选带信号，仍是待识别、不记冲突，否则那份委托接受之后的重核会跳过它。

ADR-0199 决定三的重核经同一个 `Unique` 继承这份判断。信号消失不交信封。存储靠迁移 0026。

对旧决定：

- 部分停用 ADR-0184 决定五「只交回包裹，不交分配方、客户账户或委托」里的「只交回包裹」：答复另带信号，「不交分配方、客户账户或委托」照旧。
- ADR-0184 决定六维持。它的复核记录「决定六与 UC-NO-002「权限、隔离与审计」」代价「其二」里，对方已提交的那一格由本记录收住；对方从未报过这个号、对方委托已被拒绝或撤回，这两种情形仍是已知代价。
- ADR-0184 文末追加「复核记录（2026-10-19）」，Status 行与 Links 加前向指针，adr/README 的 0184 行标注部分停用，新增 0195 行。
- ADR-0196 与 ADR-0199 不改。

越权风险点列在 ADR-0195 文中，裁定点名的「同一客户合法的重复用号也一律打成待识别」在其中。

### 文档

- **先改 CONTEXT**：PS CONTEXT 新立「歧义信号」；「接受前的声明包裹……不进身份核对」改读为不形成关联、不成为候选，只在`已提交`时作信号；「按外部标识核对包裹身份」补上答复两半与唯一命中的判法。CONTEXT-MAP `parcel-shipment → node-operations` 的核对句与重核交接句同步。NO CONTEXT 加一条可靠识别的规则，重核那一句补「恰一个候选且没有歧义信号」。
- **再改 UC**：UC-NO-002 步骤 4、「权限、隔离与审计」首条、新增验收 `AT-NO-052`。

### 完成判据对应的用例

- **裁定入 ADR、CONTEXT 与 UC**：见上面的文档提交。
- **跨客户与同客户两格（经生产装配、真库）**：`cmd/parcel-api` 的 `TestTheWiredReceptionAnswersAConflictWhenAnUnacceptedRequestAlsoDeclaresTheNumber`，子用例分「跨客户账户」「同一客户账户」「册里没有、只有声明」。它由票 04 的 `TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber` 翻转而来，已改名、改头注。
- **拒绝、撤回之后信号消失**：`TestTheWiredReceptionAssociatesOnceTheOtherDeclarationIsRejectedOrWithdrawn`。
- **两份已接受委托撞号仍答身份冲突**：`TestTheWiredReceptionAnswersAConflictWhenTwoAcceptedRequestsShareANumber`，未改，照旧绿。
- **PS 真库**：`TestASubmittedDeclarationSignalsAmbiguityWithoutBecomingACandidate`、`TestADeclarationStopsSignallingOnceItsRequestIsDecided`。
- **NO 适配器**：`TestADeclarationByASubmittedRequestBecomesTheAmbiguitySignal`。
- **NO 编排**：`TestAUniqueHitThatAnUnacceptedRequestAlsoDeclaresIsAnIdentityConflict`，子用例分唯一候选带信号（含先到那件不追加冲突版）、零候选带信号、多候选带信号。

### 先红后绿

- **红**（生产代码未改、只改用例时跑的）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm05-red.log`。真库用例答 `outcome = INTAKE_FORMED, want PENDING_IDENTIFICATION——号码另有未接受的声明，唯一命中不算唯一`，「跨客户账户」「同一客户账户」「被拒绝」「被撤回」都红；「册里没有、只有声明」本就绿。PS、NO 各层用例编译不过，报 `undefined: ports.ParcelIdentityAnswer`、`undefined: psports.ExternalIdentifierParcels` 与 `answer.Parcels undefined`。
- **绿**：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm05-green.log`。

### 门（作者自验）

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN 跑 `go test -p 1 -count=1 ./internal/parcelshipment/... ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-frontline-import/... ./internal/architecture/...`，全 ok。另跑 `./migrations/...` 与 `./internal/platform/migrate/...`，也是 ok。
- 机制清点在干净检出上重生成，迁移份数随 0026 变，单独一笔提交。
- 没跑全量 `./...`，留给推送方。

### 证据层级

`S`：夹具全为 `SYN-` 合成值与隔离合成引用。

### 越出地盘的改动

- **身份读口换形状必改的测试替身**：`internal/nodeoperations/adapters/http/receive_delivered_unit_test.go`、`cmd/parcel-frontline-import/vertical_test.go`、`internal/nodeoperations/application/receive_delivered_unit_test.go`。
- **地盘文件的同包测试**：`internal/nodeoperations/adapters/parcelshipment/identity_test.go`、`internal/parcelshipment/adapters/postgres/external_identifier_test.go`。
- **PS 迁移** `migrations/parcel_shipment/0026_submitted_request_customer_parcel_numbers.sql`。
- **ADR 与清点**：`docs/adr/0184-…`（只追加复核记录与前向指针，正文不动）、`docs/adr/README.md`、`docs/product/MECHANISM-INVENTORY.md`。
- **重核票补记**：`.scratch/no-reception-source/issues/03-…` 加了一段补记，说重核继承 `Unique()`，完成判据没动。
- **与并行票 no-reception-source/02 的合并面**：`receive_delivered_unit.go` 只动 `receive` 里身份核对那一段与冲突分支注释，`ports.go` 只动 `ParcelIdentityView` 一处并新增 `ParcelIdentityAnswer`。没碰 `reception.go` 与 NO 迁移。

### 未做

- 非作者评审，推送方在干净检出上带 DSN 跑全量（完成判据末格）。
- 可验证交付范围，即裁定 (c) 的终局方向，随一线客户端收寄流程另立。
- 身份冲突的处置与解除：歧义信号落下的冲突要靠它解开，这一项未立票。
- GLOSSARY 没加「歧义信号」：它是 PS 定义、经 CONTEXT-MAP 交给 NO 的词，没进跨上下文统一语言。是否要加，留评审定。

