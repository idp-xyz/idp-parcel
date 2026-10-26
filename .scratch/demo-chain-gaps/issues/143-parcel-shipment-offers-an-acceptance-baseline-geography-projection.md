# 143 NR 读回整份委托聚合只为取受理基线地理维

Category: enhancement
Status: in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，与 142 同批，分支 `claude-dcg142`（worktree `idp-parcel-dcg142`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R6；ADR-0215 越权风险点 1

## 现象与修法

现象、证据与修法的完整写法以复核报告 R6 一条为准（单一权威），此处只记要点：

代决「是，作为过渡」：PS 开「受理基线地理投影」端口，NR 进程不再加载收件人身份与地址本体；NR 适配器只换协作者。
