# 接管重放路只补区间追加不补 handOff——接管信封可能永不入队

Category: bug
Status: resolved

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

## Comments

- 2026-08-24 MCP-1（**受用户委托进裁断轮，放行修复，转 ready-for-agent**。基准 `88215a3`）：
  归属问题不阻这笔修复——它是既有编排的重放完备性缺陷，修的是已写下的代码自己许诺的性质
  （「重放路补追加」的续办纪律本来就该把 handOff 一并补齐），不新增能力面，不裁 PG 上下文
  的长期所有权（那一格照旧悬置）。

  **修法方向**（细节以开工时代码为准）：重放路（`govern_incident.go` 的两处 `TakeoverExisting`
  分支）在补区间追加成功后**必须补尝试 handOff**——handOff 走 `EnqueueOnce` 按（来源＋事件
  ID）幂等，先前已入队者答已入队，从未入队者此刻入队，两种历史在重放后收敛到同一终态。首次
  路径「区间追加失败即提前返回、handOff 永不被尝试」的形状同笔收敛：「追加成功 → handOff」
  这一段两条路径应共用，任一步失败留续办引用，重放从断点续齐。

  **验证要求**：①首次调用在区间追加处失败 → 重放 → 接管信封入队**恰一次**；②首次调用全程
  成功后重放 → 信封不重复入队（幂等答已入队）；③既有两条重放用例（区间不重追、续办引用
  清空）保持全绿。**顺带核一格**：Suspension/Resumption 的 `GovernanceAlreadyRecorded` 分支
  是否同型漏 handOff——同型同修，不同型不扩，结论记回本票。

  影响面照票面「影响与时效」节：治理接管口今天无消费者，无生产事故；修在消费者接上之前，
  窗口免费。

- 2026-08-24 MCP-2（实现收口，`8f2b2ff`）：修法照裁断落地——「追加成功 → handOff」抽成
  `completeTakeover` 首次与重放共用，任一步失败留该步续办引用、不越过断点（追加失败不发信封：
  信封宣告权威已切换而区间册无此区间，先发即两帐分岔）；重放凭同一命令从断点续齐，handOff
  重发同一份由 `EnqueueOnce` 按信封身份幂等收敛。**顺带核一格的结论：同型，已同修**——
  Suspend 的 `GovernanceAlreadyRecorded`、Resume 的早查已恢复与 `GovernanceAlreadyRecorded`
  三处重放分支同样只答已在册不补 handOff（handler 不留交发布成败的持久痕迹，重放不补发则
  首次 handOff 失败的信封无人再发），均补为重发在册那份。
  验证（`govern_incident_test.go` 三个新用例 + 既有用例）：①首次调用在区间追加处失败 →
  重放 → 接管信封入队恰一次，且断点修复前不越过断点发信封；②首次全程成功 → 重放 → 重发
  同一份同身份（EnqueueOnce 答已入队，适配器侧幂等由既有
  `TestResendingTheSameGovernanceIntentIsIdempotent` 证）、区间不重追；③既有两条重放用例
  保持全绿。提交态在临时 worktree 检出 `8f2b2ff` 单独验证：`go vet ./...` 干净、
  `go test -p 1 -count=1 ./...` 全仓绿（DSN 已设，真库 PG 实跑，pilotgovernance postgres
  33 用例逐个 PASS）。
