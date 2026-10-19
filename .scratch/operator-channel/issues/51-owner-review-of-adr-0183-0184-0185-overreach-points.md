# 51 ADR-0183、0184、0185 越权风险点的 owner 复核（代决）

Category: task
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 研判完整性批评点出）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判轮完整性批评：三份 ADR 的越权风险点至今没有 owner 复核行
地盘：
- docs/adr/0183-*.md（只在越权风险点旁加复核行）
- docs/adr/0184-*.md（同上）
- docs/adr/0185-*.md（同上）
- .scratch/owner-review-queue.md（生成物，按其脚本重生成）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 做什么

照 `.scratch/owner-review-queue.md` 头行的流程，对三份 ADR 末尾的越权风险点逐条复核：认可就在原文旁写「owner 复核 2026-10-xx 认可（用户 2026-10-16 授权「你作为业务和系统专家你来自决」，会话 idp-parcel-9b 代裁，逐条）：1. …」，每条一句理由；不认可的走 supersede（新 ADR 部分停用），不改写原文。

与在途票的关系：ADR-0184 风险点 2（逐字比对）的事实更正与范围归 ps-external-mark-relations/04；「唯一命中算不算可靠识别」归 ps-external-mark-relations/05；ADR-0183 决定五被 ADR-0187（operator-channel/38）部分停用；ADR-0183 风险点 1、2 的复核要与 38 的证据落册裁定一致。

## 完成判据

- [ ] 三份 ADR 的每一条越权风险点都有复核行或 supersede 指针。
- [ ] 复核理由各自引原文依据，不复述别处权威。
- [ ] owner 复核队列重生成后这三份显示「已有 owner 复核记录」。
