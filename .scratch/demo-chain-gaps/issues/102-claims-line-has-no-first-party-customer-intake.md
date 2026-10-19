# 102 索赔提交口还挂字面量未配置，客户系统经首方渠道交不了索赔

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 子代理立（票 [96](./96-customer-tracking-view-has-no-first-party-customer-intake.md) 实现时盘点：VE 的两个客户面口，查阅口已换，索赔口没有承接票）。
Blocked by: 无
地盘：`internal/visibilityexception/adapters/http` 的 `ClaimIntake` 首方实现与 `receive_claim.go` 的答复分格；`internal/visibilityexception/adapters/accessidentity` 的认证方；`internal/accessidentity` 首方铸造器加一口与能力词（迁移 `access_identity` 下一号，CHECK 全集取此前各份的并集）；准入范围对照的种子（`data/access/03`、`data/governance/`）；`cmd/parcel-api/endpoints.go` 里 `/claims` 那一行。
出处：[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四（覆盖面点名命令面的 `/claims`）、决定五（接入请求标识推导为来源请求键）、决定八（每个客户面写口定一版带形状版本的规范化形状，未定形状的口不开首方渠道；索赔提交在铸造侧判准入）；[UC-VE-007](../../../docs/application/visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md)。

## 现象（钉 `79ad6122`）

`assembleBusinessEndpoints` 里 `/claims` 的 Intake 是字面量 `visibilityhttp.UnconfiguredIntake{}`，任何令牌都答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。`visibilityhttp.ClaimIntake` 的头注仍写批次与项标识的客户侧唯一性口径「属 `PAR-INT-01` 待提供」，早于 ADR-0139。同族的提交口、送站预报两口、送站交接确认口、追踪视图查阅口都已换首方 Intake。

## 三类

- 机制：首方族在索赔口的铸造——一口一词的能力、铸造侧判准入范围（能力 `CUSTOMER_COMMAND`，事实类型取这一口的能力词）、按命令分型的信封、接入请求标识推导来源请求键；索赔的规范化摘要形状（带形状版本）由 VE 定，作同身份重放与内容冲突的判据；提交时间取到达信封（ClaimIntake 头注已钉）。答复按 ADR-0139 决定四分格。
- 产品策略：能力词的名字与粒度（随 [first-party-api/01](../../first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md)）。
- 租户取值：演示租户给合成货主系统登这一格授予、准入范围对照与权威区间，证据记 `S`。

## 完成判据

- 首方客户令牌交一笔索赔答受理；同一接入请求原样重投答已有结果，换内容答冲突；不在准入范围答 `403 OUTSIDE_ADMISSION_SCOPE`；操作者令牌答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`；没有这一格授予答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。
- 首方族三件不设时仍答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- `ClaimIntake` 头注改指 ADR-0139。
- 真库装配用例；演示上走一次，证据记 `S`。

## 规模

M
