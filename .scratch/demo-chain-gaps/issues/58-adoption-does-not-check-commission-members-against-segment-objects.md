# 58 段采用委托不核委托成员与段上参与对象：干线委托也挂得到一个只装末端包裹的段

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md) 完成记录与 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 4；与 [49](./49-booking-does-not-check-schedule-against-commission-scope.md)（订舱不核委托的运输范围）同族。
Blocked by: 用户决定（TF owner 复核 ADR-0204 风险点 4：委托成员与段对象要不要相交、在哪一层比）

## 现象

采用只核段在册、委托在册且未取消、服务方等于委托的服务提供方、段的首个参与不早于委托提交。委托列的对象成员与段上参与的对象不比：两者可能在不同层级（委托列包裹，段上是集运单元；或反过来），本上下文没有成员展开的读法可用。于是一份只列了干线包裹的委托，也能被一个只装着别的包裹的段采用，采用之后那份委托就标了开始、取消不了。

## 要裁的

- 判据：委托成员与段参与对象至少相交一件，还是段上对象都要落在委托成员里，还是只要求声明方断言。
- 层级：集运单元与成员包裹之间按哪一份成员快照展开（node-operations 的单元成员，还是委托提交时的声明）；展开读不动时答什么。
- 段后来加入的对象不在委托成员里时，连线是否受影响。

## 三类

判法是产品策略；成员展开的读口是机制。
