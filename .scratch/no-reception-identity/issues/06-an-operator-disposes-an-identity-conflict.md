# 06 身份冲突处置（UC-NO-004）：运营决定「身份冲突处置」逐链追加处置版、连带指明，按包裹的读面认处置截断，重核跳过被排除的包裹；管理台复核页加处置动作

Category: enhancement
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-nri06`，NO 迁移号 0010、access_identity 迁移号 0011、治理种子 governance/27；与 no-reception-source/04 并行。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 04 拆出（代决，用户 2026-10-16 授权「你作为业务和系统专家你来自决」）
Blocked by:
- no-reception-source/03 进 main。处置之后可再经重核，决定九改的正是它落下的重核分格；它还同改收寄库 PG 适配器、复核目录与管理台待识别实物页。
- 05 进 main。处置要靠身份一格读链头才解得开，`AT-NO-053`、`AT-NO-054` 断言的就是解开之后的身份一格。
- 与 no-reception-source/04、05 同改收寄库 PG 适配器与迁移号，不设阻塞边：谁后进谁 rebase、取下一个空迁移号；动手前照[并行会话](../../../docs/agents/parallel-sessions.md)看清在途分支。
父票：[04](./04-identity-conflicts-have-a-disposition-and-release.md)
出处：票 04 裁定各点；[ADR-0210](../../../docs/adr/0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定一至四、六至九；[UC-NO-004](../../../docs/application/node-operations/UC-NO-004-DISPOSE-AN-IDENTITY-CONFLICT.md)。
地盘：
- internal/accessidentity/operation_decision.go 及其测试（决定种类多一种）
- scripts/demo-seeds/data/access/02-grants.json、03-admission-scope-mappings.json，scripts/demo-seeds/data/governance/ 下新起一份这一种运营决定的生产权威区间（照既有 `authority-interval-operation-decision-*` 那几份的形状）
- internal/nodeoperations/domain/（处置文档的形状与规范化、处置候选、两种结果的处置版构造）及其测试
- internal/nodeoperations/ports/ports.go（记录带处置文档，读口与写口）
- internal/nodeoperations/application/（新处置编排）及其测试；no-reception-source/03 落下的重核编排（决定九那一格）
- internal/nodeoperations/adapters/postgres/reception.go（按实物读各链当前有效版本、按派生键读处置、`FindOtherUnitIntakes` 截断）、parcel_containment_view.go（截断）、review_catalogue.go（待识别实物页要的链头与处置）及各自测试
- migrations/node_operations/（处置文档一列；下一个空号）
- internal/nodeoperations/adapters/http/（处置命令口与「运营决定」操作者 Intake；复核目录查询多带的格）及其测试
- internal/nodeoperations/ports/catalogue_read.go
- cmd/parcel-api（端点表、装配、真库用例新起文件）
- apps/admin-web/src/pages/operations/**（待识别实物页的处置动作）
- .scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md（补记末尾追加一行回写：本口凭哪条代数）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `4f11563c`，读码）

- 收寄册上记身份冲突的行没有任何入口能接续：重核跳过冲突（ADR-0199 决定三），收寄编排只在新的送站里追加。
- 操作者渠道「运营决定」能力面的决定种类（`accessidentity.DecisionKind` 的词表）没有身份冲突处置这一种；NO 没有任何运营决定口。
- `Receptions.FindOtherUnitIntakes` 与 `ParcelContainmentView.LoadParcelContainment` 按行读关联与候选，不看版本链；复核目录 `ReviewCatalogue.ListUnidentifiedItems` 逐行列待识别，分不出哪一版是当前有效版本。

## 做什么

照 ADR-0210 与 UC-NO-004：

1. **授予**。`accessidentity` 的决定种类加「身份冲突处置」（字面量本票定，建议 `IDENTITY_CONFLICT_DISPOSITION`）。演示种子给演示租户的配置操作者加这一种授予、登记准入范围对照与生产权威区间；生产装配不配任何默认，未授予答「无此决定种类的授予」。
2. **命令口**。端点表加一行（路径本票定，建议 `/node-operations/identity-dispositions`），挂「运营决定」操作者 Intake：照 `tfhttp.OperatorDecisionIntake`、`shipmenthttp.OperatorDecisionIntake` 的做法在 NO 的 HTTP 包里另起，或抽共用，实施定。信封各格照 ADR-0151 决定三；坏载荷 400 `MALFORMED_REQUEST` 的各种照 ADR-0210 决定六列全。
3. **编排**（ADR-0210 决定二至四、六、七）：按（租户，所处置的版本）认重放 → 先实物后包裹取锁 → 读主实物各链当前有效版本 → 核版本是否仍是链头、各链合起来是否判冲突 → 算处置候选、核指定的包裹 → 指定一件时按处置截断读此刻关联到它的别的实物、与连带实物对照 → 主实物与连带实物逐链追加处置版 → 指定一件的那几版照 ADR-0186 取服务结果 → 需要时备一封收寄发布意图 → 原子提交。
   - 答复格：身份冲突已处置、已有结果（`EXISTING_RESULT`）、已处置、版本已换代、无可处置的冲突、不在候选之内、关联对象另有归属。线上名本票定，写进完成记录；回显的内容照 UC-NO-004 结果表。
   - 第四期（no-reception-source/04）的实物锁已在 main 就取，不在就只取包裹锁；次序与收寄、重核同一口径。
   - no-reception-source/05（第三期）已在 main 时，「都不是」那一版写身份核对态「已核对」；不在时由它的迁移回填。
4. **存储**。收寄表加处置文档一列（jsonb，自带形状版本，ADR-0014），只在处置版上有；迁移头注写既有行为空的理由。派生来源标识 `IDENTITY-DISPOSITION/<前版的判断版本>`；内容指纹格写比对格（结果、指定的包裹、连带实物集合）的规范化摘要，同一次处置的各版同一个摘要；来源文档取所在链起点那一版的，起点没有就不带。
5. **处置截断**（ADR-0210 决定八）。`FindOtherUnitIntakes` 与 `ParcelContainmentView` 两路都不再算被处置版接续过的行；处置版自己带的关联照算。还没处置过的实物读法照旧，用例钉住。截断用沿前版回溯还是在处置版上记下它截断的各版，实施定，不得改写旧行。
6. **重核例外**（ADR-0210 决定九）。被重核那一版是「都不是」的处置版、PS 唯一命中的那件包裹在它排除之列时，不追加，照零候选办。其余分格照旧。
7. **复核目录与管理台**。待识别实物页按实物把各链当前有效版本列在一起，分得出冲突是否已处置、处置了什么（结果、指定的包裹或排除的那一组、操作者、依据引用、时刻）；「处置」动作：选指定一件或都不是、填依据引用；指定一件时，「关联对象另有归属」的回显带出须连带的实物，确认后一并提交。**走 Go 那条路**：随本票分支、非作者评审与推送方全量，不走前端切片的 main 直做。admin-web 三道门照 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境) 的口径。
8. **逐口核**（ADR-0151 决定三）。本口生来自带代数，凭哪条写进完成记录，并在 operator-channel/15 补记末尾追加一行回写。
9. **AT 点名**：`AT-NO-053` 至 `AT-NO-060`；`.scratch/at-coverage-inventory.md` 照扫描口径更正。

## 不做

- 歧义信号消失后的自动解除（ADR-0210 决定一，后续方向，PS 要先交信封）。
- 对没有身份冲突的关联作单独更正口；「同一实物存在相互冲突的外部标识」那一半。
- PS 采用回滚、撤销信号；VE 侧的发现与协调（ADR-0196 决定五）。
- 向 `party-commercial` 问授权坐标（ADR-0210 决定一）。
- 身份一格读链头（票 05）。

## 完成判据

- [x] 编排用例，一码两物（`AT-NO-053`）：后到那件指定那件包裹、先到那件列为连带实物 → 后到那件追加关联版、先到那件追加「不是它」的版本，原判断与冲突版逐字不变；身份一格后到那件成立、先到那件缺少·待识别；意图一封、属后到那件；先到那件已交出的意图不撤回。反过来，先处置先到那件、指定那件包裹（后到那件没有关联，不必连带）→ 先到那件此前已就那件包裹交出过，不再交意图；后到那件另行处置为都不是。
- [x] 编排用例，确认都不是（`AT-NO-054`）：多候选与带歧义信号各一条 → 待识别版、不记冲突、记下排除的那一组，不交意图；之后重核仍只唯一命中被排除的包裹 → 不追加；号码另有新的包裹关联、重核唯一命中它 → 照常追加关联版。
- [x] 编排用例，关联对象另有归属（`AT-NO-055`）：漏列、多列、连带实物版本过时、连带实物另牵连别的包裹，各一条，都不落版并回显此刻关联到指定包裹的实物与版本。
- [x] 编排用例，代数（`AT-NO-056`）：原样重投答已有结果并回显；换处置依据引用或换操作者原样重投仍答已有结果、回显原值；换结果或换指定的包裹答已处置；所处置的版本被第三件到站的冲突版接续答版本已换代；三者都不追加、不耗版本号、不重复意图。
- [x] 编排用例（`AT-NO-057`）：候选之外的包裹答不在候选之内；没有冲突的实物答无可处置的冲突。
- [x] 编排用例，多链（`AT-NO-060`）：两条链的实物处置其中一版，两条链各追加一版。
- [x] 读面用例（`AT-NO-059`）：处置之后贴同号的第三件到站，只与此刻关联到那件包裹的实物形成一码两物，被处置为「不是它」的实物不再被追加冲突版；`ParcelContainmentView` 不再把它算作那件包裹；还没处置过的实物两面读法照旧。
- [x] HTTP 与装配用例（`AT-NO-058`）：发行方未配置、令牌无效、无此决定种类的授予、不在准入范围各答其格；请求体带租户或操作者、坏载荷各种答 400；跨租户指名与查无此物同答。
- [x] 真库用例（cmd/parcel-api，经生产装配与演示种子同形的授予）：一码两物 → 处置 → 两件身份一格如上 → 意图一封入队 → 原样重投答已有结果、册上不多一行。
- [x] 并发：处置与同一包裹上又一件实物的收寄并发，经包裹锁串行，不出现一版两个后继。
- [x] admin-web 三道门退 0；待识别实物页的处置动作有组件用例。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nri06`；在 `claude-nrs03` tip `3d6d971c` 上写成，收口前 rebase 到 main `1167b4d9`，含 no-reception-source/03、04）

**提交**

| 提交 | 内容 |
|---|---|
| `47329657` | 实施与用例：决定种类、处置命令口、编排、处置文档列、处置截断、重核例外、复核目录按实物上列、管理台处置动作、演示种子 |
| `67dc9aab` | rebase 到 no-reception-source/04 之上的跟进：处置照收寄先取实物锁、后取包裹锁（用例钉次序）；同一实物同一节点改为并入之后，多链与「另牵连别的包裹」两条用例的第二次送站改在另一个节点；HTTP 用例的收寄库替身补答实物锁那一问 |
| `5995f78b` | 处置新加的两个读面（按实物读各行、按实物上列链头）不读并入行，真库用例钉住 |
| `7c0ea1bc` | `AT-NO-058` 点名：处置口经生产运营决定 Intake 的信封各格真库用例；HTTP 与编排用例补 Covers |
| `16795590` | 机制清点在 `7c0ea1bc` 的干净检出上重生成：node_operations 迁移 9→10、access_identity 7→8、全库 232→234，接入面端点 145→146 |
| `d967964d` | 只改注释：跨文件的计数与序号改成符号名或引文（按包裹读关联的两个读面写出名字，ADR-0210 越权风险点改引原句）；之后重跑 `internal/nodeoperations/...` 与管理台 `tsc` 照过，机制清点重生成零差 |
| 本笔 | 完成记录；operator-channel/15 补记末尾一行回写；AT 覆盖盘点 `AT-NO-053` 至 `AT-NO-060` 由②入① |

**落点**

- **领域**：新文件 `internal/nodeoperations/domain/identity_disposition.go`——结果两种 `IdentityDispositionResult`（线上名 `DESIGNATED`、`NONE_OF_CANDIDATES`）、连带实物 `CoDisposedUnit`、处置文档 `IdentityDisposition`（`FormIdentityDisposition` 立、`CanonicalizeIdentityDisposition` / `ParseIdentityDisposition` 写读 NID-1、`ExcludedFor(unit)` 给出这一版排除的那一组）、比对格摘要 `CanonicalizeIdentityDispositionComparison`（NOC-1，面 `DISPOSE_IDENTITY_CONFLICT`）。`directional_basis.go` 只加 `JudgmentChainHeads`，与身份一格同一个链头读法。
- **端口**：`ReceptionRecord.Disposition` 一格；新接口 `IdentityDispositionRegister`（`FindByJudgmentVersion`、`FindUnitIntakes`）；`FindOtherUnitIntakes` 端口注补处置截断一段；`catalogue_read.go` 加 `IdentityReviewUnitRow`、`IdentityReviewHead` 与 `ReviewCatalogueRead.ListIdentityReviewUnits`。
- **编排**：新文件 `application/dispose_identity_conflict.go`（`DisposeIdentityConflictHandler.Dispose`）。重核例外收在 `identity_judgment.go`：`identityJudge` 多一格 `excluded`，唯一命中被排除的那件时交回零候选的判断；`recheck_pending_identity.go` 只把被重核那一版的 `Disposition.ExcludedFor` 交进去。
- **收寄库**：`reception.go` 只在列清单、行扫描、写入各加一格，`FindOtherUnitIntakes` 的查询多一条截断谓词；新文件 `reception_disposition.go` 放 `FindUnitIntakes` 与截断谓词 `succeededByDisposition`；`parcel_containment_view.go` 两路各加同一条谓词；新文件 `review_catalogue_disposition.go` 放 `ListIdentityReviewUnits`。
- **HTTP**：新文件 `adapters/http/dispose_identity_conflict.go`（端点、`OperatorDecisionIntake.IntakeIdentityDisposition`、`UnconfiguredIntake` 的同名一问、答复形状）。`recheck_pending_identity.go` 的 `OperatorDecisionAuthenticator` 改交 `OperatorIdentity`（租户与 `<发行方>#<sub>`），重核只取其中的租户。`query_node_operations_records.go`：读口接口加一问，待识别册的答复另带 `units`。
- **装配**：新文件 `cmd/parcel-api/assemble_identity_disposition.go`（`buildIdentityDispositionOrchestration`、`buildIdentityDispositionDeps`、`identityDispositionWithin`、事务包装、未接线读口的那一问）。
- **管理台**：`apps/admin-web/src/pages/operations/` 下新文件 `identity-disposition-api.ts`、`identity-disposition.ts`（判读纯函数）与用例、`IdentityDispositionPanel.tsx`；`records-api.ts` 加类型；`NodeOperationsReviewPage.tsx` 在待识别实物区下方挂面板，头注与区注改写成「处置在候选里指定一件或都不是、不改正式包裹身份」。

**处置文档与记录形状**

| 格 | 处置版（每条链各一版，主实物与连带实物都是） |
|---|---|
| 键 | （租户，`IDENTITY-DISPOSITION/<前版的判断版本>`） |
| content_digest | 这次处置比对格的 NOC-1 摘要：结果、指定的包裹、连带实物的集合（只比实物）；同一次处置的各版同一个 |
| kind | 主实物指定一件：`INTAKE_FORMED`，关联 X，照 ADR-0186 取服务结果、记核对态（问一次，几条链同一个答复）；其余（都不是、连带实物）：`PENDING_IDENTIFICATION`，不带关联、不带候选、不记冲突 |
| judgment_version / predecessor_version | 新签一版 / 那条链此刻的当前有效版本 |
| intake、control | 收寄事实与控制照前版，业务时间是原接收时刻，不另起控制起点 |
| source_document | 这条链起点那一版的；起点没有就不带 |
| disposition_document | NID-1：`unit`（主实物）、`disposed_version`、`result`、`parcel`、`excluded`（都不是时是处置候选）、`co_disposed`（实物与版本）、`basis`（去两端空白）、`operator`（`OPERATOR/<发行方>#<sub>`）、`disposed_at`（服务端时刻）。不带 omitempty。同一次处置的各版逐字同一份；哪一版排除了什么由 `ExcludedFor` 按这一版的实物给出：都不是排除处置候选，指定一件时连带实物只排除 X、主实物不排除 |

**迁移**

- `migrations/node_operations/0010_reception_identity_disposition.sql`：加列 `disposition_document jsonb`；约束 `reception_disposition_shaped` 守「是对象、只落在形成与待识别两格、必指名前版、不记身份冲突」。既有行留空：本迁移之前没有任何路径写处置版，空即「不是处置版」。不动种类封闭集，与 0009 的 `MERGED_INTO_EXISTING_INTAKE` 相容。
- `migrations/access_identity/0011_identity_conflict_disposition_decision.sql`：重建 `operator_grant_decision_kind_decided`，全集取本基线已有的（到 `PENDING_IDENTITY_RECHECK`）加 `IDENTITY_CONFLICT_DISPOSITION`。**给推送方**：另一会话的 0008、0009 各加一种（`TRANSPORT_COMMISSION_SUBMISSION`、`TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`），合并时这份 0011 的全集要补上那两种，否则 0011 施加之后那两种授予登不进。

**命令口与答复**

`POST /node-operations/identity-dispositions`，挂操作者渠道「运营决定」能力面，决定种类 `IDENTITY_CONFLICT_DISPOSITION`；端点表与 `main.go` 只追加行，隔离开关换不了这一行。载荷封闭：`unit`、`disposedVersion`、`result`、`parcel`（只在 `DESIGNATED`）、`coDisposedUnits[{unit, version}]`（只在 `DESIGNATED`）、`basis`。

| 格 | 状态码 | outcome / code | 回显 |
|---|---|---|---|
| 身份冲突已处置 | 201 | `IDENTITY_CONFLICT_DISPOSED` | `disposition`（这一次）、`versions`（追加的各版：实物、判断版本、前版、种类、关联或 `excluded`、核对态）、`intentVersion`（交了意图的那一版，没交不出现） |
| 已有结果 | 200 | `EXISTING_RESULT` | `disposition`（在册那一次，依据与操作者是原值）、`versions`（接续所处置版本的那一版） |
| 已处置 | 200 | `ALREADY_DISPOSED` | 同上 |
| 版本已换代 | 200 | `VERSION_SUPERSEDED` | `currentVersions`（主实物此刻各链的当前有效版本） |
| 无可处置的冲突 | 200 | `NO_IDENTITY_CONFLICT` | `currentVersions` |
| 不在候选之内 | 200 | `NOT_A_CANDIDATE` | `candidates`（处置候选） |
| 关联对象另有归属 | 200 | `ASSOCIATED_ELSEWHERE` | `associatedUnits[{unit, currentVersions}]`（此刻关联到 X 的实物与它们各链的当前有效版本） |
| 指名的实物或版本查不到、跨租户、实物与版本对不上 | 404 | `JUDGMENT_NOT_VISIBLE` | 无 outcome |
| 坏载荷 | 400 | `MALFORMED_REQUEST` | 缺格、空白、带 NUL、未知字段（含 `tenantId`、`operator`，连带实物里的也算）、甲不带 X、乙带 X 或连带实物、连带实物重复或就是主实物或缺版本、结果不在词表里、尾随内容 |
| 信封各格 | 403 / 401 / 403 / 403 / 503 | `ACCESS_CHANNEL_NOT_CONFIGURED` / `OPERATOR_CREDENTIAL_REJECTED` / `OPERATOR_NOT_GRANTED` / `OUTSIDE_ADMISSION_SCOPE` / `IDENTITY_DEPENDENCY_UNAVAILABLE` | 先认证、后解载荷 |
| 没形成答案 | 500 | `NO_ANSWER_FORMED` | 落册或意图入队失败、撞上（租户，前版）唯一；整笔回滚 |

**编排的次序与几处代决**

1. 验命令 → 按所处置的版本读出那一行（不在本租户、不是带收寄的两格、实物对不上，同答查不到）→ 按派生键认重放（比对格同答已有结果，异答已处置）。都在取锁与签版本之前。
2. **先实物后包裹**：主实物与所列连带实物按标识排序取实物锁（`ReceptionStore.FindCurrentJudgmentsAtNode` 那一把，按租户与实物、不按节点），再按关联排序取包裹锁（`FindOtherUnitIntakes` 那一把）。包裹锁的范围是 X 加主实物与所列连带实物**在册上带过的每一件关联**（不只链头）：被这次处置截断的关联都在里面，按包裹读关联的两面读不到半截处置。取完包裹锁再读链头；锁外认重放之后才提交的同版本处置在这里再认一次重放。重核不取实物锁，它在两次读之间落下的关联补锁，漏锁的一格由（租户，前版）唯一兜底。
3. 链头与「合起来判冲突」都走领域那一份（`JudgmentChainHeads`、`JudgeIdentityBasis` 的身份冲突原因），处置候选是各链头的候选与关联合起来。
4. **连带实物的版本**：所带版本须是那件实物此刻某条链的链头（逐链追加会写到它的每条链，不限定带 X 关联的那一条）；它的处置候选须恰好只有 X。
5. 追加次序：主实物各链按接收时刻、来源标识排，连带实物同。第一版就撞上派生键（同一处置的另一次请求在锁外认重放之后先提交了，锁护不住没有关联可锁的「都不是」）时读回它答重放；别的撞键与前版撞唯一一律不形成答案。
6. 意图：主实物此前哪一行`节点收寄已形成`曾关联到 X 就不交；多条链只交一封，落在接收最早的那一版。交不出去整笔回滚（处置口没有人读续办引用）。
7. **处置截断**取「沿后继走」：一行之后的链上有一版带处置文档即算被接续过（`succeededByDisposition`，每一步走 `reception_predecessor_unique`，步数封顶）。不在处置版上抄它截断的各版：链只追加，处置版一落，前面每一版沿后继都走得到它。
8. **重核例外**照零候选办，答 `STILL_UNIDENTIFIED`，不另立一格；连带实物那一版同样适用（它排除 X）。
9. **复核目录**：不另开册名，待识别册那一问另带 `units`（按实物，limit 数实物）：有过待识别那一格的实物，每件交回各链链头（形成格也在），处置版带处置文档；并入行不在其中。
10. **运营决定认证方**改交租户与操作者（照 PS 运营决定认证方的写法），处置口要把操作者写进文档；重核口行为不变。
11. no-reception-source/05 还没进 main：「都不是」那一版不写身份核对态，留给它的迁移回填与实施（给 05 的提醒：处置版里不带关联的那几版也是「已核对」）。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 编排·一码两物（`AT-NO-053`） | `internal/nodeoperations/application/dispose_identity_conflict_test.go::TestDesignatingTheLateUnitDisposesBothUnitsOfOneMark`、`::TestDesignatingTheEarlyUnitNeedsNoCoDispositionAndHandsNoSecondIntent` |
| 编排·都不是（`AT-NO-054`） | 同文件 `TestConfirmingNoneOfTheCandidatesKeepsTheUnitPendingAndRecheckSkipsTheExcluded`（指名重核与门铃按号重核都不追加；号码另有新关联照常追加） |
| 编排·关联对象另有归属（`AT-NO-055`） | 同文件 `TestADesignationMustCoDisposeExactlyTheUnitsAssociatedNow`（漏列、多列、版本过时、另牵连别的包裹各一条子用例） |
| 编排·代数（`AT-NO-056`） | 同文件 `TestReplayingADispositionAnswersTheRecordedOne`、`TestDisposingASupersededVersionAnswersVersionSuperseded` |
| 编排（`AT-NO-057`） | 同文件 `TestRejectedDispositionsLandNothing`（另含跨租户、实物与版本对不上、查无此版同答，依据空白拒） |
| 编排·多链（`AT-NO-060`） | 同文件 `TestADispositionAppendsOnEveryChainOfTheUnit`（都不是与指定一件各一遍；意图落在接收最早那一版） |
| 读面（`AT-NO-059`） | 编排面 `TestAThirdArrivalAfterADispositionConflictsOnlyWithTheUnitAssociatedNow`；存储面 `internal/nodeoperations/adapters/postgres/reception_disposition_test.go::TestOtherUnitIntakesStopAtADisposition`、`::TestParcelContainmentStopsAtADisposition`（两条都先证还没处置时读法照旧） |
| HTTP 与装配（`AT-NO-058`） | `internal/nodeoperations/adapters/http/dispose_identity_conflict_test.go::TestTheDispositionEndpointMapsIntakeRefusals`、`::TestTheDispositionIntakeTakesTenantAndOperatorFromTheEnvelope`、`::TestMalformedDispositionPayloadsAreRefused`、`::TestDispositionRefusalsAreFormedAnswers`、`::TestADispositionAnswersCreatedThenTheRecordedOne`；装配 `cmd/parcel-api/identity_disposition_test.go::TestTheWiredDispositionAnswersTheChannelGrades`，以及 `assemble_operator_decisions_test.go` 的换口表加的那一行（`TestSwappedDecisionFacesRefuseAGrantOfAnotherDecisionKind`、`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 随之覆盖本口）、`endpoints_test.go` 的探针表一行 |
| 真库 | `cmd/parcel-api/identity_disposition_test.go::TestTheWiredDispositionDisposesOneMarkOnTwoUnits`（授予、准入对照与权威区间照演示种子的登法；两件身份一格、意图多一封、原样重投答已有结果且册上不多一行不多一封、查无此版 404） |
| 并发 | 同文件 `TestADispositionAndAnotherUnitsReceptionSerializeOnTheParcel`：处置取完包裹锁后挂住，C 贴同号送站等在包裹锁上（观察连接看到 advisory 等待），处置提交后 C 只与 B 成一码两物，A 的链头不动，全表没有一版两个后继。取锁次序另由编排用例 `TestADispositionHoldsTheUnitsBeforeTheParcel` 钉住 |
| 存储与读面其余 | `reception_disposition_test.go::TestADispositionVersionKeepsItsDocumentAndTheUnitReadsEveryRow`（往返、按实物读、库面拒没前版的处置版）、`::TestIdentityReviewUnitsListEveryChainHeadWithItsDisposition`、`::TestABrokenDispositionDocumentIsLoud`、`::TestTheDispositionReadsLeaveMergeRecordsOut`；领域 `internal/nodeoperations/domain/identity_disposition_test.go`；授予册面 `internal/accessidentity/adapters/postgres/operator_registry_disposition_decision_test.go`；认证方 `internal/nodeoperations/adapters/accessidentity/operator_decision_authenticator_test.go::TestADispositionOperatorAuthenticatesForTheIdentityConflictDispositionDecision` |
| 管理台 | `apps/admin-web/src/pages/operations/identity-disposition.test.ts`（冲突与处置候选的判读、按钮可否与不可的原因、已处置的行写出处置了什么、载荷拼法、`ASSOCIATED_ELSEWHERE` 回显带出连带实物与「另牵连别的包裹」时不给一并提交、各格答复的呈现、问题码）。本仓管理台没有 React 渲染用例的设施（`tsconfig.test.json` 只收 `*.test.ts`），组件用例照 identity-recheck 的先例落在判读纯函数上 |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri06-red.log`。钉 `3d6d971c`，只写了用例、生产代码未动。Go 那一段是编译红，现象原话举例：`undefined: domain.IdentityDisposition`、`record.Disposition undefined (type ports.ReceptionRecord has no field or method Disposition)`、`undefined: application.DisposeIdentityConflictHandler`、`undefined: nodeopshttp.OperatorIdentity`、`undefined: ports.IdentityReviewUnitRow`、`undefined: buildIdentityDispositionOrchestration`、`too many arguments in call to assembleBusinessEndpoints`；运行红一条：`decision kind IDENTITY_CONFLICT_DISPOSITION of ADR-0151 decision one: ("", access identity: decision kind is unknown)`。管理台一段：`error TS2307: Cannot find module './identity-disposition' or its corresponding type declarations.`
- 绿：rebase 之前 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri06-green.log`（钉 `5995f78b`），收口 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri06-green-final.log`（钉 `16795590`，代码与 `7c0ea1bc` 相同，只多机制清点一笔）；管理台三道门 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri06-adminweb.log`。

**门**（Go 钉 `16795590`、管理台钉 `7c0ea1bc`，两者代码相同；都在 rebase 到 main `1167b4d9` 之后）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./internal/accessidentity/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-access-register/... ./cmd/parcel-governance-register/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/...` 全 ok。管理台在 Node 22 下 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（561 条全过）、`vite build` 都退 0。没有跑全量 `./...`。机制清点在 `7c0ea1bc` 的干净检出（临时 worktree）上重生成，与工作树上生成的逐字相同。

**演示种子实灌核对**：在 55432 上建临时库 `nri06_seedcheck`，`scripts/demo-seeds/migrate` 施加全部迁移（235 步，含 node_operations 0010 与 access_identity 0011），用两个登记 CLI 登 governance 23、27 与 access 01、02、03：新授予 `SYN-GRANT-DEX-CONFIG-IDENTITY-CONFLICT-DISPOSITION-01`、新对照 `SYN-MAP-OD-IDENTITY-CONFLICT-DISPOSITION-01` 答 RECORDED，新区间答 REGISTERED，全程退 0；核对完删库。日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nri06-seedcheck.log`。演示库 psb26_walk 没碰。

**证据层级**：`S`。夹具与种子全为 `SYN-` 合成值；生产装配不配任何授予或准入，谁持有「身份冲突处置」授予仍是租户取值（`PAR-INT-08`、`PAR-INT-11`），如实答无授予或不在准入范围。

**共享文件里加的行**

- `cmd/parcel-api/endpoints.go`：`assembleBusinessEndpoints` 参数表末尾加 `identityDisposition nodeopshttp.IdentityDispositionHandler`；端点表在重核那一行之后加一行（带两行注释）`{Pattern: "/node-operations/identity-dispositions", Handler: nodeopshttp.NewDisposeIdentityConflictEndpoint(operatorDecisions.nodeOperations, identityDisposition)}`。
- `cmd/parcel-api/main.go`：重核编排之后加 `identityDisposition, err := buildIdentityDispositionOrchestration(db)` 一段（带一行注释），`assembleBusinessEndpoints(...)` 实参末尾加 `identityDisposition,`。
- `cmd/parcel-api/endpoints_test.go`：探针表加 `"/node-operations/identity-dispositions"` 一行；`assembleUnwiredBusinessEndpointsWithOperatorIntakes` 实参末尾加 `unwiredIdentityDisposition{},`。
- `cmd/parcel-api/assemble_operator_decisions_test.go`：`swappedDecisionFaces` 末尾加 `"/node-operations/identity-dispositions": accessidentity.DecisionIdentityConflictDisposition`（带一行注释）。
- `internal/accessidentity/operation_decision.go`：常量 `DecisionIdentityConflictDisposition DecisionKind = "IDENTITY_CONFLICT_DISPOSITION"`、`decisionKinds` 末尾一项、类型头注加一句；对应用例 `operation_decision_test.go` 的词表加一项。
- `ports.go`、`reception.go`、`parcel_containment_view.go`、`review_catalogue.go` 一侧：只加格、加方法与新接口；既有函数里的改动是 `receptionColumns`、`scanReception`、`Save` 各加一格，`FindOtherUnitIntakes` 与 `LoadParcelContainment` 的查询各加一条截断谓词，`review_catalogue.go` 没动（按实物上列放在新文件）。`intake_identity_view.go` 没动。

**越出地盘的改动**

1. `internal/nodeoperations/adapters/accessidentity/operator_decision_authenticator.go` 与用例：认证方改交租户与操作者（处置文档要记操作者）。这是 no-reception-source/03 落下的文件，地盘没列。
2. `internal/nodeoperations/adapters/http/recheck_pending_identity.go` 与用例：`OperatorDecisionAuthenticator` 的签名随上一条改；重核口行为不变。
3. `migrations/access_identity/0011_…`（号由派活定）与新用例 `internal/accessidentity/adapters/postgres/operator_registry_disposition_decision_test.go`：地盘只列了 `operation_decision.go` 及其测试。
4. `scripts/demo-seeds/seed.sh` 一行（灌 governance/27）与 `scripts/demo-seeds/README.md` 授予表那一格加「身份冲突处置」：地盘只列了两份 access 种子与 governance 目录。
5. HTTP 用例里给 `stubReviewCatalogue`、`unreachableReviewCatalogue`、`recheckStore` 补方法，放在本票新文件 `dispose_identity_conflict_test.go`，原文件没动；`cmd/parcel-api` 里 `unwiredNodeOperationsRecords` 的那一问放在新文件 `assemble_identity_disposition.go`，`unwired_orchestration.go` 没动。

**没做**

- 「走 Go 那条路」一格：非作者评审与推送方在干净检出上带 DSN 跑全量都还没做，格未勾。
- ADR-0210 与 NO CONTEXT 没动：线上名与答复格写在本记录（ADR-0210 决定六「线上名由实施票定」）。若要像 ADR-0199 那样在 ADR 末尾追加复核记录留痕，由推送方定。
- 「都不是」那一版的身份核对态（no-reception-source/05 未进 main），见上文「编排的次序与几处代决」末项。
- 票面「不做」各项照旧：自动解除、未冲突关联的单独更正口、「同一实物相互冲突的外部标识」、PS 采用回滚与 VE 协调、向 party-commercial 问授权、身份一格读链头（票 05 已做）。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `a1faeff8`）

两轴合审，0 阻断。

- **Spec**：ADR-0210 落到。处置走运营决定新种类 `IDENTITY_CONFLICT_DISPOSITION`，复用 no-reception-source/03 的运营决定认证方（改为多交操作者）；每条链各追加一版处置版，指名链头、派生键 `IDENTITY-DISPOSITION/<前版>`，处置文档 NID-1 同一次处置各版逐字同一份；指定一件时连带实物同一次处置、排除 X；「都不是」排除处置候选，重核遇被排除的包裹不关联回；处置截断照「沿后继走」落在按包裹读关联与包裹容纳两处；命令口的重放、已处置、版本已换代与三种业务拒绝各格齐，对照格只比结果、X 与连带实物集合。取锁先实物后包裹，实物与包裹各按标识排序取，并发用例实测等在包裹锁上。管理台「处置」动作三道门过；种子在临时库实灌核对过。
- **Standards**：新读面与写面另起文件，共享文件只追加；对既有函数的改动收在 `receptionColumns`、`scanReception`、`Save` 各加一格与两条截断谓词。
- **评审修补（已做，`0c85a204`）**：access_identity 迁移 0011 重建决定种类 CHECK 时，全集预先列上另一组票 0008、0009 要加的两种——那两份编号在前却会更晚进 main，新库按编号施加时 0011 在后，只取本基线全集会把它们删掉；头注写明。ADR-0210 文末追加复核记录记落成的线上名，与 ADR-0199 同一处置。
- **留给 no-reception-source/05**：处置版里不带关联的那几版也是「已核对」，05 回填与写入核对态时照此（作者已在完成记录留提醒）。
