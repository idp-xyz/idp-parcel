# 114 多处消费者把确定性失败答成可重试的未决

Category: bug
Status: needs-triage——2026-10-19 分诊为 B（顺手可修），合并 39、41、54、77、90，见 [triage-2026-10-19](../triage-2026-10-19.md)
Blocked by: 无

## 现象

下面五张票是同一个形状：某个消费者或登记口遇到**重放也不会变**的失败（标识超长使信封越限、分区键越限、两侧封闭词表不一致、库里有坏行导致重建失败），却把它答成可重试的未决，派发因此反复重投直到弃置。

- [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)：BUY 交接口
- [41](./41-transport-commission-handoff-does-not-separate-deterministic-rejection.md)：委托交接口
- [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)：发生项登记的协议核对
- [77](./77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md)：发生项事实读口
- [90](./90-handover-consumers-retry-deterministic-rebuild-failures-as-undecided.md)：交接消费侧

## 为什么合并

分散在 SA、TF、NR、VE、PS 各处，单看每处都「只在坏数据时触发、最终会弃置」，所以没人会顺手修；而形状一致，合起来一次能定一条统一判据：确定性错误不进未决哨兵名单（这条在派发规范里已有），以及读口重建失败归哪一格。

## 不做什么

不在本票改任何派发重试策略本身。
