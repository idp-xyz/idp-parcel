# 128 UC-TF-004 验收表 AT-TF-181 新旧两行并存

Category: bug
Status: in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票无内容提交——旧行已由 docs-consistency/08 `6e53ee84f` 删去，只有票面那一笔；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M4；ADR-0204 决定二

## 现象与修法

现象、证据与修法的完整写法以复核报告 M4 一条为准（单一权威），此处只记要点：

删掉写着「不改写首次开始」的那一行。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcgdocs`，起点 `d3aa57fc3`）

### 提交

本票没有内容提交：修法在本批开工前已由别的票落进 main。只有票面完成记录那一笔。

### 核对结果

- **旧行已删**：UC-TF-004 验收表里写着「不改写首次开始」的那一行，已由票 docs-consistency/08（`6e53ee84f`，「去掉 AT-TF-181 重号行」）删去。在 `d3aa57fc3` 上全仓 grep「不改写首次开始」，`docs/` 下零命中，只剩票面、审计与复核报告里的引文。
- **留下的那一行与 ADR-0204 决定二一致**：「委托的开始取已采用各段首个参与里最早的那一条，后采用的段开始得不更早就不动（`AT-TF-189` 是更早的那一格）」。
- **AT 盘点**：`.scratch/at-coverage-inventory.md` 里 `AT-TF-181` 没有表行（属复核报告 M23 所说「TF 的多段」没有表行那一批，归票 139），没有行可删可移；总览计数不动，由推送方重算。

### 与报告相抵处

- 报告 M4 修法写「删掉那一行，并重算 at-coverage-inventory 的计数」。删行已由 docs-consistency/08 做完；重算计数按 [parallel-sessions](../../../docs/agents/parallel-sessions.md)「生成物不占号」归推送方，作者不做。

### 代决

无。

### 自验

- `go run ./tools/adr-index -check` 退 0；`go run ./tools/at-coverage-counts -check` 报生成物过期（同批票 134 在②加了一行，见票 134），属预期。
