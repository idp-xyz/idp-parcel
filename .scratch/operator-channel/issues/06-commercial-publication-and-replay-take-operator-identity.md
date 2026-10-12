# 06 商业发布批准链与计价回放端点接上操作者身份

Category: enhancement
Status: 完工，待评审与重放——2026-10-12 通道 2，代码 tip `095aae46`（分支 `mcp2-oc06`、基 `ff3b01fa`），完成记录在文末（之一、之二）。之一（代码 tip `d678d1a4`）留下的待裁，通道 1 于 20:1x 裁定：`/commercial-publications` 不换、另立 operator-channel/18（判断项 1）；`/pricing-estimates` 归本票，已在之二做完。此前同日完工于代码 tip `d678d1a4`，`/commercial-publications` 与 `/pricing-estimates` 待派单方裁。此前 in-progress——2026-10-12 通道 2 认领（通道 1 派单 `task-e8e757ff`），分支 `mcp2-oc06`、基 `ff3b01fa`。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 04（2026-10-12 通道 1 派单时裁定不挡本票：04 余下的是商业参与方身份族那几口，各走各自的隔离 Intake 变量，本票各口不经它；通道 2 开工时在 `ff3b01fa` 的 `endpoints.go` 上复核成立）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 里商业发布（`/commercial-publications`）与计价回放（`/pricing-evaluation-replays`）两处的 Intake 装配，以及把 `OperatorEnvelope` 译成 party-commercial「操作者主体引用 + 授予集」消费面的那一段。2026-10-12 20:1x 通道 1 派单扩入（之二）：`cmd/parcel-api` 里 `/pricing-estimates` 那一行与它的装配测试，`internal/parcelpricing/adapters` 的 accessidentity 与 http 里查阅读面的认证。
出处：[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定五「Intake 到位那天把信封译成它」；[ADR-0124](../../../docs/adr/0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md) 决定六「本端点的真操作者 Intake 随 ADR-0100 那一族在装配点逐端点换」。

## 做什么

1. 商业发布：录入者与批准者两身份都从 `OperatorEnvelope` 来；审批职责规则未登记时批准门照旧答未配置。
2. 计价回放端点换操作者 Intake。
3. 发布表单的词表读口 `/commercial-publication-vocabularies` 一并换操作者 Intake（2026-09-24 随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定五补）：它不要租户、不读存储读面，唯一的消费者是发布四口喂的表单。

## 不做

- 审批职责规则的取值与登记入口（租户取值与另一张票）；角色模型里有哪些授予格归 [psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)，本票只接信封，不阻于它（ADR-0126 已把规则形态定成两格）。

## 完成判据

- 发布待批准载体上录入者与批准者都来自信封（带测试）；回放端点三格各答其格。

## 完成记录之一（2026-10-12 通道 2）

分支 `mcp2-oc06`，基 `ff3b01fa`，代码 tip `d678d1a4`。证据层级一律 `S`（隔离合成，演示租户 `SYN-TENANT-01`）。

各笔：

- `5f45d17f` 票面认领。
- `ffb54dc9` party-commercial：认证适配器 `AuthenticateRegistryWrite` 交回 `OperatorIdentity`——租户，加操作者主体：引用取（发行方、sub）这一对，授予集是信封持有的能力面逐格译成的等级。原有各口只取租户，答复不变。`OperatorRegistryIntake` 实现预览、录入、批准、载体发布四口与词表读口，各在该口自己的文件里；词表读口的 Intake 失败改走命令面分流 `writeRegistrationIntakeProblem`——目录查阅那份没有操作者渠道的三格，换上之后会落成 500。
- `c41c79de` parcel-pricing：`OperatorRegistryIntake.IntakeEvaluationReplay`；`EvaluationReplayIntake` 与 `UnconfiguredIntake` 两处注释改指真译法。
- `d678d1a4` parcel-api：`/commercial-publication-previews`、`-drafts`、`-draft-approvals`、`-draft-publications`、`-vocabularies` 挂 `operatorRegistries.commercial`，`/pricing-evaluation-replays` 挂 `operatorRegistries.pricing`；`/commercial-publications` 仍挂字面量；装配测试。
- `0b4f34c5` 机制清点重生成（只有测试文件计数变）。
- 本笔：完成记录与 Status。

### 完成判据逐条

- ✅ 发布待批准载体上录入者与批准者都来自信封（带测试）。
  - `commercialhttp` `TestPublicationDraftPathTakesSubmitterAndApproverFromTheEnvelope`：内存载体上录入之后，规则没登答 `NOT_CONFIGURED`，录入者自批答 `NEEDS_ANOTHER_APPROVER`，换了人却不持那一级答 `APPROVER_NOT_QUALIFIED`，持级的另一个人 `DRAFT_APPROVED`；读回的录入者与批准者就是两枚令牌认证出的主体。
  - `commercialhttp` `TestPublicationOperatorIntakeTakesIdentityOnlyFromTheEnvelope`：四口译出的命令里租户、录入者、批准者（引用 + 授予集）都是认证出的那一份；载荷里带 `tenantId`、`submitter`、`approver`、`levels` 一律答请求畸形。
  - `cmd/parcel-api` `TestTheOperatorChannelRecordsEnvelopeSubjectsOnThePublicationCarrier`（真库）：真铸造 → 真 PC 认证适配器 → 操作者 Intake → 真库载体编排。规则没登时批准答未配置（做什么里「审批职责规则未登记时批准门照旧答未配置」）；登了「录入者与批准者不同」之后录入那枚令牌自批答需换人，另一枚批得下来；库里读回的两个身份就是两枚令牌的主体。
- ✅ 回放端点三格各答其格：`/pricing-evaluation-replays` 并入 `swappedRegistryFaces`——未带令牌 401 `OPERATOR_CREDENTIAL_REJECTED`，只有查阅授予 403 `OPERATOR_NOT_GRANTED`，操作者册读不动 503 `IDENTITY_DEPENDENCY_UNAVAILABLE`，授予齐备而载荷自报租户 400。`pricinghttp` `TestReplayIntakeTakesTheTenantFromTheAuthenticatedOperator` 钉租户取自信封；载荷带租户键、触发者键或证据层级缺席都答畸形（换了渠道也不补一个 `S`）；三格下请求体一个字节都不读。
- ✅ 词表读口一并换操作者 Intake：`commercialhttp` `TestVocabularyEndpointAnswersTheOperatorChannelGrades` 与 `cmd/parcel-api` `TestThePublicationVocabularyAnswersFromTheOperatorChannel`——认证过了照常列词（200），401 / 403 / 503 各答其格，发行方未配置仍是 403 未配置。
- ◑ 地盘里的 `/commercial-publications` 没换，见判断项 1，由另立的 operator-channel/18 接；`TestCommercialPublicationsStaysUnconfiguredOnTheOperatorChannel` 钉住操作者渠道配好、授予齐备时它仍答 403 未配置。

### 判断项（越权风险点 · 待 owner 复核）

1. **`/commercial-publications` 不换。** 它收受控批文的在线镜像（声明摘要 + 批准三格 + 角色确认），批准从批文取，与「批准者从信封来」不是一回事——操作者的批准路径是 ADR-0126 决定三的载体，本票已换。照 operator-channel/04 第三、五批「译装先下沉成 `registrationjson`、在线口不另写平行解码」的先例，换它要先把批文译装从 `cmd/parcel-commercial/translate.go`（`package main`）整份下沉，在派单卡地盘之外。开工时已报派单方，待裁：另立下沉票，或扩本票地盘。通道 1 20:1x 裁：另立 18。
2. **授予集的译法是暂定的。** 能力面当商业权限等级，与 parcel-pricing 批准门那一侧同一个（票 price-card-import/04）：租户自定的等级名经这里永远对不上，批准门恒答不合格；规则要求的恰是登记册配置写那一格时，到得了批准门的人都持有它，等于不要求。角色模型里有哪些授予格归 [psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)。
3. **回放口挂登记册配置写那一面。** ADR-0124 决定六让它「与其余写面同一批」换。ADR-0100 预留的治理登记面管的是没有租户维的试点治理登记册（ADR-0085 决定四），回放按租户隔离，不归那一面。
4. **回放命令不记触发者。** 触发者过了认证，但回放命令没有那一格；争议复核由谁发起、凭什么归另裁（ADR-0124 决定六），本票不在应用与领域里立格。
5. **发布路径四口与词表读口的授予照登记册配置写那一格认**，与 parcel-pricing 价卡草稿链同；词表读口随发布一族的依据是 ADR-0151 决定五。

### 未做

- **`/pricing-estimates`。** ADR-0152 决定六要它的真 Intake「与计价回放同批（operator-channel/06 的地盘）」换、能力面取主数据与运营查阅读；operator-workspace-gaps/05 的「不做」与 `EstimateIntake` 注释同句。派单卡的七口照 04 归类表「`/pricing-estimates` 归 operator-workspace-gaps」列，没有它。换它要先立全仓第一只「查阅读」面的操作者认证方（ppaccess 与 pricinghttp 各一型），超出卡面「parcel-pricing 回放口」的地盘，所以未做；已报派单方，待裁。回放已换而试算未换，ADR-0152 决定六的「同批」此刻不成立；`endpoints.go` 试算行注释「真 Intake 与回放同批换」照 ADR 原意未改。**通道 1 20:1x 裁归本票，已做，见完成记录之二。**

### 验证（作者那层，钉 `d678d1a4`，隔离工作树）

- `go build ./...` 退 0；`go vet ./...` 退 0；改动的 `.go` 文件 `gofmt -l` 零行，无 CR、无 BOM。
- `go test -count=1 -p 1`，`IDP_PARCEL_POSTGRES_DSN` 已设（含 PG）：动过的 `internal/partycommercial/adapters/accessidentity`、`internal/partycommercial/adapters/http`、`internal/parcelpricing/adapters/http`、`cmd/parcel-api`，反向依赖 `internal/parcelpricing/adapters/accessidentity`，外加 `./internal/architecture/...`，全部 ok，退 0。
- `-v` 单跑：`TestTheOperatorChannelRecordsEnvelopeSubjectsOnThePublicationCarrier` 与 `TestTheWiredEvaluationReplayRecordsAgainstARealDatabase` 均 PASS，不是 SKIP。
- 判别力（临时变异，跑完即 `git checkout` 还原，未提交）：
  - 词表读口退回目录查阅分流 → `TestVocabularyEndpointAnswersTheOperatorChannelGrades`、`TestThePublicationVocabularyAnswersFromTheOperatorChannel` 红；
  - 认证适配器不译授予集 → `TestTheOperatorSubjectCarriesTheFacesTheEnvelopeHolds` 与 `TestTheOperatorChannelRecordsEnvelopeSubjectsOnThePublicationCarrier` 红；
  - 预览口把逐格问题经 `malformedUnless` 归成散文 → `TestPreviewEndpointAnswersThroughTheOperatorChannel` 红；
  - 认证前先读一字节请求体 → `TestPublicationOperatorIntakeAuthenticatesBeforeReadingTheBody` 红；
  - 回放租户换成写死的 `SYN-TENANT-02`（变体能编译）→ `TestReplayIntakeTakesTheTenantFromTheAuthenticatedOperator` 红；
  - 回放行退回字面量 → `TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel` 红。
- 机制清点在 `d678d1a4` 的干净检出上重生成（`0b4f34c5`）。
- 作者不自评：非作者评审由派单方另派，重放进 main 由派单方做。

## 完成记录之二：`/pricing-estimates`（2026-10-12 通道 2）

通道 1 20:1x 派单（`task-a077ef33`）裁 `/pricing-estimates` 归本票：ADR-0152 决定六要它的真 Intake「与计价回放同批（operator-channel/06 的地盘）」、能力面取「主数据与运营查阅读」、不加隔离放行；ADR 优先于派单卡（AGENTS.md「冲突时」）。分支 `mcp2-oc06` 上接着 `f20c7d2e` 另起，未 rebase；下列各笔可整组抽掉，抽掉后回到之一的 tip。证据层级一律 `S`（隔离合成）。

各笔：

- `7313ac21` parcel-pricing：`adapters/accessidentity` 立 `OperatorOperationsReadAuthenticator`——以 `CapabilityMasterDataAndOperationsRead` 铸信封、不带准入要求，交回绑定的租户，各格经既有 `answer` 译。`adapters/http` 立 `OperatorOperationsReadAuthenticator` 接口与 `OperatorOperationsReadIntake`，`IntakeEstimate` 先认证、后解载荷，租户取认证出的身份。`EstimateIntake` 与 `UnconfiguredIntake.IntakeEstimate` 的注释改指真译法；三格答复与 `ErrOperatorNotGranted` 的注释扩到查阅读面（错误串随之改，线上码不变）。回放测试里的请求体探针改名 `bodyReadProbe`，供试算测试共用。
- `095aae46` parcel-api：`operatorRegistryIntakes` 多一格 `pricingOperationsRead`，用同一只铸造器装起来；`/pricing-estimates` 挂它，不挂登记写面的 `operatorRegistries.pricing`，该行注释改指真译法；新增装配测试 `assemble_pricing_estimate_test.go`。端点表签名与 `main.go` 不动。
- `0d45f3d2` 机制清点重生成（干净检出钉 `095aae46`）。
- 本笔：完成记录之二与 Status。

没碰 `internal/accessidentity`：铸造器按请求里的能力面核授予，查阅读那一格它本来就认，导出面不动。

### 判据逐条

- ✅ 真 Intake 换上、能力面取主数据与运营查阅读（ADR-0152 决定六）：`/pricing-estimates` 挂 `operatorRegistries.pricingOperationsRead`。`OperatorRegistryIntake` 不实现 `EstimateIntake`，这一行误挂到登记写面编译不过。
- ✅ 与计价回放同批：回放（之一 `c41c79de`、`d678d1a4`）与试算同在本分支。
- ✅ 不加隔离放行（ADR-0150）：`IsolatedOperationsReadIntake` 只实现 `PricingCatalogueIntake`，`isolatedWriteAdmittedCommandLines` 里没有这一行，本组两处都没动。
- ✅ 三格同回放，发行方未配置不回归：`cmd/parcel-api` `TestThePricingEstimateAnswersFromTheOperatorReadChannel`——未带令牌 401 `OPERATOR_CREDENTIAL_REJECTED`，只持登记册配置写 403 `OPERATOR_NOT_GRANTED`，操作者册读不动 503 `IDENTITY_DEPENDENCY_UNAVAILABLE`，持查阅读而载荷自报租户 400 `MALFORMED_REQUEST`，声明立得住时越过 Intake 到编排（未接线占位答 `NO_ANSWER_FORMED`）；发行方未配置时带不带令牌都答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，`TestEveryAssembledEndpointAnswersUnconfigured` 照旧绿。
- ✅ 租户取认证出的身份：`pricinghttp` `TestEstimateIntakeTakesTheTenantFromTheAuthenticatedOperator`——载荷带 `tenant` 或 `tenantId` 答畸形；令牌不过、未授予、身份依赖读不动、发行方未配置下请求体一个字节都不读；认证交回空租户答 `ErrOperatorIdentityMissing`，不折成坏报文。`cmd/parcel-api` `TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase`（真库）：同一份声明，绑夹具价卡所在租户的操作者得到那张卡的一份 `S` 试算评价（`FORMED`、`COMPLETED`、合计 10 USD），绑另一个租户的答 `PRICE_CARD_NOT_CONFIGURED`。
- ✅ 查阅读认证方的各格：`ppaccess` `TestOperationsReaderAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck`（准入范围取未配置那一只也认证得过——这一面不判准入）与 `TestOperationsReadGradesTranslateIntoPricingGrades`（发行方未设、令牌缺失、只持登记册配置写、公钥集读不动、操作者册读不动各答其格）。
- ✅ 该行与 `EstimateIntake` 的注释改指真译法，`UnconfiguredIntake.IntakeEstimate` 一并改。
- ✅ 判断项 1 末尾补了裁定句；之一判据里 `/commercial-publications` 那一格保持 ◑、指向 18。

### 判断项（越权风险点 · 待 owner 复核）

6. **试算这个 `POST` 挂在「主数据与运营查阅读」授予上（ADR-0152 越权风险点 3）。** 它与 ADR-0100 授予模型按读写分面的字面分类有张力，备选是新增一个「试算」能力面。本票照 ADR-0152 决定六走，不另立能力面。该 ADR 的 owner 复核记录写明那是受托复核（通道 3 代为复核并维持），不是 owner 本人认可，所以这一格仍待 owner。落到代码上的后果：只持登记册配置写、不持查阅读的操作者在试算口答 403 未授予；只持查阅读的操作者能试算、不能登记。演示种子里的 `SYN-OPERATOR-CONFIG-01` 与 `SYN-OPERATOR-VIEWER-01` 都持查阅读（`scripts/demo-seeds/data/access/02-grants.json`，取证于 `0d45f3d2`），发行方配好之后两人都走得到试算。
7. **查阅读 Intake 搭在 `operatorRegistryIntakes` 里，没有另立一个结构、给 `assembleBusinessEndpoints` 多穿一个参数。** 它是全仓第一只查阅读面的操作者 Intake（取证于 `095aae46`）；多穿参数要连带改端点表签名、`main.go` 与各测试辅助函数，超出卡面「那一行与它的装配测试」的地盘。operator-channel/05 换目录查阅面、查阅读 Intake 成族时，这一格随之迁出。
8. **查阅读认证方只交回租户，不交操作者主体。** 试算命令没有发起者那一格，谁发起了这次试算不进命令（同判断项 4 回放不记触发者）；ADR-0152 决定二放弃留痕，本票不在应用与领域里立格。

### 验证（作者那层，钉 `095aae46`，隔离工作树）

- `go build ./...` 退 0；`go vet ./...` 退 0；改动与新增的 `.go` 文件 `gofmt -l` 零行，无 CR、无 BOM。
- `go test -count=1 -p 1`，`IDP_PARCEL_POSTGRES_DSN` 已设（含 PG）：动过的 `internal/parcelpricing/adapters/http`、`internal/parcelpricing/adapters/accessidentity`、`cmd/parcel-api`，外加 `./internal/architecture/...`，全部 ok，退 0。`go list` 按生产导入与按测试导入反查，都不出上列各包。`cmd/parcel-api` 另以 `-v` 整包跑一遍：零 `--- SKIP`、零 `--- FAIL`。
- `-v` 单跑：`TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase` PASS，不是 SKIP。
- 判别力（临时变异，跑完即 `git checkout` 还原，未提交）：
  - 认证方改以登记册配置写铸信封 → `TestOperationsReaderAuthenticatesToTheBoundTenantWithoutAnAdmissionCheck`、`TestOperationsReadGradesTranslateIntoPricingGrades`、`TestThePricingEstimateAnswersFromTheOperatorReadChannel`、`TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase` 红；
  - 试算行退回字面量 `UnconfiguredIntake{}` → `TestThePricingEstimateAnswersFromTheOperatorReadChannel` 红（`TestEveryAssembledEndpointAnswersUnconfigured` 两边都绿，分不出这一格）；
  - 试算租户换成写死的 `SYN-TENANT-02`（变体能编译）→ `TestEstimateIntakeTakesTheTenantFromTheAuthenticatedOperator`、`TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase` 红；
  - 认证前先读一字节请求体 → `TestEstimateIntakeTakesTheTenantFromTheAuthenticatedOperator` 红；
  - 去掉空租户那道检查 → `TestEstimateIntakeTakesTheTenantFromTheAuthenticatedOperator` 红。
- 机制清点在 `095aae46` 的干净检出上重生成（`0d45f3d2`）。
- 作者不自评：整票（`ff3b01fa..` 新 tip）的非作者评审由派单方一次派出，重放进 main 由派单方做。
