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
- **客户追踪视图是例外**：读不到视图也是业务答案，答 `200`；`outcome` 取 `CURRENT_VIEW`（有当前视图）或 `VIEW_NOT_FOUND`（不存在、属别的账户、归属未定等读不到视图的情形同答一格）。视图尚未形成、包裹唯一归属请求方账户时答 `NO_VISIBLE_RESULT`，由票 [code/31](../../.scratch/docs-consistency/code/31-customer-tracking-query-cannot-say-no-visible-result-for-its-own-parcel.md) 落（[ADR-0246](../adr/0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md) 决定一）。查询串只认 `parcel` 一格。

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
| `POST /v1/node-operations/delivery-handover-conclusions` | UC-NO-006 交接单的客户确认或否认 | `DELIVERY_HANDOVER_CONFIRMATION` |

- 提交答复建成委托时带 `parcels`：每件 `parcelId` 加客户为它声明的 `customerParcelReference`。`parcelId` 就是查客户追踪视图用的号（[ADR-0223](../adr/0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)）。
- 客户追踪视图的答复带视图版本 `version` 与逐维三态，不带它所依据的内部投影版本；客户采用哪一版只看 `version`。
- 委托的撤回、资料修订、受控补充与逐包裹取消这几个客户面命令口已经按本文的问题详情作答，但还没挂首方渠道 Intake，今天在无前缀的 `/shipment-requests/...` 下答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`；首方 Intake 逐口换上时迁到 `/v1/shipment-requests/...`。
