# 评价重放门无执行器：PN-08 W02 历史回放要的三件——按版本引用取原方案的读口、回放编排、治理触发面

Category: enhancement
Status: draft——MCP-6 2026-09-07 立票；基线 `ReplayPricingEvaluation` 理由行已同日改写为指向本票（分支 `mcp6-pp-ratchet` 的 `1d13d510`）
Blocked by: 无（三件全是机制半边；触发面的形状若要落 ADR，预留号已尽向 MCP-1 取号）

## 条目

`internal/parcelpricing/domain ReplayPricingEvaluation`（`evaluation.go`）。三分：**有意留待**（第三种）。它不是死码——它是 CONTEXT 里好几条硬句的唯一执行器；也不是「等租户」——它缺的是三层代码，不是取值。

## 它守的规则（`docs/domain/parcel-pricing/CONTEXT.md`「Rules and invariants」）

- 「同一评价输入、价格方向、计算目的、业务时间、版本清单和版本内容摘要必须产生相同结果；**重放必须使用新的评价引用，不得复用原评价 ID**，不修改原评价或来源事实。重算结果与原评价不一致时，结果为冲突，不得当作成功回放。」
- 「重放按原评价记录的规范化版本重新规范化后再比对摘要……原评价的规范化版本已不被当前实现支持因而无法重新规范化时，结果为未形成——这是结构上算不出来，不是版本内容冲突。」（ADR-0014）
- 「回放以原版本清单必须重现不可计价；结论不同即为冲突。」
- 「重放携带原输入快照内的取值与版本引用，不重新解析在用版本。」（ADR-0099 决定四）
- 评价「可服务比价、估价、成本预测、客户计费、**争议复核和回放**」。

`ReplayPricingEvaluation(newID, original, originalPlan, evidence)` 经 `NewReplayEvaluationRequest` 把原输入快照、原版本清单、原方案内容摘要与规范化版本一并带进请求（新 ID 不得等于原 ID；`S` 只能重放成 `S`），纯函数重算后：版本清单 / 方案内容不符或规范化版本不支持时原样交回（各自的问题项已在评价里），其余状态或语义摘要不一致即 `REPLAY_RESULT_MISMATCH` 冲突。

## 该有的调用方

**PN-08 W02「历史回放」的执行编排**（`docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md`：「使用可追溯、脱敏、版本化的历史来源事实，按对象和业务时间重放候选规则」，结果记 `R`）。`EvidenceKind` 里那格 `EvidenceReplay = "R"` 就是为它留的——今天全仓没有一处生产代码造出 `R`。CONTEXT 点名的「争议复核」走同一扇门，只是触发者不同。

开发主线对 PN-08 的划分：「保存候选清单、阶段决定……的**记录能力**是机制，实际执行回放、影子、限量生产并作出 Go/No-Go 是实例」——执行回放这一动作是实例，**能执行回放的执行器**是机制，本票做的是后者。

## 今天缺的三件

1. **按版本引用取回原方案的读口。** `ports.PriceCardCatalog` 只有 `LoadApplicable(tenant, direction, scope, asOf)`——「此刻适用的那一版」；重放要的是「原评价用的那一版」（`original.PlanReference()`），两者在方案换版之后不是同一张。要一个 `FindByReference(tenant, VersionReference)`（放在 `PriceCardCatalog` 上或另立读口），postgres 适配器读价卡登记表并经 `RehydratePriceCardRegistration` 整图重验；找不到那一版是「结构上重放不了」的一种，要如实答而不是退回在用版本。
2. **回放编排** `application/replay_pricing_evaluation.go`：`Store.FindByID(original)` → 取原方案 → `domain.ReplayPricingEvaluation(newID, original, plan, evidence)` → `Store.Save`。**不交 `EvaluationHandoff`**：回放结果不是新费用，SA 的 `AT-SA-173` 幂等只对同一评价成立，一份带新 ID 的回放交出去就是一笔重复费用采用。证据层级由调用方给、受 `NewReplayEvaluationRequest` 那道「`S` 只能重放成 `S`」约束；`R` 只在 W02 隔离执行下形成。
3. **治理触发面。** 形状待裁：HTTP 登记面 + 未配置即拒（ADR-0055 / ADR-0085 同形）还是 CLI（`cmd/parcel-*` 一族）；它是治理面不是客户面。裁形状那一格可能落 ADR。

三件同票落地那天剪基线行，记数照 `production_wiring_baseline.txt` 头注纪律。

## 别把它认成已接

`application/evaluate_pricing.go` 的 `settleAgainstExisting` 也会「重算一遍比摘要」——那是**同标识重复请求的冒名比对**：纯函数重算、比语义摘要、不铸新评价引用、不入册；与 CONTEXT 那条「重放必须使用新的评价引用」守的是两件事。

## 红线

- 不为接线造占位调用；三件缺一，`ReplayPricingEvaluation` 继续留在名单上。
- 回放不读在用序列 / 目录版本（`missingSeriesBindings` 与 `missingCatalogueLinks` 对重放请求已答「不缺」，编排不得绕过）。
- 生产装配未接治理面之前，`S` 是唯一走得到的证据层级；不写死任何回放数据范围（`PAR-GOV-02` 实例半边）。

## 边界

不改 `EvaluatePricing` 纯函数、不改 `NewReplayEvaluationRequest` 的判据；争议复核的业务触发（谁能发起、凭什么）归 SA/PC 侧另裁，本票只给执行器。
