# 110 演示班期「任何提交时刻三段都有一班可准备」没有守卫用例，日历或班期一改就无声落空

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自 [98](./98-demo-walk-doc-and-seed-corrections-from-the-8197fa2f-walk.md) 的隔离评审。
Blocked by: [98](./98-demo-walk-doc-and-seed-corrections-from-the-8197fa2f-walk.md)（种子 `data/transport/07`–`23` 进 main 之后才有可守的东西）
出处：[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 票 98 补记越权风险点「后两段的班期也是凑的，且零余量咬合」；票 98 完成判据「演示提交在一天里任何时刻，三段运输准备都至少有一班 `OPPORTUNITY_PREPARED`」。

## 现象

- 票 98 的判据今天只由两样东西证明：一份没入库的一次性逐秒模拟脚本，和一次一次性库实走（只覆盖一个提交时刻）。仓里没有能复现这条判据的东西。
- 排法零余量。例如深圳口岸 18:00 开出时，快班 18:00 那一班的出发正好等于窗口起点；新加坡枢纽 20:00 开出时，穿梭 21:30 那一班的到达正好等于窗口终点。两班都靠 TF 候选判定的两端闭区间才接得住（取值只写在 `scripts/demo-seeds/README.md` 组成表 `data/transport/07`–`23` 那一行）。
- 改了下面任一格，就会有提交时刻无声地重新答 `NO_CURRENT_OPPORTUNITY`，没有任何用例先红：
  - 参考配置 `SYN-CN-SG@1` 的连接缓冲；
  - 种子 `data/network` 下的节点与连接服务日历（处理时长、截单）；
  - `data/transport` 下的周期安排。
- 首段的上海→深圳快线（票 18）同样没有守卫，本票一并守。

## 三类

机制（演示种子的守卫用例）。证据记 `S`。规模 S。

## 最小的诚实修法

在 `cmd/parcel-transport-register` 加一个用例（与既有演示种子用例同处），不碰库：

- 读参考配置 `SYN-CN-SG@1` 的连接与缓冲、`data/network` 下的服务日历（同一对象取最新修订）、`data/transport` 下全部周期安排；
- 对一天里每一分钟的提交时刻，推出三段窗口，再按物化出的候选判窗口里有没有合格的一班；
- 任一分钟任一段落空就红，并报出落空的提交时刻与段。

## 要裁的

- **复用真函数还是照抄规则**。窗口推算在 NR（`projectCandidateTime`、`departAt`），候选判定在 TF（`assessCandidate`），两处今天都不导出。
  - 照抄：用例只守种子与配置的取值，NR 或 TF 的规则改了，用例不跟着变，可能假绿。
  - 复用：要导出纯函数或另开测试缝，动两个上下文的包面。
  - 倾向复用。规则只在一处定义，用例守的是「这份种子配这份规则」。
- 容量两维不在本票：判据只说有一班落在窗口里，池够不够由既有用例与实走看。

## 既有票

none
