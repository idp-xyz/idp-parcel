# 04 审批职责规则、批准与发布

Category: enhancement
Status: in-progress——**完工，待评审与重放**（2026-10-10 通道 4；分支 `mcp4-pci04`，代码 tip `f3e79282`，基 `e266a876`；其后是登记册、清点与本票面三笔）；完成记录见文末。此前：in-progress——2026-10-10 通道 4 认领（单 task-ea81d298-cf3d-4f0e-adc3-2e4add1af98f，重派 task-d4d0063c），分支 `mcp4-pci04` 基 `e266a876`，工作树 `/home/tops/workspace/idp-parcel-mcp4-pci04`。此前：ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 03（已解：03 resolved，main `f8177a9d`）
地盘：
- `migrations/parcel_pricing/`；
- `internal/parcelpricing/`：审批职责规则、操作者主体消费面、批准与发布编排及 HTTP；
- `cmd/parcel-api/`；
- `docs/product/PILOT-PARAMETER-REGISTER.md` 一行；
- `docs/product/MECHANISM-INVENTORY.md` 重生成。
出处：[spec](../spec.md) 自决第 2 格；ADR-0101 决定五、六；ADR-0126 决定三。

## 要做的

1. **审批职责规则**（PP 自有，按租户）：录入者与批准者须否为不同主体、批准者须持哪一格授予。
   - 立读口与表；写口只给测试用。
   - 两格都不要求也是一条合法的租户声明。
2. **操作者主体消费面**：主体引用加授予集，由 Intake 把信封译成它。形状同 PC 的 `OperatorSubject`，各上下文各立一个。
3. **批准**：只接 `已校验`。
   - 先读审批职责规则：未登记即答`未配置`、不放行；主体相同或授予不足各答一格。
   - 通过后记批准者与批准时刻，转 `已批准`。
4. **发布**：只接 `已批准`。
   - 用草稿上的方案快照、源文件身份、方向授权引用，加上作为 `publicationApprover` 的批准者主体，立登记，交既有 `RegisterPriceCard`。
   - 不重解析、不重算（ADR-0101 决定四）。
   - 答案代数一格不改；落定后草稿转 `已发布`。
5. **端点**：`POST /pricing-price-card-draft-approvals` 与 `POST /pricing-price-card-draft-publications`，挂 `UnconfiguredIntake{}`。载荷只收草稿引用。（2026-10-10 通道 4：照 spec 自决第 4 格 2026-09-25 更正改挂操作者渠道的登记册 Intake，依据见判断项 1；原句留痕。）
6. **参数登记册**增一行「价卡发布审批职责规则」：实例半边，租户取值留空，答`未配置`。

## 验收

- 批准门每一格有测试：未配置、主体相同、授予不足、通过、状态不对。
- 发布路径有测试：交给登记用例的摘要与草稿上的逐字节相等；登记用例的每格答复原样交回。
- Postgres 适配器带 DSN 测试；端点表测试含新行；机制清点重生成。

## 形态

碰 Go、SQL 与端点表，走并行会话那条路。

## 完成记录（2026-10-10，通道 4；分支 `mcp4-pci04` 基 `e266a876`，代码 tip `f3e79282`；待非作者评审与重放）

派单 `task-ea81d298`；证据层级 `S`（合成替身与 `SYN-` 夹具，演示租户 `SYN-TENANT-01`）。进 main 的 SHA 由重放那一笔换，这里记分支上的：

- `1ca18b67` 认领；
- `d5d79429` 领域、端口与编排：`OperatorGrant` / `OperatorSubject`、`PriceCardApprovalDutyRule`、`PriceCardDraft.Approve` / `Registration` / `MarkPublished`；端口 `PriceCardDraftProgress`、`PriceCardApprovalDutyRuleView` / `Registry`；编排 `ApprovePriceCardDraftHandler`、`PublishPriceCardDraftHandler`；
- `087b4185` 迁移 `parcel_pricing/0012_price_card_approval_duty_rule.sql` 与 Postgres 适配器：`PriceCardApprovalDutyRules`，`PriceCardDrafts` 加 `LoadPriceCardDraft` / `AdvancePriceCardDraft`（0012 落在既有 `parcel_pricing` 目录，整目录嵌入，共享接线文件不动）；
- `f3e79282` HTTP 两口与装配：`NewApprovePriceCardDraftEndpoint` / `NewPublishPriceCardDraftEndpoint`、`OperatorRegistryIntake` 两口译法与 `UnconfiguredIntake` 两口未配置译法、`OperatorIdentity.Grants` 与身份翻译层的授予集、`cmd/parcel-api` 端点表两行与 `buildPriceCardDraftApproval` / `buildPriceCardDraftPublication` 两只事务包装；编排结果改为导出字段（照本包 `SubmitPriceCardDraftResult`）；
- `3291b79c` 参数登记册 `PAR-SET-12`「价卡发布审批职责规则」；
- `d8e8bb68` 机制清点在 `3291b79c` 的干净检出上重生成（parcelpricing 生产 119→124、测试 108→112；parcel_pricing 迁移 11→12；接入面端点 137→139；端口声明 457→460，基线口径多缺的一格是 `PriceCardApprovalDutyRuleView`，清点自标「虚低：精确口径已实现」）；
- 本票面随其后一笔。

验收逐条：

- **批准门每一格有测试。** 未配置：`TestApprovalAnswersNotConfiguredWhileTheTenantHasNoRule`（草稿一字不动）。主体相同、授予不足、通过：领域 `TestApprovalIsGatedByTheApprovalDutyRule`（另含「两格都不要求时录入者自批放行」）与编排 `TestApprovalGateAnswersEachCellOfTheRule`。状态不对：领域 `TestOnlyAValidatedDraftCanBeApproved`、编排 `TestApprovalAnswersFromTheCellTheDraftIsIn`（`草稿`答 `DRAFT_NOT_VALIDATED`，`已批准` / `已发布`各答其格，三格都不读规则）。另有并发被替换答 `DRAFT_CHANGED`、依赖读不动上抛（`TestApprovalThatLosesTheRaceOrItsDependenciesIsNotAnApproval`），HTTP 每格取 200（`TestTheApprovalEndpointAnswersEveryCell`）。
- **发布路径有测试。** 摘要逐字节相等：`TestPublishingHandsTheApprovedDraftItselfToTheRegistration` 拿交给登记用例的那一份比录入时算出的内容摘要与规范化版本，源文件身份、方向授权引用照抄，`publicationApprover` 是批准者而不是录入者；领域 `TestTheRegistrationIsTheApprovedDraftItself` 同证。每格答复原样交回：`TestEveryRegistrationAnswerIsHandedBackAsIs` 逐格演 `RECORDED`、`ALREADY_REGISTERED`（落定、草稿转`已发布`）、`CONTENT_CONFLICT`、`CANONICALIZATION_DIFFERS`（未落定、草稿留在`已批准`）与 `UNDECIDED`（原因随错误上抛），HTTP 侧 `TestThePublicationEndpointHandsTheRegistrationAnswerThrough` 原名照交。`NOT_ACCEPTED` 经草稿到不了：已批准草稿交出的登记必非零值，未单测。
- **Postgres 适配器带 DSN；端点表含新行；机制清点重生成。** `TestApprovalAndPublicationAdvanceTheDraftRow`、`TestAnAdvanceFromAStaleReadIsSuperseded`、`TestTheProgressWritesNeedATransaction`、`TestThePriceCardApprovalDutyRuleIsRegisteredOncePerTenant`；`businessEndpointProbes` 与换口名单各加两行，`TestEveryAssembledEndpointAnswersUnconfigured`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel` 按表断言；装配点 `TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase` 在真库上走「规则没登答未配置 → 登规则后换人批准 → 发布入册 RECORDED → 再发布答已发布」。清点见 `d8e8bb68`。

各层用例先于实现写下；没有单独记录红跑——实现落地前这些用例引用的符号都不存在，编译即红。没有做变异复跑。

判断项（标「越权风险点 · 待 PP owner 复核」的是照先例自裁、票面没写定的格）：

1. **两口挂 `operatorRegistries.pricing`，不挂票面第 5 条写的 `UnconfiguredIntake{}`。** 依据：spec 自决第 4 格 2026-09-25 更正（「本批各口因此直接挂这个 Intake……票 02 起照此办」）；03 已照此办且通道 1 2026-10-09 同意；operator-channel/04 归类表写明价卡导入这一批的口归 price-card-import、并说 spec 第 4 格与它一致，同票第 2 件判断项 2 也写明批准者取自信封走的是本票这条主路径。批准门要的主体与授予集只有操作者渠道交得出，挂未配置 Intake 则批准门永远走不到。两头只做了这一头。
2. **授予集的词汇与来源。越权风险点 · 待 PP owner 复核。** PP 的授予是不透明的名字（`OperatorGrant`），只比相等；身份翻译层把信封里持有的能力面逐格译成授予名（`grantableFaces`：登记册配置写、主数据与运营查阅读；运营决定一格按决定种类另答、不进 `Holds`，不在其中）。规则要求的授予若不是能力面名，经操作者渠道恒答`批准者不合格`——要表达 spec 举的「定价主管那一级」这类租户自定等级，得等接入身份能力的授权模型扩到等级，归 ADR-0100 那一族与 ADR-0085 决定四另裁，不在本票。PC 那一条用的是商业权限等级，且其 Intake 译法尚未接；两边词汇不同在 spec 自决第 2 格「形状同、不共用」之内。
3. **「状态不对」分三格答，且不读规则。越权风险点 · 待 PP owner 复核。** `草稿` → `DRAFT_NOT_VALIDATED`，`已批准` → `DRAFT_ALREADY_APPROVED`，`已发布` → `DRAFT_ALREADY_PUBLISHED`。照 PC `ApprovePublicationDraftHandler` 的分格；PP 多`草稿`一格是因为 PP 生命周期有四态，三种情形恢复动作不同。
4. **发布的答案代数。越权风险点 · 待 PP owner 复核。** `DRAFT_PUBLISHED`（登记答 `RECORDED` 或 `ALREADY_REGISTERED` 即算落定）/ `PUBLICATION_NOT_LANDED`（其余各格，登记那一格随结果原名交回）/ `DRAFT_NOT_APPROVED` / `DRAFT_ALREADY_PUBLISHED`（不重交登记）/ `DRAFT_NOT_FOUND`。登记 `UNDECIDED` 时编排连原因上抛、整笔回滚，HTTP 答 `NO_ANSWER_FORMED`，同登记口。登记答案代数一格未改（ADR-0101 决定五）。照 PC `PublishPublicationDraftHandler` 的形；PP 没有生效边界那一格。
5. **并发不加锁。** 推进口按「前一格 + 录入者 + 录入时刻 + 内容摘要」做条件 UPDATE，推进成发布时另核批准者与批准时刻；判不上答`已被替换`、行一字不动。批准据此答 `DRAFT_CHANGED`；发布在登记落定后草稿跟不上即上抛，登记写入与草稿推进同一笔事务，整笔回滚。
6. **批准、发布各一笔事务，时刻取系统时钟。** 判据同 `transactionalPriceCardDraftSubmission`：落点是业务答案就提交，返回错误整笔回滚。
7. **状态码。** 批准每格 200（改的是已有那一行，不新落行）；发布登记新落一版取 201，其余 200；载荷封闭只收 `planId`、`planVersion`，夹带身份格按未知键拒。
8. **审批职责规则只立读口、表与只给测试用的写口**，同 ADR-0126 决定五的形；登记面（CLI 或管理台）归治理写面那一族另裁。
9. **无新 ADR**，未改任何已接受 ADR 的决定，没有占 0177。

没做的、留给后续：

- 审批职责规则的登记面（同判断项 8）；租户上线时登 `PAR-SET-12` 那一行是实施的事。
- parcel-pricing `CONTEXT.md` 没加「价卡发布审批职责规则」词条：地盘不含 CONTEXT，概念权威在 ADR-0101 决定六与 spec 自决第 2 格；PC 那一条在 PC CONTEXT 有词条，要不要对齐由 PP owner 定。
- 06（管理台草稿签）等本票进 main；管理台调这两口时载荷只带 `planId`、`planVersion`。

验证（钉 `3291b79c` 的干净 detached 检出，含 PG）：

- `gofmt -l internal/parcelpricing cmd/parcel-api` 零行；`go build ./...` 与 `go vet ./...` 全仓退 0。
- `go list -test` 反查依赖本票动过的 parcelpricing 六个包、`migrations` 或 `cmd/parcel-api` 的全部包，共 62 个（其中 `cmd/*` 14 个），加 `./internal/architecture/...`，带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` 跑 `go test -count=1 -p 1`：退 0，59 ok / 4 无测试 / 0 FAIL。
- `-v` 单跑 `cmd/parcel-api` 的 `TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase` 与 `adapters/postgres` 的 `TestApprovalAndPublicationAdvanceTheDraftRow`：均 `--- PASS`，非 SKIP。
- 清点在同一检出上重生成，见 `d8e8bb68`。
- 未跑全仓 `go test ./...`，留推送方重放后那一跑。
