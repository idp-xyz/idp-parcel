# 53 NR 仍适用线上的改善改路落 `REROUTED` 复核记录时不带失效依据，过不了 `route_reassessment_rerouted_shape`

Category: bug
Status: ready-for-agent——R2（上线前必修）：仍适用线上一旦按改善阈值自动改路，复核册那一行必然被 CHECK 拒，整笔回滚。复核不取候选成本那一格已由票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 补齐（2026-10-21），这条线随之在生产接法上走得到：策略版本声明了自动改路、原计划未越过冻结边界、另有一条更便宜且改善严格大于阈值的候选时，落册被 CHECK 拒，复核编排把写库失败答成 `REASSESS_STORE_UNAVAILABLE` 未决，回滚重投到 `ABANDONED`。演示链走不到：演示策略版本没声明自动改路，演示网络只有一条候选。115 不一并修本票的理由见其完成记录。2026-10-21 协调方立（code/16 作者读码所得，见 [code/52](./52-reassessment-reads-the-initial-route-row-instead-of-the-current-plan-or-no-route.md)「相关」一节；协调方读码核实）。
Blocked by: 无
出处：code/16 修订时读码所得，不在审计报告条目里。

## 现象

仍适用线上的自动改路（`tryImprovementReroute`）形成的复核记录答 `REROUTED`，带新计划与改路决定，但不带失效依据。迁移 `0004_candidate_state_and_rerouted_shape.sql` 立的 `route_reassessment_rerouted_shape` 要求 `REROUTED` 一行 `lapse_basis IS NOT NULL`——那一行落不了库。

## 证据

- `internal/networkrouting/application/reassess_route.go`：`tryImprovementReroute` 末尾只设 `record.Conclusion = ports.ReassessmentRerouted`、`RerouteState`、`NewPlan`、`Decision`，不设失效依据。
- `migrations/network_routing/0004_candidate_state_and_rerouted_shape.sql`：`CHECK (conclusion <> 'REROUTED' OR (reviewed_plan IS NOT NULL AND lapse_basis IS NOT NULL AND new_plan IS NOT NULL AND decision IS NOT NULL …))`。
- 这条 CHECK 与「`REROUTED` 只出自失效线」的旧假设一致；ADR-0173 起仍适用线也能改路，两者没对齐。

## 三类

机制。

## 规模

S–M：要裁的是「改善改路的 `REROUTED` 用什么作依据」——另立一格依据（例如「改善超过阈值」，带被比的两个成本与阈值），还是放宽 CHECK 让 `REROUTED` 在改善线上可不带失效依据而必带改善依据。无论哪种都要一次迁移（与 code/28、code/52 撞号风险：后进 main 的改号）。补真库用例：仍适用线上改善改路落得了库。

## 最小的诚实修法

1. 复核记录加「改善依据」一格（被替代计划成本、新计划成本、所用阈值），`REROUTED` 的 CHECK 改为「失效依据或改善依据恰有其一」。
2. `tryImprovementReroute` 填改善依据。
3. 真库用例：冻结未越过、改善超过阈值时落一行 `REROUTED`、带改善依据、新计划与决定。
