# 08 补充口经首方渠道接通

Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-fpa08`（代码最后一笔 `dcc1c8a86`）。此前 in-progress——2026-10-26 会话 `tops-20` 认领（06 已进 main），分支 `claude-fpa08`；此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Status: in-progress——2026-10-26 会话 `tops-20` 认领（06 已进 main），分支 `claude-fpa08`。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 06
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeSupplement`、基准版本取自载荷带回的提交版本标识（取舍 E）、`endpoints_customer_supplement.go` 换真 Intake；他账户答找不到。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M

## 完成记录（作者）

起点 `4292299b8`；代码 `dcc1c8a86`。

**改了什么**：`CustomerSupplementIntake`（`internal/parcelshipment/adapters/http/customer_supplement_intake.go`）与认证适配 `CustomerSupplementAuthenticator`（`MintSupplement`）；`cmd/parcel-api/endpoints_customer_supplement.go` 换真 Intake、路径迁 `/v1/shipment-requests/supplements`、face 换 `customerClientFace(SHIPMENT_REQUEST_SUPPLEMENT)`、`ownIntake`；装配 `buildCustomerSupplementIntake`；领域 `CanonicalizeCustomerSupplement`（PSU-1，与 PSC-1 共用一份编码，版本名进文档与摘要前缀）；契约文档补章；AT-PS-130..133 入 UC-PS-001 与盘点①。**无 PS 应用层改动**（`FormNewSubmissionVersion` 编排原样）。

**本口形状 PSU-1 字段**：提交载荷的内容字段（`customerShipmentReference`、`requestedServiceProduct`、`requestEffectiveAt`、寄收件关系 / 地址 / 邮编 / 国家码、`destinationServiceScope`、`parcels[]`）整份重报；另 `accessRequestId`（补充自己的标识，键 `supplement:`）、`originalAccessRequestId`（指名委托）、`shipmentRequestId`、`basisSubmissionVersionId`；`parcels[]` 每件另带 `parcelId`。封闭解码，自报账户 / 租户 / 来源即未知字段 400。摘要的基准段 = `basisSubmissionVersionId`。

**代决与越权风险点**
1. 原提交身份 = 信封的租户 / 账户 / 来源（客户端主体）+ 载荷里原接入请求标识。只能指到同账户、同一客户端提交的委托；同账户不同客户端提交的委托补不了（与提交重放同口径，单客户端直客 1:1 下无影响）。
2. 成员集合取自载荷逐件带回的回执 `parcelId`（编排比对的是内部成员标识，Intake 不查库、不改应用层）。客户可以乱填，但乱填只会落在 `SUPPLEMENT_BOUNDARY_CHANGED`，不会改到别人的委托（委托先按信封账户找）。
3. 载荷里的 `shipmentRequestId` 未经核对，只进编排未决时的续办引用；填错只让该客户自己拿到对不上的引用。若要根除，须让编排从找到的委托里取该号（PS 应用层改动，归 tops-26），本票未动。
4. 补充在铸造侧判准入范围（`SHIPMENT_REQUEST_SUPPLEMENT` 在 `customerCapabilitiesAdmittedAtMint` 内，种子已登），Intake 不预取归属。
5. AT 号取 130..133（避开 07/10/11 可能同取 125 起的撞号），协调方进 main 时如需收拢请自行改号。
6. 旧无前缀路径 `/shipment-requests/supplements` 不并挂，装配后答 404（`first_party_contract_test.go` 的两张表已随改）。

**判据对用例**：成功 / 重放 / 他账户 404 同字节 / 未授予 403 / 自报账户 400 / 基准过期 / 成员对不上 / 补充后原提交重放读回新版本 = `TestACustomerClientSupplementsThroughTheFirstPartyLineAgainstARealDatabase`（AT-PS-132、133，生产装配真库）；Intake 译码 = AT-PS-130；PSU-1 摘要 = AT-PS-131。种子未改（五词授予与准入已登），判据「种子 `seed.sh` 退 0」未重跑。全量 `scripts/test-full.sh` 未跑（协调方进 main 时跑）。

**自验（含 PG）**：`go build ./...`、`go vet`（cmd/parcel-api、parcelshipment、accessidentity）、`gofmt` 干净；带 DSN `go test -count=1 -p 1 ./internal/parcelshipment/... ./internal/accessidentity/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...` 全绿（含 PG 真库）；`at-coverage-counts -check` 只报生成物过期。
