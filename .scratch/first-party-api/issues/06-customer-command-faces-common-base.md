# 06 首方客户命令五口的共用底座

Category: enhancement
Status: resolved——2026-10-26 进 main `2188f171e`（见文末「进 main 记录」；各口接通归 07–11）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa06（代码最后一笔 b3a060615）。此前：in-progress——2026-10-26 会话 `tops-20`（Claude Code）立并认领。
Blocked by: 无
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

五口共用的一层：`CustomerClientMinter` 加撤回、取消、修订、补充、索赔五个 Mint 方法骨架与 `CustomerCapability` 五词；`RequestKeyDerivation` 五个推导（命令前缀，取舍 A；含 `DeriveWithdrawalRequestKey`）；一份 access_identity 迁移一次登五词；五口规范化形状与形状版本（ADR-0139 决定八）；提交回执返回提交版本标识（取舍 E，若今天没有）；演示租户五词的授予、准入范围对照与权威区间种子；立 ADR-0310 记取舍 A–F。各口领域 Intake 不在本票（07–11）。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M

## 完成记录（2026-10-26，作者：Claude Code，受 tops-20 委派）

起点 `8537eb000`，分支 `claude-fpa06`，代码一笔 `b3a060615`。

**各层**
- accessidentity：`CustomerCapability` 五词（`customer_client.go`）；`customer_client_command_faces.go` 五个 Mint 骨架与四个新信封类型；`customer_client_envelope.go` 里 `DeriveWithdrawalRequestKey` 改为命令前缀推导、头注改写成现行答法；`customerCapabilitiesAdmittedAtMint` 同收五词（五口都是生产写，铸造侧判准入范围）。`RequestKeyDerivation` 接口不扩（其余四个推导只是 `customerAPIRequestKeys` 的方法，送站预报两口同例，免得租户既有渠道适配族多出四个必须实现的方法）。
- 迁移：access_identity `0024_customer_command_five_faces_capabilities.sql`（取号时 main 与各 `claude-*` 分支均无 0024 及以上）。
- 形状版本：PS domain `customer_command_shapes.go`（`CustomerWithdrawalShapeVersion`、`CustomerParcelCancellationShapeVersion`、`CustomerSourceDataAmendmentShapeVersion`、`CustomerSupplementShapeVersion`），VE domain `customer_claim_shape.go`（`CustomerClaimShapeVersion`）。
- 提交回执：见取舍 E。
- 种子：`data/access/18`（五笔授予）、`19`（五行对照）、`data/governance/53`–`57`（五段权威区间）、`steps/479-access-customer-command-faces.sh`，README 第 10 条；既有各节未改。
- 文档：ADR-0310；ADR-0139 只加 Status 索引注记与 Links；`first-party-api-contract.md` 加一条 `submissionVersionId`（契约补章余下归 12）；UC-PS-001 加 `AT-PS-120`–`124`，盘点①表五行与追记。`docs/adr/README.md` 未碰，需重跑 `go run ./tools/adr-index`；盘点总览计数需重跑 `tools/at-coverage-counts`（两者 `-check` 现都只报生成物过期）。

**五个词 / 方法 / 前缀 / 形状名（给 07–11）**
| 口 | 能力词 | Mint 方法（信封类型） | 键前缀 | 形状名 | 填字段的票 |
|---|---|---|---|---|---|
| 撤回 | `SHIPMENT_REQUEST_WITHDRAWAL` | `MintWithdrawal`（`WithdrawalEnvelope`） | `withdrawal:` | `PWD-1` | 07 |
| 取消包裹 | `PARCEL_CANCELLATION` | `MintParcelCancellation`（`ParcelCancellationEnvelope`） | `parcel-cancellation:` | `PCN-1` | 11 |
| 资料修订 | `SOURCE_DATA_AMENDMENT` | `MintSourceDataAmendment`（`SourceDataAmendmentEnvelope`） | `source-data-amendment:` | `PAM-1` | 10 |
| 补充 | `SHIPMENT_REQUEST_SUPPLEMENT` | `MintSupplement`（`SupplementEnvelope`） | `supplement:` | `PSU-1` | 08 |
| 索赔 | `CLAIM_SUBMISSION` | `MintClaim`（`ClaimEnvelope`） | `claim:` | `VCL-1` | 09 |

**取舍 E 的结论**：核实提交回执原本带 `shipmentRequestId`，**不带**提交版本标识。已补：`SubmitShipmentRequestResult.SubmissionVersionID()`，HTTP 答复 `submissionVersionId`（`omitempty`，随 `shipmentRequestId` 同在同缺，只加不改）。取的是**当前**提交版本（补充基准比对的就是 `CurrentSubmissionVersion()`），重放在补充之后读回补充形成的版本。取消口按 `shipmentRequestId` 指名委托，该键已在回执里。

**代决与越权风险点**（详见 ADR-0310「越权风险点」1–7，编号同）
1. 前缀串是产品词，上线后不能改（今天无已存键）。
2. 「标识在命令内唯一」靠契约不靠结构。
3. 直客 1:1 隔离靠身份构造；取消口应用层不核账户，须 Intake 反查或 PS 补闸（tops-26）。
4. 回执版本标识是「此刻的当前版本」，旧回执补充得基准过期。
5. 索赔自然键的「批次、项」由 VE 领域定义，若领域允许同键多笔，决定六要改。
6. 取消口对没登规则的租户整体不开，实施清单须含登记。
7. 本票代决并写入的另外几点：能力词名（五个字面量）、形状名（五个）、命令前缀串、五词都在铸造侧判准入、`RequestKeyDerivation` 接口不扩、补充形状 `PSU-1` 另立而不沿用 `PSC-1`（08 若认为应复用 PSC-1 的内容构造，改的是 `PSU-1` 的字段定义，不动名）。

**判据对用例**：
- 「首方凭证经 `/v1` 该口提交答成功 / 他账户答找不到 / 未授予 403 / 重放同结果（生产装配真库）」——**本票不满足、也不在本票范围**：它依赖各口领域 Intake 与端点换真（07–11），底座只到封好的信封。本票对应的是：未授予 403、准入范围不符 403、请求键三格、迁移 CHECK、回执版本标识，见下。
- `AT-PS-120`（五口铸造：授予、准入、信封、各格）→ `TestCommandFacesMintTheirEnvelopesFromTheBinding`、`TestCommandFacesAnswerTheirOwnGrades`；
- `AT-PS-121`（前缀、缺席、去空白、跨命令不撞）→ `TestCommandFaceRequestKeysCarryThePrefixAndNeedAnIdentifier`、`TestOneAccessRequestIdentifierGivesDistinctKeysAcrossCommands`、`TestCustomerWithdrawalRequestKeyCarriesTheCommandPrefix`；
- `AT-PS-122`（回执版本标识）→ `TestSubmitReceiptNamesTheCurrentSubmissionVersion`；
- `AT-PS-123`（迁移 CHECK 收五词）→ `TestCustomerCommandFaceGrantsPassTheCapabilityCheckAndRoundTrip`（PG）；
- `AT-PS-124`（形状名）→ `TestCustomerCommandShapeVersionsAreDistinct`。
- 另改了两处旧用例里把 `CLAIM_SUBMISSION`、`SHIPMENT_REQUEST_WITHDRAWAL` 当「未知词」的断言（改用 `UNLISTED_COMMAND` 等），因这两词现已入表。

**自验（含 PG）**：`go build ./...`、`go vet`（accessidentity、parcelshipment、visibilityexception/domain、cmd）、`gofmt` 干净；带 DSN `go test -count=1 -p 1 ./internal/accessidentity/... ./internal/parcelshipment/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/... ./internal/visibilityexception/domain/` 全绿（含 PG 真库）；一次性库 `fpa06_seed` 上 `seed.sh` 退 0，库里十个客户能力授予俱在，用完已 DROP；`tools/at-coverage-counts -check` 与 `tools/adr-index -check` 仅报生成物过期。未跑全量。

**未做 / 留给后续**：五口领域 Intake 与端点换真（07–11）；契约补章（12）；演示租户取消口的授权规则（随 11 登）；`docs/adr/README.md` 与盘点总览计数重跑生成器（协调方进 main 时）。

**评审后补（同分支）**：(1) 提交口 `DeriveSubmissionRequestKey` 对以五个保留前缀之一开头的接入请求标识答推导不出（400）——**这是对已上线提交口的一处收紧**；保留前缀表 `reservedRequestKeyPrefixes` 一处定义、与五口推导共用；送站预报在节点作业自己的册里判重、不同空间，未收紧。用例 `TestSubmissionRequestKeyRejectsReservedCommandPrefixes`。ADR-0310 与契约已补一句。(2) `AT-PS-124` 补 VE 常量断言 `TestCustomerClaimShapeVersionIsNamed`。

## 评审记录

评审 ← tops-20 隔离子代理(sonnet) · 钉 `596d2a764`：阻断无。非阻断：1. 提交口裸标识 `withdrawal:abc` 的键恰等于标识 `abc` 的撤回键（同一判重空间）——修订轮 `23cd17a7d` 收：提交口拒以保留命令前缀开头的标识（400），保留前缀表 `reservedRequestKeyPrefixes` 一处定义；**这是对已上线提交口的一处收紧**，ADR-0310 与契约已写。2. 四个新推导挂在 `customerAPIRequestKeys`、接口未扩，07–11 不能单独注入推导替身——不挡开工。3. 提交回执加 `submissionVersionId` 只加不改；补充之后重放读回补充形成的当前版本，判非阻断（重放语义里现状字段本随时间推进，ADR-0310 越权风险点 4 已披露），补充后重放的用例留给票 08。4. `AT-PS-124` 写五口测四口——修订轮补 VE `TestCustomerClaimShapeVersionIsNamed`。无发现：提交与送站预报推导未变、五前缀互不为前缀、Mint 骨架与送站预报同形、五词一处定义、迁移 0024 存量施加不失败、种子不多登、ADR-0139 只动 Status 注记与 Links、号不撞。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`2188f171e`。出处：分支 `claude-fpa06` tip `bbd582a32`（作者为 Sonnet 子代理）。main 上：`1e04719fa`（代码）、`b7e374de9`（完成记录）、`23cd17a7d`（评审修订）。
- **重放冲突**：AT 盘点文末追记与 code/92、code/36 的追记相邻，两边都留。
- **生成物**：ADR 索引 `43d10ba8d`；AT 盘点总览计数 `3b9dc50ad`；机制清点 `2188f171e`。三件 `-check` 退 0。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `2188f171e`：158 ok / 0 FAIL。**种子**：一次性库灌入退 0，用后删库。
- **判据第 1 条**（经 `/v1` 该口提交成功等）属各口票 07–11，本票只立底座。
