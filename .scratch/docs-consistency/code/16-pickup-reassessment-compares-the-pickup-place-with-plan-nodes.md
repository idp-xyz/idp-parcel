# 16 NR 场外揽收触发的复核把揽收地点当成计划节点来比，地点不是节点码，原计划就判失效

Category: bug
Status: ready-for-agent——R1（上线前必修）：答错。揽收复核已在生产装配（`cmd/parcel-dispatch/assemble.go` 经 `adapters/parcelshipment/reassess_on_intake.go`）；揽收地点是客户场所、不是节点码，于是第一天起每一次场外揽收复核都让初始计划失效，包裹落无当前有效路由或被改路，与 `AT-NR-034` 期望的「保留原路由」相反。没有绕法（运营无法让复核不比节点）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 docs-consistency/05 已裁（[ADR-0249](../../../docs/adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)），裁向见文末「裁向」。原 Blocked by：docs-consistency/05 先裁 [audit/nr-no.md](../audit/nr-no.md) H2 的三件（揽收复核时位置与计划怎样比；揽收算不算可控节点；它与 CONTEXT「交给运输方的交接不建立可控节点」那一句的关系）。
出处：[audit/nr-no.md](../audit/nr-no.md) H2（审计归 B，建议修法写明「随后改 CONTEXT 这三句与 AT-NR-034，再立代码票」；无论裁向，今天的比法都要改。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，NR 代码在此期间未改）。

## 现象

揽收复核取揽收地点作复核位置，拿它去比计划的节点集合，不在里面就答原计划不再适用。计划段只有节点间段与交付段，没有「计划接货范围」或揽收段可比。`structuralAutoReroute` 又把揽收当成「位于可控节点」。

## 证据

- 文档：
  - NR CONTEXT「路由形成与选择」：「有效网络收寄结果形成后，必须依据当时已知的实际接货位置……重新校验。……场外揽收时使用 `transport-fulfillment` 的揽收交接结果，不要求尚未发生的到站或节点实测。」同节：「拒收、待确认与交给运输方的交接不建立可控节点，不复核。」
  - 「路由稳定性与受控改路」：「已执行前缀只由计划段链和当前可控节点切出；可控节点只认节点收寄或权威运输交接。」
  - UC-NR-003 `AT-NR-034`：「场外揽收地点与计划接货范围一致，尚未到节点 | 使用实际接货位置复核，不虚构节点实测或到站」。
- 代码：
  - `internal/networkrouting/adapters/parcelshipment/reassess_on_intake.go`：`triggerSpecFor` 以 `source.Place()` 作复核位置；`controlKindFor` 把 `psdomain.OffsitePickupSource` 译成 `nrdomain.TransportHandoverControl`。
  - `domain.ReviewPlanApplicability`（`domain/plan_review.go`）：位置不在 `plan.Nodes()` 里即 `PLAN_ORIGIN_MISMATCH/<地点>`，`PlanNoLongerApplicable`。
  - NR 没有 `AT-NR-034` 的测试。

## 三类

机制：复核按裁定的比法判适用性。产品策略：揽收地点与计划首节点的对应判法（例如服务区域）若裁为内置，属产品策略。租户取值：服务区域的具体划分归租户网络登记，未登记答未配置。

## 规模

M（按裁向，可能涉及计划模型加揽收段，则 L）。

## 最小的诚实修法

照 05 的裁定实现；不论裁向，下面两条都成立：

1. 揽收地点不得直接与计划节点码比；比不出时答「适用性看不清」一类的未决并留原因，不判失效。
2. 揽收是否算可控节点按裁定改 `controlKindFor` 与 `structuralAutoReroute`，并补 `AT-NR-034` 的用例。

## 要裁的

全部在 Blocked by 那一裁里，已裁，见下节。

## 裁向（2026-10-21，docs-consistency/05，ADR-0249）

三件都已裁，照 [ADR-0249](../../../docs/adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md) 实现：

1. **比法：只复核硬约束，不比节点。** 揽收触发的复核只看选中候选的硬约束：被有效限制淘汰即失效，限制状态未知按「适用性看不清」（[ADR-0250](../../../docs/adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)，代码见 code/26），其余仍适用。揽收地点原样记入复核依据，不参与判断。上面「最小的诚实修法」第 1 条后半「比不出时答『适用性看不清』一类的未决」作废：比不出不是证据缺口，是比法不成立。
2. **揽收不建立当前可控节点。** 已执行前缀为空、全部计划段未执行（`onlyUnexecuted` 为真，不得出 `EXECUTED_PREFIX_AFFECTED`）；原计划仍适用即保留、结论 `STILL_APPLICABLE`，不判冻结（不落 `FREEZE_FORM_UNCONFIGURED`）、不走 `tryImprovementReroute`；失效线折自动条件时位于可控节点为假，只形成建议，阻塞 `NOT_AT_CONTROLLED_NODE`。原先无路由时照常可形成首个计划。
3. **与「交给运输方不建可控节点」的关系。** 两者都不建可控节点；揽收照复核（有效网络收寄的来源），节点交给运输方照旧不复核。揽收复核计入当前控制位置（复核册照常记触发业务时间）。

落到代码的建议形状：`ControlEvidenceKind` 加「场外揽收」一格（`controlKindFor` 把 `OffsitePickupSource` 译成它，不再译 `TransportHandoverControl`）；`ReviewPlanApplicability` 对这一格跳过节点比对；`JudgeExecutedPrefix` 对这一格答「尚无可控节点、全部未执行」的新等级；`structuralAutoReroute` 的 `atNode` 对这一格为假；`freezeOf` 前的分派对这一格直接提交仍适用。补 `AT-NR-034`、`AT-NR-084` 的领域、编排与收寄采用一路适配器用例。
