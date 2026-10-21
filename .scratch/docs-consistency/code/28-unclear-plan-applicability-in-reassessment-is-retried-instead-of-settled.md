# 28 NR 路由复核的「适用性看不清」回滚重投到 ABANDONED，照 ADR-0250 改为入账未判、留下缺口

Category: bug
Status: ready-for-agent——R2（上线后尽快）：看得见的卡住。关务目录为空、受理基线缺端点国家或关务来源未接时，这件包裹的每一次复核都占着交接分区（或小包托运分区）重投到 `ABANDONED`，拖住同一分区后面的信封；失败码指向依赖类未决，运维会去查一个没坏的依赖。绕法：租户登齐关务口岸路径目录、部署接上关务判断口之后，前两格不再出现；受理基线缺国家那一格没有绕法。2026-10-21 文档自洽 05 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc05。
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
