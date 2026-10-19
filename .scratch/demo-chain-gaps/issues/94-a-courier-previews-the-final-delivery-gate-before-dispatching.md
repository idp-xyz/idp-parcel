# 94 派送出发之前问一次交付门禁

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [66](./66-effective-delivery-records-a-final-delivery-gate-reading.md) 的完成记录。
Blocked by: [66](./66-effective-delivery-records-a-final-delivery-gate-reading.md)（读数内核与 `DeliveryGateReader` 随它进 main）
地盘：TF 新 `application/preview_delivery_gate.go`（复用 `DeliveryGateReader` 取 at 的那一步）、新 tfhttp 问询口；新 `cmd/parcel-api/assemble_delivery_gate_preview.go` + `endpoints.go` 一行（装配令牌）；文档 UC-TF-006 AT、ADR（号认领时由协调方给）。
出处：[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末「补记：票 66 落决定五时的补充裁定」越权风险点 20；先例 [ADR-0219](../../../docs/adr/0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md)（设备出发前门禁问询）。

## 现象

票 66 只在有效交付登记时问交付门禁、照记读数。读数是交付之后的记录：读到阻断时包裹已经在收件方手里。TF CONTEXT 写的是「挡下违规交付是派送出发之前那一问的事」，而派送这一侧今天没有那一问：派送任务形成、派送尝试出发都不问关务，尾程设备也没有读口。出发那一侧由 ADR-0219 补上了出发前问询，交付这一侧没有对应的口。

## 三类

- 机制：作业事实渠道开只读问询，按（对象，拟交付时点）交回与随后同参交付登记同一套读数（at 的取法同 ADR-0213 决定五与票 66 补记）；TF 不落记录，CC 照 ADR-0206 决定六记判断。授予口径照 ADR-0219（能登有效交付的就能问），不新增能力种类。
- 产品策略：问询挂在设备出发前，还是派送任务形成那一拍也问一次并把读数挂在任务上；参考做法是前者，与 ADR-0219 同形。
- 租户取值：无。

## 完成判据

- 尾程设备在派送出发之前按对象与拟交付时点问一次，答与随后同参交付登记逐字节相同的读数；阻断照答 200，挡不挡归设备与现场。
- 问询不落 TF 记录；之后的交付登记照旧按它自己那一刻重问。
- 真库装配用例；证据记 `S`。

## 规模

M
