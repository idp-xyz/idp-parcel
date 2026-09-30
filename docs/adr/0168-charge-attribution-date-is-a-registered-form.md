# ADR-0168：费用归属日按已登记形态判定，没登记就不形成

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答一个已点名的时点怎样落成归属日。）
Date: 2026-09-30

## Context

`PAR-SET-09` 把费用归属日的判定形态划成产品策略。各金额的唯一创建用例，以及既有借贷项纳入后续账期，已经是机制。截单读取归属日，不判定它从哪一个时点来。包裹创建、收寄或签收不能当成全局归属日。账户周期、业务时区和截单时刻仍是租户取值。

## Decision

**一、内置形态封闭为两套。** `SOURCE_OCCURRED` 用来源发生时点，`CHARGE_CONFIRMED` 用费用确认时点。在已登记的业务时区里落成公历日。本地时刻达到截单时刻，归属日是下一日。没有第三套，也不接受包裹创建、收寄或签收作为形态名。

**二、形态、时区与截单时刻按费用项目另册登记。** 命令是 `parcel-settlement-register charge-attribution`。截单时刻是当天的分钟数，0 到 1439。0 是午夜这一份登记，不是没登记。不登任何租户的行，不预填时区或截单时刻。

**三、没登记答未配置。** 空册与没有相符费用项目都不形成归属日。账户周期不由这次判定生成。截单编排仍只读已经形成的归属日，本记录不改纳入与发布。

## 候选与反方

- **没登记时用签收日。** CONTEXT 禁止用签收日全局替代归属日。否决。
- **没登记时用系统当天。** 那是计算日期顶上业务日期。否决。
- **把账户周期一起算出来。** 周期日历是租户取值，本记录没有那本日历。否决。

## Consequences

- 迁移是 `settlement_accounting/0031_charge_attribution.sql`。
- 经营指标派生不在本记录。
- 已进 main 的金额文法、分摊、周期费用、越权升级、连接器与评价请求触发不在本记录里改行为。

## 越权风险点

1. 达到截单时刻归到下一日，截单时刻本身不算在当天。归 settlement-accounting 的 owner 复核。
2. 归属日只到公历日，不带出账户周期。归 settlement-accounting 的 owner 复核。
3. 截单编排不调用这次判定。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- [UC-SA-003](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 6 项
