# 86 交出门禁读数阻断的那一件派生异常信号

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [65](./65-handover-records-an-outbound-release-gate-reading.md) 的完成记录；方案 [handover-dispatch-plan](../handover-dispatch-plan.md)「后继」一节所列「交接与交付门禁的阻断读数派生 VE 异常信号」中交出那一半。
Blocked by: [65](./65-handover-records-an-outbound-release-gate-reading.md)（读数随它进 main）
地盘：VE `adapters/transportfulfillment/derive_on_transport_handover.go` 与同包测试、VE 目录登记（若另立规则册）、迁移 VE（认领时由协调方给号）、`cmd/parcel-dispatch` 相应装配。交付那一半等 [66](./66-effective-delivery-records-a-final-delivery-gate-reading.md) 落地后并入或另立。
出处：[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记越权风险点 13；先例 [ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记「阻断读数派生信号」一条。

## 现象

票 65 之后，每一版`已交接`都随记交出门禁读数，阻断也照样落册、答 201。VE 消费交接信封时按键读回交接，读得到读数，但没有任何规则据「交出被阻断」形成异常信号：违规交出只有查库才看得见。出发那一侧已经有「出发门禁阻断信号规则」（票 33，ADR-0205 文末补记），交出这一侧没有。

## 三类

- 机制：识别条件（读数整格为阻断即命中，命中时刻取判断那一刻）、信号与投影同一笔、未决哨兵。
- 产品策略：交出阻断用不用出发那一册的同一类规则，还是另立一册；参考配置。
- 租户取值：信号类型、识别规则版本、可信度与启不启用，没登记就如实不形成。

## 完成判据

- 交出读数为阻断的那一件，按租户登记的规则形成一次异常信号交分诊；没登记规则如实不形成，投影照派生。
- 读数为未决、读不动或不受门禁不形成信号。
- 真库用例；演示租户的采用（若有）证据记 `S`。

## 规模

M
