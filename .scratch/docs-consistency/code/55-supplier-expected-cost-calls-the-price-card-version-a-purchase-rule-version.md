# 55 SA 把评价命中的价卡版本叫「采购规则版本」，注释还说它是 GLOSSARY「价格规则版本」的 PP 半边

Category: chore
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-cmtB
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) GL·G-02（归 A，附带代码命名与注释的跟改；修订票不改代码，故立本票）。

## 现象

- GLOSSARY「价格规则版本」（文档自洽 10a 之后）只指 `party-commercial` 拥有的商业价格政策版本，评价命中的那一版价卡是 GLOSSARY「可执行价卡版本」；settlement-accounting CONTEXT 同名词条改为指针，并写明本上下文不把评价命中的价卡版本也叫价格规则版本。
- 代码里供应商预期成本的「规则版本」一格存的是 PP 的定价方案版本，类型名与注释却按「采购规则版本」与「价格规则版本的 PP 半边」叫。

## 证据

- `internal/settlementaccounting/domain/supplier_expected_cost.go`：`PurchaseRuleVersionReference`，注释「指名采购规则版本」。
- `internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go`：这一格填 `evaluation.PlanReference().ID() + "@" + evaluation.PlanReference().Version()`，即定价方案版本。
- `internal/settlementaccounting/ports/ports.go`：`BuyEvaluationAdoption` 头注「RuleVersion 是评价命中的可执行价卡版本（GLOSSARY「价格规则版本」的 PP 半边）」——GLOSSARY 里已经没有这个半边。

## 三类

机制（命名）。产品策略、租户取值：无。

## 规模

S：类型与构造函数改名为可执行价卡版本一类的词（例如 `PriceCardVersionReference`），注释改指 GLOSSARY「可执行价卡版本」；存储列名不改（列名不是领域语言的出处，改名要迁移，不值）。

## 最小的诚实修法

只改 Go 标识符与注释，行为与库表不动；幂等三维的注释照改，把规则版本那一维写成价卡版本。

## 完成记录（2026-10-27，作者会话，分支 claude-cmtB，基 origin/main `f6a4a5298`）

先通读了这张票要改到哪：把 `PurchaseRuleVersionReference` 及构造函数改名，会牵动导出符号、HTTP 报文字段、库列。按"会改对外契约就只改注释与内部私有名"的口径，**本票只改了注释与私有名，对外名字一个都没动**；行为与对外契约零变动。

### 改了什么

- 注释：`domain/supplier_expected_cost.go` 的 `PurchaseRuleVersionReference`（改为"指名评价命中的可执行价卡版本"，写明存的是 parcel-pricing 的定价方案版本、不是 party-commercial 的价格规则版本，并写明类型名为何没动）、`CostCorrectionSpec`；`ports/ports.go` 的 `BuyEvaluationAdoption`（原"GLOSSARY「价格规则版本」的 PP 半边"改指 GLOSSARY「可执行价卡版本」）、`ExpectedCostSaveOutcome`、`ExpectedCostRegistry.LoadFirstVersion`（幂等三维写成发生项、费用项目、价卡版本；UC-SA-002 的原句"……和规则版本不得重复形成预期成本"仍逐字保留为引文，并注明那个"规则版本"就是这一维）；`ports/supplier_cost_occurrence_state.go`、`ports/catalogue_read.go`（`SupplierExpectedCostCatalogueRow`）、`application/form_supplier_expected_cost.go`、`application/form_requested_supplier_expected_cost.go`、`adapters/inbox/buy_evaluation_recorded_consumer.go`、`adapters/parcelpricing/form_on_buy_evaluation_recorded.go`、`adapters/http/query_settlement_charges.go`（`supplierExpectedCostBody` 与"无采购规则版本"）、`adapters/postgres/supplier_expected_cost.go`，以及六个测试文件里同一维的叫法。
- 内部私有名：`SupplierExpectedCost` 的私有字段 `ruleVersion` → `priceCardVersion`（含 `FormSupplierExpectedCost`、`AppendCorrection`、`supplier_cost_rehydration.go` 里的赋值），postgres 适配器里的 `ruleVersion` 局部变量、`expectedCostColumns.ruleVersion` 与 `LoadFirstVersion`/`ExpectedCostRegistry.LoadFirstVersion` 的形参，`adapters/parcelpricing/buy_evaluation.go` 的局部变量，两处测试夹具形参。

### 没动的对外名字与原因（剩余项）

1. 导出类型 `domain.PurchaseRuleVersionReference`、构造函数 `NewPurchaseRuleVersionReference`、导出字段与访问器 `SupplierExpectedCostSpec.RuleVersion`、`CostCorrectionSpec.RuleVersion`、`BuyEvaluationAdoption.RuleVersion`、`SupplierExpectedCost.RuleVersion()`，以及构造函数里的错误文本 `"purchase rule version reference"`。这些是导出符号，`cmd/parcel-dispatch/supplier_cost_on_occurrence_invalidated_test.go` 与 SA 的十几个文件引用它们；它们又与下面两条同名，单独改会留下错位。
2. HTTP 报文字段 `purchaseRuleVersion`（`adapters/http/query_settlement_charges.go` 的 `supplierExpectedCostBody`，`apps/admin-web/src/pages/settlement/api.ts` 正在读它）及 `ports.SupplierExpectedCostCatalogueRow.PurchaseRuleVersion`。字段名是管理台已读的契约，且本任务禁碰 `apps/admin-web`。
3. 库列 `purchase_rule_version`（迁移 0008，已施加、受 checksum 固定，票面也明说列名不改）。

三处写进了各自的注释（`PurchaseRuleVersionReference`、`SupplierExpectedCostCatalogueRow`、`supplierExpectedCostBody`），读码的人不会再把这一格当 party-commercial 的价格规则版本去溯源。

后续如要把名字改干净，一次性处理需要覆盖：类型与构造函数改名（机械替换，十几个文件，含上面那份 `cmd/parcel-dispatch` 测试）、`RuleVersion` 字段与访问器改名、`purchaseRuleVersion` 报文字段改名（管理台 `api.ts` 与显示处同批）、库列改名（需新迁移）。这是对外契约变更，应由 SA owner 裁；我没有立后继票。

### 代决

1. 私有名只改与这一格直接对应的 `ruleVersion`，没有碰分摊规则版本（`AllocationRuleVersionReference`）、金额规则版本（`AmountRuleVersionReference`）那两族——它们叫"规则版本"是对的。
2. UC-SA-002 原文"同一发生项、费用项目和规则版本不得重复形成预期成本"里的"规则版本"仍是文档的叫法，文档不在本票范围，注释里用引文原样保留并点明它指这一维。

### 越权风险点

- 私有字段已叫 `priceCardVersion`、导出访问器仍叫 `RuleVersion()`，名字暂时不一致；这是有意的过渡形态，由上面第 1 条的后续改名收掉。
- 未改动 `apps/admin-web`：`pages/settlement/ChargesBillingPage.tsx` 的栏头仍写"采购规则版本"（同文件两处注释也是），需在 admin-web 的下一轮里随报文字段一并对齐。

### 评审记录与修订轮（协调方）
- 评审可合入：38、40、45 三个提交的每个改动的 `.go` 文件语法树完全相同；55 的 7 个文件差异全是 `ruleVersion` → `priceCardVersion` 的私有标识符重命名；38 条新增引文机械核对 36 条逐字相同，另 2 条是带省略号的前缀引文与假想读者的一句话；新增的 ADR/UC/AT 指针逐条核过。
- 修了评审点出的两处注释（位置词「下面那个 default」、不支撑断言的 ADR-0026 指针）和一处「所以名字没有单独动」的变更叙述；在 `RuleVersion()` 访问器上补了一行注释指向类型注释；管理台结算费用册的栏头与两处说明由「采购规则版本」改为「价卡版本」（只改展示文字，报文字段 `purchaseRuleVersion` 不动）。
- 协调方裁定：导出类型 `PurchaseRuleVersionReference`、报文字段 `purchaseRuleVersion`、库列 `purchase_rule_version`、`ports.SupplierExpectedCostCatalogueRow.PurchaseRuleVersion` 保留旧名，不做改名。理由：它们是对外契约与已施加的迁移，改名要一条新迁移、十几个文件与一次报文变更，换来的只是名字一致；误导读码者的根源（注释把它说成 PC 的采购价格政策）已由注释订正，类型注释与访问器注释都直说它是 parcel-pricing 的定价方案版本。以后若有一次因别的原因要动这几处的契约，再顺手改名。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-cmtB` 与 code/38、40、45、55 同批，由协调方快进，tip `c6f983ea2`；隔离评审（Sonnet）可合入，无阻断：每个改动的 `.go` 文件在去掉注释后语法树逐个比对（程序放评审私有目录，对有代码变动的老提交试过会报差异），38、40、45 三个提交 0 差异；新增的 38 条带引号或「」的引文机械核了 36 条逐字相同，另 2 条是带省略号的前缀引文与一句假想读者的话；新增的 ADR、UC、AT 指针逐条核过。协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点（本批无变动）；全量 `scripts/test-full.sh` 退 0、159 个包 ok；`go build ./...`、`go vet ./...` 干净；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 过）、`vite build` 与 `./internal/architecture/...` 全过。无迁移、无新 ADR、无 AT 号变化。
- 本票只改注释与私有名（`ruleVersion` → `priceCardVersion`，评审语法树比对确认差异仅限 7 个文件的私有标识符），导出类型 `PurchaseRuleVersionReference`、报文字段 `purchaseRuleVersion`、库列 `purchase_rule_version`、`ports.SupplierExpectedCostCatalogueRow.PurchaseRuleVersion` 保留旧名——协调方裁定不改名：它们是对外契约与已施加的迁移，改名要一条新迁移、十几个文件与一次报文变更，换来的只是名字一致；误导读码者的根源（注释把它说成 PC 的采购价格政策）已由注释订正，类型注释与访问器注释都直说它是 parcel-pricing 的定价方案版本。协调方另把管理台结算费用册的栏头与两处说明由「采购规则版本」改为「价卡版本」（只改展示文字）。
- 去向：无新票。
