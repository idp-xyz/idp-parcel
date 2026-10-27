# 165 关务文档的请求时机只写了出发前问询，没写派送出发前的交付门禁问询

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [94](./94-a-courier-previews-the-final-delivery-gate-before-dispatching.md) 的完成记录。
Blocked by: [94](./94-a-courier-previews-the-final-delivery-gate-before-dispatching.md)（ADR-0238 随它进 main）
地盘：`docs/domain/customs-compliance/CONTEXT.md`「动作准入判断」词条、`docs/application/customs-compliance/UC-CC-013-JUDGE-ACTION-ADMISSION.md` 的目标与触发一格。只改文档，不改代码。
出处：[ADR-0238](../../../docs/adr/0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md) 越权风险点 9 与 Consequences「关务」一条。

## 现象

票 94 开了派送出发前的交付门禁问询（ADR-0238）：尾程设备在派送出发之前按件问，关务照 ADR-0206 决定六记一次判断，随后同参的有效交付命中同一判断。关务那几处权威文档写请求时机时，出发前问询（ADR-0219）各补过一句，交付前问询还没有：

- CC CONTEXT「动作准入判断」词条：「它由消费方在动作时请求……出发之前的出发前门禁问询同样请求一次判断……」。
- UC-CC-013 的目标一段与触发一格，同样只点了出发前问询。

GLOSSARY「动作准入判断」所有者一条与 CONTEXT-MAP `transport-fulfillment → customs-compliance` 是共用文档，票 94 已补；上面两处在 CC 地盘，票 94 只读，没改。

## 三类

- 机制：无，只补文档句。
- 产品策略：无。
- 租户取值：无。

## 完成判据

- 两处各补一句：派送出发之前的交付门禁问询同样请求一次判断、关务照记，问询方只翻译交回、不保存，随后的交付在它自己那一刻重问（ADR-0238）。措辞照 ADR-0219 补的那一句。
- 不改判断本身，不改代码。

## 规模

XS
