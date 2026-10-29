# 03 计价评价遇到输入单位与价卡声明的单位不同时直接答冲突，没有换算

Category: enhancement
Status: resolved——2026-10-29 进 main（`4b166dd07`；全量 164 ok / 0 FAIL，种子退 0），见文末「评审记录」「进 main 记录」。此前 in-progress——作者完工并补完评审修订轮，待进 main，分支 claude-refdata2b（代码最后一笔 `2569dbec7`）。此前：作者完工，待隔离评审与进 main（代码最后一笔 `0d337609c`）。
Blocked by: 无（接票 01，ADR-0335 已进 main）

## 现象

取证于 main `b8fe76a42`：价卡按 KG 查表而申报重量是 LB、体积系数按 CM 声明而尺寸是 IN，评价一律落「冲突」，问题项 `WEIGHT_UNIT_MISMATCH` / `LENGTH_UNIT_MISMATCH`；磅与千克的关系是定义值，目录（`refdata.Convert`）也已经能精确算出，ADR-0335 决定四把「接不接、接在评价的哪一步」留成单独的裁决。

## 做什么

设计与取舍在 [ADR-0346](../../../docs/adr/0346-evaluation-converts-input-weight-and-dimensions-to-the-units-the-plan-declares-by-the-built-in-unit-catalogue-and-records-the-factor-it-used.md)（会话代产品 owner、PP owner 接受，待复核）。

1. 评价里输入实重（含票级合计实重与合计体积重）按目录精确系数换算到价表查表单位，尺寸按系数换算到体积系数声明的长度单位，再走原有取整规则；单位一致的评价先走原函数，不动。
2. 换算记录（原值与原单位、换算值与目标单位、系数、是否精确）随评价入册并写进解释；重放与同标识重复请求只认记下的系数，不重新查目录。
3. 无限小数：重量按取整边界推出的位数向零截断并如实记，含 `NONE` 段的取整策略不收；尺寸由精确有理数一步取整。
4. `refdata.Convert` 传 nil 报错而不是 panic（上一轮评审的非阻断余项）。

## 完成判据

- 领域单测：LB→KG 查 KG 价表、G→KG、IN→CM 的体积重、1 KG→LB 的精度、跨类别与未知单位仍拒、单位一致的路径不变；评价解释含原值、换算值、系数；回放用记下的系数重算得同结果；换算记录随快照往返并在读回时逐条重验。
- 既有测试与黄金用例一字不改仍过；`internal/architecture` 门禁过。
- 适配器层：申报的磅与英寸原样进计价输入（不换算），计价封闭单位表之外的单位仍当没有测量；真库上换算评价读回、重放同摘要。
- UC-PP-001 的 AT 表补行，AT 盘点 ① 桶补行，给开发的交接改成现行；CONTEXT 与被停条款同笔改成现行规则。

## 完成记录（2026-10-29 作者）

起点 `b8fe76a42`（origin/main）；分支 `claude-refdata2b`。提交：`0d337609c`（代码与用例）；其后文档一笔（ADR-0346、ADR-0335 前向指针、CONTEXT、UC-PP-001、AT 盘点、设计吸收覆盖表一行、本票面）。

**改了什么**
- `internal/parcelpricing/domain/unit_conversion.go`（新）：换算器、换算记录 `UnitConversionStep`、读回自洽门、取整边界小数位 `conversionBoundaryScale` 与截断位数 `guardPlaces`、有理数取整 `roundRatToIncrement`。
- `input.go`：`CalculatePricingWeight` 变成对 `calculatePricingWeight(…, converter=nil)` 的薄封装，行为不变；本体在单位对不上时由换算器换算实重、合计体积重、尺寸，取整与抬高的代码一字未动。
- `evaluation.go`：`EvaluatePricing` 先调原函数，只有答了单位不一致才带换算器再算；请求多一对冻结字段，重放请求置冻结；评价多一份换算记录、读回门多一道整体验。`evaluation_snapshot.go`：快照多一个可缺的 `unitConversions` 键。
- `application/evaluate_pricing.go`：`settleAgainstExisting` 借原评价记下的换算记录（一行）。
- `internal/platform/refdata`：`Convert` 收到 nil 报 `ErrNilValue`，补用例。
- 适配器代码一行未改：`adapters/transportfulfillment` 与 `adapters/parcelshipment` 的单位译码（封闭表 `G` `KG` `OZ` `LB` / `CM` `IN`）本来就不换算，只补了用例钉住。
- 无迁移、无库表（评价快照是 JSON 文档）。

**不变的证据**
- 既有测试零改动：`git diff origin/main --numstat -- '*_test.go'` 在取证于 `acbea73c6` 时是 6 个新增文件加 `internal/platform/refdata/refdata_test.go` 中部追加的一个新用例，删除列全为 0——没有一行既有测试被删除或改写，既有黄金用例同样未动。
- 摘要与快照字节对照：同一组夹具在 `git archive origin/main` 解出的树上与本分支上各跑一遍，七行里五行（单位一致 ActualOnly、单位一致 MAX 带尺寸、缺尺寸待判断、查无费率待判断、对这几份的重放）语义摘要与快照散列逐位相同；受影响的两行（磅对千克、英寸对厘米）原是 `CONFLICT`，现为 `COMPLETED`。把这两份请求置冻结且无记录（等于换算接入之前答过冲突的评价被重放），摘要与快照散列与 main 上的 `CONFLICT` 两行逐位相同。对照脚本在作者暂存区，不入库。
- 受影响的评价路径只有两条：输入实重（含票级合计实重）单位 ≠ 价表查表单位；尺寸单位 ≠ 体积系数长度单位。

**代决与越权风险点**：ADR-0346（会话代产品 owner、PP owner 接受、待复核）越权风险点 0–7。另有本票层面的代决：
1. **ADR 号取 0346**：协调方点名；origin/main 与各 claude-* 分支最大号为 0344，0345 未见占用但按派单不取。
2. **换算记录只进快照、不进规范化文档**（ADR-0346 决定五）：不换号。依据是换算的全部影响已落在摘要覆盖的计价重量里，且没有加键；换算记录的边界写进风险点 5。
3. **单位一致的路径靠「先走原函数」保证不变**，不靠「换算为恒等」——恒等换算仍会走新代码，原函数则是同一份字节。这也让 `CalculatePricingWeight` 保持有生产调用方（接线棘轮，没动 `production_wiring_baseline.txt`）。
4. **特征判定里阈值的单位比较不换算**（决定六）：CONTEXT「判定中不得隐式换算」留作现行并加一句限定；真实参数 `R40` 那一格（阈值公制、价表英制）仍待换算规则的声明形式。
5. **适配器译码口径不放宽**：目录里有而计价封闭单位表里没有的单位（吨、毫克、毫米、米、英尺、码）仍当表外；放宽要先放宽 `WeightUnit` / `LengthUnit` 两个封闭集。

**判据对用例**（AT-PP-017 至 021，盘点 ① 桶已补行）
- 磅、克折千克 → `TestAPoundWeightIsConvertedExactlyToThePlansKilogramTableUnit`、`TestAGramWeightIsConvertedToTheKilogramTableUnit`（解释含 `unit conversion ACTUAL_WEIGHT: 2.5 LB -> 1.133980925 KG at factor 0.45359237 (exact)`，评价输入保留 2.5 LB）；票级主体 → `TestAnAggregateEvaluationConvertsTheMemberTotals`。
- 英寸折厘米的体积重 → `TestInchSidesAreConvertedToTheVolumetricFactorsCentimetres`（与直接给 25.4 cm 逐字相同）。
- 1 KG→LB 的精度 → `TestAKilogramWeightToAPoundTableKeepsEnoughPlacesAndSaysItWasTruncated`（系数 `100000000/45359237`，截断 11 位、解释写明）；截断不改变取整结果 → `TestTruncatedConversionNeverChangesTheRoundedWeight`（独立有理数参考实现、临界点两侧偏差、随机重量，CEILING 与 HALF_UP 三组；把保留位数砍小用例即失败，作者做过变异）；`NONE` 不收截断 → `TestANonTerminatingConversionIsRefusedWhenTheRoundingCannotAbsorbTruncation`；厘米折英寸 → `TestCentimetreSidesToAnInchFactorAreRoundedFromTheExactProduct`。
- 回放与重复请求 → `TestReplayOfAConvertedEvaluationReproducesTheSameResult`（含换算后待判断）、内部用例 `TestFrozenConversionUsesTheRecordedFactorNotTheCatalogue`（记下 0.5 就用 0.5，不用目录的 0.45359237）、`TestFrozenWithoutARecordKeepsTheOriginalUnitMismatchAnswer`、应用层 `TestAnEvaluationRecordedAsAUnitMismatchBeforeConversionIsNotRewrittenByTheSameRequest`（去掉借记录那一行即失败，作者做过变异）。
- 读回与真库 → `TestUnitConversionRecordsSurviveTheSnapshotAndAreRevalidatedOnReadBack`（六种篡改都被拒）、`TestAConvertedEvaluationIsReadBackWithItsConversionRecordAndReplaysEqual`（真库）。
- 不变的与仍拒的 → `TestAnEvaluationInTheTableUnitIsUntouchedByConversion`、`TestCalculatePricingWeightItselfStillRefusesAnotherWeightUnit`、`TestUnknownAndCrossCategoryUnitsStayUnitMismatch`、既有 `TestVolumetricFactorRefusesSidesMeasuredInAnotherUnit` 与 `TestConditionRefusesAThresholdMeasuredInAnotherUnit`（未改动）。
- 适配器 → `TestADeclaredPoundWeightReachesTheEvaluationUnconvertedAndIsPricedInKilograms`、`TestAPoundAndInchDeclarationIsHandedOnInItsOwnUnitsWithoutConversion`、`TestAUnitInTheCatalogueButOutsideThePricingTableStaysUnmeasured`。

**自验**（含 PG，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）
- `go build ./...`、`go vet`（`./internal/parcelpricing/...`、`./internal/platform/...`、`./internal/settlementaccounting/...`、`./internal/architecture/...`）无输出；`gofmt -l internal cmd tools scripts` 无输出。
- `go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/settlementaccounting/... ./internal/architecture/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/...`：带 DSN，41 个有测试的包全部 ok、退 0（含 `internal/architecture` 门禁与 `cmd/parcel-api` 的计价试算真库用例）。没跑全量。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 只报生成物过期（ADR 索引新增两行改动、AT 盘点 ① 桶计数 +5，PP 16→21），按「进 main 的步骤」第 4 步由推送方重生成。机制清点未跑。
- 演示种子：一次性库（建在 `idp-parcel-postgres-gate` 上，用后已删）`scripts/demo-seeds/seed.sh` 退 0，「计价登记」一步（价卡、参考序列、复核职责规则、序列复核、参考目录）全部登记。没有对种子库另起服务做评价请求——价卡内容摘要没变（评价路径的变化只在换算，方案与价卡字节不动），评价侧的证据是上面的全跑与摘要对照。

**谁消费「单位不一致」这个答复**
- 问题项编码 `WEIGHT_UNIT_MISMATCH` / `LENGTH_UNIT_MISMATCH`：全仓（Go、管理台、文档）没有任何代码按这两个码分支；只在 `evaluation.go` 发出，经 HTTP 读口与 `evaluation_issue` 子表原样透传。
- 评价状态「冲突」：结算采用适配器（买入与卖出两只）译成 `StopEvaluationConflict`，渠道候选成本（`parcelshipment/adapters/parcelpricing`）译成 `ChannelCostConflict`，计价自己的编排原样透出。此前被单位不一致挡在冲突的请求与候选，用新评价标识重算会得出金额；已在册的冲突评价不改写，同标识重放仍得旧答复（有意，ADR-0346 风险点 4）。

**留给协调方**
- ADR-0346 的 README 索引行、AT 盘点计数、机制清点：进 main 时重生成（本分支没动生成物）。
- 评价上多出的换算记录目前只在快照与解释里可读，HTTP 响应没有结构化字段；管理台若要单列「换算」，另立票（不挡）。
- 不挡上线的新发现：特征判定里阈值与特征的单位比较仍不换算（`R40` 公制阈值对英制价表），是 ADR-0346 风险点 1 点名的下一轮，请协调方记入冻结清单；目录里的吨、毫克、毫米、米、英尺、码要进评价，需先放宽 `WeightUnit` / `LengthUnit` 封闭集，同记。
- 隔离评审请重点看：`guardPlaces` 论证的 `g` 是否枚举全了（风险点 2），以及「先走原函数、答了单位不一致再换算」这一结构对既有评价的保证。

## 修订轮（2026-10-29 作者，应协调方转来的隔离评审意见，评审钉 `edf13c831`、阻断为 0）

提交：`2569dbec7`（代码与用例）；其后文档一笔。

1. **冻结模式也冻截断位数**（评审非阻断第 1 条，进 main 前定）：`convertWeight` 在冻结模式下取记录里同一个量、同一对单位的 `places`，并校验它不小于当前公式推出的下限（`guardPlaces`）；小于、没记、记成精确、只记在另一个量名下，一律按答不出处理（原来的单位不一致，重放因此判成结果不一致），不静默改用较小位数。尺寸的展示位数在冻结模式下同样取记下的，重放出的记录与原记录逐字相同。用例：`TestFrozenReplayKeepsTheRecordedTruncationPlaces`（记 20 位的评价重放与读回后重放都得同摘要；记 11 位同摘要于当前公式；记 8 位被拒；位数不同摘要里的实重串就不同）、`TestFrozenInexactConversionNeedsARecordWithAtLeastTheFloorPlaces`（内部）；去掉「取记下位数」或去掉下限校验，两个用例各自失败（作者做过两次变异）。造「位数与当前公式不同的记录」只能经测试专用的 `unit_conversion_export_test.go`，不进生产构建。ADR-0346 决定三补一句，越权风险点 2 补这条的后果与处理（位数公式只能放宽下限、不能收紧，收紧要另立决定并处理存量）；UC-PP-001 `AT-PP-020` 与盘点 ① 桶同步。
2. **ADR-0335 Status 索引注记补挂**「越权风险点 6 由 ADR-0346 决定四回答，无条款停用」（评审第 3 条）。
3. nit：parcel-pricing CONTEXT 里「规则见下一条，不在此列」改成「见「单位换算」一条」（评审第 4 条）；`unit_conversion.go` 头注「单位一致的路径根本不走本文件」改成如实——`EvaluatePricing` 先走原函数、不构造换算器，但读回门 `validUnitConversions` 与访问口 `UnitConversions` 对每个评价都会被调到，记录为空时是空操作。

**自验**（带 DSN）：`go build ./...`、`go vet`、`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/architecture/...` 26 个有测试的包全部 ok、退 0。既有测试仍零改动（`*_test.go` 对 origin/main 的 diff 删除列全为 0）。

## 评审记录（2026-10-29，隔离子代理，钉 `edf13c831`）

阻断清零。已算对的评价一字不变：测试文件对 main 纯新增（995 增、0 删）、指纹与规范化文档未加键、新增键可缺；跨树 1088 行对拍，单位一致的行摘要、快照散列、解释行数逐位相同，置冻结重放换算接入之前答过冲突的评价也逐位相同。精度：独立复算约 590 万组重量组合（12 个有序单位对 × 全部取整策略 × 边界两侧 × 抬高 × 分段）、聚合 MAX 10.8 万例、MAX+尺寸 55.8 万例、尺寸 CM↔IN 8 万例，与精确有理数直接取整 0 差异；含「无舍入段一律拒」的理由成立、不会误拒原本能评价的。体积重换的是三边、先换后乘，避开截断后相乘。回放：解释与快照记原值、换算值、系数、是否精确；评审另做端到端反例（目录系数临时改 0.5 后同标识 Handle、Replay、快照往返重放均得原结果），新标识读到新目录。受影响路径只有两条（实重单位 ≠ 价表单位、尺寸单位 ≠ 体积系数长度单位），无代码按「单位不一致」码分支，结算与渠道成本只按 CONFLICT 状态分支，已有冲突评价不翻成完成、不会重复形成费用。非阻断并已处理（`2b183b941`）：冻结模式连截断位数也只认记下的（位数小于当前下限则按单位不一致拒）、ADR-0335 Status 补挂风险点 6 由 ADR-0346 回答、两处文字。非阻断未处理：重放只认记下系数这条的端到端用例因目录是不可变包变量而无法在仓内测试钩子——评审用一次性测试证实；换算后的合计体积重与尺寸换算值没有与计价重量对账（ADR 决定三只承诺实重）。

## 进 main 记录（2026-10-29，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-refdata2b` tip `2b183b941` 作出处；隔离树 cherry-pick 到 main `b8fe76a42`，无冲突；重生成 ADR 索引、AT 盘点计数与机制清点，两项 `-check` 退 0；全量 164 ok / 0 FAIL；种子一次性库退 0；推确切 SHA `4b166dd07`。
- 修订轮冻结位数的改动评审未重跑（28 行生产代码，两次变异测试守着）。

