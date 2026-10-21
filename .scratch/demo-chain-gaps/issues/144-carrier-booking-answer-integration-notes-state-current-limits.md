# 144 承运答复口首版集成说明未写现形限制

Category: enhancement
Status: ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R8；ADR-0202 决定二

## 现象与修法

现象、证据与修法的完整写法以复核报告 R8 一条为准（单一权威），此处只记要点：

代决「是」：按现形作为首版集成契约；集成说明写明候补位次更新、候补后分批接受目前进不来。

## 补注（2026-10-21，票 [145](./145-booking-answers-from-a-non-provider-answer-booking-not-found.md) 的作者子代理）

票 145 改了答复口的一格，集成说明照 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-refusal-register.md) 写：答复方不是订舱所属委托的服务提供方时答 `200` `BOOKING_NOT_FOUND`，与订舱不在册同一格、同一副响应，线上不再有 `SOURCE_NOT_SERVICE_PROVIDER`；这一格的续办写「找租户核对订舱标识」，不写「这份答复不归你答」。凭什么拒只记在租户侧的承运答复拒收册，不对承运方透出。
