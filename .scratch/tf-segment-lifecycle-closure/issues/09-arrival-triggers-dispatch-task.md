# 到达事实触发建立派送任务：触发条件先裁

Category: enhancement
Status: draft——触发条件未裁，MCP-3 下一轮裁；裁前不转 ready-for-agent
Blocked by: 05

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 7 行：`OpenDispatchTask` 的两个触发源
是「尾程实际履约段到达派送范围」（内部触发）与「授权角色建立派送任务」（admin 写面，归票 [07](07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md)）。
内部触发那一半拆票时没落进 04–07 任何一张；MCP-3 2026-09-03 裁：**另立本票，不并进 06**——并进去会把
06 从接线票变成裁决票。

输入是移动事实（票 [05](05-movement-fact-endpoint.md) 的 `RecordMovementFact`，种类 `ARRIVAL`），所以 Blocked by 05。

## 要裁的

UC-TF-006 的触发句「尾程实际履约段到达派送范围」里有两个词今天没有定义在 TF 的 CONTEXT 里：

- **「到达」等于什么事实**：到达事实的地点 = 该对象计划段的终点节点？还是班次的终点？移动事实今天
  挂在班次（`Schedule`）上，不挂在对象或段上——从一条到达事实推到「哪些对象到了」要经装载分配
  （`FormLoadAssignment` 的成员）再到对象，这条推导链是不是 TF 拥有的判断。
- **「派送范围」是谁的词**：它像 network-routing 的服务区域（`ServiceArea`），若是，TF 触发时要读 NR 的
  判断结果还是收 NR 的事件；若是 TF 自己的词，CONTEXT 要先加词条。

裁完才知道触发放在哪一层：`RecordMovementFactHandler` 落库后同事务调 `OpenDispatchTask`（同票 06 的
形状），还是由一个消费到达事实的内部执行器异步建任务。

## 边界

裁前不写代码；不改 `RecordMovementFact` 与 `OpenDispatchTask` 的既有签名。派送任务的工作范围七件
（`OpenDispatchTaskCommand`）从哪里来——对象集、地点、时间窗、条件——也在裁的范围内：到达事实
自己给不出时间窗。
