# 133 已完成但合计为零的评价被当成矛盾上抛，请求永远停在已提交

Category: bug
Status: in-progress——2026-10-26 评审阻断已修、修订轮完，待复评与进 main，分支 claude-dcg133 tip `4c352c49b`（其后一笔只写评审记录与修订轮；已变基到 origin/main `803b97e5f`）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg133` tip `22849dd6e`（变基前的 SHA）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（code/12+46 已进 main），分支 `claude-dcg133`（worktree `idp-parcel-dcg133`）；新 ADR 取 0306。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M12；ADR-0198 决定四与越权风险点 4（BUY 路 ADR-0200 同形）

## 现象与修法

现象、证据与修法的完整写法以复核报告 M12 一条为准（单一权威），此处只记要点：

把「已完成而合计为零」移出矛盾清单，请求册加「负向结束（零额）」：入账、不重投、不建费用行；补 UC-SA-002 一行 AT；BUY 路同形。须新 ADR。

## 完成记录（2026-10-26，作者：tops-20 派出的 Claude Code 子代理）

起点：`origin/main` `1a8a51edc`（含认领）。单一权威：复核报告 M12，票面与报告无相抵；报告给的「至少做到入账不重投、在请求册上可辨认」那一档没有取，做全。

### 提交

| SHA | 内容 |
|---|---|
| `c601eaed6` | 代码、迁移与用例：形成门零额一格、两本请求册的`负向结束（零额）`、BUY 零额写口与装配、迁移 `settlement_accounting/0048` |
| `22849dd6e` | 文档：ADR-0306，ADR-0198 / ADR-0200 前向指针，SA `CONTEXT` 两句，UC-SA-002，AT 盘点 |
| 本笔 | 本完成记录 |

### 改了什么

- **领域**（SA）：`ErrZeroEvaluationTotal`——`FormCustomerCharge`、`FormSupplierExpectedCost`、`SupplierExpectedCost.AppendCorrection` 对结算币合计为零另答这一格，不成行；币种、跨币种换算步骤、同币种两额相等先判，负数与原币为零而结算币不为零照旧未受理（`zeroTotalGate`）。SELL 请求册状态 `SellRequestZeroTotal`（`ZERO_TOTAL`）、`NewZeroTotalSellEvaluationRequestConclusion`、`SellEvaluationRequestState.CarriesStopCause`。
- **应用**（SA）：`SellChargeZeroTotal` → `FormEstimatedCustomerChargeHandler` 记`负向结束（零额）`；`ExpectedCostZeroTotal` → `FormRequestedSupplierExpectedCostHandler` 经新依赖 `ZeroTotals` 记零额，册上已有停因上抛 `ErrBuyEvaluationRequestStoppedBeforeZeroTotal`，写口答不出为未决；外层 `RequestedSupplierExpectedCostZeroTotal`。
- **端口 / 适配器**：`ports.EvaluationRequestZeroTotalRecorder`、`EvaluationRequestRecord.ZeroTotal`；`postgres.EvaluationRequests.RecordZeroTotal`，`RecordStop` 不覆盖零额，`Save` 拒带零额的登记；SELL 册重建按 `CarriesStopCause` 复核；`sapricing.FormOnBuyEvaluationRecordedAdapter` 对零额入账。
- **装配**：`cmd/parcel-dispatch/assemble.go` 的 `formSupplierExpectedCostOnBuyEvaluationConsumer` 给外层接 `ZeroTotals: requests`。别的 `cmd/` 没有装这只外层。
- **迁移**：`migrations/settlement_accounting/0048_evaluation_request_zero_total_end.sql`（取 0048、空着 0047 给在途的 code/94）：SELL 状态格与停因配对两条 CHECK 收 `ZERO_TOTAL`；BUY 请求册加 `negative_end`，只收 `ZERO_TOTAL`、与停因互斥。
- **计价**（PP）：代码不改；`internal/parcelpricing/domain/evaluation_test.go` 补一条用例钉住扣减恰到零落成已完成。
- **文档**：ADR-0306（新）；ADR-0198、ADR-0200 只改 Status 行索引注记与 Links；SA `CONTEXT` 两句（「计价、组合与精度」SELL 请求结论那一条、「供应商成本与共享分摊」BUY 首版那一条）；UC-SA-002 元数据「负向结束」、主流程步 4、`AT-SA-184`、新 `AT-SA-215`；`.scratch/at-coverage-inventory.md` 补 `AT-SA-215` 入①，`AT-SA-184` 那一行（②）随场景改写。ADR 索引、AT 总览计数与机制清点未重生成（推送方的事）。

### 代决与越权风险点

1. **零额按合同结算币判**：合计取评价在结算币上的那个数（跨币种时是换算后），原币不为零而换算取整为零也落零额。读了 PP `evaluation.go` 合计与换算那一段、SA 两只读口的金额换写；没读计价的取整策略全集。owner 不同意改 `zeroTotalGate` 一处（ADR-0306 越权风险点 4）。
2. **零额判在映射之后**（SELL）：映射不全照记`待判断`，补登映射续办出的下一代再答零额。理由是零额只由形成门一处判、外层不碰金额。owner 要零额先说话，改 `FormEstimatedCustomerChargeHandler` 的次序（越权风险点 2）。
3. **零额不续办**：不带停因，参数登记与运营续办都不推；卡登错要改价，本票不开路。owner 不同意改续办队列「等着」的口径与运营续办口（越权风险点 1）。
4. **BUY 只加一列 `negative_end`**，不补完整状态格；与停因互斥，撞上作矛盾上抛。owner 要完整状态格另立迁移（ADR-0306「候选与反方」）。
5. **替代版本零额不接纠错版本**：链头停在失效纠错版本，当前有效预期成本为空；判在发生项失效与「失效未记到结算」等待之后。owner 要「重述为零」的版本，改 `AppendCorrection` 的零额一格与库上 `0008` 的金额 CHECK（越权风险点 3）。
6. **迁移前停在`已提交`的零额请求不回补**（越权风险点 5）；上线前没有租户。
7. **合计不为零的「已完成却未受理」仍上抛**：ADR-0306 只停用两份记录里「例如合计为零」那一句，其余矛盾格不动。
8. **SA `CONTEXT` 改了两条里各一句**：派单说「只改那一句」，BUY 路同形一并修，BUY 的现行规则在「供应商成本与共享分摊」那一条，不改它 `CONTEXT` 就落后于代码；两处都离「运营余额、收付款与坏账」很远，没碰资金映射、收付款入账与 `CustomerReceiptPostings`。
9. **AT 号**：`AT-SA-215`（全仓与在途分支 grep 不撞，UC-SA-002 原最大号 214）；`AT-SA-184` 改记、号不变。

### 完成判据 / 修法 → 用例

| 复核报告 M12 修法 | 落点 |
|---|---|
| 把「已完成而合计为零」移出决定四的矛盾清单（新 ADR、部分停用、指针、`CONTEXT` 同笔改） | ADR-0306「对已接受记录的作用」；ADR-0198 / ADR-0200 Status 行与 Links；SA `CONTEXT` 两句 |
| 请求册加一格「负向结束（零额）」：入账、不重投、不建费用行 | `AT-SA-215`：`TestACompletedEvaluationTotallingZeroEndsTheRequestWithoutACharge`、`TestTheSellGateEndsTheRequestWithoutAChargeForAZeroTotal`（真算的计价评价）、`TestASellRequestEndsAtZeroTotalOnceWithoutChargeOrStopCause`（真库） |
| 补 UC-SA-002 一行 AT | `AT-SA-215`；元数据「负向结束」、主流程步 4 补指针 |
| BUY 路同形一并修（ADR-0200、AT-SA-184） | `TestACompletedBuyEvaluationTotallingZeroEndsTheRequestWithoutACost`、`TestASupersedingEvaluationTotallingZeroAppendsNoCorrection`、`TestABuyRequestEndsAtZeroTotalExclusiveOfAStopCause`（真库）、`TestABuyEvaluationTotallingZeroEndsTheRequestThroughTheGate`（真库消费门、inbox 记账）；`AT-SA-184` 改记，`TestContradictionsAreErrorsNotUndecided` 改钉合计不为零的未受理 |
| 零额不吞矛盾、不进续办队列 | `TestACustomerChargeTotallingZeroIsAZeroTotalNotAnInvalidCharge`、`TestASupplierCostTotallingZeroIsAZeroTotalForTheFirstVersionAndACorrection`、`TestTheRegistersRefuseAZeroTotalPairedWithAStopCause`（库上 CHECK）、`TestABuyRequestEndsAtZeroTotalExclusiveOfAStopCause`（队列取不到） |
| PP 只在合计 < 0 时答失败 | `TestEvaluatePricingCompletesWithAZeroTotalWhenADeductionExactlyEqualsTheSubtotal` |

### 自验（含 PG，DSN `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 通过；改过与新写的 `.go` 已 `gofmt -w`，`gofmt -l internal cmd` 为空。
- `go test -p 1 -count=1`（带 DSN）：`./internal/settlementaccounting/...`、`./internal/parcelpricing/...`、反查出依赖二者（含测试依赖）的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-pricing-feed/...`、`./cmd/parcel-pricing-register/...`、`./cmd/parcel-pricing-template/...`、`./cmd/parcel-settlement-register/...`，`./internal/architecture/...`、`./migrations/...`——33 个包全部 ok，无 FAIL。新写的真库用例另以 `-v` 确认 PASS 而非 SKIP；库上 CHECK 那一条临时打印过报错，三条各撞对应约束。
- `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物过期（退 1），不报缺行或格式错。
- 全量 `scripts/test-full.sh` 与种子灌库未跑（推送方的事）。

## 评审记录

照抄 `review-133.md`（隔离评审，钉 `5c7c40f60`；小节标题降一级）：

评审 ← tops-20 隔离子代理 · 钉 `5c7c40f60` · 02:02

结论：有阻断 2 条（Standards D1、Spec S1），都改得动、范围小。

### Standards
阻断：
- D1 ADR-0232 决定八被收窄却没挂部分停用指针。决定八原句「这份评价按 `AppendCorrection` 重述全额、接在链头后面」，又写「纠错由**发生项新版本**触发，不由评价结果不同触发；同一规则版本、同一金额也追加」。ADR-0306 决定三改成合计为零就不追加——按 adr/README「收窄……都是部分停用」与 AGENTS.md「改文档」属部分停用。现状：ADR-0232 Status 行与 Links 无 0306；ADR-0306「对已接受记录的作用」只列 0198、0200。SA CONTEXT「发生项某一版失效时」那一条照旧无条件写「接在失效纠错版本后面重述全额……计价结果与被纠正那一版相同也追加」（作者把例外写进了 BUY 首版那条，管这条规则的那句没改）。`FormSupplierExpectedCostHandler.Handle` 头注「金额同不同都追加」同样过时。修法：ADR-0306 前段与正文补去向；ADR-0232 补注记与 Links；CONTEXT 那句改成现行规则；注释同步。
非阻断：
- 迁移号 0047 那句话不成立：code/94 tip `427a9d1d0` 完成记录写「不加迁移（协调方预留的 SA 0047 未用）」，可迁移 0048 头注与 ADR-0306 决定四说「0047 留给在途的另一张结算票」。功能无碍（`checkSequence` 只要求正序号、`migrate.Run` 按已记录 ID 施加），但 ADR 接受即冻结——合入前改成 0047 或删这句。
- ADR-0169 只论了半句：ADR-0306 Links 判「不相抵」只引后半句；前半句「已完成的 SELL 评价形成预估客户费用」在合计为零时也不成立。分量轻（ADR-0198 早已把零额挡成矛盾），判要不要挂指针。
无发现：ADR-0198、0200 被停两句逐字、适用场景只限合计为零、去向/来向格式合规；SA CONTEXT 改 BUY 那句必要（BUY 请求册新增一格属生命周期变化，M12 要求同形一并修），没碰资金那一节；迁移 CHECK 三值逻辑正确、存量施加不失败、SELL 两条 CHECK 与 0042 比只多 `ZERO_TOTAL`；与 code/94 不交叠；号不撞；盘点归桶属实；注释中文、architecture 通过、gofmt 空。

### Spec
阻断：
- S1 BUY 那一路零额吞掉「派生版本上已有另一份内容」这格矛盾：`FormSupplierExpectedCostHandler.Handle` 碰 `ErrZeroEvaluationTotal` 直接答 `ExpectedCostZeroTotal`，排在 `Costs.LoadExpectedCost(version)` 的内容比对之前。探针复现：同一请求先形成非零首版，再来一份零额评价 → `BUY_EVALUATION_ZERO_TOTAL`、err nil，请求册记 `ZeroTotal=true`，而 `BUYSC-EVREQ-SYN-1` 那行成本还在。与改写后的 `AT-SA-184`（无条件上抛）及 ADR-0306 决定一「照旧上抛」相反——与 UC 相抵记阻断。只有库被改过或另有路占了标识时才走得到，但这道检查本就为抓这种情况设。修法：零额那一格先按派生版本查一次，查到就答冲突。
非阻断：
- `AT-SA-215` 没写依据去哪：两本册上零额都不存评价引用与时刻，依据只能经计价按回指读回；生命周期要求「必须保存依据」，仿 `AT-SA-214`、`AT-SA-183` 写明这一例外。
无发现：零额判定次序（三处形成门零额一格都在币种、换算步骤、同币种两额相等之后；负数、原币零而结算币非零照旧未受理；SELL 外层回指不在册、结算币种对不上在形成前判）；代决 1 站得住（结算侧不取整，`minorUnitsOf` 超位即拒，结算币的零来自计价按卡声明合计取整，合 ADR-0107 决定五，ADR-0306 风险点 4 已披露）；代决 2 与 `AT-SA-174/214/215` 一致；BUY 互斥与并发四处都对（咨询锁串行、行锁兜底、重放答 Replayed）；代决 5 不是新形状。

### 取证
`go vet`；SA domain、application、adapters/parcelpricing go test；带 DSN 跑 postgres、inbox 零额用例；architecture——全过。探针：scratchpad/probe133/zero_swallow_probe_test.go；修订轮重跑 `-overlay scratchpad/probe133/overlay.json -run TestReviewProbeZeroAfterExistingDerivedVersion -v ./internal/settlementaccounting/application/`，修好后应 err 为 `ErrSupplierCostVersionContradiction`、请求册不记零额。

## 修订轮（2026-10-26，作者）

变基：`git rebase origin/main`（`803b97e5f`），无冲突（AT 盘点两边改的是不同行，自动合并）；原三笔变基后为 `18a383d6c`、`71dee17d5`、`64b7602f1`。变基后 `go run ./tools/at-coverage-counts -check` 只报生成物过期（退 1）。

| SHA | 内容 |
|---|---|
| `16a669166` | S1：BUY 零额先按派生版本查；D1 注释半边；迁移改号 0047 |
| `4c352c49b` | D1：ADR-0306 / ADR-0232 / ADR-0169 指针与 SA `CONTEXT`；ADR-0169 前半句；`AT-SA-215` 依据例外；AT 盘点 |
| 本笔 | 评审记录与修订轮 |

逐条：

- **D1（阻断）**：ADR-0306 前段补「部分停用 ADR-0232 决定八两句」，「对已接受记录的作用」逐字引「这份评价按 `AppendCorrection` 重述全额、接在链头后面」与「纠错由**发生项新版本**触发，不由评价结果不同触发；同一规则版本、同一金额也追加」，适用场景只限替代版本的新评价合计为零；ADR-0232 只改 Status 行索引注记（追加一段来向）与 Links，正文不动；SA `CONTEXT`「发生项某一版失效时」那一条改成现行规则；`FormSupplierExpectedCostHandler.Handle` 里那段注释同步。
- **S1（阻断）**：`FormSupplierExpectedCostHandler.Handle` 碰到 `ErrZeroEvaluationTotal` 先 `Costs.LoadExpectedCost(version)`：查到就答 `ExpectedCostConflict`（外层上抛 `ErrSupplierCostVersionContradiction`），不记零额；读不出答未决 `EXPECTED_COST_REGISTRY_UNAVAILABLE`。派生版本上只要有行就是矛盾——零额不成行，没有「同一份内容」可比。用例 `TestAZeroTotalOnADerivedVersionThatAlreadyCarriesACostIsAContradiction`。
  - **SELL 一并核**：没有同形问题。ADR-0198 决定四的矛盾清单里没有「派生费用上已有另一份内容」；与它同位的「请求册上已有另一份结论」判在零额之前（请求不是`已提交`即答早有结论、零写入；记结论撞上另一份上抛）。用例 `TestAZeroTotalDoesNotSwallowAnotherConclusionOnTheSellRequest` 钉住，代码不改。请求`已提交`而派生费用已在库上的那一格，费用行与改记同一事务、外键先落，生产路径走不到；非零那一路对它也不比内容，本票不改。
- **非阻断·迁移号**：`0048_evaluation_request_zero_total_end.sql` 改名 `0047_…`，SQL 不变，删去头注「0047 留给另一张结算票」一句；ADR-0306 决定四删同一句，注记与 Consequences 改号。
- **非阻断·ADR-0169**：判为要挂。决定二「已完成的 SELL 评价形成预估客户费用。」在合计为零时不成立，属收窄，照部分停用办：ADR-0306 前段与「对已接受记录的作用」补去向，ADR-0169 Status 行注记与 Links 加前向指针；SA `CONTEXT` 同义的那一句改为「已完成且合计不为零的」。同一条后几句（金额整组出自评价、命令不带金额、未完成不落零额）不停用。
- **非阻断·AT-SA-215**：仿 `AT-SA-214`、`AT-SA-183` 写明例外——生命周期「费用明细」一节「不成立……必须保存依据」在本路不落册：零额那一格不存评价引用与时刻，依据留在评价上、经计价按回指读回。

本轮 SA `CONTEXT` 新改的句子：

1. 「计价、组合与精度」SELL 那一条首句：「已完成的 SELL·客户费用评价形成预估客户费用，金额整组出自该评价。」→「已完成且合计不为零的 SELL·客户费用评价形成预估客户费用，金额整组出自该评价。」
2. 「供应商成本与共享分摊」「发生项某一版失效时」那一条：「纠错由发生项新版本触发，计价结果与被纠正那一版相同也追加；」→「纠错由发生项新版本触发，计价结果与被纠正那一版相同也追加，替代版本的评价合计为零则不追加、链头停在失效纠错版本（ADR-0306）；」

复验（修订后 tip，含 PG）：

- 评审探针：另起 `probe133/overlay-dcg133.json`（Replace 键换成本树路径，原件不动），`go test -overlay … -run TestReviewProbeZeroAfterExistingDerivedVersion -v ./internal/settlementaccounting/application/` → `err=…the derived supplier cost version already carries other content: version BUYSC-EVREQ-SYN-1`（`ErrSupplierCostVersionContradiction`）、`zeroTotalOnRegister=false`、`saves=1`。
- `go build ./...`、`go vet ./...` 通过，`gofmt -l internal cmd` 为空。
- 带 DSN `go test -p 1 -count=1`：SA、PP 全部包，反查的 `cmd/parcel-api`、`parcel-dispatch`、`parcel-pricing-feed`、`parcel-pricing-register`、`parcel-pricing-template`、`parcel-settlement-register`，`internal/architecture`、`migrations`——33 个包全 ok；零额的真库用例 `-v` 确认 PASS。
- `adr-index -check`、`at-coverage-counts -check` 只报生成物过期。
