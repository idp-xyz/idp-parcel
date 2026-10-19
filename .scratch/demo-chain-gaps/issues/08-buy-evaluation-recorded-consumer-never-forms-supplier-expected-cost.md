# 08 BUY 评价已记录的消费门对每一封都答来源引用未登记，供应商预期成本编排没装配

Category: enhancement
Status: 完工，待评审与进 main——分支 `claude-dcg08`（基于 `81a4c418`；2026-10-19 作者子代理落地，见文末「代决」与「完成记录」）。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

SA 的 BUY 评价已记录消费门对每一封都答 ErrSourceReferencesUnrecorded，供应商预期成本形成编排没装配

## 证据（读码）

internal/settlementaccounting/adapters/parcelpricing/form_on_buy_evaluation_recorded.go 的 HandleRecordedBuyEvaluation 末行恒返回 ErrSourceReferencesUnrecorded。cmd/parcel-dispatch/assemble.go 的 formSupplierExpectedCostOnBuyEvaluationConsumer 注释写「这里不装 NewFormSupplierExpectedCostHandler」「两头没接上」，已过时：PP 侧回指已在 PricingEvaluation.RequestReference，由 FormEvaluationFromRequestHandler 经 WithRequestReference 设置，sa-cc/11 已 resolved。saports.BuyEvaluationAdoption 没有请求回指字段；SA 没有 SupplierCostVersionID 铸造口（adapters/identity 只有 MintEvaluationRequestID）。

## 三类

机制。规模 M。

## 最小的诚实修法

BuyEvaluationAdoption 补带 RequestReference，由 BuyEvaluationAdapter 从 evaluation.RequestReference() 取。消费门按回指读 sapostgres.EvaluationRequests.FindByID，从记录里的 Sources 组成 FormSupplierExpectedCostCommand，铸一个版本 ID：新增铸造口，或由请求 ID 确定性派生，要定。在 dispatch 装 NewFormSupplierExpectedCostHandler 和 sapostgres.NewSupplierExpectedCosts，从未决名单里删掉 ErrSourceReferencesUnrecorded，改登 ExpectedCostUndecided 那几格。

## 要裁的

版本 ID 铸造还是由请求 ID 派生（SA owner）；PP 内部或重放形成、没有回指的 BUY 评价是答 nil 还是指名未决。

## 既有票

none（票 05 格 18 去处 sa-cc/11 已 resolved，票 05 重走一节已记「去处与代码对不上」）

## 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」，作者子代理代 SA owner 裁。决定落在 [ADR-0200](../../../docs/adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md)，这里只列结论与越权风险点。

1. **版本标识由请求派生，不铸造**：`BUYSC-<BUY 评价请求标识>`。核实过派生成立的前提：计价按回指至多为一份请求形成一份评价（计价迁移 0010 的部分唯一索引），评价不可变，所以重放与并发算出同一个标识，预期成本库主键挡第二份。`SupplierCostVersionID` 会随计价纠错换版本，但派生的只是首版，纠错版本由纠错那条路给标识，不冲突。与 SELL 一侧 `SELLCC-<请求标识>` 同一做法。类别段常量以 `Prefix` 结尾，进全仓标识前缀门禁守唯一。
2. **没有回指的 BUY 评价**：答「不归本编排」、零写入、入账。
3. **回指非空却不在 BUY 请求册上**：按协调方更正，作两侧矛盾上抛、落 `publish_failed`，不入账（请求与信封同库先于评价提交，不存在可见性滞后）。同构先例 [ADR-0198](../../../docs/adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定四。
4. **评价停下的各格入账、不重投、不改记任何册**：不可计价、评价冲突、待判断、评价失败、跨币种没带换算步骤、卡没声明金额取整。同三维已有另一标识的首版也入账。理由：评价不可变，计价不会为同一份请求再形成一份，重投只耗失败预算（ADR-0094 的判据）。
5. **金额取整未声明改口**：原先 BUY 读口报错、落 `publish_failed`；现改为读口交回「金额未声明」一格、带回指、不带金额，形成编排记未决原因 `EVALUATION_AMOUNT_PRECISION_UNDECLARED`，外层判为定局入账。与 SELL 一侧（ADR-0198 决定二）同口。BUY 请求册没有状态列，按协调方口径「入账并在票面写清楚」，**没开迁移**。
6. **其余矛盾**：派生版本上已有另一份内容、已完成的评价形成时却未受理（例如合计为零）都上抛。BUY 请求不带结算币种，不设币种核对；跨币种仍由形成门按评价内换算步骤判（`AT-SA-177`）。
7. **未决带根因**：只有 BUY 请求册、评价读口、预期成本库答不出才是未决；结果带依赖原样答出的错误，消费门的哨兵包装里用 `%w` 一并包着。

### 越权风险点（详见 ADR-0200）

1. 评价停下无痕：BUY 请求册无状态列，结算侧分不出「计价没形成评价」与「形成了但采用不了」，要按回指去计价读评价。
2. 不可计价不存排除依据：`AT-SA-175` 要求保存排除依据与评价引用，本路不落行。
3. 请求卡死：评价采用不了之后补登取整声明或汇率序列也不回补。
4. 同三维已有首版即止：发生项新版本的请求只答「已有首版」，不追加计价纠错。
5. 回指查不到就上抛：假定 BUY 评价回指只指向 BUY 请求册。

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg08` 落地。开工基点是 SELL 批次旧 tip `2ae29573`，按协调方要求 `rebase --onto origin/main` 到 `81a4c418` 之后再提交。证据级别：隔离合成，只记 `S`。

提交：

- `4e479bd0` 版本标识由 BUY 请求派生（只动 domain）。
- `67e2ff73` BUY 评价已记录按回指形成供应商预期成本，派发接上形成路（ports、adapters/parcelpricing、application、inbox 用例、`cmd/parcel-dispatch`）。读口签名与消费门构造一起改，拆开会让中间一笔 `go build` 不过，所以合成一笔。
- `f6fe7a0c` 注释去掉跨文件计数。
- `3718f0a0` ADR-0200 与文档同步。

### 做了什么

1. **领域**：`EvaluationRequest.SupplierExpectedCostVersionID()`。
2. **读口**（`ports.BuyEvaluationAdoption`、`BuyEvaluationAdapter`）：采用快照带回指 `Request`（照 BUY 请求身份读，各格都带，没有回指是零值）；新一格 `BuyEvaluationAmountUndeclared`，不带金额；`ErrAmountPrecisionUndeclared` 退役。
3. **形成编排本体**（`FormSupplierExpectedCostHandler`）：多金额未声明一格；结果多 `UndecidedCause()`，依赖答不出时交原错误，形成门拒时交领域哨兵，评价自己停下时为 nil。未决原因词表多了金额未声明与 BUY 请求册答不出，外层与本体共用一个词表。
4. **形成编排外层**（新，`FormRequestedSupplierExpectedCostHandler`）：读回指 → 读 BUY 请求 → 以请求上的合格来源引用与派生版本标识调本体 → 折结果。结果分已采用（含重放）、已有首版、评价采用不了、不归本编排、未决。本体的未决按原因分两边，不留 default。
5. **消费门**（`FormOnBuyEvaluationRecordedAdapter`）：接外层；未决哨兵是 `ErrEvaluationNotVisible`（读口那一段）与新的 `ErrSupplierCostFormationUndecided`（编排那一段），包装都带根因；`ErrSourceReferencesUnrecorded` 退役。
6. **派发**：`formSupplierExpectedCostOnBuyEvaluationConsumer` 装真 BUY 请求册、真预期成本库与两层编排；`buyEvaluationRecordedUndecidedSentinels` 换成上面的哨兵并重写头注；路由仍是 `FanOut(BUY, SELL)`，SELL 那一路没动。过时注释（「不装 NewFormSupplierExpectedCostHandler」「两头没接上」）随之改掉。
7. **顺手同步**：SELL 读口与 `SellEvaluationOutcome` 头注里「与 BUY 读口不同的一处」「SELL 这一侧多出的一格」已不成立，改掉。
8. **文档**：ADR-0200（索引加行；ADR-0164 Links 加前向指针）；UC-SA-002 步 4 补一句、`AT-SA-182` 至 `AT-SA-184`；settlement-accounting `CONTEXT` 补一条。

### 用例

- 领域：派生标识只依请求标识，换来源引用与范围不变，换请求就变。
- 读口（`buy_evaluation_test.go`，真算 SYN 卡）：已完成与待判断都带回指；没有回指是零值；金额未声明交回那一格、带回指、不带金额。
- 本体（`form_supplier_expected_cost_test.go`）：金额未声明一格；依赖答不出的根因 `errors.Is` 可辨；换算步骤缺席的根因是领域哨兵；评价自己待判断没有根因；未决原因词表封闭。
- 外层（`form_requested_supplier_expected_cost_test.go`，替身）：
  - 回指齐 → 形成首版：版本 `BUYSC-EVREQ-SYN-1`，来源引用取自请求，金额与规则版本出自评价；重放答已采用、只存一次。
  - 同一发生项新版本的另一份请求 → 已有首版，交回先到者，零新增。
  - 没有回指 → 不归本编排，不读请求册、零写入；回指不在册 → `ErrBuyEvaluationRequestNotRecorded`、零写入。
  - 不可计价、评价冲突、待判断、未形成、缺换算步骤、金额未声明 → 采用不了、零写入、本体那一格原样交回、不带未决原因。
  - 请求册、评价读口、预期成本库答不出 → 未决，原因各自指名，根因可辨。
  - 派生版本上已有另一份内容 → `ErrSupplierCostVersionContradiction`；合计为零 → `ErrUnexpectedRequestedExpectedCostFormation`；缺依赖拒。
- 消费门（`form_on_buy_evaluation_recorded_test.go`，真读口真两层编排、内存册）：形成并重放仍一份；SELL 评价与没有回指的 BUY 评价入账零写入；待判断与金额未声明入账零写入；回指不在册原样上抛、不是未决；评价读不回、读口、请求册、预期成本库答不出各带根因；词汇表之外与空白引用；缺依赖拒。
- 真库消费门 + inbox（`adapters/inbox/buy_evaluation_recorded_formation_test.go`，只有评价读口是替身）：一封信形成恰一份，同一封重投被 inbox 跳过、另一封指同一份评价按派生版本重放，都不多形成；回指不在册时整笔回滚、inbox 不记账、零写入。
- 真库派发装配（`cmd/parcel-dispatch/buy_evaluation_recorded_formation_test.go`，生产路由表一拍）：SA 真请求册落 `EVREQ-SYN-DCG08`；PP 真评价库与真交接落回指在册、没有回指、SELL、回指不在册的评价各一份、各交一封信，另入队一封指 PP 还没有的评价。
  - 回指在册的那份形成恰一份 `BUYSC-EVREQ-SYN-DCG08`：来源引用取自请求，USD 1250（评价合计 12.5 按 0.01 换写），规则版本 `SYN-CARD-BUY@v1`。
  - 没有回指的 BUY 评价、SELL 评价都 PUBLISHED，两路 inbox 各一行，零写入。
  - 回指不在册 → `dispatch.publish_failed`，原文是 `ErrBuyEvaluationRequestNotRecorded`，BUY 那一路不记账；PP 没有的评价 → `dispatch.consumer_undecided`。
  - 取代了原「信封到未决」那条用例（`TestARecordedBuyEvaluationStopsUndecided…`），PP 评价夹具 `syntheticPricingEvaluation` 随之搬进新文件并多一个回指参数。SELL 那一路的真库装配用例未改，照绿。
- 变异实测：把「回指不在册」改成不归本编排、把评价待判断改成未决，外层、消费门、inbox 真库与派发真库用例各自转红；还原后全绿。

### 验证

在 `3718f0a0` 上，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：

- `go test -p 1 -count=1 ./internal/settlementaccounting/... ./cmd/parcel-dispatch/ ./internal/architecture/` 全部 `ok`；本票新增的真库用例逐条 `PASS`，没有 `SKIP`。`internal/architecture`（含标识前缀门禁、生产可达性棘轮）照绿。
- `go build ./...` 退 0，`go vet ./internal/settlementaccounting/... ./cmd/parcel-dispatch/` 退 0，`gofmt -l internal cmd` 空。
- 没跑全量，没重生成机制清点（留给进 main 时）。

### 偏离、取舍与待裁

1. **金额取整未声明的去向改了**：从 `publish_failed` 改为入账（见代决 5）。sa-cc/01 判断题 1 当时留的就是这个问题，这里按 ADR-0198 决定二的同口径裁了。
2. **BUY 请求册不记状态**：评价停下的各格在结算侧无痕，加状态列留给 SA owner（ADR-0200 越权风险点 1）。没为此开迁移。
3. **同三维已有首版**：答「已有首版」、入账；发生项有效性更正怎样驱动计价纠错不在本票（风险点 4）。
4. **读不回里混着不自愈的格**：读口把引用译不成计价侧标识、评价不属本租户也折成读不回、按未决重投。与 SELL 那一路同一处不精确，没在本票改。
5. **演示动线仍到不了 BUY 成本**：形成路已接上，但演示租户的 BUY 链上游还缺运输收费发生项的生产路径（[07](07-transport-charge-occurrences-have-no-production-path.md)）与 BUY 触发册登记（[10](10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)）；演示 BUY 卡若仍没声明金额取整（[09](09-demo-price-cards-declare-no-amount-rounding.md)），到了这里也只会落「采用不了」。端到端不在本票。
6. 机制清点没重生成；`CONTEXT-MAP` 的 `parcel-pricing → settlement-accounting` 一段没改（它没写 BUY 消费门的现状，不过时）。
