# ADR-0161：赔付、退款与代垫回收用同一套金额文法

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答限额、比例、免赔怎样组成一个金额。）
Date: 2026-09-30

## Context

`PAR-COM-07` 与 `PAR-SET-08` 把「限额、比例、免赔怎样组成一个金额」划成计算方法。`SettleClaimAmounts` 与客户代垫回收只核规则或合同在不在，金额由命令里的主张带入。金额规则版本册已经存在（ADR-0160），CONTEXT 写明文法不在那本只存版本引用的册里。

主张、免赔、比例、限额的先后会得到不同的数。不把顺序定下来，执行器就没有可测的行为。

## Decision

**一、文法固定为一条。** 主张减去免赔，不足记 0；再按万分比向下取整到最小货币单位；再以限额封顶。比例封在 0 到 10000。不提供表达式、脚本，也不提供「先比例再免赔」的另一条。

**二、三项数值是租户取值，另册登记。** 赔付、索赔费用退款和应追偿挂所采用的金额规则版本；客户代垫回收挂所采用的合同责任引用。命令是 `parcel-settlement-register amount-grammar`。不写回金额规则版本册。不登任何租户的行，不写死限额、比例或免赔。

**三、没登记答未配置。** 空册与没有相符行都不形成金额，不用主张金额顶上。全零（限额 0、比例 0、免赔 0）是一份登记，算出 0，与没登记不是一回事。算出 0 不形成金额事实。

**四、形成金额时写下的是文法结果。** 主张仍留在命令里供幂等对照。结果可以小于主张。代垫回收仍不得超过已成立的代垫金额，这条不由文法放宽。

## 候选与反方

- **先乘比例再减免税赔。** 主张 10000、免赔 1000、比例 5000、限额不封顶时，本记录得 4500，那一条得 4000。两套并存就没有唯一展开。否决。
- **比例可以超过 100%。** 文法就不再闭合，限额也不再是唯一的上界。否决。
- **把三项数值塞进金额规则版本册。** 那本册只存版本引用。否决。
- **没登记时把主张原样当成金额。** 那是今天的缺口，不是未配置。否决。

## Consequences

- 迁移是 `settlement_accounting/0024_amount_grammar_parameter.sql`。
- 分摊形态、周期费用、越权升级不在本记录。
- 金额行上不另存一份展开。展开由这次采用的三项取值与主张重算；取值行不改。

## 越权风险点

1. 比例封在 100% 以内。赔偿超过主张的合同表达不了。归 settlement-accounting 的 owner 复核。
2. 代垫回收的三项取值挂合同责任引用，不挂金额规则版本。归 settlement-accounting 的 owner 复核。
3. 展开不落在金额行上，只由取值行与主张重算。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0160](./0160-settlement-read-ports-have-registers.md)：金额规则版本册
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 1 项
