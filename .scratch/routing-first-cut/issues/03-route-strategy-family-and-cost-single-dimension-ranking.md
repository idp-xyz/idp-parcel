# 03 路由策略版本声明排序形态；首个内置形态「满足硬约束后按成本单维择优，并列交人工」

Category: enhancement
Status: resolved · 已进 main——2026-09-24 评审 ← 通道 3（非作者）可接受、无阻断；通道 1、通道 2 已崩，通道 3 按用户令接手收尾并代行推送方重放进 main：认领 `3c98a4da`、代码笔 `e812e288` / `d4367ac0` / `af78b558`、完成记录 `238f748b`，清点 `4a3ad293`；分支 `mcp2-rfc03`（代码 tip `095724a6`、票面 tip `c73242c9`）作封存出处，新旧 SHA 对照见 Comments「进 main 记录」。迁移编号占 network_routing `0010`。此前 in-progress——通道 2 认领（通道 1 派单 task-389c2c39），同日交活；完成记录与评审原文见下文
Blocked by: 无
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「路由策略族」那一步的排序部分，与「首个内置排序策略」那一步
地盘：network-routing 领域与应用（排序与初始路由、复核两处择优出口）；目录路由策略版本的内容列与登记口（新迁移，号开工时在频道预留）；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md)、[UC-NR-001](../../../docs/application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md)、[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) 的相关句。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、七；[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-16` 已确认的机制句；[`label-channel-service-first-release/01`](../../label-channel-service-first-release/issues/01-channel-candidate-tie-break-authority.md) 的裁决。

## 做什么

1. **路由策略族**：路由策略版本声明它采用哪一种内置排序形态；形态的判断逻辑归产品，租户只选形态、填取值（ADR-0146 决定二）。首版族里只有一种形态。形态集合与校验落在领域，目录登记口经领域构造门收，未知形态拒登。
2. **首个内置形态**按 `PAR-NET-16` 已确认的那几句判：只在通过硬约束与时间可行性的合格候选里比；缺成本事实（待判断或不可计价）的候选出局，不以零或其他候选的金额顶替；币种不齐停下不比；最低成本并列且选不出唯一一条时交冲突，不按候选标识或任何无业务依据的次序收尾。
3. **并列的去处**：初始路由既不形成计划也不形成`无当前有效路由`，留痕全部候选与并列理由，等授权角色裁——人工选择入口不在本票（UC-NR-001「人工选择即使后续引入……」）；复核里并列只形成改路建议。两份 UC 的失败边界补这一格。
4. **候选成本事实的形状**（金额与币种，或待判断 / 不可计价）在本票定；它从哪里来归 02 与 10。

**要写明的一处前提变化。** label-channel/01 裁决时判路由侧「按标识升序收尾」仍然正确，前提是「那里是多维准则序，几乎不会全维打平」；ADR-0146 把首个内置形态定为成本单维，而那张票自己写过「单维恰恰最容易打平」。前提不再成立，本票据此改路由侧的并列出口，理由写进 CONTEXT 规则句，不改那张票的历史正文。

## 不做

- 不预选任何租户用哪种形态；不启用时效、可靠性等其他维度（`PAR-NET-16`：保持未配置）；不做人工裁决入口。

## 完成判据

- [x] 领域用例：缺成本出局、币种不齐停下、唯一最低者选中、最低并列交冲突，各一格。
- [x] 应用用例（内存替身）：初始路由并列时不落计划也不落无路由、结果可续办；复核并列只成建议。
- [x] 真库用例：路由策略版本带形态登记并读回；未知形态拒登。
- [x] CONTEXT 规则句与两份 UC 的失败边界同笔更新。

## 完成记录（2026-09-24，通道 2，分支 `mcp2-rfc03`）

**基线与重放**：分支基 `abc9088c`（派单时的远端 main）。认领笔 `60bbbf13` 只动票面 Status 一行；代码三笔 `d2abd929`、`eee29a45`、`095724a6`；清点笔 `4462bca3` 在 `eee29a45` 的干净检出上重生成，`095724a6` 之后重跑无差异；本笔为完成记录。重放取全部，推送方仍须在 tip 上重生成清点兑底。

**落点**

| 笔 | 做了什么 |
|---|---|
| `d2abd929` | 领域排序形态与成本单维择优；初始路由与复核两处择优出口的并列去处；CONTEXT 与 UC-NR-001 / UC-NR-003 同笔；类型可达性基线剪 `CriterionScore` |
| `eee29a45` | 路由策略版本的排序形态内容列（迁移 `network_routing/0010`）、应用受理门、登记 CLI 译装、适配器写入与两处读回、查阅体 |
| `4462bca3` | 机制清点重生成（迁移份数） |
| `095724a6` | 双轴自审修复（见文末） |

**形状要点**（供 NR owner 复核）

- `domain.RankingForm`：零值即未声明；首版族为 `COST_SINGLE_DIMENSION`，`RankingFormFrom` 逐格译回，族外词与空词都拒。
- `domain.CandidateCostFact`：已计价（最小币单位金额与币种）、待判断、不可计价，各有构造函数；没有金额就不带金额。
- `domain.RankRouteCandidates(form, candidates, costs)` 交封闭结局：`SELECTED`、`NO_QUALIFIED_CANDIDATE`、`RANKING_FORM_NOT_DECLARED`、`COSTS_PENDING`、`COSTS_UNPRICEABLE`、`CURRENCIES_DIFFER`、`TIED`。合格候选整条缺成本事实、同一候选两条事实是证据装配错误（`ErrInvalidRanking`）。
- `ports.InitialRouteEvidence` 的 `Scores` / `Priority` 换成 `RankingForm` + `CandidateCosts`（导出签名窗口已于频道报过）；多准则比较器 `SelectRouteCandidate` 与三个准则类型删除。
- 初始路由新增未决原因 `RANKING_FORM_NOT_CONFIGURED`、`CANDIDATES_TIED`、`CANDIDATE_COSTS_PENDING`、`CANDIDATE_COSTS_UNPRICEABLE`、`CANDIDATE_COST_CURRENCIES_DIFFER`；并列时 `ParcelRouteResult` 的 `Candidates()` 与 `TiedCandidates()` 交回全部候选与并列的那几家。
- 复核：自动条件都立而并列时记 `SUGGESTION_ONLY` 并形成建议，阻塞逐家 `LOWEST_COST_TIED/<候选>`；原先无路由的包裹并列时不形成首个计划，候选评估记 `CANDIDATE_REVIEW_UNDECIDED`。

**完成判据**（钉 `095724a6`，WSL，go1.26.8，真库为 55432 门禁库）

- ✅ 领域用例：唯一最低者选中 `TestCostSingleDimensionSelectsTheUniqueLowestAmongQualified`；缺成本出局 `TestCandidatesWithoutCostFactsAreExcludedNotZeroed`（另含全部缺成本的两格）；币种不齐停下 `TestCostsInDifferentCurrenciesStopTheComparison`；最低并列交冲突 `TestLowestCostTieIsHandedOverNotBrokenByIdentifier`。另有 `TestRankingFormsAreAClosedFamily`、`TestRankingRefusesToInventAnAnswer`。
- ✅ 应用用例（内存替身）：初始路由并列不落计划、不落无路由、可续办并交回候选 `TestALowestCostTieFormsNeitherPlanNorNoRoute`；复核并列只成建议 `TestALowestCostTieOnlyLeavesASuggestion`。另有原先无路由那条线 `TestAFormerNoRouteWithATiedLowestCostFormsNoFirstPlan`。
- ✅ 真库用例：`TestARouteStrategyVersionCarriesItsDeclaredRankingForm`——带形态登记读回、未声明读回、族外值适配器拒写且零行、库面 CHECK `route_strategy_version_ranking_form_known` 拦族外词。未知形态拒登另在应用受理门（专格 `RANKING_FORM_UNKNOWN`）与登记 CLI（`TestExecuteTranslatesTheDeclaredRankingForm` 与拒绝表两格）各钉一处。
- ✅ CONTEXT 规则句与两份 UC 的失败边界同笔（`d2abd929`；`095724a6` 改 CONTEXT 一处措辞）。
- 迁移 `0010` 无 CR、无 BOM（字节核过）。
- 门禁：全仓 `go build` / `go vet` 退 0；改动包与 `go list` 反查出的生产反向依赖（经 `migrations` 牵出各上下文的 postgres 适配器与全部 `cmd/*`）共 37 个目标，带 DSN `-p 1 -count=1` 全 ok；DSN 探针 `-v` 下 PASS 非 SKIP；`internal/architecture` 两道棘轮绿。自基线以来动过的 `.sql` 只有 `0010`，`.go` 见落点各笔。

**双轴自审**：`/code-review` 两轴按 workflow.md 在主会话串行做（本仓不用子代理），对基线 `abc9088c`。Standards 轴修五条：全部缺成本按 ADR-0029 分两格（此前合成一格，与领域注释自相矛盾）；`reviewCandidates` 逐格列全、只给认不出的结局留写明理由的兜底；头注去掉变更史；两处跨文件计数改写；删没有生产调用方的 `RouteRanking.Excluded`。修后复审无可处理发现。这不是非作者评审，非作者评审待推送方派。

**判断项**（交评审与推送方）

1. 原先无路由的包裹复核时并列，记候选评估未决而不挂改路建议：复核表约束 `route_reassessment_reroute_on_reviewed_plan` 只许改路三件出现在有被复核计划的记录上，要在这条线留建议得放宽该约束，超出本票地盘。票面「复核并列只成建议」在失效路径上成立。
2. 初始路由并列的留痕在结果里，不落库（NR 没有未决处理尝试的库）；经 dispatch 消费门时未决会回滚重投，烧完预算落 `ABANDONED`，与 `first-tenant-runway/07` 同形。候选成本事实今天没有来源（归 02 与 10），这一格在真进程上走不到；人工裁决入口要与它的续办触发一起立。
3. 查阅体 `/network-catalog?family=route-strategy` 透出 `rankingForm`（未声明整格缺席）：票面只要求真库读回，这一格按 ADR-0077「按族列版本行原文」顺带补上。
4. 全部缺成本分 `COSTS_PENDING` 与 `COSTS_UNPRICEABLE`、有一家待判断就先等：票面只说「出局」，分格依 ADR-0029，「先等」是本票的读法。
5. 地盘外、未动：`internal/parcelshipment/domain/channel_candidate_cost.go` 两段注释引已删的 `SelectRouteCandidate`，并说路由侧按候选标识升序收尾，本票之后不再成立，归 PS owner 改注释。

## Comments

### 评审 ← 通道 3 · 钉 `095724a6`（基 `abc9088c`，只读） · 2026-09-24（通道 1、通道 2 已崩，通道 3 按用户令接手收尾）

**Standards** — 阻断：无。非阻断：
1. 地盘外 `internal/parcelshipment/domain/channel_candidate_cost.go` 的注释仍引已删的 `SelectRouteCandidate`、说路由侧按候选标识收尾；本票进 main 后即成假话（作者判断项 5 已点名），归 PS owner 另笔改。
2. （判断）`domain.CandidateCostFact` 的币种用通用 `requiredValue`；首版可接受，/10 接比较币种时再看要不要立币种值对象。

其余信号：注释中文、无行号与跨文件计数；迁移 `0010` 只加可空列与 CHECK，头注写清为何可空、CHECK 与领域形态集合逐字同格。

**Spec** — 阻断：无。非阻断：
1. 做什么「并列的去处」要「留痕全部候选与并列理由」：候选随 `ParcelRouteResult.Candidates()` / `TiedCandidates()` 交回、不落库（作者判断项 2 已写：NR 没有未决尝试的库，候选成本今天也无来源）。持久化要随人工裁决入口一起立，立那张票时宜写进完成判据。
2. ADR-0148 决定四已按用户指示修订并接受（候选成本按比较币种合成）。CONTEXT 新规则句「已计价候选币种不齐时停下不比」今天成立——策略版本还没有比较币种一格，正是决定四「策略版本没登比较币种时……出现不同币种即停下不比」那一格；/10 补比较币种时要把这句限定上。比较币种与所引价格政策两项按 0148 Consequences 归 /10。

逐项：形态族与登记口拒族外 ✓；只比合格、缺成本出局不顶替、币种不齐停下、最低并列交冲突不按标识收尾 ✓；初始路由并列成未决、不落计划也不落无路由，复核并列只成建议 ✓——缺成本、币种不齐、形态未声明也都落未决，合 0148 决定四「不得落`无当前有效路由`」；成本事实形状 ✓；前提变化写进 CONTEXT 规则句 ✓。

结论：可接受——两轴无阻断。

### 进 main 记录（推送方 · 通道 3 代行，通道 1 已崩）

- **门**：评审 ← 通道 3（非作者）可接受、无阻断；上文四条非阻断随票记，不挡合入。
- **重放**：沿用通道 1 崩前在隔离检出 `/tmp/replay-rfc03` 上的试重放（main `256665ed` 之上 cherry-pick）：认领 `3c98a4da`（← `60bbbf13`）/ 之一 `e812e288`（← `d2abd929`）/ 之二 `d4367ac0`（← `eee29a45`）/ 修复 `af78b558`（← `095724a6`）/ 完成记录 `238f748b`（← `c73242c9`）。通道 3 接手时核过：分支各笔（清点笔除外）patch 等价，本票动过的 22 份代码文件 blob 全同。分支清点笔 `4462bca3` 不重放，批 tip 干净检出重生成为 `4a3ad293`。
- **验证**：钉 `4a3ad293`（与本记录一笔只差 `.md`）：全仓 build / vet 退 0，本票 `.go` gofmt 无输出；先单跑真库用例 `TestARouteStrategyVersionCarriesItsDeclaredRankingForm` 是 PASS 非 SKIP；带 DSN `go test -p 1 -count=1 ./...` 118 包 ok、0 FAIL（另 15 包无测试文件）。
