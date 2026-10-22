# 28 NR 路由复核的「适用性看不清」回滚重投到 ABANDONED，照 ADR-0250 改为入账未判、留下缺口

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code28
Blocked by: 无
出处：[audit/nr-no.md](../audit/nr-no.md) M8（审计归 B，由 docs-consistency/05 立 [ADR-0250](../../../docs/adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md) 代决）。

## 现象

复核编排遇原计划适用性看不清（选中候选的限制状态未知），一律答未决、回滚重投。限制状态未知今天的来处都是重投推不动的缺口，不是依赖故障。

## 证据

- 代码：
  - `internal/networkrouting/domain/plan_review.go` 的 `ReviewPlanApplicability`：选中候选 `ConstraintStatusUnknown` 时答 `PlanReviewUndecided`，缺口原样带出。
  - `internal/networkrouting/application/reassess_route.go` 的 `reassessPlan`：`PlanReviewUndecided` 一律 `handler.undecided(key, PlanReviewInconclusive)`。
  - 限制状态未知的来处：`adapters/customscompliance/customs_source.go` 的 `unknownGap`（`CUSTOMS_ENDPOINT_COUNTRY_MISSING/<端>`、`CUSTOMS_PORT_PATH_CATALOG_EMPTY`），`application/catalog_network_evidence.go` 的 `CustomsApplicabilityNotConnected`（`CUSTOMS_APPLICABILITY_NOT_CONNECTED`）。关务目录读不动在取数侧交错误，不进这一格。
- 文档：ADR-0250 决定一；NR CONTEXT「路由稳定性与受控改路」复核前提缺口两格那一条；UC-NR-003 结果「触发未判」、`AT-NR-051`、`AT-NR-067`。

## 三类

机制：复核按恢复动作分格。产品策略：无。租户取值：关务口岸路径目录、关务判断口的接入属租户与部署，本票不碰。

## 规模

S–M：一个新原因、一处折法、一次迁移、用例。

## 最小的诚实修法

1. `ports.NotJudgedReason` 加 `PLAN_APPLICABILITY_UNCLEAR`；`reassessPlan` 遇 `PlanReviewUndecided` 改走 `handler.notJudged`，`ReassessUndecidedReason` 去掉 `PlanReviewInconclusive`。
2. 未判登记（`network_routing.route_reassessment_not_judged`）加缺口与再判条件两列，取 `PlanReview.Gap()` 的缺少内容与再次判断条件；这一原因必带，其余原因可空（CHECK 照此写）。
3. 揽收复核（code/16 落地后）同走这一格。
4. 补 `AT-NR-051` 的编排、真库用例；`AT-NR-067` 的用例里去掉「适用性看不清」那一支。

## 要裁的

无，已由 ADR-0250 裁定。续办（目录登齐之后重驱）归 demo-chain-gaps/154。

## 完成记录（2026-10-21，作者会话，分支 claude-code28，基于 main `e0d0909cf`）

### 做了什么

- 编排 `ReassessRouteHandler.reassessPlan`：领域答 `PlanReviewUndecided` 时改走 `notJudged`，原因 `ports.NotJudgedPlanApplicabilityUnclear`（`PLAN_APPLICABILITY_UNCLEAR`），缺口与再判条件取 `PlanReview.Gap()`；计划与适用性都不动，不落复核。`ReassessUndecidedReason` 去掉 `PlanReviewInconclusive`。节点收寄、交接与场外揽收都经这一处，揽收（code/16 已落）不另改。
- 端口 `NotJudgedTrigger` 加 `Missing`（`domain.EvidenceGapReference`）与 `Reassess`（`domain.ReassessmentCondition`）。
- 迁移 `migrations/network_routing/0026_route_reassessment_not_judged_plan_applicability_unclear.sql`：原因封闭集加一格；加 `evidence_gap`、`reassessment_condition` 两列与 CHECK `route_reassessment_not_judged_gap_paired`（两列成对、非空白；这一格原因必带）。postgres `RecordNotJudged` 经 `notJudgedGap` 先拒一次，写口仍走 `RequireExecutor`，事务外拒写由既有 `TestANotJudgedTriggerIsWrittenOnlyInsideATransaction` 钉。
- 文档：UC-NR-003 复核未决一格去掉「今天另有 `PLAN_REVIEW_INCONCLUSIVE`」那句，`AT-NR-051` 改注已补齐，「给开发的交接」未实现表去掉这一行、已实现那句补上；NR CONTEXT 与 ADR-0250 早已写成现行，不动。AT 盘点 `AT-NR-051` 由④移入①、文末加追记，总览计数未改。机制清点重生成（迁移数加一）。

### 代决

1. **「其余原因可空」照字面写成可空，不写成必空。** ADR-0250 后果一节写「这一原因必带、其余原因可空」；CHECK 只要求这一格必带、两列成对，不像 `current_position_at`、`shipment_accepted_at` 那样写成 iff。编排今天只在这一格填缺口。
2. **登记只记缺口引用与再判条件两件，不记缺口的范围与受影响候选。** 计划复核里的缺口恒是选中候选那一家的候选级缺口，范围与候选都推得出来；照 ADR「缺口与再判条件两列」。
3. **领域答未决却没带缺口按编程错误响亮报错**，不答未决也不落未判：`ReviewPlanApplicability` 构造时恒带缺口，交不出是绕过了构造门（ADR-0029：确定性错误不进未决哨兵）。
4. **`AT-NR-067` 的用例里本来没有「适用性看不清」那一支**（全仓测试检索 `PlanReviewInconclusive`、`PLAN_REVIEW_INCONCLUSIVE` 无命中），票面第 4 点后半无可删；UC 行文已由 docs-consistency/05 改窄。
5. **真库用例走交接一路、证据视图用替身演关务来源未接**（缺口 `CUSTOMS_APPLICABILITY_NOT_CONNECTED`），并进 `AT-NR-074`–`076` 那张表；生产证据视图对三种来源答限制状态未知，由既有关务适配器（`customs_source_test.go`）、证据视图（`catalog_network_evidence_test.go`）与派发关务接线（`customs_applicability_wiring_test.go`）用例钉，本票不重复。

### 越权风险点

1. **其余原因带缺口在库里不被拒**（NR owner）。代决 1 的另一面：日后某一格误带缺口，库不报。owner 若读「可空」为「必空」，补一条 iff 即可，已有行恒满足。
2. **同一触发再投时缺口照留先到那一行**（NR owner）。未判登记按（租户，触发关联）留先到那一行（ADR-0235）；同一触发再投时缺口已变（例如目录登了、国家仍缺），这一行仍是第一次的缺口。续办（票 154）按缺口认行时要知道这一点。
3. **ADR-0250 自身的越权风险点 1–4 照旧**：关务来源未接也入账不报、受理基线缺国家是永久缺口（知会 PS owner）、原计划在看不见的限制下照走、最后一个节点上的未判再无复核（票 154）。

### 测试

- 先红：编排用例与登记册用例在实现前各自失败（编排答 `UNDECIDED/PLAN_REVIEW_INCONCLUSIVE`；登记册撞 `gap_paired`）。
- 新用例：`internal/networkrouting/application/reassess_route_test.go::TestAnUnclearPlanApplicabilityIsNotJudgedWithItsGap`；`internal/networkrouting/adapters/postgres/route_reassessment_not_judged_test.go::TestAnUnclearApplicabilityCarriesItsGap`；`cmd/parcel-dispatch/reassess_not_judged_test.go::TestReassessmentsThatCannotBeJudgedYetSettleWithTheirReasonOnTheDatabase` 加「适用性看不清」子用例。三条 `-count=3` 绿。
- 变异（都被杀）：编排不填缺口（编排用例、派发真库用例红，后者答 `REASSESS_STORE_UNAVAILABLE`）；编排改回答未决（两处红）；迁移去掉「这一格必带」子句（登记册直插用例红）；适配器去掉成对检查（登记册用例红：要求适配器先拒、不送到库）。
- `scripts/test-full.sh`：改动包与其 `go list -test` 反向依赖共 17 个包，加 `./internal/architecture/...`、`./migrations/...`，全绿。
- 一次性库 `c28seed` 灌演示种子成功、新约束在位，用完已删。
