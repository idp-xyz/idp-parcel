# 138 ADR-0201 与种子 README 把票 110 写成 108

Category: bug
Status: in-progress——2026-10-26 复评阻断已修，待进 main，分支 claude-dcgdocs tip `82656f52e`（修订轮二本票无改动，票面那一笔在其上；见文末「复评」「修订轮二」）。此前 in-progress——2026-10-26 评审阻断已修、修订轮完，待复评与进 main，分支 claude-dcgdocs tip `867b157d2`（修订轮本票无改动，票面那一笔在其上；见文末「评审记录」「修订轮」）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票内容 `a102d8581`，票面那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M21；ADR-0201 越权风险点 29

## 现象与修法

现象、证据与修法的完整写法以复核报告 M21 一条为准（单一权威），此处只记要点：

两处的 108 改为 110；ADR 正文不动，在 ADR-0201 补一则更正记录。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcgdocs`，起点 `d3aa57fc3`）

### 提交

| SHA | 内容 |
|---|---|
| `a102d8581` | `scripts/demo-seeds/README.md` 组成表 `data/transport/07`–`23` 那一行：「守卫用例归票 demo-chain-gaps/108」改为 110 |
| 票面那一笔 | 本完成记录与 Status |

### 做了什么

- 种子 README 那一行的 108 改为 `demo-chain-gaps/110`。改完全仓 `docs/`、`scripts/` 下 grep `demo-chain-gaps/108`，只剩 ADR-0201 正文里那一处（正文不动）和它文末的更正。
- ADR-0201 一侧不用补：更正记录已在它文末「复核记录（2026-10-20…）」的「**更正**」一条（「越权风险点 29 正文所写「票 demo-chain-gaps/108」应为 110」），由复核会话 `7270c63df` 写入。本票没有动 ADR-0201。

### 与报告相抵处

- 报告 M21 说种子 README 那一行「两次写成 108」。实际在 `9045b6fd` 与 `d3aa57fc3` 上都只有一处；另一处是 ADR-0201 正文。
- 报告 M21 的修法要求「在 ADR-0201 补一则更正记录」：已在，见上，不重复补。

### 代决

不给 ADR-0201 的 Status 行加「引用更正」注记。ADR-0054 那种写法用于链接本身断了的情形；这里链接一直指着 110 的文件，错的只是显示的号。2026-10-20 各份复核记录也都不往 Status 行抄，这里照办。

### 自验

改动行没有相对链接；没有新引入的行号或计数式引用。`go run ./tools/adr-index -check` 退 0。

## 评审记录（2026-10-26，tops-20 隔离子代理，钉 `472dae3aa`）

本票无发现（评审原文：「128、138 已落地属实」）。全文照抄在票 [134](./134-uc-sa-002-names-the-parcel-moment-path-exceptions.md) 的「评审记录」一节。

## 修订轮（2026-10-26）

本票无改动。同批修订轮的提交见票 134、141、144 的「修订轮」一节。

## 复评（2026-10-26，钉 `3637253f7`）

本票无新发现、无改动。复评要点与修订轮二全文见票 [141](./141-pricing-basis-moment-is-product-strategy-in-sa-context.md) 的「复评」「修订轮二」两节。
