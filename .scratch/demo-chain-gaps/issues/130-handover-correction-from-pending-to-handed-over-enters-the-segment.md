# 130 交接更正由待确认或拒收改为已交接时，对象不结束先前段参与也不进新段

Category: bug
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（票 126 已进 main），分支 `claude-dcg130`（worktree `idp-parcel-dcg130`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M7（第二件）；ADR-0216 越权风险点 2

## 现象与修法

现象、证据与修法的完整写法以复核报告 M7（第二件） 一条为准（单一权威），此处只记要点：

前版待确认或拒收、本版已交接的交接更正改走进段，先例是 label-channel-service-first-release/31 的分路。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/tf.md](../../docs-consistency/audit/tf.md) M17（归 C，审计已点名本票，不另立）。审计钉 `6b0074d0`；按 main `67438a0f` 复核，现象仍在：`application.rederiveFulfillmentParticipation` 在前版没有参与时仍答 `SegmentEntryRefusedNoParticipationToRederive`，对象不进新段；票 88、146、94 进 main 都没有碰这一路。

分级建议（文档自洽 02）：R2，与本票既定「上线后尽快」一致——由待确认改为已交接的更正不常见，对象仍挂在先前段里看得见。

文档侧由修订票 docs-consistency/08 做，本票不改文档：UC-TF-005 补一条 AT「更正为`已交接`即进段、结束先前段参与」，标「待票 130」；UC-TF-006 `AT-TF-289` 的括注写明那是本票之前的过渡态。本票落地时去掉这两处标注。

文档自洽 08（2026-10-21）已照上一段做：UC-TF-005 新立 `AT-TF-329`（待确认或拒收更正成`已交接`即进段、结束先前段的参与），「给开发的交接」记本票为它的现状缺口；UC-TF-006 `AT-TF-289` 括注写明那一种是本票之前的过渡态。本票落地时去掉这两处标注，`AT-TF-329` 补 Covers 点名。
