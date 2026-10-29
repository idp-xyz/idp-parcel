# 01 判定条件读不到体积重与计价重量，计价输入没有地址性质——超重费只能按实重收、住宅派送费表达不了

Category: enhancement
Status: resolved——2026-10-29 进 main（`6d7ada458`；全量 164 ok / 0 FAIL，种子退 0），见文末「评审记录」「进 main 记录」。此前 in-progress——2026-10-29 作者完工并补完评审修订轮（B1 阻断已修），待复评与进 main，分支 claude-pricingA（代码最后一笔 `d4eaf592b`）。此前：作者完工，待隔离评审与进 main（代码最后一笔 `ece038c03`，已随 rebase 改号为 `aa99d559d`）
Blocked by: 无（接 reference-data/03 即 ADR-0346，已进 main）

## 现象

取证于 main `7ea1aa677`（完整合成价卡 `SYN-PLAN-CN-SG-FULL-01` 取证出的计价引擎缺口①③，记在 `.scratch/launch-line-freeze-2026-10-27.md`）：

- 特征来源集合声明了 `VOLUMETRIC_WEIGHT`、`CHARGEABLE_WEIGHT`，评价器却从不把它们填进判定特征（`WithDerivedWeights` 全仓无生产调用），读它们的条件在评价里落 `CALCULATION_FAILED`；超重费只能按实重收，而真实承运商的超重费常按计费口径的重量。
- 计价输入快照与试算声明（`EstimatePayload`）都没有收件地址性质，住宅派送费表达不了；作者说「地址类型特征槽被偏远档位占用」。

## 做什么

设计与取舍在 [ADR-0347](../../../docs/adr/0347-conditions-read-the-volumetric-weight-and-the-packages-own-chargeable-weight-before-any-rule-is-judged-and-the-address-nature-is-a-declared-input-with-its-own-feature-slot.md)（会话代产品 owner、PP owner 接受，待复核）。

1. 体积重与自身计价重量（不含条件最低计价重量的抬高）进判定特征：评价在判定第一条规则之前按重量策略一次算好；缺输入待判断不是 0；读体积重的卡须有体积系数、阈值用价表单位、抬高条件不读自身计价重量，装配与读回时拒。
2. 收件地址性质作评价输入与独立的特征槽 `ADDRESS_NATURE`（封闭词商业 / 住宅），缺席答待判断；试算载荷可选 `addressNature`，试算页加下拉；来源（PS）归 PS owner 另票。
3. 不改变任何今天已算对的评价；不换规范化版本。
4. 演示租户第二张完整价卡 `SYN-PLAN-CN-SG-FULL-02`（范围 `SYN-SCOPE-03`）。

## 完成判据

- 超重费按自身计价重量触发：恰等于阈值不触发、阈值之上的最小可表示值触发；按体积重触发；抬高条件读自身计价重量被拒；缺尺寸答待判断不是 0。
- 住宅派送费按声明的地址性质收，与偏远档位同卡同票并收；没声明答待判断（评价）/ 输入不全（试算），不套默认。
- 既有评价、黄金用例、价卡内容摘要一字不变；`*_test.go` 对 origin/main 只新增。
- UC-PP-001 取空 AT 号补行，AT 盘点 ① 桶补行，CONTEXT 与 UC 的规则句改成现行。

## 完成记录（2026-10-29 作者）

起点 `7ea1aa677`（origin/main）；分支 `claude-pricingA`。提交（rebase 到 origin/main `05a05a34e` 之后的号；原号 `14525a485`、`85aec03bd`、`454e84ccb`、`ece038c03` 对应改为）：`7ec26586f`（领域、编排、载荷、导入模板与用例）、`51431b4bc`（管理台下拉与第二张完整价卡种子）、`ca35edbad`（读回门补同一道检查）、`aa99d559d`（重生成入库的价卡导入模板：条件表 category 列的说明多一句，模板版本 `PPT-1` 与表、列都不动）；其后文档一笔（ADR-0347、ADR-0152 前向指针、CONTEXT、UC-PP-001、AT 盘点、导入模板规格一行、本票面）。

**核实：「地址类型特征槽被偏远档位占用」属实。** `resolveCatalogues` 解出的偏远档位经 `WithCategories(zone, tier)` 填进 `PackageFeatures.addressType`（ADR-0109 Context 的对应关系）；绑了档位目录的卡该槽就是档位码，没绑的卡该槽缺席。FULL-01 的偏远一档、二档两条规则读的正是它（`ADDRESS_TYPE = T1 / T2`）。后果：把住宅派送费写成 `ADDRESS_TYPE = RESIDENTIAL`，在绑了档位目录的卡上**静默永不命中**，在没绑的卡上落 `CALCULATION_FAILED`——住宅派送与偏远档位无法同卡并收，与 CONTEXT「地址分类」词条写明的「同一票可以既命中住宅派送又落在某一偏远档位」直接相抵。**改法**：不改指（已发布卡与摘要读的是今天的含义），另立 `ADDRESS_NATURE` 槽（CONTEXT「特征」词条的一次拓宽，ADR-0347 决定四；这正是 shape-gaps/01 完成记录里留给 owner 的那一问：「若 owner 判定地址性质也要成为可判定量，那是「特征」词条的一次拓宽，另立票」）。

**改了什么**
- `domain/package_weight_features.go`（新）：触发条件读哪些特征、`checkPackageWeightReads`（装配与读回同一道）、`packageOwnWeights`（评价在特征阶段多算一遍包裹自身的体积重与自身计价重量，与正式计价重量同一件实现）、`conditionScale`（读体积重的阈值并入换算截断位数）。
- `domain/address_nature.go`（新）：封闭词表 `AddressNature`、`ErrAddressNatureRequired`（是 `ErrFeatureUnavailable` 的一种）。
- `feature_condition.go`：新特征来源 `ADDRESS_NATURE`、`PackageFeatures.WithAddressNature`、期望取值按词表校验。`input.go`：输入快照可缺的 `addressNature`、`WithAddressNature` / `AddressNature`。`evaluation.go`：特征阶段按需填两个派生重量与地址性质，`ADDRESS_NATURE_REQUIRED` 译成待判断。`evaluation_snapshot.go`：一个可缺键。`plan.go`、`plan_structures.go`：装配门与读回门；`NewConditionalMinimumWeight` 拒读自身计价重量。`unit_conversion.go`：`conditionScale` 一格。
- 应用层 `form_estimate_evaluations.go`：缺项码 `ADDRESS_NATURE`、命令多一格、标识只在声明了时多派生一段。传输层 `estimate_evaluations.go`：载荷键 `addressNature`。导入模板认 `ADDRESS_NATURE`。管理台试算页「收件地址性质（可不选）」下拉与缺项中文。
- `scripts/demo-seeds`：seedgen 把完整价卡构造收成按 `fullCardSpec` 取值的一份（FULL-01 产物逐字节不变，重跑 seedgen 其余各份未动）；FULL-02 数据文件 `price-card-cn-sg-full-02.json`、步骤 `161-pricing-full-card-address-nature.sh`（161 在 origin/main 与各 claude-* 分支都空）、README 一行。
- 重新生成入库的价卡导入模板 `price-card-import-template-PPT-1.xlsx`（`go run ./cmd/parcel-pricing-template`）：只有条件表 `category` 列的说明文字多了一句，表与列不变，`PPT-1` 不换。
- 无迁移、无库表（评价快照是 JSON 文档）；规范化文档**一个键都没加**，`PPC-5` 不动。

**代决与越权风险点**：ADR-0347（会话代产品 owner、PP owner 接受、待复核）越权风险点 0–8。另有本票层面的代决：
1. **ADR 号取 0347**：派单点名；origin/main 与各 claude-* 分支最大号 0346，0347 空。AT 号取 `AT-PP-022` 至 `026`（origin/main 与各分支最大 021）。
2. **自身计价重量取「进位之后、抬高之前」**（风险点 0）：派单示例口径；评价里「派生、抬高、进位」是 CONTEXT 固定的顺序，所以这个量是对同一派生链少走抬高一步另算的，不是链上一个现成的中间值。**「计费重量」一词归 SA，本票不用。**
3. **抬高条件拒读自身计价重量，但这不是为了防环**（风险点 1）：这个口径下没有环；拒是保守，放开不破坏已发布卡。ADR 里如实写了，没有把它说成防环。
4. **载荷键叫 `addressNature`，不叫派单说的 `addressType`**（风险点 8）：「地址类型」那一槽装的是偏远档位，同名的载荷键不进那一槽会是个陷阱；线上键名跟着特征走。
5. **地址性质不进规范化文档**（风险点 5）：同结算币种、比较币种；因此没有「只让新计划用新版本」要做——没有加键，ADR-0267 不触发。
6. **读回门同样拒无效条件**（`454e84ccb`）：装配门有的检查，读回整图重验也要有，否则存下来的文档绕得过去。
7. **ADR-0152 部分停用的前向指针**：决定八「不做的」里「地址性质与服务选项……」一句的地址性质那一半，Status 行索引注记与 Links 各加一条，正文不动。

**判据对用例**（`AT-PP-022` 至 `026`，盘点 ① 桶已补行）
- 超重费按自身计价重量、边界 → `TestOverweightSurchargeTriggersOnTheChargeableWeightAtTheBoundary`（体积重 20.0 → 进位 20.0 不触发；20.1 → 20.5 触发，总额 17 = 10 + 7；实重 10 而体积重 24 触发）；含端读进位后的值 → `TestChargeableWeightIsReadAfterRoundingNotBefore`（19.8 → 20.0，`>= 20` 成立而体积重本身 19.8 不成立）；抬高不连带 → `TestChargeableWeightFeatureDoesNotSeeTheConditionalMinimumRaise`（尺寸条款把计价重量抬到 30、基础价按 30 查，超重条款仍不成立）。
- 按体积重触发 → `TestVolumetricWeightFeatureIsIndependentOfTheActualWeight`；抬高条件读体积重合法 → `TestAConditionalMinimumMayReadTheVolumetricWeight`；票级主体 → `TestAggregateSubjectsReadTheTotalsAsTheirOwnWeights`；磅进千克出且换算记录与解释各只一条 → `TestChargeableWeightReadsTheConvertedWeightAndDoesNotRecordTheConversionTwice`；阈值比截断位数还细 → `TestAThresholdFinerThanTheTruncationWidensTheBoundary`（作者做过变异：`volumetricThresholdScale` 恒返回 0，用例即失败，解释里体积重截断成 2.20462262184 而阈值是 2.204622621848）。
- 缺输入不是 0、装配与读回拒 → `TestMissingDimensionsLeaveTheDerivedWeightsUnavailableNotZero`、`TestAConditionalMinimumCannotBeTriggeredByTheChargeableWeight`（含嵌在组合条件里）、`TestPlanAssemblyRefusesConditionsThatCanNeverBeEvaluated`（读实重的条件阈值单位照旧不受管）。
- 地址性质 → `TestAddressNatureVocabularyIsClosed`、`TestResidentialSurchargeFollowsTheDeclaredAddressNature`（住宅 16 = 10 + 6、商业 10、没声明 `PENDING` / `ADDRESS_NATURE_REQUIRED` 且没有合计）、`TestResidentialDeliveryAndRemoteTierCoexistOnOneCard`（同一票偏远 T1 与住宅同收 19.5；`ADDRESS_TYPE = RESIDENTIAL` 的条件不被地址性质满足）、快照往返与重放、真库 `TestAnEvaluationWithADeclaredAddressNatureIsReadBackWithItAndReplaysEqual`；试算 → `TestEstimateNamesTheMissingAddressNatureForTheCardThatReadsIt` 等三条；载荷 → `TestEstimatePayloadTranslatesTheOptionalAddressNature`（含 `addressType` 键按未知键拒）；导入模板两条；管理台 `estimate-address-nature.test.ts` 三条。
- 不改变 → 见下「既有评价零改动的证据」；`TestAPlanThatDoesNotReadTheDerivedWeightsPaysNothingForThem`、`TestAddressNatureIsOutsideTheCanonicalInputAndStillMovesTheDigestThroughTheResult`。
- 第二张完整价卡七票手算 → `cmd/parcel-pricing-register/demo_full_card_nature_test.go`。

**既有评价零改动的证据**
- `*_test.go` 对 origin/main 只新增：`git diff origin/main --numstat -- '*_test.go'` 的删除列全为 0（见自验）。
- **跨树对拍（领域层）**：同一批夹具在 `git archive origin/main` 解出的树与本分支上各跑一遍，逐行比价卡内容摘要、规范化版本、价卡登记快照散列、评价语义摘要、评价快照散列、解释散列与费用行数——共 32 行（逐包裹只按实重 4 个实重、MAX 带尺寸 3 个尺寸与缺尺寸、附加费命中 / 未命中 / 缺尺寸、拒收命中 / 未命中、条件最低计价重量命中 / 未命中、偏远档位目录命中 / 未解析、分区目录、磅与英寸换算、固定规则、重放、票级聚合与它们的价卡），**逐位相同**。对拍脚本在作者暂存区，不入库。
- **跨树对拍（真库、种子）**：两棵树各灌一个一次性库，同一个试算程序（只用两棵树都有的接口）对第一张完整价卡（`SYN-SCOPE-02`，151 行输出）与 v1 售价卡（`SYN-SCOPE-01`，80 行）各七件包裹试算，评价标识、语义摘要、全部解释行与费用行**逐字相同**。
- 结构保证：新代码只在两种情形下走——方案有条件读体积重或自身计价重量（`readsPackageWeights`）、输入声明了地址性质；其余评价走的就是引入之前的那几行。

**六票结果（FULL-02，`SYN-SCOPE-03`，一次性库灌种子后用真库价卡册、目录与序列试算，基准时点 2026-09-01，起运 200000，实重 KG、尺寸 CM；手算见 `demo_full_card_nature_test.go` 注释）**

| 票 | 包裹 | 地址性质 | 结果 | 费用行（CNY） | 合计 |
|---|---|---|---|---|---|
| T1 | 2 kg 30×20×10，目的 018000（Z1，T0 市区） | 住宅 | 已评价 | 基础 109、住宅派送 6、燃油 11.99 | 126.99 |
| T2 | 同上 | 商业 | 已评价 | 基础 109、燃油 11.99 | 120.99 |
| T3 | 同上 | **不填** | **输入不全**，缺 `ADDRESS_NATURE` | 不评价 | — |
| T4 | 同 T1，目的 098000（Z1，偏远 T1） | 住宅 | 已评价 | 基础 109、偏远一档 12、住宅派送 6、燃油 11.99 | 138.99 |
| T5 | 5 kg 60×50×40（体积重 24），目的 238000（Z2） | 商业 | 已评价 | 基础 753、超重 25（计价重量 24 > 20）、燃油 82.83 | 860.83 |
| T6 | 8 kg 50.25×40×50（体积重 20.1 → 计价重量 20.5），目的 627000（Z1，偏远 T2） | 住宅 | 已评价 | 基础 775、超重 25、偏远二档 28、住宅派送 6、燃油 85.25 | 919.25 |
| T7（边界，六票之外） | 8 kg 50×40×50（体积重恰 20.0），目的 018000 | 商业 | 已评价 | 基础 757、燃油 83.27（**不收**超重） | 840.27 |

手算与真库试算逐票一致（基础：Z1 首重 0.5 kg 55、续重每 0.5 kg 18，Z2 为 48 与 15；燃油 = 基础 × 11%，逐行取整到分）。住宅与偏远档位（T4、T6）同票并收，没有被一个槽挤掉。真浏览器（Chromium，管理台试算页挂在 5180、代理到装了真库价卡册的试算入口）：下拉三选项「不选 / 商业地址 / 住宅地址」，默认不选；不选时载荷没有 `addressNature` 键、结果区「输入不全：缺 收件地址性质（……）」；选住宅得 126.99、选商业得 120.99，载荷依次为缺席 / `RESIDENTIAL` / `COMMERCIAL`。

**自验**（含 PG，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）
- `go build ./...`、`go vet`（`./internal/parcelpricing/...`、`./internal/platform/...`、`./internal/settlementaccounting/...`、`./internal/architecture/...`、`./cmd/...`、`./scripts/...`）无输出；`gofmt -l internal cmd tools scripts` 无输出。
- `go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/settlementaccounting/... ./internal/architecture/... ./cmd/...`（带 DSN，含 `internal/architecture` 门禁、`cmd/parcel-api` 的计价试算真库用例）：55 个有测试的包，首轮 54 个 ok、1 个 FAIL——`cmd/parcel-pricing-template` 的 `TestCommittedTemplateMatchesTheGenerator`（入库的导入模板 xlsx 与生成器输出须逐字节相等，我改了 `category` 列的说明文字却没重新生成），已重生成并提交（`ece038c03`），该包与导入模板包重跑 ok；其余各包在首轮已 ok，之后只改了这份 xlsx 与文档。没跑全量。
- 管理台：`tsc -b` 退 0、`pnpm test` 902 项全过、`vite build` 过。
- 演示种子：一次性库（建在 `idp-parcel-postgres-gate` 上，用后已删）`scripts/demo-seeds/seed.sh` 退 0，「第二张完整售价卡」一步登记 `price-card: RECORDED`。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 只报生成物过期（ADR 索引新增 ADR-0347 一行、ADR-0152 一行注记；AT 盘点 ① 桶 PP 计数 +5），按「进 main 的步骤」第 4 步由推送方重生成；`adr-index` 的解析结果核过（〔代决·待 PP owner 复核〕）。机制清点未跑。

**留给协调方**
- ADR-0347 的 README 索引行、ADR-0152 的索引注记行、AT 盘点计数、机制清点：进 main 时重生成（本分支没动生成物）。
- **PS owner 另票（派单点名要写成交接的一项）**：`parcel-shipment` 没有结构化的地址性质——地址要素 `AddressElementName` 的封闭名集只有邮编与国家 / 地区码。要提供它：先回 PS CONTEXT「地址要素」词条加第三格（或另立客户申报字段），再由 `adapters/parcelshipment`（受理前估价的声明）与正式评价的输入解析器译进 `PricingInputSnapshot.WithAddressNature`。在那之前，读地址性质的卡走不通正式评价、受理前估价与路由比价（待判断 / 输入不全）——给租户写住宅派送费的卡只在试算里算得出；FULL-02 因此放在自己的范围里、不进演示动线。已写进 UC-PP-001「给开发的交接」与 ADR-0347 风险点 3。
- 冻结清单 2026-10-29 那条完整合成价卡的缺口记录：①（体积重与计价重量不进判定特征）与③（地址性质 / 地址类型槽被偏远档位占用）由本票处理，请协调方在记录上标「已处理：pricing-engine-gaps/01」；②（无条件附加费与按偏远档位查附加费价表）④（最低收费）⑤ 未动。
- **术语债**：`FeatureAddressType`（地址类型）实际装偏远档位，名字与含义错位，本票不改（改要换规范化号并迁移已发布卡）；ADR-0347 风险点 2。
- 不挡上线的新发现：晚期待判断的评价（序列读数、偏远档位、地址性质）把已成形的基础运费行留在评价上、只是没有合计（既有行为，`ADDRESS_NATURE_REQUIRED` 沿用）；读到含新特征来源价卡的旧构建在读回门上拒收（风险点 7，进生产后滚动发布才相关）。
- 隔离评审请重点看：口径取舍（自身计价重量不含抬高，风险点 0）；`readsPackageWeights` 这道守卫是不是真把「不读新特征的评价」挡在新代码之外（对拍脚本可向作者要）；`conditionScale` 的论证（决定五，变异已做）；读回门新增的一道对既有存量卡的影响（风险点 4）。

## 修订轮（2026-10-29 作者，应协调方转来的隔离评审意见，评审钉 `ce5526f92`、阻断 1 条）

**先 rebase。** `git rebase origin/main`（`05a05a34e`，B 的 `pricing-engine-gaps/02` 已先进）：六个文件文本冲突，全部取并集——`plan.go` 两处（装配门里 `checkPackageWeightReads` 与 B 的 `coherentWithPlan` 并存，读回门同；两道都在）、`seedgen/main.go`（FULL-02 与 B 的 FULL-03 两条 `write`）、`seedgen/full_card.go`（头注合并成「住宅派送费与最低收费这一张都没做：地址性质见 FULL-02，总额下限见 full_card_03.go」）、`scripts/demo-seeds/README.md`（FULL-01 一行的「超重读实重 / 住宅派送费没有做」句改成现状，FULL-02 与 FULL-03 各一行）、`.scratch/at-coverage-inventory.md` 与 `UC-PP-001` 的 AT 表（`AT-PP-022` 至 `026` 与 B 的 `030` 至 `040` 并存，节序按号）。号没有撞：ADR 0347 与 0348、AT 022–026 与 030–040。rebase 之后的对拍：领域层同一批 32 行夹具与新的 origin/main 比，逐位相同。

**B1（阻断）修了。** 提前收尾的评价丢了自身重量那一遍做过的换算。触发 = 方案有条件读体积重或自身计价重量 ∧ 输入单位要换算 ∧ 评价在正式计价重量之前收尾（拒收命中、序列取值缺失待判断、抬高条件求值出错）；原评价没有换算记录，重放与同标识重复请求（冻结、只认记下的系数）答单位不一致，被判成结果不一致。**修法**：`EvaluatePricing` 在自身重量那一遍成功后当场把换算记录与解释行记在评价上（所有提前收尾的出口因此都带着）；走到正式计价重量那一遍时以那一遍的记录为准、撤掉这一遍留的解释行（`withoutLines`），记录与解释都只有一份——不能直接沿用这一遍的记录，因为带抬高下限时正式那一遍保留的小数位可能更细，冻结重放要认的是正式那一遍记下的。ADR-0347 决定一的那句假设（「随后正式那一遍会统一记下」）已改写。
- 用例（`package_weight_early_exit_test.go`）：磅输入 × 读自身计价重量的条款 × 序列缺失（PENDING `REFERENCE_SERIES_UNRESOLVED`）/ 拒收命中（UNRATABLE `EXCLUDED_BY_RATE_CARD`）/ 抬高条件求值出错（CONFLICT `WEIGHT_UNIT_MISMATCH`），各钉「换算记录在、记录与解释行条数相等、重放同状态同摘要、快照往返后重放同样」；带抬高下限的完成评价记录与解释行各只一份且重放同摘要。应用层 `evaluate_pricing_early_exit_conversion_test.go`：拒收命中的磅件，同标识重复请求认作已有结果（`EvaluationExistingResult`），不判冲突、不重复入册。**变异**：去掉当场记录那一行，三条领域用例与应用层用例都失败（失败信息是评审复现的那个：换算数 0）。原有用例一个字未动，既有 Completed 重放用例（`TestChargeableWeightReadsTheConvertedWeightAndDoesNotRecordTheConversionTwice` 等）照过。
- 评审附带的「冻结模式下『体积重=』解释行小数位可能与原评价不同」：对不齐——冻结取记下的位数，而记下的是带下限那一遍的；只在「票级主体、合计体积重以另一个单位声明、换算结果是无限小数、卡又带抬高下限」这一格里才不同，语义摘要不含解释行；已在 ADR-0347 Consequences 写了一句。

**非阻断三条一并做了**
1. ADR-0347 风险点 7 改成如实：旧构建读到含 `ADDRESS_NATURE` 条件的 `PPC-5` 卡，答的是泛化的「价卡登记快照无效」；序列 / 目录登记与复核后的受影响范围扫描是租户级的（整笔失败、信封回滚，波及周更燃油序列），价卡草稿列表整租户报错，评价请求与售价预估在唯一适用的卡读不回时答 unavailable；写明运维约束「所有读价卡的进程换成新构建之后，才登记第一张读 `ADDRESS_NATURE` 的卡」，与 ADR-0348 风险点 6 对齐。Context 里「规范化版本是全局一个常量」一句补上 ADR-0348 之后的现状，决定六说明读新特征的方案仍写 `PPC-5`、与 `PPC-6` 互不依赖。
2. ADR-0346 Status 行索引注记补「越权风险点 2 的回答」（无条款停用，指向 ADR-0347 决定五），Links 补一条。
3. `CHARGEABLE_WEIGHT` 命名风险：导入模板条件表 `source` 列说明与导入规格对应行补「不含条件最低计价重量的抬高」（入库模板重生成）；ADR-0347 风险点 0 记明「现在没有已发布卡依赖这个量，是最便宜的改名窗口，交 PP owner」。

**自验（修订轮，含 PG）**：`go build ./...`、`go vet`（`./internal/parcelpricing/...`、`./cmd/...`、`./scripts/...`）、`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/architecture/... ./cmd/...`（带 DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）42 个有测试的包全部 ok、退 0（含 `internal/architecture` 门禁、`cmd/parcel-api` 真库用例、`cmd/parcel-pricing-template` 入库模板与生成器逐字节相等）；管理台 `tsc -b` 退 0、`pnpm test` 905 项全过（含 B 的改动）、`vite build` 过。`*_test.go` 对起点（`7ea1aa677` 的合并基）只新增，删除列全为 0。AT-PP-023 的行与盘点 ① 桶同步补了新用例。

## 评审记录（2026-10-29，隔离子代理，钉 `ce5526f92`）

阻断 1 条，已修。**B1**：换算后提前收尾的评价（拒收命中、序列缺失待判断、抬高条件求值出错）丢了换算记录，重放与同标识重复请求会判成 CONFLICT——评审用 50 磅 × 读自身计价重量 × 序列缺失 / 拒收命中两个探针复现，FULL-02 本身具备这三个要素，与 AT-PP-020 及 ADR-0346 决定三相抵。作者修（`d4eaf592b`）：自身重量那一遍成功后把换算记录与解释行当场记在评价上，走到正式计价重量那一遍时以那一遍为准并撤掉前一遍的解释行（记录与解释行各只一份）；新用例三种提前收尾 × 重放 × 快照往返重放各一条、同标识重复请求一条；变异（去掉当场记录那一行）4 条用例全红；原有用例一字未动。其余：既有评价一字不变——测试文件七个全新增、删除列 0，`PPC-5` 规范化未动；对拍钩子 2378 行（278 个确定性键 2326 行逐行相同、52 行随机标识噪声同量级）、真库 HTTP 3020 个试算请求逐字相同、守卫 `readsPackageWeights` 白盒穷举 5491 个形状与独立期望一致；阈值换算 7616 个有理数用例全匹配（变异后 2472 例不匹配）；地址性质载荷只收 `COMMERCIAL` / `RESIDENTIAL`，不填读它的卡答输入不全而不套默认，动线 `SYN-SCOPE-01` 选卡不变；ADR-0152 部分停用写法合规；FULL-02 七票试算复跑与作者表一致。非阻断并已处理：风险点 7 低估了旧构建影响（租户级、草稿列表整租户报错），改如实并写运维约束；ADR-0346 补指针；`CHARGEABLE_WEIGHT` 不含条件最低计价重量抬高的命名风险写进导入模板列说明与 ADR 风险点 0（现在没有已发布卡依赖，是最便宜的改名窗口，交 PP owner）。非阻断未处理：`FeatureAddressType` 实际装偏远档位的术语债；GLOSSARY 没收「自身计价重量」「地址性质」；旧构建读到带 `ADDRESS_NATURE` 条件的 PPC-5 卡报的是泛化「快照无效」而不是「不认这一版」，可选让读该特征的方案也写 PPC-6。

## 进 main 记录（2026-10-29，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-pricingA` tip `b66f7fef3` 作出处（已重放到 main `05a05a34e`，与 B 的 6 个文件冲突取并集）；隔离树 cherry-pick，重生成 ADR 索引、AT 盘点计数与机制清点，两项 `-check` 退 0；全量 164 ok / 0 FAIL；种子一次性库退 0；推确切 SHA `6d7ada458`。
- 修订轮未再派评审（改动限于评审复现的路径，由变异用例守着，旧评价被守卫挡在外面）。

