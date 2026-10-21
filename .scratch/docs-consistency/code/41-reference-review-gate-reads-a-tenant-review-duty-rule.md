# 41 PP 计价参考序列与目录的人工复核写死「复核者不得是登记者」，照 ADR-0262 改为读租户的参考复核职责规则

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
2. 端口与持久化：读口（按租户读，`found=false` 即未登记）与只给装配和测试用的写口；新迁移一张表（`parcel_pricing` 下接现有最大号）。
3. 编排：两只复核编排先读规则，未登记答新的 `NOT_CONFIGURED`、要求授予而复核者没有答新的复核者不合格一格，`NEEDS_ANOTHER_REVIEWER` 只在规则要求不同主体时给出；三格都不落复核行。复核命令从操作者信封带上授予集。
4. 免人工复核：租户对来源声明免人工复核时，连接器代写的那一份不经此门，行为不变。
5. 种子：`SYN-TENANT-01` 登一条要求不同主体、不要求授予的规则，演示动线行为不变。
6. 用例：`AT-PP-014`、`AT-PP-015` 的领域、编排与真库用例；`.scratch/at-coverage-inventory.md` 把两行从④移入①。

## 要裁的

无，已由 ADR-0262 裁定。登记面（治理写面）照 `PriceCardApprovalDutyRule` 的先例另裁，不在本票。
