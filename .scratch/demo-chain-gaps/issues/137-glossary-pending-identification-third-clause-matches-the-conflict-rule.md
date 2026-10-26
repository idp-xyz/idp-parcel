# 137 GLOSSARY「待识别实物」第三条与第四条字面相抵

Category: bug
Status: in-progress——2026-10-26 复评阻断已修，待进 main，分支 claude-dcgdocs tip `82656f52e`（修订轮二本票无改动，票面那一笔在其上；见文末「复评」「修订轮二」）。此前 in-progress——2026-10-26 评审阻断已修、修订轮完，待复评与进 main，分支 claude-dcgdocs tip `867b157d2`（修订轮本票无改动，票面那一笔在其上；见文末「评审记录」「修订轮」）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票内容 `7b5681a6e`，票面那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M20；ADR-0210 越权风险点 2

## 现象与修法

现象、证据与修法的完整写法以复核报告 M20 一条为准（单一权威），此处只记要点：

第三条改为「候选与正式包裹身份只由 `parcel-shipment` 给出；实物关联哪一件由本上下文按其答复判，冲突经身份冲突处置定」。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcgdocs`，起点 `d3aa57fc3`）

### 提交

| SHA | 内容 |
|---|---|
| `7b5681a6e` | GLOSSARY「待识别实物」第三条 |
| 票面那一笔 | 本完成记录与 Status |

### 做了什么

`docs/domain/GLOSSARY.md`「待识别实物」第三条：保留前半「同一条码对应多个实物或同一实物出现冲突标识时，各实物保持独立并形成身份冲突」，后半换成报告 M20 那句——「候选与正式包裹身份只由 `parcel-shipment` 给出；实物关联哪一件由 `node-operations` 按其答复判，冲突经身份冲突处置定」，ADR-0210 链接留着。第四条与 NO CONTEXT 讲唯一命中的那句都没动。

### 与报告相抵处

- 报告取证于 `9045b6fd`，那时第三条写「正式身份由 `parcel-shipment` 确认」，与第四条字面相抵。其后票 docs-consistency/10a（`e35c903c5`）已把它改成「正式包裹身份由 `parcel-shipment` 给出，实物与包裹的版本化关联与身份冲突处置归 `node-operations`」，已经不相抵。本票仍按派单落报告那一句：它多写明了两件事，候选同样只由 PS 给出，关联哪一件由 NO 按 PS 的答复判。

### 代决

1. 报告原句里的「本上下文」写作 `node-operations`：GLOSSARY 是跨上下文的词表，不属于哪一个上下文，照抄会让人读不出指谁。
2. 第四条末句「候选仍只来自 `parcel-shipment` 的答复」不删：它讲的是重核那条路，与第三条讲的不是一件事，删了重核那一格就没有这句约束了。

### 自验

新增的相对链接 `test -e` 都在；没有新引入的行号或计数式引用。

## 评审记录（2026-10-26，tops-20 隔离子代理，钉 `472dae3aa`）

本票无发现（评审原文：「137 新句不引入矛盾（比 10a 略准）」）。全文照抄在票 [134](./134-uc-sa-002-names-the-parcel-moment-path-exceptions.md) 的「评审记录」一节。

## 修订轮（2026-10-26）

本票无改动。同批修订轮的提交见票 134、141、144 的「修订轮」一节。

## 复评（2026-10-26，钉 `3637253f7`）

本票无新发现、无改动。复评要点与修订轮二全文见票 [141](./141-pricing-basis-moment-is-product-strategy-in-sa-context.md) 的「复评」「修订轮二」两节。
