# 17 NR 原计划仍适用那条线从不形成改路建议：仅建议权、有阻塞、改善候选并列，都静默只保留原计划

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：NR 原计划仍适用那条线从不形成改路建议——原计划仍有效，不答错，只是少一条提示；解冻条件：真实租户要求「有更优候选、待授权」的提示时。此前 ready-for-agent——R3（不急）：原计划仍有效，不答错；缺的是「有更优候选、待授权」的提示，是不方便与少省钱。另外在票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md)（复核不取候选成本）修好之前这一路走不到；那一格已由该票补齐（2026-10-21），本票随之暴露。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无（排在 115 之后；见「要裁的」）
出处：[audit/nr-no.md](../audit/nr-no.md) M4（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：NR CONTEXT「复核遇并列只形成改路建议」；「不满足自动改路条件时，只形成改路建议，由适用授权的运营角色决定」；「形态或阈值没声明时只形成建议，答未配置」。UC-NR-003 步骤 7C、结果「改路建议已形成」。
- 代码：`internal/networkrouting/application/reassess_route.go` 的 `tryImprovementReroute`：`authority != domain.AutomaticRerouteAllowed` 时直接 `return record`；`selection.ranking.Outcome() != domain.RankingSelected` 时同样 `return record`，都不挂建议。只有 `rerouteAfterLapse`（失效线）形成建议；`AT-NR-042` 的测试只覆盖失效线。

## 三类

机制。产品策略：改善阈值的判法已由路由策略版本声明（ADR-0173）。租户取值：阈值与自动改路权限，未声明答未配置、只建议。

## 规模

S–M：仍适用线在未越过冻结边界时，对「改善超过阈值但只有建议权」「改善候选并列」形成建议；补用例。

## 最小的诚实修法

仍适用线与失效线共用同一个形成建议的出口：判出改善候选但不满足自动改路条件时，挂改路建议（带候选、改善额与不自动的原因）；原计划照旧有效，不改写。

## 要裁的

- 审计给的另一条路：NR owner 裁定仍适用一路不出建议，把 CONTEXT 那几句限定到失效线——属 B，归修订票 docs-consistency/05；那张票若这样裁，本票作废。
- **2026-10-21 docs-consistency/05 复核：不走那条路，本票照立。** CONTEXT「不满足自动改路条件时，只形成改路建议」与「复核遇并列只形成改路建议」两句本就不分仍适用与失效两条线，没有 ADR 把它们限到失效线；UC-NR-003 `AT-NR-042` 已注明今天只有失效线形成建议、指本票。场外揽收触发的复核不试改善，仍适用一路不出建议（ADR-0249 决定二），本票不管揽收那一格。
