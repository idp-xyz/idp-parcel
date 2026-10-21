# 37 TF：逐订舱的消费关系——哪些订舱已随哪一段消耗（原题「确认订舱的容量关系（ADR-0203 同一判据）」，2026-10-21 按 ADR-0264 改范围）

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。后继票，不在演示主链上；地盘与完成判据分诊时定稿。
Blocked by: [21](./21-load-assignment-allocates-against-capacity-reservations.md)、[24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)
地盘：TF 订舱与容量的 domain、postgres。
出处：[02](./02-transport-commission-has-no-command-face.md)、[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「02 委托、订舱、承运答复」D02-9（越权风险点），「03 装载分配与容量」D03-3，「票拆分」后继票表。

## 更正（2026-10-21，文档自洽 08）

[ADR-0264](../../../docs/adr/0264-capacity-assurance-is-required-on-load-assignments-only-and-neither-a-confirmed-booking-nor-a-departure-reads-capacity.md) 裁定确认订舱不挂容量保障（部分停用 ADR-0203 决定一里确认订舱那一半）：承运答复的`接受`由承运系统交来，答复方不知道本上下文的预占，判据挂在这一口上答不出来；接受量可以超过那一班池上登记的余量，容量保障只挂在装载分配上。下面「做什么」与「完成判据」里的保障判据**作废**，不要照做。

本票保留的范围改为 ADR-0204 越权风险点 3、9 交来的「逐订舱的消费关系」：哪些订舱已随哪一段消耗，订舱一级的「尚可停止范围」据此判，而不是今天按申请时刻是否不晚于委托开始判。地盘与完成判据分诊时重定。

## 做什么

- 确认订舱用装载分配那条保障判据：班次挂了容量池，确认订舱就要引用预占并声明数量（方案 D03-3、「需要的 ADR」ADR-0203）。

## 完成判据

- 挂池班次上的确认订舱缺预占引用、预占不在本班次、超量各有业务格。
- 无池班次照旧。
- 真库测试。

## 规模

M
