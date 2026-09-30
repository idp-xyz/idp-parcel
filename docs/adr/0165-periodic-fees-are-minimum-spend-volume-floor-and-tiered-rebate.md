# ADR-0165：周期费用的计算形态是最低消费、保底量、阶梯返利

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答最低消费、保底量、阶梯返利怎样从周期实绩收成一笔金额。）
Date: 2026-09-30

## Context

`PAR-SET-07` 把最低消费、保底量、返利的计算形态划成产品策略，并标明待核。CONTEXT 只写了这些费用以合同、结算账户或考核周期为范围，包裹分摊不改变汇总结算事实。`UC-SA-002` 保存评价里的最低收费和阶梯，并写明本用例不重新实现价卡算法。

仓内没有把周期实绩收成补差、保底不足或返利的执行器。价卡上的条件最低重量是另一件事。

## Decision

**一、形态封闭为三套，另加本期不适用。** 最低消费：最低额减去周期内已计金额，不足记 0。保底量：不足数量乘已登记的单位金额，向下取整到最小货币单位；数量单位不在这里换算。阶梯返利：把基数切成互不重叠的档，每档只乘自己的万分比再向下取整，然后相加；最后一档上界之上没有开放档的部分不返利。不提供表达式。

**二、是否适用与各项数值另册登记。** 键是租户与规则引用。命令是 `parcel-settlement-register periodic-fee`。`NOT_APPLICABLE` 是一份登记。不登任何租户的行，不写死最低额、保底量、单价或档位。

**三、没登记答未配置。** 空册与没有相符行都不形成周期费用，也不把补差或返利当成 0。算出 0 是实绩已经达到门槛，不形成一笔费用，与没登记不是一回事。

**四、不改价卡，也不把结果摊回包裹。** 评价里的最低收费仍由 `parcel-pricing` 算出。本记录的金额停在周期范围。

## 候选与反方

- **把价卡的最低收费当成周期最低消费。** 那是单票计价，不是结算账户周期的补差。否决。
- **返利按到达的最高档乘全部基数。** 与分档只乘本档得到的数不同。否决。
- **没登记时补差记 0。** 0 会变成没有最低消费。否决。

## Consequences

- 迁移是 `settlement_accounting/0028_periodic_fee_form.sql`。
- 没有周期截单编排调用本执行器。后继装配传入这本登记册，不传 nil。
- 归属日、连接器、人工调整授权不在本记录。

## 越权风险点

1. 阶梯返利的基数是周期已计金额，不是件数。归 settlement-accounting 的 owner 复核。
2. 保底量的数量单位不换算。归 settlement-accounting 的 owner 复核。
3. 本记录不把执行器接进截单或确认费用。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 3 项
