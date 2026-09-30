# ADR-0169：SELL 评价形成客户费用，何时发起另册登记

Status: Accepted（2026-09-30。消费门与形成编排是机制，触发时点是产品策略。接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）
Date: 2026-09-30

## Context

评价已记录信封对 BUY 与 SELL 是同一种。现有消费门只收 BUY·供应商成本，SELL 在那里被放下。SA 没有把 SELL 评价收成客户费用的编排。SELL 评价也没有自己的请求身份。

BUY 评价请求的触发面已经单独登记（ADR-0164）。那条身份不能拿来装 SELL。

## Decision

**一、消费门只认 SELL·客户费用。** 方向或目的不是这一对的评价，这扇门答不处理。BUY 评价仍由原消费门处理，本记录不改那扇门。

**二、已完成的 SELL 评价形成预估客户费用。** 原币、结算币和换算步骤整组出自评价。命令只指名费用身份和费用项目，不另带金额。评价未完成时按待判断、未形成、不可计价或冲突停下，不落零额。

**三、何时发起是可登记的触发。** 内置时点只有发生项形成。哪些发生项原因采用它，命令是 `parcel-settlement-register sell-evaluation-trigger`。键是租户与发生项原因。不写进 BUY 触发册，也不调用 BUY 评价请求。不登任何租户的行，不预列原因。

**四、没登记答未配置。** 空册与没有相符原因都不发起。信封上还没有费用项目时，消费门指名缺这一引用，不发明费用项目，也不在本记录里把形成编排接进进程。

## 候选与反方

- **给 CalculationPurpose 加一格 SELL。** 那是 BUY 请求身份。否决。
- **SELL 评价在 BUY 消费门里顺手形成客户费用。** 两扇门的恢复动作不同。否决。
- **原因没登记也按发生项形成发起。** 那是默认时点。否决。

## Consequences

- 迁移是 `settlement_accounting/0032_sell_evaluation_trigger.sql`。
- 形成编排不进 `cmd/parcel-dispatch`。后继装配传入 SELL 读口，不把 BUY 请求编排接过来。
- 第 10 项的其余进程入口不在本记录。

## 越权风险点

1. 消费门在费用项目还没登记时停住，不调用形成编排。归 settlement-accounting 的 owner 复核。
2. 触发面只回答发不发起，不铸造 BUY 那种评价请求。归 settlement-accounting 的 owner 复核。
3. 预估费用的写入与确认写入分开。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：BUY 触发面
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)
- 票 `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md` 第 9 项
