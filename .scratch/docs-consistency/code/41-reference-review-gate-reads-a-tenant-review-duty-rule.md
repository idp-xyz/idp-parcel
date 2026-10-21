# 41 PP 计价参考序列与目录的人工复核写死复核者不得是登记者，照 ADR-0262 改为读租户的参考复核职责规则

Category: enhancement
Status: ready-for-agent——R2（上线后尽快）：单人运营的租户手工登记的序列与目录一版都进不了在用，评价停在待判断；同一租户的价卡批准却可以登成单人可批。绕法：由第二个操作者复核（多人租户照常可用）；单人租户没有绕法，只能改走连接器并声明免人工复核。2026-10-21 文档自洽 07 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc07。
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
