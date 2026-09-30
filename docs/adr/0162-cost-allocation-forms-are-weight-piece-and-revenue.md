# ADR-0162：成本分摊的内置分法是按重、按件、按收入，尾差按最大余数

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答按重、按件、按收入怎样把一笔来源金额分成份额。）
Date: 2026-09-30

## Context

`PAR-SET-06` 把分摊规则里的分法划成产品策略。`AllocateCosts` 只核分摊规则版本在不在，各份额由命令 `Portions` 带入。CONTEXT 已经要求守恒、最大余数，以及余数相同时用稳定业务顺序，并禁止没有规则时默认均摊。缺的是三套分法的执行器，以及「没登记」和「算出来」的分界。

## Decision

**一、内置分法封闭为三套。** 按重、按件、按收入。三套共用同一条展开：来源金额乘该对象的权重，除以权重合计，向下取整到最小货币单位；余下的最小货币单位按余数从大到小各加一，余数相同按目标标识升序。不提供均摊，不提供表达式。三套的差别只在权重的含义，算术相同。

**二、是否适用与选哪一套，挂所采用的分摊规则版本另册登记。** 命令是 `parcel-settlement-register allocation-form`。取值是 `BY_WEIGHT`、`BY_PIECE`、`BY_REVENUE`，或 `NOT_APPLICABLE`。不写回只存版本引用的分摊规则适用表。不登任何租户的行。

**三、各对象这次的权重随分摊交入，不预填。** 重量、件数或收入是这次来源的对象事实，不是产品常量。权重小于 0、目标重复、来源金额不是正数，拒。权重为 0 的对象不进份额。没有任何正权重时，来源金额整笔未分摊，不虚构对象。

**四、没登记答未配置。** 空册与没有相符行都不算出份额，也不改走均摊。`NOT_APPLICABLE` 是一份登记，答不适用，与没登记不是一回事。算出的份额仍由 `AllocateCosts` 按命令 `Portions` 收下并守恒；本记录不改那条编排。

## 候选与反方

- **再加一套平均分。** `UC-SA-006` 禁止未确认时创建默认比例或平均分摊。平均分会变成没登记时的退路。否决。
- **余数相同按输入顺序。** 同一组对象换个列举次序就换承接人。否决。
- **把权重写进分法登记行。** 按重的重量随包裹变，写死在规则版本上就是替这次分摊填了数。否决。
- **没登记时按件数均分。** 那是默认分母。否决。

## Consequences

- 迁移是 `settlement_accounting/0025_allocation_form_choice.sql`。
- `AllocateCosts` 仍只核规则版本，份额仍由命令带入。
- 周期费用、金额文法、越权升级不在本记录。

## 越权风险点

1. 三套分法算术相同，只靠登记的名字区分权重含义。归 settlement-accounting 的 owner 复核。
2. 稳定顺序用目标标识的字典序，不是业务上的另一套序号。归 settlement-accounting 的 owner 复核。
3. 本记录不把执行器接进 `AllocateCosts`。调用方要把份额放进 `Portions`。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- [UC-SA-006](../application/settlement-accounting/UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 2 项
- [ADR-0163](./0163-supplier-audit-escalation-compares-the-matched-amount-to-a-ceiling.md)：越权升级
