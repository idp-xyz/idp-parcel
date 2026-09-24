# 02 参数登记册逐行拆分，以「实例半边」为由的暂缓逐份重新定性

Category: task
Status: in-progress——2026-09-24 通道 4 认领（派单 task-4d61197f，通道 1 派；共享树，纯 md）
Blocked by: 无
地盘：[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md)、本目录新票。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二与 Consequences。

## 做什么

1. 登记册逐行按分界检验拆：「待提供」栏里属判断方法的部分划为产品策略，属某租户具体取值的部分留在本册。登记册此后只装租户取值。
2. 划出的产品策略逐条落去处：写进对应 `CONTEXT.md`，或立工作票（同一上下文的可合成一张）。
3. 以「实例半边」为由暂缓过的 ADR 与票列清单，逐份定性：仍是租户取值的维持；属产品策略的指向第 2 步的工作票。ADR 正文不改写。

## 不做

- 不在本票实现任何产品策略；不给任何租户取值填值。

## 完成判据

- 登记册每行都有拆分结论；清单里每份暂缓都有定性与去处。
