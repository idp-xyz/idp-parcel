# 09 送站预报的提交与撤回口：挂首方客户渠道族，一口一词，NO 定载荷形状

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `32f6a69d`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 评审 0 阻断，治理种子撞号已改为 35、36，待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-psxm09`，access_identity 迁移号 0014；与票 14 并行。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [08](./08-pre-advice-and-handover-note-model-and-window-register.md) 进 main：预报册与领域。
- 首方客户渠道 Intake 已由 [first-party-api/00](../../first-party-api/issues/00-first-party-customer-intake.md) 进 main（`38916b00`），照它落地的形状挂，不设阻塞边。能力粒度与 1:N 绑定在 [first-party-api/01](../../first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md)，两票同改客户面能力的枚举与 CHECK，谁后进谁 rebase。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定二、九；[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四、五、八；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 应用流程步骤 1、2 与 `AT-NO-069`、`AT-NO-070`。
地盘：
- `internal/accessidentity`：客户面能力 `CustomerCapability` 加两个词；`CustomerClientMinter` 加两口的铸造方法与首方族推导口；按命令分型的信封；各自测试
- `migrations/access_identity/`：客户面能力那条 CHECK 加两个词，取当时下一个空号（另一会话正在连续取号，不写死号）
- `internal/nodeoperations/adapters/accessidentity`（首方认证适配，新文件）、`adapters/http`（两口与规范化形状）、`application`（提交与撤回编排）及测试
- `cmd/parcel-api`：端点表两行、装配、真库用例新文件
- `scripts/demo-seeds`：合成货主系统 `syn-customer-client-01` 的两格授予、准入范围对照两行、一份交预报的脚本（照 `submit-one-shipment.sh` 先取令牌）；README 与 `docs/design/synthetic-demo-journey-script.md` 相应一段
- `docs/product/MECHANISM-INVENTORY.md`（重生成）、`.scratch/at-coverage-inventory.md`

## 做什么

1. **能力词**。照 first-party-api/00 代决「一口一词」加两个：送站预报提交、送站预报撤回。字面量本票定，建议 `DELIVERY_PRE_ADVICE_SUBMISSION`、`DELIVERY_PRE_ADVICE_WITHDRAWAL`。枚举与库上 CHECK 同笔改；词写进完成记录，供票 01 归并时对照。
2. **载荷形状**。NO 定一版带形状版本的规范化形状（ADR-0139 决定八），摘要进信封。
   - 提交载荷：节点、预计送达时刻、客户包裹件号集合、可选的交付方线索、接入请求标识。
   - 撤回载荷：预报标识、接入请求标识。
   - 接入请求标识是信封元数据，不进摘要（ADR-0139 决定五）。
   - 封闭解码：自报的租户、客户账户、来源都落在未知字段，答坏载荷。
3. **铸造**。照 `MintSubmission` 加两口，校验次序同 `admit`：令牌、册、证书绑定、授予、准入范围。
   - 这两口是生产写口，准入范围在铸造侧判（ADR-0139 决定八）；能力与事实类型照 ADR-0180 决定四的词登进准入范围对照。
   - 来源请求键由首方族推导口从接入请求标识取，口径同提交口（去两端空白、此外一字不动）。
   - 撤回的推导由本票为预报撤回定，与委托撤回无关（first-party-api/00 越权风险点里委托撤回那一条不在本票）。
4. **编排**。
   - 提交：按来源请求键认重放（同内容答已有结果，异内容答来源冲突）；首次签发预报标识，落 08 的册，答回标识。
   - 撤回：只撤信封里那个客户账户的预报；指名别的客户账户的预报，与查无此预报同答。
5. **答复分格**。信封格照 ADR-0139 决定四（`ACCESS_CHANNEL_NOT_CONFIGURED`、`CUSTOMER_CLIENT_CREDENTIAL_REJECTED`、`CUSTOMER_CLIENT_NOT_GRANTED`、`OUTSIDE_ADMISSION_SCOPE`、`IDENTITY_DEPENDENCY_UNAVAILABLE`）；业务格照 UC-NO-006 结果语义契约，线上名本票定、写进完成记录。
6. **演示**。种子给 `syn-customer-client-01` 两格授予，登准入范围对照；脚本取令牌后交一份预报，打印预报标识，供票 11 的演示动线接着用。证据记 `S`。

## 不做

- 收寄带预报标识、按预报判范围（票 11）。
- 客户面读口（查自己的预报列表）：另立，本票不开。
- 1:N 绑定与能力归并（first-party-api/01）。
- 契约文档生成与 `/v1` 前缀（ADR-0140 的实施票）。

## 完成判据

- [x] HTTP 与铸造用例（`AT-NO-069`）：令牌无效 401、不在册或没有这一格授予 403、不在准入范围 403、依赖读不动 503、发行方未配置 403；别族令牌（操作者、集成客户端）一律拒；坏载荷各种答 400，含请求自带租户或客户账户、号码为空或重复。
- [x] 编排用例（`AT-NO-069`、`AT-NO-070` 的撤回半）：首次答已登记并签发标识；同一接入请求原样重投答已有结果，换内容答来源冲突；撤自己的预报落撤回，撤别的客户账户的与查无此预报同答。
- [x] 真库用例（`cmd/parcel-api`，经生产装配）：合成客户端取令牌、交预报、撤回，册上各一行；撤销授予之后再交答 `CUSTOMER_CLIENT_NOT_GRANTED`。
- [x] 演示实走（`S`）：`seed.sh` 退 0，脚本交出一份预报并打印标识。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-access-register`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

M

## 完成记录（2026-10-19，作者子代理，分支 `claude-psxm09`）

授权：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；下面各条代决照此。权威：ADR-0212 决定二、九，ADR-0139 决定四、五、八，ADR-0180 决定四，UC-NO-006 与 `AT-NO-069`、`AT-NO-070`；形状照 first-party-api/00 已落地的提交口。未立新 ADR：没有遇到上述各篇没裁的难逆转取舍。

提交（已 rebase 到 `origin/main` `6b02b547`）：`f4b97177` 代码与用例、迁移 0014；`e5c43176` 演示种子、脚本与两份演示文档；`22ed85b3` 机制清点重生成；本记录与 AT 盘点另一笔。

### 落点

- `internal/accessidentity`：`customer_client.go` 的能力枚举加两词；新文件 `customer_client_pre_advice.go`（准入能力词、两种信封、两个铸造方法、预报两口的推导口）；`admission_scope_mapping.go` 的 `checkAdmissionRequirement` 认首方族那一格。
- `migrations/access_identity/0014_delivery_pre_advice_customer_capabilities.sql`：只重建客户面能力那条 CHECK。
- `internal/nodeoperations`：`domain/delivery_pre_advice_payload.go`（规范化两面与 `CompareVersionedDigest`）；`ports/delivery_pre_advice_identity.go`（预报标识签发口）；`adapters/identity/delivery_pre_advice_id.go`；`application/delivery_pre_advice.go`（提交与撤回编排）；`adapters/accessidentity/customer_pre_advice_authenticator.go`（首方认证适配）；`adapters/http/delivery_pre_advice.go`（两口、Intake 与答复）。
- `cmd/parcel-api`：新文件 `assemble_delivery_pre_advice.go`（Intake 与编排的装配、事务包装）；`endpoints.go`、`main.go`、`endpoints_test.go` 只追加，确切行见「共享文件里加的行」一节。
- `scripts/demo-seeds`：`data/access/08` 两格授予，`data/access/03` 两行对照，`data/governance/35`、`36` 两条权威区间，`seed.sh` 两行登记与注释，新脚本 `submit-one-pre-advice.sh`，README「首方客户渠道」一节末尾送站预报那一项与 `data/access/` 那一行；`docs/design/synthetic-demo-journey-script.md` 加「送站预报：客户在送站之前交」一段。

### 能力词与迁移

- 客户面能力加 `DELIVERY_PRE_ADVICE_SUBMISSION`（送站预报提交）、`DELIVERY_PRE_ADVICE_WITHDRAWAL`（送站预报撤回），一口一词；票 first-party-api/01 归并时对照这两个字面量。
- access_identity 迁移号 `0014`：`customer_client_grant_capability_decided` 重建为 `SHIPMENT_REQUEST_SUBMISSION` 加这两词。决定种类、外部事实类型与准入范围对照的 CHECK 都没动（准入范围对照本来就没有封闭词表的 CHECK），所以与 0012、0013 加的那几个词无关。迁移头注写明「之后谁再重建这条 CHECK，都取此前各份的并集」。first-party-api/01 未开工，按协调消息不替它预留词。
- 准入范围对照的词（ADR-0180 决定四末项「一个能力词，事实类型逐口取」）：能力 `CUSTOMER_COMMAND`（`accessidentity.CustomerCommandAdmissionCapability`），事实类型取这一口的能力词。`checkAdmissionRequirement` 只收在铸造侧判准入的那几个能力词；委托提交的词登不进，因为提交口的生产归属由 UC-PS-001 编排判。对照收哪些词、铸造时问哪一对，都只从 `customerCapabilitiesAdmittedAtMint` 取。

### 载荷规范化形状（ADR-0139 决定八）

- 形状版本 `NOC-1`，与本上下文别的命令口同一个版本、同一种摘要前缀；新立两面，没有在既有面上扩列。
  - 提交面 `SUBMIT_DELIVERY_PRE_ADVICE`：`node`、`expected_arrival_at`（UTC、截到微秒，与册的精度相同）、`numbers`（排序后编码，号码本身逐字）、`delivering_party`（缺席写空串；在场的线索不许空白，两种写法分得开）。
  - 撤回面 `WITHDRAW_DELIVERY_PRE_ADVICE`：只有 `pre_advice`。
- 不进摘要的：租户、客户账户、客户端（都取自信封），接入请求标识（信封元数据，ADR-0139 决定五），签发的预报标识与形成时刻（本上下文现签）。
- 摘要进 NO 的来源信封：`domain.CustomerRequestSource{Channel, RequestKey, ContentDigest}` 与预报或撤回同行落册。摘要由编排从命令内容现算，与本上下文别的命令口一样，Intake 递不进一个与内容不符的摘要。
- 比对用 `domain.CompareVersionedDigest`：同串答同一份，同形状异串答另一份，不带前缀或前缀认不出答形状认不出，编排据此不形成答案。不复用 `CompareStoredDigest`：那一个要替 NOC-1 之前的无版本摘要按旧口径重算，这两口没有那段历史。
- 线格式（`nodeopshttp`）：提交 `{"accessRequestId", "node", "expectedArrivalAt", "customerParcelNumbers": [...], "deliveringParty"?}`；撤回 `{"accessRequestId", "preAdviceId"}`。封闭解码，自报的 `tenantId`、`customerAccountId`、`source` 都落在未知字段，答 400。

### 铸造与认证

- `CustomerClientMinter.MintDeliveryPreAdviceSubmission` / `MintDeliveryPreAdviceWithdrawal` 分别交回 `DeliveryPreAdviceSubmissionEnvelope` / `DeliveryPreAdviceWithdrawalEnvelope`，两种信封与委托的 `SubmissionEnvelope`、`WithdrawalEnvelope` 也分型。两口共用 `mintCustomerCommand`：经 `admit`（令牌 → 册 → 证书绑定 → 这一口的授予 → 准入范围，次序同 `MintSubmission` 用的那一段）之后才读请求，再推导来源请求键，经 `seal` 封出。
- 首方族推导口：`DeriveDeliveryPreAdviceSubmissionRequestKey`、`DeriveDeliveryPreAdviceWithdrawalRequestKey` 都取接入请求标识，去两端空白，此外一字不动，缺席或空白答 `ErrRequestKeyNotDerived`。撤回不加前缀、不另推：预报与预报撤回在 NO 各有自己的判重空间（两张表各按（租户，来源，来源请求键）唯一），同一接入请求标识先交预报、再撤回是两次来源请求，撤回不会被判成提交的重放。委托撤回的 `DeriveWithdrawalRequestKey` 一字未动，仍答推导不出。
- NO 的首方认证适配 `noaccess.CustomerPreAdviceAuthenticator` 把信封译成 `nodeopshttp.CustomerIdentity`（租户、客户账户、客户端 = 发行方 `#` 主体、来源请求键），并逐格译共享能力的错：未配置、令牌不过、未授予、不在准入范围、依赖读不动（核验方、客户渠道册、准入范围三者）、推导不出（答坏载荷）；读载荷自己的错原样过。

### 编排的重放与冲突代数（`application.DeliveryPreAdviceHandler`）

- 提交：规范化 → 来源（客户端、来源请求键空白即拒）→ 签发标识 → 形成预报（号码为空或重复即拒）→ 落册。册答新落：`DELIVERY_PRE_ADVICE_REGISTERED`。册答已在册（同租户同客户端同来源请求键）：摘要同，答 `EXISTING_RESULT`；摘要异，答 `SOURCE_CONFLICT`。两格都回显在册那一份，连同当初签发的标识。标识先签后落，撞上在册那一份时这一次签的号作废，客户拿到的永远是第一次签的那个。
- 撤回：册只在预报属信封里那个客户账户时落撤回。册答查无此预报：`ErrDeliveryPreAdviceNotFound`，别的客户账户的预报与不存在的预报同答。册答已在册：回显的撤回若是这一个接入请求落下的，摘要同答 `EXISTING_RESULT`、异答 `SOURCE_CONFLICT`（同一接入请求改指别的预报）；若是另一个接入请求落下的，这份预报已撤回、撤回是终态，答 `EXISTING_RESULT` 并回显在册那一份（代决 5）。
- 拒形答 `ErrDeliveryPreAdviceRejected`；册读写不动、签不出标识、在册摘要形状认不出都原样上抛，不形成答案。预报不交意图：预报没有下游要通知（ADR-0212 决定六「预报没有触发」）。

### 端点与答复

| 格 | 提交 `POST /node-operations/delivery-pre-advices` | 撤回 `POST /node-operations/delivery-pre-advice-withdrawals` |
|---|---|---|
| 首方族发行方参数未设 | `403 ACCESS_CHANNEL_NOT_CONFIGURED` | 同 |
| 令牌缺失、过期、校验不过、证书绑定对不上，别族令牌 | `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED` | 同 |
| 不在客户渠道册，或没有这一口能力的生效授予 | `403 CUSTOMER_CLIENT_NOT_GRANTED` | 同 |
| 不在准入范围（含本产品权威串没设） | `403 OUTSIDE_ADMISSION_SCOPE` | 同 |
| 发行方公钥集、客户渠道册或准入范围读不动 | `503 IDENTITY_DEPENDENCY_UNAVAILABLE` | 同 |
| 坏载荷 | `400 MALFORMED_REQUEST` | 同 |
| 首次 | `201`，`DELIVERY_PRE_ADVICE_REGISTERED`，`preAdvice` 回显 | `201`，`DELIVERY_PRE_ADVICE_WITHDRAWN`，`withdrawal` 回显 |
| 已有结果 / 来源冲突 | `200`，`EXISTING_RESULT` / `SOURCE_CONFLICT`，回显在册那一份 | 同 |
| 指名的预报看不见 | — | `404 DELIVERY_PRE_ADVICE_NOT_VISIBLE` |
| 编排没形成答案 | `500 NO_ANSWER_FORMED` | 同 |

信封各格不带 `outcome`、认证没过时一个字节的载荷都不读。`preAdvice` 回显 `preAdviceId`、`customerAccount`、`node`、`expectedArrivalAt`、`customerParcelNumbers`、`deliveringParty`（在场时）、`formedAt`；`withdrawal` 回显 `preAdviceId`、`withdrawnAt`。两个隔离开关都换不了这两口。

### 共享文件里加的行

- `cmd/parcel-api/endpoints.go`：`assembleBusinessEndpoints` 参数表末尾两行 `customerPreAdvice *nodeopshttp.CustomerPreAdviceIntake,`、`deliveryPreAdvices nodeopshttp.DeliveryPreAdviceHandler,`；提交口那段之后一行 `preAdviceSubmissionIntake, preAdviceWithdrawalIntake := customerPreAdviceIntakes(customerPreAdvice)`（带两行注释）；端点表末尾两行 `{Pattern: "/node-operations/delivery-pre-advices", …NewSubmitDeliveryPreAdviceEndpoint(preAdviceSubmissionIntake, deliveryPreAdvices)}`、`{Pattern: "/node-operations/delivery-pre-advice-withdrawals", …NewWithdrawDeliveryPreAdviceEndpoint(preAdviceWithdrawalIntake, deliveryPreAdvices)}`（带三行注释）。
- `cmd/parcel-api/main.go`：`customerSubmission` 那一段之后加 `customerPreAdvice, err := buildCustomerPreAdviceIntake(customerClientMinter)`、`deliveryPreAdvices, err := buildDeliveryPreAdviceOrchestration(db)` 两段；`assembleBusinessEndpoints(` 实参末尾两行 `customerPreAdvice,`、`deliveryPreAdvices,`。
- `cmd/parcel-api/endpoints_test.go`：`businessEndpointProbes` 末尾两行（两条新路径）；`assembleUnwiredBusinessEndpointsWithOperatorIntakes` 实参末尾两行 `nil,`、`unwiredDeliveryPreAdvices{},`（占位类型照时窗那一口的先例放在本票的 `assemble_delivery_pre_advice_test.go`）。
- `internal/accessidentity/customer_client.go`：枚举常量两行与 `customerCapabilities` 两行。
- rebase 到 `6b02b547` 时这三份 cmd 文件与第三波的追加行撞在同一处，两边都留，第三波在前、本票在后；本票测试的辅助函数与 `assemble_booking_faces_test.go` 撞名，本票这一只改名 `postPreAdvice`。

### 种子与演示脚本

- `syn-customer-client-01` 加两格授予（`SYN-GRANT-CUSTOMER-PRE-ADVICE-SUBMISSION-01`、`…-WITHDRAWAL-01`）。准入范围对照两行（能力 `CUSTOMER_COMMAND`，事实类型取能力词，对象范围 `SYN-PILOT-SCOPE/customer-commands@v1`，依据 `SYN-MAP-BASIS-03`）。治理登记册两条本产品权威区间（`data/governance/35`、`36`，权威串 `SYN-AUTH/idp-parcel-pilot`）。
- `scripts/demo-seeds/submit-one-pre-advice.sh [客户包裹件号 ...]`：照 `submit-one-shipment.sh` 先取首方令牌（或用 `IDP_PARCEL_CUSTOMER_TOKEN`），交一份预报，签发的标识单独一行打到标准输出，说明打到标准错误，供票 11 的动线 `PRE_ADVICE_ID=$(…)` 接着用。节点缺省 `SYN-NODE-SHA-HUB`，预计送达缺省此刻往后两小时，可用环境变量改；不给号码时用按 UTC 时刻起的合成号。
- README「首方客户渠道」一节末尾送站预报那一项、`data/access/` 那一行，`docs/design/synthetic-demo-journey-script.md` 一段（含实走记录）。

### 用例与完成判据对照

- HTTP 与铸造（`AT-NO-069`）：
  - `internal/accessidentity/customer_client_pre_advice_test.go`：`TestDeliveryPreAdviceCapabilitiesJoinTheClosedVocabulary`、`TestPreAdviceLinesMintTheirEnvelopesFromTheBinding`、`TestPreAdviceLinesAnswerTheirOwnGrades`、`TestPreAdviceLinesNeedAnAccessRequestIdentifier`、`TestCustomerCommandAdmissionWordsAreOnlyThoseMintedLinesAskFor`。
  - `internal/nodeoperations/adapters/accessidentity/customer_pre_advice_authenticator_test.go`：`TestCustomerPreAdviceAuthenticatorTranslatesTheEnvelope`、`TestCustomerPreAdviceAuthenticatorTranslatesEveryGrade`。
  - `internal/nodeoperations/adapters/http/delivery_pre_advice_test.go`：`TestPreAdviceLinesAnswerTheEnvelopeGrades`、`TestMalformedPreAdvicePayloadsAnswerBadRequest`（含自带租户、客户账户、来源，号码为空或重复）、`TestPreAdviceLinesAnswerUnconfiguredAndKeepTheMethodGate`、`TestSubmittingAPreAdviceTranslatesThePayloadAndAnswersRegistered`、`TestPreAdviceLinesMapTheirOutcomes`、`TestWithdrawingAPreAdviceTakesTheAccountFromTheEnvelope`。
  - 别族令牌一律拒、发行方未配置、不在准入范围（经装配点）：`cmd/parcel-api/assemble_delivery_pre_advice_test.go` 的 `TestPreAdviceLinesRefuseOtherFamiliesAndJudgeAdmission`、`TestPreAdviceLinesAnswerNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset`。
- 编排（`AT-NO-069`、`AT-NO-070` 的撤回半）：`internal/nodeoperations/application/delivery_pre_advice_test.go` 的 `TestSubmittingAPreAdviceRegistersReplaysAndConflicts`、`TestAPreAdviceThatCannotFormNeverReachesTheRegister`、`TestWithdrawingAPreAdviceTouchesOnlyTheEnvelopesOwnAccount`、`TestDeliveryPreAdviceHandlerRefusesMissingDependencies`；载荷形状在 `internal/nodeoperations/domain/delivery_pre_advice_payload_test.go`（`TestPreAdviceSubmissionCanonicalizesAsNOC1`、`TestPreAdviceSubmissionDigestSeparatesEveryContentChange`、`TestPreAdviceSubmissionCanonicalizationRefusesIncompleteContent`、`TestPreAdviceWithdrawalCanonicalizesAsNOC1`、`TestVersionedDigestComparisonOnlyComparesWithinItsShape`）。
- 真库（经生产装配）：`cmd/parcel-api/assemble_delivery_pre_advice_test.go` 的 `TestACustomerClientSubmitsAndWithdrawsAPreAdviceAgainstARealDatabase`。合成客户端持两格授予，对照与区间照种子登法；发行方替身签令牌；交预报、重投、换内容、另一客户账户的客户端撤这份与撤不存在的同答 404、本客户端撤回与重投，册上预报与撤回各一行，撤销提交授予之后再交答 `CUSTOMER_CLIENT_NOT_GRANTED`。迁移 0014 的 CHECK 另有 `internal/accessidentity/adapters/postgres/customer_client_pre_advice_grant_test.go` 的 `TestDeliveryPreAdviceGrantsPassTheCapabilityCheckAndRoundTrip`。标识签发器：`internal/nodeoperations/adapters/identity/delivery_pre_advice_id_test.go`。
- 演示实走（`S`）：见下面「验证」。

### 先红后绿

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm09-red.log`。第一段在 `4642eb2b` 上只加用例，各包编译不过（`undefined: accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission`、`minter.MintDeliveryPreAdviceSubmission undefined`、`undefined: nodeopshttp.PresentedCustomerCredential`、`undefined: application.SubmitDeliveryPreAdviceCommand`、`undefined: domain.CanonicalizeDeliveryPreAdviceSubmissionPayload` 一类）。第二段只加了 Go 枚举、还没有迁移 0014 时，真库用例的原话：`登记事务：register customer client grant: ERROR: new row for relation "customer_client_grant" violates check constraint "customer_client_grant_capability_decided" (SQLSTATE 23514)`。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm09-green.log`，钉 rebase 后的 `e5c43176`。

### 验证

- 门（钉 `e5c43176`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 55432 DSN 以 `-p 1 -count=1` 跑 `./internal/accessidentity/...`、`./internal/nodeoperations/...`、`./cmd/parcel-api/...`、`./cmd/parcel-access-register/...`、`./migrations/...`、`./scripts/demo-seeds/...`、`./internal/architecture/...` 全 ok。没跑全量。机制清点在同一 tip 上重生成（`22ed85b3`）。
- 演示实走（`S`，rebase 前在 `a8042e10`）：一次性库 `psxm09_demo`，`seed.sh` 退 0，两格授予、两行对照答 `RECORDED`，两条区间答 `REGISTERED`。共享的集成客户端发行方容器是按旧配置起的，答 `invalid_client`、不认合成货主系统，所以另起一只私有 Dex（同一份 `integration-dex.yaml`，发布在 `127.0.0.1:5562`，issuer 串仍是 `http://127.0.0.1:5558/dex`），没有重启别的会话在用的容器。`parcel-api` 只带首方族三件（JWKS 指 5562）与 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY=SYN-AUTH/idp-parcel-pilot` 起。`submit-one-pre-advice.sh SYN-PCLREF-PSXM09-0001 SYN-PCLREF-PSXM09-0002` 答 `DELIVERY_PRE_ADVICE_REGISTERED`，标准输出打出 `PREADV-BGOJWJ3AE7XUSBEIUHTQFA5YGU`。同一进程上：不带令牌答 401，自报 `tenantId` 答 400，撤回答 201、原样重投答 200 `EXISTING_RESULT`，撤一份不存在的答 404 `DELIVERY_PRE_ADVICE_NOT_VISIBLE`。同一 UTC 秒内再跑脚本、不给号码，撞同一接入请求标识，答 `SOURCE_CONFLICT`，脚本退 1；隔一秒再跑，签出 `PREADV-T7VC26KBK6DXBVUU4DDMDYQ2S4`。册上两行预报（来源 `http://127.0.0.1:5558/dex#ChZzeW4tY3VzdG9tZXItY2xpZW50LTAx`，摘要 `NOC-1:` 前缀）、一行撤回。实走记录在 scratchpad 的 `psxm09-demo-walk.log`。走完已停进程、删容器、删库。
- rebase 后在另一个一次性库 `psxm09_seed2` 上重灌一次合批后的种子，`seed.sh` 退 0，本票的授予与对照照常 `RECORDED`，用完删库。`psb26_walk` 未碰。

### 代决

1. **两个能力词的字面量**：`DELIVERY_PRE_ADVICE_SUBMISSION`、`DELIVERY_PRE_ADVICE_WITHDRAWAL`，照票面建议。
2. **准入要求取（`CUSTOMER_COMMAND`，能力词）**，不拿能力词顶能力格。取法照集成客户端族的 `EXTERNAL_FACT`：授予单位进事实类型格，族一级的词进能力格，阶段治理能按「客户业务命令面」一并登区间。只在铸造侧判准入的能力词登得进对照。
3. **形状版本用 `NOC-1` 新立两面**，不另起版本：形状描述「业务内容是否同一份」，不随入口换版。号码排序后编码，时刻截到微秒，交付方线索缺席写空串。
4. **撤回的来源请求键与提交同一口径**（取接入请求标识），靠两本册各自判重分开，不加前缀。
5. **另一个接入请求撤一份已撤回的预报，答 `EXISTING_RESULT` 并回显在册那一份撤回**：两次撤回的内容都只是这份预报，是同一个结论；撤回是终态，不另立「已撤回」一格。
6. **标识先签后落**，重投时这一次签的号作废；前缀 `PREADV`，随机 128 位，不取库序列。
7. **路径** `/node-operations/delivery-pre-advices`、`/node-operations/delivery-pre-advice-withdrawals`：随本上下文命令口的前缀，撤回另立子资源，理由同 `/shipment-requests/withdrawals`。
8. **看不见的预报答 `404 DELIVERY_PRE_ADVICE_NOT_VISIBLE`**，不答 200 带 outcome：照本上下文身份冲突处置口「指名的对象查不到答 404，与越权探针同答」的先例。业务格线上名见上表。
9. **提交口不判有效时窗，也不核节点在不在网络目录**：时窗是收寄判范围时读的租户取值（ADR-0212 决定二），节点不在目录的预报只是永远比不上。号码逐字交给 PS 入册门，不去两端空白。
10. **规范化形状放在 NO 领域、不放在 HTTP 适配器**（票面「地盘」写的是 `adapters/http`）：本上下文别的命令口都在领域里定形状、由编排从命令内容现算摘要，Intake 递不进一个与内容不符的摘要。
11. **演示的对象范围另起** `SYN-PILOT-SCOPE/customer-commands@v1`，不与运营决定、外部事实的合成范围合并；治理区间按种子登法另登两条。

### 越权风险点（待 owner 复核）

1. `CUSTOMER_COMMAND` 这个词进了准入范围对照与演示的治理区间。owner 若要每一口在治理登记册上是独立的能力，改的是 `customerCommandAdmission` 一处与种子里那两行对照、两条区间，已登的对照与区间要结束重登。
2. 两个能力词进了库上的 CHECK 与种子授予。票 first-party-api/01 若按用例归并（例如一个「送站预报」词管两口），改的是枚举、CHECK、已登的授予行与准入对照的事实类型。
3. 代决 5 让后到的撤回看不出「这一次请求不是落册的那一次」。owner 若要分开，加一格 outcome，在编排的撤回已在册分支里分。
4. 预计送达时刻不与此刻比，过去的时刻也收。范围靠收寄时按时窗判兜住；owner 若要在提交时就拒，加在编排的形成门前，那是租户取值还是产品策略要先裁。
5. 线格式字段名在 ADR-0140 的契约生成之前由本票定；契约票若改名，客户系统与 `submit-one-pre-advice.sh` 一起改。
6. 撤回与提交共用接入请求标识的口径（代决 4）：客户系统若把同一个标识先用于提交、再用于撤回，两笔都成立，不会被当成违约。owner 若要求接入请求标识在客户面各口之间也唯一，就要一个跨口的判重空间，改的是推导口与 NO 两本册。

### 越出地盘的改动

- `internal/nodeoperations/domain/delivery_pre_advice_payload.go`（及用例）：形状放进领域，见代决 10。
- `internal/nodeoperations/ports/delivery_pre_advice_identity.go`、`internal/nodeoperations/adapters/identity/delivery_pre_advice_id.go`（及用例）：预报标识的签发口与签发器，照收寄结果版本那一对的形状另起一对。
- `internal/accessidentity/admission_scope_mapping.go`：`checkAdmissionRequirement` 加首方族一格，并把头注的「两族」改成「各族」。票面只写了「登进准入范围对照」，不改它就登不进。
- `scripts/demo-seeds/data/governance/35`、`36` 与 `seed.sh` 那两行：票面只写了「准入范围对照两行」，没有区间，对照就只换坐标、不放行。
- `internal/accessidentity/adapters/postgres/customer_client_pre_advice_grant_test.go`：只加用例，钉迁移 0014。
- 没有动 `register_pre_advice_window` 一支、时窗册与 `node_operations` 迁移（票 14 的地盘），也没有动 `internal/nodeoperations/ports/delivery_scope_registers.go`。

### 未做

- 完成判据末格「走 Go 那条路」只做了分支与作者自验；非作者评审、推送方在干净检出上带 DSN 跑全量，归推送方。
- 没在一次性库上起进程实走「撤销授予之后再交」：真库用例覆盖了这一格。
- 票面「不做」一节照旧：收寄带预报标识（票 11）、客户面读口、1:N 与能力归并（first-party-api/01）、契约生成与 `/v1`（ADR-0140）。

### AT 盘点

`.scratch/at-coverage-inventory.md` 照口径同步：`AT-NO-069` 由②入①；`AT-NO-070` 已在①，行内补撤回命令半边，收寄与重核那一半仍归票 11；见那份文件的追记十五。

### 评审后改号（2026-10-19）

评审 0 阻断，只有一处撞号：另一会话的第四波已占治理种子 `governance/32`、`33`、`34`。本票两份权威区间改为 `35-authority-interval-customer-command-delivery-pre-advice-submission.json`、`36-authority-interval-customer-command-delivery-pre-advice-withdrawal.json`，文件名其余部分与内容不变，`seed.sh` 那两行与 README `data/access/` 那一行随改（`0f04a07a`）。上文各节引的号已改成 35、36；实走记录里的 `seed.sh` 灌入发生在改号之前。改号后：`origin/main` 仍是 `6b02b547`，rebase 无事可做；`gofmt -l .` 无输出，`go build ./...`、`scripts/demo-seeds/...` 与 `cmd/parcel-api/...` 的 `go vet` 退 0，带 DSN 跑 `./scripts/demo-seeds/...`、`./cmd/parcel-api/...` 全 ok；一次性库 `psxm09_seed3` 上 `seed.sh` 退 0，治理登记册上两条 `CUSTOMER_COMMAND` 区间在册，用完删库，`psb26_walk` 未碰。机制清点重生成无差。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `e61c49fd`，改号后钉 `f7fa3fd3`）

两轴合审，0 阻断。

- **Spec**：ADR-0212 决定二、九与 ADR-0139 决定四、五、八落到。客户面能力加 `DELIVERY_PRE_ADVICE_SUBMISSION`、`DELIVERY_PRE_ADVICE_WITHDRAWAL`，迁移 access_identity 0014 只重建客户面能力那条 CHECK、头注写明取并集；铸造照 `MintSubmission` 的校验次序，准入范围在铸造侧判。两面规范化形状挂 NOC-1，接入请求标识、租户、客户账户、客户端都不进摘要；封闭解码拒自报身份。编排的重放与冲突代数（原样重投已有结果、换内容来源冲突，回显在册）；撤别的客户账户的预报与不存在的同答 404，不泄露存在性。端点答复各格与首方族一致。演示种子与脚本在一次性库实灌实走过。
- **作者自定的格认可**：规范化形状放在 NO 领域（与 NO 其余命令口一致）；预报标识签发器另起端口与适配器；另一接入请求去撤已撤回的预报答已有结果。
- **评审修补（已做，`0f04a07a`）**：两份权威区间种子撞了另一会话已占的 `governance/32`、`33`，改为 35、36。
- **与另一会话的共享文件**：`cmd/parcel-api` 三份与 `customer_client.go` 只追加，rebase 时与对方第三波两边都保留。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-psxm09`（作者已 rebase 到另一会话第三波之后、改治理种子号）快进推送；验证之后远端只前进了两笔纯票面（另一会话立票），再 rebase 一次，非 `.scratch` 文件与验证钉逐字相同，未重跑。
- 推送方全量一次：钉 `32f6a69d` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 151 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
