# ADR-0164：BUY 评价请求的触发面是发生项形成，原因没登记就不发起

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答发生项形成时怎样决定要不要发起一次 BUY 评价请求。）
Date: 2026-09-30

## Context

`assemble_evaluation_request.go` 把请求评价编排装起来只为 fail-fast，没有进程内触发面调它。谁在什么业务时点为哪些发生项发起请求被留成产品题。请求编排本身已经会登记并交信封。缺的是「要不要发起」这一层。

路由择优时的 BUY 评价是另一条请求，目的与请求方不同，不能共用这条请求身份。

## Decision

**一、内置触发时点只有「发生项形成」。** 调用方交来一份运输收费发生项时，若该租户把这个发生项原因登记为 `OCCURRENCE_FORMED`，触发面发起一次 BUY 评价请求。不提供按结算周期批量的另一套。不从运输履约登记册推导三件来源引用。

**二、哪些发生项原因采用这一时点，另册登记。** 命令是 `parcel-settlement-register buy-evaluation-trigger`。键是租户与发生项原因。不预列订舱、取消、失败尝试或实际履约。不登任何租户的行。

**三、没登记答未配置。** 空册与没有相符原因都不发起请求，也不改用一份默认原因。触发面长在 `evaluationRequestOrchestration.Trigger`。`Request` 仍是已经决定要发起之后的登记编排，本记录不改它。

## 候选与反方

- **发生项一形成就对所有原因发起。** 那是把原因清单写成产品默认。否决。
- **再给一套结算周期批量。** 账期是租户取值，本记录没有那条执行器。否决。
- **与路由择优共用评价请求身份。** 两路的目的不同，共用会让重放把择优评价当成供应商成本请求。否决。

## Consequences

- 迁移是 `settlement_accounting/0026_buy_evaluation_trigger.sql`。
- 面单交易与渠道择优的触发面不在本记录。
- 金额文法、分摊分法、供应商审核越权升级不在本记录。

## 越权风险点

1. 只做发生项形成，不做结算周期批量。归 settlement-accounting 的 owner 复核。
2. 触发面不从运输履约登记册补三件引用。归 settlement-accounting 的 owner 复核。
3. `Request` 仍可被直接调用，不经过触发面。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 8 项
