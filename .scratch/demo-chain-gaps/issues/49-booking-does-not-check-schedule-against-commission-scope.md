# 49 订舱不核班次与委托的运输范围：末端委托下也订得到干线那一班

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md) 完成记录与 [ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记风险点 12（与正文风险点 3 同源）。
Blocked by: 用户决定（TF owner 复核 ADR-0202 风险点 3 与 12：委托要不要固定运输范围、范围用什么表达）

## 现象

订舱提交只核「委托在册且未取消」与「班次在册」。委托上没有运输范围一格（ADR-0202 风险点 3：不核协议是否覆盖这段运输范围），班次上没有运营方一格，所以班次的方向是否落在委托要运的那一段里、班次是不是这家服务提供方在运营，都没人判。2026-10-19 一次性库实测：末端承运商的委托（`SYN-COMMISSION-LM-01`）下订 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21`（干线那一班）答 `201 BOOKING_SUBMITTED`。

## 要裁的

- 委托提交时固定运输范围的表达：NR 连接代码加起讫节点（与班次方向身份同形，ADR-0201 决定二），还是计划段引用，还是二者之一可选。
- 订舱核对加一格（班次方向不在委托范围内）放在班次在册之后；答复格名与续办。
- 班次要不要有运营方一格；没有时订舱是否只核方向。
- 已在册的委托与订舱没有范围，补登还是只对新提交生效。

## 三类

判法是产品策略；范围与运营方的取值是租户取值（PAR-NET-08）。
