# 142 NR 侧没有用例守住计划段窗口依据的 CONNECTION 拼写

Category: enhancement
Status: in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，与 143 同批，分支 `claude-dcg142`（worktree `idp-parcel-dcg142`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R4；ADR-0201 决定二

## 现象与修法

现象、证据与修法的完整写法以复核报告 R4 一条为准（单一权威），此处只记要点：

代决「是」：两条认作 NR 对 TF 的契约；NR 侧加一条用例守 `CONNECTION/<代码>@<版本>` 恰一格与「同代码跨版本只改起讫」。
