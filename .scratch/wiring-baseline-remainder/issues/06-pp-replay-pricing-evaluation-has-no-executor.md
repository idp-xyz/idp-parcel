# 评价重放门无执行器：PN-08 W02 历史回放要的三件——按版本引用取原方案的读口、回放编排、治理触发面

Category: enhancement
Status: resolved——MCP-2 2026-09-07 六笔在分支 `mcp2-wbr06`（基 main `299f2a2e`）落地：`2532d97b` 件① 读口 / `aad985cf` 件② 编排 + 基线剪行 / `6ee8c740` ADR-0124 / `2dd1708f` 件③ HTTP 面 / `64fe5bf2` 件③ 端点表装配 / `01514b05` 清点；本收口笔在其上。验证强度在文末「完成记录」；main 上的 SHA 待 MCP-1 重放后对照。此前：MCP-6 2026-09-07 立票（draft），基线 `ReplayPricingEvaluation` 理由行同日改写为指向本票（分支 `mcp6-pp-ratchet` 的 `1d13d510`，main 上为 `7cef122f`）；MCP-1 18:2x 派给 MCP-2（task-269d98b6），上一会话建了树、零提交，19:0x 新会话接续
Blocked by: 无（三件全是机制半边；触发面的形状已落 ADR-0124，号经 MCP-1 19:1x 确认）

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

## 三件的形状与理由（MCP-2 2026-09-07，task-269d98b6；按 MCP-1 派单「owner 授权自决口径」）

**① 读口：另立 `ports.PriceCardVersionLoader`，一个方法 `FindByReference(ctx, tenant, VersionReference) (PricingPlanVersion, bool, error)`；PG `PriceCards` 兼实现。** 不扩 `PriceCardCatalog`：扩写侧接口会拆全部写侧替身，`ReferenceSeriesVersionLoader` 就是同形先例（`catalogue_read.go` / `reference_series_register.go` 两处头注写过这条理由）。键是引用三元里的（标识、版本），对主键 `(tenant_id, plan_id, plan_version)`；不带方向 / 范围 / 时点条件——重放不选卡，原评价已经选过了。读回经 `RehydratePriceCardRegistration` 整图重验与比对列交叉核，两条读法（`LoadApplicable` 与本口）共用一份列面与重验（`priceCardRow.plan`）。**不在册答 false 不退回在用版本**；快照按另一套规范化折装、本构建重建不了时原样包出 `ErrCanonicalizationVersionUnsupported` 供编排分格；引用种类不是 `pricing-plan` 答 error——答 false 会让调用方读成「那一版不在册」而去登一张本就不该存在的卡。

**② 编排：`application.ReplayPricingEvaluationHandler`，结果代数八格。** 取原评价（租户不符视同不在册——评价标识全局唯一，读口不按租户过滤，租户在编排核；越权探针与真不存在同答）→ 按 `original.PlanReference()` 经①取原方案 → `domain.ReplayPricingEvaluation` → 入册。八格：`RECORDED` / `EXISTING_RESULT`（同回放引用重算后 replayOf、证据层级、语义摘要都同——摘要刻意不含前两样，所以单独比）/ `IDENTITY_CONFLICT` / `ORIGINAL_NOT_FOUND` / `PLAN_VERSION_NOT_ON_REGISTER` / `PLAN_CANONICALIZATION_UNSUPPORTED`（两格分开按恢复动作：前者能登，后者只能等构建）/ `NOT_ACCEPTED`（复用原引用、`S` 升级、缺格——都是改请求）/ `UNDECIDED` 带成因。**不交 `EvaluationHandoff`，且 `ReplayPricingEvaluationDeps` 结构上没有那一格**——`AT-SA-173` 的幂等键是评价标识，一份带新引用的回放交出去就是第二笔费用采用；用例用反射钉住依赖里没有交付口。编排不重新解析任何在用版本：原输入快照、原版本清单、原内容摘要与规范化版本都在原评价里，`missingSeriesBindings` / `missingCatalogueLinks` 那两格没被绕过（本编排根本不调它们）。`ReplayPricingEvaluationResult` 字段导出（形随 `ReferenceSeriesPreview`），传输层与测试能造任一格。

**③ 触发面：`parcel-api` 端点表上的 `POST /pricing-evaluation-replays`，字面量 `UnconfiguredIntake{}`；不开 CLI。** 裁决与理由全在 [ADR-0124](../../../docs/adr/0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md)：身份缝（触发者来自 `OperatorEnvelope`，CLI 只能读一个自报的触发者）、W02 批量是实例半边无输入可跑、命令三样（原引用 / 新引用 / 证据层级）由触发方声明一样不默认、`outcome` 只说编排、回放不交结算。`/domain-modeling` 走过：边界是 `parcel-pricing` 一个上下文内，回放的语言（新引用、原版本清单、冲突、未形成、`S`/`R`/`P`）CONTEXT 里已经齐了——**不加词条、不改硬句**；只裁触发面形状，落 ADR 是因为三条判据（难逆转：端点表一行与操作者面从此按它换真；无上下文读不出为什么不开 CLI；真有取舍：CLI / 两口并立 / 执行器铸引用 / 面上默认 `S` / 交 SA 带标记 / 并进评价编排六条被否）都成立。载荷逐字段表单（ADR-0101 决定八自裁：低频、结构简单）。

**基线行**：`ReplayPricingEvaluation` 随②同笔剪掉——棘轮的 `TestWiringBaselineHasNoStaleEntry` 在编排落地那一笔就红，剪与编排不同笔会让那一笔在干净检出上不过；票面「三件缺一不剪」禁的是为接线造占位调用，本编排是真调用方，③在同票紧随两笔落地。成因三分是第二种（全仓该名一处声明，新增的 `ReplayPricingEvaluation*` 全是前缀名，门禁比精确相等）。量在隔离树父提交 `2532d97b` 上两法同得剪前 6、本笔单独作用于其上剪后 5；parcel-pricing 组至此清空。

### 越权风险点（单列，供 MCP-1 / owner 复核）

- **ADR-0124 决定五引 `AT-SA-173` 判「回放交出去会被采用成第二笔费用」是从 SA 验收句推的**，没读 SA 消费 `OutboxEvaluationHandoff` 的适配器怎么认 `replayOf`。若 SA 侧已按 `replayOf` 跳过，决定仍成立（不交更安全），理由要改口。
- **决定三让触发方声明证据层级**，等于把「这次回放的来源算不算 `R`」交给操作者面；`R` 该由谁认定属 PN-08 治理（`W09` 评审）与 `PAR-GOV-01`，机制只保证不替他们默认、`S` 不升级由领域门守。
- **票面「红线」第三条写的是 `PAR-GOV-02`（影子运行范围）**，参数登记册里「历史回放数据范围」是 `PAR-GOV-01`；ADR-0124 引的是 `PAR-GOV-01`，票面原句不改（立票人的话），在此指出。
- **`ReplayPricingEvaluationResult` 改成导出字段**是形状选择不是裁决——`ReferenceSeriesPreview` 已这么做；若 owner 偏好访问器，传输层测试要另给一条构造路径。

## 完成记录（2026-09-07，MCP-2；分支 `mcp2-wbr06`，基线 main `299f2a2e`，不推——MCP-1 重放进 main）

| 笔 | SHA | 内容 |
|---|---|---|
| ① | `2532d97b` | `ports.PriceCardVersionLoader.FindByReference` + PG `PriceCards` 兼实现（两条读法共用 `priceCardRow.plan`）+ 真库用例两条（同方案 v1→v2 换版后 `LoadApplicable` 只答 v2 而 `FindByReference(v1)` 原样交回 v1；未登版本 / 别租户答 false、价表引用答 error） |
| ② | `aad985cf` | `application.ReplayPricingEvaluationHandler` 八格 + 用例四条（重现 / 结构上重放不了不形成评价 / 复用引用与 `S` 升级与缺租户拒 / 同版本引用内容变了入册为冲突）+ `minimalPlanPriced` 夹具；同笔剪 `production_wiring_baseline.txt` 的 `ReplayPricingEvaluation` 行（6→5，钉 `2532d97b`）并改写 PP 段理由与头注流水 |
| — | `6ee8c740` | ADR-0124 + `docs/adr/README.md` 一行 |
| ③a | `2dd1708f` | `adapters/http`：`EvaluationReplayIntake` / `EvaluationReplayer` / `NewReplayEvaluationEndpoint` / `DecodeEvaluationReplayPayload` + `Command(tenant)` / 封闭响应形状；`UnconfiguredIntake` 兼任；结果类型改导出字段；用例八条 |
| ③b | `64fe5bf2` | `cmd/parcel-api`：端点表 `/pricing-evaluation-replays` 行、`assemble_pricing_replay.go` 事务包装（依赖无交付口）、`unwiredEvaluationReplay` 占位、探针表一行、真库装配用例（登卡 → 形成 `S` 评价入册 → 回放 RECORDED 重现 → 同引用 EXISTING_RESULT → 原方案版本从未登记答 PLAN_VERSION_NOT_ON_REGISTER 不入册） |
| — | `01514b05` | 机制清点在 `64fe5bf2` 干净检出重生成：parcelpricing 生产 85→88 / 测试 84→86 / http 8→9 / 端点 10→11；cmd/ 生产 54→55 / 测试 76→77；合计生产 830→833 / 测试 778→780；接入面端点 101→102；端口声明 355→356，精确口径缺 9 不变 |
| — | 本笔 | 本票转 resolved + 本记录 |

**触及文件**：`internal/parcelpricing/ports/price_card_version_loader.go`（新）；`internal/parcelpricing/adapters/postgres/price_card_catalog.go`（+`_test.go`）；`internal/parcelpricing/application/replay_pricing_evaluation.go`（新，+`_test.go`）、`evaluate_pricing_test.go`（夹具抽 `minimalPlanPriced`）；`internal/parcelpricing/adapters/http/replay_evaluation.go`（新，+`_test.go`）、`unconfigured_intake.go`；`cmd/parcel-api/assemble_pricing_replay.go`（新，+`_test.go`）、`endpoints.go`、`endpoints_test.go`、`main.go`、`unwired_orchestration.go`；`internal/architecture/production_wiring_baseline.txt`（PP 段 + 头注一段）；`docs/adr/0124-*.md`（新）、`docs/adr/README.md`（一行）；`docs/product/MECHANISM-INVENTORY.md`（生成）；本票。**未碰**：`domain/`（`EvaluatePricing` 与 `NewReplayEvaluationRequest` 一字未动）、PP CONTEXT、迁移（无新表：评价册与价卡登记册两张既有表够用）、`apps/admin-web`、任何其他上下文。

**验收对照**（票面「今天缺的三件」逐条）：1 读口 ✓（①，不在册不退回在用版本 ✓，整图重验 ✓）；2 编排 ✓（②，不交 `EvaluationHandoff` 且结构上无那一格 ✓，证据层级由调用方给、`S` 只能重放成 `S` 由领域门守 ✓，不读在用序列 / 目录 ✓）；3 触发面 ✓（ADR-0124 + ③a/③b，HTTP 登记面形 + 未配置即拒；「治理面不是客户面」→ 路径按动词 `-replays`、触发者从操作者信封来）。红线三条：无占位调用 ✓；不绕过 `missingSeriesBindings` / `missingCatalogueLinks` ✓；`S` 是生产装配里唯一走得到的层级（端点答 403 直到操作者 Intake 就位）、未写死任何回放范围 ✓。边界两条 ✓。

**验证强度**（干净 detached 检出 `idp-parcel-mcp2-wbr06-verify` @ `01514b05`，验后拆、无残留）：`gofmt -l .` 零输出；`go build ./...`、`go vet ./...` 退 0；清点工具 vet / test 退 0，清点在该 SHA 重跑 `git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空；**含 DSN**（门禁容器 `127.0.0.1:55432`，healthy）`go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 no test files**（约 596s）。探针一反一正（`cmd/parcel-api -run TestTheWiredEvaluationReplayRecordsAgainstARealDatabase` + `parcelpricing/adapters/postgres -run TestPriceCardFindByReference`）：无 DSN `--- SKIP` 3 / `--- PASS` 0；含 DSN `--- SKIP` 0 / `--- PASS` 3。棘轮 `production_wiring_ratchet` 与其余架构门在该检出全绿（transaction_closure 门在途中拦过一次探针里的 `t.Fatalf`，已改在闭包外断言）。未跑 `-race`（本机走不了，CI 覆盖）。自基线 `299f2a2e` 以来动了 `.go`，未动 `.sql`。

**要 MCP-1 落的共享行**：`docs/adr/README.md` 0124 一行（0123 下方）、`cmd/parcel-api/endpoints.go` PP 组末尾一行 + `assembleBusinessEndpoints` 一个参数（`main.go` / `endpoints_test.go` 跟一行）、`production_wiring_baseline.txt` PP 段——重放进 main 时父提交若不是 `2532d97b`，基线条目数按新父提交重取。

**父 spec**：`wiring-baseline-remainder/spec.md` 状态行不由本票改，完工对齐归 MCP-1。

## Comments

- 2026-09-07 · MCP-2：三件齐、基线行已剪、ADR-0124 已落。W02 真要跑历史回放时缺的是实例半边（`PAR-GOV-01` 数据集、候选版本组）与一个批量驱动——驱动消费本票的 `ReplayPricingEvaluationHandler` 即可，另立票；争议复核的业务触发规则归 SA/PC；评价册查阅面要不要透出 `replayOf` 另立票。
