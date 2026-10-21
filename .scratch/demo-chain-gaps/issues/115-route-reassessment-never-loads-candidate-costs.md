# 115 路由复核不取候选成本：证据里的候选成本恒空，成本单维下候选评估恒未决、无路由那条线报错

Category: bug
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（票 [105](./105-route-plan-receives-cost-citations-of-unselected-candidates.md) 落地时读码发现；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘：NR `application/reassess_route.go`（`ReassessRouteDeps`、`reviewCandidates`、`formPlanFromEvidence`）；生产装配 `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 给复核接成本取数侧。
出处：票 [routing-first-cut/10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md)（成本族在取数侧兑现）；`ports.InitialRouteEvidence` 的注释「CandidateCosts 按形态要的维度给出逐候选事实，每个合格候选恰一条」。

## 现象（读码，钉 `4cf203fb`，证据 `S`）

- 候选成本只由初始路由编排向取数侧要：`CreateInitialRouteHandler.judgeParcel` 调 `RouteCandidateCostSource.LoadCandidateCosts`，把事实填进 `evidence.CandidateCosts`。全仓非测试代码里给 `CandidateCosts` 赋值的只此一处，证据视图 `CatalogInitialRouteEvidence` 明写「成本不在这里折」。
- 复核 `ReassessRouteHandler` 的依赖里没有成本取数侧，排序直接用证据视图交回的 `evidence.CandidateCosts`，生产上恒空。
- `RankRouteCandidates` 遇到合格候选没有成本事实答 `ErrInvalidRanking`（「什么都没说是证据装配漏了」）。路由策略版本声明了成本单维排序时（演示策略 `SYN-RS-CN-SG-01` 第 2 版就声明了），复核各条线因此：
  - 失效线：`reviewCandidates` 把错吞成 `CANDIDATE_REVIEW_UNDECIDED`，候选可用时才做的 7B/7C 改路评估整段不做——计划失效后从不形成改路建议或自动改路。
  - 原先无路由那条线（`reassessNoRoute`）与改善改路（`tryImprovementReroute`）：`formPlanFromEvidence` 上抛 `rank route candidates`，整次复核报错，而不是形成首个计划或记未决。
- 复核形成的新计划（首个计划、自动改路）不带逐段成本出处：`formPlanFromEvidence` 不交 `CostCitations`。初始路由的计划带（票 routing-first-cut/10）。

没人看见，是因为没人断言：交接复核的真库用例走到失效那一格时，按上面的路径候选评估应落未决，用例只断言失效本身；无路由那条线没有真库用例。

## 三类

机制。规模 S–M：复核接成本取数侧（与初始路由同一口、同一份携带内容），排序前填 `CandidateCosts`，形成计划时按票 105 的口径只取选中候选的出处；取数侧没接或读不回各归一个未决格，不吞成「候选评估未决」以外的东西。

## 要裁的

- 复核的未决原因集合要不要分出「成本取数侧没接 / 读不回」两格（初始路由有 `COST_SOURCE_NOT_CONFIGURED` 与 `COST_SOURCE_UNAVAILABLE`），还是在复核这一侧并入候选评估未决。按「恢复动作不同就分格」的既有口径应分，归 NR owner 确认。

## 既有票

routing-first-cut/10（候选成本取数侧）、demo-chain-gaps/105（计划只取选中候选的出处）、demo-chain-gaps/63（交接触发复核）。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/nr-no.md](../../docs-consistency/audit/nr-no.md) H3（归 C，高）与本票同一现象，落在本票，不另立。按 main `67438a0f` 复核，现象仍在：`application.ReassessRouteDeps` 仍没有成本取数口，生产证据视图 `LoadInitialRouteEvidence` 不填 `CandidateCosts`，`domain.RankRouteCandidates` 遇合格候选无成本事实答 `ErrInvalidRanking`。审计补的两点：

- 三条路的后果各不相同：`reassessNoRoute` 上抛、整封重投到 `ABANDONED`；`reviewCandidates` 吞成 `CANDIDATE_REVIEW_UNDECIDED`，失效之后永不形成改路决定或建议；`tryImprovementReroute` 在声明了自动改路时同样上抛。演示种子 `scripts/demo-seeds/data/network/23-route-strategy-cn-sg-v2.json` 声明了 `COST_SINGLE_DIMENSION`，所以今天就走得到。ADR-0235 越权风险点 12 写「今天两种都到不了」，这是第三种。
- 应用测试用 `routableEvidence`、`cheaperAlternative` 直接往证据里塞 `CandidateCosts`，所以是绿的；本票须补一条 cmd 层用真证据跑「先前无路由 → 首个计划」（`AT-NR-039`）的用例。

同笔并入 [audit/nr-no.md](../../docs-consistency/audit/nr-no.md) L15（两处 Go 注释与 ADR 相反或过时，本票地盘内）：
- `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 给 `AutoReroute` 的注释「登记过的键才走 7B/7C」，与 ADR-0173 决定四、NR CONTEXT「自动改路事实目录不再是判断权威，复核不读它」及 `ReassessRouteDeps.AutoReroute` 字段注释相反；
- `internal/networkrouting/ports/ports.go` 的 `RequestCarriedContent` 注释「首版只含地理解析投影；服务要求与承诺上界随 routing-first-cut/08、09 加入」，结构体已有 `Commitment`。
两处改成中文写现行规则，不留变更叙述。

分级建议（文档自洽 02）：R1——CONTEXT 写的首个计划、受控改路、改路建议在生产接法上都走不通，失效之后一律报「未决」误导运营，无绕法。由协调方定稿。修好之后 [docs-consistency/code/17](../../docs-consistency/code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md)（仍适用线不出改路建议）才会暴露。
