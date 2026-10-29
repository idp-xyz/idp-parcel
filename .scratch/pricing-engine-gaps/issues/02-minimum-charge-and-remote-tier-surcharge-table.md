# 02 计价引擎没有单票合计的最低 / 最高收费，附加费价表读不到偏远档位

Category: enhancement
Status: in-progress——2026-10-29 作者完工，待隔离评审与进 main，分支 claude-pricingB（代码最后一笔 `6c7ca8ff8`）。
Blocked by: 无（接票 01 之外独立；ADR-0346 单位换算已在 main）

## 现象

取证于 main `7ea1aa677`，由完整合成价卡 `SYN-PLAN-CN-SG-FULL-01` 取证出（冻结清单 2026-10-29 条目的第②④格）：

1. 最低收费表达不了：CONTEXT 规则句要求最低 / 最高收费由版本化规则声明，`PricingPlanStructures` 却没有这一格，评价里能改写金额的只有逐行取整与合计取整。
2. 偏远一档二档只能各写一条附加费规则、各占一个费用代码：附加费价表（`TABLE_LOOKUP`）只按分区与计价重量查档，读不到偏远档位；「同卡内一个费用代码只能有一条附加费规则」。
3. 派单另问「v1 价卡的偏远附加是无条件的，怎么装配通过的」。核实结论写在 ADR-0348 Context：v1 的偏远附加是 `FixedChargeRule`（无条件固定规则，只能给定额），不是附加费规则；附加费规则（`NewSurchargeRule`）必须带触发条件、没有无条件写法，空组合被拒。两个说法都对，不矛盾；缺的是「按偏远档位查表」，不是「无条件」。本票不新增附加费规则的无条件写法。

## 做什么

设计与取舍在 [ADR-0348](../../../docs/adr/0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md)（会话代产品 owner、PP owner 接受，待复核）。

1. 方案层声明单票合计的下限与上限（卡币种、各端可缺、声明了金额取整时落在进位单位整倍数上）；评价在各行收齐之后、结算币种换算与合计取整之前施加：低于下限补一条 `MINIMUM_CHARGE` 补差行（金额是差额，各行相加仍等于合计），高于上限补一条 `MAXIMUM_CHARGE` 冲减行（方向 `DEDUCT`、金额仍为正，不引入负金额），范围内不补行但解释如实记一行。
2. 附加费价表可声明按偏远档位查档：一条规则一个费用代码，触发条件定哪些档位收费、价表定各档金额；解不出档位仍答 `REMOTE_TIER_UNRESOLVED`、价表缺该档答 `RATE_NOT_FOUND`，不套默认；不改变按分区查表的既有读法。
3. 不改变任何今天已算对的评价：新结构是规范化文档里的新键，照 ADR-0267 换号并取能表达内容的最低一版——带新结构的方案写 `PPC-6`，其余仍写 `PPC-5`，字节不动，本构建两版都认。
4. 演示：新建 `SYN-PLAN-CN-SG-FULL-03`（范围 `SYN-SCOPE-04`），不改 FULL-01。
5. 管理台费用代码册追加 `MAXIMUM_CHARGE`。

## 完成判据

- 领域单测：低于 / 恰好等于 / 高于下限、上限冲减、与折扣冲减行 / 互斥组 / 百分比费用 / 逐行加合计取整 / 只合计取整 / 结算币种换算 / 逐委托主体的先后；构造期的拒绝（币种、顺序、非正、整倍数、费用代码占用）；按档位查表的 T1 / T2 / T0、解不出档位、价表缺档、没绑档位目录；规范化版本与读回门；旧形夹具摘要钉子。
- 既有测试一字不改，既有评价、黄金用例、指纹、既有价卡（`SYN-PLAN-CN-SG-01`、`SYN-PLAN-CN-SG-FULL-01`）结果一字不变；跨树对拍。
- 真库：带下限的价卡以 `PPC-6` 登记装回，带补差行的评价读回并重放得同一摘要；一次性库灌种子退 0、七票试算与手算一致。
- UC-PP-001 的 AT 表补行、AT 盘点 ① 桶补行、CONTEXT 规则句改成现行（不挂票号）。

## 完成记录（2026-10-29 作者）

起点 `7ea1aa677`（origin/main）；分支 `claude-pricingB`。提交：`c8548d18d`（领域代码与用例）、`2dae10341`（演示卡三、种子步骤、cmd 手算用例、管理台词典）、`6c7ca8ff8`（接线棘轮调用点复位 + 逐委托与真库用例）；其后文档一笔（ADR-0348、CONTEXT、UC-PP-001、AT 盘点、本票面）。

**改了什么**
- `internal/parcelpricing/domain/total_bounds.go`（新）：`TotalChargeBounds`、`WithTotalChargeBounds`、施加（`adjust`）、与方案自洽的判（币种、整倍数、费用代码占用、按档位查档须绑档位目录）、补差行 / 冲减行契约。
- `plan_structures.go`：`PricingPlanStructures` 多可缺的 `totalBounds`；`SurchargeCalculation` 多查档维度 `TableKey`（缺省按分区）与 `NewRemoteTierTableLookupSurcharge`。
- `surcharge_execution.go`：查表分支按维度取分区或档位；`surchargeContext` 带评价解出的档位；按档位查表的解释多一段 `looked up by remote tier T1`。
- `evaluation.go`：`ChargeLineTotalBound` 费用行类型；附加费收齐之后、换算之前施加总额上下限；重放 / 自校门认 `PPC-5` 与 `PPC-6`；完成评价的守恒校验认补差行（码与方向配对、只能最后一行）。
- `fingerprint.go`、`plan.go`、`plan_snapshot.go`、`registered_price_card.go`、`price_card_draft.go`：`PPC-6` 常量、`supportedCanonicalization`、`requiredCanonicalization`（取能表达内容的最低一版）、规范化文档与快照各两处可缺键、读回门认两版且要求「记的版本 = 内容该写的最低一版」；评价摘要的版本随所评方案走。
- 演示：`seedgen/full_card_03.go`、`data/pricing/price-card-cn-sg-full-03.json`、`steps/165-pricing-full-card-minimum-and-tier.sh`、README 一行、FULL-01 里两处已失实的说明改成现行（只动注释与文档，种子输出不变）。
- `apps/admin-web/src/vocabulary/status.ts`：费用代码册追加 `MAXIMUM_CHARGE`（最高收费）。
- 无迁移、无库表。

**既有评价零改动的证据**
- 测试文件对起点 `7ea1aa677` 纯新增：`git diff 7ea1aa677 --numstat -- '*_test.go' '*.test.ts'` 列出的全是新增文件（领域包三份、真库一份、cmd 一份、管理台词典一份），删除列全为 0；没有一行既有测试被改。
- 跨树对拍：探针测试（只用两树都有的夹具与接口）同一份放进 `git archive origin/main` 解出的树与本分支各跑一遍，124 行逐行 diff 为空——覆盖空结构、固定规则含 `DEDUCT`、定额附加费、互斥组、按分区查表（命中与缺档）、按基数取百分比的燃油、逐行加合计金额取整、声明了全部旧结构的合成价卡、绑目录的卡（档位命中 / 未命中 / 无读数），每份方案配缺尺寸、多种尺寸、重件、分区缺失、超出价表段六类输入；每一行含方案内容摘要、登记快照散列、评价语义摘要、评价快照散列、解释散列、规范化版本、快照往返摘要，并对每份评价重放，重放均同摘要。起点树的探针输出与本分支的探针输出完全一致。
- 种子：`go run ./scripts/demo-seeds/seedgen` 重跑，既有各份种子文件（v1 售价卡、成本卡、FULL-01、各目录与序列）逐字节不变，只多出 `price-card-cn-sg-full-03.json`。
- 字面值钉子：`TestATPP035DigestsOfOldShapeFixturesAreTheOnesRecordedBeforeBoundsExisted` 把两份旧形夹具的方案内容摘要与评价语义摘要作为字面值钉住，值是在 origin/main 上算出来记下的。
- 受影响面：只有声明了总额上下限或按档位查表的方案；其余方案的规范化版本、摘要、快照、评价、解释一个字不变。

**代决与越权风险点**：ADR-0348（会话代产品 owner、PP owner 接受、待复核）越权风险点 0–9。另有本票层面的代决：
1. **ADR 号取 0348**：协调方点名；origin/main 与各 claude-* 分支最大号为 0346，0347 空着（预计 claude-pricingA 取），未占用。
2. **补差行是差额、不是整笔；冲减行金额为正、方向 `DEDUCT`**（决定二，风险点 1、3）：保住「各行按方向相加等于合计」；参考设计（Rating Runtime 的 `MINIMUM_ADJUSTMENT` / `MAXIMUM_CAP`、受控案例 `FDC-007` 至 `FDC-010`）是同一口径。
3. **与折扣、互斥组、百分比费用的先后**（决定二，风险点 2）：之后；补差行不进燃油一类费用的基数。
4. **总额对象是全部费用行之和**（风险点 0）：个别承运商最低收费只对运费，本版不接，需要基数声明时另立。
5. **价表缺某档位答待判断、不按零**（决定三）：`T0` 不收靠触发条件不列它，而不是价表里写零（否则每票市区件带一条零金额偏远费行）。
6. **价表维度放在查表型附加费计算上，不放在 `RateTableVersion` 上**：放价表上会让所有价表族、含基础价表的规范化文档都加键。
7. **换号取「能表达内容的最低一版」而不是同号改形**（决定四）：同号改形在首个租户实例进生产之前虽可（ADR-0267 决定三），但演示库已存 `PPC-5` 方案与评价、派单要求它们不动。
8. **演示下限取 120.99 CNY 而非 30**：基础价最低一档（0.5 kg，Z1）的合计也有 61.05，造不出低于 30 的合计；120.99 恰是 FULL-01「小件市区」一票（2 kg）的合计，好让「一票刚好等于下限」有一票真能造出的包裹。它在金额取整进位单位 0.01 的整倍数上。最高收费演示卡不加（机制随产品发布并有单元用例）。
9. **`CurrentCanonicalizationVersion()` 保留并仍返回 `PPC-5`**：既有用例拿它与普通方案的版本比，语义是「基础版本」；评价摘要遇到本构建不认的版本时退回它，也让接线棘轮有生产调用点（`TestNoNewProductionFactoryGoesUnwired` 在第一笔单独时会红，第三笔复位）。

**判据对用例**（AT-PP-030 至 040，盘点 ① 桶已补行）
- AT-PP-030 补差行 → `TestATPP030…`（合计 10 低于下限 25，补 15，行之和 25）、真库 `TestAnEvaluationWithAMinimumChargeSupplementIsReadBackAndReplaysEqual`。
- AT-PP-031 恰好等于 / 高于下限 → `TestATPP031…`（解释 `equal to the declared minimum` / `within the declared bounds`）。
- AT-PP-032 上限冲减 → `TestATPP032…`、`TestBothBoundsDeclaredOnlyOneSideCanFire`。
- AT-PP-033 先后 → `TestATPP033…`（折扣后补）、互斥组 / 燃油基数 / 取整两种 / 结算币种换算 / 逐委托主体各一例。
- AT-PP-034 拒绝 → `TestATPP034BoundsThatDoNotHoldAreRefused`、读回门 `TestATPP034TheBoundLineContractSurvivesTheEvaluationSnapshot`。
- AT-PP-035 不声明则如旧 → `TestATPP035…` 两条（含字面值钉子）。
- AT-PP-036 至 038 档位查表 → `TestATPP036…`（T1 12 / T2 28 / T0 不收，一条规则一个费用代码）、`TestATPP037…`（解不出档位、价表缺档）、`TestATPP038…`（没绑档位目录拒、取较大值操作数、按分区不变）。
- AT-PP-039 规范化版本 → `TestATPP039…` 两条、真库 `TestAPriceCardWithTotalBoundsIsRegisteredAndLoadedBackAsPPC6`。
- AT-PP-040 演示卡三 → `TestTheDemoFullCardThreeMinimumChargeAndTierTablePriceTheSevenKindsOfParcelByHand`。

**七票试算**（一次性库 `scripts/demo-seeds/seed.sh` 退 0，价卡以 `PPC-6` 登记入库，经真库的试算编排——价卡、目录、序列都从库里读——逐票列费用行；与手算及 cmd 用例一致；计价基准时点 2026-10-29，燃油序列当期 11%，始发邮编 200120，范围 `SYN-SCOPE-04`）

| 票 | 包裹 | 目的 | 费用行 | 合计 |
|---|---|---|---|---|
| ① 轻小件市区触发最低收费 | 1 kg，20×15×10 cm | 018956（Z1，T0） | 基础 73 + 燃油 8.03 + `MINIMUM_CHARGE` 39.96 | 120.99 |
| ② 小件偏远一档 | 2 kg，30×20×10 | 098123（T1） | 基础 109 + 燃油 11.99 + `REMOTE_AREA_SURCHARGE` 12 | 132.99 |
| ③ 小件偏远二档 | 2 kg，30×20×10 | 627123（T2） | 基础 109 + 燃油 11.99 + `REMOTE_AREA_SURCHARGE` 28 | 148.99 |
| ④ 大件超尺寸 | 3 kg，120×80×60 | 018956 | 基础 4195 + 燃油 461.45 + 超尺寸 35 | 4691.45 |
| ⑤ 重件超重 | 25 kg，40×30×20 | 018956 | 基础 937 + 燃油 103.07 + 超重 25 | 1065.07 |
| ⑥ 小件市区恰好等于下限 | 2 kg，30×20×10 | 018956 | 基础 109 + 燃油 11.99，不补行，解释记 `equal to the declared minimum 120.99 CNY` | 120.99 |
| ⑦ 南向市区触发最低收费 | 2 kg，30×20×10 | 238456（Z2，T0） | 基础 93 + 燃油 10.23 + `MINIMUM_CHARGE` 17.76 | 120.99 |

同一次试算里对 FULL-01（范围 `SYN-SCOPE-02`）两票对照：小件市区 120.99、小件 T2 148.99（`REMOTE_AREA_EXTENDED_SURCHARGE` 28），规范化版本仍 `PPC-5`，与 FULL-01 的 cmd 手算用例一致。试算输出存证在作者草稿区 `scratchpad/pricingB/trial-output.txt`。

**自验**（含 PG，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）
- `go build ./...`、`go vet`（`./internal/parcelpricing/...`、`./internal/platform/...`、`./internal/settlementaccounting/...`、`./internal/architecture/...`、`./cmd/...`、`./scripts/demo-seeds/...`）无输出；`gofmt -l internal cmd tools scripts` 无输出。
- `go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/settlementaccounting/... ./internal/architecture/... ./cmd/... ./tools/...`：带 DSN，58 个有测试的包全部 ok、退 0（含接线棘轮与可达性棘轮）；其后新增的逐委托与真库两个测试文件在 `internal/parcelpricing/...` 上单独跑过，过。
- 管理台：`node scripts/run-tests.mjs`（借主树的 node_modules）900 个用例全过，含新增的费用代码词典用例。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 只报生成物过期（ADR 索引新增一行、AT 盘点 ① 桶 960→971、合计 1763→1774、PP 21→32，用 `-dir` 指到临时副本试算得出），按「进 main 的步骤」由推送方重生成；`-check` 没有报缺行 / 多行。机制清点未跑。
- 起点之后 origin/main 又前进了若干笔（均为 admin-web）；`git merge-tree --write-tree HEAD origin/main` 无冲突。
- 一次性库 `pricingb_seed_1793257947` 建在 `idp-parcel-postgres-gate` 上，用后已删；未碰 5173、`:19080`、`demo_live`。

**留给协调方**
- ADR-0348 的 README 索引行、AT 盘点计数、机制清点：进 main 时重生成（本分支没动生成物）。UC-PP-001 的 AT 表与盘点 ① 桶里，本票的 AT-PP-030 至 040 紧接 AT-PP-021 之后，claude-pricingA 的 AT-PP-022 起也加在那一带，会有相邻的文本冲突，取并集即可。
- 冻结清单 2026-10-29 条目：第②格（无条件附加费 / 按偏远档位查附加费价表）与第④格（最低收费）本票补掉；第①③⑤格不在本票。第②格里「无条件附加费」的说法要改：附加费规则本就没有无条件写法，无条件定额走固定规则，缺的只是「按偏远档位查表」，已补。
- 不挡上线的新发现（不立票，请记入冻结清单）：
  1. 试算响应（`estimateChargeBody`）的费用行不带方向，管理台试算页会把 `MAXIMUM_CHARGE` 冲减行显示成正数（既有 `DEDUCT` 固定规则同病）；要补是给响应加方向字段并让页面按方向显示。演示卡三不含最高收费，所以演示里看不到。
  2. 价卡导入模板暂不支持总额上下限与按档位查档（R2：真实承运商价目表常有）；要接得另立模板格式的决定。
  3. 声明了总额上下限的卡会出 `MINIMUM_CHARGE` / `MAXIMUM_CHARGE` 费用行，结算的费用项目映射要有这两个码——租户取值，随租户上线登记，没登记按既有口径答未配置。
  4. 第一张带新结构的价卡登记之前，所有读价卡的进程必须已是认 `PPC-6` 的构建（滚动发布约束，ADR-0348 风险点 6）：眼下只是文档约束、代码里没有门；旧进程遇到 `PPC-6` 卡时各读价卡路径出错即停、不会算错金额，序列 / 目录登记与复核的受影响范围扫描是租户级的，会波及 `parcel-pricing-feed` 周更燃油序列。最小防护（登记 `PPC-6` 价卡前要求显式部署开关，默认关）列为后续加固，不在本轮。
- 共享草稿区的名字冲突：开工时我往共享 scratchpad 根写了 `dbname`、`seed.log`，并 `rm -rf` 重建了 `main-tree`，可能覆盖了别的会话同名文件；之后我已把自己的产物挪进 `scratchpad/pricingB/`。
- 隔离评审请重点看：`requiredCanonicalization` 的「记的版本必须等于内容该写的最低一版」是否会误伤某条读回路径；总额上下限在评价里的位置（附加费收齐之后、换算之前）与 `validCompletedCharges` 里补差行「只能最后一行」的约束；`PPC-6` 方案的评价摘要写 `PPC-6` 这一点。

## 修订轮（2026-10-29 作者，应协调方转来的隔离评审意见，评审钉 `32e652fc8`、阻断为 0）

只改文字，不动行为与既有测试。

1. 三处 ADR 决定号引错改正：`plan_structures.go` 的 `tableKey` 注释与 `fingerprint.go` 里查档维度那一键的注释改指决定三，`plan.go` 的 `valid()` 注释改指决定四，`remote_tier_table_test.go` 头注改指决定三。
2. ADR-0348 决定四如实写旧构建的读法：价卡登记快照与草稿答 `ErrCanonicalizationVersionUnsupported`；带补差行的 `PPC-6` 评价快照答泛化的 `ErrEvaluationSnapshotInvalid`；不带补差行的 `PPC-6` 评价旧构建照常读成完成。
3. ADR-0348 风险点 6 补各读价卡路径遇到 `PPC-6` 卡时的结果（试算整次 unavailable、评价请求与售价预估只在唯一适用卡是 `PPC-6` 时 unavailable、序列与目录登记 / 复核的受影响范围扫描租户级整笔失败），并明记这眼下只是文档约束、代码里没有门，最小防护列为后续加固。
4. 位置词与计数：`fingerprint.go` 的「见上」「只多两处」「那两处」改用符号名与结构名；`total_bounds_test.go` 的「下面两个摘要」、`seedgen/main.go` 的「绑上面已登记的」改指名；cmd 用例注释里的「七类包裹」去掉计数。`hashPricingEvaluation` 里退回 `CurrentCanonicalizationVersion()` 的那条走不到的回退，注释写明用途——给接线棘轮一个生产调用点。

自验（带 DSN）：`go build ./...`、`go vet`、`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/architecture/... ./cmd/parcel-pricing-register/...` 全部 ok。
