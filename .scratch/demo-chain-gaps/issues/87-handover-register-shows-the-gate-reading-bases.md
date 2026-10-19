# 87 运营查阅看得到交出门禁读数的依据原文

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [65](./65-handover-records-an-outbound-release-gate-reading.md) 的完成记录。
Blocked by: [65](./65-handover-records-an-outbound-release-gate-reading.md)（读数随它进 main）
地盘：TF `ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`（或另开按键的交接详情读口）与各自测试；管理台呈现另立前端切片。
出处：[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记越权风险点 14。

## 现象

票 65 之后，交出门禁读数的逐对象依据原文（适用的受门禁动作、依据种类、来源）与关务判断标识只存在交接登记册的 `gate_reading_detail` 列里。交接答复按 ADR-0205 决定一的口径不回给登记方，查阅册 `registry=transport-handover` 只列整格。运营要查一件交出为什么未决或阻断，今天没有读面，只能查库。

出发门禁读数同样没有运营读面，可一并考虑，但不是本票的完成判据。

## 三类

机制（查阅面如实呈现在册内容）。读面走操作者渠道的运营查阅授予，不回给作业设备。

## 完成判据

- 运营查阅能按交接版本看到交出门禁读数的整格、意图（at / onward，没声明去向写明）、停点或那件对象的那一格连同依据原文与关务判断标识。
- 拒收与待确认那一版没有读数，不出现。
- 真库用例。

## 规模

S
