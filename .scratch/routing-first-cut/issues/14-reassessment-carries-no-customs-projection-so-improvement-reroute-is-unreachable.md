# 14 复核与改路入口不携关务投影，计划上有位置时改善改路走不到

Category: bug
Status: needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [12](12-cc-customs-applicability-judgment-for-route-candidates.md) 阻断修复的非作者补评审：通道 3 的旁注，通道 4 的路径清单可互证，均钉 `18be66e8`。只读取证，未动代码。
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为出自 12 的评审。
相关：[auto-reroute-demo-reachability/02](../../auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md)（失效后改路那一支）；[ADR-0173](../../../docs/adr/0173-auto-reroute-is-folded-from-the-strategy-version.md)（自动改路由策略版本折出）。

## 现象（钉 `18be66e8`，按代码判，未实跑）

- 重校 `ValidateReachabilityJudgmentHandler.Handle` 与复核/改路 `ReassessRouteHandler.Handle` 读证据视图时不携候选的关务投影（两端国家），CC 因此对两端都答状态未知·缺码。
- 计划上有位置时，`ReviewPlanApplicability` 于是恒得 `PlanReviewUndecided`，复核落 `PlanReviewInconclusive`，`tryImprovementReroute` 走不到；`structuralAutoReroute` 用的是同一份证据。
- 不是 12 引入的：接关务之前（NotConnected 时代）关务一格同样恒为未知，结果相同。

## 待分诊

- 复核与改路该不该携带候选关务投影（与可达性、初始路由同源取），还是这一支本来就要等别的票——分诊先答这一问，再定地盘与阻塞边。
- 是否与 auto-reroute-demo-reachability/02 合并处理：那张走的是失效后改路，本条挡的是改善改路，可能共用同一处缺口。
- 先实跑确认一次：上面是按代码判的，分诊时在演示租户或真库用例上取一次 `PlanReviewInconclusive` 的现场。
