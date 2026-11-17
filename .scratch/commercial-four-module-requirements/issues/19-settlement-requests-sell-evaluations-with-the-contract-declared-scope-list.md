# 19 结算两条发起路按合同声明的销售价格来源请计价，两本采用册不再说计价范围

Category: enhancement
Status: resolved——2026-11-17 进 `main` `d31f2760d`，见「进 main 记录」。此前 in-progress——2026-11-17 Cursor 会话认领，分支 `cursor/cfmr-19-sell-evaluation-uses-contract-scope-list`（worktree `idp-parcel-cfmr19`），基线 `3243f281e`。此前 ready-for-agent
父票：[spec](../spec.md)
Blocked by: 票 [17](./17-sell-price-sources-are-fixed-with-the-acceptance-time-closure-and-read-by-settlement.md)、票 [18](./18-pricing-selects-the-price-card-across-an-ordered-scope-list.md)
地盘：`internal/settlementaccounting`（两条发起路、评价请求、停因与续办、两本采用册的登记）、`internal/parcelpricing/adapters/settlementaccounting`（计价读结算评价请求记录的那只适配器，只动它把列表译给计价的那一段；计价的领域与应用归票 18）、`cmd/parcel-settlement-register`、`docs/application/settlement-accounting`

对应要求：Contract「价卡授权」落到金额上的那一半。规则在结算 `CONTEXT.md` 讲接受前财务控制的那一条，与 ADR-0380 决定二、三、七。

## 现象

取证钉 `54b32c1b8`。受理前估价与包裹计费时点两条路找到结算账户之后，评价请求上的范围都取采用册上的那一格（`PricingScope: adoption.PricingScope()`）。两本册不可变，只有命令行一条登记路；ADR-0280 要求的两本同值的核，代码里没有。

## 要做的

1. **两条路读合同的列表。** 合同版本带销售价格约定时，请求带合同声明的有序列表，不读采用册上的范围。
2. **旧规范化版本的合同照旧。** 没有销售价格约定的合同，照旧读采用册上的范围，请求只带一个。
3. **显式不计销售价。** 包裹计费时点不发起销售评价，这一拍消费入账、不重投，不记发起停办；受理前估价答`未配置`，不拿零顶。
4. **请求的身份不动。** SELL 请求的自然键是（租户、包裹、计费时点）加代次，列表不进自然键；同一件包裹、同一计费时点只发起一次照旧。请求记录带整份列表，供计价回读；计价那一侧读这份记录的适配器把列表原样译给计价（票 18 留给本票的那一段）。
5. **停因与续办。** 请求停下时，停因照旧是答出来的那一格；另逐个记下问到过的每一个来源各自的那一格。带计价范围的「参数已登记」，其范围是问到过的来源之一、且这类登记解得开那个来源自己的那一格，即续办。续办出来的下一代照抄前一代的列表，不重走链。
6. **两本采用册的登记面。** 受理前估价采用只必填形态，SELL 计费时点采用只必填计费时点；计价范围可缺。已登的行不删不改。两本册之间的那一核只对同一户两本都带计价范围的行做。旧规范化版本合同遇到不带计价范围的行：估价答`未配置`，包裹计费时点记发起停办「采用行没有计价范围」，这一格没有登记解得开。
7. **留痕。** 估价的留痕从每份估价一个计价范围、一张卡，改成逐件带计价范围与价卡版本；预估客户费用带出用的是列表里的哪一个范围。
8. **确认事实的主要计费范围**取评价实际用上的那一个范围（ADR-0380 决定九），不取请求上的列表。

## 要裁的

- ADR-0280 的那道核还做不做（ADR-0380 越权风险点 8）：票 `docs-consistency/code/62` 是关掉还是只对旧形状照做，待 SA owner 定。
- 一个结算账户下同时有新旧两种形状的合同时，采用册上的范围对新形状的合同不被读。登记面要不要提示这一点。
- 逐来源记停因值不值（ADR-0380 越权风险点 13），以及同一户的费用带不同的主要计费范围之后对账与信用怎样读（越权风险点 14）。SA owner 复核之前照 ADR 做。

## 三类

全部是机制。哪个账户采用哪种形态、哪个计费时点，仍是租户取值。

## 完成判据

- 同一件包裹，估价所凭的解析与接受时固定的解析采用同一版合同：受理前估价与随后形成的预估客户费用出自同一份列表、同一个范围，金额相同。
- 合同发了新一版、列表不同：已接受的委托下的包裹仍按旧列表计费。
- 列表里前一个范围没有卡时停办不发生，落到后一个；全部落空时停办，任何一个范围登了新卡都续办。
- 请求停在列表中间的一个范围上：问到过的范围上登了解得开它自己那一格的参数，续办；排在其后、没有问到的范围上登卡，不续办。
- 旧规范化版本合同遇到不带计价范围的采用行：估价答`未配置`；包裹计费时点记发起停办「采用行没有计价范围」，任何登记都不续办它。
- 受理前估价两件包裹落在不同范围：留痕逐件带计价范围与价卡版本。
- 评价出自列表里的后一个范围：确认事实的主要计费范围是后一个，不是列表的第一项。
- 旧规范化版本合同的包裹，行为与今天相同。
- 合同声明不计销售价：计费时点不发起、不停办；要估价时答`未配置`。
- `UC-SA-002`「给开发的交接」里对应的那一行随之删去或改成现状。

## 进 main 记录

2026-11-17，照 `docs/agents/workflow.md`「进 main 的步骤」与 `AGENTS.md`「交付模式」。这条链怎样落，用户在票 16 上定过：评审无阻断就跑全量测试与种子并落 `main`。本票评审有阻断，改完才落；落之前没有再问。推送方就是作者会话（Cursor）。

- **`main` 上的 SHA**：`040a32814`、`21fba37f4`、`e6e58852e`、`d31f2760d`，底是 `3243f281e`。推的是验过的那个 SHA 本身，非强推；推后 `origin/main` 答 `d31f2760d`。
- **出处**：分支 `cursor/cfmr-19-sell-evaluation-uses-contract-scope-list`。落地时 `main` 仍是它的底，重放是空操作，SHA 没有换。
- **非作者评审**：一轮，两轴隔离子代理，评审基点 `3243f281e`，钉 `e6e58852e`。两轴各有阻断，已在 `d31f2760d` 改掉。逐条见 Comments。`d31f2760d` 自审，没有再送评审。
- **生成物**：没有重生成，门已从 CI 摘掉。
- **全量真库测试**：在 `d31f2760d` 上跑 `scripts/test-full.sh`，退 0；176 个包通过，没有失败。
- **灌一次性库**：`scripts/demo-seeds/seed.sh` 灌进一只专用的一次性库，退 0，库用后已删。迁移 `0059` 施加成功。
- **CI**：`d31f2760d` 那一次推送，Go 的各测试分片成功，admin-web 的测试失败——新的未形成原因 `ESTIMATE_SELL_PRICE_ARRANGEMENT_UNAVAILABLE` 在页面词表里没有中文。收口这一笔补上「销售价格约定读不到」，停因契约测试在本机重跑通过。
- **收口这一笔**：只改本票与 `spec.md` 的子票表，自审；全量测试与种子未跑。
- **还开着的**：票 20、21，见 `dox/work.md`「在做」。登记不带计价范围的行时要不要提示没有续办路，票面「要裁的」仍留着，没做。

## Comments

- 2026-11-12：由票 15 拆出。
- 2026-11-12（激活）：用户在问题卡上选定激活票 16 至 21，本票转 `ready-for-agent`；此前 `draft`，等 ADR-0380 进 `main`（已在 `486a4e8fe` 进）。仍等票 17、18。冻结清单「记录」里 2026-10-27 讲两本采用册登同一个范围的那一行，同笔注明：ADR-0380 之后那一项只对旧规范化版本合同的账户还要核，代码由本票承接。票 17、18 已进 `main`（`e51a02b26`、`3243f281e`），阻塞解除。
- 2026-11-17（做到这里，未收口）：包裹计费时点这一条发起路接上了合同的三种答复。
  1. **已做。** 合同有有序列表时，请求带这份列表，不读采用册上的范围（`TestTheRequestCarriesTheContractSellPriceSourcesNotTheAdoptionScope`）。续办照抄列表。显式不计销售价不发起、不记停办（`TestAnExplicitNoSellPriceDoesNotRequestOrStall`），消费侧入账不重投。旧合同仍读采用册；没登范围记发起停办 `SELL_BILLING_MOMENT_SCOPE_ABSENT`，任何登记都不续办（`TestAnOldContractWithNoAdoptionScopeStallsAndNothingResumesIt`）。请求的自然键没加列表。多于一项的列表落在 `sell_evaluation_request_pricing_scope`（迁移 `0059`），`pricing_scope_id` 仍是第一项；只带一个范围的行没有子行。计价读这份记录的适配器把列表译进 `Scopes`。两本采用册的计价范围改为可缺。生产装配接上了销售价格约定读口。
  2. **2026-11-17 续做，代码已补上，还没评审、没进 main。** 受理前估价读同一次控制所凭的解析：有列表就交给试算，显式不计销售价与旧合同没登范围都答未配置、不问试算（`TestTheEstimateAsksTheContractSellPriceSources`、`TestAnExplicitNoSellPriceAnswersUnconfigured`、`TestAnOldContractWithoutAnAdoptionScopeAnswersUnconfigured`）。逐件用上的来源与价卡写进试算（`TestAParcelKeepsTheSellPriceSourceItLandedOn`）。请求停下时把问到过的来源各自的停因记在 `sell_evaluation_request_source_stop`；续办按那一格自己的停因、并且范围是问到过的才取（`TestAPriceCardResumesOnlyAnAskedSellPriceSource`）。确认事实的主要计费范围取评价实际用上的来源（`TestThePrimaryChargingScopeIsTheScopeTheEvaluationUsed`）。两本采用册都带范围时，后登的一本范围不同即冲突、不写。`UC-SA-002` 在冻结的 `docs/` 里没改，现行句子写在 `dox/contexts/settlement-accounting.md`。
- 2026-11-17（不计销售价再前移）：账户没登、时点没采用时也不记发起停办（`TestAnExplicitNoSellPriceDoesNotStallWhenTheAccountIsMissing`）。约定读不回仍答未决。
- 2026-11-17（非作者评审，一轮）：两个隔离子代理，评审基点 `3243f281e`，钉 `e6e58852e`。照交付模式只评这一轮、只修挡上线的。
  1. **两轴都有阻断，已改（`d31f2760d`）。** 有逐来源停因之后，不带计价范围的登记不再核请求答出来的那一格，费用项目映射与取整未声明续办不到。改成：不带范围的登记照旧核请求那一格；带范围的登记只核问到过的那个来源自己的格子。没见过的来源结论词、对不上的解释行不再静默丢掉。测试 `TestAPriceCardResumesOnlyAnAskedSellPriceSource` 补了不带范围的两问。
  2. **非阻断，没有改。** 登记不带计价范围的行时，登记面没有提示没有续办路。票面「要裁的」已把要不要提示留给 SA owner。
  3. **评审者自报没做的。** 两轴都没跑测试。作者重跑了结算的领域、应用、读计价的适配器与真库适配器，其后又跑了全量真库测试。没有再送评审。
