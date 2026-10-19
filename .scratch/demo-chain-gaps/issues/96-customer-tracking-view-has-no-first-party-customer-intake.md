# 96 客户追踪视图口还挂字面量未配置，首方客户渠道读不到已形成的客户视图

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点二）。
Blocked by: 无
地盘：首方客户族的查询 Intake（`internal/accessidentity` 或 VE `adapters/http`，照提交口那一口的分法）；`cmd/parcel-api/endpoints.go` 里 `/customer-tracking-view` 那一行；能力词表；种子 `scripts/demo-seeds/data/access/08` 一行授予与 README 授予说明。
出处：[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四（覆盖面点名查阅面的 `/customer-tracking-view`，逐端点换）、决定八（查阅口不判准入范围）；[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

演示包裹有效交付、PS 终局之后，VE 已按投影形成 `SYN-ACCOUNT-01` 名下这件包裹的客户视图（`visibility_exception.customer_view` 一行，带前版回指）。`GET /customer-tracking-view` 带配置员令牌、带合成货主系统的首方客户令牌，都答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。

`assembleBusinessEndpoints` 里这一口的 Intake 是字面量 `visibilityhttp.UnconfiguredIntake{}`。`visibilityhttp.QueryIntake` 的头注仍写认证方式「属 `PAR-INT-01` 待提供」，早于 ADR-0139。提交口与送站预报两口已换首方 Intake，这一口没有承接票。

## 三类

- 机制：首方客户族的查询 Intake——令牌核验、客户渠道册取（租户，客户账户），查询键只取册上那一行，请求里自报的账户不看；查阅口不判准入范围（ADR-0139 决定八）；答复格照决定四分格。
- 产品策略：客户面能力词表给这一口一格，粒度随 [first-party-api/01](../../first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md)。
- 租户取值：演示租户给合成货主系统登这一格授予（种子 `data/access/08`），证据记 `S`。

## 完成判据

- 首方客户令牌读自己账户名下包裹的视图答 `200`；别的账户的包裹与不存在的包裹同答（UC-VE-008 不可枚举）；操作者令牌答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`；没有这一格授予答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。
- 首方族三件不设时仍答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- `QueryIntake` 头注改指 ADR-0139。
- 真库装配用例；演示上重走一次，证据记 `S`。

## 规模

S–M
