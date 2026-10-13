# 28 票 25 评审尾巴：估价停格分清溢出与负数、申报单位与正式评价同口径、留痕时点跨重驱的前提

Category: enhancement
Status: 完工，待评审与重放——2026-10-13 19:4x 通道 3（派单 `task-5b451348` ← 通道 1）：做什么三条落在 `mcp3-psb28`，代码 tip `d819231f`；没有增删文件，机制清点在 `d819231f` 上重生成零差，无清点笔。见文末「完成记录」。不自评、不推 main，非作者评审与重放归通道 1。此前 in-progress——2026-10-13 19:31 通道 3 认领（派单 `task-5b451348` ← 通道 1），分支 `mcp3-psb28`，基 `d4a3779c`（= origin/main），隔离 worktree `idp-parcel-mcp3-psb28`。此前 ready-for-agent——2026-10-13 19:3x 通道 1 立（用户 19:2x 经队列点头「按你的建议开始吧」），出自[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)两段非作者评审的非阻断：第二段 Standards 2、3 与 Spec 1（`task-386c6296`），其中 Standards 2、3 即第一段 Standards ②、①（`task-c7cfb347`），原文摘在票 25 的 Comments。
Blocked by: 无
地盘：`internal/settlementaccounting/domain`（`FormPerParcelSellTrialSum` 与停格）、`internal/settlementaccounting/application`（`NotFormedReason` 与停格到原因的映射）、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`（只动 `notFormedOutcomeFor` 跟新原因）、`internal/parcelpricing/adapters/parcelshipment`（`measurementOf`）、必要时 `internal/parcelpricing/adapters/transportfulfillment/pricing_input.go` 的 `weightUnit` / `lengthUnit`、`internal/settlementaccounting/adapters/postgres` 估价留痕册的用例与头注、`cmd/parcel-dispatch` 的纵向用例；各处用例随改。不碰 PS 领域、PS 提交版本申报读口与 PS 编排（[票 27](./27-pre-acceptance-control-reads-only-the-current-submission-version.md)的地盘）。
出处：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定二（停法）、决定三（与正式 SELL 评价同口径）、决定六（留痕）；票 25 判断项 8、9。

## 现象（读码，钉 main `6e4d9a1d`）

1. SA 领域 `FormPerParcelSellTrialSum` 对单份为负、加不进 int64 与合计不是正数同答 `PreAcceptanceEstimateNotPositive`，SA 应用层照映为 `EstimateNotPositive`（`ESTIMATE_NOT_POSITIVE`），经 PS `notFormedOutcomeFor` 落进处理尝试与续办引用。票 25 判断项 9 说分格「是为了续办引用与处理尝试里留下停在哪一格」，这一格把三种停法并成了一个。决定二只写了「合计不是正数」，单份为负而合计为正时也停，是多出来的一格。
2. PP `adapters/parcelshipment` 的 `measurementOf` 先 `strings.ToUpper` 再认单位；正式 SELL 输入解析 `adapters/transportfulfillment/pricing_input.go` 的 `weightUnit` / `lengthUnit` 只认大写原词。申报 `kg` 时受理前估得出冻结额，受理后正式评价却报单位在表外。`cmd/parcel-dispatch` 的 `preAcceptanceTrials` 把受理前试算接上生产图之后，这一差生产可达。
3. 估价留痕册 `PreAcceptanceEstimateTraces.RecordEstimate` 判重放与冲突时连 `trial_at` 一起比，两本账不比时刻。票 25 判断项 8：组合策略下冻结已落、账期那一项停`未配置`时事务照样提交，留痕随之已落；登记后重驱若换了判断时点，同一请求身份撞留痕冲突、落`请求冲突`，推不动。今天唯一形得成的时点语义是「提交接收」（PS `adapters/partycommercial` 的 `AsOfValueSource` 头注：折成该提交版本的系统接收时间），跨重驱不变——但票 25 没举证，也没写下这个前提。

## 做什么

1. 停格分清：溢出与单份为负各给一格，领域停格、SA 原因、PS 映射三处同笔，PS 侧仍落`未形成`。按决定二，单份为负而合计为正不在停法里；若留现状照停，写判断项并标越权风险点。
2. 单位同口径：`measurementOf` 与正式评价的单位解析取同一口径。两条路选一条并写判断项——受理前也不折叠（`kg` 当表外单位，与正式评价同停），或两边都折叠（改的是正式 SELL 评价的输入解析，那一侧既有用例要跟）。前者改动小、不动正式评价的行为。
3. 留痕时点：补一条用例，钉住同一请求在「提交接收」语义下跨重驱是重放、不是冲突（纵向或 SA 适配器层，开工时核）；在留痕端口或册的头注写明这个前提——换成会随重驱移动的时点语义时，这里会落`请求冲突`。

## 不做

- 不改 ADR-0179；读法与决定二、三对不上时停下报通道 1。
- 不碰票 27 的地盘；PS `pre_acceptance_control.go` 只动 `notFormedOutcomeFor`。
- 不改留痕表结构，迁移 `0035` 不动。

## 完成判据

- 第 1 条：三种停法各有用例、各落不同原因串；把任一格并回 `NOT_POSITIVE` 的变异转红。
- 第 2 条：同一份申报 `kg` 在受理前与正式评价两条路得同一答复，有用例。
- 第 3 条：用例为 PASS 非 SKIP（真库那条带 DSN `-v` 确认）；头注写明前提。
- 架构门禁、机制清点、受影响包组绿，口径照 [parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「全量只跑一次」。证据层级只记 `S`。

## 完成记录（通道 3 · 2026-10-13 · 派单 `task-5b451348` ← 通道 1 · 分支 `mcp3-psb28` 基 `d4a3779c`，代码 tip `d819231f`）

做什么三条全部落地，各先写 red 再改、每笔编得过、每笔后推分支。证据层级一律 `S`。

### 各笔

| SHA | 内容 |
|---|---|
| `93b5287b` | 认领：Status 行 |
| `8704376e` | 第 1 条：领域 `FormPerParcelSellTrialSum` 对单份为负答 `PreAcceptanceEstimateTrialNegative`、对合计加不进 int64 答 `PreAcceptanceEstimateSumOverflow`；SA `NotFormedReason` 加 `EstimateTrialNegative`（`ESTIMATE_TRIAL_NEGATIVE`）与 `EstimateSumOverflow`（`ESTIMATE_SUM_OVERFLOW`）；PS `notFormedOutcomeFor` 两格落`未形成`。三处同笔 |
| `d5d0d1bb` | 第 2 条：PP `adapters/parcelshipment` 的 `measurementOf` 去掉 `strings.ToUpper`，单位按原词进计价单位表；正式评价 `adapters/transportfulfillment` 一行生产代码没动，只加一条用例钉住它对小写 `kg` 答表外 |
| `d819231f` | 第 3 条：SA `adapters/postgres` 真库用例钉跨重驱的重放与冲突；`ports.PreAcceptanceEstimateTraces` 与册的 `RecordEstimate` 头注写明前提。不改行为、不动迁移 `0035` |

### 完成判据

- ✅ 第 1 条：三种停法各有用例、各落不同原因串。领域 `TestAPerParcelSellTrialSumStopsInsteadOfFillingAGap`：合计为零 → `NOT_POSITIVE`，一份为负而合计仍为正 → `TRIAL_NEGATIVE`，加不进 int64 → `SUM_OVERFLOW`，另核各停格名字非空、两两不同。SA `TestEveryEstimateCellStopsBeforeTouchingEitherLedger` 三格各落 `ESTIMATE_NOT_POSITIVE` / `ESTIMATE_TRIAL_NEGATIVE` / `ESTIMATE_SUM_OVERFLOW`，用例自带的「原因两两不同」照守，都在碰两本账之前停。PS `TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction` 两格为 `SA-ESTIMATE_TRIAL_NEGATIVE`、`SA-ESTIMATE_SUM_OVERFLOW`，都落`未形成`。变异（`go test -overlay`，副本在本机 `/tmp/psb28-m1a`…`m1d`）：领域两格、SA 应用层 `estimateGapReason` 两处映射，任一并回 `NOT_POSITIVE`，三个包都转红；基线绿。
- ✅ 第 2 条：同一份申报 `kg` 在两条路得同一答复——都当表外单位、不计价，两条路各一条用例，喂的是同一种 PS 申报值。受理前 `TestAnUntranslatableUnitLeavesTheParcelUnmeasured` 的「重量单位小写」「外廓单位小写」两格：整件当没有测量交回，入口据此停在缺件（`MEASUREMENT` → `ESTIMATE_DECLARATION_INCOMPLETE`）。正式评价 `TestALowercaseDeclaredUnitIsOutsideTheTableForTheCustomerCharge`（`CUSTOMER_CHARGE`）：`PricingInputUnavailable`，缺项点名 `kg`。没写成一条跨两路的用例：正式评价的单位解析 `weightUnit` / `lengthUnit` 是包内未导出函数，两个适配器同级，互不导入。改前代码即变异：受理前那两格红，改后绿。原用例 `TestASubmissionDeclarationTranslatesEachParcelAndTheRoute` 改报大写原词。
- ✅ 第 3 条：`TestARedriveUnderTheSubmissionReceiptAsOfIsAReplayWhileAMovingAsOfConflicts`（带 DSN `-v` 为 `PASS`，非 `SKIP`）：同一控制请求首轮落一行；两轮重驱的判断时点取同一个「提交接收」时刻、只有落库时刻不同，都答重放；判断时点换成重驱那一刻即答冲突，已落那一行的 `trial_at` 不动。头注写在端口 `PreAcceptanceEstimateTraces`（判断时点算内容；`SubmissionReceiptAsOf` 不读本地时钟，满足前提；换成会随重驱移动的时点语义时，前一轮已提交留痕的委托第二轮停在`请求冲突`，两本账对同额却是重放）与册的 `RecordEstimate`（`trial_at` 在比较之列、`recorded_at` 不在）。变异（副本在 `/tmp/psb28-m3a`、`m3b`）：比较里去掉 `trial_at`，「时点随重驱移动」转红；把落库时刻当判断时点存，「登记后重驱」转红。
- ✅ 架构门禁、机制清点、受影响包组绿，见下「验证」。

### 判断项与越权风险点

1. **越权风险点 · 待 owner 复核（SA）**：单份为负而合计仍为正照旧停，落 `ESTIMATE_TRIAL_NEGATIVE`。决定二只写了「合计不是正数」，这一格是本形态的取舍，写在 `FormPerParcelSellTrialSum` 头注：一件的 SELL 试算为负，是价卡对那一件给了一笔返还；加进合计会让冻结少于其余各件试算之和，等于拿这一件的负数去抵另几件该占的资金。停下的代价是这类委托停在`未形成`。owner 若认为负数件该照加，改法是删领域里那一格；SA 原因与 PS 映射随之删，用例照改。
2. 判断项：合计加不进 int64 仍是业务停（`ESTIMATE_SUM_OVERFLOW`，落`未形成`），不升格为编程错误：那个数出自价卡，不是适配器译坏了。处置与原先并在「非正」里时相同，只是名字分开。
3. 判断项：负数件先停之后，「合计不是正数」只剩各件都为零这一种来路。名字仍叫 `NOT_POSITIVE`、原因串仍是 `ESTIMATE_NOT_POSITIVE`，不改名：原因串会落进处理尝试与续办引用，改名会让前后两批记录对不上。
4. 判断项（第 2 条）：两条路选了不折叠。理由：不动正式 SELL 评价的行为；网络路由读申报那一路（`networkrouting` 的 `adapters/parcelpricing`，用例「单位不在表内」用的正是小写 `kg`）同样按原词认，受理前原是唯一折叠的一处。代价：申报小写单位的委托在受理前就停在缺件，而不是先冻结、受理后再评不出；恢复动作是客户按原词改报。大小写算不算同一单位是 PP 的产品策略；要折叠就两边同一笔改，先改正式评价那一侧，那时要标越权风险点（PP）。
5. 判断项（第 3 条）：用例落在 SA 适配器层（真库留痕册），不落纵向。`cmd/parcel-dispatch` 闭环夹具的商业依据替身 `synRCommercialBasis.FormJudgmentAsOf` 用固定锚点形成时点，纵向用例在那上面会平凡通过，钉不住前提。「提交接收」不读时钟那一半由 PS `partycommercial` 的 `submission_receipt_as_of_test.go` 守，本票不碰 PS 地盘。判据本身不改。
6. 范围一句：PS `pre_acceptance_control.go` 只动了 `notFormedOutcomeFor` 那一个 case 的取值列表；`cmd/parcel-dispatch`、`internal/architecture` 下的基线与票 27 的地盘一行没动。

### 验证（作者那层，go1.26.8，证据层级 `S`）

钉 `d819231f`（worktree `idp-parcel-mcp3-psb28`，`git status --porcelain --untracked-files=all` 为空），19:43–19:44：

- 自 `d4a3779c` 起改过的 12 个 `.go`，按入库字节（`git cat-file blob HEAD:<path>`）过 toolchain `gofmt -l` 为空、无 CR、无 BOM。
- `go build ./...`、`go vet ./...` 全仓退 0。
- 改过的 7 个包经 `go list -test` 反查反向依赖，连同自身 19 个包（三个 `cmd`、SA 各适配器、PP 与 PS、CC 对 SA 的适配器等）；带 `IDP_PARCEL_POSTGRES_DSN` 跑 `go test -count=1 -p 1 <这 19 个> ./internal/architecture/...`：19 ok / 0 FAIL / 1 无测试文件（SA `ports`）/ 0 cached，退 0。
- `-v` 单跑：`TestARedriveUnderTheSubmissionReceiptAsOfIsAReplayWhileAMovingAsOfConflicts`、`TestAPreAcceptanceEstimateTraceIsWrittenOnceAndReproducible` 为 `PASS`，无 `SKIP`。
- 机制清点在 `d819231f` 上重跑生成器（`tools/mechanism-inventory`），`git status` 为空：三笔没有增删文件。
- 变异：见完成判据各条。
