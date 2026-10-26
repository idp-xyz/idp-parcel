# 41 PP 计价参考序列与目录的人工复核写死复核者不得是登记者，照 ADR-0262 改为读租户的参考复核职责规则

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code41（2026-10-25）。此前 ready-for-agent——R2（上线后尽快）：单人运营的租户手工登记的序列与目录一版都进不了在用，评价停在待判断；同一租户的价卡批准却可以登成单人可批。绕法：由第二个操作者复核（多人租户照常可用）；单人租户没有绕法，只能改走连接器并声明免人工复核。2026-10-21 文档自洽 07 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc07。
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) B2-05（审计归 B，由 docs-consistency/07 立 [ADR-0262](../../../docs/adr/0262-reference-series-and-catalogue-review-duty-separation-is-a-tenant-governance-rule.md) 代决）。

## 现象

序列复核与目录复核的领域构造门写死复核者不得与登记者相同，不读任何租户规则；规则缺席时也不答未配置。

## 证据

- 代码：
  - `internal/parcelpricing/domain/reference_series_review.go` 的 `NewSeriesReview`：`reviewer == registration.registrant` 即 `ErrSeriesReviewerIsRegistrant`。
  - `internal/parcelpricing/domain/reference_catalogue_review.go` 的 `NewCatalogueReview`：同一判法，`ErrCatalogueReviewerIsRegistrant`。
  - `internal/parcelpricing/application/review_reference_series.go`、`register_reference_catalogue.go`：两只编排把它译成 `NEEDS_ANOTHER_REVIEWER`，没有未配置一格；复核命令只带复核者主体，不带操作者授予。
  - 对照：`internal/parcelpricing/domain/price_card_draft_approval.go` 的 `PriceCardApprovalDutyRule`（不同主体一格、要求授予一格）与 `ports.PriceCardApprovalDutyRuleView`，规则未登记时批准门答未配置。
- 文档：ADR-0262 决定一；PP CONTEXT「序列版本复核」词条；UC-PP-001 `AT-PP-014`、`AT-PP-015`。

## 三类

机制：复核门按规则判。产品策略：规则的形状与「缺省朝拦」（ADR-0262 已定）。租户取值：某个租户要不要不同主体、复核者须持哪一格授予。

## 规模

M：领域规则类型、读口与表各一，两只编排各接一次，两格新结果，种子一条，用例。

## 最小的诚实修法

1. 领域：新增参考复核职责规则（一租户一条：须否不同主体、可缺的要求授予），形状照 `PriceCardApprovalDutyRule`；`NewSeriesReview`、`NewCatalogueReview` 改为收规则与复核者的授予集，按规则判，去掉写死的比较。
2. 端口与持久化：读口（按租户读，`found=false` 即未登记）与写口——写口给受控 CLI、装配与测试用，有意不照 `PriceCardApprovalDutyRule` 的先例（那一套没有 CLI 登记口）；新迁移一张表（`parcel_pricing` 下接现有最大号）。
3. 编排：两只复核编排先读规则，未登记答新的 `NOT_CONFIGURED`、要求授予而复核者没有答新的复核者不合格一格，`NEEDS_ANOTHER_REVIEWER` 只在规则要求不同主体时给出；三格都不落复核行。复核命令从操作者信封带上授予集。
   - 受控 CLI（`cmd/parcel-pricing-register -kind reference-series-review` 与 `-kind reference-catalogue-review`）走同一编排、同一道门，不绕过规则：复核者取批文的 `reviewer` 串，授予集为空；规则要求不同主体时照比登记者，要求授予时一律答复核者不合格（ADR-0262 越权风险点 4）。
   - 规则的登记口：`parcel-pricing-register` 增一种登记（例如 `-kind reference-review-duty-rule`，批文 `{"tenant", "distinctSubjects", "requiredGrant"}`），一租户一条、同内容重放、异内容答冲突；在线登记面随治理写面那一族另裁。
4. 免人工复核的代写复核（协调方 2026-10-21 裁定）：今天 `application/feed_reference_series.go` 复用同一只 `ReviewReferenceSeriesHandler`，复核者为 `binding.ConnectorIdentity()`，同样受不同主体比较。改法：`ReviewReferenceSeriesCommand` 加一格「免复核声明下的系统代写」，只由 `feed_reference_series.go` 在免复核分支上置位；带这一格时编排跳过规则读取（规则未登记也不答 `NOT_CONFIGURED`），写死的不同主体比较照做（登记者等于连接器身份即 `NEEDS_ANOTHER_REVIEWER`）。操作者面与受控 CLI 的复核口不得置这一格（载荷里出现即拒）。
4a. 目录复核的操作者面：`cmd/parcel-api` 今天没有目录复核端点，目录复核只有受控 CLI，规则要求授予的租户无法复核目录（ADR-0262 越权风险点 4）。本票一并补 `/pricing-reference-catalogue-reviews`，形照 `/pricing-reference-series-reviews`（登记册配置写族、复核者与授予集取自操作者信封）；不另立票。
5. 种子：`scripts/demo-seeds/steps/150-pricing.sh` 在序列与目录复核之前，经上面的 CLI 登记口给 `SYN-TENANT-01` 登一条要求不同主体、不要求授予的规则（`SYN-` 数据文件放在 `data/pricing/`）；种子的登记者 `SYN-PRICING-OPS-01` 与复核者 `SYN-PRICING-REVIEW-01` 本就不同，演示动线不退成未配置、行为不变。
6. 用例：`AT-PP-014`、`AT-PP-015`（含经受控 CLI 复核而规则要求授予时一律答复核者不合格那一支）、`AT-PP-016`（代写复核）的领域、编排与真库用例，目录操作者面端点的 HTTP 用例；`.scratch/at-coverage-inventory.md` 把 `AT-PP-014`、`015` 从④移入①、`AT-PP-016` 从②移入①（代写那一格今天的行为已对，要守的是 code/41 落地后不回归）。

## 要裁的

无，已由 ADR-0262 裁定。登记面（治理写面）照 `PriceCardApprovalDutyRule` 的先例另裁，不在本票。

## 完成记录（2026-10-25，分支 claude-code41）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 PP owner 作出，待复核。基线 main `061156bcc`，作者自验期间 main 未前移。

### 做了什么（对「最小的诚实修法」逐条）

1. **领域**：`domain.ReferenceReviewDutyRule`（一租户一条：须否不同主体、可缺的要求授予；形照 `PriceCardApprovalDutyRule`，复核者用同一套 `OperatorSubject`、`OperatorGrant`）。`NewSeriesReview`、`NewCatalogueReview` 改收复核者主体与规则、按规则判，写死的比较去掉；授予不合格新哨兵 `ErrReferenceReviewerLacksRequiredGrant`（序列与目录共用），`ErrSeriesReviewerIsRegistrant`、`ErrCatalogueReviewerIsRegistrant` 只在规则要求不同主体时给出；他租户的规则答 `ErrReferenceReviewDutyRuleTenantMismatch`（装配错，不是业务答案）。
2. **端口与持久化**：`ports.ReferenceReviewDutyRuleView`（`found=false` 即未登记）与 `ReferenceReviewDutyRuleRegistry`；迁移 `parcel_pricing/0014_reference_review_duty_rule.sql`（取号前核过 main 最大 0013、`git branch -a` 在途分支无一占 0014）；`adapters/postgres.ReferenceReviewDutyRules`，一租户一条、同内容重放、异内容冲突、原行不被顶替，插入与撞键后读回走同一笔环境事务；无事务即拒的负面证据进 `transaction_guard_test.go`（PBC-08 门禁认得）。规则登记另有应用层用例 `RegisterReferenceReviewDutyRuleHandler`。
3. **编排**：两只复核编排先读登记、再读规则：未登记答 `NOT_CONFIGURED`，要求授予而复核者没有答 `REVIEWER_NOT_QUALIFIED`，`NEEDS_ANOTHER_REVIEWER` 只在规则要求不同主体时给出；三格都是确定性业务答案（`nil` 错误、不进任何未决哨兵表），都不到复核册、不交「参数已登记」。复核命令带 `ReviewerGrants`，操作者面从信封译来，受控 CLI 留空。装配漏了规则读口答未决带 `ErrReferenceReviewDutyRuleViewMissing`。
   - 受控 CLI 两种复核走同一编排同一道门：要求不同主体时照比登记者，要求授予时一律 `REVIEWER_NOT_QUALIFIED`（退出码 2）。
   - 规则登记口：`parcel-pricing-register -kind reference-review-duty-rule`，批文 `{"tenant","distinctSubjects","requiredGrant"}`；入册与重放 0、冲突 2、坏文档 1、未决 3。
4. **代写复核**：`ReviewReferenceSeriesCommand` 加不导出的一格 `delegatedUnderExemption`，只有 `feed_reference_series.go` 的免复核分支置位；带这一格时不读规则（规则未登记也不答未配置），领域走 `NewExemptionDelegatedSeriesReview`，写死的不同主体比较照做。操作者面载荷与 CLI 复核文档里出现同名键 `systemDelegatedUnderExemption`（或任何未知键）即拒，各有用例。
   - 4a **目录复核操作者面**：`pricinghttp.NewReviewReferenceCatalogueEndpoint` 与 `OperatorRegistryIntake.IntakeReferenceCatalogueReview`（复核者与授予集取自信封），`UnconfiguredIntake` 同口；`cmd/parcel-api` 新特性 `endpoints_pricing_catalogue_review.go`（`registerEndpoints`，lines/build/mount，挂登记册配置写）＋ `assemble_pricing_catalogue_review.go` ＋ `endpoints_pricing_catalogue_review_test.go` 的未接线占位；`endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go` 未改。序列复核装配同接规则读口（`assemble_pricing_registration.go`）。
5. **种子**：`seedgen` 产出 `data/pricing/reference-review-duty-rule.json`（`SYN-TENANT-01` 要求不同主体、不要求授予），`steps/150-pricing.sh` 在序列与目录复核之前登它；种子 README 的 `data/pricing/` 一行补一句。
6. **用例**：`AT-PP-014`、`015`、`016` 的领域、编排、真库、HTTP 与 CLI 用例（全部点名，见 `.scratch/at-coverage-inventory.md` ①三行）；反向依赖的装配与派发用例照演示种子先登规则。UC-PP-001「给开发的交接」的现状缺口表唯一一行已到，整张表删去，另补一条交接句（规则属租户取值、两个复核口、CLI 带不出授予集）。盘点只移行（`AT-PP-014`、`015` ④→①，`AT-PP-016` ②→①），总览计数未动（`tools/at-coverage-counts -check` 因此红，留推送方重算）。
- **管理台**：有序列复核面板（`SeriesReviewPanel`），标签表补 `NOT_CONFIGURED`、`REVIEWER_NOT_QUALIFIED`，须换人与面板提示改按规则说；管理台**没有目录复核页**，本票不新建，目录操作者面只到端点。

### 代决

1. **授予先于主体比较。** UC-PP-001 `AT-PP-015`「经受控 CLI 复核而规则要求某一格授予时，不论复核者是谁，一律答复核者不合格」；先比主体的话，规则两格都要求时登记者本人经 CLI 复核答须换人，换了人照样过不去，指错恢复动作。价卡批准门（`PriceCardDraft.Approve`）是先比主体，两处顺序因此不同。
2. **代写一格不导出、领域另立构造器。** 不导出让「只由来源喂价置位」由包可见性在结构上成立（操作者面与 CLI 在包外造命令，给不出这一格）；代写走 `NewExemptionDelegatedSeriesReview`，不在编排里伪造一条「要求不同主体」的规则——那样的规则看着像租户的声明。
3. **结果格的名字**：`NOT_CONFIGURED` 与价卡批准同词；复核者不合格取 `REVIEWER_NOT_QUALIFIED`，照 `APPROVER_NOT_QUALIFIED` 的构词。CLI 退出码：两格与须换人同归治理答案 2。
4. **CLI 两种复核文档改封闭解码**（未知键、尾随内容都答用法错），不只拒代写那一个键：只拒一个键名，换个拼法会被静默丢掉。仓内种子与测试批文没有多余键。
5. **规则文档两格都必须写出**：`distinctSubjects` 缺即拒，`requiredGrant` 缺即拒、不要求写 `null`。缺键按「不要求」读是放行方向，与「缺省朝拦」同一判法。
6. **授予不合格一个哨兵、须换人仍按序列与目录各一**：规则共用，主体比较的哨兵沿用两处既有的。
7. **编排顺序**：先读登记（版本不在册先答 `VERSION_UNKNOWN`），再读规则，再立复核者主体；复核者主体立不住答 `NOT_ACCEPTED`。规则行只增不改，复核读规则不加锁。
8. **CLI 装配抽成 `assemble(db)`**，真库用例 `cmd/parcel-pricing-register/roundtrip_test.go` 走同一份接线。
9. **目录复核口的未接线占位放在特性测试文件**（照价卡收尾特性的先例），不进 `unwired_orchestration.go`。

### 越权风险点

1. **授予先于主体**（PP owner）：见代决 1。要改回先比主体，`AT-PP-015` 的 CLI 那一支得同改。
2. **操作者授予集只是能力面名**（PP owner）：信封译进授予集的只有 `adapters/accessidentity` 的 `grantableFaces` 那几格能力面（登记册配置写、主数据与运营查阅读）。租户把规则的要求授予登成自定的等级名，操作者面恒答复核者不合格；登成 `REGISTRY_CONFIGURATION_WRITE`，到得了复核口的人都持有它，等于不要求。与价卡批准门同一暂定译法（那里的注释已写明），本票未动。
3. **CLI 复核文档变严**（PP owner）：先前被静默忽略的多余键现在退出码 1；库外手写批文若带多余键要改。
4. **规则文档缺键即拒**（PP owner）：比没有 CLI 口的价卡批准规则严；运营不要求授予时须写 `null`。
5. **代写一格靠包边界**（PP owner）：`parcelpricing/application` 包内别的编排在结构上也置得上它；只有来源喂价置位这件事没有静态门禁。
6. **已灌库按本分支重跑种子**：迁移 0014 施加、规则补登 `RECORDED`、复核答 `ALREADY_RECORDED`；之后网络目录那一节答未决（3）中止——main 自己在已灌库上重跑停在同一处（README 既有说明，用 `--reset`），不是本票引入。
7. **生成物与登记册**：端点多一行，机制清点与 AT 盘点计数由推送方重生成；参数登记册 `PAR-SET-11` 的复核责任方一格未改（ADR-0262 Consequences 交文档自洽 10）。

### 验证

- `go build ./... && go vet ./...` 退 0；`gofmt -l` 无输出；新增与改动文件无 CR、无 BOM。
- `scripts/test-full.sh`（一次性 PostgreSQL，含 PG）跑改动包、`go list -test` 算出的反向依赖、`./cmd/parcel-api/`、`./cmd/parcel-pricing-register/...`、`./internal/architecture/...`、`./migrations/...`、`./scripts/demo-seeds/...`，共 75 个包模式，退 0、零 FAIL。
- 新用例 26 个 `-count=3 -v`（含真库），78 次 PASS、零 FAIL、零 SKIP。
- 变异（各自确认被杀后还原）：① 规则未登记时退回写死的不同主体比较 → 编排、真库、CLI 真库三条用例红；② CLI 复核文档退回宽松解码 → `TestReviewDocumentsCannotClaimTheExemptionDelegation` 红；③ 主体比较挪到授予之前 → 领域、编排与 CLI 真库用例红；④ 喂价不置代写一格 → 喂价编排四条用例红。
- 管理台三道门（`tsc -b --noEmit`、`run-tests.mjs` 601 过 0 败、`vite build`）退 0；`node_modules` 软链主树，用后删去软链与产物。
- 种子：一次性库 `c41seed` 干净灌入退 0，计价一节规则 `RECORDED`、两份序列复核与目录复核 `RECORDED`，库上三条复核行 `APPROVED`、复核者 `SYN-PRICING-REVIEW-01`，不退成未配置；另用临时 detached worktree 按 main 灌、再按分支重跑，结果见越权风险点 6。库已 `DROP … WITH (FORCE)`，临时 worktree 已删、登记已摘。
