# 21 SELL 评价到客户费用：请求面、消费门与形成编排

Category: enhancement
Status: needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；分诊时先定触发策略，机制半边随之拆笔
Blocked by: 无
地盘：`internal/settlementaccounting`（SELL 请求、消费门、客户费用形成编排）、`internal/parcelpricing`（请求入口若需扩）、装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 19。

## 现象

SELL 评价没有请求面：PP 的生产入口只有 SA 评价请求信封一条，而 SA 只发 BUY 目的的请求（`RequestBuyEvaluation`）。评价已记录信封的消费门只收 BUY·供应商成本，同处注释原话「SELL 那一半只能在这里安静地走掉」。SA 应用层没有由评价形成客户费用的编排，`domain.FormCustomerCharge` 唯一的生产调用在库适配器的重建路径上。

## 做什么

- **机制半边**：SA 发起 SELL 评价请求；评价已记录的消费门接住 SELL；由 SELL 评价形成客户费用的编排。
- **产品策略半边**：SELL 评价何时发起的内置触发策略。

## 要裁的

1. 触发点：接受决定、有效网络收寄、包裹终局，还是按计费事件逐项——各对应一种常规计费形态，内置哪一种、其余是否作为可显式采用的参考配置。
2. 客户费用的三段演进（`CONTEXT` 已定）与本编排的起点：SELL 评价形成的是哪一段。

## 完成判据

- 一份 SELL 评价能在生产进程里形成客户费用，触发策略有内置执行器；票 05 格 19 越过（隔离形态实测只记 `S`）。
