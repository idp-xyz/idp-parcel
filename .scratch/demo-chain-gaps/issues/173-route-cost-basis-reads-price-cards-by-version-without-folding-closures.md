# 173 线路成本依据按版本点读、不折价卡收尾，收尾后择优仍按被收尾的那一版试算，与结算按后继形成的预期成本静默分叉

Category: enhancement
Status: needs-triage——2026-10-21 票 [150](./150-price-card-has-no-controlled-cut-off-for-a-superseded-version.md) 的作者子代理按协调方转来的隔离评审意见立（用户「你作为业务和系统专家自决……解决所有的问题」授权；评审读码属实，严重度中等，不挡 150）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：PP 给 network-routing 的点读口（`internal/parcelpricing` 的 ports 与 `adapters/postgres`）；NR `adapters/parcelpricing`（`cost_source.go`）与择优编排；产品策略那一格归 network-routing owner。
出处：[ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 越权风险点 6。

## 现象与证据

- network-routing 的线路版本按「方案/版本」指名每一段的成本依据（`network_routing.line_cost_basis`）。取数侧 `internal/networkrouting/adapters/parcelpricing/cost_source.go` 的 `resolveLegs` 按这条引用走 `Plans.FindByReference` 读卡，生产接的是 `pppostgres.PriceCards`；那一口为重放而立，按 ADR-0234 决定五不折截止与退役。
- 读回的卡交 `FormPlanGivenEstimateEvaluations` 试算，评价领域 `EvaluatePricing` 里的 `VERSION_NOT_APPLICABLE` 门看的是卡发布时的适用期，截止点之后、退役之后照样过门。
- 自营段经 `PlanReferenceForInternalPolicy` 从内部成本政策解析到卡的引用，再走同一个点读，同样不折。

## 后果

- 一版卡被截止或退役之后，择优照旧按它试算，结算那一侧却按后继形成预期成本，两边静默分叉。
- 旧版有缺陷时（票 150 的动因：缺重量段、取整未声明、不可计价……），那一段在择优里继续不可计价或待判断：候选被排除，或排序停在 `RankingCostsPending`，补登的后继帮不上。
- 换费率的截止之后，择优按旧价比，可能选了更贵的路。
- 不会错记钱：NR 的试算评价记 S、用完即弃，不进结算。

## 今天的运营约束

ADR-0234 写下：收尾一版卡之后，指名它的线路版本要另发一版改指后继，直到本票落地。截止时改指的那一版线路须正好从截止点起生效，才与选卡口径对得上。可运营今天查不到「哪些线路版本指名了这一版卡」，约束执行不了。

## 要做的（三类）

- **机制**：PP 给 network-routing 一个按 `at` 折进收尾的点读——同一版卡在那一刻是否仍选得中（`domain.SelectablePriceCardPeriod` 那一处判法），已不可选时答复带指名的后继；重放用的 `FindByReference` 不动。另立「哪些线路版本指名了这一版卡」的查阅面，让上面的运营约束执行得了。
- **产品策略**（network-routing owner 裁）：遇到已收尾的版本，是让那一段待判断、出处指名后继，还是自动跟随同一方案的后继。
- **租户取值**：哪一版线路指名哪一版卡，照旧由租户登记。

## 规模

M
