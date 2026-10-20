# 105 两条以上候选都出价时，未选中候选的成本出处也交给计划，段号重复，初始路由计划形成报错

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg105（2026-10-19 作者子代理，见文末「完成记录」；隔离评审一轮阻断 0，见文末「评审记录」；进 main 时由协调方转 resolved 并注合入 SHA）。此前 ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（出自票 [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md) 的隔离评审；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘：NR `adapters/parcelpricing/cost_source.go`（`LoadCandidateCosts` 的合成）、`ports` 的 `RouteCandidateCosts`、`application/create_initial_route.go`（`planRecord`）。
出处：票 [routing-first-cut/10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md) 完成记录「出处留痕：选中候选逐段……随计划 jsonb 一并落」；[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 文末补记越权风险点 6。

## 现象

候选成本取数侧把**每一条**已出价候选的逐段出处都交回，编排原样交给计划；计划只容得下选中那一条的出处。两条以上候选都出价、排序选中其一时，初始路由计划形成报错，整次处理报错——既不是未决，也不是无路由。

演示网络只有一条候选（`SYN-LINE-CN-SG-01@1`），所以演示动线没有暴露。真实租户的多线路、多供应商比价正是这一格。

## 证据（读码，钉 `5f0aa66d`，证据 `S`）

- `RouteCandidateCostAdapter.LoadCandidateCosts` 逐候选合成，已出价候选的出处一律追加进 `RouteCandidateCosts.Citations`；待判断、不可计价的候选不留出处。
- 出处 `PlannedLegCostCitation` 只带段号，不带候选。
- `CreateInitialRouteHandler.planRecord` 把整份 `costs.Citations` 原样交 `FormInitialRoutePlan`。后者要求出处段号互不重复、不越过选中候选的段数，不然答 `ErrInvalidRoutePlan`，编排上抛 `form initial route plan`。
- 于是两条候选各自第 1 段的出处段号都是 1，计划形成必报错。选中候选段数比另一条已出价候选少时，越界那一条也触发同一错。
- 取数侧单测 `TestTwoCandidatesPriceIndependentlyWithTheirOwnCards` 已显示两条候选各交回一条段号 1 的出处；编排层没有「两条候选都出价」的用例，计划形成那一层也就没人撞上。
- 票 routing-first-cut/20 把出处的评价标识从 NR 按「候选 + 段号」铸的形状换成 parcel-pricing 派生的 `EST-…`，从标识上更认不出一条出处属于哪条候选。

## 三类

机制。规模 S。

## 最小的诚实修法

取数侧不知道哪条会被选中（排序在编排里），所以出处要带候选维：`RouteCandidateCosts` 的出处按候选分开交回（出处本身加候选，或按候选键分组），编排在排序选中之后只取选中候选那一份交给计划。计划形状与 `FormInitialRoutePlan` 的段号校验不变。

完成判据：

- 编排层用例：两条候选都出价、选中便宜那条，计划形成，出处只有选中候选的各段。
- 取数侧用例：两条候选的出处各自认得出属于哪条候选。

## 要裁的

- 未选中候选的出处要不要也随计划留痕（审计「为什么没选它」）。按票 routing-first-cut/10 的口径（「选中候选逐段」）只留选中候选那一份，不改计划形状。要留就得让计划的出处带候选维、段号校验按候选分，那是另一种计划形状，归 NR owner 裁。

## 既有票

routing-first-cut/10（逐段成本出处随计划留痕）、routing-first-cut/20（出处的评价标识改由 PP 派生）。

## 完成记录（2026-10-19，作者子代理，分支 `claude-dcg105`）

### 做了什么

- NR 端口：`RouteCandidateCosts.Citations` 的元素换成 `CandidateLegCostCitation`，即候选加原来的逐段出处（嵌入 `domain.PlannedLegCostCitation`）。另加 `RouteCandidateCosts.CitationsOf(candidate)`，按候选取逐段出处。
- 取数侧 `LoadCandidateCosts`：已出价候选的每条出处都标上它属于哪条候选。待判断、不可计价的候选照旧不留出处。
- 编排 `judgeParcel`：排序选中之后，只把 `CitationsOf(selected)` 交 `planRecord`。计划形状、`FormInitialRoutePlan` 的段号校验和库面 jsonb 都不变。
- 文档：UC-NR-001 立 `AT-NR-073`；AT 盘点在 ① 补一行并写追记。
- 立票 [115](./115-route-reassessment-never-loads-candidate-costs.md)：路由复核从不取候选成本。

### 代决

用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-19 同意做分诊 A 组。

- **候选维加在出处上，不按候选键分组成 map。** 交回顺序保留；单候选的生产装配真库用例（`len(costs.Citations)`、逐条读方案引用）不用改，照样能验。候选维只加在端口这一层：领域的出处、计划和落库形状都不动，计划本来就只属于一条候选。
- **选中候选那一份由编排取。** 取数侧不知道会选谁，排序在编排里；「计划只留选中候选」这一条只在 `planRecord` 的调用处写一次。
- **「要裁的」那一格：未选中候选的出处不随计划留痕。** 照票 routing-first-cut/10 的口径（「选中候选逐段」），计划形状不改。
- **不新立 ADR。** 这次修的是代码偏离已有口径的 bug，没有新增难逆转的取舍。
- **AT 号只用 `AT-NR-073`。** 协调方预分了 073–076，074–076 没用。新立票号只用了 115，116 没用。
- **用例落在应用层和取数侧，不加真库多候选用例。** 演示网络只有一条候选，真库上要再造一条线路和一张成本卡。本票库面不变，真库那一层多证不出什么；两条候选并存的路径由编排用例走真的 `FormInitialRoutePlan` 来证。

### 越权风险点

1. **计划不留未选中候选的金额和出处**（NR owner）。计划里的候选列表只有结局与淘汰原因，候选成本事实不落库，选路比价的评价也不入册（`routeCostEvidence` 那条注释：用完即弃）。所以要回答「为什么没选它」，只能按判断时点重放取数链。若要留，计划出处就得带候选维、段号校验按候选分开，落库形状随之改，算另一种计划形状。
2. **`AT-NR-073` 把口径写进了 UC 验收表**（NR owner）。「未选中候选的金额只作为候选成本事实参与排序，出处不随计划留痕」原先只写在票 routing-first-cut/10 的完成记录和领域注释里。现在 UC 上有了明文，owner 改口径时要连 UC 一起改。
3. **端口形状变了**（NR owner）。`RouteCandidateCosts.Citations` 的元素类型换了。实现方只有 `adapters/parcelpricing` 的取数侧和 `cmd/parcel-dispatch` 里恒报错的那一只，都已对齐。测试替身只交 `Facts`，不受影响。

### 用例

- 编排：`TestTwoPricedCandidatesFormAPlanCitingOnlyTheSelectedOne`（`AT-NR-073`），三个子格：两条候选段数相同、选中那条段数更少、选中那条段数更多。临时把整份出处交给计划复现过修前的行为：三格都报 `form initial route plan: network routing: invalid initial route plan`。
- 取数侧：`TestEachCitationNamesTheCandidateWhoseLegItPrices`（`AT-NR-073` 取数侧半边）。一段和两段的两条候选都出价、第三条缺依据：每条出处认得出所属候选，段号按各自的段链从 1 计，待判断的候选没有出处。
- 既有用例不改，照常通过：取数侧单候选用例、生产装配真库用例（`reference_network_wiring_test.go`、`route_cost_reading_completion_wiring_test.go`）。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 都干净。
- `scripts/test-full.sh`，跑的包：`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-network-register/`、`./internal/networkrouting/...`、`./internal/parcelshipment/adapters/networkrouting/`、`./internal/transportfulfillment/adapters/networkrouting/`、`./internal/visibilityexception/adapters/networkrouting/`、`./internal/architecture/...`，全部 ok。
- 演示种子、迁移和接线都没改；`docs/product/MECHANISM-INVENTORY.md` 没有重生成。

## 评审记录（2026-10-19，隔离评审一轮）

阻断 0 项。非阻断 2 项：建议进 main 前修的 1 项归协调方，进 main 时办；另 1 项修订时补跑了真库，已落实。本轮代码没改，只改票面。

### 建议进 main 前修的

1. **Status 还是 in-progress**（未改，留给协调方）。评审给的修法本来就指定协调方：进 main 时把 Status 转 resolved，并注明合入的 SHA。作者这边写不出合入 SHA，任务口径也要求作者收工时停在 in-progress。所以本轮只在 Status 行补一句，交代进 main 时谁来转、转时注什么。分诊表与 ADR-0189 越权风险点 6 都链到本票，转 resolved 之前，它们看到的本票仍是未完。

### 其余非阻断

2. **生产装配真库用例只编过、没起真库重跑**（已补跑）。评审只用 `go vet` 确认这两份用例编得过。修订时用 `scripts/test-full.sh -v -run` 把 `reference_network_wiring_test.go` 和 `route_cost_reading_completion_wiring_test.go` 里的用例逐个点名重跑，`-v` 输出里每个都是 PASS，没有跳过的。评审说进 main 的全量会顺带跑到它们，这一条不变。

### 验证（修订后）

- `go build ./...`、`go vet ./...` 干净；本分支改动的 Go 文件 `gofmt -l` 为空。
- `scripts/test-full.sh`，跑的包：`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-network-register/`、`./internal/networkrouting/...`、`./internal/parcelshipment/adapters/networkrouting/`、`./internal/transportfulfillment/adapters/networkrouting/`、`./internal/visibilityexception/adapters/networkrouting/`、`./internal/architecture/...`，全部 ok。这组包是改动包的全部反向依赖，修订时用 `go list` 重新算过，和完成记录里那组一样。
- 没跑全量，没重生成 `docs/product/MECHANISM-INVENTORY.md`；演示种子、迁移和接线都没改。
