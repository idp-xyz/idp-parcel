# 接管重放路只补区间追加不补 handOff——接管信封可能永不入队

Category: bug
Status: needs-triage

发现于 OUTBOX-PK-STEP2（接管格修复 `b0e928e`，票面 [outbox-partition-key/03](../../outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md)）实现过程，
MCP-2 报回未动代码；本票只记现象与边界，不带方案。

## 现象

`GovernIncidentHandler.TakeOver` 的首次入册路径 = 写接管行 → 区间追加 → handOff（发接管信封）。
若首次调用在**区间追加失败**处中断，重试会走 `TakeoverExisting` 分支：该分支**只补区间追加，
不补 handOff**。于是这次接管的信封从未被尝试入队——不是被 `EnqueueOnce` 幂等吞掉，是压根
没到过入队那一步。

## 与同族票的分界

[outbox-partition-key/02](../../outbox-partition-key/issues/02-pod-correction-is-silently-swallowed-by-enqueue-once.md)（POD 更正被吞）是「入队被幂等挡下」；本票是「重放路径缺一步 handOff」。
两者都属「该不该发」类，但修法不同——那边看信封身份，这边看重放路径的完备性（补 handOff，
或把 handOff 与追加纳入同一提交边界再整体重试）。修法归实现票，此处不拍。

## 影响与时效

按[消费方向图](../../outbox-handoff-consumption-map/report.md)，治理接管口今天没有消费者
（应有但未开），故当下无生产事故；接上消费者后这就是一次静默丢失。接管格的信封 ID 与
分区键已由 `b0e928e` 修对（ID 补权威方+生效起点），本缺陷与那次修复正交。

## 归属

pilot-governance 当前无主（同四处待裁的 PG 格局）。修复须先派 PG 归属或进裁断轮。
