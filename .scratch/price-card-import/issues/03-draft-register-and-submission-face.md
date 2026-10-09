# 03 草稿册、录入口与草稿查阅读口

Category: enhancement
Status: in-progress——**完工，待评审与重放**（2026-10-09 13:1x 通道 2；分支 `mcp2-pci03` 基 `187dccd2`，代码 tip `034b80ce`，其后只有本票面收口笔；完成记录见文末，非作者评审由通道 1 另派）。此前：in-progress——2026-10-08 通道 2 认领（单 task-23300dc1-8f6a-4e3e-a8da-f8b2ff450f7a），分支 `mcp2-pci03` 基 `187dccd2`，工作树 `/home/tops/workspace/idp-parcel-mcp2-pci03`。此前：ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 02
地盘：
- `migrations/parcel_pricing/`：新模块；
- `internal/parcelpricing/`：草稿领域件、端口、Postgres 适配器、录入编排与 HTTP；
- `cmd/parcel-api/`：装配与端点表；
- `docs/product/MECHANISM-INVENTORY.md` 重生成。
出处：[spec](../spec.md) 自决第 1 格；ADR-0101 决定三；ADR-0126 决定三（一版一行、重放、修订、内容已固定）。

## 要做的

1. **草稿册**：键为租户 × 方案标识 × 方案版本，一版一行。行上记：
   - 状态（spec 自决第 1 格的四格）；
   - 源文件身份；
   - 方向授权引用；
   - `已校验` 起才有的方案快照与规范化摘要；
   - `草稿` 时的逐格问题；
   - 录入者主体（取自信封）与录入时刻。
   草稿不进版本清单，任何评价读不到它。
2. **持久化形状留在领域**。方案快照复用登记文档那组折装函数，不另立第二个口径。草稿与已发布版本是两条持久化路径，所以可以有自己的一扇门（判据见 `plan_snapshot.go` 文件头注）。
3. **录入编排**：与 02 的预览同一段解码。
   - 同版同内容再录是重放。
   - `已批准` 之前换内容是修订，替换那一行。
   - `已批准` 之后答内容已固定。
   - 连方案身份都读不出来的文件不落行，答未受理带问题。
4. **端点**：`POST /pricing-price-card-drafts`，另加一个草稿查阅读口（按租户列出，可按状态筛）；两口挂哪个 Intake 以 [spec](../spec.md) 自决第 4 格为准（含其 2026-09-25 更正）。查阅读口的路径与方法照端点表现行约定取。此处原写「两者都挂 `UnconfiguredIntake{}`」，与 spec 的更正成了两套口径；2026-10-09 经通道 1 同意，照票 02 先例改为只引那一格。

## 验收

- 领域与编排单测覆盖：
  - 四格状态的进入条件；
  - 重放、修订、内容已固定；
  - 未受理不落行。
- Postgres 适配器带 DSN 测试：一版一行、替换、读回整图重验。
- 端点表测试含新行；机制清点重生成。

## 形态

碰 Go、SQL 与端点表，走并行会话那条路。

## 完成记录（2026-10-09，通道 2）

分支 `mcp2-pci03` 基 `187dccd2`，工作树 `/home/tops/workspace/idp-parcel-mcp2-pci03`。代码 tip `034b80ce`，其后只有本票面收口笔。真正进 main 的 SHA 由重放那一笔换，本记录记分支上的：

- `89fee819` 草稿册：领域行 `PriceCardDraft`（状态四格）、端口 `PriceCardDraftRegister` 与 `PriceCardDraftRead`、录入编排 `SubmitPriceCardDraftHandler`、迁移 `migrations/parcel_pricing/0011_price_card_draft.sql`、Postgres 适配器 `PriceCardDrafts`；
- `e11c1cda` HTTP 两口：录入口 `NewSubmitPriceCardDraftEndpoint`（`POST /pricing-price-card-drafts`）、草稿查阅读口 `NewQueryPriceCardDraftsEndpoint`（`GET /pricing-price-card-draft-views`）、操作者渠道译法 `IntakePriceCardDraftSubmission` 与 `IntakePriceCardDraftQuery`、`UnconfiguredIntake` 补两口的未配置译法；
- `a5f6d87c` 端点表与装配：两行挂 `operatorRegistries.pricing`，`buildPriceCardDraftSubmission` 接真读口、真库草稿册与事务包装，`main` 另立一个草稿册适配器作查阅读口；
- `a6d1462d` 机制清点在 `a5f6d87c` 的干净检出上重生成（parcelpricing 生产 113→119、测试 103→108；parcel_pricing 迁移 10→11；接入面端点 135→137）；
- `034b80ce` `OperatorRegistryIntake` 头注改为「价卡导入那一批的各口也挂它」，不再逐口列。

验收逐条：

- **四格状态的进入条件。** `TestASubmissionWithProblemsEntersDraft`、`TestASubmissionWithContentEntersValidated`：带逐格问题进`草稿`、带内容进`已校验`；`TestAnIncoherentSubmissionIsNotADraft` 钉两样都带或都不带立不成行。`已批准`与`已发布`只经重建门读回（`TestApprovedAndPublishedDraftsCarryTheirTraces`），录入立不出这两格；库上的 CHECK 镜像同一条（`TestTheDraftTableRefusesRowsNoPathCouldWrite`）。
- **重放、修订、内容已固定。** `TestResubmittingTheSameVersionReplaysRevisesOrMeetsFixedContent`（领域）与 `TestResubmissionsAnswerReplayRevisionOrFixedContent`（编排）钉三格；Postgres 侧 `TestADraftVersionIsOneRowAndTheSameContentReplays`、`TestARevisionReplacesTheRowBeforeApproval`、`TestAnApprovedOrPublishedDraftHasFixedContent`（后两格用一条 UPDATE 造出，批准与发布的写口归票 04）。装配点 `TestTheWiredPriceCardDraftSubmissionRecordsAgainstARealDatabase` 在真库上钉首录落一行`草稿`、同一份字节再录答重放——重放读得到首行即证首录那笔事务提交了。
- **未受理不落行。** `TestAFileWithoutAPlanIdentityIsNotAcceptedAndLeavesNoRow`：连方案身份都读不出来的文件答未受理、册上没有行；HTTP 侧 `TestANotAcceptedSubmissionAnswersTheReadingWithoutARow` 答 200 且不带 `draft`。
- **Postgres 适配器带 DSN。** 一版一行、替换、读回整图重验与比对列交叉核在上面三笔里；`TestTheDraftListRefusesARowThatWasTamperedWith` 钉比对列与内容文档分岔时读口交回错误；`TestTheDraftListIsPerTenantNewestFirstAndFiltersByStatus` 钉按租户列出、按录入时刻倒序、按状态筛。
- **端点表测试含新行。** `businessEndpointProbes` 加 `/pricing-price-card-drafts`（POST）与 `/pricing-price-card-draft-views`（GET）两行，`TestEveryAssembledEndpointAnswersUnconfigured` 按该表逐行断言。
- **机制清点重生成**，见 `a6d1462d`。

判断项：

- **两口挂操作者渠道的登记册 Intake，不挂 `UnconfiguredIntake{}`。** 票面第 4 条原写后者，与 spec 自决第 4 格的 2026-09-25 更正（「本批各口因此直接挂这个 Intake……票 02 起照此办」）和 ADR-0101 Consequences（「对应 HTTP 端点全部挂 ADR-0100 的操作者 Intake」）成了两套口径。2026-10-09 通道 1 回信同意照票 02 先例把第 4 条改为只引 spec 那一格，本笔已改。查阅读口因此也不走查阅行的 Intake 变量：只持查阅授予的答未授予（`TestThePriceCardDraftViewsAnswerFromTheOperatorChannel` 的「read grant only」那一格）。
- **查阅读口路径取 `/pricing-price-card-draft-views`、方法 GET。** 端点表一个路径只挂一个方法，`/pricing-price-card-drafts` 已是录入口，照 `/shipment-requests` 与 `/shipment-request-views` 的先例另立 `-views`。
- **录入口只有写下了册上那一行的两格带 `draft` 并取 201。** 重放与内容已固定答 200 且不带行：两格里应用层交回的是本次拟录的那份而不是册上的，重放可落在已批准的行上，带出去就把一行已批准的草稿答成了`已校验`。册上此刻是什么由查阅读口答。
- **查阅读口的页大小取 200，端口不要翻页。** 页大小是操作者渠道契约的一格（ADR-0100，渠道归产品），不是租户取值；一个租户在途的价卡草稿以「版」计，量级远在这个数之下。查询串只认 `status` 一格，别的键（含调用方自带的 `limit`）一律拒。
- **录入编排整段包进一笔事务，含读模板那一步。** 判据同 `transactionalPriceCardRegistration`：事务边界归装配点，落点是业务答案就提交，返回错误整笔回滚。代价是解析期间占着一个库连接；上传是偶发的操作者动作，不为此把事务边界拆进适配器。

超出票面的，照实记：

- **装配测试三处**不在验收所列：`swappedRegistryFaces` 加录入口一行、`TestThePriceCardDraftViewsAnswerFromTheOperatorChannel` 钉 GET 口五格、`TestTheWiredPriceCardDraftSubmissionRecordsAgainstARealDatabase` 在真库上钉事务包装。三处各做过一次变异（两行端点各换成 `UnconfiguredIntake{}`、录入改走事务外的 ctx），都红，已还原。
- **`034b80ce` 只改一处头注**，见上。

验证（2026-10-09 13:0x，钉 `034b80ce` 的干净 detached 检出 `/tmp/idp-verify-pci03-89fee819`）：

- `go build ./...` 与 `go vet ./...` 全仓退出 0；`gofmt -l .` 零行；机制清点在该检出上重跑零差。
- 带 `IDP_PARCEL_POSTGRES_DSN`（`127.0.0.1:55432`，门禁容器 `idp-parcel-postgres-gate`）`go test -count=1 -v` 跑 `./cmd/parcel-api/`、`./internal/parcelpricing/adapters/accessidentity/`、`./internal/parcelpricing/adapters/http/`、`./internal/architecture/...`：4 包 `ok`，236 PASS / 0 FAIL / 0 SKIP。探针 `TestTheWiredPriceCardDraftSubmissionRecordsAgainstARealDatabase` 的 `-v` 为 PASS。
- 此前 `89fee819` 那一跑（同口径，含 DSN）覆盖了本票动过的 4 个 parcelpricing 包（domain、application、ports、adapters/postgres）与 migrations 的全部反向依赖（含 14 个 `cmd/*`）：59 `ok` / 0 FAIL / 4 无测试，唯一 SKIP 是 `pgtest` 的子进程辅助用例。`89fee819..034b80ce` 之间的改动都在 parcelpricing 的 http 适配器与 `cmd/parcel-api`，两处都在本轮范围内。

给 04 的交接：草稿册的推进口还没有——`已批准`与`已发布`两格今天只能经重建门读回，测试里用一条 UPDATE 造出。批准要落的是批准者与批准时刻（`PriceCardDraft.Approver`、`ApprovedAt`），发布落发布时刻；两格的痕迹缺一，重建门就拒。审批职责规则按 spec 自决第 2 格由 parcel-pricing 自有一条、按租户，写口只给测试用。录入者已记在行上，取的是认证出的 `OperatorIdentity.Operator`。

## 判断项（2026-10-09，通道 2；通道 1 同日回信同意前两条的做法）

- 票面第 4 条改为只引 spec 自决第 4 格，两口挂 `operatorRegistries.pricing`。依据与对照见上「完成记录」的判断项第一条。
- 查阅读口取 `GET /pricing-price-card-draft-views`，见判断项第二条。
- 录入口的 `draft` 行只在 `DRAFT_SUBMITTED` 与 `DRAFT_REVISED` 两格出现，见判断项第三条。
- 查阅读口页大小 200、查询串封闭，见判断项第四条。
- 录入的事务边界包住整段编排，见判断项第五条。

## Comments

（合入前非作者评审由通道 1 另派，评完写在这里。）
