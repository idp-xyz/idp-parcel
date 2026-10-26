# 10 资料修订口经首方渠道接通

Category: enhancement
Status: in-progress——2026-10-26 作者完工（PS/装配侧），待隔离评审与进 main，分支 `claude-fpa10`（代码最后一笔 e15265e10）；演示租户的授予与种子、「演示客户端能对自己的委托做修订」留到票 first-party-api/13 进 main 之后补一笔。此前in-progress——2026-10-26 会话 `tops-20` 认领（前置 code/36、code/44 已进 main），分支 `claude-fpa10`。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 06；docs-consistency/code/44（tops-26）
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeSourceDataAmendment`、请求方（客户本人）到 PC 商业坐标的映射、PC 裁定沿用既有适配器、演示租户登授权册与允许矩阵参考配置、`endpoints_customer_amendment.go` 换真 Intake。

## 完成判据

- [x] 首方客户凭证经 `/v1` 该口提交；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例，授予在用例里用 `SaveGrant` 造）。「演示租户答成功」待 13（授予登记入口与演示种子）进 main 后补。
- [x] 新 AT 行与 AT 盘点①表行；Go 门禁。 [ ] 全量 `scripts/test-full.sh`（由推送方跑）；种子 `seed.sh` 退 0（本票未改种子，待 13）。

## 规模

L

## 完成记录

- **起点** `3f34ded92`。**提交** 代码 `e15265e10`（分支 `claude-fpa10`），文档一笔随后。无 PS 应用层改动，无迁移，未改种子。
- **改了什么**：
  - 域：`CanonicalizeCustomerSourceDataAmendment`（形状 `PAM-1`，`customer_command_shapes.go`），要素内容复用提交口的 `AddressElementsOf`。
  - 首方 Intake：`adapters/http/customer_amendment_intake.go`（`CustomerAmendmentIntake`、`NewCustomerAmendmentHandler` 核对委托号）；认证器 `CustomerAmendmentAuthenticator`（`adapters/accessidentity`）。
  - 坐标映射：`adapters/partycommercial/customer_self_amendment_request_source.go`（`CustomerSelfAmendmentRequestSource`，常量 `CustomerSelfServiceAuthorityLevel = "CUSTOMER_SELF_SERVICE"`）；`assemble_customer_amendment.go` 把 `RequestSource` 从 nil 接成它，读解析键登记面 `CommercialResolutionKeyStore`。
  - 端点：`endpoints_customer_amendment.go` 迁 `/v1/shipment-requests/source-data-amendments`（`customerClientFace(SOURCE_DATA_AMENDMENT)`），旧路径出 `customerFaceCommandPatterns`、进 `TestTheUnversionedFirstPartyPathsAreGone`。
  - 文档：[ADR-0311](../../../docs/adr/0311-a-first-party-customers-source-data-amendment-asks-party-commercial-at-the-product-level-customer-self-service-and-the-tenant-controls-it-by-granting-that-level.md)、契约「资料修订」一段（含重放语义）、UC-PS-002 的 `AT-PS-140` 至 `143`、AT 盘点①。
- **本口形状字段（`PAM-1`）**：`accessRequestId`、`originalAccessRequestId`、`shipmentRequestId`（必填）、`dataGroup`、`parcelId`（可选）、`basisVersionId`（可选，缺省即接受基线，仅 `SUPPLEMENT` 合法）、`intent`、`reason`、`requestEffectiveAt`（可选）、内容字段（`postalCode`、`countryCode`、重量与外廓四格，按 `dataGroup` 取用）。摘要纳入原提交标识、委托号、范围、基准、意图、原因、适用时间与内容。
- **对 13 的作者（PC 授予登记入口与演示种子）**：授予的动作 `SOURCE_DATA_AMENDMENT`，**等级逐字 `CUSTOMER_SELF_SERVICE`**，法人与范围必须等于账户解析键行（演示 `SYN-LE-01`、`SYN-SCOPE-01`，见 `resolution-key-syn-account-01.json`）；证据形状 `first-party-amendment:<租户>/<账户>/<来源>/<原提交请求键>#<资料组>[/<包裹>]`（只是询问的一部分，授予不登证据）。允许矩阵演示补格：种子发布批现只登 `SYN-GROUP-CONSIGNEE-ADDRESS` 一格，而首方口的资料组词是 `SENDER_PLACE` / `DELIVERY_PLACE` / `DECLARED_MEASUREMENT`，要让演示客户端能改，需在发布批的 `sourceDataAmendment` 里补这几个资料组的格（阶段 `ACCEPTED_NOT_YET_RECEIVED`，意图 `SUPPLEMENT`、`CORRECTION`、`EXPLICIT_CLEAR` 视演示要走的）。
- **代决与越权风险点**：
  1. 等级取产品常量、证据取被修订委托（ADR-0311 决定一至四，风险点 1 至 5）；证据没有修订请求自己的标识，是因为授权询问不带它而改询问要动 PS 应用层。
  2. `shipmentRequestId` 做成**必填**（票面写「若带」）：范围构造要它，Intake 不反查；不符按找不到答。
  3. 载荷多了 `reason`（必填）与 `basisVersionId`：票面载荷没列，但命令的 `Reason` 必填，更正意图域层要求前序基准（`FormCustomerSourceDataVersion`），否则 500。`basisVersionId` 接收提交回执的 `submissionVersionId` 或前一次修订答复的 `sourceDataVersionId`（都按 `SourceDataVersionID` 构造，域不校验它是否真存在于委托上——**这是域层现状，不是本票引入**）。
  4. 内容字段落不进范围、显式清空带内容在 Intake 就答 400，而不是静默丢字段。
  5. 授权时点取注入时钟：重放时钟不同，授权可能读出不同答复，契约已写。
- **判据对用例**：成功、重放同结果、他账户 404 同字节、委托号不符 404、未授予 403、自报 400、同标识改内容冲突、解析键未登记答授权不可用、无授予答授权规则未配置并在登授予后原样重放形成版本 —— `TestACustomerClientAmendsThroughTheFirstPartyLineAgainstARealDatabase`（`AT-PS-143`）。
- **自验**（含 PG，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）：`go build ./...`、`go vet` 受影响包、`gofmt`；`go test -count=1 -p 1 ./cmd/parcel-api/... ./internal/parcelshipment/... ./internal/accessidentity/... ./internal/architecture/... ./migrations/... ./tools/...` 全绿。`at-coverage-counts -check` 与 `adr-index -check` 只报生成物过期（未手改 `docs/adr/README.md` 与总览计数）。未跑全量。
