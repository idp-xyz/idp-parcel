# 62 同一结算账户在受理前估价采用与 SELL 计费时点采用两本册上可以登不同的计价范围，冻结与预估客户费用会出自两张价卡

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：SA 受理前估价采用与 SELL 计费时点采用两本册可登不同范围——演示租户登同一范围撞不上；绕法：上线清单要求两本同范围、靠人核（已记入冻结文件「记录」，上线清单必须有这一条）；解冻条件：真实租户登记时撞上，或上线清单核对不了。此前 ready-for-agent——建议 R3（不急）：演示租户两本册登同一个范围，撞不上；真实租户上线时登记错一本就会让冻结额与预估客户费用出自两张卡，冻结不足或过多看起来像计量差。绕法：上线清单要求两本同范围，靠人核。2026-10-21 文档自洽 11 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc11。
Blocked by: 无
出处：审计 [audit/cross.md](../audit/cross.md) B2·B2-10；裁定 [ADR-0280](../../../docs/adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md)。

## 现象

- ADR-0280：同一（租户、结算账户）在受理前估价采用册与 SELL 计费时点采用册上登的计价范围必须相同；登任一本时另一本已有这一户且范围不同，答冲突、不落行、不交那一本的登记信封；同一户在两本上的登记串行。
- 代码今天：两只登记编排各写各的册，互不读；两本册可以给同一户登不同的范围。

## 证据

- `internal/settlementaccounting/application/register_pre_acceptance_estimate_adoption.go` 的 `RegisterPreAcceptanceEstimateAdoptionHandler.Register`，经 `adapters/postgres/pre_acceptance_estimate_adoption.go` 的 `SaveAdoption` 落册，`已登记`与`重放`交 `HandOffPreAcceptanceEstimateAdoptionRegistered`，冲突不交。
- `internal/settlementaccounting/application/register_sell_customer_charge_registers.go` 的 `RegisterSellBillingMomentAdoptionHandler.Register`，经 `adapters/postgres/sell_billing_moment_adoption.go` 的 `SaveSellBillingMomentAdoption` 落册，再由 `announceEvaluationParameter` 交信封（冲突不交）。
- 领域：`domain.PreAcceptanceEstimateAdoption` 与 `domain.SellBillingMomentAdoption` 各带 `PricingScope()`（同一种 `EstimatePricingScope`）。
- 迁移 settlement_accounting `0034_pre_acceptance_estimate_adoption.sql`、`0038_sell_billing_moment_adoption.sql`：两张表各自的键是（租户、结算账户）。
- 文档：ADR-0179 决定三、五；ADR-0190 决定三、越权风险点 9 与 Consequences 验收句。

## 三类

产品策略：同户一份范围（ADR-0280 决定一）。机制：登记时的核与串行。租户取值：某户登哪个范围，不变。

## 规模

S：两只编排各加一次读与一道比较，两只仓储各加一把按（租户、结算账户）的事务锁或在同一笔里读另一本；不加迁移。

## 最小的诚实修法

1. 两只登记编排在落本册的同一笔事务里读另一本上同一户的行：没有或范围相同照常；范围不同答各自那一本的冲突格（估价采用 `PreAcceptanceEstimateAdoptionConflict`，计费时点采用 `CatalogueConflict`），不落行，照两只编排既有的做法冲突不交信封。冲突答复带出另一本的那一行与范围，受控 CLI（`cmd/parcel-settlement-register`，两本今天只有这一个登记口）照透出。
2. 两只编排对同一（租户、结算账户）取同一把事务锁，并发的两笔串行。
3. 用例：先登估价采用再登不同范围的计费时点采用答冲突、不交信封；反序同；同范围两本都落；只登一本照常；并发两笔不同范围只有一笔落。
4. 演示种子不改：两本已登同一个范围。

## 相关

- UC-SA-002「接受前财务控制」与「给开发的交接」记了这一缺口。
