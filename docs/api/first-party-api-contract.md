# 首方客户 API 契约

本文是首方客户 API（租户的货主客户系统调用的那一族，[ADR-0139](../adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)）对调用方的契约说明：版本、状态码、问题详情、身份与时间、重放这几条横切规则，以及首方族今天开着哪几口。各条的取舍归所引的 ADR，本文只把它们写成调用方读得懂的一份，不另立口径；冲突时以 ADR 为准。

逐端点的请求与响应字段今天以各上下文 `adapters/http` 里处理器的响应结构体为准。机器可读的 OpenAPI 契约从端点描述符生成（[ADR-0140](../adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 决定一），生成器、描述符与 `/docs`、`/openapi/v1.json` 入口还没落，见票 [docs-consistency/code/82](../../.scratch/docs-consistency/code/82-operator-and-integration-families-still-answer-the-legacy-problem-shape-and-the-contract-is-not-generated.md)；落地之后字段以生成物为准，本文的横切规则并进生成物的说明。

证据层级：本文所述行为只在演示租户 `SYN-TENANT-01` 上走通过，只记 `S`。

## 版本与兼容性

- **URL 主版本**：首方族各口带前缀 `/v1`（ADR-0140 决定二）。前缀属装配，不属处理器；同一处理器将来在 `/v2` 下服务，改的是装配表。
- **兼容性基线**：首方契约还没有对外发布过基线（ADR-0140 决定五）。首次对外发布时显式标记首个基线，之后每笔改动与最近一次已发布基线比对，判为破坏的只能进下一个主版本。基线之前的改名不留并行期：`/shipment-requests` 等无前缀路径已直接改为 `/v1/...`，旧路径答 `404`。
- **`outcome` 新增取值不算破坏**，条件是调用方守这一条：**遇到不认识的 `outcome`，视为服务端已形成答案、但不可自动处置，出队交人**，不重发、不当成失败。删除或改名 `outcome` 取值是破坏。
- **弃用**：进入弃用期的端点或字段带 `Deprecation`（RFC 9745）与 `Sunset`（RFC 8594）头，契约在该操作上标 `deprecated`。窗口是主版本进入弃用之后至少 12 个月（ADR-0140 决定六，产品策略）。`Sunset` 之前当前主版本照常作答。

## 状态码

状态码只回答「服务端有没有形成答案」，业务判别一律在响应体的 `outcome`（[ADR-0022](../adr/0022-http-status-carries-answer-formed-not-business-verdict.md)）。

| 类别 | 含义 | 调用方动作 |
|---|---|---|
| `201` / `200` | 形成了业务答案：`201` 表示新建了对象，其余答案 `200` | 按 `outcome` 处置 |
| `4xx` | 调用方的错，重发同一请求结果不变 | 出队交人，不自动重发 |
| `429` | 限流（未落地，票 [first-party-api/02](../../.scratch/first-party-api/issues/02-per-client-rate-limit-and-quota.md)） | 唯一可以重发的 4xx：按 `Retry-After` 秒数之后重发（ADR-0140 决定三） |
| `5xx` | 服务端没能形成答案 | 留在队里，退避后重发同一请求 |

- 不收 `Idempotency-Key`（ADR-0022）：重放的识别靠接入请求标识与规范化内容摘要，见下文「重放」。
- **指名的对象看不见答 `404`**：委托、预报、交接单不存在，或属于别的租户、别的客户账户，同答一格，不分「不存在」与「无权」（ADR-0022）。码按上下文：`SHIPMENT_REQUEST_NOT_VISIBLE`、`DELIVERY_PRE_ADVICE_NOT_VISIBLE`、`DELIVERY_HANDOVER_NOTE_NOT_VISIBLE`。
- **客户追踪视图是例外**：读不到视图也是业务答案，答 `200`。`outcome` 三格（[ADR-0246](../adr/0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md) 决定一），按调用方的续办分：
  - `CURRENT_VIEW`：有当前视图，带 `view`。各维待确认或不展示也是这一格，逐维看。
  - `NO_VISIBLE_RESULT`：包裹所在委托已`已接受`、唯一归属本账户，视图尚未形成。稍后再查；不表示未收寄、未运输或遗失。
  - `VIEW_NOT_FOUND`：不存在、属别的账户、归属未定（含委托接受之前）、多份已接受委托同时声明，同答一格、同一份字节。核对你给的号。刚从提交答复拿到、委托还没接受的号也落这一格：稍后再查，或请租户客服核对（决定五）。
  - 后两格只有 `outcome`。归属读不动答 `500 NO_ANSWER_FORMED`，与视图读不回同一格，照 5xx 退避重发。
  - 查询串只认 `parcel` 一格。

## 问题详情

各口处理器作答的 `4xx` 与 `5xx`，响应体是 RFC 9457 问题详情，媒体类型 `application/problem+json`（ADR-0140 决定三）。仅限这一类（方法不对的 `405` 由处理器自答，在内）：路径不存在的 `404` 与进程兜底的 `500`（处理器崩溃时由恢复中间件答）出自路由层，今天不是问题详情，并入票 [code/82](../../.scratch/docs-consistency/code/82-operator-and-integration-families-still-answer-the-legacy-problem-shape-and-the-contract-is-not-generated.md)。

| 成员 | 内容 |
|---|---|
| `type` | 一码一个稳定 URI：`tag:idp.xyz,2026:problem:<code>` |
| `title` | 码的固定短语，同一个码永远同一句，不随请求变 |
| `status` | HTTP 状态码 |
| `code` | 稳定错误码，调用方据以分流 |
| `requestId` | 本次请求的关联标识；找租户运营或排障时带上它 |
| `detail` | 只在「改请求才会好」的 4xx 出现，只说调用方自己这份载荷错在哪一格；是散文，不据此分支。`403`、`404` 与 `5xx` 一律不带 |

- 问题详情不带 `outcome`：它只说明为什么没有业务答案。
- 措辞不带任何别的客户、租户或线路的信息（UC-PS-001 的泄露约束）。
- 认证与授予各格按恢复动作分开（ADR-0139 决定四）：`401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`（令牌缺失、过期、校验不过，或证书绑定对不上：换令牌），`403 CUSTOMER_CLIENT_NOT_GRANTED`（客户端不在册或没有这一口的授予：找租户运营），`403 ACCESS_CHANNEL_NOT_CONFIGURED`（租户还没配置首方渠道），`403 OUTSIDE_ADMISSION_SCOPE`（这笔生产写不在准入范围内），`503 IDENTITY_DEPENDENCY_UNAVAILABLE`（身份依赖读不动：重发）。

## 身份、来源信封与时间

- 调用方以 OAuth 2.0 客户端凭据取令牌，带 `Authorization: Bearer`。租户与货主客户账户只取自客户渠道册上这台客户端的绑定；请求体或查询串里自报的账户按坏请求拒绝。
- 每个命令载荷带 `accessRequestId`（接入请求标识）。它铸成这一笔的来源请求键，是重放识别的键（ADR-0139 决定五）。
- **首方契约不收客户声明的来源发生时间。** 来源信封由本产品的接入适配器铸造，来源发生时间与系统接收时间都记本系统收到这次请求的时刻；客户要表达「这份请求从何时起在业务上适用」，写在 `requestEffectiveAt`（客户请求生效时间），它进规范化内容与内容摘要（[ADR-0258](../adr/0258-the-first-party-family-declares-no-source-occurrence-time-and-records-the-receipt-instant.md)）。客户系统离线攒批再发，来源发生时间记的是送达时刻。

## 重放

- 同一 `accessRequestId`、同一内容再发一次，是同一请求的重放：答 `EXISTING_RESULT`，回显原来那一份。提交口的 `EXISTING_RESULT` 另带 `priorOutcome`（上一次答出的那一格）；上一次答 5xx、或停在生产归属未决的，重发接着办（[ADR-0266](../adr/0266-a-submission-replay-answers-what-the-first-round-answered-and-only-unresolved-ownership-is-judged-again.md)）。
- 同一 `accessRequestId` 携带不同内容，是来源冲突，答对应口的冲突 `outcome`，原请求不被覆盖。
- 答复丢失时，用同一 `accessRequestId` 原样重发取回原结果；没有另开按委托号读回的口。

## 首方族今天开着的口

| 方法与路径 | 用例 | 能力授予 |
|---|---|---|
| `POST /v1/shipment-requests` | [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 提交委托 | `SHIPMENT_REQUEST_SUBMISSION` |
| `GET /v1/customer-tracking-view?parcel=<包裹号>` | [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md) 客户追踪视图 | `CUSTOMER_TRACKING_VIEW_QUERY` |
| `POST /v1/node-operations/delivery-pre-advices` | [UC-NO-006](../application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 送站预报 | `DELIVERY_PRE_ADVICE_SUBMISSION` |
| `POST /v1/node-operations/delivery-pre-advice-withdrawals` | UC-NO-006 预报撤回 | `DELIVERY_PRE_ADVICE_WITHDRAWAL` |
| `POST /v1/shipment-requests/supplements` | [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 委托补充（`AT-PS-036` 第一支） | `SHIPMENT_REQUEST_SUPPLEMENT` |
| `POST /v1/shipment-requests/withdrawals` | [UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md) 委托撤回（`AT-PS-134` 至 `AT-PS-137`） | `SHIPMENT_REQUEST_WITHDRAWAL` |
| `POST /v1/node-operations/delivery-handover-conclusions` | UC-NO-006 交接单的客户确认或否认 | `DELIVERY_HANDOVER_CONFIRMATION` |
| `POST /v1/claims` | [UC-VE-007](../application/visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md) 索赔提交 | `CLAIM_SUBMISSION` |

- 提交答复建成委托时带 `parcels`：每件 `parcelId` 加客户为它声明的 `customerParcelReference`。`parcelId` 就是查客户追踪视图用的号（[ADR-0223](../adr/0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)）。
- 接入请求标识不得以保留命令前缀（`withdrawal:`、`parcel-cancellation:`、`source-data-amendment:`、`supplement:`、`claim:`）开头；提交口收到这样的标识答 400（推导不出）。
- 提交答复在指名委托时同带 `submissionVersionId`（该委托此刻的当前提交版本标识，随 `shipmentRequestId` 同在同缺；重放在补充之后读回的是补充形成的当前版本）。客户下一次补充以它为基准载荷带回，基准过期答 `SUPPLEMENT_BASIS_STALE`（[ADR-0310](../adr/0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md) 取舍 E）。
- 客户追踪视图的答复带视图版本 `version` 与逐维三态，不带它所依据的内部投影版本；客户采用哪一版只看 `version`。
- **委托补充**（形状 `PSU-1`）：客户在`已提交`委托上重报整份内容，形成同一委托的新提交版本。载荷是提交载荷的内容字段（`customerShipmentReference`、`requestedServiceProduct`、`requestEffectiveAt`、寄收件关系 / 地址 / 邮编 / 国家码、`destinationServiceScope`、`parcels[]`），外加：`accessRequestId`（补充自己的接入请求标识）、`originalAccessRequestId`（原提交的接入请求标识，指名委托）、`shipmentRequestId` 与 `basisSubmissionVersionId`（提交回执里的委托号与 `submissionVersionId`）；`parcels[]` 每件另带回执里的 `parcelId`，成员集合须与委托当前版本一致。租户与账户取自册上绑定：委托只在信封账户名下找，他账户的委托与不存在的委托同答 `404`；载荷里自报账户、租户或操作者答 `400`。答复 `outcome`：`SUPPLEMENT_RECORDED`（`201`，带新版本的 `submissionVersionId`，下一次补充以它为基准）、`SUPPLEMENT_ALREADY_HANDLED`（`200`，重放读回原版本）、`SUPPLEMENT_BASIS_STALE`（基准不是当前版本：先读当前版本，即对原提交重放取回执）、`SUPPLEMENT_SOURCE_CONFLICT`、`SUPPLEMENT_BOUNDARY_CHANGED`（成员集合变了，改走关联新委托）、`SUPPLEMENT_DECISION_ALREADY_FORMED`、`SUPPLEMENT_UNDECIDED`。补充形成新版本之后，对原提交重放，回执读回的是新版本的 `submissionVersionId`。`shipmentRequestId` 须与 `originalAccessRequestId` 指到的委托一致，不一致按找不到答 `404`。`originalAccessRequestId` 填错答 `404` 之前，这次补充请求的来源指纹已被保全，同一 `accessRequestId` 就此被占用：改正后要换一个新的接入请求标识重试；同一 `accessRequestId` 携带不同的原提交标识、委托号或成员，答 `SUPPLEMENT_SOURCE_CONFLICT`。
- **委托撤回**（形状 `PWD-1`）：客户撤回仍为`已提交`的委托。载荷只有四格：`accessRequestId`（撤回自己的接入请求标识）、`originalAccessRequestId`（原提交的接入请求标识，指名委托）、`reason`（结构化原因代码，不是自由文本，必填）、`shipmentRequestId`（回执委托号，可不带；带了就与找到的委托核对，不一致答 `404`）。租户、账户与撤回者取自册上绑定（撤回者记渠道行上的客户端），载荷里自报账户、租户、来源或撤回者答 `400`。授权就是账户绑定加撤回授予，无 PC 裁定；委托只在信封账户、同一客户端名下找，他账户、别的客户端的委托与不存在的委托同答 `404`、同字节。摘要把原接入请求标识、委托号（带与不带是两份内容）与原因纳入：同一撤回 `accessRequestId` 改指别的委托、补带委托号或改原因答 `SOURCE_CONFLICT`。答复 `outcome`：`FORMED`（`201`，带 `requestState: WITHDRAWN` 与 `withdrawalId`，随附资金释放未完成时带 `compensationReference`，提供方确定不受理时带 `controlReleaseNotAccepted`）、`DECISION_ALREADY_FORMED`（`200`，已撤回、已接受或已拒绝的委托重放答已有决定并带那份决定：已撤回带原 `withdrawalId`；已接受或已拒绝带 `decisionKind`，撤回不成立）、`SOURCE_CONFLICT`、`NOT_AUTHORIZED`、`UNDECIDED`（带 `pendingReason` 与 `continuationReference`）。
  - **重放语义（客户端必须按此实现）**：撤回的重放是接着办，不是读回上一次的答案。委托仍为`已提交`时，原样重放会重新判断并可能形成撤回。直客撤回的授权是渠道绑定，今天不会出现 `NOT_AUTHORIZED`，但契约保留这一格；未获授权、不允许、等复核一类请求，条件变了之后原样重放可能形成版本或决定，客户端不要假定同一请求永远得到同一个答案。`UNDECIDED` 是可重放的：同一 `accessRequestId` 原样重放，授权恢复后形成撤回。并发重放同时到达时，输的一路答 `UNDECIDED`，`pendingReason` 为 `STALE_SHIPMENT_REQUEST_REVISION`，需再重放一次读到已有决定，客户端要把撤回的未决当作可重放。
  - **重放必须逐字节同载荷**：判重按摘要，原样重放才算重放。例如首次没带 `shipmentRequestId`、重发时补上，会答 `SOURCE_CONFLICT`，不会读回原撤回；想补带委托号，换一个新的撤回 `accessRequestId`。
  - **撤回后读回**：提交回执不表达委托状态，撤回之后对原提交重放仍答 `EXISTING_RESULT` 与原回执同一版本；要读回撤回结果，重放撤回请求（答同一份 `withdrawalId`）。换一个撤回 `accessRequestId` 再撤同一委托也读回原撤回，不形成第二次。
- 委托的资料修订与逐包裹取消这几个客户面命令口已经按本文的问题详情作答，但还没挂首方渠道 Intake，今天在无前缀的 `/shipment-requests/...` 下答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`；首方 Intake 逐口换上时迁到 `/v1/shipment-requests/...`。

## 索赔口 `POST /v1/claims`（形状 VCL-1）

- **载荷**（封闭，其外任何一格答 `400 MALFORMED_REQUEST`，自报的租户、账户、操作者与提交时间都在此列）：`accessRequestId`、`batch`（客户侧批次引用）、`item`（项标识，批次内唯一）、`applicant`（申请人引用）、`contract`（合同责任范围引用）、`target`（目标包裹，用提交答复里的 `parcelId`）、`kind`（索赔类型引用）。租户与货主客户账户取自册上绑定。
- **提交时间**取本系统收到请求的时刻，不收客户自报；首次索赔期限从它起算。
- **唯一性**以（账户、批次、项）为自然键（ADR-0310 取舍 F）。同一对（批次、项）再次到达：申请人、合同、目标、类型都相同当重放，答 `200 CLAIM_EXISTING_RESULT` 并回显原项；任一不同答 `200 NOT_ACCEPTED`，带 `notAcceptedReason: CLAIM_CONTENT_DIFFERS_FROM_ACCEPTED`、不带原项，原项不被覆盖；`accessRequestId` 只管传输重放，不进任何判据。已在案的（批次、项）若属于同租户的另一个账户，答 `200 NOT_ACCEPTED`、不带原项。
- **引用核对**：`target` 须是信封账户当前已接受委托声明的包裹，`contract` 须登记在信封账户名下；不属于、不存在、多份委托同时声明同一包裹都答 `404 CLAIM_REFERENCE_NOT_FOUND`，字节相同，不泄露别的账户的引用存在与否。`applicant` 不在这里核：授权名单按（租户、账户）登记，由资格审核的授权维核，名单外的申请人形成不予受理，不是请求错误。
- **答复**：受理 `201 CLAIM_RECEIVED`，回执只带 `batch`、`item`、`submittedAt`，资格与责任结论字段结构上不存在。`401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`、`403 CUSTOMER_CLIENT_NOT_GRANTED`、`403 OUTSIDE_ADMISSION_SCOPE`（索赔提交在铸造侧判准入范围）、`503 IDENTITY_DEPENDENCY_UNAVAILABLE` 都是问题详情，不带 `outcome`。
